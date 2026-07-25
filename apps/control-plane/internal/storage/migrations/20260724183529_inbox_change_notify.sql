-- 收件箱变更通知(P1-C2):把 SSE 的每连接轮询换成 LISTEN/NOTIFY。
--
-- 此前每条收件箱 SSE 连接每 2 秒自己打一次库探测游标(PeekInboxChange,含
-- project_members EXISTS 子查询)。SSE 已提升到全局布局,每个打开的标签页一条连接,
-- 成本随标签页数线性增长;远程库上每次探测还要占用一个连接一整个 RTT。
--
-- 为什么放触发器而不是应用层发 NOTIFY:
-- 这里的 NOTIFY 是缓存失效信号,不是业务事实,放在 DB 不违反"业务状态在控制平面"。
-- 而 inbox_items 的写入点是分散的——除 inbox 自己的两条 upsert 外,tenant_team.sql
-- 也直接改它,历史迁移里还有两处 UPDATE。应用层逐点插桩漏掉任何一处,后果就是
-- "后端已有待办、界面不刷新"——这正是此前已经修过一次的严重缺陷。触发器只有一处,
-- 跨模块写入与将来的迁移写入都盖得住。
--
-- 载荷只放 tenant_id:通知不携带业务数据,客户端收到后仍走既有的 ListItems 重拉,
-- 授权与筛选不在流内复刻(与原轮询实现同口径)。
--
-- 注意 NOTIFY 不保证送达(监听连接断开期间的通知会丢),所以服务端保留低频兜底轮询,
-- 见 inbox handler。

-- AFTER ... FOR EACH STATEMENT 而不是 FOR EACH ROW:一次批量 upsert/取消动辄影响
-- 多行,逐行发通知会把同一次变更放大成几十条唤醒。语句级每次写入最多一条通知,
-- 客户端本来就是"收到即重拉",折叠不丢信息。
-- 语句级触发器拿不到 NEW/OLD 行,所以用 transition table 取受影响行的租户。
CREATE OR REPLACE FUNCTION notify_inbox_change_stmt() RETURNS TRIGGER AS $$
DECLARE
    changed_tenant UUID;
BEGIN
    IF TG_OP = 'DELETE' THEN
        SELECT tenant_id INTO changed_tenant FROM old_rows WHERE tenant_id IS NOT NULL LIMIT 1;
    ELSE
        SELECT tenant_id INTO changed_tenant FROM new_rows WHERE tenant_id IS NOT NULL LIMIT 1;
    END IF;
    IF changed_tenant IS NOT NULL THEN
        PERFORM pg_notify('inbox_changed', changed_tenant::text);
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION notify_inbox_change_stmt() IS
    '语句级收件箱变更通知:每次写语句最多广播一条 inbox_changed(载荷为 tenant_id),供控制平面 SSE 扇出;不携带业务数据。';

DROP TRIGGER IF EXISTS trg_inbox_items_notify_insert ON inbox_items;
CREATE TRIGGER trg_inbox_items_notify_insert
    AFTER INSERT ON inbox_items
    REFERENCING NEW TABLE AS new_rows
    FOR EACH STATEMENT EXECUTE FUNCTION notify_inbox_change_stmt();

DROP TRIGGER IF EXISTS trg_inbox_items_notify_update ON inbox_items;
CREATE TRIGGER trg_inbox_items_notify_update
    AFTER UPDATE ON inbox_items
    REFERENCING NEW TABLE AS new_rows
    FOR EACH STATEMENT EXECUTE FUNCTION notify_inbox_change_stmt();

DROP TRIGGER IF EXISTS trg_inbox_items_notify_delete ON inbox_items;
CREATE TRIGGER trg_inbox_items_notify_delete
    AFTER DELETE ON inbox_items
    REFERENCING OLD TABLE AS old_rows
    FOR EACH STATEMENT EXECUTE FUNCTION notify_inbox_change_stmt();

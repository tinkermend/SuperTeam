-- 飞书 outbox 变更通知(PR-2 唤醒):connector 长轮询 ListOutbox 在空队列时等待,
-- 写入 pending 后立刻被 NOTIFY 唤醒,避免固定 2s 轮询的卡更新延迟。
--
-- 与 inbox_changed 同构:载荷仅 tenant_id;NOTIFY 不保证送达,connector 仍应
-- 在 wait 超时后重查,作为兜底。
-- 铁律:connector 不连库——LISTEN 在控制平面进程内,HTTP 长轮询对外。

CREATE OR REPLACE FUNCTION notify_feishu_outbox_change_stmt() RETURNS TRIGGER AS $$
DECLARE
    changed_tenant UUID;
BEGIN
    IF TG_OP = 'DELETE' THEN
        SELECT tenant_id INTO changed_tenant
        FROM old_rows
        WHERE tenant_id IS NOT NULL
        LIMIT 1;
    ELSE
        -- 仅唤醒 pending 待投递(含新建与 requeue 回 pending);sent/failed 等不惊扰。
        SELECT tenant_id INTO changed_tenant
        FROM new_rows
        WHERE tenant_id IS NOT NULL
          AND status = 'pending'
        LIMIT 1;
    END IF;
    IF changed_tenant IS NOT NULL THEN
        PERFORM pg_notify('feishu_outbox_changed', changed_tenant::text);
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

COMMENT ON FUNCTION notify_feishu_outbox_change_stmt() IS
    '语句级飞书 outbox 变更通知:广播 feishu_outbox_changed(tenant_id),供控制平面长轮询唤醒 connector 消费。';

DROP TRIGGER IF EXISTS trg_feishu_outbox_notify_insert ON feishu_outbox;
CREATE TRIGGER trg_feishu_outbox_notify_insert
    AFTER INSERT ON feishu_outbox
    REFERENCING NEW TABLE AS new_rows
    FOR EACH STATEMENT EXECUTE FUNCTION notify_feishu_outbox_change_stmt();

DROP TRIGGER IF EXISTS trg_feishu_outbox_notify_update ON feishu_outbox;
CREATE TRIGGER trg_feishu_outbox_notify_update
    AFTER UPDATE ON feishu_outbox
    REFERENCING NEW TABLE AS new_rows
    FOR EACH STATEMENT EXECUTE FUNCTION notify_feishu_outbox_change_stmt();

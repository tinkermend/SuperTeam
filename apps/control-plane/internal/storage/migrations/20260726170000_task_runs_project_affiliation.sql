-- 运行必须归属项目（run-project affiliation spec 2026-07-26）
-- C2: task_runs 增加一等 project_id 列并三路回填；C6: 物理清理无归属历史运行；
-- C4b: 取消存量 open 态运行恢复收件箱事项（该恢复链已整体退役）。
--
-- 此前「运行属于哪个项目」没有 schema 承载，靠两级可失效的间接引用 join 出来：
-- project_tasks.digital_employee_run_id 单值指针（重试/返工会覆盖，被覆盖的旧 run
-- 即失联）+ tasks.params.metadata.anchor_project_id JSON 兜底。本迁移把归属提升为
-- NOT NULL 一等列，由三条派发路径（项目任务派发 / chat / 自动化）在写入时落值。

-- 1) 加可空列（先回填后收紧）
ALTER TABLE task_runs ADD COLUMN project_id UUID;
COMMENT ON COLUMN task_runs.project_id IS '归属项目 ID（不变量：任何运行必须归属项目；项目任务派发取 project_tasks.project_id，chat 取锚点项目）';

-- 2a) 回填第一路：project_tasks 当前指针
UPDATE task_runs tr
SET project_id = pt.project_id
FROM project_tasks pt
WHERE pt.tenant_id = tr.tenant_id
  AND pt.digital_employee_run_id = tr.id
  AND tr.project_id IS NULL;

-- 2b) 回填第二路：attempt 血缘（指针被重试覆盖的历史 run 由此找回归属）
UPDATE task_runs tr
SET project_id = pt.project_id
FROM project_task_attempts a
JOIN project_tasks pt
  ON pt.tenant_id = a.tenant_id
 AND pt.id = a.project_task_id
WHERE a.tenant_id = tr.tenant_id
  AND a.digital_employee_run_id = tr.id
  AND tr.project_id IS NULL;

-- 2c) 回填第三路：tasks.params.metadata 锚点（chat run 与部分旧 task run）
UPDATE task_runs tr
SET project_id = anchor.project_id
FROM tasks t
CROSS JOIN LATERAL (
    SELECT COALESCE(
        NULLIF(t.params #>> '{metadata,anchor_project_id}', ''),
        NULLIF(t.params #>> '{metadata,project_id}', '')
    ) AS raw
) candidate
CROSS JOIN LATERAL (
    SELECT CASE
        WHEN candidate.raw ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
        THEN candidate.raw::uuid
        ELSE NULL
    END AS project_id
) anchor
WHERE t.tenant_id = tr.tenant_id
  AND t.id = tr.task_id
  AND anchor.project_id IS NOT NULL
  AND tr.project_id IS NULL;

-- 3) C6: 物理清理三路回填后仍无归属的运行（A2 拍板）。实测 dev 库为 60 条
-- 全终态 E2E/smoke 产物；清理是加 NOT NULL 的前提。级联清 task_events 残迹；
-- execution ledger / 审计事实不动。tasks 行仅当其所有 run 均被清理时删除。
CREATE TABLE orphan_task_runs_cleanup AS
SELECT tr.tenant_id, tr.id AS run_id, tr.task_id
FROM task_runs tr
WHERE tr.project_id IS NULL;

DELETE FROM task_events te
USING orphan_task_runs_cleanup o
WHERE te.tenant_id = o.tenant_id
  AND te.task_id = o.task_id;

DELETE FROM task_runs tr
USING orphan_task_runs_cleanup o
WHERE tr.tenant_id = o.tenant_id
  AND tr.id = o.run_id;

DELETE FROM tasks t
USING (SELECT DISTINCT tenant_id, task_id FROM orphan_task_runs_cleanup) o
WHERE t.tenant_id = o.tenant_id
  AND t.id = o.task_id
  AND NOT EXISTS (
      SELECT 1 FROM task_runs tr
      WHERE tr.tenant_id = t.tenant_id AND tr.task_id = t.id
  );

DROP TABLE orphan_task_runs_cleanup;

-- 4) 收紧为 NOT NULL 并给项目维度读路径建索引
ALTER TABLE task_runs ALTER COLUMN project_id SET NOT NULL;
CREATE INDEX idx_task_runs_tenant_project ON task_runs (tenant_id, project_id, created_at DESC);

-- 5) C4b: standalone 失败恢复链（重试/确认关闭）已整体退役；取消存量 open 态
-- 事项，避免删代码后收件箱残留无 handler 的哑动作卡。resolved/cancelled 历史保留。
UPDATE inbox_items
SET status = 'cancelled', resolved_at = NOW(), updated_at = NOW()
WHERE item_type = 'digital_employee_run_recovery'
  AND status = 'open';

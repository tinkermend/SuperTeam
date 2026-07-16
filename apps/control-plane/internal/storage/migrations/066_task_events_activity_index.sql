-- 运行总览三期：跨员工动态流端点（ListDigitalEmployeeActivity）按 (tenant_id, created_at DESC, id DESC)
-- 排序 + since 游标过滤，补覆盖索引避免规模化后顺序扫描。
CREATE INDEX idx_task_events_tenant_created ON task_events(tenant_id, created_at DESC, id DESC);

COMMENT ON INDEX idx_task_events_tenant_created IS '跨员工动态流按租户+时间倒序读取与游标增量拉取';

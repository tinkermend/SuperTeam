ALTER TABLE project_plan_revisions
    ADD COLUMN created_event_id UUID;

COMMENT ON COLUMN project_plan_revisions.created_event_id IS '创建该计划版本时产生的项目事件ID，用于 ProjectCoordinator Continue-As-New 后恢复协调作业输出事件链。';

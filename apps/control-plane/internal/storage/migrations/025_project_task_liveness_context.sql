CREATE TABLE project_task_attempt_context_updates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    project_task_id UUID NOT NULL REFERENCES project_tasks(id) ON DELETE CASCADE,
    attempt_id UUID REFERENCES project_task_attempts(id) ON DELETE SET NULL,
    update_kind VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    delivery_mode VARCHAR(50) NOT NULL,
    created_event_id UUID REFERENCES project_events(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_project_task_context_update_delivery CHECK (
        delivery_mode IN ('hot_inject', 'next_attempt', 'waiting_human', 'cancel_and_replan')
    )
);

CREATE INDEX idx_project_task_context_updates_task
    ON project_task_attempt_context_updates(tenant_id, project_task_id, created_at DESC);

CREATE TRIGGER update_project_task_attempt_context_updates_updated_at
    BEFORE UPDATE ON project_task_attempt_context_updates
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

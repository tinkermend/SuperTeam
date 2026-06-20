DROP TRIGGER IF EXISTS update_execution_ledger_events_updated_at ON execution_ledger_events;

CREATE OR REPLACE FUNCTION create_execution_ledger_event(
    p_tenant_id UUID,
    p_team_id UUID,
    p_project_id UUID,
    p_project_task_id UUID,
    p_project_task_attempt_id UUID,
    p_event_type VARCHAR,
    p_source_type VARCHAR,
    p_source_id VARCHAR,
    p_actor_type VARCHAR,
    p_actor_id VARCHAR,
    p_runtime_node_id UUID,
    p_provider_type VARCHAR,
    p_provider_session_id VARCHAR,
    p_input_summary TEXT,
    p_output_summary TEXT,
    p_error_family VARCHAR,
    p_error_code VARCHAR,
    p_error_message TEXT,
    p_retryable BOOLEAN,
    p_artifact_refs JSONB,
    p_evidence_refs JSONB,
    p_metadata JSONB,
    p_occurred_at TIMESTAMPTZ,
    p_idempotency_key VARCHAR
)
RETURNS execution_ledger_events AS $$
DECLARE
    result execution_ledger_events%ROWTYPE;
BEGIN
    INSERT INTO execution_ledger_events (
        tenant_id,
        team_id,
        project_id,
        project_task_id,
        project_task_attempt_id,
        event_type,
        source_type,
        source_id,
        actor_type,
        actor_id,
        runtime_node_id,
        provider_type,
        provider_session_id,
        input_summary,
        output_summary,
        error_family,
        error_code,
        error_message,
        retryable,
        artifact_refs,
        evidence_refs,
        metadata,
        occurred_at,
        idempotency_key
    ) VALUES (
        p_tenant_id,
        p_team_id,
        p_project_id,
        p_project_task_id,
        p_project_task_attempt_id,
        p_event_type,
        p_source_type,
        p_source_id,
        p_actor_type,
        p_actor_id,
        p_runtime_node_id,
        p_provider_type,
        p_provider_session_id,
        p_input_summary,
        p_output_summary,
        p_error_family,
        p_error_code,
        p_error_message,
        p_retryable,
        COALESCE(p_artifact_refs, '[]'::jsonb),
        COALESCE(p_evidence_refs, '[]'::jsonb),
        COALESCE(p_metadata, '{}'::jsonb),
        COALESCE(p_occurred_at, NOW()),
        p_idempotency_key
    )
    ON CONFLICT (tenant_id, idempotency_key) DO NOTHING
    RETURNING * INTO result;

    IF FOUND THEN
        RETURN result;
    END IF;

    SELECT *
    INTO result
    FROM execution_ledger_events
    WHERE tenant_id = p_tenant_id
      AND idempotency_key = p_idempotency_key;

    IF FOUND THEN
        RETURN result;
    END IF;

    RAISE EXCEPTION 'create_execution_ledger_event could not insert or find idempotency key % for tenant %',
        p_idempotency_key,
        p_tenant_id;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION reject_execution_ledger_events_mutation()
RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'execution_ledger_events is append-only and does not allow %', TG_OP;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS prevent_execution_ledger_events_update ON execution_ledger_events;
CREATE TRIGGER prevent_execution_ledger_events_update
    BEFORE UPDATE ON execution_ledger_events
    FOR EACH ROW EXECUTE FUNCTION reject_execution_ledger_events_mutation();

DROP TRIGGER IF EXISTS prevent_execution_ledger_events_delete ON execution_ledger_events;
CREATE TRIGGER prevent_execution_ledger_events_delete
    BEFORE DELETE ON execution_ledger_events
    FOR EACH ROW EXECUTE FUNCTION reject_execution_ledger_events_mutation();

CREATE INDEX IF NOT EXISTS idx_execution_ledger_events_task_time
    ON execution_ledger_events(tenant_id, project_task_id, occurred_at ASC, created_at ASC)
    WHERE project_task_id IS NOT NULL;

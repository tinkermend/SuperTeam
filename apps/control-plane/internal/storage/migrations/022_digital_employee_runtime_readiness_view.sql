-- 数字员工 runtime 就绪读视图
-- 将 overview_rows CTE 中决定「可执行(runnable)」的事实与判定提升为真实视图,
-- 供协调器在选择执行人时按 runtime 就绪过滤,避免选中未绑定 runtime 的数字员工导致任务静默 stranded。
-- 判定与 GetDigitalEmployeeOverviewSummary 的 runnable_count FILTER 完全一致,杜绝两处漂移。

CREATE OR REPLACE VIEW digital_employee_runtime_readiness AS
WITH latest_runs AS (
    SELECT DISTINCT ON (tr.tenant_id, tr.digital_employee_id)
        tr.tenant_id,
        tr.digital_employee_id,
        tr.status
    FROM task_runs tr
    JOIN tasks t ON t.id = tr.task_id AND t.tenant_id = tr.tenant_id
    WHERE tr.digital_employee_id IS NOT NULL
      AND t.deleted_at IS NULL
    ORDER BY tr.tenant_id, tr.digital_employee_id, tr.updated_at DESC, tr.created_at DESC
),
effective_configs AS (
    SELECT DISTINCT ON (ec.tenant_id, ec.digital_employee_id)
        ec.tenant_id,
        ec.digital_employee_id,
        ec.id AS effective_config_id
    FROM digital_employee_effective_configs ec
    WHERE ec.status = 'approved'
      AND ec.revoked_at IS NULL
    ORDER BY ec.tenant_id, ec.digital_employee_id, ec.created_at DESC, ec.updated_at DESC
),
governance_configs AS (
    SELECT DISTINCT ON (ec.tenant_id, ec.digital_employee_id)
        ec.tenant_id,
        ec.digital_employee_id,
        ec.status
    FROM digital_employee_effective_configs ec
    WHERE ec.revoked_at IS NULL
    ORDER BY ec.tenant_id, ec.digital_employee_id, ec.created_at DESC, ec.updated_at DESC
),
provider_capabilities AS (
    SELECT DISTINCT ON (rc.tenant_id, rc.runtime_node_id, rc.provider_type)
        rc.tenant_id,
        rc.runtime_node_id,
        rc.provider_type,
        rc.available,
        rc.status,
        rc.health_status
    FROM runtime_capabilities rc
    WHERE rc.capability_type = 'provider'
      AND rc.disabled_at IS NULL
      AND rc.archived_at IS NULL
    ORDER BY rc.tenant_id, rc.runtime_node_id, rc.provider_type, rc.last_seen_at DESC NULLS LAST, rc.updated_at DESC
)
SELECT
    de.tenant_id,
    de.id AS digital_employee_id,
    (
        de.status IN ('ready', 'active')
        AND COALESCE(dei.status, 'missing')::text IN ('ready', 'active')
        AND ec.effective_config_id IS NOT NULL
        AND dei.runtime_node_id IS NOT NULL
        AND rn.status = 'online'
        AND rn.disabled_at IS NULL
        AND rn.archived_at IS NULL
        AND NULLIF(BTRIM(COALESCE(dei.agent_home_dir, '')), '') IS NOT NULL
        AND COALESCE(gc.status, 'missing')::text = 'approved'
        AND COALESCE(pc.available, false)::boolean = true
        AND COALESCE(pc.status, '')::text = 'healthy'
        AND COALESCE(pc.health_status, '')::text = 'healthy'
    )::boolean AS is_runtime_ready
FROM digital_employees de
LEFT JOIN digital_employee_execution_instances dei
  ON dei.tenant_id = de.tenant_id
 AND dei.digital_employee_id = de.id
 AND dei.deleted_at IS NULL
LEFT JOIN runtime_nodes rn
  ON rn.id = dei.runtime_node_id
 AND rn.tenant_id = dei.tenant_id
LEFT JOIN provider_capabilities pc
  ON pc.tenant_id = dei.tenant_id
 AND pc.runtime_node_id = dei.runtime_node_id
 AND pc.provider_type = dei.provider_type
LEFT JOIN effective_configs ec
  ON ec.tenant_id = de.tenant_id
 AND ec.digital_employee_id = de.id
LEFT JOIN governance_configs gc
  ON gc.tenant_id = de.tenant_id
 AND gc.digital_employee_id = de.id
WHERE de.deleted_at IS NULL;

COMMENT ON VIEW digital_employee_runtime_readiness IS '数字员工 runtime 就绪读视图,is_runtime_ready 与 overview runnable 判定一致,供协调器执行人选择过滤';

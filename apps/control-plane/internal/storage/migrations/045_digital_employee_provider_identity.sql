ALTER TABLE digital_employees
    ADD COLUMN provider_type VARCHAR(100);

WITH ranked_execution_instances AS (
    SELECT
        dei.tenant_id,
        dei.digital_employee_id,
        dei.provider_type,
        ROW_NUMBER() OVER (
            PARTITION BY dei.tenant_id, dei.digital_employee_id
            ORDER BY
                CASE
                    WHEN dei.status IN ('ready', 'active') THEN 0
                    WHEN dei.status = 'provisioning' THEN 1
                    ELSE 2
                END,
                dei.ready_at DESC NULLS LAST,
                dei.updated_at DESC,
                dei.created_at DESC,
                dei.id DESC
        ) AS provider_rank
    FROM digital_employee_execution_instances dei
    WHERE dei.deleted_at IS NULL
      AND dei.disabled_at IS NULL
      AND dei.status NOT IN ('disabled', 'error')
)
UPDATE digital_employees de
SET provider_type = COALESCE(
        ranked_execution_instances.provider_type,
        NULLIF(BTRIM(de.metadata->>'provider_type'), ''),
        'codex'
    ),
    updated_at = NOW()
FROM ranked_execution_instances
WHERE ranked_execution_instances.tenant_id = de.tenant_id
  AND ranked_execution_instances.digital_employee_id = de.id
  AND ranked_execution_instances.provider_rank = 1
  AND de.provider_type IS NULL;

UPDATE digital_employees
SET provider_type = COALESCE(NULLIF(BTRIM(metadata->>'provider_type'), ''), 'codex'),
    updated_at = NOW()
WHERE provider_type IS NULL;

ALTER TABLE digital_employees
    ALTER COLUMN provider_type SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_digital_employees_tenant_provider_status
    ON digital_employees(tenant_id, provider_type, status)
    WHERE deleted_at IS NULL;

ALTER TABLE project_task_attempts
    ADD COLUMN digital_employee_id UUID,
    ADD COLUMN provider_type VARCHAR(100);

UPDATE project_task_attempts pta
SET digital_employee_id = pt.assigned_digital_employee_id,
    provider_type = COALESCE(
        NULLIF(BTRIM(pta.execution_context_packet->>'provider_type'), ''),
        de.provider_type
    )
FROM project_tasks pt
LEFT JOIN digital_employees de
  ON de.tenant_id = pt.tenant_id
 AND de.id = pt.assigned_digital_employee_id
WHERE pta.tenant_id = pt.tenant_id
  AND pta.project_task_id = pt.id
  AND (
      pta.digital_employee_id IS NULL
      OR pta.provider_type IS NULL
  );

UPDATE project_task_attempts
SET provider_type = NULLIF(BTRIM(execution_context_packet->>'provider_type'), '')
WHERE provider_type IS NULL
  AND NULLIF(BTRIM(execution_context_packet->>'provider_type'), '') IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_project_task_attempts_digital_employee
    ON project_task_attempts(tenant_id, digital_employee_id, created_at DESC)
    WHERE digital_employee_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_project_task_attempts_provider_type
    ON project_task_attempts(tenant_id, provider_type, status, created_at DESC)
    WHERE provider_type IS NOT NULL;

COMMENT ON COLUMN digital_employees.provider_type IS '数字员工主 Provider 类型，由服务端 Provider 注册表校验，不使用数据库枚举。';
COMMENT ON INDEX idx_digital_employees_tenant_provider_status IS '按租户、Provider 类型和生命周期状态查询未删除数字员工。';
COMMENT ON COLUMN project_task_attempts.digital_employee_id IS '执行尝试实际分派的数字员工 ID，历史记录可为空。';
COMMENT ON COLUMN project_task_attempts.provider_type IS '执行尝试使用的 Provider 类型，优先来自上下文包或数字员工主 Provider。';
COMMENT ON INDEX idx_project_task_attempts_digital_employee IS '按数字员工查询执行尝试历史。';
COMMENT ON INDEX idx_project_task_attempts_provider_type IS '按 Provider 类型和尝试状态查询执行尝试。';

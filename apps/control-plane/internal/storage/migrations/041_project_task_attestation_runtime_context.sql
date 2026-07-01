ALTER TABLE project_task_attestations
    ADD COLUMN digital_employee_id UUID,
    ADD COLUMN capability_manifest_version VARCHAR(255),
    ADD COLUMN provider_auth_mode VARCHAR(40) NOT NULL DEFAULT 'host';

UPDATE project_task_attestations
SET digital_employee_id = COALESCE(project_tasks.assigned_digital_employee_id, '00000000-0000-0000-0000-000000000000'::uuid)
FROM project_tasks
WHERE project_task_attestations.tenant_id = project_tasks.tenant_id
  AND project_task_attestations.project_id = project_tasks.project_id
  AND project_task_attestations.project_task_id = project_tasks.id
  AND project_task_attestations.digital_employee_id IS NULL;

UPDATE project_task_attestations
SET digital_employee_id = '00000000-0000-0000-0000-000000000000'::uuid
WHERE digital_employee_id IS NULL;

ALTER TABLE project_task_attestations
    ALTER COLUMN digital_employee_id SET NOT NULL,
    ADD CONSTRAINT chk_project_task_attestations_provider_auth_mode
        CHECK (provider_auth_mode IN ('host', 'employee', 'explicit_credential'));

COMMENT ON COLUMN project_task_attestations.digital_employee_id IS '生成该执行证明的数字员工 ID；历史缺失记录使用全零 UUID 占位。';
COMMENT ON COLUMN project_task_attestations.capability_manifest_version IS '执行时使用的员工能力 manifest 版本。';
COMMENT ON COLUMN project_task_attestations.provider_auth_mode IS '执行时 Provider 认证模式：host、employee 或 explicit_credential。';

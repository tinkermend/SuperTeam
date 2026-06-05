ALTER TABLE digital_employees
    ADD COLUMN IF NOT EXISTS owner_user_id UUID,
    ADD COLUMN IF NOT EXISTS employee_type VARCHAR(100);

UPDATE digital_employees de
SET owner_user_id = COALESCE(
    (
        SELECT tm.principal_id
        FROM tenant_members tm
        WHERE tm.tenant_id = de.tenant_id
          AND tm.principal_type = 'user'
          AND tm.status = 'active'
          AND tm.disabled_at IS NULL
          AND (
              tm.team_id = de.team_id
              OR tm.team_id IS NULL
          )
          AND tm.role IN ('owner', 'admin', 'maintainer')
        ORDER BY
          CASE WHEN tm.team_id = de.team_id THEN 0 ELSE 1 END,
          tm.created_at ASC
        LIMIT 1
    ),
    (
        SELECT tm.principal_id
        FROM tenant_members tm
        WHERE tm.tenant_id = de.tenant_id
          AND tm.principal_type = 'user'
          AND tm.status = 'active'
          AND tm.disabled_at IS NULL
        ORDER BY
          CASE
              WHEN tm.team_id = de.team_id THEN 0
              WHEN tm.team_id IS NULL THEN 1
              ELSE 2
          END,
          tm.created_at ASC
        LIMIT 1
    )
)
WHERE de.owner_user_id IS NULL;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM digital_employees
        WHERE owner_user_id IS NULL
    ) THEN
        RAISE EXCEPTION 'digital_employees.owner_user_id unresolved before NOT NULL migration';
    END IF;
END $$;

UPDATE digital_employees
SET employee_type = CASE
    WHEN role ILIKE '%database%' OR role ILIKE '%db%' OR role ILIKE '%dba%' OR role ILIKE '%mysql%' OR role ILIKE '%postgres%' OR role ILIKE '%postgresql%' OR role ILIKE '%数据库%' THEN 'database_admin'
    WHEN role ILIKE '%devops%' OR role ILIKE '%ops%' OR role ILIKE '%sre%' OR role ILIKE '%platform%' OR role ILIKE '%infra%' OR role ILIKE '%运维%' THEN 'devops_engineer'
    WHEN role ILIKE '%frontend%' OR role ILIKE '%front-end%' OR role ILIKE '%前端%' THEN 'frontend_engineer'
    WHEN role ILIKE '%backend%' OR role ILIKE '%back-end%' OR role ILIKE '%server%' OR role ILIKE '%api%' OR role ILIKE '%后端%' THEN 'backend_engineer'
    WHEN role ILIKE '%fullstack%' OR role ILIKE '%full-stack%' OR role ILIKE '%全栈%' THEN 'fullstack_engineer'
    WHEN role ILIKE '%implementation%' OR role ILIKE '%implement%' OR role ILIKE '%delivery%' OR role ILIKE '%实施%' OR role ILIKE '%交付%' THEN 'implementation_engineer'
    ELSE 'general_engineer'
END
WHERE employee_type IS NULL OR employee_type = '';

ALTER TABLE digital_employees
    ALTER COLUMN owner_user_id SET NOT NULL,
    ALTER COLUMN employee_type SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_digital_employees_owner_status
    ON digital_employees(tenant_id, owner_user_id, status, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_digital_employees_type_status
    ON digital_employees(tenant_id, employee_type, status, created_at DESC)
    WHERE deleted_at IS NULL;

COMMENT ON COLUMN digital_employees.owner_user_id IS '数字员工归属人类用户ID，由控制平面从登录上下文写入';
COMMENT ON COLUMN digital_employees.employee_type IS '数字员工专业类型，由服务端注册表校验，不使用数据库枚举';
COMMENT ON INDEX idx_digital_employees_owner_status IS '按归属人和状态查询数字员工';
COMMENT ON INDEX idx_digital_employees_type_status IS '按专业类型和状态查询数字员工';

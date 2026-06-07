ALTER TABLE digital_employee_config_revisions
    ADD COLUMN IF NOT EXISTS budget_policy JSONB NOT NULL DEFAULT '{}'::jsonb;

COMMENT ON COLUMN digital_employee_config_revisions.budget_policy IS '数字员工预算策略，包含每日 token 上限；空对象表示无预算上限';

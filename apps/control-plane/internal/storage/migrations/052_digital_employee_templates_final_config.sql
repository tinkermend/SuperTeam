ALTER TABLE digital_employee_templates
    ADD COLUMN IF NOT EXISTS persona_memory_markdown TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS capability_bindings JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS budget_policy JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE digital_employee_templates
    DROP COLUMN IF EXISTS default_capability_selection,
    DROP COLUMN IF EXISTS default_context_policy_override,
    DROP COLUMN IF EXISTS default_approval_policy;

COMMENT ON COLUMN digital_employee_templates.persona_memory_markdown IS '模板预填的人格记忆 Markdown';
COMMENT ON COLUMN digital_employee_templates.capability_bindings IS '模板预填的数字员工能力绑定';
COMMENT ON COLUMN digital_employee_templates.budget_policy IS '模板预填的预算策略';

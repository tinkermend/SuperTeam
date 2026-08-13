-- Autonomy P2: binary automation rule autonomy tier (spec 2026-08-13 §4 / §9).
-- Default pause_at_gate = 遇闸暂停; full_auto must be explicit at create/patch.

ALTER TABLE automation_rules
    ADD COLUMN IF NOT EXISTS autonomy_tier VARCHAR(32) NOT NULL DEFAULT 'pause_at_gate';

ALTER TABLE automation_rules
    DROP CONSTRAINT IF EXISTS chk_automation_rules_autonomy_tier;

ALTER TABLE automation_rules
    ADD CONSTRAINT chk_automation_rules_autonomy_tier
    CHECK (autonomy_tier IN ('pause_at_gate', 'full_auto'));

COMMENT ON COLUMN automation_rules.autonomy_tier IS
    '自治档位：pause_at_gate（缺省，遇闸停车等人）| full_auto（闸照触发但策略自动放行，resolved_by=policy:{rule_id}）';

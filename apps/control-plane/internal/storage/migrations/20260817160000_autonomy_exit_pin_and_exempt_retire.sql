-- Autonomy F6: pin exit / acknowledge tiered semantics for full_auto multi-exit
-- playbooks (spec 2026-08-17 §6.5.6). Also strip retired acceptance_human_judgment_exempt
-- from project coordination_policy JSON (§5).

ALTER TABLE automation_rules
    ADD COLUMN IF NOT EXISTS pinned_exit_deliverable VARCHAR(128),
    ADD COLUMN IF NOT EXISTS acknowledge_exit_tier_semantics BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN automation_rules.pinned_exit_deliverable IS
    'full_auto + 多出口剧本时可选钉死的 exit deliverable；与 acknowledge_exit_tier_semantics 二选一';
COMMENT ON COLUMN automation_rules.acknowledge_exit_tier_semantics IS
    '创建者显式确认「浅档自动、深档停人」的分档语义；未 pin exit 时用于满足预签完整性';

ALTER TABLE external_integrations
    ADD COLUMN IF NOT EXISTS pinned_exit_deliverable VARCHAR(128),
    ADD COLUMN IF NOT EXISTS acknowledge_exit_tier_semantics BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN external_integrations.pinned_exit_deliverable IS
    '同 automation_rules.pinned_exit_deliverable';
COMMENT ON COLUMN external_integrations.acknowledge_exit_tier_semantics IS
    '同 automation_rules.acknowledge_exit_tier_semantics';

-- Retire acceptance_human_judgment_exempt from live project policies.
UPDATE projects
SET coordination_policy = coordination_policy - 'acceptance_human_judgment_exempt',
    updated_at = NOW()
WHERE coordination_policy ? 'acceptance_human_judgment_exempt';

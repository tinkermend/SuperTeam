-- Autonomy P5: external API integrations (spec 2026-08-13 §8/§8.1).
-- Pre-authorized execution entry with envelope: binding pins project/employee/
-- skills/playbook/tier at grant time; enforcement is a LIVE reference — effective
-- tier and skill surface are recomputed per call against current project/playbook
-- ceilings (tighten applies immediately; loosening never auto-upgrades).

CREATE TABLE external_integrations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL,
    name VARCHAR(200) NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    allow_chat_run BOOLEAN NOT NULL DEFAULT FALSE,
    allow_demand_submit BOOLEAN NOT NULL DEFAULT FALSE,
    -- Envelope: skill ids projected on every chat run; validated against the
    -- project Chat skill surface at call time (live intersection).
    skill_ids JSONB NOT NULL DEFAULT '[]'::jsonb,
    scenario_template_key VARCHAR(128),
    autonomy_tier VARCHAR(32) NOT NULL DEFAULT 'pause_at_gate',
    -- P5 first-version budget: per-integration fixed-window hourly hard limit.
    max_calls_per_hour INTEGER NOT NULL DEFAULT 60,
    budget_window_start TIMESTAMPTZ,
    budget_window_count INTEGER NOT NULL DEFAULT 0,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    -- Grantor: runs/demands created through this integration act as this user
    -- (same model as automation_rules.actor_user_id).
    created_by_user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_external_integrations_autonomy_tier
        CHECK (autonomy_tier IN ('pause_at_gate', 'full_auto')),
    CONSTRAINT chk_external_integrations_status
        CHECK (status IN ('active', 'disabled')),
    CONSTRAINT chk_external_integrations_budget
        CHECK (max_calls_per_hour > 0)
);

CREATE INDEX idx_external_integrations_tenant
    ON external_integrations (tenant_id);

CREATE INDEX idx_external_integrations_tenant_project
    ON external_integrations (tenant_id, project_id);

COMMENT ON TABLE external_integrations IS
    '外部 API 集成绑定：带信封的预授权执行入口（两动词：信封内 chat run / 提交 plan|loop demand）。活引用：调用时现算 Effective(剧本上限, 项目上限, 绑定档) 与技能面交集。';
COMMENT ON COLUMN external_integrations.autonomy_tier IS
    '开通时选定的自治档；调用时受项目/剧本上限单向收紧（收紧即时生效，放松不自动升档）';
COMMENT ON COLUMN external_integrations.max_calls_per_hour IS
    'P5 第一版预算：每小时调用数硬闸（固定窗口计数），超限 429';

-- Dedicated integration tokens (decision §8.1-2): separate lifecycle from
-- auth_service_tokens. Plaintext is `xi_` + 64 hex chars (32 random bytes);
-- stored as SHA-256 hex for deterministic indexed lookup — safe for
-- high-entropy random tokens (unlike passwords), avoids per-request bcrypt scans.
CREATE TABLE external_integration_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    integration_id UUID NOT NULL,
    token_sha256 CHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ,
    CONSTRAINT chk_external_integration_tokens_status
        CHECK (status IN ('active', 'revoked'))
);

CREATE UNIQUE INDEX uq_external_integration_tokens_sha
    ON external_integration_tokens (token_sha256);

CREATE INDEX idx_external_integration_tokens_integration
    ON external_integration_tokens (integration_id);

COMMENT ON TABLE external_integration_tokens IS
    '外部集成专用 token（与 connector service token 分家）：明文仅签发时返回一次，可独立吊销';

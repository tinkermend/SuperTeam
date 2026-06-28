-- Task prompt templates: reusable prompt scaffolds with dynamic variables.
-- Scope: SYSTEM (whole tenant), TEAM (team_id), PERSONAL (creator_id).
-- Soft-delete + partial indexes follow the 037 baseline.

CREATE TABLE IF NOT EXISTS task_prompt_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    category_code VARCHAR(64) NOT NULL DEFAULT 'general',
    scope VARCHAR(16) NOT NULL,
    team_id UUID,
    creator_id UUID NOT NULL,
    variables JSONB NOT NULL DEFAULT '[]'::jsonb,
    use_count INTEGER NOT NULL DEFAULT 0,
    deleted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ck_task_prompt_templates_title_not_blank   CHECK (btrim(title)   <> ''),
    CONSTRAINT ck_task_prompt_templates_content_not_blank CHECK (btrim(content) <> ''),
    CONSTRAINT ck_task_prompt_templates_scope             CHECK (scope IN ('SYSTEM', 'TEAM', 'PERSONAL')),
    CONSTRAINT ck_task_prompt_templates_team_scope        CHECK (
        (scope = 'TEAM' AND team_id IS NOT NULL) OR
        (scope IN ('SYSTEM', 'PERSONAL') AND team_id IS NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_task_prompt_templates_tenant_active
    ON task_prompt_templates(tenant_id, use_count DESC, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_task_prompt_templates_tenant_system_title_active
    ON task_prompt_templates(tenant_id, title)
    WHERE scope = 'SYSTEM' AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_task_prompt_templates_tenant_team_title_active
    ON task_prompt_templates(tenant_id, team_id, title)
    WHERE scope = 'TEAM' AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_task_prompt_templates_tenant_personal_title_active
    ON task_prompt_templates(tenant_id, creator_id, title)
    WHERE scope = 'PERSONAL' AND deleted_at IS NULL;

-- Team constitution becomes a first-class team column; backfill from the
-- currently-active governance revision. Rename skill_team_bindings to the
-- team-first convention (team_<subject>_bindings).

ALTER TABLE tenant_teams
    ADD COLUMN IF NOT EXISTS constitution JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE tenant_teams t
SET constitution = r.constitution
FROM tenant_team_config_revisions r
WHERE r.tenant_id = t.tenant_id
  AND r.team_id = t.id
  AND r.status = 'active';

ALTER TABLE skill_team_bindings RENAME TO team_skill_bindings;

-- Migration: 011_team_multiple_owners
-- Description: Convert human_owner_user_id to human_owner_user_ids array

-- 1. Add new column to tenant_teams
ALTER TABLE tenant_teams ADD COLUMN human_owner_user_ids UUID[] NOT NULL DEFAULT '{}'::uuid[];

-- 2. Migrate existing data in tenant_teams
UPDATE tenant_teams 
SET human_owner_user_ids = ARRAY[human_owner_user_id] 
WHERE human_owner_user_id IS NOT NULL;

-- 3. Drop old column from tenant_teams
ALTER TABLE tenant_teams DROP COLUMN human_owner_user_id;

-- 4. Add new column to tenant_team_config_revisions
ALTER TABLE tenant_team_config_revisions ADD COLUMN human_owner_user_ids UUID[] NOT NULL DEFAULT '{}'::uuid[];

-- 5. Migrate existing data in tenant_team_config_revisions
UPDATE tenant_team_config_revisions 
SET human_owner_user_ids = ARRAY[human_owner_user_id] 
WHERE human_owner_user_id IS NOT NULL;

-- 6. Drop old column from tenant_team_config_revisions
ALTER TABLE tenant_team_config_revisions DROP COLUMN human_owner_user_id;

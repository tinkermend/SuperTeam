CREATE TABLE digital_employee_environment_variables (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    team_id UUID NOT NULL REFERENCES tenant_teams(id),
    digital_employee_id UUID NOT NULL REFERENCES digital_employees(id),
    name TEXT NOT NULL,
    encrypted_value TEXT NOT NULL,
    encryption_key_id TEXT NOT NULL,
    value_fingerprint TEXT NOT NULL,
    sensitive BOOLEAN NOT NULL DEFAULT true,
    status VARCHAR(50) NOT NULL DEFAULT 'active',
    created_by UUID REFERENCES auth_users(id),
    updated_by UUID REFERENCES auth_users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT digital_employee_env_name_not_blank CHECK (btrim(name) <> ''),
    CONSTRAINT digital_employee_env_encrypted_value_not_blank CHECK (btrim(encrypted_value) <> ''),
    CONSTRAINT digital_employee_env_key_id_not_blank CHECK (btrim(encryption_key_id) <> ''),
    CONSTRAINT digital_employee_env_status_supported CHECK (status IN ('active', 'disabled'))
);

CREATE UNIQUE INDEX digital_employee_env_unique_active_name
    ON digital_employee_environment_variables (tenant_id, digital_employee_id, name)
    WHERE deleted_at IS NULL;

CREATE INDEX digital_employee_env_employee_idx
    ON digital_employee_environment_variables (tenant_id, digital_employee_id)
    WHERE deleted_at IS NULL;

CREATE INDEX digital_employee_env_team_idx
    ON digital_employee_environment_variables (tenant_id, team_id)
    WHERE deleted_at IS NULL;

ALTER TABLE skills
    ALTER COLUMN metadata SET DEFAULT '{}'::jsonb;

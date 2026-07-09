CREATE TABLE digital_employee_templates (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL REFERENCES tenants(id),
  type VARCHAR(64) NOT NULL,
  label VARCHAR(128) NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  default_role VARCHAR(128) NOT NULL DEFAULT '',
  recommended_skills JSONB NOT NULL DEFAULT '[]',
  recommended_mcp_servers JSONB NOT NULL DEFAULT '[]',
  recommended_provider_types JSONB NOT NULL DEFAULT '[]',
  default_capability_selection JSONB NOT NULL DEFAULT '{}',
  default_context_policy_override JSONB NOT NULL DEFAULT '{}',
  default_approval_policy JSONB NOT NULL DEFAULT '{}',
  metadata JSONB NOT NULL DEFAULT '{}',
  status VARCHAR(16) NOT NULL DEFAULT 'active',
  is_system BOOLEAN NOT NULL DEFAULT false,
  deleted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT digital_employee_templates_status_check CHECK (status IN ('active', 'disabled'))
);

CREATE UNIQUE INDEX digital_employee_templates_tenant_type_key
  ON digital_employee_templates (tenant_id, type) WHERE deleted_at IS NULL;

CREATE INDEX digital_employee_templates_tenant_status_idx
  ON digital_employee_templates (tenant_id, status) WHERE deleted_at IS NULL;

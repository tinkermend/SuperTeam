-- Project runtime eligibility set (chosen at creation) + soft per-employee
-- affinity used at dispatch. project_placements stays but no longer drives
-- node selection.
CREATE TABLE project_runtime_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    runtime_node_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_project_runtime_nodes_project
        FOREIGN KEY (tenant_id, project_id)
        REFERENCES projects(tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT uq_project_runtime_nodes UNIQUE (project_id, runtime_node_id)
);

CREATE TABLE project_employee_node_affinity (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    digital_employee_id UUID NOT NULL,
    runtime_node_id UUID NOT NULL,
    last_run_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_project_employee_node_affinity_project
        FOREIGN KEY (tenant_id, project_id)
        REFERENCES projects(tenant_id, id) ON DELETE CASCADE,
    CONSTRAINT uq_project_employee_node_affinity UNIQUE (project_id, digital_employee_id)
);
CREATE INDEX idx_project_runtime_nodes_project ON project_runtime_nodes (tenant_id, project_id);

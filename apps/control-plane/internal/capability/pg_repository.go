package capability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgRepository struct {
	q *queries.Queries
}

func NewPgRepository(q *queries.Queries) Repository {
	return &PgRepository{q: q}
}

func (r *PgRepository) CreateMCPServerDefinition(ctx context.Context, req CreateMCPServerDefinitionRequest) (MCPDefinition, error) {
	if err := r.requireQueries(); err != nil {
		return MCPDefinition{}, err
	}
	visibility, err := marshalProviderVisibility(req.ProviderVisibility)
	if err != nil {
		return MCPDefinition{}, err
	}
	row, err := r.q.CreateMCPServerDefinition(ctx, queries.CreateMCPServerDefinitionParams{
		TenantID:           req.TenantID,
		Name:               req.Name,
		ServerKey:          req.ServerKey,
		Description:        req.Description,
		Transport:          string(req.Transport),
		Url:                req.URL,
		AuthStrategy:       string(req.AuthStrategy),
		RequiredEnvVars:    req.RequiredEnvVars,
		OptionalEnvVars:    req.OptionalEnvVars,
		ProviderVisibility: visibility,
		ToolAllowlist:      req.ToolAllowlist,
		RiskLevel:          req.RiskLevel,
		Metadata:           nil,
		CreatedBy:          nullUUIDFromValue(req.UserID),
	})
	if err != nil {
		return MCPDefinition{}, err
	}
	return mcpDefinitionFromQuery(row), nil
}

func (r *PgRepository) ListMCPServerDefinitions(ctx context.Context, req ListMCPServerDefinitionsRequest) ([]MCPDefinition, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	rows, err := r.q.ListMCPServerDefinitions(ctx, req.TenantID)
	if err != nil {
		return nil, err
	}
	definitions := make([]MCPDefinition, 0, len(rows))
	for _, row := range rows {
		definitions = append(definitions, mcpDefinitionFromQuery(row))
	}
	return definitions, nil
}

func (r *PgRepository) GetMCPServerDefinition(ctx context.Context, tenantID, serverID uuid.UUID) (MCPDefinition, error) {
	if err := r.requireQueries(); err != nil {
		return MCPDefinition{}, err
	}
	row, err := r.q.GetMCPServerDefinition(ctx, queries.GetMCPServerDefinitionParams{
		TenantID: tenantID,
		ID:       serverID,
	})
	if err != nil {
		return MCPDefinition{}, mapNoRows(err)
	}
	return mcpDefinitionFromQuery(row), nil
}

func (r *PgRepository) DeleteMCPServerDefinition(ctx context.Context, req DeleteMCPServerDefinitionRequest) error {
	if err := r.requireQueries(); err != nil {
		return err
	}
	return r.q.DeleteMCPServerDefinition(ctx, queries.DeleteMCPServerDefinitionParams{
		TenantID: req.TenantID,
		ID:       req.ServerID,
	})
}

func (r *PgRepository) CreateTeamMCPBinding(ctx context.Context, req CreateTeamMCPBindingRequest) (MCPBinding, error) {
	if err := r.requireQueries(); err != nil {
		return MCPBinding{}, err
	}
	row, err := r.q.CreateTeamMCPBinding(ctx, queries.CreateTeamMCPBindingParams{
		TenantID:         req.TenantID,
		TeamID:           req.TeamID,
		McpServerID:      req.MCPServerID,
		CredentialEnvVar: textFromString(req.CredentialEnvVar),
		Metadata:         nil,
		CreatedBy:        nullUUIDFromValue(req.UserID),
	})
	if err != nil {
		return MCPBinding{}, err
	}
	teamID := row.TeamID
	return MCPBinding{
		ID:               row.ID,
		TenantID:         row.TenantID,
		TeamID:           &teamID,
		MCPServerID:      row.McpServerID,
		CredentialEnvVar: row.CredentialEnvVar.String,
		Status:           row.Status,
		SourceScope:      "team",
		CreatedAt:        timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:        timeFromTimestamptz(row.UpdatedAt),
	}, nil
}

func (r *PgRepository) ListTeamMCPBindings(ctx context.Context, req TeamScopedRequest) ([]MCPBinding, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	rows, err := r.q.ListTeamMCPBindings(ctx, queries.ListTeamMCPBindingsParams{
		TenantID: req.TenantID,
		TeamID:   req.TeamID,
	})
	if err != nil {
		return nil, err
	}
	bindings := make([]MCPBinding, 0, len(rows))
	for _, row := range rows {
		teamID := row.TeamID
		bindings = append(bindings, MCPBinding{
			ID:               row.ID,
			TenantID:         row.TenantID,
			TeamID:           &teamID,
			MCPServerID:      row.McpServerID,
			CredentialEnvVar: row.CredentialEnvVar.String,
			Status:           row.Status,
			ServerName:       row.ServerName,
			ServerKey:        row.ServerKey,
			URL:              row.Url,
			Transport:        MCPTransport(row.Transport),
			AuthStrategy:     MCPAuthStrategy(row.AuthStrategy),
			RequiredEnvVars:  row.RequiredEnvVars,
			RiskLevel:        row.RiskLevel,
			SourceScope:      "team",
			CreatedAt:        timeFromTimestamptz(row.CreatedAt),
			UpdatedAt:        timeFromTimestamptz(row.UpdatedAt),
		})
	}
	return bindings, nil
}

func (r *PgRepository) DeleteTeamMCPBinding(ctx context.Context, req DeleteTeamMCPBindingRequest) error {
	if err := r.requireQueries(); err != nil {
		return err
	}
	return r.q.DeleteTeamMCPBinding(ctx, queries.DeleteTeamMCPBindingParams{
		TenantID: req.TenantID,
		TeamID:   req.TeamID,
		ID:       req.BindingID,
	})
}

func (r *PgRepository) CreateEmployeeMCPBindingV2(ctx context.Context, req CreateEmployeeMCPBindingV2Request) (MCPBinding, error) {
	if err := r.requireQueries(); err != nil {
		return MCPBinding{}, err
	}
	row, err := r.q.CreateEmployeeMCPBindingV2(ctx, queries.CreateEmployeeMCPBindingV2Params{
		TenantID:          req.TenantID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		McpServerID:       req.MCPServerID,
		CredentialEnvVar:  textFromString(req.CredentialEnvVar),
		Metadata:          nil,
		CreatedBy:         nullUUIDFromValue(req.UserID),
	})
	if err != nil {
		return MCPBinding{}, err
	}
	employeeID := row.DigitalEmployeeID
	return MCPBinding{
		ID:                row.ID,
		TenantID:          row.TenantID,
		DigitalEmployeeID: &employeeID,
		MCPServerID:       row.McpServerID,
		CredentialEnvVar:  row.CredentialEnvVar.String,
		Status:            row.Status,
		SourceScope:       "employee",
		CreatedAt:         timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:         timeFromTimestamptz(row.UpdatedAt),
	}, nil
}

func (r *PgRepository) ListEmployeeMCPBindingsV2(ctx context.Context, req EmployeeScopedRequest) ([]MCPBinding, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	rows, err := r.q.ListEmployeeMCPBindingsV2(ctx, queries.ListEmployeeMCPBindingsV2Params{
		TenantID:          req.TenantID,
		DigitalEmployeeID: req.DigitalEmployeeID,
	})
	if err != nil {
		return nil, err
	}
	bindings := make([]MCPBinding, 0, len(rows))
	for _, row := range rows {
		employeeID := row.DigitalEmployeeID
		bindings = append(bindings, MCPBinding{
			ID:                row.ID,
			TenantID:          row.TenantID,
			DigitalEmployeeID: &employeeID,
			MCPServerID:       row.McpServerID,
			CredentialEnvVar:  row.CredentialEnvVar.String,
			Status:            row.Status,
			ServerName:        row.ServerName,
			ServerKey:         row.ServerKey,
			URL:               row.Url,
			Transport:         MCPTransport(row.Transport),
			AuthStrategy:      MCPAuthStrategy(row.AuthStrategy),
			RequiredEnvVars:   row.RequiredEnvVars,
			RiskLevel:         row.RiskLevel,
			SourceScope:       "employee",
			CreatedAt:         timeFromTimestamptz(row.CreatedAt),
			UpdatedAt:         timeFromTimestamptz(row.UpdatedAt),
		})
	}
	return bindings, nil
}

func (r *PgRepository) DeleteEmployeeMCPBindingV2(ctx context.Context, req DeleteEmployeeMCPBindingV2Request) error {
	if err := r.requireQueries(); err != nil {
		return err
	}
	return r.q.DeleteEmployeeMCPBindingV2(ctx, queries.DeleteEmployeeMCPBindingV2Params{
		TenantID:          req.TenantID,
		DigitalEmployeeID: req.DigitalEmployeeID,
		ID:                req.BindingID,
	})
}

// PutProjectMCPBindings is a declarative replace: soft-delete every active binding for the
// project, then insert the desired set. Delete-then-insert without a transaction
// (PgRepository only wraps *queries.Queries)：部分失败向"更少能力"收敛（fail-closed），
// 运行时投影只会少投不会多投，不存在静默扩权。
func (r *PgRepository) PutProjectMCPBindings(ctx context.Context, tenantID, projectID, createdBy uuid.UUID, items []ProjectMCPBindingInput) ([]MCPBinding, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	if err := r.q.SoftDeleteProjectMCPBindingsForProject(ctx, queries.SoftDeleteProjectMCPBindingsForProjectParams{
		TenantID:  tenantID,
		ProjectID: projectID,
	}); err != nil {
		return nil, err
	}
	for _, item := range items {
		if _, err := r.q.CreateProjectMCPBinding(ctx, queries.CreateProjectMCPBindingParams{
			TenantID:         tenantID,
			ProjectID:        projectID,
			McpServerID:      item.MCPServerID,
			CredentialEnvVar: textFromString(item.CredentialEnvVar),
			Metadata:         nil,
			CreatedBy:        nullUUIDFromValue(createdBy),
		}); err != nil {
			return nil, err
		}
	}
	return r.ListProjectMCPBindings(ctx, ProjectScopedRequest{TenantID: tenantID, ProjectID: projectID})
}

func (r *PgRepository) ListProjectMCPBindings(ctx context.Context, req ProjectScopedRequest) ([]MCPBinding, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	rows, err := r.q.ListProjectMCPBindings(ctx, queries.ListProjectMCPBindingsParams{
		TenantID:  req.TenantID,
		ProjectID: req.ProjectID,
	})
	if err != nil {
		return nil, err
	}
	bindings := make([]MCPBinding, 0, len(rows))
	for _, row := range rows {
		projectID := row.ProjectID
		bindings = append(bindings, MCPBinding{
			ID:               row.ID,
			TenantID:         row.TenantID,
			ProjectID:        &projectID,
			MCPServerID:      row.McpServerID,
			CredentialEnvVar: row.CredentialEnvVar.String,
			Status:           row.Status,
			ServerName:       row.ServerName,
			ServerKey:        row.ServerKey,
			URL:              row.Url,
			Transport:        MCPTransport(row.Transport),
			AuthStrategy:     MCPAuthStrategy(row.AuthStrategy),
			RequiredEnvVars:  row.RequiredEnvVars,
			RiskLevel:        row.RiskLevel,
			SourceScope:      "project",
			CreatedAt:        timeFromTimestamptz(row.CreatedAt),
			UpdatedAt:        timeFromTimestamptz(row.UpdatedAt),
		})
	}
	return bindings, nil
}

// ListEffectiveProjectMCPServers resolves the project-bound MCP servers ready for runtime
// projection. MissingEnvVars 用目标员工的已配置 env 集合判定——项目绑定同样只在员工
// env 满足时进入运行时 payload，与员工侧投影同一套过滤语义。
func (r *PgRepository) ListEffectiveProjectMCPServers(ctx context.Context, tenantID, projectID, digitalEmployeeID uuid.UUID) ([]EffectiveMCPServer, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	rows, err := r.q.ListEffectiveProjectMCPBindingsForRuntime(ctx, queries.ListEffectiveProjectMCPBindingsForRuntimeParams{
		TenantID:  tenantID,
		ProjectID: projectID,
	})
	if err != nil {
		return nil, err
	}
	configured, err := r.q.ListConfiguredEmployeeEnvVarNames(ctx, queries.ListConfiguredEmployeeEnvVarNamesParams{
		TenantID:          tenantID,
		DigitalEmployeeID: digitalEmployeeID,
	})
	if err != nil {
		return nil, err
	}
	configuredSet := make(map[string]struct{}, len(configured))
	for _, name := range configured {
		configuredSet[name] = struct{}{}
	}
	servers := make([]EffectiveMCPServer, 0, len(rows))
	for _, row := range rows {
		server := EffectiveMCPServer{
			ServerID:         row.ServerID,
			ServerKey:        row.ServerKey,
			Name:             row.Name,
			Transport:        MCPTransport(row.Transport),
			URL:              row.Url,
			AuthStrategy:     MCPAuthStrategy(row.AuthStrategy),
			CredentialEnvVar: row.CredentialEnvVar.String,
			RequiredEnvVars:  row.RequiredEnvVars,
			ToolAllowlist:    row.ToolAllowlist,
			RiskLevel:        row.RiskLevel,
			SourceScope:      row.SourceScope,
		}
		server.MissingEnvVars = missingFromSet(row.RequiredEnvVars, configuredSet)
		servers = append(servers, server)
	}
	return servers, nil
}

func (r *PgRepository) ListEffectiveMCPBindingsV2(ctx context.Context, req EmployeeScopedRequest) ([]EffectiveMCPServer, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	rows, err := r.q.ListEffectiveMCPBindingsV2ForEmployee(ctx, queries.ListEffectiveMCPBindingsV2ForEmployeeParams{
		TenantID:          req.TenantID,
		DigitalEmployeeID: req.DigitalEmployeeID,
	})
	if err != nil {
		return nil, err
	}
	configured, err := r.q.ListConfiguredEmployeeEnvVarNames(ctx, queries.ListConfiguredEmployeeEnvVarNamesParams{
		TenantID:          req.TenantID,
		DigitalEmployeeID: req.DigitalEmployeeID,
	})
	if err != nil {
		return nil, err
	}
	configuredSet := make(map[string]struct{}, len(configured))
	for _, name := range configured {
		configuredSet[name] = struct{}{}
	}
	servers := make([]EffectiveMCPServer, 0, len(rows))
	for _, row := range rows {
		server := EffectiveMCPServer{
			ServerID:         row.ServerID,
			ServerKey:        row.ServerKey,
			Name:             row.Name,
			Transport:        MCPTransport(row.Transport),
			URL:              row.Url,
			AuthStrategy:     MCPAuthStrategy(row.AuthStrategy),
			CredentialEnvVar: row.CredentialEnvVar.String,
			RequiredEnvVars:  row.RequiredEnvVars,
			ToolAllowlist:    row.ToolAllowlist,
			RiskLevel:        row.RiskLevel,
			SourceScope:      row.SourceScope,
		}
		server.MissingEnvVars = missingFromSet(row.RequiredEnvVars, configuredSet)
		servers = append(servers, server)
	}
	return servers, nil
}

func (r *PgRepository) ListConfiguredEmployeeEnvVarNames(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]string, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	return r.q.ListConfiguredEmployeeEnvVarNames(ctx, queries.ListConfiguredEmployeeEnvVarNamesParams{
		TenantID:          tenantID,
		DigitalEmployeeID: digitalEmployeeID,
	})
}

func mcpDefinitionFromQuery(row queries.McpServer) MCPDefinition {
	return MCPDefinition{
		ID:                 row.ID,
		TenantID:           row.TenantID,
		Name:               row.Name,
		ServerKey:          row.ServerKey,
		Description:        row.Description,
		Transport:          MCPTransport(row.Transport),
		URL:                row.Url,
		AuthStrategy:       MCPAuthStrategy(row.AuthStrategy),
		RequiredEnvVars:    row.RequiredEnvVars,
		OptionalEnvVars:    row.OptionalEnvVars,
		ProviderVisibility: unmarshalProviderVisibility(row.ProviderVisibility),
		ToolAllowlist:      row.ToolAllowlist,
		RiskLevel:          row.RiskLevel,
		Status:             row.Status,
		CreatedBy:          uuidPtrFromNull(row.CreatedBy),
		CreatedAt:          timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:          timeFromTimestamptz(row.UpdatedAt),
	}
}

func marshalProviderVisibility(visibility map[string]bool) ([]byte, error) {
	if len(visibility) == 0 {
		return nil, nil
	}
	return json.Marshal(visibility)
}

func unmarshalProviderVisibility(raw []byte) map[string]bool {
	if len(raw) == 0 {
		return nil
	}
	visibility := map[string]bool{}
	if err := json.Unmarshal(raw, &visibility); err != nil {
		return nil
	}
	return visibility
}

func textFromString(value string) pgtype.Text {
	trimmed := value
	if trimmed == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: trimmed, Valid: true}
}

func (r *PgRepository) SkillExistsForTenant(ctx context.Context, tenantID, skillID uuid.UUID) (bool, error) {
	if err := r.requireQueries(); err != nil {
		return false, err
	}
	return r.q.SkillExistsForTenant(ctx, queries.SkillExistsForTenantParams{TenantID: tenantID, ID: skillID})
}

func (r *PgRepository) ListSkillMCPDependencies(ctx context.Context, tenantID, skillID uuid.UUID) ([]SkillMCPDependency, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	rows, err := r.q.ListSkillMCPDependencies(ctx, queries.ListSkillMCPDependenciesParams{TenantID: tenantID, SkillID: skillID})
	if err != nil {
		return nil, err
	}
	deps := make([]SkillMCPDependency, 0, len(rows))
	for _, row := range rows {
		deps = append(deps, skillMCPDependencyFromRow(row.ID, row.TenantID, row.SkillID, row.McpServerID, row.Note, timeFromTimestamptz(row.CreatedAt), row.ServerKey, row.ServerName, row.AuthStrategy, row.RiskLevel, row.ServerStatus))
	}
	return deps, nil
}

// ReplaceSkillMCPDependencies is delete-then-insert without a transaction (PgRepository
// only wraps *queries.Queries). Partial failure leaves the skill with fewer declared
// dependencies, which fails closed: the dispatch gate blocks on missing bindings, never
// silently grants.
func (r *PgRepository) ReplaceSkillMCPDependencies(ctx context.Context, tenantID, skillID uuid.UUID, items []SkillMCPDependencyInput) ([]SkillMCPDependency, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	if err := r.q.DeleteSkillMCPDependenciesForSkill(ctx, queries.DeleteSkillMCPDependenciesForSkillParams{TenantID: tenantID, SkillID: skillID}); err != nil {
		return nil, err
	}
	for _, item := range items {
		if err := r.q.InsertSkillMCPDependency(ctx, queries.InsertSkillMCPDependencyParams{
			TenantID: tenantID, SkillID: skillID, McpServerID: item.MCPServerID, Note: item.Note,
		}); err != nil {
			return nil, err
		}
	}
	return r.ListSkillMCPDependencies(ctx, tenantID, skillID)
}

func (r *PgRepository) ListDependentSkills(ctx context.Context, tenantID, serverID uuid.UUID) ([]DependentSkill, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	rows, err := r.q.ListDependentSkillsForMCPServer(ctx, queries.ListDependentSkillsForMCPServerParams{TenantID: tenantID, McpServerID: serverID})
	if err != nil {
		return nil, err
	}
	out := make([]DependentSkill, 0, len(rows))
	for _, row := range rows {
		out = append(out, DependentSkill{SkillID: row.SkillID, Slug: row.Slug, Name: row.Name})
	}
	return out, nil
}

func (r *PgRepository) ListSkillMCPDependenciesForSkills(ctx context.Context, tenantID uuid.UUID, skillIDs []uuid.UUID) ([]SkillMCPDependency, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	if len(skillIDs) == 0 {
		return nil, nil
	}
	rows, err := r.q.ListSkillMCPDependenciesForSkills(ctx, queries.ListSkillMCPDependenciesForSkillsParams{TenantID: tenantID, SkillIds: skillIDs})
	if err != nil {
		return nil, err
	}
	deps := make([]SkillMCPDependency, 0, len(rows))
	for _, row := range rows {
		deps = append(deps, skillMCPDependencyFromRow(row.ID, row.TenantID, row.SkillID, row.McpServerID, row.Note, timeFromTimestamptz(row.CreatedAt), row.ServerKey, row.ServerName, row.AuthStrategy, row.RiskLevel, row.ServerStatus))
	}
	return deps, nil
}

func skillMCPDependencyFromRow(id, tenantID, skillID, serverID uuid.UUID, note string, createdAt time.Time, serverKey, serverName, authStrategy, riskLevel, serverStatus string) SkillMCPDependency {
	return SkillMCPDependency{
		ID: id, TenantID: tenantID, SkillID: skillID, MCPServerID: serverID,
		Note: note, CreatedAt: createdAt, ServerKey: serverKey, ServerName: serverName,
		AuthStrategy: MCPAuthStrategy(authStrategy), RiskLevel: riskLevel, ServerStatus: serverStatus,
	}
}

func (r *PgRepository) requireQueries() error {
	if r == nil || r.q == nil {
		return fmt.Errorf("%w: postgres queries are required", ErrInvalidInput)
	}
	return nil
}

func nullUUIDFromPtr(value *uuid.UUID) uuid.NullUUID {
	if value == nil || *value == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *value, Valid: true}
}

func nullUUIDFromValue(value uuid.UUID) uuid.NullUUID {
	if value == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: value, Valid: true}
}

func uuidPtrFromNull(value uuid.NullUUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	return &value.UUID
}

func timeFromTimestamptz(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func mapNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

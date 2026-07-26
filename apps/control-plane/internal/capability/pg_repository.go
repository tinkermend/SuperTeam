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
	if err := r.writeTeamCapabilityAudit(ctx, req.TenantID, req.TeamID, req.UserID, "team.mcp.bind", map[string]any{
		"team_id":            req.TeamID.String(),
		"mcp_server_id":      req.MCPServerID.String(),
		"binding_id":         row.ID.String(),
		"credential_env_var": req.CredentialEnvVar,
	}); err != nil {
		return MCPBinding{}, err
	}
	// 团队接管：同一 MCP 只留一份，成员各自的个人绑定就地物理收敛。
	// 不靠读路径静默屏蔽——那会留下"个人绑定列表看得见、生效列表看不见"的幽灵行。
	takenOver, err := r.TakeoverTeamMCPBindings(ctx, req.TenantID, req.TeamID, req.MCPServerID)
	if err != nil {
		return MCPBinding{}, err
	}
	if len(takenOver) > 0 {
		converged := make([]map[string]any, 0, len(takenOver))
		for _, item := range takenOver {
			converged = append(converged, map[string]any{
				"digital_employee_id":      item.DigitalEmployeeID.String(),
				"employee_name":            item.EmployeeName,
				"prior_credential_env_var": item.PriorCredentialEnvVar,
			})
		}
		if err := r.writeTeamCapabilityAudit(ctx, req.TenantID, req.TeamID, req.UserID, "team.mcp.takeover", map[string]any{
			"team_id":             req.TeamID.String(),
			"mcp_server_id":       req.MCPServerID.String(),
			"converged_count":     len(takenOver),
			"converged_employees": converged,
		}); err != nil {
			return MCPBinding{}, err
		}
	}
	teamID := row.TeamID
	return MCPBinding{
		ID:               row.ID,
		TenantID:         row.TenantID,
		TeamID:           &teamID,
		MCPServerID:      row.McpServerID,
		CredentialEnvVar: row.CredentialEnvVar.String,
		SourceScope:      "team",
		CreatedAt:        timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:        timeFromTimestamptz(row.UpdatedAt),
	}, nil
}

// ListTeamMCPTakeoverTargets 列出本团队成员里已自行绑定该 MCP 的个人绑定。
// 预览与执行共用这一条，保证"看到的"和"接管的"是同一批。
func (r *PgRepository) ListTeamMCPTakeoverTargets(ctx context.Context, tenantID, teamID, mcpServerID uuid.UUID) ([]MCPTakeover, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	rows, err := r.q.ListTeamMCPTakeoverTargets(ctx, queries.ListTeamMCPTakeoverTargetsParams{
		TenantID:    tenantID,
		TeamID:      teamID,
		McpServerID: mcpServerID,
	})
	if err != nil {
		return nil, err
	}
	targets := make([]MCPTakeover, 0, len(rows))
	for _, row := range rows {
		targets = append(targets, MCPTakeover{
			DigitalEmployeeID:     row.DigitalEmployeeID,
			EmployeeName:          row.EmployeeName,
			PriorCredentialEnvVar: row.CredentialEnvVar,
		})
	}
	return targets, nil
}

func (r *PgRepository) TakeoverTeamMCPBindings(ctx context.Context, tenantID, teamID, mcpServerID uuid.UUID) ([]MCPTakeover, error) {
	targets, err := r.ListTeamMCPTakeoverTargets(ctx, tenantID, teamID, mcpServerID)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, nil
	}
	if err := r.q.TakeoverTeamMCPBindings(ctx, queries.TakeoverTeamMCPBindingsParams{
		TenantID:    tenantID,
		TeamID:      teamID,
		McpServerID: mcpServerID,
	}); err != nil {
		return nil, err
	}
	return targets, nil
}

// TeamProvidesMCPServer 员工侧绑定前的冲突判据：所属团队是否已提供同一个 MCP。
func (r *PgRepository) TeamProvidesMCPServer(ctx context.Context, tenantID, employeeID, mcpServerID uuid.UUID) (bool, error) {
	if err := r.requireQueries(); err != nil {
		return false, err
	}
	return r.q.TeamProvidesMCPServer(ctx, queries.TeamProvidesMCPServerParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
		McpServerID:       mcpServerID,
	})
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
	// 先解析出 server_key 再删：审计详情里裸 binding_id 对人没有意义，
	// 而删除后就查不到它指向哪个 MCP 了。
	serverKey, serverID := "", ""
	existing, err := r.q.ListTeamMCPBindings(ctx, queries.ListTeamMCPBindingsParams{
		TenantID: req.TenantID,
		TeamID:   req.TeamID,
	})
	if err != nil {
		return err
	}
	for _, row := range existing {
		if row.ID == req.BindingID {
			serverKey = row.ServerKey
			serverID = row.McpServerID.String()
			break
		}
	}
	if err := r.q.DeleteTeamMCPBinding(ctx, queries.DeleteTeamMCPBindingParams{
		TenantID: req.TenantID,
		TeamID:   req.TeamID,
		ID:       req.BindingID,
	}); err != nil {
		return err
	}
	return r.writeTeamCapabilityAudit(ctx, req.TenantID, req.TeamID, req.UserID, "team.mcp.unbind", map[string]any{
		"team_id":       req.TeamID.String(),
		"binding_id":    req.BindingID.String(),
		"mcp_server_id": serverID,
		"server_key":    serverKey,
	})
}

// writeTeamCapabilityAudit 落一条 resource_type=team 的团队审计事件。资源维度必须是
// team，团队审计流(GET /teams/{id}/audit)按 resource_type='team' 过滤，落在别的
// 资源上等于不可见。
func (r *PgRepository) writeTeamCapabilityAudit(
	ctx context.Context,
	tenantID, teamID, actorUserID uuid.UUID,
	action string,
	details map[string]any,
) error {
	payload, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = r.q.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: tenantID, Valid: tenantID != uuid.Nil},
		EventType:    "team_management",
		ActorType:    "user",
		ActorID:      actorUserID.String(),
		ResourceType: pgtype.Text{String: "team", Valid: true},
		ResourceID:   pgtype.Text{String: teamID.String(), Valid: true},
		Action:       action,
		Details:      payload,
	})
	return err
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
		deps = append(deps, skillMCPDependencyFromRow(row.ID, row.TenantID, row.SkillID, row.McpServerID, row.Note, timeFromTimestamptz(row.CreatedAt), row.ServerKey, row.ServerName, row.AuthStrategy, row.RiskLevel))
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
		deps = append(deps, skillMCPDependencyFromRow(row.ID, row.TenantID, row.SkillID, row.McpServerID, row.Note, timeFromTimestamptz(row.CreatedAt), row.ServerKey, row.ServerName, row.AuthStrategy, row.RiskLevel))
	}
	return deps, nil
}

func skillMCPDependencyFromRow(id, tenantID, skillID, serverID uuid.UUID, note string, createdAt time.Time, serverKey, serverName, authStrategy, riskLevel string) SkillMCPDependency {
	return SkillMCPDependency{
		ID: id, TenantID: tenantID, SkillID: skillID, MCPServerID: serverID,
		Note: note, CreatedAt: createdAt, ServerKey: serverKey, ServerName: serverName,
		AuthStrategy: MCPAuthStrategy(authStrategy), RiskLevel: riskLevel,
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

func (r *PgRepository) ListTeamMCPReadiness(ctx context.Context, tenantID, teamID uuid.UUID) ([]TeamMCPReadinessEntry, error) {
	if err := r.requireQueries(); err != nil {
		return nil, err
	}
	rows, err := r.q.ListTeamMCPReadiness(ctx, queries.ListTeamMCPReadinessParams{
		TenantID: tenantID,
		TeamID:   teamID,
	})
	if err != nil {
		return nil, err
	}
	entries := make([]TeamMCPReadinessEntry, 0, len(rows))
	for _, row := range rows {
		entries = append(entries, TeamMCPReadinessEntry{
			MCPServerID:       row.McpServerID,
			ServerKey:         row.ServerKey,
			ServerName:        row.ServerName,
			RequiredEnvVars:   row.RequiredEnvVars,
			DigitalEmployeeID: row.DigitalEmployeeID,
			EmployeeName:      row.EmployeeName,
			MissingEnvVars:    row.MissingEnvVars,
		})
	}
	return entries, nil
}

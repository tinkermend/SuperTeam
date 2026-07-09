package employee

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgRepository struct {
	q   *queries.Queries
	db  employeeTransactionBeginner
	sql queries.DBTX
}

type employeeTransactionBeginner interface {
	queries.DBTX
	Begin(context.Context) (pgx.Tx, error)
}

func NewPgRepository(q *queries.Queries, db ...employeeTransactionBeginner) Repository {
	var beginner employeeTransactionBeginner
	if len(db) > 0 {
		beginner = db[0]
	}
	var sql queries.DBTX
	if beginner != nil {
		sql = beginner
	}
	return &PgRepository{q: q, db: beginner, sql: sql}
}

func (r *PgRepository) WithTransaction(ctx context.Context, fn func(Repository) error) error {
	if r.db == nil {
		return fn(r)
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin employee transaction: %w", err)
	}
	// Roll back unconditionally on any early return or panic; it is a no-op
	// once Commit succeeds, so a leaked open tx can never strand a pooled conn.
	defer func() { _ = tx.Rollback(ctx) }()
	txRepo := &PgRepository{q: r.q.WithTx(tx), sql: tx}
	if err := fn(txRepo); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit employee transaction: %w", err)
	}
	return nil
}

func (r *PgRepository) CreateDigitalEmployee(ctx context.Context, params CreateDigitalEmployeeParams) (DigitalEmployeeRecord, error) {
	permissionPolicy, err := jsonbFromMap(params.PermissionPolicy, "permission_policy")
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}
	contextPolicy, err := jsonbFromMap(params.ContextPolicy, "context_policy")
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}
	approvalPolicy, err := jsonbFromMap(params.ApprovalPolicy, "approval_policy")
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}
	metadata, err := jsonbFromMap(params.Metadata, "metadata")
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}

	employee, err := r.q.CreateDigitalEmployee(ctx, queries.CreateDigitalEmployeeParams{
		TenantID:         params.TenantID,
		TeamID:           nullUUIDFromPtr(params.TeamID),
		OwnerUserID:      params.OwnerUserID,
		EmployeeType:     params.EmployeeType,
		ProviderType:     params.ProviderType,
		Name:             params.Name,
		Role:             params.Role,
		Description:      textFromPtr(params.Description),
		Status:           string(params.Status),
		PermissionPolicy: permissionPolicy,
		ContextPolicy:    contextPolicy,
		ApprovalPolicy:   approvalPolicy,
		RiskLevel:        params.RiskLevel,
		Metadata:         metadata,
	})
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}
	return digitalEmployeeRecordFromQuery(employee)
}

func (r *PgRepository) ListDigitalEmployees(ctx context.Context, params ListDigitalEmployeesParams) ([]DigitalEmployeeRecord, error) {
	employees, err := r.q.ListDigitalEmployees(ctx, queries.ListDigitalEmployeesParams{
		TenantID:   params.TenantID,
		TeamID:     nullUUIDFromPtr(params.TeamID),
		Status:     textFromStatus(params.Status),
		Assignment: textFromOptionalString(params.Assignment),
		Offset:     params.Offset,
		Limit:      params.Limit,
	})
	if err != nil {
		return nil, err
	}
	records := make([]DigitalEmployeeRecord, 0, len(employees))
	for _, employee := range employees {
		record, err := digitalEmployeeRecordFromQuery(employee)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (r *PgRepository) GetDigitalEmployee(ctx context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeRecord, error) {
	employee, err := r.q.GetDigitalEmployee(ctx, queries.GetDigitalEmployeeParams{
		ID:       employeeID,
		TenantID: tenantID,
	})
	if err != nil {
		return DigitalEmployeeRecord{}, mapNoRows(err)
	}
	return digitalEmployeeRecordFromQuery(employee)
}

func (r *PgRepository) GetDigitalEmployeeForDelete(ctx context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeRecord, error) {
	employee, err := r.q.GetDigitalEmployeeForDelete(ctx, queries.GetDigitalEmployeeForDeleteParams{
		ID:       employeeID,
		TenantID: tenantID,
	})
	if err != nil {
		return DigitalEmployeeRecord{}, mapNoRows(err)
	}
	return digitalEmployeeRecordFromQuery(employee)
}

func (r *PgRepository) ListDigitalEmployeeDeleteBlockers(ctx context.Context, tenantID, employeeID uuid.UUID) ([]DigitalEmployeeDeleteBlocker, error) {
	runRows, err := r.q.ListDigitalEmployeeDeleteRunBlockers(ctx, queries.ListDigitalEmployeeDeleteRunBlockersParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		return nil, err
	}
	projectRows, err := r.q.ListDigitalEmployeeDeleteProjectTaskBlockers(ctx, queries.ListDigitalEmployeeDeleteProjectTaskBlockersParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		return nil, err
	}
	blockers := make([]DigitalEmployeeDeleteBlocker, 0, len(runRows)+len(projectRows))
	for _, row := range runRows {
		blockers = append(blockers, DigitalEmployeeDeleteBlocker{
			Type:      DigitalEmployeeDeleteBlockerTypeRun,
			ID:        row.ID,
			Status:    row.Status,
			Title:     row.Title,
			RunID:     uuidPtr(row.RunID),
			ProjectID: uuidPtrFromNull(row.ProjectID),
		})
	}
	for _, row := range projectRows {
		blockers = append(blockers, DigitalEmployeeDeleteBlocker{
			Type:      DigitalEmployeeDeleteBlockerTypeProjectTask,
			ID:        row.ID,
			Status:    row.Status,
			Title:     row.Title,
			RunID:     uuidPtrFromNull(row.RunID),
			ProjectID: uuidPtr(row.ProjectID),
		})
	}
	return blockers, nil
}

func (r *PgRepository) SoftDeleteDigitalEmployeeCascade(ctx context.Context, params SoftDeleteDigitalEmployeeCascadeParams) (DigitalEmployeeDeleteCascadeResult, error) {
	deletedAt := pgtype.Timestamptz{Time: params.DeletedAt.UTC(), Valid: true}
	cascade := DigitalEmployeeDeleteCascadeResult{}

	instances, err := r.q.SoftDeleteDigitalEmployeeExecutionInstancesForDelete(ctx, queries.SoftDeleteDigitalEmployeeExecutionInstancesForDeleteParams{
		TenantID:          params.TenantID,
		DigitalEmployeeID: params.DigitalEmployeeID,
		DeletedAt:         deletedAt,
	})
	if err != nil {
		return cascade, err
	}
	cascade.ExecutionInstances = int64(len(instances))
	if len(instances) > 0 {
		first := instances[0]
		cascade.ExecutionInstanceID = uuidPtr(first.ID)
		cascade.RuntimeNodeID = uuidPtr(first.RuntimeNodeID)
		cascade.ProviderType = first.ProviderType
		cascade.AgentHomeDir = first.AgentHomeDir
	}

	envRows, err := r.q.SoftDeleteDigitalEmployeeEnvironmentVariablesForDelete(ctx, queries.SoftDeleteDigitalEmployeeEnvironmentVariablesForDeleteParams{TenantID: params.TenantID, DigitalEmployeeID: params.DigitalEmployeeID, DeletedAt: deletedAt})
	if err != nil {
		return cascade, err
	}
	cascade.EnvironmentVariables = int64(len(envRows))

	mcpRows, err := r.q.SoftDeleteDigitalEmployeeMCPBindingsForDelete(ctx, queries.SoftDeleteDigitalEmployeeMCPBindingsForDeleteParams{TenantID: params.TenantID, DigitalEmployeeID: params.DigitalEmployeeID, DeletedAt: deletedAt})
	if err != nil {
		return cascade, err
	}
	cascade.MCPBindings = int64(len(mcpRows))
	cascade.MCPBindingIDs = appendUUIDs(cascade.MCPBindingIDs, mcpRows)

	mcpV2Rows, err := r.q.SoftDeleteDigitalEmployeeMCPBindingsV2ForDelete(ctx, queries.SoftDeleteDigitalEmployeeMCPBindingsV2ForDeleteParams{TenantID: params.TenantID, DigitalEmployeeID: params.DigitalEmployeeID, DeletedAt: deletedAt})
	if err != nil {
		return cascade, err
	}
	cascade.MCPBindingsV2 = int64(len(mcpV2Rows))
	cascade.MCPBindingV2IDs = appendUUIDs(cascade.MCPBindingV2IDs, mcpV2Rows)

	skillRows, err := r.q.DisableSkillAgentBindingsForDelete(ctx, queries.DisableSkillAgentBindingsForDeleteParams{TenantID: params.TenantID, DigitalEmployeeID: params.DigitalEmployeeID, DeletedAt: deletedAt})
	if err != nil {
		return cascade, err
	}
	cascade.SkillBindings = int64(len(skillRows))
	cascade.SkillBindingIDs = appendUUIDs(cascade.SkillBindingIDs, skillRows)

	configRows, err := r.q.ArchiveDigitalEmployeeConfigRevisionsForDelete(ctx, queries.ArchiveDigitalEmployeeConfigRevisionsForDeleteParams{TenantID: params.TenantID, DigitalEmployeeID: params.DigitalEmployeeID, DeletedAt: deletedAt})
	if err != nil {
		return cascade, err
	}
	cascade.ConfigRevisions = int64(len(configRows))

	workspaceRows, err := r.q.SoftDeleteDigitalEmployeeWorkspaceFilesForDelete(ctx, queries.SoftDeleteDigitalEmployeeWorkspaceFilesForDeleteParams{TenantID: params.TenantID, DigitalEmployeeID: params.DigitalEmployeeID, DeletedAt: deletedAt})
	if err != nil {
		return cascade, err
	}
	cascade.WorkspaceFiles = int64(len(workspaceRows))
	cascade.WorkspaceFileIDs = appendUUIDs(cascade.WorkspaceFileIDs, workspaceRows)

	affinityRows, err := r.q.DeleteProjectEmployeeNodeAffinitiesForEmployeeDelete(ctx, queries.DeleteProjectEmployeeNodeAffinitiesForEmployeeDeleteParams{TenantID: params.TenantID, DigitalEmployeeID: params.DigitalEmployeeID})
	if err != nil {
		return cascade, err
	}
	cascade.ProjectAffinities = int64(len(affinityRows))

	_, err = r.q.SoftDeleteDigitalEmployeeForDelete(ctx, queries.SoftDeleteDigitalEmployeeForDeleteParams{
		ID:        params.DigitalEmployeeID,
		TenantID:  params.TenantID,
		DeletedAt: deletedAt,
	})
	if err != nil {
		return cascade, mapNoRows(err)
	}
	return cascade, nil
}

func (r *PgRepository) CreateDigitalEmployeeDeleteAuditEvent(ctx context.Context, params DigitalEmployeeDeleteAuditEventParams) error {
	details, err := json.Marshal(digitalEmployeeDeleteAuditDetails(params))
	if err != nil {
		return err
	}
	_, err = r.q.CreateAuditEvent(ctx, queries.CreateAuditEventParams{
		TenantID:     uuid.NullUUID{UUID: params.TenantID, Valid: params.TenantID != uuid.Nil},
		EventType:    "digital_employee_management",
		ActorType:    "user",
		ActorID:      params.ActorUserID.String(),
		ResourceType: pgtype.Text{String: "digital_employee", Valid: true},
		ResourceID:   pgtype.Text{String: params.Employee.ID.String(), Valid: true},
		Action:       "digital_employee.delete",
		Details:      details,
	})
	return err
}

func (r *PgRepository) EnsureTeamExists(ctx context.Context, tenantID, teamID uuid.UUID) error {
	if _, err := r.q.GetTenantTeam(ctx, queries.GetTenantTeamParams{
		ID:       teamID,
		TenantID: tenantID,
	}); err != nil {
		return mapNoRows(err)
	}
	return nil
}

func (r *PgRepository) GetTeamBaseline(ctx context.Context, tenantID, teamID uuid.UUID) (TeamBaseline, error) {
	if r.sql == nil {
		return TeamBaseline{}, fmt.Errorf("%w: postgres is not configured", ErrInvalidInput)
	}
	team, err := r.q.GetTenantTeam(ctx, queries.GetTenantTeamParams{
		ID:       teamID,
		TenantID: tenantID,
	})
	if err != nil {
		return TeamBaseline{}, mapNoRows(err)
	}

	constitution, err := mapFromJSONB(team.Constitution, "constitution")
	if err != nil {
		return TeamBaseline{}, err
	}

	skillRows, err := r.sql.Query(ctx, `
SELECT s.slug
FROM team_skill_bindings stb
JOIN skills s
  ON s.tenant_id = stb.tenant_id
 AND s.id = stb.skill_id
 AND s.deleted_at IS NULL
WHERE stb.tenant_id = $1
  AND stb.team_id = $2
ORDER BY s.slug ASC
`, tenantID, teamID)
	if err != nil {
		return TeamBaseline{}, err
	}
	defer skillRows.Close()

	skills := make([]string, 0)
	for skillRows.Next() {
		var slug string
		if err := skillRows.Scan(&slug); err != nil {
			return TeamBaseline{}, err
		}
		skills = append(skills, slug)
	}
	if err := skillRows.Err(); err != nil {
		return TeamBaseline{}, err
	}

	mcpRows, err := r.q.ListTeamMCPBindings(ctx, queries.ListTeamMCPBindingsParams{
		TenantID: tenantID,
		TeamID:   teamID,
	})
	if err != nil {
		return TeamBaseline{}, err
	}
	mcpServers := make([]string, 0, len(mcpRows))
	for _, row := range mcpRows {
		if strings.EqualFold(strings.TrimSpace(row.Status), "active") &&
			strings.EqualFold(strings.TrimSpace(row.ServerStatus), "active") {
			mcpServers = append(mcpServers, row.ServerKey)
		}
	}

	return TeamBaseline{
		Constitution: constitution,
		Skills:       skills,
		MCPServers:   mcpServers,
	}, nil
}

func (r *PgRepository) ListRuntimeProviderOptionsForCreate(ctx context.Context, tenantID, teamID uuid.UUID) ([]RuntimeProviderOption, error) {
	rows, err := r.q.ListRuntimeProviderOptionsForDigitalEmployeeCreate(ctx, queries.ListRuntimeProviderOptionsForDigitalEmployeeCreateParams{
		TenantID: tenantID,
		TeamID:   teamID,
	})
	if err != nil {
		return nil, err
	}
	options := make([]RuntimeProviderOption, 0, len(rows))
	for _, row := range rows {
		options = append(options, RuntimeProviderOption{
			RuntimeNodeID:         row.RuntimeNodeID,
			NodeID:                row.NodeID,
			RuntimeName:           row.RuntimeName,
			ProviderType:          stringFromText(row.ProviderType),
			RuntimeStatus:         row.RuntimeStatus,
			ProviderStatus:        stringFromText(row.ProviderStatus),
			HealthStatus:          stringFromText(row.HealthStatus),
			CurrentLoad:           row.CurrentLoad,
			MaxSlots:              row.MaxSlots,
			AgentHomeDir:          row.AgentHomeDir,
			AgentHomeDirAvailable: strings.TrimSpace(row.AgentHomeDir) != "",
			Available:             row.Available,
			DisabledReason:        row.DisabledReason,
		})
	}
	return options, nil
}

func (r *PgRepository) GetRuntimeProvisioningPreflight(ctx context.Context, tenantID, teamID, runtimeNodeID uuid.UUID, providerType string) (RuntimeProvisioningPreflight, error) {
	preflight, err := r.q.GetRuntimeProvisioningPreflight(ctx, queries.GetRuntimeProvisioningPreflightParams{
		TenantID:      tenantID,
		TeamID:        teamID,
		RuntimeNodeID: runtimeNodeID,
		ProviderType:  providerType,
	})
	if err != nil {
		return RuntimeProvisioningPreflight{}, mapNoRows(err)
	}
	governanceSnapshot, err := mapFromJSONValue(preflight.GovernanceSnapshot, "governance_snapshot")
	if err != nil {
		return RuntimeProvisioningPreflight{}, err
	}
	return RuntimeProvisioningPreflight{
		TenantID:              preflight.TenantID,
		TeamID:                preflight.TeamID,
		RuntimeNodeID:         preflight.RuntimeNodeID,
		NodeID:                preflight.NodeID,
		AgentHomeDir:          preflight.AgentHomeDir,
		GovernanceSnapshot:    governanceSnapshot,
		HasActiveTeamConfig:   preflight.HasActiveTeamConfig,
		RuntimeOnline:         preflight.RuntimeOnline,
		EnrollmentApproved:    preflight.EnrollmentApproved,
		RuntimeSessionActive:  preflight.RuntimeSessionActive,
		ProviderAvailable:     preflight.ProviderAvailable,
		ProviderPolicyAllowed: preflight.ProviderPolicyAllowed,
		RuntimePolicyAllowed:  preflight.RuntimePolicyAllowed,
	}, nil
}

func (r *PgRepository) ListRuntimeProviderOptionsForTeamLessCreate(ctx context.Context, tenantID uuid.UUID) ([]RuntimeProviderOption, error) {
	rows, err := r.q.ListRuntimeProviderOptionsForTeamLessCreate(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	options := make([]RuntimeProviderOption, 0, len(rows))
	for _, row := range rows {
		options = append(options, RuntimeProviderOption{
			RuntimeNodeID:         row.RuntimeNodeID,
			NodeID:                row.NodeID,
			RuntimeName:           row.RuntimeName,
			ProviderType:          stringFromText(row.ProviderType),
			RuntimeStatus:         row.RuntimeStatus,
			ProviderStatus:        stringFromText(row.ProviderStatus),
			HealthStatus:          stringFromText(row.HealthStatus),
			CurrentLoad:           row.CurrentLoad,
			MaxSlots:              row.MaxSlots,
			AgentHomeDir:          row.AgentHomeDir,
			AgentHomeDirAvailable: strings.TrimSpace(row.AgentHomeDir) != "",
			Available:             row.Available,
			DisabledReason:        row.DisabledReason,
		})
	}
	return options, nil
}

func (r *PgRepository) GetRuntimeProvisioningPreflightTeamLess(ctx context.Context, tenantID, runtimeNodeID uuid.UUID, providerType string) (RuntimeProvisioningPreflight, error) {
	preflight, err := r.q.GetRuntimeProvisioningPreflightTeamLess(ctx, queries.GetRuntimeProvisioningPreflightTeamLessParams{
		RuntimeNodeID: runtimeNodeID,
		TenantID:      tenantID,
		ProviderType:  providerType,
	})
	if err != nil {
		return RuntimeProvisioningPreflight{}, mapNoRows(err)
	}
	governanceSnapshot, err := mapFromJSONValue(preflight.GovernanceSnapshot, "governance_snapshot")
	if err != nil {
		return RuntimeProvisioningPreflight{}, err
	}
	return RuntimeProvisioningPreflight{
		TenantID:              preflight.TenantID,
		TeamID:                preflight.TeamID.UUID,
		RuntimeNodeID:         preflight.RuntimeNodeID,
		NodeID:                preflight.NodeID,
		AgentHomeDir:          preflight.AgentHomeDir,
		GovernanceSnapshot:    governanceSnapshot,
		HasActiveTeamConfig:   preflight.HasActiveTeamConfig,
		RuntimeOnline:         preflight.RuntimeOnline,
		EnrollmentApproved:    preflight.EnrollmentApproved,
		RuntimeSessionActive:  preflight.RuntimeSessionActive,
		ProviderAvailable:     preflight.ProviderAvailable,
		ProviderPolicyAllowed: preflight.ProviderPolicyAllowed,
		RuntimePolicyAllowed:  preflight.RuntimePolicyAllowed,
	}, nil
}

func (r *PgRepository) UpdateDigitalEmployeeStatus(ctx context.Context, tenantID, employeeID uuid.UUID, status DigitalEmployeeStatus) (DigitalEmployeeRecord, error) {
	employee, err := r.q.UpdateDigitalEmployeeStatus(ctx, queries.UpdateDigitalEmployeeStatusParams{
		Status:   string(status),
		ID:       employeeID,
		TenantID: tenantID,
	})
	if err != nil {
		return DigitalEmployeeRecord{}, mapNoRows(err)
	}
	return digitalEmployeeRecordFromQuery(employee)
}

func (r *PgRepository) UpsertDigitalEmployeeExecutionInstance(ctx context.Context, params UpsertExecutionInstanceParams) (DigitalEmployeeExecutionInstanceRecord, error) {
	workspacePolicy, err := jsonbFromMap(params.WorkspacePolicy, "workspace_policy")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	sessionPolicy, err := jsonbFromMap(params.SessionPolicy, "session_policy")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	runtimeSelector, err := jsonbFromMap(params.RuntimeSelector, "runtime_selector")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	capacityRequirements, err := jsonbFromMap(params.CapacityRequirements, "capacity_requirements")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	fallbackPolicy, err := jsonbFromMap(params.FallbackPolicy, "fallback_policy")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	metadata, err := jsonbFromMap(params.Metadata, "metadata")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}

	instance, err := r.q.UpsertDigitalEmployeeExecutionInstance(ctx, queries.UpsertDigitalEmployeeExecutionInstanceParams{
		ProviderType:         params.ProviderType,
		AgentHomeDir:         params.AgentHomeDir,
		WorkspacePolicy:      workspacePolicy,
		SessionPolicy:        sessionPolicy,
		RuntimeSelector:      runtimeSelector,
		CapacityRequirements: capacityRequirements,
		FallbackPolicy:       fallbackPolicy,
		Status:               string(params.Status),
		Metadata:             metadata,
		RuntimeNodeID:        params.RuntimeNodeID,
		DigitalEmployeeID:    params.DigitalEmployeeID,
		TenantID:             params.TenantID,
	})
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, mapNoRows(err)
	}
	return executionInstanceRecordFromQuery(instance)
}

func (r *PgRepository) CreateRuntimeCommandReceipt(ctx context.Context, req CreateRuntimeCommandReceiptRequest) error {
	payload, err := jsonbFromMap(redactRuntimeEventPayloadForPersistence(req.Payload), "payload")
	if err != nil {
		return err
	}
	_, err = r.q.CreateRuntimeCommandReceipt(ctx, queries.CreateRuntimeCommandReceiptParams{
		TenantID:      req.TenantID,
		CommandID:     req.CommandID,
		CommandType:   req.CommandType,
		RuntimeNodeID: req.RuntimeNodeID,
		NodeID:        req.NodeID,
		ResourceType:  req.ResourceType,
		ResourceID:    req.ResourceID,
		Status:        req.Status,
		Payload:       payload,
		DispatchedAt:  timestamptzFromPtr(req.DispatchedAt),
	})
	return err
}

func (r *PgRepository) WaitForRuntimeCommandCompletion(ctx context.Context, tenantID uuid.UUID, commandID string, interval time.Duration) (*RuntimeCommandReceipt, error) {
	if interval <= 0 {
		interval = defaultProvisioningPollInterval
	}
	for {
		receipt, err := r.q.GetRuntimeCommandReceiptByCommandID(ctx, queries.GetRuntimeCommandReceiptByCommandIDParams{
			TenantID:  tenantID,
			CommandID: commandID,
		})
		if err != nil {
			return nil, mapNoRows(err)
		}
		mapped := runtimeCommandReceiptFromQuery(receipt)
		if isTerminalReceiptStatus(mapped.Status) {
			return mapped, nil
		}

		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (r *PgRepository) AbortProvisionedDigitalEmployee(ctx context.Context, tenantID, employeeID, executionInstanceID uuid.UUID, reason string) error {
	return r.q.AbortProvisionedDigitalEmployee(ctx, queries.AbortProvisionedDigitalEmployeeParams{
		TenantID:            tenantID,
		DigitalEmployeeID:   employeeID,
		ExecutionInstanceID: executionInstanceID,
		Reason:              reason,
	})
}

func (r *PgRepository) GetDigitalEmployeeExecutionInstanceByEmployeeID(ctx context.Context, tenantID, employeeID uuid.UUID) (DigitalEmployeeExecutionInstanceRecord, error) {
	instance, err := r.q.GetDigitalEmployeeExecutionInstanceByEmployeeID(ctx, queries.GetDigitalEmployeeExecutionInstanceByEmployeeIDParams{
		DigitalEmployeeID: employeeID,
		TenantID:          tenantID,
	})
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, mapNoRows(err)
	}
	return executionInstanceRecordFromQuery(instance)
}

func (r *PgRepository) CreateWorkspaceFile(ctx context.Context, params CreateWorkspaceFileParams) (WorkspaceFileRecord, error) {
	metadata, err := jsonbFromMap(params.Metadata, "metadata")
	if err != nil {
		return WorkspaceFileRecord{}, err
	}
	row, err := r.q.CreateDigitalEmployeeWorkspaceFile(ctx, queries.CreateDigitalEmployeeWorkspaceFileParams{
		TenantID:          params.TenantID,
		TeamID:            nullUUIDFromPtr(params.TeamID),
		DigitalEmployeeID: params.DigitalEmployeeID,
		Path:              params.Path,
		FileRole:          params.FileRole,
		MimeType:          params.MimeType,
		SyncPolicy:        params.SyncPolicy,
		Status:            params.Status,
		Metadata:          metadata,
		CreatedBy:         nullUUIDFromPtr(params.CreatedBy),
	})
	if err != nil {
		return WorkspaceFileRecord{}, err
	}
	return workspaceFileRecordFromQuery(row)
}

func (r *PgRepository) CreateWorkspaceFileRevision(ctx context.Context, params CreateWorkspaceFileRevisionParams) (WorkspaceFileRevisionRecord, error) {
	metadata, err := jsonbFromMap(params.Metadata, "metadata")
	if err != nil {
		return WorkspaceFileRevisionRecord{}, err
	}
	row, err := r.q.CreateDigitalEmployeeWorkspaceFileRevision(ctx, queries.CreateDigitalEmployeeWorkspaceFileRevisionParams{
		TenantID:       params.TenantID,
		FileID:         params.FileID,
		RevisionNumber: params.RevisionNumber,
		ContentText:    textFromPtr(&params.ContentText),
		ContentHash:    params.ContentHash,
		SizeBytes:      params.SizeBytes,
		StorageBackend: params.StorageBackend,
		ObjectKey:      textFromPtr(params.ObjectKey),
		CreatedBy:      nullUUIDFromPtr(params.CreatedBy),
		ChangeNote:     textFromPtr(params.ChangeNote),
		Metadata:       metadata,
	})
	if err != nil {
		return WorkspaceFileRevisionRecord{}, err
	}
	return workspaceFileRevisionRecordFromQuery(row)
}

func (r *PgRepository) ActivateWorkspaceFileRevision(ctx context.Context, tenantID, fileID, revisionID uuid.UUID) (WorkspaceFileRecord, error) {
	row, err := r.q.ActivateDigitalEmployeeWorkspaceFileRevision(ctx, queries.ActivateDigitalEmployeeWorkspaceFileRevisionParams{
		TenantID:   tenantID,
		FileID:     fileID,
		RevisionID: revisionID,
	})
	if err != nil {
		return WorkspaceFileRecord{}, mapNoRows(err)
	}
	return workspaceFileRecordFromQuery(row)
}

func (r *PgRepository) GetWorkspaceFileByPath(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID, filePath string) (WorkspaceFileRecord, error) {
	row, err := r.q.GetDigitalEmployeeWorkspaceFileByPath(ctx, queries.GetDigitalEmployeeWorkspaceFileByPathParams{
		TenantID:          tenantID,
		DigitalEmployeeID: digitalEmployeeID,
		Path:              filePath,
	})
	if err != nil {
		return WorkspaceFileRecord{}, mapNoRows(err)
	}
	return workspaceFileRecordFromQuery(row)
}

func (r *PgRepository) GetNextWorkspaceFileRevisionNumber(ctx context.Context, tenantID, fileID uuid.UUID) (int32, error) {
	return r.q.GetNextDigitalEmployeeWorkspaceFileRevisionNumber(ctx, queries.GetNextDigitalEmployeeWorkspaceFileRevisionNumberParams{
		TenantID: tenantID,
		FileID:   fileID,
	})
}

func (r *PgRepository) ListWorkspaceFiles(ctx context.Context, req ListWorkspaceFilesRequest) ([]WorkspaceFile, error) {
	rows, err := r.q.ListCurrentDigitalEmployeeWorkspaceFiles(ctx, queries.ListCurrentDigitalEmployeeWorkspaceFilesParams{
		TenantID:          req.TenantID,
		DigitalEmployeeID: req.DigitalEmployeeID,
	})
	if err != nil {
		return nil, err
	}
	records := make([]WorkspaceFile, 0, len(rows))
	for _, row := range rows {
		record, err := workspaceFileFromCurrentRow(row)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (r *PgRepository) ListWorkspaceFilesForSync(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]WorkspaceFileForSyncRecord, error) {
	rows, err := r.q.ListCurrentDigitalEmployeeWorkspaceFilesForSync(ctx, queries.ListCurrentDigitalEmployeeWorkspaceFilesForSyncParams{
		TenantID:          tenantID,
		DigitalEmployeeID: digitalEmployeeID,
	})
	if err != nil {
		return nil, err
	}
	records := make([]WorkspaceFileForSyncRecord, 0, len(rows))
	for _, row := range rows {
		record, err := workspaceFileForSyncRecordFromQuery(row)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (r *PgRepository) ListEnvironmentVariables(ctx context.Context, req ListEnvironmentVariablesRequest) ([]EnvironmentVariableRecord, error) {
	store, err := r.envSQLStore()
	if err != nil {
		return nil, err
	}
	rows, err := store.Query(ctx, `
SELECT id, tenant_id, team_id, digital_employee_id, name, encrypted_value,
       encryption_key_id, value_fingerprint, sensitive, status,
       created_by, updated_by, created_at, updated_at
FROM digital_employee_environment_variables
WHERE tenant_id = $1
  AND digital_employee_id = $2
  AND deleted_at IS NULL
ORDER BY name ASC
`, req.TenantID, req.DigitalEmployeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]EnvironmentVariableRecord, 0)
	for rows.Next() {
		record, err := scanEnvironmentVariableRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (r *PgRepository) UpsertEnvironmentVariable(ctx context.Context, req UpsertEnvironmentVariableStoreRequest) (EnvironmentVariableRecord, error) {
	store, err := r.envSQLStore()
	if err != nil {
		return EnvironmentVariableRecord{}, err
	}
	row := store.QueryRow(ctx, `
INSERT INTO digital_employee_environment_variables (
    tenant_id, team_id, digital_employee_id, name,
    encrypted_value, encryption_key_id, value_fingerprint, sensitive,
    status, created_by, updated_by, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'active',$9,$9,NOW())
ON CONFLICT (tenant_id, digital_employee_id, name) WHERE deleted_at IS NULL
DO UPDATE SET
    encrypted_value = EXCLUDED.encrypted_value,
    encryption_key_id = EXCLUDED.encryption_key_id,
    value_fingerprint = EXCLUDED.value_fingerprint,
    sensitive = EXCLUDED.sensitive,
    status = 'active',
    updated_by = EXCLUDED.updated_by,
    updated_at = NOW()
RETURNING id, tenant_id, team_id, digital_employee_id, name, encrypted_value,
          encryption_key_id, value_fingerprint, sensitive, status,
          created_by, updated_by, created_at, updated_at
`, req.TenantID, nullUUIDFromPtr(req.TeamID), req.DigitalEmployeeID, req.Name, req.EncryptedValue, req.EncryptionKeyID, req.ValueFingerprint, req.Sensitive, nullUUIDFromPtr(req.UpdatedBy))
	record, err := scanEnvironmentVariableRecord(row)
	if err != nil {
		return EnvironmentVariableRecord{}, err
	}
	return record, nil
}

func (r *PgRepository) DeleteEnvironmentVariable(ctx context.Context, req DeleteEnvironmentVariableRequest) error {
	store, err := r.envSQLStore()
	if err != nil {
		return err
	}
	_, err = store.Exec(ctx, `
UPDATE digital_employee_environment_variables
SET deleted_at = NOW(), updated_at = NOW()
WHERE tenant_id = $1
  AND digital_employee_id = $2
  AND name = $3
  AND deleted_at IS NULL
`, req.TenantID, req.DigitalEmployeeID, req.Name)
	return err
}

func (r *PgRepository) ListRuntimeEnvironmentVariables(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) ([]EnvironmentVariableRecord, error) {
	store, err := r.envSQLStore()
	if err != nil {
		return nil, err
	}
	rows, err := store.Query(ctx, `
SELECT id, tenant_id, team_id, digital_employee_id, name, encrypted_value,
       encryption_key_id, value_fingerprint, sensitive, status,
       created_by, updated_by, created_at, updated_at
FROM digital_employee_environment_variables
WHERE tenant_id = $1
  AND digital_employee_id = $2
  AND status = 'active'
  AND deleted_at IS NULL
ORDER BY name ASC
`, tenantID, digitalEmployeeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]EnvironmentVariableRecord, 0)
	for rows.Next() {
		record, err := scanEnvironmentVariableRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (r *PgRepository) envSQLStore() (queries.DBTX, error) {
	if r.sql == nil {
		return nil, fmt.Errorf("%w: employee repository sql store is required", ErrInvalidInput)
	}
	return r.sql, nil
}

func (r *PgRepository) UpsertWorkspaceFileSync(ctx context.Context, params UpsertWorkspaceFileSyncParams) error {
	return r.q.UpsertDigitalEmployeeWorkspaceFileSync(ctx, queries.UpsertDigitalEmployeeWorkspaceFileSyncParams{
		TenantID:            params.TenantID,
		DigitalEmployeeID:   params.DigitalEmployeeID,
		ExecutionInstanceID: params.ExecutionInstanceID,
		FileID:              params.FileID,
		RevisionID:          params.RevisionID,
		RuntimeNodeID:       params.RuntimeNodeID,
		Status:              params.Status,
		SyncedHash:          textFromPtr(params.SyncedHash),
		ErrorMessage:        textFromPtr(params.ErrorMessage),
		LastCommandID:       textFromPtr(params.LastCommandID),
		LastSyncedAt:        timestamptzFromPtr(params.LastSyncedAt),
	})
}

func (r *PgRepository) CreateDigitalEmployeeConfigRevision(ctx context.Context, params CreateConfigRevisionParams) (DigitalEmployeeConfigRevisionRecord, error) {
	capabilityBindings, err := jsonbFromMap(params.CapabilityBindings, "capability_bindings")
	if err != nil {
		return DigitalEmployeeConfigRevisionRecord{}, err
	}
	budgetPolicy, err := jsonbFromMap(params.BudgetPolicy, "budget_policy")
	if err != nil {
		return DigitalEmployeeConfigRevisionRecord{}, err
	}
	revision, err := r.q.CreateDigitalEmployeeConfigRevision(ctx, queries.CreateDigitalEmployeeConfigRevisionParams{
		TenantID:              params.TenantID,
		DigitalEmployeeID:     params.DigitalEmployeeID,
		RevisionNumber:        params.RevisionNumber,
		PersonaMemoryMarkdown: params.PersonaMemoryMarkdown,
		CapabilityBindings:    capabilityBindings,
		BudgetPolicy:          budgetPolicy,
		Status:                string(params.Status),
		ApprovedBy:            nullUUIDFromPtr(params.ApprovedBy),
		ApprovedAt:            timestamptzFromPtr(params.ApprovedAt),
	})
	if err != nil {
		return DigitalEmployeeConfigRevisionRecord{}, err
	}
	return configRevisionRecordFromQuery(digitalEmployeeConfigRevisionRecordAdapter{
		digitalEmployeeConfigRevisionQueryAdapter: digitalEmployeeConfigRevisionQueryAdapter{
			id:                    revision.ID,
			tenantID:              revision.TenantID,
			digitalEmployeeID:     revision.DigitalEmployeeID,
			revisionNumber:        revision.RevisionNumber,
			personaMemoryMarkdown: revision.PersonaMemoryMarkdown,
			capabilityBindings:    revision.CapabilityBindings,
			budgetPolicy:          revision.BudgetPolicy,
		},
		status:     revision.Status,
		approvedBy: revision.ApprovedBy,
		approvedAt: revision.ApprovedAt,
		archivedAt: revision.ArchivedAt,
		createdAt:  revision.CreatedAt,
		updatedAt:  revision.UpdatedAt,
	})
}

func (r *PgRepository) GetDigitalEmployeeConfigRevision(ctx context.Context, tenantID, digitalEmployeeID, employeeConfigRevisionID uuid.UUID) (EmployeeConfigInput, error) {
	revision, err := r.q.GetDigitalEmployeeConfigRevision(ctx, queries.GetDigitalEmployeeConfigRevisionParams{
		ID:                employeeConfigRevisionID,
		TenantID:          tenantID,
		DigitalEmployeeID: digitalEmployeeID,
	})
	if err != nil {
		return EmployeeConfigInput{}, mapNoRows(err)
	}
	return employeeConfigInputFromQuery(digitalEmployeeConfigRevisionQueryAdapter{
		id:                    revision.ID,
		tenantID:              revision.TenantID,
		digitalEmployeeID:     revision.DigitalEmployeeID,
		revisionNumber:        revision.RevisionNumber,
		personaMemoryMarkdown: revision.PersonaMemoryMarkdown,
		capabilityBindings:    revision.CapabilityBindings,
		budgetPolicy:          revision.BudgetPolicy,
	})
}

func (r *PgRepository) GetLatestDigitalEmployeeConfigRevision(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (EmployeeConfigInput, error) {
	revision, err := r.q.GetLatestDigitalEmployeeConfigRevision(ctx, queries.GetLatestDigitalEmployeeConfigRevisionParams{
		TenantID:          tenantID,
		DigitalEmployeeID: digitalEmployeeID,
	})
	if err != nil {
		return EmployeeConfigInput{}, mapNoRows(err)
	}
	return employeeConfigInputFromQuery(digitalEmployeeConfigRevisionQueryAdapter{
		id:                    revision.ID,
		tenantID:              revision.TenantID,
		digitalEmployeeID:     revision.DigitalEmployeeID,
		revisionNumber:        revision.RevisionNumber,
		personaMemoryMarkdown: revision.PersonaMemoryMarkdown,
		capabilityBindings:    revision.CapabilityBindings,
		budgetPolicy:          revision.BudgetPolicy,
	})
}

func (r *PgRepository) GetNextDigitalEmployeeConfigRevisionNumber(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (int32, error) {
	nextRevision, err := r.q.GetNextDigitalEmployeeConfigRevisionNumber(ctx, queries.GetNextDigitalEmployeeConfigRevisionNumberParams{
		TenantID:          tenantID,
		DigitalEmployeeID: digitalEmployeeID,
	})
	if err != nil {
		return 0, err
	}
	return nextRevision, nil
}

func (r *PgRepository) GetSchedulingCapabilityFacts(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (SchedulingCapabilityFacts, error) {
	skillCounts, err := r.q.GetDigitalEmployeeSchedulingSkillCounts(ctx, queries.GetDigitalEmployeeSchedulingSkillCountsParams{
		TenantID:          tenantID,
		DigitalEmployeeID: digitalEmployeeID,
	})
	if err != nil {
		return SchedulingCapabilityFacts{}, mapNoRows(err)
	}
	mcpRows, err := r.q.ListEffectiveMCPBindingsV2ForEmployee(ctx, queries.ListEffectiveMCPBindingsV2ForEmployeeParams{
		TenantID:          tenantID,
		DigitalEmployeeID: digitalEmployeeID,
	})
	if err != nil {
		return SchedulingCapabilityFacts{}, err
	}
	configuredEnvVars, err := r.q.ListConfiguredEmployeeEnvVarNames(ctx, queries.ListConfiguredEmployeeEnvVarNamesParams{
		TenantID:          tenantID,
		DigitalEmployeeID: digitalEmployeeID,
	})
	if err != nil {
		return SchedulingCapabilityFacts{}, err
	}

	var personalMCPServerCount int
	var inheritedMCPServerCount int
	for _, row := range mcpRows {
		switch row.SourceScope {
		case "employee":
			personalMCPServerCount++
		case "team":
			inheritedMCPServerCount++
		}
	}
	configuredEnvVarCount, missingNames := schedulingCapabilityEnvFacts(mcpRows, configuredEnvVars)

	return SchedulingCapabilityFacts{
		PersonalSkillCount:      int(skillCounts.PersonalSkillCount),
		InheritedSkillCount:     int(skillCounts.InheritedSkillCount),
		MissingRequiredSkills:   []string{},
		PersonalMCPServerCount:  personalMCPServerCount,
		InheritedMCPServerCount: inheritedMCPServerCount,
		ConfiguredEnvVarCount:   configuredEnvVarCount,
		MissingEnvironmentNames: missingNames,
	}, nil
}

func schedulingCapabilityEnvFacts(mcpRows []queries.ListEffectiveMCPBindingsV2ForEmployeeRow, configuredEnvVars []string) (int, []string) {
	configured := make(map[string]struct{}, len(configuredEnvVars))
	for _, name := range configuredEnvVars {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			configured[trimmed] = struct{}{}
		}
	}

	missingEnvironmentNames := make(map[string]struct{})
	for _, row := range mcpRows {
		for _, name := range row.RequiredEnvVars {
			addMissingSchedulingEnvName(missingEnvironmentNames, configured, name)
		}
		if row.CredentialEnvVar.Valid {
			addMissingSchedulingEnvName(missingEnvironmentNames, configured, row.CredentialEnvVar.String)
		}
	}

	missingNames := make([]string, 0, len(missingEnvironmentNames))
	for name := range missingEnvironmentNames {
		missingNames = append(missingNames, name)
	}
	sort.Strings(missingNames)
	return len(configured), missingNames
}

func addMissingSchedulingEnvName(missing, configured map[string]struct{}, name string) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return
	}
	if _, ok := configured[trimmed]; !ok {
		missing[trimmed] = struct{}{}
	}
}

func (r *PgRepository) GetDigitalEmployeeRunStats(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (DigitalEmployeeRunStats, error) {
	row, err := r.q.GetDigitalEmployeeRunStats(ctx, queries.GetDigitalEmployeeRunStatsParams{
		TenantID:          tenantID,
		DigitalEmployeeID: digitalEmployeeID,
	})
	if err != nil {
		return DigitalEmployeeRunStats{}, mapNoRows(err)
	}
	return DigitalEmployeeRunStats{
		TotalCount:     row.TotalCount,
		SucceededCount: row.SucceededCount,
		FailedCount:    row.FailedCount,
		CancelledCount: row.CancelledCount,
		Last7dCount:    row.Last7dCount,
		Prev7dCount:    row.Prev7dCount,
		AvgDurationSec: pgFloat8Ptr(row.AvgDurationSec),
		P90DurationSec: pgFloat8Ptr(row.P90DurationSec),
	}, nil
}

// ListRunsDetailed forwards to the run-scoped PgRunRepository so the broad Repository
// interface also exposes the enriched run history list. This mirrors the
// GetDigitalEmployeeRunStats forwarding above.
func (r *PgRepository) ListRunsDetailed(ctx context.Context, tenantID, employeeID uuid.UUID, filter DigitalEmployeeRunListFilter) (*DigitalEmployeeRunListResult, error) {
	return (&PgRunRepository{q: r.q}).ListRunsDetailed(ctx, tenantID, employeeID, filter)
}

func pgFloat8Ptr(value pgtype.Float8) *float64 {
	if !value.Valid {
		return nil
	}
	v := value.Float64
	return &v
}

func (r *PgRepository) GetDigitalEmployeeOverview(ctx context.Context, req GetDigitalEmployeeOverviewRequest) (*DigitalEmployeeOverview, error) {
	summaryParams := queries.GetDigitalEmployeeOverviewSummaryParams{
		TenantID:        req.TenantID,
		Q:               textFromOptionalString(req.Query),
		TeamID:          nullUUIDFromPtr(req.TeamID),
		Status:          textFromOptionalString(string(req.Status)),
		EmployeeType:    textFromOptionalString(req.EmployeeType),
		ProviderType:    textFromOptionalString(req.ProviderType),
		RuntimeNodeID:   nullUUIDFromPtr(req.RuntimeNodeID),
		RiskLevel:       textFromOptionalString(req.RiskLevel),
		ExecutionStatus: textFromOptionalString(string(req.ExecutionStatus)),
		RunStatus:       textFromOptionalString(string(req.RunStatus)),
	}
	summary, err := r.q.GetDigitalEmployeeOverviewSummary(ctx, summaryParams)
	if err != nil {
		return nil, err
	}

	operationalFactRows, err := r.q.ListDigitalEmployeeOverviewOperationalFacts(ctx, queries.ListDigitalEmployeeOverviewOperationalFactsParams{
		TenantID:        req.TenantID,
		Q:               summaryParams.Q,
		TeamID:          summaryParams.TeamID,
		Status:          summaryParams.Status,
		EmployeeType:    summaryParams.EmployeeType,
		ProviderType:    summaryParams.ProviderType,
		RuntimeNodeID:   summaryParams.RuntimeNodeID,
		RiskLevel:       summaryParams.RiskLevel,
		ExecutionStatus: summaryParams.ExecutionStatus,
		RunStatus:       summaryParams.RunStatus,
	})
	if err != nil {
		return nil, err
	}
	operationalStates := make([]DigitalEmployeeOperationalState, 0, len(operationalFactRows))
	for _, row := range operationalFactRows {
		operationalStates = append(operationalStates, overviewOperationalStateFromFactsRow(row))
	}

	itemRows, err := r.q.ListDigitalEmployeeOverviewItems(ctx, queries.ListDigitalEmployeeOverviewItemsParams{
		TenantID:        req.TenantID,
		Q:               summaryParams.Q,
		TeamID:          summaryParams.TeamID,
		Status:          summaryParams.Status,
		EmployeeType:    summaryParams.EmployeeType,
		ProviderType:    summaryParams.ProviderType,
		RuntimeNodeID:   summaryParams.RuntimeNodeID,
		RiskLevel:       summaryParams.RiskLevel,
		ExecutionStatus: summaryParams.ExecutionStatus,
		RunStatus:       summaryParams.RunStatus,
		Limit:           req.Limit,
		Offset:          req.Offset,
	})
	if err != nil {
		return nil, err
	}
	labelsByType, err := r.ListEmployeeTemplateLabels(ctx, req.TenantID)
	if err != nil {
		return nil, err
	}
	items := make([]DigitalEmployeeOverviewItem, 0, len(itemRows))
	for _, row := range itemRows {
		items = append(items, overviewItemFromQuery(row, labelsByType))
	}

	filterRows, err := r.q.ListDigitalEmployeeOverviewFilterOptions(ctx, req.TenantID)
	if err != nil {
		return nil, err
	}

	return &DigitalEmployeeOverview{
		Summary: DigitalEmployeeOverviewSummary{
			TotalCount:                 summary.TotalCount,
			RunnableCount:              summary.RunnableCount,
			RunningCount:               summary.RunningCount,
			WaitingRuntimeCount:        summary.WaitingRuntimeCount,
			ErrorCount:                 summary.ErrorCount,
			HighRiskCount:              summary.HighRiskCount,
			ReadyCount:                 summary.ReadyCount,
			PendingRuntimeBindingCount: summary.PendingRuntimeBindingCount,
			PendingConfigApprovalCount: summary.PendingConfigApprovalCount,
			FailedRecentRunCount:       summary.FailedRecentRunCount,
			OperationalStatusCounts:    overviewOperationalStatusCountsFromStates(operationalStates),
		},
		QueueSummary: DigitalEmployeeOverviewQueueSummary{
			PendingRuntimeBindingCount: summary.PendingRuntimeBindingCount,
			StaleConfigCount:           summary.StaleConfigCount,
			FailedRecentRunCount:       summary.FailedRecentRunCount,
		},
		Items:   items,
		Filters: overviewFiltersFromQuery(filterRows, labelsByType),
		Pagination: OverviewPagination{
			Limit:      req.Limit,
			Offset:     req.Offset,
			TotalCount: summary.TotalCount,
		},
	}, nil
}

// AreRuntimeReady reports which of the given digital employees are runtime-ready,
// using the digital_employee_runtime_readiness view whose predicate matches the
// overview "runnable" classification exactly.
func (r *PgRepository) AreRuntimeReady(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]bool, error) {
	if len(employeeIDs) == 0 {
		return map[uuid.UUID]bool{}, nil
	}
	rows, err := r.q.AreEmployeesRuntimeReady(ctx, queries.AreEmployeesRuntimeReadyParams{
		TenantID:           tenantID,
		DigitalEmployeeIds: employeeIDs,
	})
	if err != nil {
		return nil, err
	}
	ready := make(map[uuid.UUID]bool, len(rows))
	for _, row := range rows {
		ready[row.DigitalEmployeeID] = row.IsRuntimeReady
	}
	return ready, nil
}

// OperationalSignals carries the per-employee load and reliability counts that the
// planning profile builder uses to score how busy and how reliable a digital employee
// is. All counts are scoped to a recent window (currently 30 days) so the signal
// reflects current behaviour.
type OperationalSignals struct {
	InFlightAttemptCount   int32
	RecentSuccessCount     int32
	RecentFailureCount     int32
	RecentHumanRejectCount int32
}

// GetDigitalEmployeeOperationalSignals batch-loads recent load and reliability counts
// for the given digital employees. Employees with no recent attempts are omitted from
// the result; callers should treat absence as zero signals.
func (r *PgRepository) GetDigitalEmployeeOperationalSignals(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]OperationalSignals, error) {
	if len(employeeIDs) == 0 {
		return map[uuid.UUID]OperationalSignals{}, nil
	}
	rows, err := r.q.CountDigitalEmployeeOperationalSignals(ctx, queries.CountDigitalEmployeeOperationalSignalsParams{
		TenantID:           tenantID,
		DigitalEmployeeIds: employeeIDs,
	})
	if err != nil {
		return nil, err
	}
	signals := make(map[uuid.UUID]OperationalSignals, len(rows))
	for _, row := range rows {
		if !row.DigitalEmployeeID.Valid {
			continue
		}
		signals[row.DigitalEmployeeID.UUID] = OperationalSignals{
			InFlightAttemptCount:   row.InFlightAttemptCount,
			RecentSuccessCount:     row.RecentSuccessCount,
			RecentFailureCount:     row.RecentFailureCount,
			RecentHumanRejectCount: row.RecentHumanRejectCount,
		}
	}
	return signals, nil
}

func overviewItemFromQuery(row queries.ListDigitalEmployeeOverviewItemsRow, labelsByType map[string]string) DigitalEmployeeOverviewItem {
	executionStatus := overviewExecutionStatus(row.ExecutionStatus)
	latestRunStatus := overviewRunStatus(row.LatestRunStatus)
	dailyTokenLimit := int32PtrFromJSONString(row.DailyTokenLimitText)
	usagePercent := overviewUsagePercent(row.TodayBudgetUsageTokens, dailyTokenLimit)
	recentEvents := recentEventsFromJSON(row.RecentEventsJson)
	workbenchInput := overviewWorkbenchStatusInput{
		IdentityStatus:        DigitalEmployeeStatus(row.Status),
		ExecutionStatus:       executionStatus,
		RuntimeStatus:         row.RuntimeStatus,
		RuntimeDisabled:       row.RuntimeDisabledAt.Valid,
		RuntimeArchived:       row.RuntimeArchivedAt.Valid,
		NodeID:                row.NodeID,
		ProviderType:          row.ProviderType,
		ProviderAvailable:     row.ProviderAvailable,
		ProviderStatus:        row.ProviderStatus,
		HealthStatus:          row.HealthStatus,
		AgentHomeDirAvailable: row.AgentHomeDirAvailable,
		GovernanceStatus:      row.GovernanceStatus,
		RunStatus:             latestRunStatus,
	}
	workbenchStatus := overviewWorkbenchStatus(workbenchInput)
	operationalState := overviewOperationalStateFromInput(workbenchInput, workbenchStatus, latestRunStatus, row.LatestRunErrorFamily, overviewOperationalFacts{
		HasEmployeeScopedHumanBlocker: row.OperationalHasEmployeeScopedHumanBlocker,
		HasProjectAcceptanceBlocker:   row.OperationalHasProjectAcceptanceBlocker,
		HasQueuedWork:                 row.OperationalHasQueuedWork,
		HasWorkingTask:                row.OperationalHasWorkingTask,
		HasActiveWork:                 row.OperationalHasActiveWork,
		HasTaskFailure:                row.OperationalHasTaskFailure,
	})

	var latestRun *DigitalEmployeeLatestRunSummary
	if row.LatestRunID.Valid && row.LatestRunID.UUID != uuid.Nil {
		latestRun = &DigitalEmployeeLatestRunSummary{
			RunID:        row.LatestRunID.UUID,
			TaskID:       row.LatestRunTaskID.UUID,
			Status:       latestRunStatus,
			Title:        row.LatestRunTitle,
			StartedAt:    timePtrFromPgTimestamptz(row.LatestRunStartedAt),
			UpdatedAt:    timePtrFromPgTimestamptz(row.LatestRunUpdatedAt),
			FinishedAt:   timePtrFromPgTimestamptz(row.LatestRunFinishedAt),
			DurationSec:  int32PtrFromJSONString(row.LatestRunDurationSec),
			TokenUsage:   int32PtrFromJSONString(row.LatestRunTokenUsage),
			ErrorMessage: stringFromText(row.LatestRunErrorMessage),
		}
	}

	budgetUsage := int32PtrFromPgInt4(row.BudgetUsageTokens30d)
	var budgetUsageValue int32
	if budgetUsage != nil {
		budgetUsageValue = *budgetUsage
	}

	return DigitalEmployeeOverviewItem{
		IdentitySummary: DigitalEmployeeIdentitySummary{
			ID:                row.ID,
			TenantID:          row.TenantID,
			TeamID:            uuidPtrFromNullUUID(row.TeamID),
			TeamName:          row.TeamName,
			OwnerUserID:       row.OwnerUserID,
			OwnerDisplayName:  row.OwnerDisplayName,
			EmployeeType:      row.EmployeeType,
			EmployeeTypeLabel: overviewEmployeeTypeLabel(row.EmployeeType, labelsByType),
			Name:              row.Name,
			Role:              row.Role,
			Description:       stringPtrFromPgText(row.Description),
			Status:            DigitalEmployeeStatus(row.Status),
			RiskLevel:         row.RiskLevel,
			AvatarAsset:       avatarAssetFromOverviewMetadata(row.Metadata),
		},
		ExecutionSummary: DigitalEmployeeExecutionSummary{
			ExecutionInstanceID:   uuidPtrFromNullUUID(row.ExecutionInstanceID),
			Status:                executionStatus,
			RuntimeNodeID:         uuidPtrFromNullUUID(row.RuntimeNodeID),
			NodeID:                row.NodeID,
			RuntimeName:           row.RuntimeName,
			RuntimeStatus:         row.RuntimeStatus,
			ProviderType:          row.ProviderType,
			ProviderStatus:        row.ProviderStatus,
			HealthStatus:          row.HealthStatus,
			AgentHomeDirAvailable: row.AgentHomeDirAvailable,
		},
		LatestRunSummary: latestRun,
		GovernanceSummary: DigitalEmployeeGovernanceSummary{
			EffectiveConfigID:      uuidPtrFromNullUUID(row.EffectiveConfigID),
			Status:                 row.GovernanceStatus,
			TeamRevisionNumber:     int32PtrFromPgInt4(row.TeamRevisionNumber),
			EmployeeRevisionNumber: int32PtrFromPgInt4(row.EmployeeRevisionNumber),
			SkillsCount:            row.SkillsCount,
			MCPServersCount:        row.McpServersCount,
			ConstitutionRef:        row.ConstitutionRef,
		},
		BudgetSummary: DigitalEmployeeBudgetSummary{
			DailyTokenLimit:   dailyTokenLimit,
			UsageTokensToday:  row.TodayBudgetUsageTokens,
			UsagePercentToday: usagePercent,
			LimitExceeded:     dailyTokenLimit != nil && row.TodayBudgetUsageTokens >= *dailyTokenLimit,
			UsageTokens30d:    budgetUsage,
			RunCount30d:       row.BudgetRunCount30d,
			Currency:          "USD",
			Source:            overviewBudgetSource(row.BudgetRunCount30d, budgetUsageValue),
		},
		WorkbenchStatus:  workbenchStatus,
		OperationalState: operationalState,
		RecentEvents:     recentEvents,
	}
}

type overviewOperationalFacts struct {
	HasEmployeeScopedHumanBlocker bool
	HasProjectAcceptanceBlocker   bool
	HasQueuedWork                 bool
	HasWorkingTask                bool
	HasActiveWork                 bool
	HasTaskFailure                bool
}

func overviewOperationalStateFromFactsRow(row queries.ListDigitalEmployeeOverviewOperationalFactsRow) DigitalEmployeeOperationalState {
	latestRunStatus := overviewRunStatus(row.LatestRunStatus)
	workbenchInput := overviewWorkbenchStatusInput{
		IdentityStatus:        DigitalEmployeeStatus(row.Status),
		ExecutionStatus:       overviewExecutionStatus(row.ExecutionStatus),
		RuntimeStatus:         row.RuntimeStatus,
		RuntimeDisabled:       row.RuntimeDisabledAt.Valid,
		RuntimeArchived:       row.RuntimeArchivedAt.Valid,
		NodeID:                row.NodeID,
		ProviderType:          row.ProviderType,
		ProviderAvailable:     row.ProviderAvailable,
		ProviderStatus:        row.ProviderStatus,
		HealthStatus:          row.HealthStatus,
		AgentHomeDirAvailable: row.AgentHomeDirAvailable,
		GovernanceStatus:      row.GovernanceStatus,
		RunStatus:             latestRunStatus,
	}
	return overviewOperationalStateFromInput(workbenchInput, overviewWorkbenchStatus(workbenchInput), latestRunStatus, row.LatestRunErrorFamily, overviewOperationalFacts{
		HasEmployeeScopedHumanBlocker: row.OperationalHasEmployeeScopedHumanBlocker,
		HasProjectAcceptanceBlocker:   row.OperationalHasProjectAcceptanceBlocker,
		HasQueuedWork:                 row.OperationalHasQueuedWork,
		HasWorkingTask:                row.OperationalHasWorkingTask,
		HasActiveWork:                 row.OperationalHasActiveWork,
		HasTaskFailure:                row.OperationalHasTaskFailure,
	})
}

func overviewOperationalStateFromInput(workbenchInput overviewWorkbenchStatusInput, workbenchStatus WorkbenchStatus, latestRunStatus OverviewRunStatus, latestRunErrorFamily string, facts overviewOperationalFacts) DigitalEmployeeOperationalState {
	baseDispatchReady := overviewOperationalDispatchReady(workbenchInput)
	hasRunQueued := latestRunStatus == OverviewRunStatusQueued || latestRunStatus == OverviewRunStatusDispatching
	hasRunWorking := latestRunStatus == OverviewRunStatusRunning || latestRunStatus == OverviewRunStatusCancelling
	hasRunFailed := latestRunStatus == OverviewRunStatusFailed || latestRunStatus == OverviewRunStatusTimedOut
	latestRunErrorFamily = strings.TrimSpace(latestRunErrorFamily)
	hasRunProviderFailure := hasRunFailed && (latestRunErrorFamily == "dispatch_failed" || latestRunErrorFamily == "provider" || strings.HasPrefix(latestRunErrorFamily, "provider_"))

	return ResolveDigitalEmployeeOperationalState(DigitalEmployeeOperationalInput{
		DispatchReady:                 baseDispatchReady,
		ConfigurationMissing:          !baseDispatchReady && workbenchStatus == WorkbenchStatusPendingBinding,
		RuntimeUnavailable:            workbenchInput.RuntimeDisabled || workbenchInput.RuntimeArchived || strings.TrimSpace(workbenchInput.RuntimeStatus) != "online",
		HasProviderFailure:            hasRunProviderFailure,
		HasTaskFailure:                facts.HasTaskFailure || (hasRunFailed && !hasRunProviderFailure),
		HasActiveWork:                 facts.HasActiveWork || hasRunQueued || hasRunWorking,
		HasWorkingRun:                 facts.HasWorkingTask || hasRunWorking,
		HasQueuedWork:                 facts.HasQueuedWork || hasRunQueued,
		HasEmployeeScopedHumanBlocker: facts.HasEmployeeScopedHumanBlocker,
		HasProjectAcceptanceBlocker:   facts.HasProjectAcceptanceBlocker,
	})
}

func overviewOperationalStatusCountsFromStates(states []DigitalEmployeeOperationalState) map[DigitalEmployeeOperationalStatus]int32 {
	counts := make(map[DigitalEmployeeOperationalStatus]int32)
	for _, state := range states {
		counts[state.Status]++
	}
	return counts
}

func avatarAssetFromOverviewMetadata(metadata []byte) *DigitalEmployeeAvatarAsset {
	return AvatarAssetFromMetadata(jsonMapFromBytes(metadata))
}

func overviewFiltersFromQuery(rows []queries.ListDigitalEmployeeOverviewFilterOptionsRow, labelsByType map[string]string) DigitalEmployeeOverviewFilters {
	filters := DigitalEmployeeOverviewFilters{
		Teams:             []OverviewFilterOption{},
		Statuses:          []OverviewFilterOption{},
		EmployeeTypes:     []OverviewFilterOption{},
		Providers:         []OverviewFilterOption{},
		RuntimeNodes:      []OverviewFilterOption{},
		RiskLevels:        []OverviewFilterOption{},
		ExecutionStatuses: []OverviewFilterOption{},
		RunStatuses:       []OverviewFilterOption{},
	}
	for _, row := range rows {
		value := strings.TrimSpace(row.Value)
		if value == "" {
			continue
		}
		label := strings.TrimSpace(row.Label)
		if label == "" {
			label = value
		}
		label = overviewFilterLabel(row.FilterType, value, label, labelsByType)
		option := OverviewFilterOption{Value: value, Label: label}
		switch row.FilterType {
		case "team":
			filters.Teams = append(filters.Teams, option)
		case "employee_type":
			filters.EmployeeTypes = append(filters.EmployeeTypes, option)
		case "status":
			filters.Statuses = append(filters.Statuses, option)
		case "provider":
			filters.Providers = append(filters.Providers, option)
		case "runtime_node":
			filters.RuntimeNodes = append(filters.RuntimeNodes, option)
		case "risk_level":
			filters.RiskLevels = append(filters.RiskLevels, option)
		case "execution_status":
			filters.ExecutionStatuses = append(filters.ExecutionStatuses, option)
		case "run_status":
			filters.RunStatuses = append(filters.RunStatuses, option)
		}
	}
	return filters
}

func overviewFilterLabel(filterType, value, fallback string, labelsByType map[string]string) string {
	switch filterType {
	case "employee_type":
		return overviewEmployeeTypeLabel(value, labelsByType)
	case "status":
		return overviewStatusLabel(value, fallback)
	case "risk_level":
		return overviewRiskLevelLabel(value, fallback)
	case "execution_status":
		return overviewExecutionStatusLabel(value, fallback)
	case "run_status":
		return overviewRunStatusLabel(value, fallback)
	case "provider", "provider_type":
		return overviewProviderLabel(value, fallback)
	default:
		if fallback != "" {
			return fallback
		}
		return value
	}
}

func overviewEmployeeTypeLabel(value string, labelsByType map[string]string) string {
	if value == "custom_agent" {
		return customAgentEmployeeTypeDefinition().Label
	}
	if label, ok := labelsByType[value]; ok && label != "" {
		return label
	}
	return value
}

func overviewStatusLabel(value, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "draft":
		return "草稿"
	case "ready":
		return "已就绪"
	case "active":
		return "活跃中"
	case "disabled":
		return "已禁用"
	case "error":
		return "异常"
	default:
		return fallback
	}
}

func overviewRiskLevelLabel(value, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low":
		return "低风险"
	case "normal":
		return "普通风险"
	case "medium":
		return "中风险"
	case "high":
		return "高风险"
	case "critical":
		return "严重风险"
	default:
		return fallback
	}
}

func overviewExecutionStatusLabel(value, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(OverviewExecutionStatusMissing):
		return "未绑定 Runtime"
	case string(OverviewExecutionStatusProvisioning):
		return "准备中"
	case string(OverviewExecutionStatusReady):
		return "已就绪"
	case string(OverviewExecutionStatusActive):
		return "活跃中"
	case string(OverviewExecutionStatusDisabled):
		return "已禁用"
	case string(OverviewExecutionStatusError):
		return "异常"
	default:
		return fallback
	}
}

func overviewRunStatusLabel(value, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(OverviewRunStatusNone):
		return "暂无运行"
	case string(OverviewRunStatusQueued):
		return "排队中"
	case string(OverviewRunStatusDispatching):
		return "下发中"
	case string(OverviewRunStatusRunning):
		return "运行中"
	case string(OverviewRunStatusCancelling):
		return "取消中"
	case string(OverviewRunStatusCompleted):
		return "已完成"
	case string(OverviewRunStatusFailed):
		return "失败"
	case string(OverviewRunStatusCancelled):
		return "已取消"
	case string(OverviewRunStatusTimedOut):
		return "已超时"
	default:
		return fallback
	}
}

func overviewProviderLabel(value, fallback string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "codex":
		return "Codex"
	case "claude-code", "claude_code", "claude":
		return "Claude Code"
	case "opencode", "open-code", "open_code":
		return "OpenCode"
	default:
		return fallback
	}
}

func int32FromJSONString(value string) int32 {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0
	}
	if strings.HasPrefix(trimmed, `"`) {
		var decoded string
		if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
			trimmed = strings.TrimSpace(decoded)
		}
	}
	parsed, err := strconv.ParseInt(trimmed, 10, 32)
	if err != nil {
		return 0
	}
	return int32(parsed)
}

func overviewExecutionStatus(value string) OverviewExecutionStatus {
	status := OverviewExecutionStatus(strings.TrimSpace(value))
	switch status {
	case OverviewExecutionStatusProvisioning, OverviewExecutionStatusReady, OverviewExecutionStatusActive, OverviewExecutionStatusDisabled, OverviewExecutionStatusError:
		return status
	default:
		return OverviewExecutionStatusMissing
	}
}

func overviewRunStatus(value string) OverviewRunStatus {
	status := OverviewRunStatus(strings.TrimSpace(value))
	switch status {
	case OverviewRunStatusQueued, OverviewRunStatusDispatching, OverviewRunStatusRunning, OverviewRunStatusCancelling, OverviewRunStatusCompleted, OverviewRunStatusFailed, OverviewRunStatusCancelled, OverviewRunStatusTimedOut:
		return status
	default:
		return OverviewRunStatusNone
	}
}

type overviewWorkbenchStatusInput struct {
	IdentityStatus        DigitalEmployeeStatus
	ExecutionStatus       OverviewExecutionStatus
	RuntimeStatus         string
	RuntimeDisabled       bool
	RuntimeArchived       bool
	NodeID                string
	ProviderType          string
	ProviderAvailable     bool
	ProviderStatus        string
	HealthStatus          string
	AgentHomeDirAvailable bool
	GovernanceStatus      string
	RunStatus             OverviewRunStatus
}

func overviewWorkbenchStatus(input overviewWorkbenchStatusInput) WorkbenchStatus {
	if input.IdentityStatus == DigitalEmployeeStatusDisabled ||
		input.IdentityStatus == DigitalEmployeeStatusError ||
		input.ExecutionStatus == OverviewExecutionStatusDisabled ||
		input.ExecutionStatus == OverviewExecutionStatusError ||
		input.RunStatus == OverviewRunStatusFailed ||
		input.RunStatus == OverviewRunStatusTimedOut {
		return WorkbenchStatusError
	}
	if input.ExecutionStatus == OverviewExecutionStatusMissing ||
		input.ExecutionStatus == OverviewExecutionStatusProvisioning ||
		strings.TrimSpace(input.RuntimeStatus) == "" ||
		strings.TrimSpace(input.NodeID) == "" ||
		strings.TrimSpace(input.ProviderType) == "" ||
		!input.AgentHomeDirAvailable {
		return WorkbenchStatusPendingBinding
	}
	if input.GovernanceStatus == "missing" ||
		input.GovernanceStatus == "pending_approval" ||
		input.GovernanceStatus == "stale" {
		return WorkbenchStatusPendingBinding
	}
	if input.IdentityStatus != DigitalEmployeeStatusReady &&
		input.IdentityStatus != DigitalEmployeeStatusActive {
		return WorkbenchStatusPendingBinding
	}
	if input.ExecutionStatus != OverviewExecutionStatusReady &&
		input.ExecutionStatus != OverviewExecutionStatusActive {
		return WorkbenchStatusPendingBinding
	}
	if input.GovernanceStatus != "approved" {
		return WorkbenchStatusPendingBinding
	}
	if input.RuntimeDisabled ||
		input.RuntimeArchived ||
		strings.TrimSpace(input.RuntimeStatus) != "online" {
		return WorkbenchStatusError
	}
	if !input.ProviderAvailable ||
		strings.TrimSpace(input.ProviderStatus) != "healthy" ||
		strings.TrimSpace(input.HealthStatus) != "healthy" {
		return WorkbenchStatusError
	}
	return WorkbenchStatusReady
}

func overviewOperationalDispatchReady(input overviewWorkbenchStatusInput) bool {
	input.RunStatus = OverviewRunStatusNone
	return overviewWorkbenchStatus(input) == WorkbenchStatusReady
}

func overviewUsagePercent(today int32, limit *int32) *int32 {
	if limit == nil || *limit <= 0 {
		return nil
	}
	percent := int32((int64(today) * 100) / int64(*limit))
	if percent > 100 {
		percent = 100
	}
	return &percent
}

func overviewBudgetSource(runCount, usageTokens int32) string {
	if runCount <= 0 || usageTokens <= 0 {
		return "unavailable"
	}
	return "run_usage_projection"
}

func recentEventsFromJSON(raw []byte) []DigitalEmployeeRecentEventSummary {
	if len(raw) == 0 {
		return []DigitalEmployeeRecentEventSummary{}
	}
	var payload []struct {
		Label      string     `json:"label"`
		Status     string     `json:"status"`
		OccurredAt *time.Time `json:"occurred_at"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return []DigitalEmployeeRecentEventSummary{}
	}
	events := make([]DigitalEmployeeRecentEventSummary, 0, len(payload))
	for _, item := range payload {
		events = append(events, DigitalEmployeeRecentEventSummary{
			Label:      item.Label,
			Status:     item.Status,
			OccurredAt: item.OccurredAt,
		})
	}
	return events
}

func textFromOptionalString(value string) pgtype.Text {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: trimmed, Valid: true}
}

func uuidPtrFromNullUUID(value uuid.NullUUID) *uuid.UUID {
	return uuidPtrFromNull(value)
}

func stringPtrFromPgText(value pgtype.Text) *string {
	return stringPtrFromText(value)
}

func int32PtrFromPgInt4(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	copied := value.Int32
	return &copied
}

func timePtrFromPgTimestamptz(value pgtype.Timestamptz) *time.Time {
	return timePtrFromTimestamptz(value)
}

func int32PtrFromJSONString(value string) *int32 {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if strings.HasPrefix(trimmed, `"`) {
		var decoded string
		if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
			return nil
		}
		trimmed = strings.TrimSpace(decoded)
		if trimmed == "" {
			return nil
		}
	}
	parsed, err := strconv.ParseInt(trimmed, 10, 32)
	if err != nil {
		return nil
	}
	copied := int32(parsed)
	return &copied
}

func workspaceFileRecordFromQuery(row queries.DigitalEmployeeWorkspaceFile) (WorkspaceFileRecord, error) {
	metadata, err := mapFromJSONB(row.Metadata, "metadata")
	if err != nil {
		return WorkspaceFileRecord{}, err
	}
	return WorkspaceFileRecord{
		ID:                row.ID,
		TenantID:          row.TenantID,
		TeamID:            uuidPtrFromNullUUID(row.TeamID),
		DigitalEmployeeID: row.DigitalEmployeeID,
		Path:              row.Path,
		FileRole:          row.FileRole,
		MimeType:          row.MimeType,
		SyncPolicy:        row.SyncPolicy,
		CurrentRevisionID: uuidPtrFromNull(row.CurrentRevisionID),
		Status:            row.Status,
		Metadata:          metadata,
		CreatedBy:         uuidPtrFromNull(row.CreatedBy),
		CreatedAt:         timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:         timeFromTimestamptz(row.UpdatedAt),
		ArchivedAt:        timePtrFromTimestamptz(row.ArchivedAt),
		DeletedAt:         timePtrFromTimestamptz(row.DeletedAt),
	}, nil
}

func workspaceFileRevisionRecordFromQuery(row queries.DigitalEmployeeWorkspaceFileRevision) (WorkspaceFileRevisionRecord, error) {
	metadata, err := mapFromJSONB(row.Metadata, "metadata")
	if err != nil {
		return WorkspaceFileRevisionRecord{}, err
	}
	return WorkspaceFileRevisionRecord{
		ID:             row.ID,
		TenantID:       row.TenantID,
		FileID:         row.FileID,
		RevisionNumber: row.RevisionNumber,
		ContentText:    textValue(row.ContentText),
		ContentHash:    row.ContentHash,
		SizeBytes:      row.SizeBytes,
		StorageBackend: row.StorageBackend,
		ObjectKey:      stringPtrFromText(row.ObjectKey),
		CreatedBy:      uuidPtrFromNull(row.CreatedBy),
		CreatedAt:      timeFromTimestamptz(row.CreatedAt),
		ChangeNote:     stringPtrFromText(row.ChangeNote),
		Metadata:       metadata,
	}, nil
}

func workspaceFileForSyncRecordFromQuery(row queries.ListCurrentDigitalEmployeeWorkspaceFilesForSyncRow) (WorkspaceFileForSyncRecord, error) {
	fileMetadata, err := mapFromJSONB(row.FileMetadata, "file_metadata")
	if err != nil {
		return WorkspaceFileForSyncRecord{}, err
	}
	revisionMetadata, err := mapFromJSONB(row.RevisionMetadata, "revision_metadata")
	if err != nil {
		return WorkspaceFileForSyncRecord{}, err
	}
	return WorkspaceFileForSyncRecord{
		FileID:            row.FileID,
		TenantID:          row.TenantID,
		TeamID:            uuidPtrFromNullUUID(row.TeamID),
		DigitalEmployeeID: row.DigitalEmployeeID,
		Path:              row.Path,
		FileRole:          row.FileRole,
		MimeType:          row.MimeType,
		SyncPolicy:        row.SyncPolicy,
		FileMetadata:      fileMetadata,
		RevisionID:        row.RevisionID,
		RevisionNumber:    row.RevisionNumber,
		ContentText:       textValue(row.ContentText),
		ContentHash:       row.ContentHash,
		SizeBytes:         row.SizeBytes,
		StorageBackend:    row.StorageBackend,
		ObjectKey:         stringPtrFromText(row.ObjectKey),
		RevisionMetadata:  revisionMetadata,
	}, nil
}

func workspaceFileFromCurrentRow(row queries.ListCurrentDigitalEmployeeWorkspaceFilesRow) (WorkspaceFile, error) {
	if _, err := mapFromJSONB(row.FileMetadata, "file_metadata"); err != nil {
		return WorkspaceFile{}, err
	}
	if _, err := mapFromJSONB(row.RevisionMetadata, "revision_metadata"); err != nil {
		return WorkspaceFile{}, err
	}
	return WorkspaceFile{
		ID:                row.FileID,
		TenantID:          row.TenantID,
		TeamID:            uuidPtrFromNullUUID(row.TeamID),
		DigitalEmployeeID: row.DigitalEmployeeID,
		Path:              row.Path,
		FileRole:          row.FileRole,
		MimeType:          row.MimeType,
		SyncPolicy:        row.SyncPolicy,
		Status:            row.Status,
		CurrentRevisionID: row.RevisionID,
		RevisionNumber:    row.RevisionNumber,
		Content:           textValue(row.ContentText),
		ContentHash:       row.ContentHash,
		SizeBytes:         row.SizeBytes,
		StorageBackend:    row.StorageBackend,
		ObjectKey:         stringPtrFromText(row.ObjectKey),
		CreatedBy:         uuidPtrFromNull(row.CreatedBy),
		ChangeNote:        stringPtrFromText(row.ChangeNote),
		CreatedAt:         timeFromTimestamptz(row.FileCreatedAt),
		UpdatedAt:         timeFromTimestamptz(row.FileUpdatedAt),
	}, nil
}

type environmentVariableScanner interface {
	Scan(dest ...any) error
}

func scanEnvironmentVariableRecord(scanner environmentVariableScanner) (EnvironmentVariableRecord, error) {
	var record EnvironmentVariableRecord
	var status string
	var createdBy uuid.NullUUID
	var updatedBy uuid.NullUUID
	var teamID uuid.NullUUID
	if err := scanner.Scan(
		&record.ID,
		&record.TenantID,
		&teamID,
		&record.DigitalEmployeeID,
		&record.Name,
		&record.EncryptedValue,
		&record.EncryptionKeyID,
		&record.ValueFingerprint,
		&record.Sensitive,
		&status,
		&createdBy,
		&updatedBy,
		&record.CreatedAt,
		&record.UpdatedAt,
	); err != nil {
		return EnvironmentVariableRecord{}, mapNoRows(err)
	}
	record.Status = EnvironmentVariableStatus(status)
	record.TeamID = uuidPtrFromNullUUID(teamID)
	record.CreatedBy = uuidPtrFromNull(createdBy)
	record.UpdatedBy = uuidPtrFromNull(updatedBy)
	return record, nil
}

func digitalEmployeeRecordFromQuery(employee queries.DigitalEmployee) (DigitalEmployeeRecord, error) {
	permissionPolicy, err := mapFromJSONB(employee.PermissionPolicy, "permission_policy")
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}
	contextPolicy, err := mapFromJSONB(employee.ContextPolicy, "context_policy")
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}
	approvalPolicy, err := mapFromJSONB(employee.ApprovalPolicy, "approval_policy")
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}
	metadata, err := mapFromJSONB(employee.Metadata, "metadata")
	if err != nil {
		return DigitalEmployeeRecord{}, err
	}
	return DigitalEmployeeRecord{
		ID:               employee.ID,
		TenantID:         employee.TenantID,
		TeamID:           uuidPtrFromNull(employee.TeamID),
		OwnerUserID:      employee.OwnerUserID,
		EmployeeType:     employee.EmployeeType,
		ProviderType:     employee.ProviderType,
		Name:             employee.Name,
		Role:             employee.Role,
		Description:      stringPtrFromText(employee.Description),
		Status:           DigitalEmployeeStatus(employee.Status),
		PermissionPolicy: permissionPolicy,
		ContextPolicy:    contextPolicy,
		ApprovalPolicy:   approvalPolicy,
		RiskLevel:        employee.RiskLevel,
		Metadata:         metadata,
		DisabledAt:       timePtrFromTimestamptz(employee.DisabledAt),
		ArchivedAt:       timePtrFromTimestamptz(employee.ArchivedAt),
		DeletedAt:        timePtrFromTimestamptz(employee.DeletedAt),
		CreatedAt:        timeFromTimestamptz(employee.CreatedAt),
		UpdatedAt:        timeFromTimestamptz(employee.UpdatedAt),
	}, nil
}

type digitalEmployeeConfigRevisionRecordRow interface {
	digitalEmployeeConfigRevisionQueryRow
	GetStatus() string
	GetApprovedBy() uuid.NullUUID
	GetApprovedAt() pgtype.Timestamptz
	GetArchivedAt() pgtype.Timestamptz
	GetCreatedAt() pgtype.Timestamptz
	GetUpdatedAt() pgtype.Timestamptz
}

type digitalEmployeeConfigRevisionRecordAdapter struct {
	digitalEmployeeConfigRevisionQueryAdapter
	status     string
	approvedBy uuid.NullUUID
	approvedAt pgtype.Timestamptz
	archivedAt pgtype.Timestamptz
	createdAt  pgtype.Timestamptz
	updatedAt  pgtype.Timestamptz
}

func (r digitalEmployeeConfigRevisionRecordAdapter) GetStatus() string { return r.status }
func (r digitalEmployeeConfigRevisionRecordAdapter) GetApprovedBy() uuid.NullUUID {
	return r.approvedBy
}
func (r digitalEmployeeConfigRevisionRecordAdapter) GetApprovedAt() pgtype.Timestamptz {
	return r.approvedAt
}
func (r digitalEmployeeConfigRevisionRecordAdapter) GetArchivedAt() pgtype.Timestamptz {
	return r.archivedAt
}
func (r digitalEmployeeConfigRevisionRecordAdapter) GetCreatedAt() pgtype.Timestamptz {
	return r.createdAt
}
func (r digitalEmployeeConfigRevisionRecordAdapter) GetUpdatedAt() pgtype.Timestamptz {
	return r.updatedAt
}

func configRevisionRecordFromQuery(revision digitalEmployeeConfigRevisionRecordRow) (DigitalEmployeeConfigRevisionRecord, error) {
	input, err := employeeConfigInputFromQuery(revision)
	if err != nil {
		return DigitalEmployeeConfigRevisionRecord{}, err
	}
	return DigitalEmployeeConfigRevisionRecord{
		ID:                    input.ID,
		TenantID:              input.TenantID,
		DigitalEmployeeID:     input.DigitalEmployeeID,
		RevisionNumber:        input.RevisionNumber,
		PersonaMemoryMarkdown: input.PersonaMemoryMarkdown,
		CapabilityBindings:    cloneMap(input.CapabilityBindings),
		BudgetPolicy:          cloneMap(input.BudgetPolicy),
		Status:                ConfigRevisionStatus(revision.GetStatus()),
		ApprovedBy:            uuidPtrFromNull(revision.GetApprovedBy()),
		ApprovedAt:            timePtrFromTimestamptz(revision.GetApprovedAt()),
		ArchivedAt:            timePtrFromTimestamptz(revision.GetArchivedAt()),
		CreatedAt:             timeFromTimestamptz(revision.GetCreatedAt()),
		UpdatedAt:             timeFromTimestamptz(revision.GetUpdatedAt()),
	}, nil
}

type digitalEmployeeConfigRevisionQueryRow interface {
	GetID() uuid.UUID
	GetTenantID() uuid.UUID
	GetDigitalEmployeeID() uuid.UUID
	GetRevisionNumber() int32
	GetPersonaMemoryMarkdown() string
	GetCapabilityBindings() []byte
	GetBudgetPolicy() []byte
}

type digitalEmployeeConfigRevisionQueryAdapter struct {
	id                    uuid.UUID
	tenantID              uuid.UUID
	digitalEmployeeID     uuid.UUID
	revisionNumber        int32
	personaMemoryMarkdown string
	capabilityBindings    []byte
	budgetPolicy          []byte
}

func (r digitalEmployeeConfigRevisionQueryAdapter) GetID() uuid.UUID       { return r.id }
func (r digitalEmployeeConfigRevisionQueryAdapter) GetTenantID() uuid.UUID { return r.tenantID }
func (r digitalEmployeeConfigRevisionQueryAdapter) GetDigitalEmployeeID() uuid.UUID {
	return r.digitalEmployeeID
}
func (r digitalEmployeeConfigRevisionQueryAdapter) GetRevisionNumber() int32 { return r.revisionNumber }
func (r digitalEmployeeConfigRevisionQueryAdapter) GetPersonaMemoryMarkdown() string {
	return r.personaMemoryMarkdown
}
func (r digitalEmployeeConfigRevisionQueryAdapter) GetCapabilityBindings() []byte {
	return r.capabilityBindings
}
func (r digitalEmployeeConfigRevisionQueryAdapter) GetBudgetPolicy() []byte { return r.budgetPolicy }

func employeeConfigInputFromQuery(revision digitalEmployeeConfigRevisionQueryRow) (EmployeeConfigInput, error) {
	capabilityBindings, err := mapFromJSONB(revision.GetCapabilityBindings(), "capability_bindings")
	if err != nil {
		return EmployeeConfigInput{}, err
	}
	budgetPolicy, err := mapFromJSONB(revision.GetBudgetPolicy(), "budget_policy")
	if err != nil {
		return EmployeeConfigInput{}, err
	}
	return EmployeeConfigInput{
		ID:                    revision.GetID(),
		TenantID:              revision.GetTenantID(),
		DigitalEmployeeID:     revision.GetDigitalEmployeeID(),
		RevisionNumber:        revision.GetRevisionNumber(),
		PersonaMemoryMarkdown: revision.GetPersonaMemoryMarkdown(),
		CapabilityBindings:    capabilityBindings,
		BudgetPolicy:          budgetPolicy,
	}, nil
}

func executionInstanceRecordFromQuery(instance queries.DigitalEmployeeExecutionInstance) (DigitalEmployeeExecutionInstanceRecord, error) {
	workspacePolicy, err := mapFromJSONB(instance.WorkspacePolicy, "workspace_policy")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	sessionPolicy, err := mapFromJSONB(instance.SessionPolicy, "session_policy")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	runtimeSelector, err := mapFromJSONB(instance.RuntimeSelector, "runtime_selector")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	capacityRequirements, err := mapFromJSONB(instance.CapacityRequirements, "capacity_requirements")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	fallbackPolicy, err := mapFromJSONB(instance.FallbackPolicy, "fallback_policy")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	metadata, err := mapFromJSONB(instance.Metadata, "metadata")
	if err != nil {
		return DigitalEmployeeExecutionInstanceRecord{}, err
	}
	return DigitalEmployeeExecutionInstanceRecord{
		ID:                   instance.ID,
		TenantID:             instance.TenantID,
		DigitalEmployeeID:    instance.DigitalEmployeeID,
		RuntimeNodeID:        instance.RuntimeNodeID,
		ProviderType:         instance.ProviderType,
		AgentHomeDir:         instance.AgentHomeDir,
		WorkspacePolicy:      workspacePolicy,
		SessionPolicy:        sessionPolicy,
		RuntimeSelector:      runtimeSelector,
		CapacityRequirements: capacityRequirements,
		FallbackPolicy:       fallbackPolicy,
		Status:               ExecutionInstanceStatus(instance.Status),
		ReadyAt:              timePtrFromTimestamptz(instance.ReadyAt),
		DisabledAt:           timePtrFromTimestamptz(instance.DisabledAt),
		ErrorAt:              timePtrFromTimestamptz(instance.ErrorAt),
		ErrorMessage:         stringPtrFromText(instance.ErrorMessage),
		DeletedAt:            timePtrFromTimestamptz(instance.DeletedAt),
		Metadata:             metadata,
		CreatedAt:            timeFromTimestamptz(instance.CreatedAt),
		UpdatedAt:            timeFromTimestamptz(instance.UpdatedAt),
	}, nil
}

func mapNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func nullUUIDFromPtr(value *uuid.UUID) uuid.NullUUID {
	if value == nil || *value == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *value, Valid: true}
}

func nullUUIDFromUUID(value uuid.UUID) uuid.NullUUID {
	if value == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: value, Valid: true}
}

func uuidPtrFromNull(value uuid.NullUUID) *uuid.UUID {
	if !value.Valid || value.UUID == uuid.Nil {
		return nil
	}
	copied := value.UUID
	return &copied
}

func uuidPtr(value uuid.UUID) *uuid.UUID {
	if value == uuid.Nil {
		return nil
	}
	copied := value
	return &copied
}

func appendUUIDs(dst []uuid.UUID, src []uuid.UUID) []uuid.UUID {
	return append(dst, src...)
}

func digitalEmployeeDeleteAuditDetails(params DigitalEmployeeDeleteAuditEventParams) map[string]any {
	teamID := ""
	if params.Employee.TeamID != nil {
		teamID = params.Employee.TeamID.String()
	}
	executionInstanceID := ""
	if params.CascadeResult.ExecutionInstanceID != nil {
		executionInstanceID = params.CascadeResult.ExecutionInstanceID.String()
	}
	runtimeNodeID := ""
	if params.CascadeResult.RuntimeNodeID != nil {
		runtimeNodeID = params.CascadeResult.RuntimeNodeID.String()
	}
	return map[string]any{
		"digital_employee_id":   params.Employee.ID.String(),
		"name":                  params.Employee.Name,
		"team_id":               teamID,
		"provider_type":         coalesceString(params.CascadeResult.ProviderType, params.Employee.ProviderType),
		"runtime_node_id":       runtimeNodeID,
		"execution_instance_id": executionInstanceID,
		"agent_home_dir":        params.CascadeResult.AgentHomeDir,
		"cascade_counts": map[string]any{
			"execution_instances":   params.CascadeResult.ExecutionInstances,
			"environment_variables": params.CascadeResult.EnvironmentVariables,
			"mcp_bindings":          params.CascadeResult.MCPBindings,
			"mcp_bindings_v2":       params.CascadeResult.MCPBindingsV2,
			"skill_bindings":        params.CascadeResult.SkillBindings,
			"config_revisions":      params.CascadeResult.ConfigRevisions,
			"workspace_files":       params.CascadeResult.WorkspaceFiles,
			"project_affinities":    params.CascadeResult.ProjectAffinities,
		},
		"cleanup_candidates": map[string]any{
			"agent_home_dir":     params.CascadeResult.AgentHomeDir,
			"workspace_file_ids": uuidStrings(params.CascadeResult.WorkspaceFileIDs),
			"mcp_binding_ids":    uuidStrings(append(params.CascadeResult.MCPBindingIDs, params.CascadeResult.MCPBindingV2IDs...)),
			"skill_binding_ids":  uuidStrings(params.CascadeResult.SkillBindingIDs),
		},
		"deleted_at": params.DeletedAt.UTC().Format(time.RFC3339Nano),
	}
}

func coalesceString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func uuidStrings(values []uuid.UUID) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != uuid.Nil {
			out = append(out, value.String())
		}
	}
	return out
}

func textFromPtr(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func textFromStatus(status DigitalEmployeeStatus) pgtype.Text {
	if status == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(status), Valid: true}
}

func timestamptzFromPtr(value *time.Time) pgtype.Timestamptz {
	if value == nil || value.IsZero() {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: value.UTC(), Valid: true}
}

func stringPtrFromText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func timePtrFromTimestamptz(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time.UTC()
	return &t
}

func timeFromTimestamptz(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

func jsonbFromMap(value map[string]any, field string) ([]byte, error) {
	encoded, err := json.Marshal(cloneMap(value))
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", field, err)
	}
	return encoded, nil
}

func mapFromJSONB(raw []byte, field string) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("decode %s: %w", field, err)
	}
	if decoded == nil {
		return map[string]any{}, nil
	}
	return decoded, nil
}

func mapFromJSONValue(value any, field string) (map[string]any, error) {
	switch typed := value.(type) {
	case nil:
		return map[string]any{}, nil
	case []byte:
		return mapFromJSONB(typed, field)
	case string:
		return mapFromJSONB([]byte(typed), field)
	case map[string]any:
		return cloneMap(typed), nil
	default:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return nil, fmt.Errorf("encode %s: %w", field, err)
		}
		return mapFromJSONB(encoded, field)
	}
}

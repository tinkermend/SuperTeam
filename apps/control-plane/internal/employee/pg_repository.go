package employee

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgRepository struct {
	q  *queries.Queries
	db employeeTransactionBeginner
}

type employeeTransactionBeginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

func NewPgRepository(q *queries.Queries, db ...employeeTransactionBeginner) Repository {
	var beginner employeeTransactionBeginner
	if len(db) > 0 {
		beginner = db[0]
	}
	return &PgRepository{q: q, db: beginner}
}

func (r *PgRepository) WithTransaction(ctx context.Context, fn func(Repository) error) error {
	if r.db == nil {
		return fn(r)
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin employee transaction: %w", err)
	}
	txRepo := &PgRepository{q: r.q.WithTx(tx)}
	if err := fn(txRepo); err != nil {
		_ = tx.Rollback(ctx)
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
		TenantID: params.TenantID,
		TeamID:   nullUUIDFromPtr(params.TeamID),
		Status:   textFromStatus(params.Status),
		Offset:   params.Offset,
		Limit:    params.Limit,
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

func (r *PgRepository) EnsureTeamExists(ctx context.Context, tenantID, teamID uuid.UUID) error {
	if _, err := r.q.GetTenantTeam(ctx, queries.GetTenantTeamParams{
		ID:       teamID,
		TenantID: tenantID,
	}); err != nil {
		return mapNoRows(err)
	}
	return nil
}

func (r *PgRepository) GetCurrentTeamConfigRevision(ctx context.Context, tenantID, teamID uuid.UUID) (TeamConfigInput, error) {
	revision, err := r.q.GetCurrentTenantTeamConfigRevision(ctx, queries.GetCurrentTenantTeamConfigRevisionParams{
		TenantID: tenantID,
		TeamID:   teamID,
	})
	if err != nil {
		return TeamConfigInput{}, mapNoRows(err)
	}
	return teamConfigInputFromQuery(revision)
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
	payload, err := jsonbFromMap(redactRuntimeEventPayload(req.Payload), "payload")
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

func (r *PgRepository) CreateDigitalEmployeeConfigRevision(ctx context.Context, params CreateConfigRevisionParams) (DigitalEmployeeConfigRevisionRecord, error) {
	roleProfile, err := jsonbFromMap(params.RoleProfile, "role_profile")
	if err != nil {
		return DigitalEmployeeConfigRevisionRecord{}, err
	}
	constitutionAddendum, err := jsonbFromMap(params.ConstitutionAddendum, "constitution_addendum")
	if err != nil {
		return DigitalEmployeeConfigRevisionRecord{}, err
	}
	capabilitySelection, err := jsonbFromMap(params.CapabilitySelection, "capability_selection")
	if err != nil {
		return DigitalEmployeeConfigRevisionRecord{}, err
	}
	contextPolicyOverride, err := jsonbFromMap(params.ContextPolicyOverride, "context_policy_override")
	if err != nil {
		return DigitalEmployeeConfigRevisionRecord{}, err
	}
	approvalPolicyOverride, err := jsonbFromMap(params.ApprovalPolicyOverride, "approval_policy_override")
	if err != nil {
		return DigitalEmployeeConfigRevisionRecord{}, err
	}
	outputContractAddendum, err := jsonbFromMap(params.OutputContractAddendum, "output_contract_addendum")
	if err != nil {
		return DigitalEmployeeConfigRevisionRecord{}, err
	}
	revision, err := r.q.CreateDigitalEmployeeConfigRevision(ctx, queries.CreateDigitalEmployeeConfigRevisionParams{
		TenantID:               params.TenantID,
		DigitalEmployeeID:      params.DigitalEmployeeID,
		RevisionNumber:         params.RevisionNumber,
		RoleProfile:            roleProfile,
		ConstitutionAddendum:   constitutionAddendum,
		CapabilitySelection:    capabilitySelection,
		ContextPolicyOverride:  contextPolicyOverride,
		ApprovalPolicyOverride: approvalPolicyOverride,
		OutputContractAddendum: outputContractAddendum,
		Status:                 string(params.Status),
		ApprovedBy:             nullUUIDFromPtr(params.ApprovedBy),
		ApprovedAt:             timestamptzFromPtr(params.ApprovedAt),
	})
	if err != nil {
		return DigitalEmployeeConfigRevisionRecord{}, err
	}
	return configRevisionRecordFromQuery(revision)
}

func (r *PgRepository) GetTeamConfigRevision(ctx context.Context, tenantID, teamConfigRevisionID uuid.UUID) (TeamConfigInput, error) {
	revision, err := r.q.GetTenantTeamConfigRevision(ctx, queries.GetTenantTeamConfigRevisionParams{
		ID:       teamConfigRevisionID,
		TenantID: tenantID,
	})
	if err != nil {
		return TeamConfigInput{}, mapNoRows(err)
	}
	return teamConfigInputFromQuery(revision)
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
	return employeeConfigInputFromQuery(revision)
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

func (r *PgRepository) GetCurrentDigitalEmployeeEffectiveConfig(ctx context.Context, tenantID, digitalEmployeeID uuid.UUID) (DigitalEmployeeEffectiveConfigRecord, error) {
	effectiveConfig, err := r.q.GetCurrentDigitalEmployeeEffectiveConfig(ctx, queries.GetCurrentDigitalEmployeeEffectiveConfigParams{
		TenantID:          tenantID,
		DigitalEmployeeID: digitalEmployeeID,
	})
	if err != nil {
		return DigitalEmployeeEffectiveConfigRecord{}, mapNoRows(err)
	}
	return effectiveConfigRecordFromQuery(effectiveConfig)
}

func (r *PgRepository) CreateDigitalEmployeeEffectiveConfig(ctx context.Context, params CreateEffectiveConfigParams) (DigitalEmployeeEffectiveConfigRecord, error) {
	effectiveConfigSnapshot, err := jsonbFromMap(params.EffectiveConfig, "effective_config_snapshot")
	if err != nil {
		return DigitalEmployeeEffectiveConfigRecord{}, err
	}
	validationResult, err := jsonbFromMap(params.ValidationResult, "validation_result")
	if err != nil {
		return DigitalEmployeeEffectiveConfigRecord{}, err
	}
	effectiveConfig, err := r.q.CreateDigitalEmployeeEffectiveConfig(ctx, queries.CreateDigitalEmployeeEffectiveConfigParams{
		TenantID:                   params.TenantID,
		DigitalEmployeeID:          params.DigitalEmployeeID,
		TenantTeamConfigRevisionID: params.TeamConfigRevisionID,
		EmployeeConfigRevisionID:   params.EmployeeConfigRevisionID,
		EffectiveConfigSnapshot:    effectiveConfigSnapshot,
		ValidationResult:           validationResult,
		Status:                     string(params.Status),
		ApprovedBy:                 nullUUIDFromPtr(params.ApprovedBy),
		ApprovedAt:                 timestamptzFromPtr(params.ApprovedAt),
	})
	if err != nil {
		return DigitalEmployeeEffectiveConfigRecord{}, err
	}
	return effectiveConfigRecordFromQuery(effectiveConfig)
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
	items := make([]DigitalEmployeeOverviewItem, 0, len(itemRows))
	for _, row := range itemRows {
		items = append(items, overviewItemFromQuery(row))
	}

	filterRows, err := r.q.ListDigitalEmployeeOverviewFilterOptions(ctx, req.TenantID)
	if err != nil {
		return nil, err
	}

	return &DigitalEmployeeOverview{
		Summary: DigitalEmployeeOverviewSummary{
			TotalCount:          summary.TotalCount,
			RunnableCount:       summary.RunnableCount,
			RunningCount:        summary.RunningCount,
			WaitingRuntimeCount: summary.WaitingRuntimeCount,
			ErrorCount:          summary.ErrorCount,
			HighRiskCount:       summary.HighRiskCount,
		},
		Items:   items,
		Filters: overviewFiltersFromQuery(filterRows),
		Pagination: OverviewPagination{
			Limit:      req.Limit,
			Offset:     req.Offset,
			TotalCount: summary.TotalCount,
		},
	}, nil
}

func overviewItemFromQuery(row queries.ListDigitalEmployeeOverviewItemsRow) DigitalEmployeeOverviewItem {
	var latestRun *DigitalEmployeeLatestRunSummary
	if row.LatestRunID.Valid && row.LatestRunID.UUID != uuid.Nil {
		latestRun = &DigitalEmployeeLatestRunSummary{
			RunID:        row.LatestRunID.UUID,
			TaskID:       row.LatestRunTaskID.UUID,
			Status:       overviewRunStatus(row.LatestRunStatus),
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
			EmployeeTypeLabel: overviewEmployeeTypeLabel(row.EmployeeType),
			Name:              row.Name,
			Role:              row.Role,
			Description:       stringPtrFromPgText(row.Description),
			Status:            DigitalEmployeeStatus(row.Status),
			RiskLevel:         row.RiskLevel,
		},
		ExecutionSummary: DigitalEmployeeExecutionSummary{
			ExecutionInstanceID:   uuidPtrFromNullUUID(row.ExecutionInstanceID),
			Status:                overviewExecutionStatus(row.ExecutionStatus),
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
			UsageTokens30d: budgetUsage,
			RunCount30d:    row.BudgetRunCount30d,
			Currency:       "USD",
			Source:         overviewBudgetSource(row.BudgetRunCount30d, budgetUsageValue),
		},
	}
}

func overviewFiltersFromQuery(rows []queries.ListDigitalEmployeeOverviewFilterOptionsRow) DigitalEmployeeOverviewFilters {
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
		if row.FilterType == "employee_type" {
			label = overviewEmployeeTypeLabel(value)
		}
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

func overviewEmployeeTypeLabel(value string) string {
	definition, ok := EmployeeTypeDefinitionByType(value)
	if !ok {
		return value
	}
	return definition.Label
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

func overviewBudgetSource(runCount, usageTokens int32) string {
	if runCount <= 0 || usageTokens <= 0 {
		return "unavailable"
	}
	return "run_usage_projection"
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
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parsed := int32FromJSONString(value)
	return &parsed
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

func configRevisionRecordFromQuery(revision queries.DigitalEmployeeConfigRevision) (DigitalEmployeeConfigRevisionRecord, error) {
	input, err := employeeConfigInputFromQuery(revision)
	if err != nil {
		return DigitalEmployeeConfigRevisionRecord{}, err
	}
	return DigitalEmployeeConfigRevisionRecord{
		ID:                     input.ID,
		TenantID:               input.TenantID,
		DigitalEmployeeID:      input.DigitalEmployeeID,
		RevisionNumber:         input.RevisionNumber,
		RoleProfile:            cloneMap(input.RoleProfile),
		ConstitutionAddendum:   cloneMap(input.ConstitutionAddendum),
		CapabilitySelection:    cloneMap(input.CapabilitySelection),
		ContextPolicyOverride:  cloneMap(input.ContextPolicyOverride),
		ApprovalPolicyOverride: cloneMap(input.ApprovalPolicyOverride),
		OutputContractAddendum: cloneMap(input.OutputContractAddendum),
		Status:                 ConfigRevisionStatus(revision.Status),
		ApprovedBy:             uuidPtrFromNull(revision.ApprovedBy),
		ApprovedAt:             timePtrFromTimestamptz(revision.ApprovedAt),
		ArchivedAt:             timePtrFromTimestamptz(revision.ArchivedAt),
		CreatedAt:              timeFromTimestamptz(revision.CreatedAt),
		UpdatedAt:              timeFromTimestamptz(revision.UpdatedAt),
	}, nil
}

func teamConfigInputFromQuery(revision queries.TenantTeamConfigRevision) (TeamConfigInput, error) {
	constitution, err := mapFromJSONB(revision.Constitution, "constitution")
	if err != nil {
		return TeamConfigInput{}, err
	}
	capabilityPolicy, err := mapFromJSONB(revision.CapabilityPolicy, "capability_policy")
	if err != nil {
		return TeamConfigInput{}, err
	}
	contextPolicy, err := mapFromJSONB(revision.ContextPolicy, "context_policy")
	if err != nil {
		return TeamConfigInput{}, err
	}
	approvalPolicy, err := mapFromJSONB(revision.ApprovalPolicy, "approval_policy")
	if err != nil {
		return TeamConfigInput{}, err
	}
	artifactContract, err := mapFromJSONB(revision.ArtifactContract, "artifact_contract")
	if err != nil {
		return TeamConfigInput{}, err
	}
	internalCollaborationPolicy, err := mapFromJSONB(revision.InternalCollaborationPolicy, "internal_collaboration_policy")
	if err != nil {
		return TeamConfigInput{}, err
	}
	runtimeScopePolicy, err := mapFromJSONB(revision.RuntimeScopePolicy, "runtime_scope_policy")
	if err != nil {
		return TeamConfigInput{}, err
	}
	return TeamConfigInput{
		ID:                          revision.ID,
		TenantID:                    revision.TenantID,
		TeamID:                      revision.TeamID,
		RevisionNumber:              revision.RevisionNumber,
		Status:                      TeamConfigRevisionStatus(revision.Status),
		Constitution:                constitution,
		CapabilityPolicy:            capabilityPolicy,
		ContextPolicy:               contextPolicy,
		ApprovalPolicy:              approvalPolicy,
		ArtifactContract:            artifactContract,
		InternalCollaborationPolicy: internalCollaborationPolicy,
		RuntimeScopePolicy:          runtimeScopePolicy,
	}, nil
}

func employeeConfigInputFromQuery(revision queries.DigitalEmployeeConfigRevision) (EmployeeConfigInput, error) {
	roleProfile, err := mapFromJSONB(revision.RoleProfile, "role_profile")
	if err != nil {
		return EmployeeConfigInput{}, err
	}
	constitutionAddendum, err := mapFromJSONB(revision.ConstitutionAddendum, "constitution_addendum")
	if err != nil {
		return EmployeeConfigInput{}, err
	}
	capabilitySelection, err := mapFromJSONB(revision.CapabilitySelection, "capability_selection")
	if err != nil {
		return EmployeeConfigInput{}, err
	}
	contextPolicyOverride, err := mapFromJSONB(revision.ContextPolicyOverride, "context_policy_override")
	if err != nil {
		return EmployeeConfigInput{}, err
	}
	approvalPolicyOverride, err := mapFromJSONB(revision.ApprovalPolicyOverride, "approval_policy_override")
	if err != nil {
		return EmployeeConfigInput{}, err
	}
	outputContractAddendum, err := mapFromJSONB(revision.OutputContractAddendum, "output_contract_addendum")
	if err != nil {
		return EmployeeConfigInput{}, err
	}
	return EmployeeConfigInput{
		ID:                     revision.ID,
		TenantID:               revision.TenantID,
		DigitalEmployeeID:      revision.DigitalEmployeeID,
		RevisionNumber:         revision.RevisionNumber,
		RoleProfile:            roleProfile,
		ConstitutionAddendum:   constitutionAddendum,
		CapabilitySelection:    capabilitySelection,
		ContextPolicyOverride:  contextPolicyOverride,
		ApprovalPolicyOverride: approvalPolicyOverride,
		OutputContractAddendum: outputContractAddendum,
	}, nil
}

func effectiveConfigRecordFromQuery(effectiveConfig queries.DigitalEmployeeEffectiveConfig) (DigitalEmployeeEffectiveConfigRecord, error) {
	effectiveConfigSnapshot, err := mapFromJSONB(effectiveConfig.EffectiveConfigSnapshot, "effective_config_snapshot")
	if err != nil {
		return DigitalEmployeeEffectiveConfigRecord{}, err
	}
	validationResult, err := mapFromJSONB(effectiveConfig.ValidationResult, "validation_result")
	if err != nil {
		return DigitalEmployeeEffectiveConfigRecord{}, err
	}
	return DigitalEmployeeEffectiveConfigRecord{
		ID:                       effectiveConfig.ID,
		TenantID:                 effectiveConfig.TenantID,
		DigitalEmployeeID:        effectiveConfig.DigitalEmployeeID,
		TeamConfigRevisionID:     effectiveConfig.TenantTeamConfigRevisionID,
		EmployeeConfigRevisionID: effectiveConfig.EmployeeConfigRevisionID,
		EffectiveConfig:          effectiveConfigSnapshot,
		ValidationResult:         validationResult,
		Status:                   EffectiveConfigStatus(effectiveConfig.Status),
		ApprovedBy:               uuidPtrFromNull(effectiveConfig.ApprovedBy),
		ApprovedAt:               timePtrFromTimestamptz(effectiveConfig.ApprovedAt),
		RevokedAt:                timePtrFromTimestamptz(effectiveConfig.RevokedAt),
		CreatedAt:                timeFromTimestamptz(effectiveConfig.CreatedAt),
		UpdatedAt:                timeFromTimestamptz(effectiveConfig.UpdatedAt),
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

func uuidPtrFromNull(value uuid.NullUUID) *uuid.UUID {
	if !value.Valid || value.UUID == uuid.Nil {
		return nil
	}
	copied := value.UUID
	return &copied
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

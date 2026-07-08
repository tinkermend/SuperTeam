package employee

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/superteam/control-plane/internal/storage/queries"
)

func (r *PgRepository) ListEmployeeTemplates(ctx context.Context, params ListEmployeeTemplatesParams) ([]EmployeeTemplateRecord, error) {
	rows, err := r.q.ListEmployeeTemplates(ctx, params.TenantID)
	if err != nil {
		return nil, fmt.Errorf("list employee templates: %w", err)
	}
	records := make([]EmployeeTemplateRecord, 0, len(rows))
	for _, row := range rows {
		record, err := employeeTemplateRecordFromRow(row)
		if err != nil {
			return nil, err
		}
		if params.ActiveOnly && record.Status != "active" {
			continue
		}
		records = append(records, record)
	}
	return records, nil
}

func (r *PgRepository) GetEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) (EmployeeTemplateRecord, error) {
	row, err := r.q.GetEmployeeTemplateByID(ctx, queries.GetEmployeeTemplateByIDParams{
		TenantID: tenantID,
		ID:       templateID,
	})
	if err != nil {
		return EmployeeTemplateRecord{}, mapNoRows(err)
	}
	return employeeTemplateRecordFromRow(row)
}

func (r *PgRepository) GetEmployeeTemplateByType(ctx context.Context, tenantID uuid.UUID, employeeType string) (EmployeeTemplateRecord, error) {
	row, err := r.q.GetEmployeeTemplateByType(ctx, queries.GetEmployeeTemplateByTypeParams{
		TenantID: tenantID,
		Type:     employeeType,
	})
	if err != nil {
		return EmployeeTemplateRecord{}, mapNoRows(err)
	}
	return employeeTemplateRecordFromRow(row)
}

func (r *PgRepository) CreateEmployeeTemplate(ctx context.Context, params CreateEmployeeTemplateParams) (EmployeeTemplateRecord, error) {
	recommendedSkills, err := jsonbFromStringSlice(params.RecommendedSkills)
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	recommendedMCPServers, err := jsonbFromStringSlice(params.RecommendedMCPServers)
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	recommendedProviderTypes, err := jsonbFromStringSlice(params.RecommendedProviderTypes)
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	defaultCapabilitySelection, err := jsonbFromMap(params.DefaultCapabilitySelection, "default_capability_selection")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	defaultContextPolicyOverride, err := jsonbFromMap(params.DefaultContextPolicyOverride, "default_context_policy_override")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	defaultApprovalPolicy, err := jsonbFromMap(params.DefaultApprovalPolicy, "default_approval_policy")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	metadata, err := jsonbFromMap(params.Metadata, "metadata")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}

	row, err := r.q.CreateEmployeeTemplate(ctx, queries.CreateEmployeeTemplateParams{
		TenantID:                     params.TenantID,
		Type:                         params.Type,
		Label:                        params.Label,
		Description:                  params.Description,
		DefaultRole:                  params.DefaultRole,
		RecommendedSkills:            recommendedSkills,
		RecommendedMcpServers:        recommendedMCPServers,
		RecommendedProviderTypes:     recommendedProviderTypes,
		DefaultCapabilitySelection:   defaultCapabilitySelection,
		DefaultContextPolicyOverride: defaultContextPolicyOverride,
		DefaultApprovalPolicy:        defaultApprovalPolicy,
		Metadata:                     metadata,
	})
	if err != nil {
		return EmployeeTemplateRecord{}, mapTemplateConstraintError(err)
	}
	return employeeTemplateRecordFromRow(row)
}

func (r *PgRepository) UpdateEmployeeTemplate(ctx context.Context, params UpdateEmployeeTemplateParams) (EmployeeTemplateRecord, error) {
	recommendedSkills, err := jsonbFromStringSlice(params.RecommendedSkills)
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	recommendedMCPServers, err := jsonbFromStringSlice(params.RecommendedMCPServers)
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	recommendedProviderTypes, err := jsonbFromStringSlice(params.RecommendedProviderTypes)
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	defaultCapabilitySelection, err := jsonbFromMap(params.DefaultCapabilitySelection, "default_capability_selection")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	defaultContextPolicyOverride, err := jsonbFromMap(params.DefaultContextPolicyOverride, "default_context_policy_override")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	defaultApprovalPolicy, err := jsonbFromMap(params.DefaultApprovalPolicy, "default_approval_policy")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	metadata, err := jsonbFromMap(params.Metadata, "metadata")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}

	row, err := r.q.UpdateEmployeeTemplate(ctx, queries.UpdateEmployeeTemplateParams{
		TenantID:                     params.TenantID,
		ID:                           params.ID,
		Label:                        params.Label,
		Description:                  params.Description,
		DefaultRole:                  params.DefaultRole,
		RecommendedSkills:            recommendedSkills,
		RecommendedMcpServers:        recommendedMCPServers,
		RecommendedProviderTypes:     recommendedProviderTypes,
		DefaultCapabilitySelection:   defaultCapabilitySelection,
		DefaultContextPolicyOverride: defaultContextPolicyOverride,
		DefaultApprovalPolicy:        defaultApprovalPolicy,
		Metadata:                     metadata,
	})
	if err != nil {
		return EmployeeTemplateRecord{}, mapNoRows(err)
	}
	return employeeTemplateRecordFromRow(row)
}

func (r *PgRepository) SetEmployeeTemplateStatus(ctx context.Context, tenantID, templateID uuid.UUID, status string) (EmployeeTemplateRecord, error) {
	row, err := r.q.SetEmployeeTemplateStatus(ctx, queries.SetEmployeeTemplateStatusParams{
		TenantID: tenantID,
		ID:       templateID,
		Status:   status,
	})
	if err != nil {
		return EmployeeTemplateRecord{}, mapNoRows(err)
	}
	return employeeTemplateRecordFromRow(row)
}

func (r *PgRepository) SoftDeleteEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) error {
	affected, err := r.q.SoftDeleteEmployeeTemplate(ctx, queries.SoftDeleteEmployeeTemplateParams{
		TenantID: tenantID,
		ID:       templateID,
	})
	if err != nil {
		return fmt.Errorf("soft delete employee template: %w", err)
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PgRepository) ListEmployeeTemplateLabels(ctx context.Context, tenantID uuid.UUID) (map[string]string, error) {
	rows, err := r.q.ListEmployeeTemplateLabels(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("list employee template labels: %w", err)
	}
	labels := make(map[string]string, len(rows))
	for _, row := range rows {
		labels[row.Type] = row.Label
	}
	return labels, nil
}

func employeeTemplateRecordFromRow(row queries.DigitalEmployeeTemplate) (EmployeeTemplateRecord, error) {
	recommendedSkills, err := stringSliceFromJSONB(row.RecommendedSkills, "recommended_skills")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	recommendedMCPServers, err := stringSliceFromJSONB(row.RecommendedMcpServers, "recommended_mcp_servers")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	recommendedProviderTypes, err := stringSliceFromJSONB(row.RecommendedProviderTypes, "recommended_provider_types")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	defaultCapabilitySelection, err := mapFromJSONB(row.DefaultCapabilitySelection, "default_capability_selection")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	defaultContextPolicyOverride, err := mapFromJSONB(row.DefaultContextPolicyOverride, "default_context_policy_override")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	defaultApprovalPolicy, err := mapFromJSONB(row.DefaultApprovalPolicy, "default_approval_policy")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	metadata, err := mapFromJSONB(row.Metadata, "metadata")
	if err != nil {
		return EmployeeTemplateRecord{}, err
	}
	return EmployeeTemplateRecord{
		ID:                           row.ID,
		TenantID:                     row.TenantID,
		Type:                         row.Type,
		Label:                        row.Label,
		Description:                  row.Description,
		DefaultRole:                  row.DefaultRole,
		RecommendedSkills:            recommendedSkills,
		RecommendedMCPServers:        recommendedMCPServers,
		RecommendedProviderTypes:     recommendedProviderTypes,
		DefaultCapabilitySelection:   defaultCapabilitySelection,
		DefaultContextPolicyOverride: defaultContextPolicyOverride,
		DefaultApprovalPolicy:        defaultApprovalPolicy,
		Metadata:                     metadata,
		Status:                       row.Status,
		IsSystem:                     row.IsSystem,
		CreatedAt:                    timeFromTimestamptz(row.CreatedAt),
		UpdatedAt:                    timeFromTimestamptz(row.UpdatedAt),
	}, nil
}

func jsonbFromStringSlice(values []string) ([]byte, error) {
	if values == nil {
		values = []string{}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("encode string slice: %w", err)
	}
	return encoded, nil
}

func stringSliceFromJSONB(raw []byte, field string) ([]string, error) {
	if len(raw) == 0 {
		return []string{}, nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("decode %s: %w", field, err)
	}
	if values == nil {
		values = []string{}
	}
	return values, nil
}

func mapTemplateConstraintError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: template type already exists for this tenant", ErrInvalidInput)
	}
	return err
}

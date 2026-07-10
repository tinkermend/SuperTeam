package employee

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var employeeTemplateTypePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,63}$`)

func (s *Service) ListEmployeeTemplates(ctx context.Context, tenantID uuid.UUID) ([]EmployeeTemplateRecord, error) {
	if tenantID == uuid.Nil {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	return s.repository.ListEmployeeTemplates(ctx, ListEmployeeTemplatesParams{TenantID: tenantID})
}

func (s *Service) GetEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) (EmployeeTemplateRecord, error) {
	if tenantID == uuid.Nil || templateID == uuid.Nil {
		return EmployeeTemplateRecord{}, fmt.Errorf("%w: tenant_id and template_id are required", ErrInvalidInput)
	}
	return s.repository.GetEmployeeTemplate(ctx, tenantID, templateID)
}

func (s *Service) CreateEmployeeTemplate(ctx context.Context, params CreateEmployeeTemplateParams) (EmployeeTemplateRecord, error) {
	if params.TenantID == uuid.Nil {
		return EmployeeTemplateRecord{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	normalizedType := strings.ToLower(strings.TrimSpace(params.Type))
	if !employeeTemplateTypePattern.MatchString(normalizedType) {
		return EmployeeTemplateRecord{}, fmt.Errorf("%w: type must match %s", ErrInvalidInput, employeeTemplateTypePattern.String())
	}
	if normalizedType == "custom_agent" {
		return EmployeeTemplateRecord{}, fmt.Errorf("%w: custom_agent is a reserved type", ErrInvalidInput)
	}
	label := strings.TrimSpace(params.Label)
	if label == "" {
		return EmployeeTemplateRecord{}, fmt.Errorf("%w: label is required", ErrInvalidInput)
	}
	if _, err := s.repository.GetEmployeeTemplateByType(ctx, params.TenantID, normalizedType); err == nil {
		return EmployeeTemplateRecord{}, fmt.Errorf("%w: template type %q already exists for this tenant", ErrInvalidInput, normalizedType)
	} else if !errorIsNotFound(err) {
		return EmployeeTemplateRecord{}, err
	}

	params.Type = normalizedType
	params.Label = label
	params.RecommendedSkills = nonNilStringSlice(params.RecommendedSkills)
	params.RecommendedMCPServers = nonNilStringSlice(params.RecommendedMCPServers)
	params.RecommendedProviderTypes = nonNilStringSlice(params.RecommendedProviderTypes)
	params.PersonaMemoryMarkdown = strings.TrimSpace(params.PersonaMemoryMarkdown)
	params.CapabilityBindings = nonNilMap(params.CapabilityBindings)
	params.BudgetPolicy = nonNilMap(params.BudgetPolicy)
	params.Metadata = nonNilMap(params.Metadata)

	return s.repository.CreateEmployeeTemplate(ctx, params)
}

func (s *Service) UpdateEmployeeTemplate(ctx context.Context, params UpdateEmployeeTemplateParams) (EmployeeTemplateRecord, error) {
	if params.TenantID == uuid.Nil || params.ID == uuid.Nil {
		return EmployeeTemplateRecord{}, fmt.Errorf("%w: tenant_id and id are required", ErrInvalidInput)
	}
	label := strings.TrimSpace(params.Label)
	if label == "" {
		return EmployeeTemplateRecord{}, fmt.Errorf("%w: label is required", ErrInvalidInput)
	}
	params.Label = label
	params.RecommendedSkills = nonNilStringSlice(params.RecommendedSkills)
	params.RecommendedMCPServers = nonNilStringSlice(params.RecommendedMCPServers)
	params.RecommendedProviderTypes = nonNilStringSlice(params.RecommendedProviderTypes)
	params.PersonaMemoryMarkdown = strings.TrimSpace(params.PersonaMemoryMarkdown)
	params.CapabilityBindings = nonNilMap(params.CapabilityBindings)
	params.BudgetPolicy = nonNilMap(params.BudgetPolicy)
	params.Metadata = nonNilMap(params.Metadata)

	return s.repository.UpdateEmployeeTemplate(ctx, params)
}

func (s *Service) SetEmployeeTemplateStatus(ctx context.Context, tenantID, templateID uuid.UUID, status string) (EmployeeTemplateRecord, error) {
	normalizedStatus := strings.ToLower(strings.TrimSpace(status))
	if normalizedStatus != "active" && normalizedStatus != "disabled" {
		return EmployeeTemplateRecord{}, fmt.Errorf("%w: status must be active or disabled", ErrInvalidInput)
	}
	return s.repository.SetEmployeeTemplateStatus(ctx, tenantID, templateID, normalizedStatus)
}

func (s *Service) DeleteEmployeeTemplate(ctx context.Context, tenantID, templateID uuid.UUID) error {
	if tenantID == uuid.Nil || templateID == uuid.Nil {
		return fmt.Errorf("%w: tenant_id and template_id are required", ErrInvalidInput)
	}
	return s.repository.SoftDeleteEmployeeTemplate(ctx, tenantID, templateID)
}

func errorIsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

func nonNilStringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func nonNilMap(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	return values
}

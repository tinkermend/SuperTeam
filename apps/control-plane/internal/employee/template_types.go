package employee

import (
	"time"

	"github.com/google/uuid"
)

type EmployeeTemplateRecord struct {
	ID                           uuid.UUID
	TenantID                     uuid.UUID
	Type                         string
	Label                        string
	Description                  string
	DefaultRole                  string
	RecommendedSkills            []string
	RecommendedMCPServers        []string
	RecommendedProviderTypes     []string
	DefaultCapabilitySelection   map[string]any
	DefaultContextPolicyOverride map[string]any
	DefaultApprovalPolicy        map[string]any
	Metadata                     map[string]any
	Status                       string
	IsSystem                     bool
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
}

// ToDefinition projects the persisted template into the EmployeeTypeDefinition
// shape consumed by the create-employee wizard and creation-time defaults.
func (r EmployeeTemplateRecord) ToDefinition() EmployeeTypeDefinition {
	return EmployeeTypeDefinition{
		Type:                         r.Type,
		Label:                        r.Label,
		Description:                  r.Description,
		DefaultRole:                  r.DefaultRole,
		RecommendedSkills:            cloneStringSlice(r.RecommendedSkills),
		RecommendedMCPServers:        cloneStringSlice(r.RecommendedMCPServers),
		RecommendedProviderTypes:     cloneStringSlice(r.RecommendedProviderTypes),
		DefaultCapabilitySelection:   cloneEmployeeTypeMap(r.DefaultCapabilitySelection),
		DefaultContextPolicyOverride: cloneEmployeeTypeMap(r.DefaultContextPolicyOverride),
		DefaultApprovalPolicy:        cloneEmployeeTypeMap(r.DefaultApprovalPolicy),
		Metadata:                     cloneEmployeeTypeMap(r.Metadata),
	}
}

type ListEmployeeTemplatesParams struct {
	TenantID   uuid.UUID
	ActiveOnly bool
}

type CreateEmployeeTemplateParams struct {
	TenantID                     uuid.UUID
	Type                         string
	Label                        string
	Description                  string
	DefaultRole                  string
	RecommendedSkills            []string
	RecommendedMCPServers        []string
	RecommendedProviderTypes     []string
	DefaultCapabilitySelection   map[string]any
	DefaultContextPolicyOverride map[string]any
	DefaultApprovalPolicy        map[string]any
	Metadata                     map[string]any
}

type UpdateEmployeeTemplateParams struct {
	TenantID                     uuid.UUID
	ID                           uuid.UUID
	Label                        string
	Description                  string
	DefaultRole                  string
	RecommendedSkills            []string
	RecommendedMCPServers        []string
	RecommendedProviderTypes     []string
	DefaultCapabilitySelection   map[string]any
	DefaultContextPolicyOverride map[string]any
	DefaultApprovalPolicy        map[string]any
	Metadata                     map[string]any
}

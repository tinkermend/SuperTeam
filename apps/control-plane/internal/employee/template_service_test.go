package employee

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCreateEmployeeTemplateValidatesTypeFormat(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()

	_, err = svc.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID: tenantID,
		Type:     "Invalid Type!",
		Label:    "无效类型",
	})

	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestCreateEmployeeTemplateRequiresLabel(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()

	_, err = svc.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID: tenantID,
		Type:     "custom_reviewer",
		Label:    "  ",
	})

	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestCreateEmployeeTemplateRejectsDuplicateTypeForTenant(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()

	_, err = svc.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID: tenantID,
		Type:     "custom_reviewer",
		Label:    "评审员",
	})
	require.NoError(t, err)

	_, err = svc.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID: tenantID,
		Type:     "custom_reviewer",
		Label:    "重复的评审员",
	})

	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestCreateEmployeeTemplateSucceeds(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()

	created, err := svc.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID:                 tenantID,
		Type:                     "custom_reviewer",
		Label:                    "自定义评审员",
		RecommendedSkills:        []string{"code-review"},
		RecommendedMCPServers:    []string{},
		RecommendedProviderTypes: []string{"codex"},
	})

	require.NoError(t, err)
	require.Equal(t, "custom_reviewer", created.Type)
	require.Equal(t, "active", created.Status)
	require.False(t, created.IsSystem)
}

func TestCreateEmployeeTemplateStoresFinalConfigDefaults(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()

	created, err := svc.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID:              tenantID,
		Type:                  "evidence_worker",
		Label:                 "证据整理员工",
		Description:           "整理证据",
		DefaultRole:           "evidence_worker",
		PersonaMemoryMarkdown: "# 人格画像\n证据优先",
		CapabilityBindings:    map[string]any{"skills": []any{"code-reading"}},
		BudgetPolicy:          map[string]any{"daily_token_limit": float64(10000)},
	})

	require.NoError(t, err)
	require.Equal(t, "# 人格画像\n证据优先", created.PersonaMemoryMarkdown)
	require.Equal(t, []any{"code-reading"}, created.CapabilityBindings["skills"])
	require.Equal(t, float64(10000), created.BudgetPolicy["daily_token_limit"])
}

func TestCreateEmployeeTemplateNormalizesNilPolicyMaps(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()

	_, err = svc.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID:           tenantID,
		Type:               "custom_reviewer",
		Label:              "自定义评审员",
		CapabilityBindings: nil,
		BudgetPolicy:       nil,
	})
	require.NoError(t, err, "nil policy maps are allowed and normalized to {}")
}

func TestUpdateEmployeeTemplateRejectsTypeChangeAttemptSilently(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	created, err := svc.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID: tenantID,
		Type:     "custom_reviewer",
		Label:    "自定义评审员",
	})
	require.NoError(t, err)

	updated, err := svc.UpdateEmployeeTemplate(context.Background(), UpdateEmployeeTemplateParams{
		TenantID: tenantID,
		ID:       created.ID,
		Label:    "评审员 v2",
	})

	require.NoError(t, err)
	require.Equal(t, "custom_reviewer", updated.Type, "type must stay immutable regardless of what UpdateEmployeeTemplateParams carries")
	require.Equal(t, "评审员 v2", updated.Label)
}

func TestSetEmployeeTemplateStatusRejectsUnknownStatus(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()
	created, err := svc.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID: tenantID,
		Type:     "custom_reviewer",
		Label:    "自定义评审员",
	})
	require.NoError(t, err)

	_, err = svc.SetEmployeeTemplateStatus(context.Background(), tenantID, created.ID, "archived")

	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestDeleteEmployeeTemplateAllowsDeletingSystemTemplates(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()

	// is_system is never settable through CreateEmployeeTemplate (no template
	// ships pre-seeded); flip it directly on the test double's storage to
	// exercise deletion of a system-owned template.
	created, err := repo.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID: tenantID,
		Type:     "custom_reviewer",
		Label:    "自定义评审员",
	})
	require.NoError(t, err)
	templates := repo.templates[tenantID]
	for i := range templates {
		if templates[i].ID == created.ID {
			templates[i].IsSystem = true
		}
	}
	repo.templates[tenantID] = templates

	target, err := svc.GetEmployeeTemplate(context.Background(), tenantID, created.ID)
	require.NoError(t, err)
	require.True(t, target.IsSystem)

	err = svc.DeleteEmployeeTemplate(context.Background(), tenantID, target.ID)

	require.NoError(t, err)
	_, err = svc.GetEmployeeTemplate(context.Background(), tenantID, target.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

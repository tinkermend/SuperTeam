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
		Type:     "database_admin",
		Label:    "重复的数据库管理",
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

func TestCreateEmployeeTemplateNormalizesNilPolicyMaps(t *testing.T) {
	repo := newMemoryRepository()
	svc, err := NewService(repo)
	require.NoError(t, err)
	tenantID := uuid.New()

	_, err = svc.CreateEmployeeTemplate(context.Background(), CreateEmployeeTemplateParams{
		TenantID:                   tenantID,
		Type:                       "custom_reviewer",
		Label:                      "自定义评审员",
		DefaultCapabilitySelection: nil,
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
	templates, err := svc.ListEmployeeTemplates(context.Background(), tenantID)
	require.NoError(t, err)
	require.NotEmpty(t, templates)
	target := templates[0]
	require.True(t, target.IsSystem)

	err = svc.DeleteEmployeeTemplate(context.Background(), tenantID, target.ID)

	require.NoError(t, err)
	_, err = svc.GetEmployeeTemplate(context.Background(), tenantID, target.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

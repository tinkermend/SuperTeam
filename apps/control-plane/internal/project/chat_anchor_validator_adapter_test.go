package project

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/superteam/control-plane/internal/employee"
)

// TestChatAnchorProjectValidatorAdapter_ApprovesActiveProject confirms the
// happy path: an existing, active, same-tenant project is approved (nil
// error) — the baseline every rejection case below is contrasted against.
func TestChatAnchorProjectValidatorAdapter_ApprovesActiveProject(t *testing.T) {
	repo := newMemoryRepository()
	tenantID, projectID := uuid.New(), uuid.New()
	repo.projects[projectID] = Project{
		ID:       projectID,
		TenantID: tenantID,
		Status:   ProjectStatusRunning,
	}
	service, err := NewService(repo)
	require.NoError(t, err)
	validator := NewChatAnchorProjectValidatorAdapter(service)

	err = validator.ValidateChatAnchorProject(context.Background(), tenantID, projectID)

	require.NoError(t, err)
}

// TestChatAnchorProjectValidatorAdapter_RejectsUnknownProject covers the §13
// 400 rejection matrix's "not exists" case: requireActiveProject's
// ErrProjectNotFound must be mapped to employee.ErrInvalidInput (400-mapped by
// the employee handler layer), not surfaced as the raw project-package
// sentinel.
func TestChatAnchorProjectValidatorAdapter_RejectsUnknownProject(t *testing.T) {
	repo := newMemoryRepository()
	tenantID := uuid.New()
	service, err := NewService(repo)
	require.NoError(t, err)
	validator := NewChatAnchorProjectValidatorAdapter(service)

	err = validator.ValidateChatAnchorProject(context.Background(), tenantID, uuid.New())

	require.ErrorIs(t, err, employee.ErrInvalidInput)
	require.False(t, errors.Is(err, ErrProjectNotFound), "adapter must not leak the raw project.ErrProjectNotFound sentinel across the package boundary")
}

// TestChatAnchorProjectValidatorAdapter_RejectsCrossTenantProject covers the
// §13 rejection matrix's "cross-tenant" case. memoryRepository.GetProject (and
// the production pg_repository behind it) treats a project row that exists
// but belongs to a different tenant identically to "no such row" — it returns
// ErrProjectNotFound rather than a distinct cross-tenant sentinel — so this
// is, by construction, the same code path as the not-found case above; this
// test exercises it explicitly (project exists for tenant A, validated
// against tenant B) rather than relying on the not-found test to imply it.
func TestChatAnchorProjectValidatorAdapter_RejectsCrossTenantProject(t *testing.T) {
	repo := newMemoryRepository()
	ownerTenantID, otherTenantID, projectID := uuid.New(), uuid.New(), uuid.New()
	repo.projects[projectID] = Project{
		ID:       projectID,
		TenantID: ownerTenantID,
		Status:   ProjectStatusRunning,
	}
	service, err := NewService(repo)
	require.NoError(t, err)
	validator := NewChatAnchorProjectValidatorAdapter(service)

	err = validator.ValidateChatAnchorProject(context.Background(), otherTenantID, projectID)

	require.ErrorIs(t, err, employee.ErrInvalidInput)
}

// TestChatAnchorProjectValidatorAdapter_RejectsArchivedProject covers the §13
// rejection matrix's "archived" case: requireActiveProject's ErrProjectArchived
// must also map to employee.ErrInvalidInput.
func TestChatAnchorProjectValidatorAdapter_RejectsArchivedProject(t *testing.T) {
	repo := newMemoryRepository()
	tenantID, projectID := uuid.New(), uuid.New()
	archivedAt := time.Now()
	repo.projects[projectID] = Project{
		ID:         projectID,
		TenantID:   tenantID,
		Status:     ProjectStatusArchived,
		ArchivedAt: &archivedAt,
	}
	service, err := NewService(repo)
	require.NoError(t, err)
	validator := NewChatAnchorProjectValidatorAdapter(service)

	err = validator.ValidateChatAnchorProject(context.Background(), tenantID, projectID)

	require.ErrorIs(t, err, employee.ErrInvalidInput)
	require.False(t, errors.Is(err, ErrProjectArchived), "adapter must not leak the raw project.ErrProjectArchived sentinel across the package boundary")
}

// --- ValidateChatParticipant (团队归属参与门禁) ---

func TestChatAnchorValidatorAdapter_ApprovesActiveProjectMemberParticipant(t *testing.T) {
	repo := newMemoryRepository()
	tenantID, projectID, employeeID := uuid.New(), uuid.New(), uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning}
	repo.members[projectID] = []ProjectMember{{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
		PrincipalType: PrincipalTypeDigitalEmployee, PrincipalID: employeeID,
		ProjectRole: ProjectRoleExecutor, Status: "active",
	}}
	service, err := NewService(repo)
	require.NoError(t, err)
	validator := NewChatAnchorProjectValidatorAdapter(service).(employee.ChatParticipantValidator)

	require.NoError(t, validator.ValidateChatParticipant(context.Background(), tenantID, projectID, employeeID))
}

func TestChatAnchorValidatorAdapter_RejectsNonMemberParticipant(t *testing.T) {
	repo := newMemoryRepository()
	tenantID, projectID := uuid.New(), uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning}
	service, err := NewService(repo)
	require.NoError(t, err)
	validator := NewChatAnchorProjectValidatorAdapter(service).(employee.ChatParticipantValidator)

	err = validator.ValidateChatParticipant(context.Background(), tenantID, projectID, uuid.New())
	require.ErrorIs(t, err, employee.ErrInvalidInput)
}

func TestChatAnchorValidatorAdapter_RejectsInactiveMemberParticipant(t *testing.T) {
	repo := newMemoryRepository()
	tenantID, projectID, employeeID := uuid.New(), uuid.New(), uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning}
	repo.members[projectID] = []ProjectMember{{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
		PrincipalType: PrincipalTypeDigitalEmployee, PrincipalID: employeeID,
		ProjectRole: ProjectRoleExecutor, Status: "removed",
	}}
	service, err := NewService(repo)
	require.NoError(t, err)
	validator := NewChatAnchorProjectValidatorAdapter(service).(employee.ChatParticipantValidator)

	err = validator.ValidateChatParticipant(context.Background(), tenantID, projectID, employeeID)
	require.ErrorIs(t, err, employee.ErrInvalidInput)
}

func TestChatAnchorValidatorAdapter_RejectsHumanPrincipalAsParticipant(t *testing.T) {
	repo := newMemoryRepository()
	tenantID, projectID, humanID := uuid.New(), uuid.New(), uuid.New()
	repo.projects[projectID] = Project{ID: projectID, TenantID: tenantID, Status: ProjectStatusRunning}
	repo.members[projectID] = []ProjectMember{{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
		PrincipalType: PrincipalTypeHumanUser, PrincipalID: humanID,
		ProjectRole: ProjectRoleOwner, Status: "active",
	}}
	service, err := NewService(repo)
	require.NoError(t, err)
	validator := NewChatAnchorProjectValidatorAdapter(service).(employee.ChatParticipantValidator)

	err = validator.ValidateChatParticipant(context.Background(), tenantID, projectID, humanID)
	require.ErrorIs(t, err, employee.ErrInvalidInput)
}

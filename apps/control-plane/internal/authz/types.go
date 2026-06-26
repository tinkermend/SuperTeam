package authz

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

const (
	ActorUser        = "user"
	ActorRuntimeNode = "runtime_node"
	ActorEmployee    = "employee"
	ActorService     = "service_account"
)

const (
	ResourceConsole    = "console"
	ResourceTenant     = "tenant"
	ResourceTeam       = "team"
	ResourceProject    = "project"
	ResourceEmployee   = "employee"
	ResourceSkill      = "skill"
	ResourceCredential = "credential"
	ResourceTask       = "task"
)

const (
	ActionConsoleAccess      = "console.access"
	ActionTenantAccess       = "tenant.access"
	ActionTeamAccess         = "team.access"
	ActionTaskClaim          = "task.claim"
	ActionRuntimeScopeManage = "runtime_scope.manage"
	ActionAuthzCenterRead    = "authz_center.read"

	ActionUserProjectTeamScopeRead   = "user.project_team_scope.read"
	ActionUserProjectTeamScopeManage = "user.project_team_scope.manage"

	ActionEmployeeCreate         = "employee.create"
	ActionEmployeeRead           = "employee.read"
	ActionEmployeeStatusUpdate   = "employee.status.update"
	ActionEmployeeExecutionBind  = "employee.execution.bind"
	ActionEmployeeConfigCreate   = "employee.config.create"
	ActionEmployeeConfigPreview  = "employee.config.preview"
	ActionEmployeeConfigApprove  = "employee.config.approve"
	ActionEmployeeCapabilityEdit = "employee.capability.edit"
	ActionEmployeeRunCreate      = "employee.run.create"
	ActionEmployeeRunStop        = "employee.run.stop"
	ActionEmployeeRunLogRead     = "employee.run.log.read"

	ActionCredentialRead   = "credential.read"
	ActionCredentialCreate = "credential.create"
	ActionCredentialDelete = "credential.delete"

	ActionSkillRead    = "skill.read"
	ActionSkillUpload  = "skill.upload"
	ActionSkillDelete  = "skill.delete"
	ActionSkillInstall = "skill.install"

	ActionTeamCreate                      = "team.create"
	ActionTeamRead                        = "team.read"
	ActionTeamUpdate                      = "team.update"
	ActionTeamDisable                     = "team.disable"
	ActionTeamArchive                     = "team.archive"
	ActionTeamRestore                     = "team.restore"
	ActionTeamDelete                      = "team.delete"
	ActionTeamMemberAdd                   = "team.member.add"
	ActionTeamMemberRemove                = "team.member.remove"
	ActionTeamMemberChangeRole            = "team.member.change_role"
	ActionTeamMemberRequestPrivilegedRole = "team.member.request_privileged_role"
	ActionTeamMemberApprovePrivilegedRole = "team.member.approve_privileged_role"
	ActionTeamGovernanceRead              = "team.governance.read"
	ActionTeamGovernanceEdit              = "team.governance.edit"
	ActionTeamGovernanceApprove           = "team.governance.approve"
	ActionTeamCapabilityBind              = "team.capability.bind"
	ActionTeamCapabilityUnbind            = "team.capability.unbind"
	ActionTeamCapabilityManage            = "team.capability.manage"
	ActionTeamAuditRead                   = "team.audit.read"
	ActionTeamLendingPolicyRead           = "team.lending.policy.read"
	ActionTeamLendingPolicyEdit           = "team.lending.policy.edit"
	ActionTeamLendingRequestRead          = "team.lending.request.read"
	ActionTeamLendingRequestDecide        = "team.lending.request.decide"

	ActionProjectCreate           = "project.create"
	ActionProjectRead             = "project.read"
	ActionProjectUpdate           = "project.update"
	ActionProjectArchive          = "project.archive"
	ActionProjectMemberRead       = "project.member.read"
	ActionProjectMemberManage     = "project.member.manage"
	ActionProjectDemandRead       = "project.demand.read"
	ActionProjectDemandSubmit     = "project.demand.submit"
	ActionProjectTaskRead         = "project.task.read"
	ActionProjectEventRead        = "project.event.read"
	ActionProjectDecisionRead     = "project.decision.read"
	ActionProjectDecisionResolve  = "project.decision.resolve"
	ActionProjectEvidenceRead     = "project.evidence.read"
	ActionProjectEvidenceCreate   = "project.evidence.create"
	ActionProjectEvidenceUpdate   = "project.evidence.update"
	ActionProjectArtifactRead     = "project.artifact.read"
	ActionProjectReportRead       = "project.report.read"
	ActionProjectBudgetRead       = "project.budget.read"
	ActionProjectAcceptanceCreate = "project.acceptance.create"
	ActionProjectAcceptanceRead   = "project.acceptance.read"
	ActionProjectConfigRead       = "project.config.read"
	ActionProjectConfigEdit       = "project.config.edit"

	ActionAuditRead = "audit.read"

	ActionTaskRead   = "task.read"
	ActionTaskCreate = "task.create"
	ActionTaskUpdate = "task.update"
	ActionTaskCancel = "task.cancel"
)

const (
	RoleOwner    = "owner"
	RoleAdmin    = "admin"
	RoleApprover = "approver"
	RoleMember   = "member"
	RoleViewer   = "viewer"
)

const (
	ReasonAllowed                        = "allowed"
	ReasonNoMembership                   = "no active membership"
	ReasonInvalidActor                   = "invalid actor"
	ReasonInvalidResource                = "invalid resource"
	ReasonUnsupportedAction              = "unsupported action"
	ReasonRuntimeScopeMissing            = "runtime scope does not cover task"
	ReasonPrivilegedRoleRequiresApproval = "privileged role requires approval"
	ReasonLastTeamOwner                  = "cannot remove last team owner"
)

var (
	ErrNoMembership      = errors.New("no active membership")
	ErrUnsupportedAction = errors.New("unsupported action")
)

type ActorRef struct {
	Type string
	ID   string
}

type ResourceRef struct {
	Type string
	ID   string
}

type CheckRequest struct {
	Actor       ActorRef
	Action      string
	Resource    ResourceRef
	TenantID    uuid.UUID
	TeamID      *uuid.UUID
	Context     map[string]any
	AuditReason string
}

type Decision struct {
	Allowed       bool
	Reason        string
	MatchedRule   string
	RequiresAudit bool
	Snapshot      map[string]any
}

type DecisionRecord struct {
	TenantID     uuid.UUID
	TeamID       *uuid.UUID
	ActorType    string
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	Allowed      bool
	Reason       string
	MatchedRule  string
	Engine       string
	Snapshot     map[string]any
}

type EngineStatus struct {
	Engine          string
	Status          string
	EngineVersion   string
	OpenFGAStoreID  string
	OpenFGAModelID  string
	RecentDiffCount int64
}

type EngineReporter interface {
	AuthzEngineStatus() EngineStatus
}

type DecisionRecorder interface {
	RecordDecision(ctx context.Context, record DecisionRecord) error
}

type Membership struct {
	TenantID      uuid.UUID
	TeamID        *uuid.UUID
	PrincipalType string
	PrincipalID   uuid.UUID
	Role          string
	Status        string
}

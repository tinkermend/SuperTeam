package permission

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

// ResourceTypeEmployeeConfigRevision is the S1 subject resource_type: a digital
// employee 治理配置修订 (role / permission_policy / sensitive capability grants)
// pending approval before activation.
const ResourceTypeEmployeeConfigRevision = "digital_employee_config_revision"

// ErrConfigActivatorNotReady is returned when an S1 approval is applied but the
// employee-config seam (ActivateConfigRevision) has not been wired yet. Per the
// 2026-07-20 spec §10.1 sequencing, this session registers the S1 slot + the
// Apply calling convention only; the employee-config spec delivers the activator
// and the producing side. Until then S1 approvals cannot be produced, so this
// path is not exercised end-to-end (E2E for S1 is BLOCKED on that seam).
var ErrConfigActivatorNotReady = errors.New(
	"permission: employee config activation not wired — 待员工配置 spec 交付 ActivateConfigRevision(接缝见 spec §4.4/§10.1)")

// ActivateConfigRevisionInput is the apply-seam contract (§4.4). The employee
// domain owns the implementation: draft→active + role/permission 写回员工行 + 审计,
// idempotent.
type ActivateConfigRevisionInput struct {
	TenantID    uuid.UUID
	EmployeeID  uuid.UUID
	RevisionID  uuid.UUID
	ActivatedBy uuid.UUID
}

// ConfigRevisionActivator is implemented by the employee domain (employee-config
// spec). The permission center only calls it from the S1 subject's Apply.
type ConfigRevisionActivator interface {
	ActivateConfigRevision(ctx context.Context, in ActivateConfigRevisionInput) error
}

type employeeConfigSubject struct {
	activator ConfigRevisionActivator
}

// NewEmployeeConfigSubject builds the S1 subject. Pass a nil activator to register
// the slot before the employee-config seam is ready — Apply then returns
// ErrConfigActivatorNotReady rather than silently succeeding.
func NewEmployeeConfigSubject(activator ConfigRevisionActivator) Subject {
	return &employeeConfigSubject{activator: activator}
}

func (s *employeeConfigSubject) ResourceType() string { return ResourceTypeEmployeeConfigRevision }
func (s *employeeConfigSubject) Actions() []Action    { return DefaultActions() }

func (s *employeeConfigSubject) Apply(ctx context.Context, in ApplyInput) error {
	if s.activator == nil {
		return ErrConfigActivatorNotReady
	}
	revisionID, err := uuidFromPayload(in.Request.ContextPayload, "revision_id")
	if err != nil {
		// Fall back to the approval's resource_id, which the producing side sets
		// to the config revision id.
		revisionID = in.Request.ResourceID
	}
	employeeID, _ := uuidFromPayload(in.Request.ContextPayload, "employee_id")
	return s.activator.ActivateConfigRevision(ctx, ActivateConfigRevisionInput{
		TenantID:    in.Request.TenantID,
		EmployeeID:  employeeID,
		RevisionID:  revisionID,
		ActivatedBy: in.DecidedBy,
	})
}

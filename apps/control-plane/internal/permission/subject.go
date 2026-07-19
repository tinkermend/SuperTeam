// Package permission implements the 权限中心「权限审批」域 — the platform's
// operation-permission approval domain. It reads the approval fact table directly
// (category=permission), never through the inbox projection, and dispatches
// post-approval side-effects to registered subjects (§4.3 / D5 of the
// 2026-07-20 permission-center-refactor spec).
package permission

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/approval"
)

// Action is a decision verb offered on a pending permission approval.
type Action struct {
	Key   string `json:"key"`   // approval.ApprovalDecision value
	Label string `json:"label"` // 中文标签
	Tone  string `json:"tone"`  // positive / destructive / neutral
}

// DefaultActions is the standard 三选 decision vocabulary shared by permission
// subjects (同意 / 驳回 / 要求补证).
func DefaultActions() []Action {
	return []Action{
		{Key: string(approval.ApprovalDecisionApproved), Label: "同意", Tone: "positive"},
		{Key: string(approval.ApprovalDecisionRejected), Label: "驳回", Tone: "destructive"},
		{Key: string(approval.ApprovalDecisionNeedsMoreEvidence), Label: "要求补证", Tone: "neutral"},
	}
}

// ApplyInput carries an approved request into a subject's side-effect.
type ApplyInput struct {
	Request   approval.ApprovalRequest
	Decision  approval.ApprovalDecision
	DecidedBy uuid.UUID
	Comment   string
}

// Subject is a permission-approval subject (§4.3). Each subject registers its
// resource_type, decision vocabulary and the idempotent Apply side-effect run
// after approval. Adding a new subject registers here rather than editing a
// closed switch — this pays down the §3.3 hardcoding debt (D5).
type Subject interface {
	ResourceType() string
	Actions() []Action
	// Apply runs the subject's business side-effect. It is invoked ONLY for the
	// approved decision and MUST be idempotent (it may run before the request is
	// marked resolved, and may retry). rejected / needs_more_evidence close or
	// hold the request with no side-effect.
	Apply(ctx context.Context, in ApplyInput) error
}

// Registry is the generic subject registry.
type Registry struct {
	subjects map[string]Subject
}

func NewRegistry() *Registry {
	return &Registry{subjects: map[string]Subject{}}
}

// Register adds a subject keyed by its resource_type. A later registration for
// the same resource_type replaces the earlier one.
func (r *Registry) Register(s Subject) {
	if s == nil {
		return
	}
	r.subjects[s.ResourceType()] = s
}

func (r *Registry) Get(resourceType string) (Subject, bool) {
	s, ok := r.subjects[resourceType]
	return s, ok
}

// ActionsFor returns the decision vocabulary for a resource_type, falling back
// to the default 三选 when a subject is unknown so the read path stays resilient
// to legacy or not-yet-registered rows.
func (r *Registry) ActionsFor(resourceType string) []Action {
	if s, ok := r.subjects[resourceType]; ok {
		return s.Actions()
	}
	return DefaultActions()
}

// Apply dispatches the post-approval side-effect to the registered subject.
func (r *Registry) Apply(ctx context.Context, in ApplyInput) error {
	s, ok := r.subjects[in.Request.ResourceType]
	if !ok {
		return fmt.Errorf("permission: no subject registered for resource_type %q", in.Request.ResourceType)
	}
	return s.Apply(ctx, in)
}

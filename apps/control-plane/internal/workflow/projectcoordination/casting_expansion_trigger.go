package projectcoordination

import (
	"context"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/project"
)

// castingExpansionStore is the narrow store seam for post-completion expansion
// proposals (design §7.1 / G9). Type-asserted off a.store so ActivityStore and
// its fakes need not grow.
type castingExpansionStore interface {
	MaybeRequestCastingExpansionAfterTask(ctx context.Context, input MaybeRequestCastingExpansionInput) (MaybeRequestCastingExpansionResult, error)
}

// MaybeRequestCastingExpansionInput identifies a just-accepted completed task.
type MaybeRequestCastingExpansionInput struct {
	TenantID        uuid.UUID
	ProjectID       uuid.UUID
	CompletedTaskID uuid.UUID
}

// MaybeRequestCastingExpansionResult mirrors project.MaybeCastingExpansionResult
// for Temporal activity serialization.
type MaybeRequestCastingExpansionResult struct {
	Requested        bool
	DecisionID       uuid.UUID
	DemandID         uuid.UUID
	SuggestedRoleKey string
	SkippedReason    string
}

// CastingExpansionProposer is optional wiring from project.Service so the
// coordinator can open casting_expansion after task completion without
// duplicating readiness/vocab rules.
type CastingExpansionProposer interface {
	MaybeRequestCastingExpansionForCompletedTask(ctx context.Context, tenantID, projectID, completedTaskID uuid.UUID) (project.MaybeCastingExpansionResult, error)
}

// MaybeRequestCastingExpansion is the version-fenced Activity invoked after an
// accepted EmployeeTaskCompleted. It never fails the task graph: callers should
// log and swallow errors. Returns Requested=false when casting is complete or
// an expansion is already pending.
func (a *Activities) MaybeRequestCastingExpansion(ctx context.Context, input MaybeRequestCastingExpansionInput) (MaybeRequestCastingExpansionResult, error) {
	store, ok := a.store.(castingExpansionStore)
	if a.store == nil || !ok {
		// Unwired store (tests / old workers): no-op, not an error.
		return MaybeRequestCastingExpansionResult{SkippedReason: "store_unwired"}, nil
	}
	return store.MaybeRequestCastingExpansionAfterTask(ctx, input)
}

// MaybeRequestCastingExpansionAfterTask delegates to the optional proposer.
func (s *ProjectStore) MaybeRequestCastingExpansionAfterTask(ctx context.Context, input MaybeRequestCastingExpansionInput) (MaybeRequestCastingExpansionResult, error) {
	if s == nil || s.castingExpansion == nil {
		return MaybeRequestCastingExpansionResult{SkippedReason: "proposer_unwired"}, nil
	}
	out, err := s.castingExpansion.MaybeRequestCastingExpansionForCompletedTask(ctx, input.TenantID, input.ProjectID, input.CompletedTaskID)
	if err != nil {
		return MaybeRequestCastingExpansionResult{}, err
	}
	return MaybeRequestCastingExpansionResult{
		Requested:        out.Requested,
		DecisionID:       out.DecisionID,
		DemandID:         out.DemandID,
		SuggestedRoleKey: out.SuggestedRoleKey,
		SkippedReason:    out.SkippedReason,
	}, nil
}

// WithCastingExpansionProposer attaches the project-service expansion proposer
// (typically *project.Service after casting deps are wired).
func (s *ProjectStore) WithCastingExpansionProposer(proposer CastingExpansionProposer) *ProjectStore {
	if s != nil {
		s.castingExpansion = proposer
	}
	return s
}

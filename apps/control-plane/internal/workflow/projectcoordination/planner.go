package projectcoordination

import (
	"errors"
	"strings"

	"github.com/google/uuid"
)

var ErrInvalidRouteDecision = errors.New("invalid route decision")

type CoordinationSnapshot struct {
	ProjectID            uuid.UUID
	Demand               DemandSnapshot
	DigitalEmployeePool  []ProjectMemberSnapshot
	CoordinationPolicy   map[string]any
	PreviousRouteContext map[string]any
}

type DemandSnapshot struct {
	ID      uuid.UUID
	Title   string
	Content string
}

type ProjectMemberSnapshot struct {
	PrincipalID uuid.UUID
	ProjectRole string
	Status      string
	DisplayName string
}

type RouteDecisionPlan struct {
	CandidateDigitalEmployeeIDs []uuid.UUID
	SelectedDigitalEmployeeIDs  []uuid.UUID
	Reason                      string
	InputRequirements           map[string]any
	ExpectedOutputs             []string
	BudgetEstimate              map[string]any
	RequiresHumanReview         bool
	TaskTitle                   string
	TaskSummary                 string
}

func PlanDemandRoute(snapshot CoordinationSnapshot) (RouteDecisionPlan, error) {
	candidates := activeExecutorIDs(snapshot.DigitalEmployeePool)
	if len(candidates) == 0 {
		return RouteDecisionPlan{}, ErrInvalidRouteDecision
	}
	selected := []uuid.UUID{candidates[0]}
	title := strings.TrimSpace(snapshot.Demand.Title)
	if title == "" {
		title = "处理项目需求"
	}
	decision := RouteDecisionPlan{
		CandidateDigitalEmployeeIDs: candidates,
		SelectedDigitalEmployeeIDs:  selected,
		Reason:                      "选择项目数字员工池中的 active executor 作为第一执行人",
		InputRequirements: map[string]any{
			"demand_id": snapshot.Demand.ID.String(),
			"title":     title,
			"content":   snapshot.Demand.Content,
		},
		ExpectedOutputs:     []string{"execution_summary", "evidence_refs", "recommended_next_action"},
		BudgetEstimate:      map[string]any{"mode": "policy_default"},
		RequiresHumanReview: highRiskPolicyEnabled(snapshot.CoordinationPolicy),
		TaskTitle:           title,
		TaskSummary:         snapshot.Demand.Content,
	}
	return decision, ValidateRouteDecision(decision, candidates)
}

func ValidateRouteDecision(decision RouteDecisionPlan, poolIDs []uuid.UUID) error {
	if strings.TrimSpace(decision.Reason) == "" || len(decision.SelectedDigitalEmployeeIDs) == 0 || len(decision.ExpectedOutputs) == 0 {
		return ErrInvalidRouteDecision
	}
	pool := map[uuid.UUID]struct{}{}
	for _, id := range poolIDs {
		if id != uuid.Nil {
			pool[id] = struct{}{}
		}
	}
	for _, id := range decision.SelectedDigitalEmployeeIDs {
		if id == uuid.Nil {
			return ErrInvalidRouteDecision
		}
		if _, ok := pool[id]; !ok {
			return ErrInvalidRouteDecision
		}
	}
	return nil
}

func activeExecutorIDs(members []ProjectMemberSnapshot) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(members))
	for _, member := range members {
		if member.PrincipalID != uuid.Nil && member.ProjectRole == "executor" && member.Status == "active" {
			ids = append(ids, member.PrincipalID)
		}
	}
	return ids
}

func highRiskPolicyEnabled(policy map[string]any) bool {
	value, ok := policy["require_human_review_for_new_demands"].(bool)
	return ok && value
}

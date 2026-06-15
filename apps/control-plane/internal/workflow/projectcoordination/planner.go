package projectcoordination

import (
	"context"
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

type RoutePlanner interface {
	Plan(ctx context.Context, snapshot CoordinationSnapshot) (RouteDecisionPlan, error)
}

type RouteDecisionPlan struct {
	Reason              string
	RequiresHumanReview bool
	BudgetEstimate      map[string]any
	TemplateKey         string
	PlannerMetadata     map[string]any
	Tasks               []PlannedTask
}

type PlannedTask struct {
	Key                   string
	Title                 string
	Summary               string
	SelectedEmployeeID    uuid.UUID
	TaskKind              string
	StageIndex            *int32
	RiskLevel             string
	RequiresHumanApproval bool
	ExpectedOutputs       []string
	InputRequirements     map[string]any
	HandoffContract       map[string]any
	BlockedByKeys         []string
}

type HeuristicRoutePlanner struct{}

func (HeuristicRoutePlanner) Plan(ctx context.Context, snapshot CoordinationSnapshot) (RouteDecisionPlan, error) {
	_ = ctx
	candidates := activeExecutorIDs(snapshot.DigitalEmployeePool)
	if len(candidates) == 0 {
		return RouteDecisionPlan{}, ErrInvalidRouteDecision
	}
	title := strings.TrimSpace(snapshot.Demand.Title)
	if title == "" {
		title = "处理项目需求"
	}
	summary := strings.TrimSpace(snapshot.Demand.Content)
	if summary == "" {
		summary = title
	}
	requiresHumanReview := highRiskPolicyEnabled(snapshot.CoordinationPolicy)
	stageIndex := int32(0)
	expectedOutputs := []string{"execution_summary", "evidence_refs", "recommended_next_action"}
	if multiExecutorFanoutEnabled(snapshot.CoordinationPolicy) && len(candidates) > 1 {
		tasks := make([]PlannedTask, 0, len(candidates))
		for index, candidateID := range candidates {
			taskStageIndex := stageIndex
			key := "execute_demand_" + uuidShort(candidateID)
			tasks = append(tasks, PlannedTask{
				Key:                   key,
				Title:                 title,
				Summary:               summary,
				SelectedEmployeeID:    candidateID,
				TaskKind:              "execution",
				StageIndex:            &taskStageIndex,
				RiskLevel:             "normal",
				RequiresHumanApproval: requiresHumanReview,
				ExpectedOutputs:       expectedOutputs,
				InputRequirements: map[string]any{
					"demand_id":               snapshot.Demand.ID.String(),
					"title":                   title,
					"content":                 snapshot.Demand.Content,
					"fanout_index":            index,
					"selected_employee_id":    candidateID.String(),
					"selected_employee_count": len(candidates),
				},
				HandoffContract: map[string]any{
					"expected_outputs": stringsToAny(expectedOutputs),
					"completion_path":  "project_task_writeback",
				},
			})
		}
		decision := RouteDecisionPlan{
			Reason:              "按协调策略将需求分派给项目数字员工池中的所有 active executor",
			RequiresHumanReview: requiresHumanReview,
			BudgetEstimate:      map[string]any{"mode": "policy_default", "task_count": len(tasks)},
			TemplateKey:         "heuristic.multi_executor_fanout",
			PlannerMetadata: map[string]any{
				"planner":  "heuristic_route_planner",
				"strategy": "all_active_executors",
			},
			Tasks: tasks,
		}
		return decision, ValidateRouteDecision(decision, candidates)
	}
	decision := RouteDecisionPlan{
		Reason:              "选择项目数字员工池中的 active executor 作为第一执行人",
		RequiresHumanReview: requiresHumanReview,
		BudgetEstimate:      map[string]any{"mode": "policy_default"},
		TemplateKey:         "heuristic.single_task",
		PlannerMetadata: map[string]any{
			"planner":  "heuristic_route_planner",
			"strategy": "first_active_executor",
		},
		Tasks: []PlannedTask{{
			Key:                   "execute_demand",
			Title:                 title,
			Summary:               summary,
			SelectedEmployeeID:    candidates[0],
			TaskKind:              "execution",
			StageIndex:            &stageIndex,
			RiskLevel:             "normal",
			RequiresHumanApproval: requiresHumanReview,
			ExpectedOutputs:       expectedOutputs,
			InputRequirements: map[string]any{
				"demand_id": snapshot.Demand.ID.String(),
				"title":     title,
				"content":   snapshot.Demand.Content,
			},
			HandoffContract: map[string]any{
				"expected_outputs": stringsToAny(expectedOutputs),
				"completion_path":  "project_task_writeback",
			},
		}},
	}
	return decision, ValidateRouteDecision(decision, candidates)
}

func PlanDemandRoute(snapshot CoordinationSnapshot) (RouteDecisionPlan, error) {
	return HeuristicRoutePlanner{}.Plan(context.Background(), snapshot)
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

func multiExecutorFanoutEnabled(policy map[string]any) bool {
	value, ok := policy["dispatch_all_active_executors"].(bool)
	if ok && value {
		return true
	}
	strategy, ok := policy["fallback_planner_strategy"].(string)
	return ok && strategy == "all_active_executors"
}

func uuidShort(id uuid.UUID) string {
	text := strings.ReplaceAll(id.String(), "-", "")
	if len(text) > 12 {
		return text[:12]
	}
	return text
}

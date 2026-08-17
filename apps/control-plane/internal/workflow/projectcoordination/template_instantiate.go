package projectcoordination

import (
	"errors"
	"fmt"
	"strings"

	"github.com/superteam/control-plane/internal/scenariotemplate"
)

// errNoTemplateSkeleton means the snapshot is not a templated playbook and
// planning should fall through to the reasoning-model planner.
var errNoTemplateSkeleton = errors.New("snapshot has no scenario template skeleton")

// InstantiatePlanFromTemplate builds a RouteDecisionPlan from the bound
// scenario template skeleton, casting, and (optional) pinned exit. This is not
// the test-only HeuristicRoutePlanner: it never fans out, never invents graph
// edges, and never calls a model.
func InstantiatePlanFromTemplate(snapshot CoordinationSnapshot) (RouteDecisionPlan, error) {
	if snapshot.ScenarioTemplate == nil || strings.TrimSpace(snapshot.ScenarioTemplate.Key) == "" {
		return RouteDecisionPlan{}, errNoTemplateSkeleton
	}
	spec, err := scenariotemplate.ParseSpec(snapshot.ScenarioTemplate.Spec)
	if err != nil {
		return RouteDecisionPlan{}, fmt.Errorf("scenario template spec: %w", err)
	}
	if len(spec.Skeleton) == 0 {
		return RouteDecisionPlan{}, errNoTemplateSkeleton
	}
	pool := activeExecutorIDs(snapshot.DigitalEmployeePool)
	if len(pool) == 0 {
		return RouteDecisionPlan{}, ErrInvalidRouteDecision
	}
	exit := strings.TrimSpace(snapshot.PinnedExitDeliverable)
	if exit == "" {
		exit = DeepestExitDeliverableWithCasting(snapshot)
	}
	if exit == "" && len(spec.Exits) > 0 {
		exit = spec.Exits[len(spec.Exits)-1].Deliverable
	}
	steps := spec.Skeleton
	if len(spec.Exits) > 0 {
		if strings.TrimSpace(exit) == "" {
			return RouteDecisionPlan{}, invalidRouteDecision("template declares exits but none could be selected")
		}
		pruned, pruneErr := pruneSkeletonForExit(spec, exit)
		if pruneErr != nil {
			return RouteDecisionPlan{}, invalidRouteDecision("invalid exit_deliverable %q: %v", exit, pruneErr)
		}
		steps = pruned
	}
	roleTitle := map[string]string{}
	roleCaps := map[string][]string{}
	for _, role := range spec.Roles {
		roleTitle[role.Key] = role.Title
		roleCaps[role.Key] = append([]string{}, role.RequiredCapabilities...)
	}
	taskByStep := map[string]string{}
	tasks := make([]PlannedTask, 0, len(steps))
	demandTitle := strings.TrimSpace(snapshot.Demand.Title)
	if demandTitle == "" {
		demandTitle = snapshot.ScenarioTemplate.Name
	}
	if demandTitle == "" {
		demandTitle = snapshot.ScenarioTemplate.Key
	}
	placeholder := pool[0]
	for i, step := range steps {
		stage := int32(i)
		produces := make([]string, 0, len(step.ProducesDefaults))
		for _, produce := range step.ProducesDefaults {
			if name := strings.TrimSpace(produce.Name); name != "" {
				produces = append(produces, name)
			}
		}
		title := strings.TrimSpace(step.Title)
		if title == "" {
			title = strings.TrimSpace(roleTitle[step.Role])
		}
		if title == "" {
			title = step.Role
		}
		if title == "" {
			title = step.Step
		}
		handoffContract := map[string]any{"completion_path": "project_task_attempt_writeback"}
		if len(step.NotesDefaults) > 0 {
			notes := make([]map[string]any, 0, len(step.NotesDefaults))
			noteNames := make([]string, 0, len(step.NotesDefaults))
			for _, note := range step.NotesDefaults {
				notes = append(notes, map[string]any{
					"name":        note.Name,
					"description": note.Description,
				})
				noteNames = append(noteNames, note.Name)
			}
			handoffContract["notes"] = notes
			handoffContract["consumer_view"] = fmt.Sprintf("以「%s」视角消费上游结论（关注：%s）", title, strings.Join(noteNames, "、"))
		}
		task := PlannedTask{
			Key:                      step.Step,
			Title:                    demandTitle + " · " + title,
			Summary:                  strings.TrimSpace(snapshot.Demand.Content),
			SelectedEmployeeID:       placeholder,
			EmployeeSelectionReason:  fmt.Sprintf("剧本步骤 %s 角色 %s", step.Step, step.Role),
			RequiredCapabilities:     append([]string{}, roleCaps[step.Role]...),
			MatchedCapabilities:      []string{},
			MissingCapabilities:      []string{},
			PermissionRequirements:   []string{},
			ToolRequirements:         []string{},
			RuntimeRequirements:      []string{},
			VerificationRequirements: []string{},
			TaskKind:                 "feature_development",
			StageIndex:               &stage,
			RiskLevel:                "normal",
			ExpectedOutputs:          append([]string{}, produces...),
			Produces:                 produces,
			InputRequirements:        map[string]any{},
			HandoffContract:          handoffContract,
			RoleKey:                  strings.TrimSpace(step.Role),
		}
		if task.Summary == "" {
			task.Summary = task.Title
		}
		if len(step.RequiredInputsDefaults) > 0 {
			inputs := make([]map[string]any, 0, len(step.RequiredInputsDefaults))
			for _, input := range step.RequiredInputsDefaults {
				entry := map[string]any{"name": input.Name, "required": input.Required == nil || *input.Required}
				if input.Kind != "" {
					entry["kind"] = input.Kind
				}
				inputs = append(inputs, entry)
			}
			task.InputRequirements["required_inputs"] = inputs
		}
		tasks = append(tasks, task)
		taskByStep[step.Step] = task.Key
	}
	for i, step := range steps {
		blocked := make([]string, 0, len(step.DependsOn))
		for _, dep := range step.DependsOn {
			if key, ok := taskByStep[dep]; ok {
				blocked = append(blocked, key)
			}
		}
		tasks[i].BlockedByKeys = blocked
	}
	criteria := make([]PlanAcceptanceCriterion, 0, len(spec.DefaultAcceptanceCriteria))
	for i, criterion := range spec.DefaultAcceptanceCriteria {
		if !scenariotemplate.ExitCondMet(spec, scenariotemplate.SpecConstraintWhen{ExitAtOrBeyond: criterion.AppliesFromExit}, exit) {
			continue
		}
		id := fmt.Sprintf("template_criterion_%d", i+1)
		satisfied := make([]string, 0)
		if step, ok := spec.StepByProduce(criterion.AppliesFromExit); ok {
			if key, found := taskByStep[step.Step]; found {
				satisfied = append(satisfied, key)
			}
		}
		if len(satisfied) == 0 && len(tasks) > 0 {
			satisfied = []string{tasks[len(tasks)-1].Key}
		}
		criteria = append(criteria, PlanAcceptanceCriterion{
			ID:                 id,
			Statement:          criterion.Statement,
			SatisfiedBy:        satisfied,
			VerificationMethod: strings.TrimSpace(criterion.VerificationMethod),
			Severity:           strings.TrimSpace(criterion.Severity),
			Source:             CriterionSourceTemplateDeclared,
		})
	}
	for i := range criteria {
		normalizeCriterionDefaults(&criteria[i])
	}
	exits := make([]PlanExitOption, 0, len(spec.Exits))
	for _, item := range spec.Exits {
		exits = append(exits, PlanExitOption{Deliverable: item.Deliverable, Label: item.Label})
	}
	return RouteDecisionPlan{
		Reason:                 "按场景模板骨架实例化任务图，编制指定执行人",
		BudgetEstimate:         map[string]any{"mode": "template_instantiate", "task_count": len(tasks)},
		TemplateKey:            snapshot.ScenarioTemplate.Key,
		TemplateVersion:        snapshot.ScenarioTemplate.Version,
		ExitDeliverable:        exit,
		AvailableExits:         exits,
		PlannerMetadata:        map[string]any{"planner": "template_instantiate"},
		PlanAcceptanceCriteria: criteria,
		Tasks:                  tasks,
	}, nil
}

func finalizeInstantiatedPlan(snapshot CoordinationSnapshot, plan RouteDecisionPlan) (RouteDecisionPlan, error) {
	applyAcceptanceCriteriaDefaults(&plan, snapshot.CoordinationPolicy)
	if plannerName, _ := plan.PlannerMetadata["planner"].(string); plannerName != "template_instantiate" {
		ApplyTaskTypeDefaults(&plan)
	}
	applyRequiredHumanReviewPolicy(snapshot, &plan)
	ApplyPlanningProfileScores(snapshot, &plan)
	ApplyPlaybookCasting(snapshot, &plan)
	if err := ValidateRouteDecisionPlan(snapshot, plan, GraphValidationPolicy{MaxTasks: 12}); err != nil {
		return RouteDecisionPlan{}, err
	}
	if err := EnforceScenarioTemplateGovernance(snapshot, &plan); err != nil {
		return RouteDecisionPlan{}, err
	}
	AnnotateAndValidateExtraTaskRoles(snapshot, &plan)
	ensureHumanJudgmentCriterion(&plan, snapshot.CoordinationPolicy)
	stripPlannerOnlyRiskFlags(&plan)
	return plan, nil
}

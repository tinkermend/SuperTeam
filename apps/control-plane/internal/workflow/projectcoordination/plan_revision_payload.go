package projectcoordination

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
)

var defaultFinalSummaryRequiredSections = []string{"conclusion", "evidence", "risks", "next_steps"}

type PlanRevisionPayload struct {
	Summary                string                           `json:"summary"`
	TemplateKey            string                           `json:"template_key,omitempty"`
	TemplateVersion        int                              `json:"template_version,omitempty"`
	ExitDeliverable        string                           `json:"exit_deliverable,omitempty"`
	AvailableExits         []PlanExitOption                 `json:"available_exits,omitempty"`
	ConstraintNotes        []PlanConstraintNote             `json:"constraint_notes,omitempty"`
	Assumptions            []string                         `json:"assumptions"`
	RiskAssessment         PlanRevisionRiskAssessment       `json:"risk_assessment"`
	HumanReview            PlanRevisionHumanReview          `json:"human_review"`
	Tasks                  []PlanRevisionTask               `json:"tasks"`
	PlanAcceptanceCriteria []PlanAcceptanceCriterion        `json:"plan_acceptance_criteria,omitempty"`
	FinalSummaryContract   PlanRevisionFinalSummaryContract `json:"final_summary_contract"`
}

type PlanRevisionRiskAssessment struct {
	HighestRiskLevel string   `json:"highest_risk_level,omitempty"`
	HighRiskTaskKeys []string `json:"high_risk_task_keys,omitempty"`
}

type PlanRevisionHumanReview struct {
	Required bool     `json:"required"`
	Reasons  []string `json:"reasons"`
}

type PlanRevisionFinalSummaryContract struct {
	RequiredSections []string `json:"required_sections"`
}

// PlanAcceptanceCriterion is a plan-level acceptance standard the human reviews and
// approves. SatisfiedBy names the task keys whose produces feed this criterion.
// It is checked for plan-internal integrity the same way blocked_by_keys and
// required_inputs are — see the 2026-07-10 plan-phase refactor spec §4.2/§4.3.
// It is NOT a second capability vocabulary: the keys it references are produced by
// the same planner call, in the same plan, approved by the same human.
type PlanAcceptanceCriterion struct {
	ID          string   `json:"id"`
	Statement   string   `json:"statement"`
	SatisfiedBy []string `json:"satisfied_by"`
	// VerificationMethod names how this criterion is judged: automated_test
	// (decidable by command/test evidence, SatisfiedBy required) or
	// human_judgment (business/intent judgment, SatisfiedBy may be empty). See
	// acceptance_criteria.go's knownVerificationMethods registry. Readers must
	// treat an empty value as automated_test (normalizeCriterionDefaults'
	// default), since planner output predating this field omits it.
	VerificationMethod string `json:"verification_method,omitempty"`
	// Severity is blocking (default, readers must treat empty as blocking) or
	// non_blocking.
	Severity string `json:"severity,omitempty"`
	// EvidenceHint is an optional planner-authored pointer to what evidence
	// would satisfy this criterion.
	EvidenceHint string `json:"evidence_hint,omitempty"`
	// AmbiguityFlag is server-computed by markAmbiguousCriteria; it flags a
	// statement as too vague to judge without rejecting the plan.
	AmbiguityFlag bool `json:"ambiguity_flag,omitempty"`
}

type PlanRevisionTask struct {
	PlannedTaskKey              string         `json:"planned_task_key"`
	Title                       string         `json:"title"`
	Objective                   string         `json:"objective"`
	TaskType                    string         `json:"task_type"`
	SelectedEmployeeID          string         `json:"selected_employee_id"`
	EmployeeSelectionReason     string         `json:"employee_selection_reason"`
	RequiredCapabilities        []string       `json:"required_capabilities"`
	MatchedCapabilities         []string       `json:"matched_capabilities"`
	MissingCapabilities         []string       `json:"missing_capabilities"`
	PermissionRequirements      []string       `json:"permission_requirements"`
	ToolRequirements            []string       `json:"tool_requirements"`
	RuntimeRequirements         []string       `json:"runtime_requirements"`
	InputContextRefs            []string       `json:"input_context_refs"`
	InputRequirements           map[string]any `json:"input_requirements,omitempty"`
	Produces                    []string       `json:"produces,omitempty"`
	ExpectedOutputs             []string       `json:"expected_outputs"`
	AcceptanceCriteria          []string       `json:"acceptance_criteria"`
	VerificationRequirements    []string       `json:"verification_requirements"`
	DependsOn                   []string       `json:"depends_on"`
	RiskLevel                   string         `json:"risk_level"`
	HumanReviewRequired         bool           `json:"human_review_required"`
	SelectionScore              int            `json:"selection_score,omitempty"`
	SelectionConfidence         float64        `json:"selection_confidence,omitempty"`
	PlanningProfileSnapshotHash string         `json:"planning_profile_snapshot_hash,omitempty"`
	HandoffContract             map[string]any `json:"handoff_contract,omitempty"`
}

type PlanRevisionPayloadValidationResult struct {
	Acceptable      bool
	PlanFingerprint string
	Errors          []string
	Warnings        []string
	ReviewRequired  bool
	ReviewReasons   []string
}

func BuildPlanRevisionPayload(plan RouteDecisionPlan) PlanRevisionPayload {
	tasks := make([]PlanRevisionTask, 0, len(plan.Tasks))
	humanReview := PlanRevisionHumanReview{
		Required: plan.RequiresHumanReview,
		Reasons:  []string{},
	}
	if plan.RequiresHumanReview {
		humanReview.Reasons = append(humanReview.Reasons, "plan_requires_human_review")
	}
	riskAssessment := PlanRevisionRiskAssessment{}
	for _, task := range plan.Tasks {
		if task.RequiresHumanApproval {
			humanReview.Required = true
			humanReview.Reasons = appendUniqueString(humanReview.Reasons, "task_requires_human_approval:"+task.Key)
		}
		if isHighRiskLevel(task.RiskLevel) {
			riskAssessment.HighRiskTaskKeys = append(riskAssessment.HighRiskTaskKeys, task.Key)
		}
		riskAssessment.HighestRiskLevel = higherRiskLevel(riskAssessment.HighestRiskLevel, task.RiskLevel)
		tasks = append(tasks, PlanRevisionTask{
			PlannedTaskKey:              task.Key,
			Title:                       task.Title,
			Objective:                   task.Summary,
			TaskType:                    task.TaskKind,
			SelectedEmployeeID:          task.SelectedEmployeeID.String(),
			EmployeeSelectionReason:     task.EmployeeSelectionReason,
			RequiredCapabilities:        clonePlanRevisionStringSlice(task.RequiredCapabilities),
			MatchedCapabilities:         clonePlanRevisionStringSlice(task.MatchedCapabilities),
			MissingCapabilities:         clonePlanRevisionStringSlice(task.MissingCapabilities),
			PermissionRequirements:      clonePlanRevisionStringSlice(task.PermissionRequirements),
			ToolRequirements:            clonePlanRevisionStringSlice(task.ToolRequirements),
			RuntimeRequirements:         clonePlanRevisionStringSlice(task.RuntimeRequirements),
			InputContextRefs:            inputContextRefsFromRequirements(task.InputRequirements),
			InputRequirements:           clonePlanRevisionAnyMap(task.InputRequirements),
			Produces:                    clonePlanRevisionStringSlice(task.Produces),
			ExpectedOutputs:             clonePlanRevisionStringSlice(task.ExpectedOutputs),
			AcceptanceCriteria:          acceptanceCriteriaFromHandoffContract(task.HandoffContract, task.ExpectedOutputs),
			VerificationRequirements:    clonePlanRevisionStringSlice(task.VerificationRequirements),
			DependsOn:                   clonePlanRevisionStringSlice(task.BlockedByKeys),
			RiskLevel:                   task.RiskLevel,
			HumanReviewRequired:         task.RequiresHumanApproval,
			SelectionScore:              task.SelectionScore,
			SelectionConfidence:         task.SelectionConfidence,
			PlanningProfileSnapshotHash: task.PlanningProfileSnapshotHash,
			HandoffContract:             clonePlanRevisionAnyMap(task.HandoffContract),
		})
	}
	return PlanRevisionPayload{
		Summary:                plan.Reason,
		TemplateKey:            plan.TemplateKey,
		TemplateVersion:        plan.TemplateVersion,
		ExitDeliverable:        plan.ExitDeliverable,
		AvailableExits:         plan.AvailableExits,
		ConstraintNotes:        plan.ConstraintNotes,
		Assumptions:            []string{},
		RiskAssessment:         riskAssessment,
		HumanReview:            humanReview,
		Tasks:                  tasks,
		PlanAcceptanceCriteria: plan.PlanAcceptanceCriteria,
		FinalSummaryContract: PlanRevisionFinalSummaryContract{
			RequiredSections: clonePlanRevisionStringSlice(defaultFinalSummaryRequiredSections),
		},
	}
}

func CanonicalPlanFingerprint(payload PlanRevisionPayload) (string, error) {
	canonical := canonicalPlanRevisionPayload(payload)
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func ValidatePlanRevisionPayload(payload PlanRevisionPayload) PlanRevisionPayloadValidationResult {
	result := PlanRevisionPayloadValidationResult{Acceptable: true}
	fingerprint, err := CanonicalPlanFingerprint(payload)
	if err != nil {
		result.Acceptable = false
		result.Errors = append(result.Errors, "payload_not_encodable")
		return result
	}
	result.PlanFingerprint = fingerprint
	if payload.HumanReview.Required {
		result.ReviewRequired = true
		for _, reason := range payload.HumanReview.Reasons {
			result.ReviewReasons = appendUniqueString(result.ReviewReasons, reason)
		}
	}
	validateFinalSummaryContract(payload.FinalSummaryContract, &result)
	if len(payload.Tasks) == 0 {
		result.Errors = append(result.Errors, "missing_tasks")
	}
	keys := make(map[string]struct{}, len(payload.Tasks))
	keyCounts := make(map[string]int, len(payload.Tasks))
	for _, task := range payload.Tasks {
		key := strings.TrimSpace(task.PlannedTaskKey)
		if key == "" {
			result.Errors = append(result.Errors, "missing_planned_task_key")
		} else if key != task.PlannedTaskKey {
			result.Errors = append(result.Errors, "non_canonical_planned_task_key:"+key)
		} else {
			keyCounts[key]++
			if keyCounts[key] > 1 {
				result.Errors = append(result.Errors, "duplicate_planned_task_key:"+key)
			}
			keys[key] = struct{}{}
		}
		employeeID, err := uuid.Parse(strings.TrimSpace(task.SelectedEmployeeID))
		if err != nil {
			result.Errors = append(result.Errors, "invalid_selected_employee_id:"+key)
		} else if employeeID == uuid.Nil {
			result.Errors = append(result.Errors, "missing_selected_employee_id:"+key)
		}
		if len(nonEmptyPlanRevisionStrings(task.ExpectedOutputs)) == 0 {
			result.Errors = append(result.Errors, "missing_expected_outputs:"+key)
		}
		if len(nonEmptyPlanRevisionStrings(task.AcceptanceCriteria)) == 0 {
			result.Errors = append(result.Errors, "missing_acceptance_criteria:"+key)
		}
		if task.HumanReviewRequired {
			result.ReviewRequired = true
			result.ReviewReasons = appendUniqueString(result.ReviewReasons, "task_requires_human_approval:"+key)
		}
		if isHighRiskLevel(task.RiskLevel) {
			result.ReviewRequired = true
			result.ReviewReasons = appendUniqueString(result.ReviewReasons, "high_risk_task:"+key)
		}
	}
	for _, task := range payload.Tasks {
		taskKey := strings.TrimSpace(task.PlannedTaskKey)
		for _, dep := range task.DependsOn {
			depKey := strings.TrimSpace(dep)
			if depKey != dep {
				result.Errors = append(result.Errors, "non_canonical_dependency:"+taskKey+"->"+depKey)
				continue
			}
			if _, ok := keys[depKey]; !ok {
				result.Errors = append(result.Errors, "unknown_dependency:"+taskKey+"->"+depKey)
			}
		}
	}
	if planRevisionPayloadHasCycle(payload.Tasks) {
		result.Errors = append(result.Errors, "cycle_detected")
	}
	if len(result.Errors) > 0 {
		result.Acceptable = false
	}
	return result
}

func PlanRevisionPayloadToPlannedTasks(payload PlanRevisionPayload) []PlannedTask {
	tasks := make([]PlannedTask, 0, len(payload.Tasks))
	for _, task := range payload.Tasks {
		handoffContract := clonePlanRevisionAnyMap(task.HandoffContract)
		if handoffContract == nil {
			handoffContract = map[string]any{}
		}
		if len(task.AcceptanceCriteria) > 0 {
			if _, ok := handoffContract["acceptance_criteria"]; !ok {
				handoffContract["acceptance_criteria"] = clonePlanRevisionStringSlice(task.AcceptanceCriteria)
			}
		}
		inputRequirements := clonePlanRevisionAnyMap(task.InputRequirements)
		if inputRequirements == nil {
			inputRequirements = map[string]any{}
			if len(task.InputContextRefs) > 0 {
				inputRequirements["input_context_refs"] = clonePlanRevisionStringSlice(task.InputContextRefs)
			}
		}
		employeeID, _ := uuid.Parse(strings.TrimSpace(task.SelectedEmployeeID))
		tasks = append(tasks, PlannedTask{
			Key:                         task.PlannedTaskKey,
			Title:                       task.Title,
			Summary:                     task.Objective,
			SelectedEmployeeID:          employeeID,
			EmployeeSelectionReason:     task.EmployeeSelectionReason,
			RequiredCapabilities:        clonePlanRevisionStringSlice(task.RequiredCapabilities),
			MatchedCapabilities:         clonePlanRevisionStringSlice(task.MatchedCapabilities),
			MissingCapabilities:         clonePlanRevisionStringSlice(task.MissingCapabilities),
			PermissionRequirements:      clonePlanRevisionStringSlice(task.PermissionRequirements),
			ToolRequirements:            clonePlanRevisionStringSlice(task.ToolRequirements),
			RuntimeRequirements:         clonePlanRevisionStringSlice(task.RuntimeRequirements),
			VerificationRequirements:    clonePlanRevisionStringSlice(task.VerificationRequirements),
			SelectionScore:              task.SelectionScore,
			SelectionConfidence:         task.SelectionConfidence,
			PlanningProfileSnapshotHash: task.PlanningProfileSnapshotHash,
			TaskKind:                    task.TaskType,
			RiskLevel:                   task.RiskLevel,
			RequiresHumanApproval:       task.HumanReviewRequired,
			ExpectedOutputs:             clonePlanRevisionStringSlice(task.ExpectedOutputs),
			Produces:                    clonePlanRevisionStringSlice(task.Produces),
			InputRequirements:           inputRequirements,
			HandoffContract:             handoffContract,
			BlockedByKeys:               clonePlanRevisionStringSlice(task.DependsOn),
		})
	}
	return tasks
}

func canonicalPlanRevisionPayload(payload PlanRevisionPayload) PlanRevisionPayload {
	canonical := PlanRevisionPayload{
		Summary:         payload.Summary,
		TemplateKey:     payload.TemplateKey,
		TemplateVersion: payload.TemplateVersion,
		ExitDeliverable: payload.ExitDeliverable,
		// AvailableExits and ConstraintNotes are server-authored annotations, not
		// planner decisions: they must not perturb the plan fingerprint, so they are
		// deliberately excluded from the canonical form below.
		Assumptions: sortedPlanRevisionStrings(payload.Assumptions),
		RiskAssessment: PlanRevisionRiskAssessment{
			HighestRiskLevel: payload.RiskAssessment.HighestRiskLevel,
			HighRiskTaskKeys: sortedPlanRevisionStrings(payload.RiskAssessment.HighRiskTaskKeys),
		},
		HumanReview: PlanRevisionHumanReview{
			Required: payload.HumanReview.Required,
			Reasons:  sortedPlanRevisionStrings(payload.HumanReview.Reasons),
		},
		Tasks:                  make([]PlanRevisionTask, 0, len(payload.Tasks)),
		PlanAcceptanceCriteria: canonicalPlanAcceptanceCriteria(payload.PlanAcceptanceCriteria),
		FinalSummaryContract: PlanRevisionFinalSummaryContract{
			RequiredSections: sortedPlanRevisionStrings(payload.FinalSummaryContract.RequiredSections),
		},
	}
	for _, task := range payload.Tasks {
		canonical.Tasks = append(canonical.Tasks, canonicalPlanRevisionTask(task))
	}
	sortPlanRevisionTasks(canonical.Tasks)
	return canonical
}

// canonicalPlanAcceptanceCriteria copies plan-level acceptance criteria into a
// deterministic, order-independent form (sorted by ID, SatisfiedBy sorted) so
// CanonicalPlanFingerprint is sensitive to every criterion field a human
// reviewer approved, including verification_method and severity, without
// being perturbed by planner output ordering.
func canonicalPlanAcceptanceCriteria(criteria []PlanAcceptanceCriterion) []PlanAcceptanceCriterion {
	canonical := make([]PlanAcceptanceCriterion, 0, len(criteria))
	for _, criterion := range criteria {
		canonical = append(canonical, PlanAcceptanceCriterion{
			ID:                 criterion.ID,
			Statement:          criterion.Statement,
			SatisfiedBy:        sortedPlanRevisionStrings(criterion.SatisfiedBy),
			VerificationMethod: criterion.VerificationMethod,
			Severity:           criterion.Severity,
			EvidenceHint:       criterion.EvidenceHint,
			AmbiguityFlag:      criterion.AmbiguityFlag,
		})
	}
	sortPlanAcceptanceCriteria(canonical)
	return canonical
}

func sortPlanAcceptanceCriteria(criteria []PlanAcceptanceCriterion) {
	for i := 1; i < len(criteria); i++ {
		current := criteria[i]
		j := i - 1
		for j >= 0 && current.ID < criteria[j].ID {
			criteria[j+1] = criteria[j]
			j--
		}
		criteria[j+1] = current
	}
}

func canonicalPlanRevisionTask(task PlanRevisionTask) PlanRevisionTask {
	return PlanRevisionTask{
		PlannedTaskKey:              task.PlannedTaskKey,
		Title:                       task.Title,
		Objective:                   task.Objective,
		TaskType:                    task.TaskType,
		SelectedEmployeeID:          task.SelectedEmployeeID,
		EmployeeSelectionReason:     task.EmployeeSelectionReason,
		RequiredCapabilities:        sortedPlanRevisionStrings(task.RequiredCapabilities),
		MatchedCapabilities:         sortedPlanRevisionStrings(task.MatchedCapabilities),
		MissingCapabilities:         sortedPlanRevisionStrings(task.MissingCapabilities),
		PermissionRequirements:      sortedPlanRevisionStrings(task.PermissionRequirements),
		ToolRequirements:            sortedPlanRevisionStrings(task.ToolRequirements),
		RuntimeRequirements:         sortedPlanRevisionStrings(task.RuntimeRequirements),
		InputContextRefs:            sortedPlanRevisionStrings(task.InputContextRefs),
		InputRequirements:           canonicalPlanRevisionMapPreserveArrays(task.InputRequirements),
		Produces:                    sortedPlanRevisionStrings(task.Produces),
		ExpectedOutputs:             sortedPlanRevisionStrings(task.ExpectedOutputs),
		AcceptanceCriteria:          sortedPlanRevisionStrings(task.AcceptanceCriteria),
		VerificationRequirements:    sortedPlanRevisionStrings(task.VerificationRequirements),
		DependsOn:                   sortedPlanRevisionStrings(task.DependsOn),
		RiskLevel:                   task.RiskLevel,
		HumanReviewRequired:         task.HumanReviewRequired,
		SelectionScore:              task.SelectionScore,
		SelectionConfidence:         task.SelectionConfidence,
		PlanningProfileSnapshotHash: task.PlanningProfileSnapshotHash,
		HandoffContract:             canonicalPlanRevisionMapPreserveArrays(task.HandoffContract),
	}
}

func sortPlanRevisionTasks(tasks []PlanRevisionTask) {
	for i := 1; i < len(tasks); i++ {
		current := tasks[i]
		j := i - 1
		for j >= 0 && planRevisionTaskLess(current, tasks[j]) {
			tasks[j+1] = tasks[j]
			j--
		}
		tasks[j+1] = current
	}
}

func planRevisionTaskLess(left, right PlanRevisionTask) bool {
	if left.PlannedTaskKey != right.PlannedTaskKey {
		return left.PlannedTaskKey < right.PlannedTaskKey
	}
	if left.Title != right.Title {
		return left.Title < right.Title
	}
	return left.Objective < right.Objective
}

func validateFinalSummaryContract(contract PlanRevisionFinalSummaryContract, result *PlanRevisionPayloadValidationResult) {
	sections := map[string]struct{}{}
	for _, section := range contract.RequiredSections {
		normalized := strings.TrimSpace(section)
		if normalized != "" {
			sections[normalized] = struct{}{}
		}
	}
	for _, required := range defaultFinalSummaryRequiredSections {
		if _, ok := sections[required]; !ok {
			result.Errors = append(result.Errors, "missing_final_summary_required_section:"+required)
		}
	}
}

func planRevisionPayloadHasCycle(tasks []PlanRevisionTask) bool {
	dependencies := make(map[string][]string, len(tasks))
	for _, task := range tasks {
		key := strings.TrimSpace(task.PlannedTaskKey)
		if key == "" {
			continue
		}
		dependencies[key] = clonePlanRevisionStringSlice(task.DependsOn)
	}
	state := map[string]int{}
	var visit func(string) bool
	visit = func(key string) bool {
		switch state[key] {
		case 1:
			return true
		case 2:
			return false
		}
		state[key] = 1
		for _, dep := range dependencies[key] {
			depKey := strings.TrimSpace(dep)
			if _, ok := dependencies[depKey]; !ok {
				continue
			}
			if visit(depKey) {
				return true
			}
		}
		state[key] = 2
		return false
	}
	for key := range dependencies {
		if visit(key) {
			return true
		}
	}
	return false
}

func acceptanceCriteriaFromHandoffContract(contract map[string]any, expectedOutputs []string) []string {
	switch value := contract["acceptance_criteria"].(type) {
	case []string:
		if criteria := nonEmptyPlanRevisionStrings(value); len(criteria) > 0 {
			return criteria
		}
	case []any:
		if criteria := planRevisionStringSliceFromAny(value); len(criteria) > 0 {
			return criteria
		}
	case string:
		if criteria := nonEmptyPlanRevisionStrings([]string{value}); len(criteria) > 0 {
			return criteria
		}
	}
	return clonePlanRevisionStringSlice(expectedOutputs)
}

func inputContextRefsFromRequirements(requirements map[string]any) []string {
	if requirements == nil {
		return []string{}
	}
	refs := []string{}
	for _, key := range []string{"input_context_refs", "context_refs", "required_context", "required_context_refs"} {
		refs = append(refs, planRevisionStringSliceFromAny(requirements[key])...)
	}
	return refs
}

func planRevisionStringSliceFromAny(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case string:
		return nonEmptyPlanRevisionStrings([]string{typed})
	case []string:
		return nonEmptyPlanRevisionStrings(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if stringItem, ok := item.(string); ok {
				values = append(values, stringItem)
			}
		}
		return nonEmptyPlanRevisionStrings(values)
	default:
		return nil
	}
}

func nonEmptyPlanRevisionStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized != "" {
			result = append(result, normalized)
		}
	}
	return result
}

func clonePlanRevisionStringSlice(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string(nil), values...)
}

func sortedPlanRevisionStrings(values []string) []string {
	cloned := clonePlanRevisionStringSlice(values)
	for i := 1; i < len(cloned); i++ {
		current := cloned[i]
		j := i - 1
		for j >= 0 && current < cloned[j] {
			cloned[j+1] = cloned[j]
			j--
		}
		cloned[j+1] = current
	}
	return cloned
}

func clonePlanRevisionAnyMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = clonePlanRevisionAny(value)
	}
	return cloned
}

func canonicalPlanRevisionMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	canonical := make(map[string]any, len(values))
	for key, value := range values {
		canonical[key] = canonicalPlanRevisionAny(value)
	}
	return canonical
}

func canonicalPlanRevisionMapPreserveArrays(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	canonical := make(map[string]any, len(values))
	for key, value := range values {
		canonical[key] = canonicalPlanRevisionAnyPreserveArrays(value)
	}
	return canonical
}

func clonePlanRevisionAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return clonePlanRevisionAnyMap(typed)
	case []string:
		return clonePlanRevisionStringSlice(typed)
	case []any:
		cloned := make([]any, 0, len(typed))
		for _, item := range typed {
			cloned = append(cloned, clonePlanRevisionAny(item))
		}
		return cloned
	default:
		return value
	}
}

func canonicalPlanRevisionAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return canonicalPlanRevisionMap(typed)
	case []string:
		return sortedPlanRevisionStrings(typed)
	case []any:
		values := make([]any, 0, len(typed))
		allStrings := true
		stringValues := make([]string, 0, len(typed))
		for _, item := range typed {
			canonicalItem := canonicalPlanRevisionAny(item)
			values = append(values, canonicalItem)
			stringItem, ok := canonicalItem.(string)
			if !ok {
				allStrings = false
			}
			if ok {
				stringValues = append(stringValues, stringItem)
			}
		}
		if allStrings {
			sorted := sortedPlanRevisionStrings(stringValues)
			result := make([]any, 0, len(sorted))
			for _, item := range sorted {
				result = append(result, item)
			}
			return result
		}
		return values
	default:
		return value
	}
}

func canonicalPlanRevisionAnyPreserveArrays(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return canonicalPlanRevisionMapPreserveArrays(typed)
	case []string:
		return clonePlanRevisionStringSlice(typed)
	case []any:
		values := make([]any, 0, len(typed))
		for _, item := range typed {
			values = append(values, canonicalPlanRevisionAnyPreserveArrays(item))
		}
		return values
	default:
		return value
	}
}

func isHighRiskLevel(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high", "critical":
		return true
	default:
		return false
	}
}

func higherRiskLevel(left, right string) string {
	if riskRank(right) > riskRank(left) {
		return strings.ToLower(strings.TrimSpace(right))
	}
	return left
}

func riskRank(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low", "normal":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	case "critical":
		return 4
	default:
		return 0
	}
}

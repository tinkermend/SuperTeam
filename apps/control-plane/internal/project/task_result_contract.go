package project

import (
	"fmt"
	"strings"
)

type TaskResultStatus string

const (
	TaskResultStatusCompleted      TaskResultStatus = "completed"
	TaskResultStatusRevisionNeeded TaskResultStatus = "revision_needed"
	TaskResultStatusBlocked        TaskResultStatus = "blocked"
	TaskResultStatusFailed         TaskResultStatus = "failed"
	TaskResultStatusCancelled      TaskResultStatus = "cancelled"
)

type TaskResultCriterionStatus string

const (
	TaskResultCriterionStatusPassed          TaskResultCriterionStatus = "passed"
	TaskResultCriterionStatusFailed          TaskResultCriterionStatus = "failed"
	TaskResultCriterionStatusNeedsHuman      TaskResultCriterionStatus = "needs_human"
	TaskResultCriterionStatusNotApplicable   TaskResultCriterionStatus = "not_applicable"
	TaskResultCriterionStatusHumanOverridden TaskResultCriterionStatus = "human_overridden"

	TaskResultCriterionPassed          = TaskResultCriterionStatusPassed
	TaskResultCriterionFailed          = TaskResultCriterionStatusFailed
	TaskResultCriterionNeedsHuman      = TaskResultCriterionStatusNeedsHuman
	TaskResultCriterionNotApplicable   = TaskResultCriterionStatusNotApplicable
	TaskResultCriterionHumanOverridden = TaskResultCriterionStatusHumanOverridden
)

type TaskResultVerificationStatus string

const (
	TaskResultVerificationStatusPassed  TaskResultVerificationStatus = "passed"
	TaskResultVerificationStatusFailed  TaskResultVerificationStatus = "failed"
	TaskResultVerificationStatusSkipped TaskResultVerificationStatus = "skipped"

	TaskResultVerificationPassed  = TaskResultVerificationStatusPassed
	TaskResultVerificationFailed  = TaskResultVerificationStatusFailed
	TaskResultVerificationSkipped = TaskResultVerificationStatusSkipped
)

type TaskResultDecision string

const (
	TaskResultDecisionValidationFailed    TaskResultDecision = "validation_failed"
	TaskResultDecisionCompleteAccepted    TaskResultDecision = "complete_accepted"
	TaskResultDecisionWaitingHumanReview  TaskResultDecision = "waiting_human_review"
	TaskResultDecisionRevisionAttempt     TaskResultDecision = "revision_attempt"
	TaskResultDecisionRevisionTask        TaskResultDecision = "revision_task"
	TaskResultDecisionBlockedWaitingHuman TaskResultDecision = "blocked_waiting_human"
	TaskResultDecisionFailedRetryable     TaskResultDecision = "failed_retryable"
	TaskResultDecisionFailedRecovery      TaskResultDecision = "failed_recovery"
	TaskResultDecisionCancelledTerminal   TaskResultDecision = "cancelled_terminal"
	TaskResultDecisionReplanRequested     TaskResultDecision = "replan_requested"
)

type TaskResultContract struct {
	Status             TaskResultStatus              `json:"status"`
	Summary            string                        `json:"summary"`
	AcceptanceResults  []TaskResultAcceptanceResult  `json:"acceptance_results,omitempty"`
	EvidenceRefs       []TaskResultRef               `json:"evidence_refs,omitempty"`
	ArtifactRefs       []TaskResultRef               `json:"artifact_refs,omitempty"`
	ChangesMade        []TaskResultChange            `json:"changes_made,omitempty"`
	Verification       []TaskResultVerification      `json:"verification,omitempty"`
	Risks              []TaskResultRisk              `json:"risks,omitempty"`
	FollowUpRequests   []TaskResultFollowUpRequest   `json:"follow_up_requests,omitempty"`
	HumanReviewRequest *TaskResultHumanReviewRequest `json:"human_review_request,omitempty"`
	RevisionRequest    *TaskResultRevisionRequest    `json:"revision_request,omitempty"`
	Blocker            *TaskResultBlocker            `json:"blocker,omitempty"`
	Failure            *TaskResultFailure            `json:"failure,omitempty"`
	ReplanRequest      *TaskResultReplanRequest      `json:"replan_request,omitempty"`
	Cancellation       *TaskResultCancellation       `json:"cancellation,omitempty"`
}

type TaskResultAcceptanceResult struct {
	ID                  string                    `json:"id,omitempty"`
	Criterion           string                    `json:"criterion,omitempty"`
	CriterionID         string                    `json:"criterion_id,omitempty"`
	Name                string                    `json:"name,omitempty"`
	Status              TaskResultCriterionStatus `json:"status"`
	Summary             string                    `json:"summary,omitempty"`
	EvidenceRefs        []string                  `json:"evidence_refs,omitempty"`
	HumanAcceptedReason string                    `json:"human_accepted_reason,omitempty"`
}

type TaskResultRef struct {
	ID       string         `json:"id,omitempty"`
	Kind     string         `json:"kind,omitempty"`
	Type     string         `json:"type,omitempty"`
	Ref      string         `json:"ref,omitempty"`
	URI      string         `json:"uri,omitempty"`
	URL      string         `json:"url,omitempty"`
	Title    string         `json:"title,omitempty"`
	Summary  string         `json:"summary,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type TaskResultChange struct {
	Type         string          `json:"type,omitempty"`
	Ref          string          `json:"ref,omitempty"`
	Summary      string          `json:"summary,omitempty"`
	Files        []string        `json:"files,omitempty"`
	ArtifactRefs []TaskResultRef `json:"artifact_refs,omitempty"`
}

type TaskResultVerification struct {
	Status       TaskResultVerificationStatus `json:"status"`
	Type         string                       `json:"type,omitempty"`
	Ref          string                       `json:"ref,omitempty"`
	Summary      string                       `json:"summary,omitempty"`
	Method       string                       `json:"method,omitempty"`
	EvidenceRefs []TaskResultRef              `json:"evidence_refs,omitempty"`
}

type TaskResultRisk struct {
	Summary             string `json:"summary,omitempty"`
	Description         string `json:"description,omitempty"`
	Severity            string `json:"severity,omitempty"`
	Level               string `json:"level,omitempty"`
	Mitigation          string `json:"mitigation,omitempty"`
	RequiresHumanReview bool   `json:"requires_human_review,omitempty"`
}

type TaskResultFollowUpRequest struct {
	Type               string          `json:"type,omitempty"`
	Summary            string          `json:"summary,omitempty"`
	RequiredBy         string          `json:"required_by,omitempty"`
	MissingInformation []TaskResultRef `json:"missing_information,omitempty"`
}

type TaskResultHumanReviewRequest struct {
	Reason                     string   `json:"reason,omitempty"`
	Prompt                     string   `json:"prompt,omitempty"`
	Options                    []string `json:"options,omitempty"`
	RequiredBy                 string   `json:"required_by,omitempty"`
	ReviewType                 string   `json:"review_type,omitempty"`
	SuggestedResolutionOptions []string `json:"suggested_resolution_options,omitempty"`
}

type TaskResultRevisionRequest struct {
	Reason                 string   `json:"reason,omitempty"`
	RecommendedTaskTitle   string   `json:"recommended_task_title,omitempty"`
	RecommendedTaskSummary string   `json:"recommended_task_summary,omitempty"`
	ContractChanged        bool     `json:"contract_changed,omitempty"`
	RequestedChanges       []string `json:"requested_changes,omitempty"`
}

type TaskResultBlocker struct {
	Reason           string          `json:"reason,omitempty"`
	ResolutionPrompt string          `json:"resolution_prompt,omitempty"`
	RequiredBy       string          `json:"required_by,omitempty"`
	ContextRefs      []TaskResultRef `json:"context_refs,omitempty"`
}

type TaskResultFailure struct {
	ErrorFamily            string `json:"error_family,omitempty"`
	Retryable              *bool  `json:"retryable,omitempty"`
	RecoveryRecommendation string `json:"recovery_recommendation,omitempty"`
	Message                string `json:"message,omitempty"`
}

type TaskResultReplanRequest struct {
	Reason      string   `json:"reason,omitempty"`
	Scope       string   `json:"scope,omitempty"`
	Constraints []string `json:"constraints,omitempty"`
}

type TaskResultCancellation struct {
	Reason      string `json:"reason,omitempty"`
	CancelledBy string `json:"cancelled_by,omitempty"`
}

type TaskResultValidation struct {
	Valid    bool                        `json:"valid"`
	Decision TaskResultDecision          `json:"decision"`
	Errors   []TaskResultValidationError `json:"errors,omitempty"`
	Warnings []string                    `json:"warnings,omitempty"`
}

type TaskResultValidationError = string

type TaskResultValidationResult = TaskResultValidation

func ValidateTaskResultContract(task ProjectTask, result TaskResultContract) TaskResultValidation {
	validation := TaskResultValidation{Valid: true}

	if !validTaskResultStatus(result.Status) {
		validation.Errors = append(validation.Errors, "status_invalid:"+string(result.Status))
	}
	if strings.TrimSpace(result.Summary) == "" {
		validation.Errors = append(validation.Errors, "summary_required")
	}
	validation.Errors = append(validation.Errors, taskResultRefBlankErrors("evidence_ref_blank", result.EvidenceRefs)...)
	validation.Errors = append(validation.Errors, taskResultRefBlankErrors("artifact_ref_blank", result.ArtifactRefs)...)
	validation.Errors = append(validation.Errors, validateTaskResultVerifications(task, result.Status, result.Verification)...)

	switch result.Status {
	case TaskResultStatusCompleted:
		validation.Errors = append(validation.Errors, validateCompletedTaskResult(task, result)...)
	case TaskResultStatusRevisionNeeded:
		if result.RevisionRequest == nil || strings.TrimSpace(result.RevisionRequest.Reason) == "" {
			validation.Errors = append(validation.Errors, "revision_reason_required")
		}
	case TaskResultStatusBlocked:
		if result.Blocker == nil {
			validation.Errors = append(validation.Errors, "blocker_reason_required", "blocker_required_by_required")
			break
		}
		if strings.TrimSpace(result.Blocker.Reason) == "" {
			validation.Errors = append(validation.Errors, "blocker_reason_required")
		}
		if strings.TrimSpace(result.Blocker.RequiredBy) == "" {
			validation.Errors = append(validation.Errors, "blocker_required_by_required")
		}
	case TaskResultStatusFailed:
		if result.Failure == nil {
			validation.Errors = append(validation.Errors, "failure_error_family_required", "failure_retryable_required", "failure_recovery_recommendation_required")
			break
		}
		if strings.TrimSpace(result.Failure.ErrorFamily) == "" {
			validation.Errors = append(validation.Errors, "failure_error_family_required")
		}
		if result.Failure.Retryable == nil {
			validation.Errors = append(validation.Errors, "failure_retryable_required")
		}
		if strings.TrimSpace(result.Failure.RecoveryRecommendation) == "" {
			validation.Errors = append(validation.Errors, "failure_recovery_recommendation_required")
		}
	case TaskResultStatusCancelled:
		if result.Cancellation == nil || strings.TrimSpace(result.Cancellation.Reason) == "" {
			validation.Errors = append(validation.Errors, "cancellation_reason_required")
		}
	}

	if len(validation.Errors) > 0 {
		validation.Valid = false
		validation.Decision = TaskResultDecisionValidationFailed
		return validation
	}

	validation.Decision = mapTaskResultDecision(task, result)
	return validation
}

func TaskResultContractFromLegacyCompletion(req CompleteProjectTaskAttemptRequest) TaskResultContract {
	result := TaskResultContract{
		Status:       TaskResultStatusCompleted,
		Summary:      strings.TrimSpace(req.Conclusion),
		EvidenceRefs: taskResultRefsFromAny(req.EvidenceRefs, "evidence"),
		ArtifactRefs: taskResultRefsFromAny(req.ArtifactRefs, "artifact"),
	}
	if nextAction := strings.TrimSpace(req.RecommendedNextAction); nextAction != "" {
		result.FollowUpRequests = append(result.FollowUpRequests, TaskResultFollowUpRequest{
			Summary:            nextAction,
			RequiredBy:         "human",
			MissingInformation: taskResultRefsFromAny(req.MissingInformation, "missing_information"),
		})
	}
	if req.RequiresHumanReview {
		result.HumanReviewRequest = &TaskResultHumanReviewRequest{
			Reason:     "legacy_completion_requires_human_review",
			Prompt:     "Review legacy completion result",
			RequiredBy: "human",
		}
	}
	return result
}

func AdaptCompletionEvidenceToResultContract(req CompleteProjectTaskAttemptRequest) TaskResultContract {
	return TaskResultContractFromLegacyCompletion(req)
}

func TaskResultContractFromFailure(req FailProjectTaskAttemptRequest) TaskResultContract {
	retryable := retryableFailureFamily(req.FailureFamily)
	if req.Retryable != nil {
		retryable = *req.Retryable
	}
	result := TaskResultContract{
		Status:  TaskResultStatusFailed,
		Summary: strings.TrimSpace(req.FailureSummary),
		Failure: &TaskResultFailure{
			ErrorFamily: strings.TrimSpace(req.FailureFamily),
			Retryable:   &retryable,
		},
	}
	if retryable {
		result.Failure.RecoveryRecommendation = "retry_original_attempt"
	} else {
		result.Failure.RecoveryRecommendation = "manual_recovery_required"
	}
	return result
}

func TaskResultContractFromWaitHuman(req WaitHumanProjectTaskAttemptRequest) TaskResultContract {
	summary := strings.TrimSpace(req.Summary)
	if summary == "" {
		summary = strings.TrimSpace(req.Reason)
	}
	reason := strings.TrimSpace(req.Reason)
	return TaskResultContract{
		Status:  TaskResultStatusBlocked,
		Summary: summary,
		Blocker: &TaskResultBlocker{
			Reason:           reason,
			ResolutionPrompt: reason,
			RequiredBy:       "human",
			ContextRefs:      taskResultRefsFromAny(req.MissingContextRefs, "missing_context"),
		},
		HumanReviewRequest: &TaskResultHumanReviewRequest{
			Reason:                     reason,
			Prompt:                     reason,
			Options:                    req.SuggestedResolutionOptions,
			RequiredBy:                 "human",
			SuggestedResolutionOptions: req.SuggestedResolutionOptions,
		},
	}
}

func validTaskResultStatus(status TaskResultStatus) bool {
	switch status {
	case TaskResultStatusCompleted,
		TaskResultStatusRevisionNeeded,
		TaskResultStatusBlocked,
		TaskResultStatusFailed,
		TaskResultStatusCancelled:
		return true
	default:
		return false
	}
}

func validateCompletedTaskResult(task ProjectTask, result TaskResultContract) []string {
	var errors []string
	requiredOutputs := stringSetFromAny(task.ExpectedOutputs)
	if requiredOutputs["evidence_refs"] && !hasUsableTaskResultRef(result.EvidenceRefs) {
		errors = append(errors, "expected_output_missing:evidence_refs")
	}
	if requiredOutputs["artifact_refs"] && !hasUsableTaskResultRef(result.ArtifactRefs) {
		errors = append(errors, "expected_output_missing:artifact_refs")
	}
	if requiredOutputs["verification"] && len(result.Verification) == 0 {
		errors = append(errors, "expected_output_missing:verification")
	}

	for _, criterion := range requiredAcceptanceCriteria(task.HandoffContract) {
		acceptanceResult, ok := acceptanceResultForCriterion(result.AcceptanceResults, criterion)
		if !ok {
			errors = append(errors, "acceptance_result_missing:"+criterion)
			continue
		}
		errors = append(errors, validateCompletedAcceptanceResult(criterion, acceptanceResult)...)
	}
	return errors
}

func validateTaskResultVerifications(task ProjectTask, status TaskResultStatus, verifications []TaskResultVerification) []string {
	var errors []string
	requiresRuntimeAttestation := boolFromTaskContract(task.HandoffContract, "requires_runtime_attestation")
	for _, verification := range verifications {
		switch verification.Status {
		case TaskResultVerificationStatusPassed, TaskResultVerificationStatusSkipped:
		case TaskResultVerificationStatusFailed:
			if status == TaskResultStatusCompleted {
				errors = append(errors, "verification_failed")
			}
		default:
			errors = append(errors, "verification_status_invalid:"+string(verification.Status))
		}
		if status == TaskResultStatusCompleted && verification.Status == TaskResultVerificationStatusPassed && requiresRuntimeAttestation && !verificationHasAttestationRef(verification) {
			errors = append(errors, "verification_attestation_ref_required")
		}
	}
	return errors
}

func verificationHasAttestationRef(verification TaskResultVerification) bool {
	for _, ref := range verification.EvidenceRefs {
		if strings.EqualFold(strings.TrimSpace(ref.Kind), "attestation") ||
			strings.EqualFold(strings.TrimSpace(ref.Type), "attestation") ||
			strings.HasPrefix(strings.TrimSpace(ref.Ref), "attestation:") {
			return usableTaskResultRef(ref)
		}
	}
	return false
}

func boolFromTaskContract(contract map[string]any, key string) bool {
	value, ok := contract[key]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func validateCompletedAcceptanceResult(criterion string, result TaskResultAcceptanceResult) []string {
	switch result.Status {
	case TaskResultCriterionStatusPassed, TaskResultCriterionStatusHumanOverridden:
	case TaskResultCriterionStatusNotApplicable:
		if strings.TrimSpace(result.HumanAcceptedReason) == "" {
			return []string{"acceptance_result_human_reason_required:" + criterion}
		}
	default:
		return []string{"acceptance_result_not_accepted:" + criterion}
	}
	var errors []string
	if hasBlankStringRef(result.EvidenceRefs) {
		errors = append(errors, "acceptance_result_evidence_blank:"+criterion)
	}
	if !hasUsableStringRef(result.EvidenceRefs) {
		errors = append(errors, "acceptance_result_evidence_missing:"+criterion)
	}
	return errors
}

func retryableFailureFamily(family string) bool {
	switch strings.TrimSpace(family) {
	case FailureFamilyTransientRuntime, FailureFamilyTransientProvider, FailureFamilyTimeout:
		return true
	default:
		return false
	}
}

func hasUsableTaskResultRef(refs []TaskResultRef) bool {
	for _, ref := range refs {
		if usableTaskResultRef(ref) {
			return true
		}
	}
	return false
}

func usableTaskResultRef(ref TaskResultRef) bool {
	return strings.TrimSpace(ref.Ref) != "" ||
		strings.TrimSpace(ref.URI) != "" ||
		strings.TrimSpace(ref.URL) != "" ||
		strings.TrimSpace(ref.ID) != ""
}

func taskResultRefBlankErrors(code string, refs []TaskResultRef) []string {
	var errors []string
	for _, ref := range refs {
		if !usableTaskResultRef(ref) {
			errors = append(errors, code)
		}
	}
	return errors
}

func hasUsableStringRef(refs []string) bool {
	for _, ref := range refs {
		if strings.TrimSpace(ref) != "" {
			return true
		}
	}
	return false
}

func hasBlankStringRef(refs []string) bool {
	for _, ref := range refs {
		if strings.TrimSpace(ref) == "" {
			return true
		}
	}
	return false
}

func requiredAcceptanceCriteria(contract map[string]any) []string {
	if len(contract) == 0 {
		return nil
	}
	return stringsFromAny(contract["acceptance_criteria"])
}

func acceptanceResultForCriterion(results []TaskResultAcceptanceResult, criterion string) (TaskResultAcceptanceResult, bool) {
	for _, result := range results {
		if matchesCriterion(result, criterion) {
			return result, true
		}
	}
	return TaskResultAcceptanceResult{}, false
}

func matchesCriterion(result TaskResultAcceptanceResult, criterion string) bool {
	candidates := []string{result.Criterion, result.CriterionID, result.ID, result.Name}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) == criterion {
			return true
		}
	}
	return false
}

func mapTaskResultDecision(task ProjectTask, result TaskResultContract) TaskResultDecision {
	switch result.Status {
	case TaskResultStatusCompleted:
		if taskResultNeedsHumanReview(task, result) {
			return TaskResultDecisionWaitingHumanReview
		}
		return TaskResultDecisionCompleteAccepted
	case TaskResultStatusRevisionNeeded:
		if result.RevisionRequest != nil && result.RevisionRequest.ContractChanged {
			return TaskResultDecisionRevisionTask
		}
		return TaskResultDecisionRevisionAttempt
	case TaskResultStatusBlocked:
		return TaskResultDecisionBlockedWaitingHuman
	case TaskResultStatusFailed:
		if result.ReplanRequest != nil {
			return TaskResultDecisionReplanRequested
		}
		if result.Failure != nil && result.Failure.Retryable != nil && *result.Failure.Retryable && taskResultRetryBudgetRemains(task) {
			return TaskResultDecisionFailedRetryable
		}
		return TaskResultDecisionFailedRecovery
	case TaskResultStatusCancelled:
		return TaskResultDecisionCancelledTerminal
	default:
		return TaskResultDecisionValidationFailed
	}
}

func NormalizeTaskResultDecision(task ProjectTask, result TaskResultContract) TaskResultDecision {
	return mapTaskResultDecision(task, result)
}

func taskResultNeedsHumanReview(task ProjectTask, result TaskResultContract) bool {
	if task.RequiresHumanApproval || result.HumanReviewRequest != nil {
		return true
	}
	for _, risk := range result.Risks {
		if risk.RequiresHumanReview || highRiskLevel(risk.Severity) || highRiskLevel(risk.Level) {
			return true
		}
	}
	if task.RiskLevel == nil {
		return false
	}
	return highRiskLevel(*task.RiskLevel)
}

func highRiskLevel(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high", "critical", "高", "高风险", "严重":
		return true
	default:
		return false
	}
}

func taskResultRetryBudgetRemains(task ProjectTask) bool {
	if task.MaxAttempts == nil {
		return true
	}
	return task.AttemptCount < *task.MaxAttempts
}

func taskResultRefsFromAny(values []any, defaultKind string) []TaskResultRef {
	refs := make([]TaskResultRef, 0, len(values))
	for _, value := range values {
		ref, ok := taskResultRefFromAny(value, defaultKind)
		if ok {
			refs = append(refs, ref)
		}
	}
	return refs
}

func taskResultRefFromAny(value any, defaultKind string) (TaskResultRef, bool) {
	switch typed := value.(type) {
	case nil:
		return TaskResultRef{}, false
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return TaskResultRef{}, false
		}
		return TaskResultRef{Kind: defaultKind, Type: defaultKind, Ref: text}, true
	case map[string]any:
		refType := firstNonBlankString(stringFromMap(typed, "type"), stringFromMap(typed, "kind"), defaultKind)
		ref := TaskResultRef{
			Kind:     firstNonBlankString(stringFromMap(typed, "kind"), stringFromMap(typed, "type"), defaultKind),
			Type:     refType,
			Ref:      firstNonBlankString(stringFromMap(typed, "ref"), stringFromMap(typed, "id"), stringFromMap(typed, "uri"), stringFromMap(typed, "url")),
			URI:      stringFromMap(typed, "uri"),
			URL:      stringFromMap(typed, "url"),
			Title:    firstNonBlankString(stringFromMap(typed, "title"), stringFromMap(typed, "name")),
			Summary:  stringFromMap(typed, "summary"),
			Metadata: typed,
		}
		ref.ID = stringFromMap(typed, "id")
		if strings.TrimSpace(ref.Ref) == "" {
			return TaskResultRef{}, false
		}
		return ref, true
	case map[string]string:
		refType := firstNonBlankString(typed["type"], typed["kind"], defaultKind)
		ref := TaskResultRef{
			ID:      strings.TrimSpace(typed["id"]),
			Kind:    firstNonBlankString(typed["kind"], typed["type"], defaultKind),
			Type:    refType,
			Ref:     firstNonBlankString(typed["ref"], typed["id"], typed["uri"], typed["url"]),
			URI:     strings.TrimSpace(typed["uri"]),
			URL:     strings.TrimSpace(typed["url"]),
			Title:   firstNonBlankString(typed["title"], typed["name"]),
			Summary: strings.TrimSpace(typed["summary"]),
		}
		if strings.TrimSpace(ref.Ref) == "" {
			return TaskResultRef{}, false
		}
		return ref, true
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			return TaskResultRef{}, false
		}
		return TaskResultRef{Kind: defaultKind, Type: defaultKind, Ref: text}, true
	}
}

func stringsFromAny(value any) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case []string:
		return nonBlankStrings(typed)
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, stringsFromAny(item)...)
		}
		return values
	case map[string]any:
		return stringsFromCriterionMap(typed)
	case map[string]string:
		return stringsFromCriterionStringMap(typed)
	default:
		text := strings.TrimSpace(fmt.Sprint(value))
		if text == "" {
			return nil
		}
		return []string{text}
	}
}

func stringsFromCriterionMap(value map[string]any) []string {
	if criterionRequired, ok := boolFromAny(value["required"]); ok && !criterionRequired {
		return nil
	}
	for _, key := range []string{"criterion", "name", "id", "key", "title"} {
		text := stringFromMap(value, key)
		if text != "" {
			return []string{text}
		}
	}
	return nil
}

func stringsFromCriterionStringMap(value map[string]string) []string {
	if required, ok := value["required"]; ok && strings.EqualFold(strings.TrimSpace(required), "false") {
		return nil
	}
	for _, key := range []string{"criterion", "name", "id", "key", "title"} {
		text := strings.TrimSpace(value[key])
		if text != "" {
			return []string{text}
		}
	}
	return nil
}

func nonBlankStrings(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		text := strings.TrimSpace(value)
		if text != "" {
			result = append(result, text)
		}
	}
	return result
}

func stringFromMap(values map[string]any, key string) string {
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func firstNonBlankString(values ...string) string {
	for _, value := range values {
		text := strings.TrimSpace(value)
		if text != "" {
			return text
		}
	}
	return ""
}

func boolFromAny(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		text := strings.ToLower(strings.TrimSpace(typed))
		if text == "true" {
			return true, true
		}
		if text == "false" {
			return false, true
		}
	}
	return false, false
}

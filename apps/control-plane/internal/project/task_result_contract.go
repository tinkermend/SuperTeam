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
)

type TaskResultVerificationStatus string

const (
	TaskResultVerificationStatusPassed  TaskResultVerificationStatus = "passed"
	TaskResultVerificationStatusFailed  TaskResultVerificationStatus = "failed"
	TaskResultVerificationStatusSkipped TaskResultVerificationStatus = "skipped"
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
	Status             TaskResultStatus
	Summary            string
	AcceptanceResults  []TaskResultAcceptanceResult
	EvidenceRefs       []TaskResultRef
	ArtifactRefs       []TaskResultRef
	Changes            []TaskResultChange
	Verification       TaskResultVerification
	Risks              []TaskResultRisk
	FollowUpRequests   []TaskResultFollowUpRequest
	HumanReviewRequest *TaskResultHumanReviewRequest
	RevisionRequest    *TaskResultRevisionRequest
	Blocker            *TaskResultBlocker
	Failure            *TaskResultFailure
	ReplanRequest      *TaskResultReplanRequest
	Cancellation       *TaskResultCancellation
}

type TaskResultAcceptanceResult struct {
	ID                  string
	Criterion           string
	CriterionID         string
	Name                string
	Status              TaskResultCriterionStatus
	Summary             string
	EvidenceRefs        []TaskResultRef
	HumanAcceptedReason string
}

type TaskResultRef struct {
	ID       string
	Kind     string
	Ref      string
	URI      string
	URL      string
	Title    string
	Metadata map[string]any
}

type TaskResultChange struct {
	Summary      string
	Files        []string
	ArtifactRefs []TaskResultRef
}

type TaskResultVerification struct {
	Status       TaskResultVerificationStatus
	Summary      string
	Method       string
	EvidenceRefs []TaskResultRef
}

type TaskResultRisk struct {
	Summary             string
	Severity            string
	Mitigation          string
	RequiresHumanReview bool
}

type TaskResultFollowUpRequest struct {
	Summary            string
	RequiredBy         string
	MissingInformation []TaskResultRef
}

type TaskResultHumanReviewRequest struct {
	Reason                     string
	RequiredBy                 string
	ReviewType                 string
	SuggestedResolutionOptions []string
}

type TaskResultRevisionRequest struct {
	Reason           string
	ContractChanged  bool
	RequestedChanges []string
}

type TaskResultBlocker struct {
	Reason      string
	RequiredBy  string
	ContextRefs []TaskResultRef
}

type TaskResultFailure struct {
	ErrorFamily            string
	Retryable              *bool
	RecoveryRecommendation string
	Message                string
}

type TaskResultReplanRequest struct {
	Reason string
	Scope  string
}

type TaskResultCancellation struct {
	Reason      string
	CancelledBy string
}

type TaskResultValidation struct {
	Valid    bool
	Decision TaskResultDecision
	Errors   []string
	Warnings []string
}

func ValidateTaskResultContract(task ProjectTask, result TaskResultContract) TaskResultValidation {
	validation := TaskResultValidation{Valid: true}

	if !validTaskResultStatus(result.Status) {
		validation.Errors = append(validation.Errors, "status_invalid:"+string(result.Status))
	}
	if strings.TrimSpace(result.Summary) == "" {
		validation.Errors = append(validation.Errors, "summary_required")
	}

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
			RequiredBy: "human",
		}
	}
	return result
}

func TaskResultContractFromFailure(req FailProjectTaskAttemptRequest) TaskResultContract {
	result := TaskResultContract{
		Status:  TaskResultStatusFailed,
		Summary: strings.TrimSpace(req.FailureSummary),
		Failure: &TaskResultFailure{
			ErrorFamily: strings.TrimSpace(req.FailureFamily),
			Retryable:   req.Retryable,
		},
	}
	if req.Retryable != nil && *req.Retryable {
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
			Reason:      reason,
			RequiredBy:  "human",
			ContextRefs: taskResultRefsFromAny(req.MissingContextRefs, "missing_context"),
		},
		HumanReviewRequest: &TaskResultHumanReviewRequest{
			Reason:                     reason,
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
	if requiredOutputs["evidence_refs"] && len(result.EvidenceRefs) == 0 {
		errors = append(errors, "expected_output_missing:evidence_refs")
	}
	if requiredOutputs["artifact_refs"] && len(result.ArtifactRefs) == 0 {
		errors = append(errors, "expected_output_missing:artifact_refs")
	}
	if requiredOutputs["verification"] && result.Verification.Status == "" {
		errors = append(errors, "expected_output_missing:verification")
	}
	if result.Verification.Status == TaskResultVerificationStatusFailed {
		errors = append(errors, "verification_failed")
	}

	for _, criterion := range requiredAcceptanceCriteria(task.HandoffContract) {
		acceptanceResult, ok := acceptanceResultForCriterion(result.AcceptanceResults, criterion)
		if !ok {
			errors = append(errors, "acceptance_result_missing:"+criterion)
			continue
		}
		errors = append(errors, validateCompletedAcceptanceResult(criterion, acceptanceResult, result.EvidenceRefs)...)
	}
	return errors
}

func validateCompletedAcceptanceResult(criterion string, result TaskResultAcceptanceResult, contractEvidenceRefs []TaskResultRef) []string {
	switch result.Status {
	case TaskResultCriterionStatusPassed, TaskResultCriterionStatusHumanOverridden:
	case TaskResultCriterionStatusNotApplicable:
		if strings.TrimSpace(result.HumanAcceptedReason) == "" {
			return []string{"acceptance_result_human_reason_required:" + criterion}
		}
	default:
		return []string{"acceptance_result_not_accepted:" + criterion}
	}
	if len(result.EvidenceRefs) == 0 && len(contractEvidenceRefs) == 0 {
		return []string{"acceptance_result_evidence_missing:" + criterion}
	}
	return nil
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
		if result.Failure != nil && result.Failure.Retryable != nil && *result.Failure.Retryable && taskResultRetryBudgetRemains(task) {
			return TaskResultDecisionFailedRetryable
		}
		if result.ReplanRequest != nil {
			return TaskResultDecisionReplanRequested
		}
		return TaskResultDecisionFailedRecovery
	case TaskResultStatusCancelled:
		return TaskResultDecisionCancelledTerminal
	default:
		return TaskResultDecisionValidationFailed
	}
}

func taskResultNeedsHumanReview(task ProjectTask, result TaskResultContract) bool {
	if task.RequiresHumanApproval || result.HumanReviewRequest != nil {
		return true
	}
	for _, risk := range result.Risks {
		if risk.RequiresHumanReview || highRiskLevel(risk.Severity) {
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
		return TaskResultRef{Kind: defaultKind, Ref: text}, true
	case map[string]any:
		ref := TaskResultRef{
			Kind:     firstNonBlankString(stringFromMap(typed, "kind"), stringFromMap(typed, "type"), defaultKind),
			Ref:      firstNonBlankString(stringFromMap(typed, "ref"), stringFromMap(typed, "id"), stringFromMap(typed, "uri"), stringFromMap(typed, "url")),
			URI:      stringFromMap(typed, "uri"),
			URL:      stringFromMap(typed, "url"),
			Title:    firstNonBlankString(stringFromMap(typed, "title"), stringFromMap(typed, "name")),
			Metadata: typed,
		}
		ref.ID = stringFromMap(typed, "id")
		if strings.TrimSpace(ref.Ref) == "" {
			return TaskResultRef{}, false
		}
		return ref, true
	case map[string]string:
		ref := TaskResultRef{
			ID:    strings.TrimSpace(typed["id"]),
			Kind:  firstNonBlankString(typed["kind"], typed["type"], defaultKind),
			Ref:   firstNonBlankString(typed["ref"], typed["id"], typed["uri"], typed["url"]),
			URI:   strings.TrimSpace(typed["uri"]),
			URL:   strings.TrimSpace(typed["url"]),
			Title: firstNonBlankString(typed["title"], typed["name"]),
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
		return TaskResultRef{Kind: defaultKind, Ref: text}, true
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
	for _, key := range []string{"criterion", "name", "id", "key", "title"} {
		text := stringFromMap(value, key)
		if text != "" {
			return []string{text}
		}
	}
	return nil
}

func stringsFromCriterionStringMap(value map[string]string) []string {
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

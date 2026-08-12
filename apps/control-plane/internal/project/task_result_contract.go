package project

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/google/uuid"
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
	// BlockedResolvableUpstream means the employee is starved by an upstream task's
	// output. The platform appends the owner (plus downstream) rather than waiting
	// on a human or bouncing back to the same employee.
	TaskResultDecisionBlockedResolvableUpstream TaskResultDecision = "blocked_resolvable_upstream"
	TaskResultDecisionFailedRetryable           TaskResultDecision = "failed_retryable"
	TaskResultDecisionFailedRecovery            TaskResultDecision = "failed_recovery"
	TaskResultDecisionCancelledTerminal         TaskResultDecision = "cancelled_terminal"
	TaskResultDecisionReplanRequested           TaskResultDecision = "replan_requested"
)

type TaskResultContract struct {
	Status             TaskResultStatus              `json:"status"`
	Summary            string                        `json:"summary"`
	AcceptanceResults  []TaskResultAcceptanceResult  `json:"acceptance_results,omitempty"`
	EvidenceRefs       []TaskResultRef               `json:"evidence_refs,omitempty"`
	ArtifactRefs       []TaskResultRef               `json:"artifact_refs,omitempty"`
	ChangesMade        []TaskResultChange            `json:"changes_made,omitempty"`
	Deliverables       []TaskResultDeliverable       `json:"deliverables,omitempty"`
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

func (c *TaskResultContract) UnmarshalJSON(data []byte) error {
	type alias TaskResultContract
	aux := struct {
		EvidenceRef *TaskResultRef `json:"evidence_ref,omitempty"`
		*alias
	}{alias: (*alias)(c)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if len(c.EvidenceRefs) == 0 && aux.EvidenceRef != nil {
		c.EvidenceRefs = []TaskResultRef{*aux.EvidenceRef}
	}
	return nil
}

// TaskResultDeliverable is one named handoff output. A deliverable counts as
// delivered only when it carries a value or a ref; the platform checks the set
// against the task's planner-declared produces on completion.
type TaskResultDeliverable struct {
	Name    string `json:"name"`
	Kind    string `json:"kind,omitempty"`
	Value   string `json:"value,omitempty"`
	Ref     string `json:"ref,omitempty"`
	Summary string `json:"summary,omitempty"`
}

// UnmarshalJSON 对 value 做类型宽容(调度韧性缺陷家族#4):执行者交付结构化
// 对象/数组/数字时不再 400 拒收——非字符串 JSON 原样紧凑序列化为字符串存储。
// 拒收会让"会话已完成、结果写不回"的任务永久卡 running,比宽容的代价大得多。
func (d *TaskResultDeliverable) UnmarshalJSON(data []byte) error {
	type deliverableAlias struct {
		Name    string          `json:"name"`
		Kind    string          `json:"kind,omitempty"`
		Value   json.RawMessage `json:"value,omitempty"`
		Ref     string          `json:"ref,omitempty"`
		Summary string          `json:"summary,omitempty"`
	}
	var alias deliverableAlias
	if err := json.Unmarshal(data, &alias); err != nil {
		return err
	}
	d.Name = alias.Name
	d.Kind = alias.Kind
	d.Ref = alias.Ref
	d.Summary = alias.Summary
	d.Value = ""
	if len(alias.Value) == 0 {
		return nil
	}
	var asString string
	if err := json.Unmarshal(alias.Value, &asString); err == nil {
		d.Value = asString
		return nil
	}
	trimmed := strings.TrimSpace(string(alias.Value))
	if trimmed == "null" {
		return nil
	}
	compact := &bytes.Buffer{}
	if err := json.Compact(compact, alias.Value); err != nil {
		return fmt.Errorf("deliverable value is not valid json: %w", err)
	}
	d.Value = compact.String()
	return nil
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

func (r *TaskResultAcceptanceResult) UnmarshalJSON(data []byte) error {
	type alias TaskResultAcceptanceResult
	aux := struct {
		EvidenceRef string `json:"evidence_ref,omitempty"`
		*alias
	}{alias: (*alias)(r)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if len(r.EvidenceRefs) == 0 && strings.TrimSpace(aux.EvidenceRef) != "" {
		r.EvidenceRefs = []string{strings.TrimSpace(aux.EvidenceRef)}
	}
	return nil
}

type TaskResultRef struct {
	ID      string `json:"id,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Type    string `json:"type,omitempty"`
	Ref     string `json:"ref,omitempty"`
	URI     string `json:"uri,omitempty"`
	URL     string `json:"url,omitempty"`
	Title   string `json:"title,omitempty"`
	Summary string `json:"summary,omitempty"`
	// 证据地基(spec §4.6):runtime 采集上传的对象形态 artifact 引用。
	// Sha256 存在即代表内容寻址对象,物化时据此核验并建立血缘。
	Name        string         `json:"name,omitempty"`
	Sha256      string         `json:"sha256,omitempty"`
	SizeBytes   int64          `json:"size_bytes,omitempty"`
	ContentType string         `json:"content_type,omitempty"`
	Truncated   bool           `json:"truncated,omitempty"`
	IsEvidence  bool           `json:"is_evidence,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
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

func (v *TaskResultVerification) UnmarshalJSON(data []byte) error {
	type alias TaskResultVerification
	aux := struct {
		EvidenceRef *TaskResultRef `json:"evidence_ref,omitempty"`
		*alias
	}{alias: (*alias)(v)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if len(v.EvidenceRefs) == 0 && aux.EvidenceRef != nil {
		v.EvidenceRefs = []TaskResultRef{*aux.EvidenceRef}
	}
	return nil
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
	Reason           string `json:"reason,omitempty"`
	ResolutionPrompt string `json:"resolution_prompt,omitempty"`
	RequiredBy       string `json:"required_by,omitempty"`
	// MissingInputs are produces-keys the employee declares it needs but did not
	// receive. Each must appear in this task's input_requirements.required_inputs
	// (Plan 3); the platform resolves the owner by lookup, never by asking a model
	// who is at fault. See the 2026-07-10 plan-phase refactor spec §4.6(a).
	MissingInputs []string        `json:"missing_inputs,omitempty"`
	ContextRefs   []TaskResultRef `json:"context_refs,omitempty"`
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

	// A completed result must deliver every output the planner declared in
	// produces: downstream tasks were validated against these names, so a
	// missing one starves a consumer. Tasks without produces are unaffected.
	delivered := map[string]bool{}
	for _, deliverable := range result.Deliverables {
		name := strings.TrimSpace(deliverable.Name)
		if name != "" && (strings.TrimSpace(deliverable.Value) != "" || strings.TrimSpace(deliverable.Ref) != "") {
			delivered[name] = true
		}
	}
	for _, name := range taskPlannerProduces(task) {
		if !delivered[name] {
			errors = append(errors, "handoff_deliverable_missing:"+name)
		}
	}
	return errors
}

// enrichContractWithHandoffVerification appends a platform-authored
// verification entry per fulfilled produces item. It runs after validation, so
// every produces item is known to be delivered; these entries record that the
// check happened, distinct from employee-claimed verifications.
func enrichContractWithHandoffVerification(task ProjectTask, contract TaskResultContract) TaskResultContract {
	if contract.Status != TaskResultStatusCompleted {
		return contract
	}
	for _, name := range taskPlannerProduces(task) {
		contract.Verification = append(contract.Verification, TaskResultVerification{
			Status:  TaskResultVerificationStatusPassed,
			Type:    "handoff_fulfillment",
			Method:  "platform_produces_check",
			Summary: "deliverable \"" + name + "\" 已交付（平台核对）",
		})
	}
	return contract
}

// resolveDeclaredDeliverableRefs 把契约里 deliverables[].ref 从相对路径/文件名
// 改写为已物化 declared 工件的 artifact_ref_id(v2 spec §3)。匹配不到的 Ref
// 与纯 value 项原样保留——值型交付物合法,不硬性失败。原始路径在改写时挪进
// Summary(为空时)以免血缘信息丢失。
func resolveDeclaredDeliverableRefs(contract TaskResultContract, declared map[string]uuid.UUID) TaskResultContract {
	if len(declared) == 0 || len(contract.Deliverables) == 0 {
		return contract
	}
	for index, deliverable := range contract.Deliverables {
		ref := strings.TrimSpace(deliverable.Ref)
		if ref == "" {
			continue
		}
		artifactRefID, ok := declared[ref]
		if !ok {
			// 兼容 agent 只写文件名、省略前缀、或仍用旧 deliverables/ 前缀。
			// 新前缀 .superteam/sessions/{command_id}/deliverables/ 与旧前缀都认。
			for _, alias := range declaredDeliverableLookupAliases(ref) {
				artifactRefID, ok = declared[alias]
				if ok {
					break
				}
			}
		}
		if !ok {
			continue
		}
		contract.Deliverables[index].Ref = artifactRefID.String()
		if strings.TrimSpace(deliverable.Summary) == "" {
			contract.Deliverables[index].Summary = ref
		}
	}
	return contract
}

// declaredDeliverableLookupAliases 把契约 ref 展开成可能出现在 declared 映射里的键：
// 文件名、去掉旧 `deliverables/` 前缀、去掉新会话输出前缀后的相对路径。
func declaredDeliverableLookupAliases(ref string) []string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	seen := map[string]struct{}{ref: {}}
	aliases := make([]string, 0, 4)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		aliases = append(aliases, value)
	}
	if trimmed, ok := strings.CutPrefix(ref, "deliverables/"); ok {
		add(trimmed)
	}
	if rest, ok := stripSessionDeliverablesPrefix(ref); ok {
		add(rest)
		add("deliverables/" + rest)
	}
	if base := path.Base(ref); base != "." && base != "/" && base != ref {
		add(base)
	}
	return aliases
}

// stripSessionDeliverablesPrefix 识别 `.superteam/sessions/{command_id}/deliverables/` 前缀。
func stripSessionDeliverablesPrefix(ref string) (string, bool) {
	const sessionsPrefix = ".superteam/sessions/"
	if !strings.HasPrefix(ref, sessionsPrefix) {
		return "", false
	}
	rest := strings.TrimPrefix(ref, sessionsPrefix)
	slash := strings.IndexByte(rest, '/')
	if slash <= 0 {
		return "", false
	}
	afterCommand := rest[slash+1:]
	const deliverablesPrefix = "deliverables/"
	if !strings.HasPrefix(afterCommand, deliverablesPrefix) {
		return "", false
	}
	return strings.TrimPrefix(afterCommand, deliverablesPrefix), true
}

// taskPlannerProduces mirrors projectcoordination.plannerProducesFromMetadata:
// the planner's produces declarations are persisted under
// ProjectTask.PlannerMetadata["produces"].
func taskPlannerProduces(task ProjectTask) []string {
	switch values := task.PlannerMetadata["produces"].(type) {
	case []any:
		produces := make([]string, 0, len(values))
		for _, value := range values {
			if produced, ok := value.(string); ok && strings.TrimSpace(produced) != "" {
				produces = append(produces, strings.TrimSpace(produced))
			}
		}
		return produces
	case []string:
		produces := make([]string, 0, len(values))
		for _, produced := range values {
			if strings.TrimSpace(produced) != "" {
				produces = append(produces, strings.TrimSpace(produced))
			}
		}
		return produces
	default:
		return nil
	}
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

// stringRefIsAttestation reports whether an acceptance-result evidence ref
// (a plain string, unlike the structured TaskResultRef checked by
// taskResultRefIsAttestation) points at a runtime attestation. Used to tighten
// automated_test acceptance results: a passed/human_overridden verdict against
// a snapshot automated_test criterion must carry at least one such ref, or the
// employee is merely self-reporting without machine-checked proof.
func stringRefIsAttestation(ref string) bool {
	return strings.HasPrefix(strings.TrimSpace(ref), "attestation:")
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
		// Every missing input must be one this task declared in required_inputs.
		// An undeclared name is a contract violation -> human.
		if result.Blocker != nil && len(result.Blocker.MissingInputs) > 0 {
			var requiredInputs []any
			if raw, ok := task.InputRequirements["required_inputs"]; ok {
				if slice, ok := raw.([]any); ok {
					requiredInputs = slice
				}
			}
			declared := stringSetFromAny(requiredInputs)
			allDeclared := true
			for _, missing := range result.Blocker.MissingInputs {
				if !declared[missing] {
					allDeclared = false
					break
				}
			}
			if allDeclared {
				return TaskResultDecisionBlockedResolvableUpstream
			}
		}
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

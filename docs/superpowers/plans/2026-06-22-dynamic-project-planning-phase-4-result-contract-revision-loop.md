# Dynamic Project Planning Phase 4 Result Contract Revision Loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a durable `TaskResultContract` writeback path that validates task acceptance criteria, decides whether to unlock downstream tasks, request human review, create revision work, trigger append-only replan, or generate the final demand summary.

**Architecture:** Keep all result interpretation in the Control Plane. Runtime Agent only submits a structured result contract or legacy writeback fields; Control Plane validates the contract against the accepted `ProjectTask`, persists an append-only result record, records audit/project events, and signals the project coordinator to advance the graph. Revision and replan actions append new facts rather than rewriting executed tasks or attempts.

**Tech Stack:** Go Control Plane, PostgreSQL migrations, sqlc, Temporal workflow/activity package `projectcoordination`, OpenAPI contract generation, Rust Runtime Agent writeback model, React Query project console, Vitest.

---

## Execution Preflight

Use an isolated worktree for implementation because this phase touches migrations, generated sqlc, OpenAPI, Runtime Agent payloads, workflow state, Web read models, and real runtime smoke.

```bash
git status --short
git worktree add .worktrees/dynamic-planning-phase-4 -b codex/dynamic-planning-phase-4
cd .worktrees/dynamic-planning-phase-4
git status --short
```

Expected:

- root checkout may contain unrelated user work
- implementation worktree should be clean
- branch should be `codex/dynamic-planning-phase-4`

Read these docs before editing:

```bash
sed -n '1,260p' docs/superpowers/specs/2026-06-21-dynamic-project-planning-orchestration-v1-phase-4-result-contract-revision-loop.md
sed -n '1,760p' docs/superpowers/specs/2026-06-21-dynamic-project-planning-orchestration-v1-design.md
sed -n '1,260p' DATABASE_DESIGN.md
sed -n '1,220p' DESIGN.md
```

Current code facts to preserve:

- `apps/control-plane/internal/project/service.go` currently accepts legacy completion fields in `CompleteProjectTaskAttempt`, validates `ExpectedOutputs`/`HandoffContract`, writes `project_execution_summaries`, and only signals `EmployeeTaskCompleted` after accepted completion.
- `apps/control-plane/internal/workflow/projectcoordination/project_store.go` already runs `PreDispatchGate` before `StartProjectTaskRun` and links `project_task_attempts.dispatch_gate_result_id`.
- `ResolveReadyDownstream` only moves blocked dependents back to `planned`; downstream dispatch still returns through Phase 3 gate.
- `project_task_attempts` is the attempt fact source. Keep attempt history append-only and do not overwrite old result payloads.
- `project_execution_summaries` is an existing lightweight summary table. Phase 4 adds structured result facts beside it, not inside a free-text summary field.
- Current latest migration is `033_digital_employee_env_and_skill_dependencies.sql`; use `034_project_task_results.sql` unless a newer migration appears in the implementation worktree.

## Scope Check

This plan covers one coherent subsystem: ProjectTask result handling after Runtime/Provider execution. It includes minimal Runtime payload support and a read-only Web visibility slice because those are needed to submit and inspect the result contract.

Do not reimplement Phase 1 planning profiles, Phase 2 PlanRevision decomposition, or Phase 3 PreDispatchGate. Do not let Runtime Agent create, cancel, or rewire `ProjectTask` records. Do not add a general graph editor.

## File Structure

- Create `apps/control-plane/internal/project/task_result_contract.go`: result contract types, status constants, validation helpers, decision types, and legacy completion-to-contract adapter.
- Create `apps/control-plane/internal/project/task_result_contract_test.go`: pure validation and decision tests for completed, revision, blocked, failed, cancelled, human review, and final-summary readiness.
- Modify `apps/control-plane/internal/project/types.go`: add project event types, request/response structs, `ProjectTaskResult`, `ProjectDemandSummary`, `TaskResultContract` fields on runtime writeback requests, and generic result signal payload types.
- Modify `apps/control-plane/internal/project/repository.go`: add repository methods for task result records, latest-result linkage, revision task ancestry, project demand summary records, and result-linked decision requests.
- Create `apps/control-plane/internal/storage/migrations/034_project_task_results.sql`: add `project_task_results`, `project_demand_summaries`, `project_tasks.revision_of_task_id`, `project_tasks.latest_task_result_id`, and `project_decision_requests.project_task_result_id`.
- Modify `apps/control-plane/internal/storage/migrations/atlas.sum`: update through the repo migration workflow after adding migration `034`.
- Modify `apps/control-plane/internal/storage/migrations_test.go`: assert UUID-first design, tenant-first indexes, append-only uniqueness, JSONB safety comments, and FK coverage.
- Modify `apps/control-plane/internal/storage/queries/project.sql`: add sqlc queries for result insert/read/list/link, revision task lookup, terminal graph summaries, demand summary creation, and demand terminal status updates.
- Regenerate `apps/control-plane/internal/storage/queries/*.go` using `corepack pnpm generate:control-plane`.
- Modify `apps/control-plane/internal/project/pg_repository.go`: implement task result and demand summary persistence in the existing transaction style.
- Modify `apps/control-plane/internal/project/pg_repository_test.go`: database-backed tests for idempotent result records, latest-result linkage, decision linkage, revision ancestry, and demand summary persistence.
- Modify `apps/control-plane/internal/project/service.go`: route legacy complete/fail/wait-human writebacks through a shared result processor and signal the coordinator with a structured result decision.
- Modify `apps/control-plane/internal/project/service_test.go`: cover result validation rejection, completion unlock signal, human review wait, revision request, blocked request, failed retry/recovery, cancelled terminal, and legacy adapter compatibility.
- Modify `apps/control-plane/internal/project/coordination_signal.go`: add `SignalEmployeeTaskResultDecision` and `EmployeeTaskResultDecisionSignal`.
- Modify `apps/control-plane/internal/workflow/projectcoordination/client.go`: send the new result decision signal to the project coordinator workflow.
- Modify `apps/control-plane/internal/workflow/projectcoordination/types.go`: add result decision activity input/output types.
- Modify `apps/control-plane/internal/workflow/projectcoordination/activities.go`: add activities for applying a result decision, creating a revision task or retry attempt, requesting replan, and generating final summaries.
- Create `apps/control-plane/internal/workflow/projectcoordination/task_result_decision.go`: coordinator-side decision application logic.
- Create `apps/control-plane/internal/workflow/projectcoordination/task_result_decision_test.go`: pure and fake-store tests for downstream unlock, revision task creation, blocked waits, failure recovery, replan request, cancellation, and final summary generation.
- Modify `apps/control-plane/internal/workflow/projectcoordination/project_store.go`: implement repository-backed result decision activities, using existing `ResolveReadyDownstream`, `HoldDownstreamForFailure`, `RequestProjectAcceptanceReview`, `DecomposeAcceptedPlanRevision`, and `DispatchProjectTask` paths where possible.
- Modify `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`: cover persisted event/request side effects for every result decision.
- Modify `apps/control-plane/internal/workflow/projectcoordination/workflow.go`: receive `EmployeeTaskResultDecisionSignal`, apply decisions serially, and dispatch any ready downstream task through Phase 3 gate.
- Modify `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`: verify result decisions do not bypass gate and final summary is generated only after the demand graph reaches a valid terminal condition.
- Modify `apps/control-plane/internal/project/handler.go`: add generic result writeback handler and read-only result/demand summary response mapping.
- Modify `apps/control-plane/internal/api/server.go`: register `POST /api/v1/runtime/project-task-attempts/{attemptId}/result`, `GET /api/v1/projects/{projectId}/tasks/{taskId}/results`, and `GET /api/v1/projects/{projectId}/demands/{demandId}/summary`.
- Modify `apps/control-plane/internal/project/handler_test.go` and `apps/control-plane/internal/api/project_routes_test.go`: cover route registration, request parsing, and response shape.
- Modify `contracts/control-plane/openapi.yaml`: add TaskResultContract schemas, generic result writeback route, task result list route, demand summary route, and generated client models.
- Modify `apps/runtime-agent/src/controlplane/models.rs`: add serializable `TaskResultContract` structs and optional `result_contract` on writeback payloads.
- Modify `apps/runtime-agent/src/commands/executor.rs`: parse provider JSON result contracts, synthesize legacy-compatible completed contracts, and include result contract on complete/fail/wait-human writebacks.
- Modify `apps/runtime-agent/tests/runtime_command_executor_test.rs`: assert structured result contract submission for completed, revision_needed, blocked, and failed provider outputs.
- Modify `apps/web/src/lib/api/projects.ts`: add `ProjectTaskResult`, `TaskResultContract`, `ProjectDemandSummary`, and fetchers.
- Modify `apps/web/src/lib/api/projects.test.ts`: cover the new fetchers and response decoding.
- Modify `apps/web/src/features/projects/index.tsx`: fetch task results and demand summary with existing project detail queries.
- Modify `apps/web/src/features/projects/components/project-operational-detail.tsx`: show latest task result decisions and final demand summary in the existing operational detail layout.
- Modify `apps/web/src/features/projects/index.test.tsx`: verify result status, revision/replan indicators, final summary, evidence refs, and risk rendering.
- Modify `CHANGELOG.md`: add one entry with `TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'` when implementation is complete.

### Task 1: Domain Contract And Pure Validation

**Files:**
- Create: `apps/control-plane/internal/project/task_result_contract.go`
- Create: `apps/control-plane/internal/project/task_result_contract_test.go`
- Modify: `apps/control-plane/internal/project/types.go`

- [ ] **Step 1: Write failing validation tests**

Create `apps/control-plane/internal/project/task_result_contract_test.go`:

```go
package project

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestValidateTaskResultContractAcceptsCompletedWithAllCriteria(t *testing.T) {
	taskID := uuid.MustParse("00000000-0000-0000-0000-000000000401")
	task := ProjectTask{
		ID:              taskID,
		ExpectedOutputs: []any{"execution_summary", "evidence_refs", "verification"},
		HandoffContract: map[string]any{
			"acceptance_criteria": []any{
				map[string]any{"criterion": "列出关键 SQL 或查询摘要", "required": true},
				map[string]any{"criterion": "说明剩余风险", "required": true},
			},
			"required_refs": []any{"artifact:sql-summary"},
		},
	}

	result := TaskResultContract{
		Status:  TaskResultStatusCompleted,
		Summary: "完成数据库异常分析，异常集中在 2026-06-20 批次。",
		AcceptanceResults: []TaskResultAcceptanceResult{
			{Criterion: "列出关键 SQL 或查询摘要", Status: TaskResultCriterionPassed, EvidenceRefs: []string{"artifact:sql-summary"}},
			{Criterion: "说明剩余风险", Status: TaskResultCriterionPassed, EvidenceRefs: []string{"artifact:risk-note"}},
		},
		EvidenceRefs: []TaskResultRef{{Type: "query_result", Ref: "artifact:sql-summary", Summary: "只读 SQL 查询摘要"}},
		ArtifactRefs: []TaskResultRef{{Type: "markdown", Ref: "artifact:analysis-report", Summary: "分析报告"}},
		Verification: []TaskResultVerification{{Type: "database_query", Status: TaskResultVerificationPassed, Summary: "只读查询成功"}},
		Risks:        []TaskResultRisk{{Level: "medium", Description: "仅覆盖 2026-06-20 至 2026-06-21 数据"}},
	}

	validation := ValidateTaskResultContract(task, result)

	require.True(t, validation.Valid)
	require.Empty(t, validation.Errors)
	require.Equal(t, TaskResultDecisionCompleteAccepted, validation.Decision)
}

func TestValidateTaskResultContractRejectsCompletedMissingCriteria(t *testing.T) {
	task := ProjectTask{
		ID:              uuid.New(),
		ExpectedOutputs: []any{"execution_summary", "evidence_refs"},
		HandoffContract: map[string]any{
			"acceptance_criteria": []any{
				"列出关键 SQL 或查询摘要",
				"说明剩余风险",
			},
		},
	}

	result := TaskResultContract{
		Status:  TaskResultStatusCompleted,
		Summary: "完成分析",
		AcceptanceResults: []TaskResultAcceptanceResult{
			{Criterion: "列出关键 SQL 或查询摘要", Status: TaskResultCriterionPassed, EvidenceRefs: []string{"artifact:sql-summary"}},
		},
		EvidenceRefs: []TaskResultRef{{Type: "query_result", Ref: "artifact:sql-summary"}},
	}

	validation := ValidateTaskResultContract(task, result)

	require.False(t, validation.Valid)
	require.Contains(t, validation.Errors, "acceptance_result_missing:说明剩余风险")
	require.Equal(t, TaskResultDecisionValidationFailed, validation.Decision)
}

func TestValidateTaskResultContractRequiresRevisionReason(t *testing.T) {
	result := TaskResultContract{
		Status:  TaskResultStatusRevisionNeeded,
		Summary: "需要补充测试证据",
	}

	validation := ValidateTaskResultContract(ProjectTask{ID: uuid.New()}, result)

	require.False(t, validation.Valid)
	require.Contains(t, validation.Errors, "revision_reason_required")
}

func TestValidateTaskResultContractMapsBlockedToHumanWait(t *testing.T) {
	result := TaskResultContract{
		Status:  TaskResultStatusBlocked,
		Summary: "缺少数据库只读权限",
		Blocker: &TaskResultBlocker{
			Reason:           "database_permission_missing",
			RequiredBy:       "human_owner",
			ResolutionPrompt: "授权只读数据库访问或取消该分析任务。",
		},
		HumanReviewRequest: &TaskResultHumanReviewRequest{
			Reason:  "permission_required",
			Prompt:  "是否允许继续数据库只读分析？",
			Options: []string{"approve", "deny", "request_more_context"},
		},
	}

	validation := ValidateTaskResultContract(ProjectTask{ID: uuid.New()}, result)

	require.True(t, validation.Valid)
	require.Equal(t, TaskResultDecisionBlockedWaitingHuman, validation.Decision)
}

func TestValidateTaskResultContractMapsFailedRetryable(t *testing.T) {
	retryable := true
	result := TaskResultContract{
		Status:  TaskResultStatusFailed,
		Summary: "Provider API transient failure",
		Failure: &TaskResultFailure{
			ErrorFamily:            "provider_overloaded",
			Retryable:              &retryable,
			RecoveryRecommendation: "retry_after_backoff",
		},
	}

	validation := ValidateTaskResultContract(ProjectTask{ID: uuid.New(), AttemptCount: 1, MaxAttempts: int32Ptr(3)}, result)

	require.True(t, validation.Valid)
	require.Equal(t, TaskResultDecisionFailedRetryable, validation.Decision)
}

func TestLegacyCompletionContractAdapterProducesCompletedContract(t *testing.T) {
	req := CompleteProjectTaskAttemptRequest{
		Conclusion:            "完成查询并输出结论",
		EvidenceRefs:          []any{map[string]any{"ref": "artifact:query-result-1", "type": "query_result"}},
		ArtifactRefs:          []any{map[string]any{"ref": "artifact:analysis-report", "type": "markdown"}},
		RecommendedNextAction: "继续下游验证任务",
	}

	contract := TaskResultContractFromLegacyCompletion(req)

	require.Equal(t, TaskResultStatusCompleted, contract.Status)
	require.Equal(t, "完成查询并输出结论", contract.Summary)
	require.Equal(t, "artifact:query-result-1", contract.EvidenceRefs[0].Ref)
	require.Equal(t, "artifact:analysis-report", contract.ArtifactRefs[0].Ref)
	require.Equal(t, "继续下游验证任务", contract.FollowUpRequests[0].Summary)
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestValidateTaskResultContract|TestLegacyCompletionContractAdapter' -count=1
```

Expected: FAIL with undefined symbols `TaskResultContract`, `TaskResultStatusCompleted`, `ValidateTaskResultContract`, and `TaskResultContractFromLegacyCompletion`.

- [ ] **Step 3: Add the domain contract implementation**

Create `apps/control-plane/internal/project/task_result_contract.go`:

```go
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
	TaskResultCriterionPassed          TaskResultCriterionStatus = "passed"
	TaskResultCriterionFailed          TaskResultCriterionStatus = "failed"
	TaskResultCriterionNeedsHuman      TaskResultCriterionStatus = "needs_human"
	TaskResultCriterionNotApplicable   TaskResultCriterionStatus = "not_applicable"
	TaskResultCriterionHumanOverridden TaskResultCriterionStatus = "human_overridden"
)

type TaskResultVerificationStatus string

const (
	TaskResultVerificationPassed TaskResultVerificationStatus = "passed"
	TaskResultVerificationFailed TaskResultVerificationStatus = "failed"
	TaskResultVerificationSkipped TaskResultVerificationStatus = "skipped"
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
	AcceptanceResults  []TaskResultAcceptanceResult  `json:"acceptance_results"`
	EvidenceRefs       []TaskResultRef               `json:"evidence_refs"`
	ArtifactRefs       []TaskResultRef               `json:"artifact_refs"`
	ChangesMade        []TaskResultChange            `json:"changes_made"`
	Verification       []TaskResultVerification      `json:"verification"`
	Risks              []TaskResultRisk              `json:"risks"`
	FollowUpRequests   []TaskResultFollowUpRequest   `json:"follow_up_requests"`
	HumanReviewRequest *TaskResultHumanReviewRequest `json:"human_review_request,omitempty"`
	RevisionRequest    *TaskResultRevisionRequest    `json:"revision_request,omitempty"`
	Blocker            *TaskResultBlocker            `json:"blocker,omitempty"`
	Failure            *TaskResultFailure            `json:"failure,omitempty"`
	ReplanRequest      *TaskResultReplanRequest      `json:"replan_request,omitempty"`
	Cancellation        *TaskResultCancellation       `json:"cancellation,omitempty"`
}

type TaskResultAcceptanceResult struct {
	Criterion           string                   `json:"criterion"`
	Status              TaskResultCriterionStatus `json:"status"`
	EvidenceRefs        []string                 `json:"evidence_refs"`
	Notes               string                   `json:"notes"`
	HumanAcceptedReason string                   `json:"human_accepted_reason,omitempty"`
}

type TaskResultRef struct {
	Type    string `json:"type"`
	Ref     string `json:"ref"`
	Summary string `json:"summary,omitempty"`
}

type TaskResultChange struct {
	Type    string `json:"type"`
	Ref     string `json:"ref,omitempty"`
	Summary string `json:"summary"`
}

type TaskResultVerification struct {
	Type    string                       `json:"type"`
	Status  TaskResultVerificationStatus `json:"status"`
	Summary string                       `json:"summary"`
	Ref     string                       `json:"ref,omitempty"`
}

type TaskResultRisk struct {
	Level       string `json:"level"`
	Description string `json:"description"`
}

type TaskResultFollowUpRequest struct {
	Type    string `json:"type"`
	Summary string `json:"summary"`
}

type TaskResultHumanReviewRequest struct {
	Reason  string   `json:"reason"`
	Prompt  string   `json:"prompt"`
	Options []string `json:"options"`
}

type TaskResultRevisionRequest struct {
	Reason                 string `json:"reason"`
	ContractChanged        bool   `json:"contract_changed"`
	RecommendedTaskTitle   string `json:"recommended_task_title,omitempty"`
	RecommendedTaskSummary string `json:"recommended_task_summary,omitempty"`
}

type TaskResultBlocker struct {
	Reason           string `json:"reason"`
	RequiredBy       string `json:"required_by"`
	ResolutionPrompt string `json:"resolution_prompt"`
}

type TaskResultFailure struct {
	ErrorFamily            string `json:"error_family"`
	Retryable              *bool  `json:"retryable"`
	RecoveryRecommendation string `json:"recovery_recommendation"`
}

type TaskResultReplanRequest struct {
	Reason      string   `json:"reason"`
	Constraints []string `json:"constraints"`
}

type TaskResultCancellation struct {
	Reason      string `json:"reason"`
	CancelledBy string `json:"cancelled_by"`
}

type TaskResultValidation struct {
	Valid    bool
	Decision TaskResultDecision
	Errors   []string
	Warnings []string
}

func ValidateTaskResultContract(task ProjectTask, result TaskResultContract) TaskResultValidation {
	errors := make([]string, 0)
	if !validTaskResultStatus(result.Status) {
		errors = append(errors, fmt.Sprintf("status_invalid:%s", result.Status))
	}
	if strings.TrimSpace(result.Summary) == "" {
		errors = append(errors, "summary_required")
	}

	requiredCriteria := requiredAcceptanceCriteria(task)
	acceptanceByCriterion := map[string]TaskResultAcceptanceResult{}
	for _, item := range result.AcceptanceResults {
		criterion := strings.TrimSpace(item.Criterion)
		if criterion == "" {
			errors = append(errors, "acceptance_result_criterion_required")
			continue
		}
		acceptanceByCriterion[criterion] = item
	}

	for _, criterion := range requiredCriteria {
		item, ok := acceptanceByCriterion[criterion]
		if !ok {
			errors = append(errors, "acceptance_result_missing:"+criterion)
			continue
		}
		if result.Status == TaskResultStatusCompleted && !criterionAccepted(item) {
			errors = append(errors, "acceptance_result_not_passed:"+criterion)
		}
		if result.Status == TaskResultStatusCompleted && len(item.EvidenceRefs) == 0 {
			errors = append(errors, "acceptance_result_evidence_missing:"+criterion)
		}
	}

	expected := stringSetFromAny(task.ExpectedOutputs)
	if expected["execution_summary"] && strings.TrimSpace(result.Summary) == "" {
		errors = append(errors, "expected_output_missing:execution_summary")
	}
	if expected["evidence_refs"] && len(result.EvidenceRefs) == 0 {
		errors = append(errors, "expected_output_missing:evidence_refs")
	}
	if expected["artifact_refs"] && len(result.ArtifactRefs) == 0 {
		errors = append(errors, "expected_output_missing:artifact_refs")
	}
	if expected["verification"] && len(result.Verification) == 0 {
		errors = append(errors, "expected_output_missing:verification")
	}

	switch result.Status {
	case TaskResultStatusCompleted:
		for _, verification := range result.Verification {
			if verification.Status == TaskResultVerificationFailed {
				errors = append(errors, "completed_verification_failed:"+strings.TrimSpace(verification.Type))
			}
		}
	case TaskResultStatusRevisionNeeded:
		if result.RevisionRequest == nil || strings.TrimSpace(result.RevisionRequest.Reason) == "" {
			errors = append(errors, "revision_reason_required")
		}
	case TaskResultStatusBlocked:
		if result.Blocker == nil || strings.TrimSpace(result.Blocker.Reason) == "" || strings.TrimSpace(result.Blocker.RequiredBy) == "" {
			errors = append(errors, "blocker_required")
		}
	case TaskResultStatusFailed:
		if result.Failure == nil || strings.TrimSpace(result.Failure.ErrorFamily) == "" || result.Failure.Retryable == nil || strings.TrimSpace(result.Failure.RecoveryRecommendation) == "" {
			errors = append(errors, "failure_detail_required")
		}
	case TaskResultStatusCancelled:
		if result.Cancellation == nil || strings.TrimSpace(result.Cancellation.Reason) == "" {
			errors = append(errors, "cancellation_reason_required")
		}
	}

	decision := taskResultDecision(task, result)
	if len(errors) > 0 {
		return TaskResultValidation{Valid: false, Decision: TaskResultDecisionValidationFailed, Errors: errors}
	}
	return TaskResultValidation{Valid: true, Decision: decision, Errors: []string{}}
}

func TaskResultContractFromLegacyCompletion(req CompleteProjectTaskAttemptRequest) TaskResultContract {
	result := TaskResultContract{
		Status:            TaskResultStatusCompleted,
		Summary:           strings.TrimSpace(req.Conclusion),
		EvidenceRefs:      taskResultRefsFromAny(req.EvidenceRefs),
		ArtifactRefs:      taskResultRefsFromAny(req.ArtifactRefs),
		FollowUpRequests:  []TaskResultFollowUpRequest{},
		HumanReviewRequest: nil,
	}
	if strings.TrimSpace(req.RecommendedNextAction) != "" {
		result.FollowUpRequests = append(result.FollowUpRequests, TaskResultFollowUpRequest{
			Type:    "recommended_next_action",
			Summary: strings.TrimSpace(req.RecommendedNextAction),
		})
	}
	if req.RequiresHumanReview {
		result.HumanReviewRequest = &TaskResultHumanReviewRequest{
			Reason:  "runtime_requested_review",
			Prompt:  req.Conclusion,
			Options: []string{"accept_result", "request_revision", "request_replan", "cancel_task"},
		}
	}
	return result
}

func TaskResultContractFromFailure(req FailProjectTaskAttemptRequest) TaskResultContract {
	retryable := false
	if req.Retryable != nil {
		retryable = *req.Retryable
	}
	return TaskResultContract{
		Status:  TaskResultStatusFailed,
		Summary: strings.TrimSpace(req.FailureSummary),
		Failure: &TaskResultFailure{
			ErrorFamily:            strings.TrimSpace(req.FailureFamily),
			Retryable:              &retryable,
			RecoveryRecommendation: "runtime_failure_recovery",
		},
	}
}

func TaskResultContractFromWaitHuman(req WaitHumanProjectTaskAttemptRequest) TaskResultContract {
	return TaskResultContract{
		Status:  TaskResultStatusBlocked,
		Summary: strings.TrimSpace(req.Summary),
		Blocker: &TaskResultBlocker{
			Reason:           strings.TrimSpace(req.Reason),
			RequiredBy:       "human_owner",
			ResolutionPrompt: strings.TrimSpace(req.Summary),
		},
		HumanReviewRequest: &TaskResultHumanReviewRequest{
			Reason:  strings.TrimSpace(req.Reason),
			Prompt:  strings.TrimSpace(req.Summary),
			Options: req.SuggestedResolutionOptions,
		},
	}
}

func validTaskResultStatus(status TaskResultStatus) bool {
	switch status {
	case TaskResultStatusCompleted, TaskResultStatusRevisionNeeded, TaskResultStatusBlocked, TaskResultStatusFailed, TaskResultStatusCancelled:
		return true
	default:
		return false
	}
}

func criterionAccepted(item TaskResultAcceptanceResult) bool {
	return item.Status == TaskResultCriterionPassed ||
		item.Status == TaskResultCriterionHumanOverridden ||
		(item.Status == TaskResultCriterionNotApplicable && strings.TrimSpace(item.HumanAcceptedReason) != "")
}

func taskResultDecision(task ProjectTask, result TaskResultContract) TaskResultDecision {
	switch result.Status {
	case TaskResultStatusCompleted:
		if result.HumanReviewRequest != nil || projectTaskRequiresResultReview(task, result) {
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
		if result.Failure != nil && result.Failure.Retryable != nil && *result.Failure.Retryable && projectTaskHasRetryBudget(task) {
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

func requiredAcceptanceCriteria(task ProjectTask) []string {
	raw, ok := task.HandoffContract["acceptance_criteria"]
	if !ok {
		return []string{}
	}
	items, ok := raw.([]any)
	if !ok {
		return []string{}
	}
	criteria := make([]string, 0, len(items))
	for _, item := range items {
		switch typed := item.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				criteria = append(criteria, trimmed)
			}
		case map[string]any:
			required, hasRequired := typed["required"].(bool)
			if hasRequired && !required {
				continue
			}
			if criterion, ok := typed["criterion"].(string); ok {
				if trimmed := strings.TrimSpace(criterion); trimmed != "" {
					criteria = append(criteria, trimmed)
				}
			}
		}
	}
	return criteria
}

func taskResultRefsFromAny(values []any) []TaskResultRef {
	refs := make([]TaskResultRef, 0, len(values))
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				refs = append(refs, TaskResultRef{Type: "ref", Ref: trimmed})
			}
		case map[string]any:
			ref, _ := typed["ref"].(string)
			refType, _ := typed["type"].(string)
			summary, _ := typed["summary"].(string)
			if strings.TrimSpace(ref) != "" {
				refs = append(refs, TaskResultRef{Type: strings.TrimSpace(refType), Ref: strings.TrimSpace(ref), Summary: strings.TrimSpace(summary)})
			}
		}
	}
	return refs
}

func projectTaskRequiresResultReview(task ProjectTask, result TaskResultContract) bool {
	return task.RequiresHumanApproval || strings.EqualFold(stringValue(task.RiskLevel), "high") || len(result.Risks) > 0 && strings.EqualFold(result.Risks[0].Level, "high")
}

func projectTaskHasRetryBudget(task ProjectTask) bool {
	if task.MaxAttempts == nil {
		return true
	}
	return task.AttemptCount < *task.MaxAttempts
}
```

Modify `apps/control-plane/internal/project/types.go` by adding these event constants beside the existing project task event constants:

```go
ProjectEventTaskResultRecorded          ProjectEventType = "project_task.result.recorded"
ProjectEventTaskResultValidationFailed ProjectEventType = "project_task.result.validation_failed"
ProjectEventTaskRevisionRequested      ProjectEventType = "project_task.revision.requested"
ProjectEventTaskRevisionCreated        ProjectEventType = "project_task.revision.created"
ProjectEventTaskReplanRequested        ProjectEventType = "project_task.replan.requested"
ProjectEventDemandSummaryCreated       ProjectEventType = "demand.summary.created"
```

- [ ] **Step 4: Run tests and verify they pass**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestValidateTaskResultContract|TestLegacyCompletionContractAdapter' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/control-plane/internal/project/task_result_contract.go apps/control-plane/internal/project/task_result_contract_test.go apps/control-plane/internal/project/types.go
git commit -m "feat: add project task result contract domain model"
```

### Task 2: Result Persistence And Repository

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/034_project_task_results.sql`
- Modify: `apps/control-plane/internal/storage/migrations/atlas.sum`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
- Modify: `apps/control-plane/internal/storage/queries/project.sql`
- Regenerate: `apps/control-plane/internal/storage/queries/*.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository_test.go`
- Modify: `apps/control-plane/internal/project/types.go`

- [ ] **Step 1: Write failing migration tests**

Add these assertions to `apps/control-plane/internal/storage/migrations_test.go` in the existing migration test style:

```go
func TestProjectTaskResultsMigrationIsAppendOnlyAndTenantScoped(t *testing.T) {
	body := readMigration(t, "034_project_task_results.sql")

	require.Contains(t, body, "CREATE TABLE project_task_results")
	require.Contains(t, body, "tenant_id UUID NOT NULL")
	require.Contains(t, body, "project_task_id UUID NOT NULL")
	require.Contains(t, body, "attempt_id UUID")
	require.Contains(t, body, "contract_payload JSONB NOT NULL")
	require.Contains(t, body, "validation_errors JSONB NOT NULL DEFAULT '[]'::jsonb")
	require.Contains(t, body, "CREATE UNIQUE INDEX uq_project_task_results_idempotency")
	require.Contains(t, body, "ON project_task_results(tenant_id, project_task_id, attempt_id, idempotency_key)")
	require.Contains(t, body, "CREATE INDEX idx_project_task_results_tenant_task_created")
	require.Contains(t, body, "COMMENT ON COLUMN project_task_results.contract_payload")
	require.NotContains(t, strings.ToLower(body), "raw_log")
	require.NotContains(t, strings.ToLower(body), "secret")
}

func TestProjectDemandSummariesMigrationIsDemandScoped(t *testing.T) {
	body := readMigration(t, "034_project_task_results.sql")

	require.Contains(t, body, "CREATE TABLE project_demand_summaries")
	require.Contains(t, body, "demand_id UUID NOT NULL")
	require.Contains(t, body, "summary_payload JSONB NOT NULL")
	require.Contains(t, body, "CREATE UNIQUE INDEX uq_project_demand_summaries_idempotency")
	require.Contains(t, body, "CREATE INDEX idx_project_demand_summaries_tenant_demand_created")
	require.Contains(t, body, "COMMENT ON COLUMN project_demand_summaries.summary_payload")
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/storage -run 'TestProjectTaskResultsMigration|TestProjectDemandSummariesMigration' -count=1
```

Expected: FAIL because `034_project_task_results.sql` does not exist.

- [ ] **Step 3: Add migration `034_project_task_results.sql`**

Create `apps/control-plane/internal/storage/migrations/034_project_task_results.sql`:

```sql
CREATE TABLE project_task_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    project_task_id UUID NOT NULL,
    attempt_id UUID,
    execution_summary_id UUID,
    result_status VARCHAR(32) NOT NULL,
    validation_status VARCHAR(32) NOT NULL,
    decision VARCHAR(64) NOT NULL,
    contract_payload JSONB NOT NULL,
    validation_errors JSONB NOT NULL DEFAULT '[]'::jsonb,
    validation_warnings JSONB NOT NULL DEFAULT '[]'::jsonb,
    idempotency_key VARCHAR(255) NOT NULL,
    human_review_request JSONB NOT NULL DEFAULT '{}'::jsonb,
    replan_request JSONB NOT NULL DEFAULT '{}'::jsonb,
    revision_request JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_event_id UUID,
    decision_request_id UUID,
    revision_task_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_project_task_results_task
        FOREIGN KEY (tenant_id, project_id, project_task_id)
        REFERENCES project_tasks(tenant_id, project_id, id) ON DELETE CASCADE,
    CONSTRAINT fk_project_task_results_attempt
        FOREIGN KEY (tenant_id, project_task_id, attempt_id)
        REFERENCES project_task_attempts(tenant_id, project_task_id, id),
    CONSTRAINT fk_project_task_results_execution_summary
        FOREIGN KEY (execution_summary_id) REFERENCES project_execution_summaries(id),
    CONSTRAINT chk_project_task_results_status CHECK (
        result_status IN ('completed', 'revision_needed', 'blocked', 'failed', 'cancelled')
    ),
    CONSTRAINT chk_project_task_results_validation_status CHECK (
        validation_status IN ('accepted', 'rejected')
    ),
    CONSTRAINT chk_project_task_results_decision CHECK (
        decision IN (
            'validation_failed',
            'complete_accepted',
            'waiting_human_review',
            'revision_attempt',
            'revision_task',
            'blocked_waiting_human',
            'failed_retryable',
            'failed_recovery',
            'cancelled_terminal',
            'replan_requested'
        )
    )
);

CREATE UNIQUE INDEX uq_project_task_results_idempotency
    ON project_task_results(tenant_id, project_task_id, attempt_id, idempotency_key)
    WHERE attempt_id IS NOT NULL;

CREATE UNIQUE INDEX uq_project_task_results_manual_idempotency
    ON project_task_results(tenant_id, project_task_id, idempotency_key)
    WHERE attempt_id IS NULL;

CREATE UNIQUE INDEX uq_project_task_results_tenant_task_id
    ON project_task_results(tenant_id, project_task_id, id);

CREATE UNIQUE INDEX uq_project_task_results_tenant_project_id
    ON project_task_results(tenant_id, project_id, id);

CREATE INDEX idx_project_task_results_tenant_task_created
    ON project_task_results(tenant_id, project_id, project_task_id, created_at DESC);

CREATE INDEX idx_project_task_results_decision
    ON project_task_results(tenant_id, project_id, decision, created_at DESC);

ALTER TABLE project_tasks
    ADD COLUMN revision_of_task_id UUID,
    ADD COLUMN latest_task_result_id UUID;

ALTER TABLE project_tasks
    ADD CONSTRAINT fk_project_tasks_revision_of
    FOREIGN KEY (tenant_id, revision_of_task_id)
    REFERENCES project_tasks(tenant_id, id);

ALTER TABLE project_tasks
    ADD CONSTRAINT fk_project_tasks_latest_task_result
    FOREIGN KEY (tenant_id, id, latest_task_result_id)
    REFERENCES project_task_results(tenant_id, project_task_id, id);

CREATE INDEX idx_project_tasks_revision_of
    ON project_tasks(tenant_id, project_id, revision_of_task_id)
    WHERE revision_of_task_id IS NOT NULL;

CREATE INDEX idx_project_tasks_latest_task_result
    ON project_tasks(tenant_id, latest_task_result_id)
    WHERE latest_task_result_id IS NOT NULL;

ALTER TABLE project_decision_requests
    ADD COLUMN project_task_result_id UUID;

ALTER TABLE project_decision_requests
    ADD CONSTRAINT fk_project_decision_requests_task_result
    FOREIGN KEY (tenant_id, project_id, project_task_result_id)
    REFERENCES project_task_results(tenant_id, project_id, id);

CREATE INDEX idx_project_decision_requests_task_result
    ON project_decision_requests(tenant_id, project_task_result_id)
    WHERE project_task_result_id IS NOT NULL;

CREATE TABLE project_demand_summaries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    project_id UUID NOT NULL,
    demand_id UUID NOT NULL,
    status VARCHAR(32) NOT NULL,
    conclusion TEXT NOT NULL,
    summary_payload JSONB NOT NULL,
    report_ref_id UUID,
    acceptance_required BOOLEAN NOT NULL DEFAULT true,
    idempotency_key VARCHAR(255) NOT NULL,
    created_event_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- NOTE: project_demands only has a single-column PK (id); there is no UNIQUE
    -- index on (tenant_id, project_id, id), so the FK must target the PK column.
    CONSTRAINT fk_project_demand_summaries_demand
        FOREIGN KEY (demand_id)
        REFERENCES project_demands(id) ON DELETE CASCADE,
    CONSTRAINT fk_project_demand_summaries_report
        FOREIGN KEY (report_ref_id) REFERENCES project_report_refs(id),
    CONSTRAINT chk_project_demand_summaries_status CHECK (
        status IN ('completed', 'blocked', 'failed', 'cancelled')
    )
);

CREATE UNIQUE INDEX uq_project_demand_summaries_idempotency
    ON project_demand_summaries(tenant_id, demand_id, idempotency_key);

CREATE INDEX idx_project_demand_summaries_tenant_demand_created
    ON project_demand_summaries(tenant_id, project_id, demand_id, created_at DESC);

CREATE TRIGGER update_project_task_results_updated_at
    BEFORE UPDATE ON project_task_results
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_project_demand_summaries_updated_at
    BEFORE UPDATE ON project_demand_summaries
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE project_task_results IS 'ProjectTask 结构化结果契约记录，保存 Runtime/Provider 写回后的可审计结果、校验状态和调度决策。';
COMMENT ON COLUMN project_task_results.contract_payload IS 'TaskResultContract JSON，不保存完整日志、密钥、连接串或大段 prompt。';
COMMENT ON COLUMN project_task_results.validation_errors IS '结果契约校验错误数组。';
COMMENT ON COLUMN project_task_results.human_review_request IS '结果需要人类判断时的请求摘要，不作为审批事实源。';
COMMENT ON COLUMN project_task_results.replan_request IS '任务结果触发重规划时的结构化原因和约束。';
COMMENT ON COLUMN project_task_results.revision_request IS '任务结果触发修订时的结构化原因和建议。';
COMMENT ON COLUMN project_tasks.revision_of_task_id IS '该任务是否为另一个 ProjectTask 的 append-only 修订任务。';
COMMENT ON COLUMN project_tasks.latest_task_result_id IS '该任务最新结构化结果记录ID。';
COMMENT ON COLUMN project_decision_requests.project_task_result_id IS '该人类决策由哪个结构化任务结果触发。';
COMMENT ON TABLE project_demand_summaries IS '项目需求最终总结记录，按 demand 生成 append-only 总结和报告引用。';
COMMENT ON COLUMN project_demand_summaries.summary_payload IS '最终需求总结 JSON，包含目标、结论、任务状态、证据、人工决策、验证、风险和下一步建议。';
```

- [ ] **Step 4: Add sqlc queries and repository types**

Append these query names to `apps/control-plane/internal/storage/queries/project.sql` using the same parameter style as existing project queries:

```sql
-- name: CreateProjectTaskResult :one
INSERT INTO project_task_results (
    tenant_id, project_id, project_task_id, attempt_id, execution_summary_id,
    result_status, validation_status, decision, contract_payload, validation_errors,
    validation_warnings, idempotency_key, human_review_request, replan_request,
    revision_request, created_event_id, decision_request_id, revision_task_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('project_task_id')::uuid,
    sqlc.narg('attempt_id')::uuid,
    sqlc.narg('execution_summary_id')::uuid,
    sqlc.arg('result_status')::text,
    sqlc.arg('validation_status')::text,
    sqlc.arg('decision')::text,
    sqlc.arg('contract_payload')::jsonb,
    COALESCE(sqlc.narg('validation_errors')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('validation_warnings')::jsonb, '[]'::jsonb),
    sqlc.arg('idempotency_key')::text,
    COALESCE(sqlc.narg('human_review_request')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('replan_request')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('revision_request')::jsonb, '{}'::jsonb),
    sqlc.narg('created_event_id')::uuid,
    sqlc.narg('decision_request_id')::uuid,
    sqlc.narg('revision_task_id')::uuid
) ON CONFLICT (tenant_id, project_task_id, attempt_id, idempotency_key)
WHERE attempt_id IS NOT NULL
DO UPDATE SET updated_at = project_task_results.updated_at
RETURNING *;

-- name: LinkProjectTaskLatestResult :one
UPDATE project_tasks
SET latest_task_result_id = sqlc.arg('task_result_id')::uuid
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND id = sqlc.arg('project_task_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
RETURNING *;

-- name: ListProjectTaskResults :many
SELECT * FROM project_task_results
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND project_task_id = sqlc.arg('project_task_id')::uuid
ORDER BY created_at DESC
LIMIT sqlc.arg('limit') OFFSET sqlc.arg('offset');

-- name: LinkProjectTaskResultDecisionRequest :one
UPDATE project_task_results
SET decision_request_id = sqlc.arg('decision_request_id')::uuid
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: LinkProjectTaskResultRevisionTask :one
UPDATE project_task_results
SET revision_task_id = sqlc.arg('revision_task_id')::uuid
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND id = sqlc.arg('id')::uuid
RETURNING *;

-- name: CreateProjectDemandSummary :one
INSERT INTO project_demand_summaries (
    tenant_id, project_id, demand_id, status, conclusion, summary_payload,
    report_ref_id, acceptance_required, idempotency_key, created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('demand_id')::uuid,
    sqlc.arg('status')::text,
    sqlc.arg('conclusion')::text,
    sqlc.arg('summary_payload')::jsonb,
    sqlc.narg('report_ref_id')::uuid,
    sqlc.arg('acceptance_required')::boolean,
    sqlc.arg('idempotency_key')::text,
    sqlc.narg('created_event_id')::uuid
) ON CONFLICT (tenant_id, demand_id, idempotency_key)
DO UPDATE SET updated_at = project_demand_summaries.updated_at
RETURNING *;

-- name: GetLatestProjectDemandSummary :one
SELECT * FROM project_demand_summaries
WHERE tenant_id = sqlc.arg('tenant_id')::uuid
  AND project_id = sqlc.arg('project_id')::uuid
  AND demand_id = sqlc.arg('demand_id')::uuid
ORDER BY created_at DESC
LIMIT 1;
```

`ProjectTask` already exists at `apps/control-plane/internal/project/types.go:285` and `CreateProjectTaskRequest` already exists at `apps/control-plane/internal/project/repository.go:228`. Do NOT paste these two structs verbatim — that is a duplicate type declaration and will not compile. Instead, ADD only the new fields `RevisionOfTaskID *uuid.UUID` and `LatestTaskResultID *uuid.UUID` to the existing `ProjectTask`, and add `RevisionOfTaskID *uuid.UUID` to the existing `CreateProjectTaskRequest`. The two structs below are shown as the target shape for reference only; `ProjectTaskResult` and `ProjectDemandSummary` are the genuinely new types to add to `types.go`:

```go
type ProjectTask struct {
	ID                         uuid.UUID
	TenantID                   uuid.UUID
	ProjectID                  uuid.UUID
	DemandID                   *uuid.UUID
	Title                      string
	Summary                    *string
	Status                     string
	AssignedDigitalEmployeeID  *uuid.UUID
	RuntimeTaskID              *uuid.UUID
	DigitalEmployeeRunID       *uuid.UUID
	RiskLevel                  *string
	RequiresHumanApproval      bool
	CoordinationJobID          *uuid.UUID
	RouteDecisionID            *uuid.UUID
	PlannedTaskKey             *string
	TaskKind                   *string
	StageIndex                 *int32
	ExpectedOutputs            []any
	InputRequirements          map[string]any
	HandoffContract            map[string]any
	PlannerMetadata            map[string]any
	BlockedByTaskIDs           []uuid.UUID
	CurrentAttemptID           *uuid.UUID
	LatestDispatchGateResultID *uuid.UUID
	LatestTaskResultID         *uuid.UUID
	RevisionOfTaskID           *uuid.UUID
	AcceptedPlanRevisionID     *uuid.UUID
	DecompositionClaimKey      *string
	AttemptCount               int32
	MaxAttempts                *int32
	RetryNotBefore             *time.Time
	WaitingReason              *string
	WaitingRequestID           *uuid.UUID
	TerminalReason             *string
	TerminalEventID            *uuid.UUID
	CancelledBy                *string
	FailedBy                   *string
	StatusChangedAt            time.Time
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

type CreateProjectTaskRequest struct {
	TenantID                  uuid.UUID
	ProjectID                 uuid.UUID
	DemandID                  *uuid.UUID
	Title                     string
	Summary                   string
	Status                    string
	AssignedDigitalEmployeeID *uuid.UUID
	RuntimeTaskID             *uuid.UUID
	DigitalEmployeeRunID      *uuid.UUID
	RiskLevel                 string
	RequiresHumanApproval     bool
	CoordinationJobID         *uuid.UUID
	RouteDecisionID           *uuid.UUID
	PlannedTaskKey            *string
	TaskKind                  *string
	StageIndex                *int32
	RevisionOfTaskID          *uuid.UUID
	AcceptedPlanRevisionID    *uuid.UUID
	DecompositionClaimKey     *string
	ExpectedOutputs           []any
	InputRequirements         map[string]any
	HandoffContract           map[string]any
	PlannerMetadata           map[string]any
	BlockedByTaskIDs          []uuid.UUID
}

type ProjectTaskResult struct {
	ID                 uuid.UUID
	TenantID           uuid.UUID
	ProjectID          uuid.UUID
	ProjectTaskID      uuid.UUID
	AttemptID          *uuid.UUID
	ExecutionSummaryID *uuid.UUID
	ResultStatus       TaskResultStatus
	ValidationStatus   string
	Decision           TaskResultDecision
	Contract           TaskResultContract
	ValidationErrors   []string
	ValidationWarnings []string
	IdempotencyKey     string
	DecisionRequestID  *uuid.UUID
	RevisionTaskID     *uuid.UUID
	CreatedEventID     *uuid.UUID
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type ProjectDemandSummary struct {
	ID                 uuid.UUID
	TenantID           uuid.UUID
	ProjectID          uuid.UUID
	DemandID           uuid.UUID
	Status             string
	Conclusion         string
	SummaryPayload     map[string]any
	ReportRefID        *uuid.UUID
	AcceptanceRequired bool
	IdempotencyKey     string
	CreatedEventID     *uuid.UUID
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
```

Update the existing `CreateProjectTask` query in `apps/control-plane/internal/storage/queries/project.sql` so revision ancestry is persisted on task creation:

```sql
INSERT INTO project_tasks (
    tenant_id,
    project_id,
    demand_id,
    title,
    summary,
    status,
    assigned_digital_employee_id,
    runtime_task_id,
    digital_employee_run_id,
    risk_level,
    requires_human_approval,
    coordination_job_id,
    route_decision_id,
    planned_task_key,
    task_kind,
    stage_index,
    revision_of_task_id,
    accepted_plan_revision_id,
    decomposition_claim_key,
    expected_outputs,
    input_requirements,
    handoff_contract,
    planner_metadata
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.narg('demand_id')::uuid,
    sqlc.arg('title')::text,
    sqlc.narg('summary')::text,
    sqlc.arg('status')::text,
    sqlc.narg('assigned_digital_employee_id')::uuid,
    sqlc.narg('runtime_task_id')::uuid,
    sqlc.narg('digital_employee_run_id')::uuid,
    sqlc.narg('risk_level')::text,
    sqlc.arg('requires_human_approval')::boolean,
    sqlc.narg('coordination_job_id')::uuid,
    sqlc.narg('route_decision_id')::uuid,
    sqlc.narg('planned_task_key')::text,
    sqlc.narg('task_kind')::text,
    sqlc.narg('stage_index')::integer,
    sqlc.narg('revision_of_task_id')::uuid,
    sqlc.narg('accepted_plan_revision_id')::uuid,
    sqlc.narg('decomposition_claim_key')::text,
    COALESCE(sqlc.narg('expected_outputs')::jsonb, '[]'::jsonb),
    COALESCE(sqlc.narg('input_requirements')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('handoff_contract')::jsonb, '{}'::jsonb),
    COALESCE(sqlc.narg('planner_metadata')::jsonb, '{}'::jsonb)
) RETURNING *;
```

Add repository request structs and interface methods to `apps/control-plane/internal/project/repository.go`:

```go
RecordProjectTaskResult(ctx context.Context, req RecordProjectTaskResultRequest) (ProjectTaskResult, error)
ListProjectTaskResults(ctx context.Context, req ListProjectTaskResultsRequest) ([]ProjectTaskResult, error)
LinkProjectTaskLatestResult(ctx context.Context, tenantID, projectID, projectTaskID, resultID uuid.UUID) (ProjectTask, error)
LinkProjectTaskResultDecisionRequest(ctx context.Context, tenantID, projectID, resultID, decisionRequestID uuid.UUID) (ProjectTaskResult, error)
LinkProjectTaskResultRevisionTask(ctx context.Context, tenantID, projectID, resultID, revisionTaskID uuid.UUID) (ProjectTaskResult, error)
CreateProjectDemandSummary(ctx context.Context, req CreateProjectDemandSummaryRequest) (ProjectDemandSummary, error)
GetLatestProjectDemandSummary(ctx context.Context, tenantID, projectID, demandID uuid.UUID) (ProjectDemandSummary, error)
```

- [ ] **Step 5: Implement repository and database tests**

Add these tests to `apps/control-plane/internal/project/pg_repository_test.go`:

```go
func TestPgRepositoryRecordProjectTaskResultIsIdempotentAndLinksLatest(t *testing.T) {
	repo, tenantID := newTestProjectRepository(t)
	projectRecord := createTestProject(t, repo, tenantID)
	task := createTestProjectTask(t, repo, tenantID, projectRecord.ID, "planned")

	contract := TaskResultContract{Status: TaskResultStatusCompleted, Summary: "完成结果"}
	first, err := repo.RecordProjectTaskResult(context.Background(), RecordProjectTaskResultRequest{
		TenantID:         tenantID,
		ProjectID:        projectRecord.ID,
		ProjectTaskID:    task.ID,
		ResultStatus:     TaskResultStatusCompleted,
		ValidationStatus: "accepted",
		Decision:         TaskResultDecisionCompleteAccepted,
		Contract:         contract,
		IdempotencyKey:   "attempt-result-1",
	})
	require.NoError(t, err)

	second, err := repo.RecordProjectTaskResult(context.Background(), RecordProjectTaskResultRequest{
		TenantID:         tenantID,
		ProjectID:        projectRecord.ID,
		ProjectTaskID:    task.ID,
		ResultStatus:     TaskResultStatusCompleted,
		ValidationStatus: "accepted",
		Decision:         TaskResultDecisionCompleteAccepted,
		Contract:         contract,
		IdempotencyKey:   "attempt-result-1",
	})
	require.NoError(t, err)
	require.Equal(t, first.ID, second.ID)

	updated, err := repo.LinkProjectTaskLatestResult(context.Background(), tenantID, projectRecord.ID, task.ID, first.ID)
	require.NoError(t, err)
	require.NotNil(t, updated.LatestTaskResultID)
	require.Equal(t, first.ID, *updated.LatestTaskResultID)
}
```

Then implement the mapper and repository methods in `apps/control-plane/internal/project/pg_repository.go` using the existing `jsonbObject`, `jsonbArray`, `mapFromJSON`, and transaction helpers.

Run:

```bash
corepack pnpm generate:control-plane
go test ./apps/control-plane/internal/storage ./apps/control-plane/internal/project -run 'TestProjectTaskResultsMigration|TestProjectDemandSummariesMigration|TestPgRepositoryRecordProjectTaskResult' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/storage/migrations/034_project_task_results.sql apps/control-plane/internal/storage/migrations/atlas.sum apps/control-plane/internal/storage/migrations_test.go apps/control-plane/internal/storage/queries apps/control-plane/internal/project/repository.go apps/control-plane/internal/project/pg_repository.go apps/control-plane/internal/project/pg_repository_test.go apps/control-plane/internal/project/types.go
git commit -m "feat: persist project task result contracts"
```

### Task 3: OpenAPI And Runtime Payload Contract

**Files:**
- Modify: `contracts/control-plane/openapi.yaml`
- Modify generated files from `corepack pnpm generate:control-plane`
- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/handler.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/project/handler_test.go`
- Modify: `apps/control-plane/internal/api/project_routes_test.go`
- Modify: `apps/runtime-agent/src/controlplane/models.rs`
- Modify: `apps/runtime-agent/src/commands/executor.rs`
- Modify: `apps/runtime-agent/tests/runtime_command_executor_test.rs`

- [ ] **Step 1: Write failing route and Runtime tests**

Add a handler test that posts a structured result:

```go
func TestCompleteProjectTaskAttemptResultRouteParsesTaskResultContract(t *testing.T) {
	service := &handlerTestService{}
	handler := NewHTTPHandler(service)
	attemptID := uuid.MustParse("00000000-0000-0000-0000-000000000441")
	taskID := uuid.MustParse("00000000-0000-0000-0000-000000000442")
	body := strings.NewReader(`{
		"project_task_id":"00000000-0000-0000-0000-000000000442",
		"runtime_node_id":"00000000-0000-0000-0000-000000000443",
		"lease_token":"lease-token",
		"idempotency_key":"result-1",
		"result_contract":{
			"status":"completed",
			"summary":"完成分析",
			"acceptance_results":[{"criterion":"输出结论","status":"passed","evidence_refs":["artifact:report"]}],
			"evidence_refs":[{"type":"report","ref":"artifact:report"}],
			"verification":[{"type":"unit_test","status":"passed","summary":"测试通过"}]
		}
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/project-task-attempts/"+attemptID.String()+"/result", body)
	req = req.WithContext(context.WithValue(req.Context(), middleware.TenantIDKey, testTenantID))
	rr := httptest.NewRecorder()

	handler.SubmitProjectTaskAttemptResult(rr, req)

	require.Equal(t, http.StatusAccepted, rr.Code)
	require.Equal(t, attemptID, service.submitProjectTaskAttemptResultReq.AttemptID)
	require.Equal(t, taskID, service.submitProjectTaskAttemptResultReq.ProjectTaskID)
	require.Equal(t, TaskResultStatusCompleted, service.submitProjectTaskAttemptResultReq.ResultContract.Status)
}
```

Add a Runtime Agent test in `apps/runtime-agent/tests/runtime_command_executor_test.rs`:

```rust
#[tokio::test]
async fn project_task_completion_writeback_includes_structured_result_contract() {
    let summary = serde_json::json!({
        "result_contract": {
            "status": "completed",
            "summary": "完成分析",
            "acceptance_results": [
                {"criterion": "输出结论", "status": "passed", "evidence_refs": ["artifact:report"]}
            ],
            "evidence_refs": [{"type": "report", "ref": "artifact:report"}],
            "verification": [{"type": "command", "status": "passed", "summary": "命令通过"}],
            "risks": []
        }
    })
    .to_string();

    let captured = run_project_task_completion_and_capture_writeback(Some(summary)).await;

    let contract = captured
        .get("result_contract")
        .expect("result_contract is sent");
    assert_eq!(contract["status"], "completed");
    assert_eq!(contract["summary"], "完成分析");
    assert_eq!(contract["acceptance_results"][0]["criterion"], "输出结论");
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/api -run 'TestCompleteProjectTaskAttemptResultRoute|TestRuntimeRoutesProjectTaskAttemptResult' -count=1
cargo test --manifest-path apps/runtime-agent/Cargo.toml project_task_completion_writeback_includes_structured_result_contract -- --nocapture
```

Expected: FAIL because the generic result route and `result_contract` model do not exist.

- [ ] **Step 3: Add OpenAPI schemas and handler request types**

In `contracts/control-plane/openapi.yaml`, add:

```yaml
  /api/v1/runtime/project-task-attempts/{attemptId}/result:
    post:
      operationId: submitProjectTaskAttemptResult
      summary: Submit a structured ProjectTask result contract
      parameters:
        - name: attemptId
          in: path
          required: true
          schema:
            type: string
            format: uuid
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/SubmitProjectTaskAttemptResultRequest"
      responses:
        "202":
          description: Result accepted for processing
```

Add schemas:

```yaml
    TaskResultStatus:
      type: string
      enum: [completed, revision_needed, blocked, failed, cancelled]
    TaskResultCriterionStatus:
      type: string
      enum: [passed, failed, needs_human, not_applicable, human_overridden]
    TaskResultContract:
      type: object
      required: [status, summary, acceptance_results, evidence_refs, artifact_refs, verification, risks]
      properties:
        status:
          $ref: "#/components/schemas/TaskResultStatus"
        summary:
          type: string
        acceptance_results:
          type: array
          items:
            $ref: "#/components/schemas/TaskResultAcceptanceResult"
        evidence_refs:
          type: array
          items:
            $ref: "#/components/schemas/TaskResultRef"
        artifact_refs:
          type: array
          items:
            $ref: "#/components/schemas/TaskResultRef"
        changes_made:
          type: array
          items:
            type: object
            additionalProperties: true
        verification:
          type: array
          items:
            type: object
            additionalProperties: true
        risks:
          type: array
          items:
            type: object
            additionalProperties: true
        follow_up_requests:
          type: array
          items:
            type: object
            additionalProperties: true
        human_review_request:
          type: object
          additionalProperties: true
        revision_request:
          type: object
          additionalProperties: true
        blocker:
          type: object
          additionalProperties: true
        failure:
          type: object
          additionalProperties: true
        replan_request:
          type: object
          additionalProperties: true
        cancellation:
          type: object
          additionalProperties: true
    TaskResultAcceptanceResult:
      type: object
      required: [criterion, status, evidence_refs]
      properties:
        criterion:
          type: string
        status:
          $ref: "#/components/schemas/TaskResultCriterionStatus"
        evidence_refs:
          type: array
          items:
            type: string
        notes:
          type: string
        human_accepted_reason:
          type: string
    TaskResultRef:
      type: object
      required: [type, ref]
      properties:
        type:
          type: string
        ref:
          type: string
        summary:
          type: string
    SubmitProjectTaskAttemptResultRequest:
      allOf:
        - $ref: "#/components/schemas/ProjectTaskAttemptRuntimeFields"
        - type: object
          required: [result_contract]
          properties:
            provider_session_id:
              type: string
            result_contract:
              $ref: "#/components/schemas/TaskResultContract"
```

Add `result_contract` as an optional property to `CompleteProjectTaskAttemptRequest`, `FailProjectTaskAttemptRequest`, and `WaitHumanProjectTaskAttemptRequest` so old endpoints can use the same processor.

- [ ] **Step 4: Add Runtime models and parser**

Modify `apps/runtime-agent/src/controlplane/models.rs`:

```rust
#[derive(Debug, Clone, Serialize, Default)]
pub struct TaskResultContract {
    pub status: String,
    pub summary: String,
    #[serde(default)]
    pub acceptance_results: Vec<serde_json::Value>,
    #[serde(default)]
    pub evidence_refs: Vec<serde_json::Value>,
    #[serde(default)]
    pub artifact_refs: Vec<serde_json::Value>,
    #[serde(default)]
    pub changes_made: Vec<serde_json::Value>,
    #[serde(default)]
    pub verification: Vec<serde_json::Value>,
    #[serde(default)]
    pub risks: Vec<serde_json::Value>,
    #[serde(default)]
    pub follow_up_requests: Vec<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub human_review_request: Option<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub revision_request: Option<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub blocker: Option<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub failure: Option<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub replan_request: Option<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub cancellation: Option<serde_json::Value>,
}

#[derive(Debug, Clone, Serialize)]
pub struct ProjectTaskCompleteWriteback {
    pub project_task_id: String,
    pub lease_token: String,
    pub runtime_node_id: String,
    pub idempotency_key: String,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub provider_session_id: Option<String>,
    pub conclusion: String,
    pub evidence_refs: Vec<serde_json::Value>,
    pub artifact_refs: Vec<serde_json::Value>,
    pub confidence_factors: HashMap<String, serde_json::Value>,
    pub uncertainty: String,
    pub missing_information: Vec<serde_json::Value>,
    pub recommended_next_action: String,
    pub requires_human_review: bool,
    #[serde(skip_serializing_if = "Option::is_none", default)]
    pub result_contract: Option<TaskResultContract>,
}
```

Modify `apps/runtime-agent/src/commands/executor.rs` so `project_task_complete_writeback` calls this helper:

```rust
fn parsed_result_contract(value: Option<&serde_json::Value>, fallback_summary: &str, evidence_refs: &[serde_json::Value], artifact_refs: &[serde_json::Value]) -> TaskResultContract {
    if let Some(contract) = value
        .and_then(|value| value.get("result_contract"))
        .and_then(serde_json::Value::as_object)
    {
        return TaskResultContract {
            status: contract
                .get("status")
                .and_then(serde_json::Value::as_str)
                .unwrap_or("completed")
                .to_string(),
            summary: contract
                .get("summary")
                .and_then(serde_json::Value::as_str)
                .unwrap_or(fallback_summary)
                .to_string(),
            acceptance_results: contract
                .get("acceptance_results")
                .and_then(serde_json::Value::as_array)
                .cloned()
                .unwrap_or_default(),
            evidence_refs: contract
                .get("evidence_refs")
                .and_then(serde_json::Value::as_array)
                .cloned()
                .unwrap_or_else(|| evidence_refs.to_vec()),
            artifact_refs: contract
                .get("artifact_refs")
                .and_then(serde_json::Value::as_array)
                .cloned()
                .unwrap_or_else(|| artifact_refs.to_vec()),
            changes_made: contract
                .get("changes_made")
                .and_then(serde_json::Value::as_array)
                .cloned()
                .unwrap_or_default(),
            verification: contract
                .get("verification")
                .and_then(serde_json::Value::as_array)
                .cloned()
                .unwrap_or_default(),
            risks: contract
                .get("risks")
                .and_then(serde_json::Value::as_array)
                .cloned()
                .unwrap_or_default(),
            follow_up_requests: contract
                .get("follow_up_requests")
                .and_then(serde_json::Value::as_array)
                .cloned()
                .unwrap_or_default(),
            human_review_request: contract.get("human_review_request").cloned(),
            revision_request: contract.get("revision_request").cloned(),
            blocker: contract.get("blocker").cloned(),
            failure: contract.get("failure").cloned(),
            replan_request: contract.get("replan_request").cloned(),
            cancellation: contract.get("cancellation").cloned(),
        };
    }
    TaskResultContract {
        status: "completed".to_string(),
        summary: fallback_summary.to_string(),
        acceptance_results: Vec::new(),
        evidence_refs: evidence_refs.to_vec(),
        artifact_refs: artifact_refs.to_vec(),
        changes_made: Vec::new(),
        verification: Vec::new(),
        risks: Vec::new(),
        follow_up_requests: Vec::new(),
        human_review_request: None,
        revision_request: None,
        blocker: None,
        failure: None,
        replan_request: None,
        cancellation: None,
    }
}
```

- [ ] **Step 5: Generate contracts and run tests**

Run:

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/api -run 'TestCompleteProjectTaskAttemptResultRoute|TestRuntimeRoutesProjectTaskAttemptResult' -count=1
cargo test --manifest-path apps/runtime-agent/Cargo.toml project_task_completion_writeback_includes_structured_result_contract -- --nocapture
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add contracts/control-plane/openapi.yaml apps/control-plane/internal/api apps/control-plane/internal/project apps/runtime-agent/src/controlplane/models.rs apps/runtime-agent/src/commands/executor.rs apps/runtime-agent/tests/runtime_command_executor_test.rs
git commit -m "feat: accept structured project task result writebacks"
```

### Task 4: Control Plane Result Processor

**Files:**
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/service_test.go`
- Modify: `apps/control-plane/internal/project/coordination_signal.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/client.go`
- Modify: `apps/control-plane/internal/project/types.go`

- [ ] **Step 1: Write failing service tests**

Add tests to `apps/control-plane/internal/project/service_test.go`:

```go
func TestSubmitProjectTaskAttemptResultRecordsAcceptedCompletionAndSignalsDecision(t *testing.T) {
	repo := newMemoryRepository()
	coordinator := &recordingCoordinator{}
	service := NewService(repo).WithCoordinator(coordinator)
	task, attempt := seedRunningProjectTaskAttempt(repo, "execution_summary", "evidence_refs")

	result, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: ProjectTaskAttemptRuntimeRequest{
			TenantID:       task.TenantID,
			AttemptID:      attempt.ID,
			ProjectTaskID:  task.ID,
			RuntimeNodeID:  *attempt.RuntimeNodeID,
			LeaseToken:     attempt.LeaseToken,
			IdempotencyKey: "result-complete-1",
		},
		ResultContract: TaskResultContract{
			Status:      TaskResultStatusCompleted,
			Summary:     "完成分析",
			EvidenceRefs: []TaskResultRef{{Type: "report", Ref: "artifact:report"}},
		},
	})

	require.NoError(t, err)
	require.Equal(t, TaskResultDecisionCompleteAccepted, result.Result.Decision)
	require.Equal(t, task.ID, coordinator.resultDecision.ProjectTaskID)
	require.Equal(t, TaskResultDecisionCompleteAccepted, coordinator.resultDecision.Decision)
}

func TestSubmitProjectTaskAttemptResultRejectsInvalidContractWithoutCompletingTask(t *testing.T) {
	repo := newMemoryRepository()
	service := NewService(repo).WithCoordinator(&recordingCoordinator{})
	task, attempt := seedRunningProjectTaskAttempt(repo, "evidence_refs")

	_, err := service.SubmitProjectTaskAttemptResult(context.Background(), SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: ProjectTaskAttemptRuntimeRequest{
			TenantID:       task.TenantID,
			AttemptID:      attempt.ID,
			ProjectTaskID:  task.ID,
			RuntimeNodeID:  *attempt.RuntimeNodeID,
			LeaseToken:     attempt.LeaseToken,
			IdempotencyKey: "result-invalid-1",
		},
		ResultContract: TaskResultContract{Status: TaskResultStatusCompleted, Summary: "缺证据"},
	})

	require.ErrorIs(t, err, ErrInvalidProjectEvidence)
	updated, _ := repo.GetProjectTask(context.Background(), task.TenantID, task.ID)
	require.Equal(t, ProjectTaskStatusRunning, updated.Status)
	require.Empty(t, repo.coordinatorSignals)
	require.Equal(t, "rejected", repo.taskResults[0].ValidationStatus)
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestSubmitProjectTaskAttemptResult' -count=1
```

Expected: FAIL because `SubmitProjectTaskAttemptResult` and `EmployeeTaskResultDecisionSignal` do not exist.

- [ ] **Step 3: Add request/result structs and signal**

Add to `apps/control-plane/internal/project/types.go`:

```go
type SubmitProjectTaskAttemptResultRequest struct {
	ProjectTaskAttemptRuntimeRequest
	DigitalEmployeeID uuid.UUID
	ResultContract    TaskResultContract
}

type SubmitProjectTaskAttemptResultResponse struct {
	Result  ProjectTaskResult
	Summary *ExecutionSummary
	Event   ProjectEvent
}
```

Add to `apps/control-plane/internal/project/coordination_signal.go`:

```go
SignalEmployeeTaskResultDecision(ctx context.Context, signal EmployeeTaskResultDecisionSignal) error
```

Add the signal type:

```go
type EmployeeTaskResultDecisionSignal struct {
	TenantID             uuid.UUID
	ProjectID            uuid.UUID
	ProjectTaskID        uuid.UUID
	AttemptID            *uuid.UUID
	TaskResultID         uuid.UUID
	ExecutionSummaryID   *uuid.UUID
	ResultStatus         TaskResultStatus
	Decision             TaskResultDecision
	DecisionRequestID    *uuid.UUID
	RevisionTaskID       *uuid.UUID
	CompletedEventID     *uuid.UUID
	FailedEventID        *uuid.UUID
	WorkflowID           string
}
```

- [ ] **Step 4: Implement shared processor**

In `apps/control-plane/internal/project/service.go`, add `SubmitProjectTaskAttemptResult` and make `CompleteProjectTaskAttempt`, `FailProjectTaskAttempt`, and `WaitHumanProjectTaskAttempt` call it through adapters.

```go
func (s *Service) SubmitProjectTaskAttemptResult(ctx context.Context, req SubmitProjectTaskAttemptResultRequest) (*SubmitProjectTaskAttemptResultResponse, error) {
	req.ResultContract.Summary = strings.TrimSpace(req.ResultContract.Summary)
	if req.ResultContract.Summary == "" {
		return nil, ErrInvalidProject
	}

	task, attempt, err := s.validateAttemptRuntimeRequest(ctx, req.ProjectTaskAttemptRuntimeRequest)
	if err != nil {
		return nil, err
	}
	digitalEmployeeID, err := digitalEmployeeIDForProjectTask(task)
	if err != nil {
		return nil, err
	}
	req.DigitalEmployeeID = digitalEmployeeID

	projectRecord, err := s.repository.GetProject(ctx, req.TenantID, task.ProjectID)
	if err != nil {
		return nil, err
	}

	validation := ValidateTaskResultContract(task, req.ResultContract)
	validationStatus := "accepted"
	if !validation.Valid {
		validationStatus = "rejected"
	}

	eventType := ProjectEventTaskResultRecorded
	eventSummary := "项目任务结构化结果已写回"
	if !validation.Valid {
		eventType = ProjectEventTaskResultValidationFailed
		eventSummary = "项目任务结构化结果校验失败"
	}
	event, err := s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: task.ProjectID,
		EventType: eventType,
		ActorType: "runtime_agent",
		ActorID:   req.RuntimeNodeID.String(),
		Summary:   eventSummary,
		Payload: map[string]any{
			"project_task_id": task.ID.String(),
			"attempt_id": attempt.ID.String(),
			"result_status": string(req.ResultContract.Status),
			"decision": string(validation.Decision),
			"validation_errors": validation.Errors,
		},
	})
	if err != nil {
		return nil, err
	}

	var summary *ExecutionSummary
	if validation.Valid && req.ResultContract.Status == TaskResultStatusCompleted {
		created, err := s.repository.CreateExecutionSummary(ctx, CreateExecutionSummaryRequest{
			TenantID:              req.TenantID,
			ProjectID:             task.ProjectID,
			ProjectTaskID:         task.ID,
			DigitalEmployeeID:     digitalEmployeeID,
			Conclusion:            req.ResultContract.Summary,
			EvidenceRefs:          taskResultRefsToAny(req.ResultContract.EvidenceRefs),
			ArtifactRefs:          taskResultRefsToAny(req.ResultContract.ArtifactRefs),
			ConfidenceFactors:     map[string]any{"source": "task_result_contract"},
			Uncertainty:           taskResultRiskSummary(req.ResultContract.Risks),
			MissingInformation:    []any{},
			RecommendedNextAction: taskResultFollowUpSummary(req.ResultContract.FollowUpRequests),
			RequiresHumanReview:   validation.Decision == TaskResultDecisionWaitingHumanReview,
			CreatedEventID:        &event.ID,
		})
		if err != nil {
			return nil, err
		}
		summary = &created
	}

	result, err := s.repository.RecordProjectTaskResult(ctx, RecordProjectTaskResultRequest{
		TenantID:             req.TenantID,
		ProjectID:            task.ProjectID,
		ProjectTaskID:        task.ID,
		AttemptID:            &attempt.ID,
		ExecutionSummaryID:   executionSummaryID(summary),
		ResultStatus:         req.ResultContract.Status,
		ValidationStatus:     validationStatus,
		Decision:             validation.Decision,
		Contract:             req.ResultContract,
		ValidationErrors:     validation.Errors,
		ValidationWarnings:   validation.Warnings,
		IdempotencyKey:       req.IdempotencyKey,
		CreatedEventID:       &event.ID,
	})
	if err != nil {
		return nil, err
	}
	if _, err := s.repository.LinkProjectTaskLatestResult(ctx, req.TenantID, task.ProjectID, task.ID, result.ID); err != nil {
		return nil, err
	}

	if !validation.Valid {
		return nil, ErrInvalidProjectEvidence
	}

	if err := s.coordinator.SignalEmployeeTaskResultDecision(ctx, EmployeeTaskResultDecisionSignal{
		TenantID:           req.TenantID,
		ProjectID:          task.ProjectID,
		ProjectTaskID:      task.ID,
		AttemptID:          &attempt.ID,
		TaskResultID:       result.ID,
		ExecutionSummaryID: executionSummaryID(summary),
		ResultStatus:       req.ResultContract.Status,
		Decision:           validation.Decision,
		CompletedEventID:   completedEventID(validation.Decision, event.ID),
		FailedEventID:      failedEventID(validation.Decision, event.ID),
		WorkflowID:         projectRecord.CoordinationWorkflowID,
	}); err != nil {
		_ = s.appendWorkflowSignalEvent(ctx, req.TenantID, task.ProjectID, "EmployeeTaskResultDecision", "failed", err, map[string]any{
			"project_task_id": task.ID.String(),
			"task_result_id": result.ID.String(),
		})
		return nil, err
	}

	return &SubmitProjectTaskAttemptResultResponse{Result: result, Summary: summary, Event: event}, nil
}
```

Keep the legacy endpoints compatible by adapting them:

```go
func (s *Service) CompleteProjectTaskAttempt(ctx context.Context, req CompleteProjectTaskAttemptRequest) (*ExecutionSummary, error) {
	contract := req.ResultContract
	if contract.Status == "" {
		contract = TaskResultContractFromLegacyCompletion(req)
	}
	result, err := s.SubmitProjectTaskAttemptResult(ctx, SubmitProjectTaskAttemptResultRequest{
		ProjectTaskAttemptRuntimeRequest: req.ProjectTaskAttemptRuntimeRequest,
		DigitalEmployeeID:                req.DigitalEmployeeID,
		ResultContract:                   contract,
	})
	if err != nil {
		return nil, err
	}
	return result.Summary, nil
}
```

Add equivalent adapters for fail and wait-human using `TaskResultContractFromFailure` and `TaskResultContractFromWaitHuman`.

- [ ] **Step 5: Run tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestSubmitProjectTaskAttemptResult|TestCompleteProjectTaskAttempt|TestFailProjectTaskAttempt|TestWaitHumanProjectTaskAttempt' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/project/service.go apps/control-plane/internal/project/service_test.go apps/control-plane/internal/project/coordination_signal.go apps/control-plane/internal/workflow/projectcoordination/client.go apps/control-plane/internal/project/types.go
git commit -m "feat: process project task result decisions"
```

### Task 5: Coordinator Result Decision State Machine

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/types.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/activities.go`
- Create: `apps/control-plane/internal/workflow/projectcoordination/task_result_decision.go`
- Create: `apps/control-plane/internal/workflow/projectcoordination/task_result_decision_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`

- [ ] **Step 1: Write failing decision tests**

Create `apps/control-plane/internal/workflow/projectcoordination/task_result_decision_test.go`:

```go
package projectcoordination

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	project "github.com/superteam/control-plane/internal/project"
)

func TestApplyTaskResultDecisionUnlocksCompletedDownstreamThroughGate(t *testing.T) {
	store := newFakeResultDecisionStore()
	upstreamID := uuid.MustParse("00000000-0000-0000-0000-000000000501")
	downstreamID := uuid.MustParse("00000000-0000-0000-0000-000000000502")
	store.readyDownstream = []uuid.UUID{downstreamID}

	result, err := ApplyTaskResultDecisionForTest(store, TaskResultDecisionInput{
		TenantID:      uuid.New(),
		ProjectID:     uuid.New(),
		ProjectTaskID: upstreamID,
		TaskResultID:  uuid.New(),
		Decision:      project.TaskResultDecisionCompleteAccepted,
	})

	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{downstreamID}, result.ReadyTaskIDs)
	require.Equal(t, []uuid.UUID{downstreamID}, store.dispatchedThroughGate)
}

func TestApplyTaskResultDecisionCreatesRevisionTaskWhenContractChanged(t *testing.T) {
	store := newFakeResultDecisionStore()
	taskID := uuid.MustParse("00000000-0000-0000-0000-000000000511")
	resultID := uuid.MustParse("00000000-0000-0000-0000-000000000512")
	store.taskResult = project.ProjectTaskResult{
		ID:            resultID,
		ProjectTaskID: taskID,
		Decision:      project.TaskResultDecisionRevisionTask,
		Contract: project.TaskResultContract{
			Status:  project.TaskResultStatusRevisionNeeded,
			Summary: "需要换员工补测试",
			RevisionRequest: &project.TaskResultRevisionRequest{
				Reason:                 "验收标准新增浏览器验证",
				ContractChanged:        true,
				RecommendedTaskTitle:   "补充浏览器验证",
				RecommendedTaskSummary: "补做真实浏览器验证并回写证据",
			},
		},
	}

	output, err := ApplyTaskResultDecisionForTest(store, TaskResultDecisionInput{
		TenantID:      uuid.New(),
		ProjectID:     uuid.New(),
		ProjectTaskID: taskID,
		TaskResultID:  resultID,
		Decision:      project.TaskResultDecisionRevisionTask,
	})

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, output.RevisionTaskID)
	require.Equal(t, taskID, store.createdRevision.RevisionOfTaskID)
	require.Equal(t, resultID, store.linkedRevisionResultID)
}

func TestApplyTaskResultDecisionRequestsReplanWithoutEditingOldTasks(t *testing.T) {
	store := newFakeResultDecisionStore()
	resultID := uuid.New()
	store.taskResult = project.ProjectTaskResult{
		ID:       resultID,
		Decision: project.TaskResultDecisionReplanRequested,
		Contract: project.TaskResultContract{
			Status:  project.TaskResultStatusFailed,
			Summary: "原计划方向不成立",
			ReplanRequest: &project.TaskResultReplanRequest{
				Reason:      "根因路径改变",
				Constraints: []string{"保留旧任务历史", "重新走 Phase 2 review"},
			},
		},
	}

	_, err := ApplyTaskResultDecisionForTest(store, TaskResultDecisionInput{
		TenantID:      uuid.New(),
		ProjectID:     uuid.New(),
		ProjectTaskID: uuid.New(),
		TaskResultID:  resultID,
		Decision:      project.TaskResultDecisionReplanRequested,
	})

	require.NoError(t, err)
	require.Equal(t, "replan_decision", store.decisionType)
	require.False(t, store.editedExistingTasks)
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestApplyTaskResultDecision' -count=1
```

Expected: FAIL because `TaskResultDecisionInput` and decision application logic do not exist.

- [ ] **Step 3: Add decision activity types**

Add to `apps/control-plane/internal/workflow/projectcoordination/types.go`:

```go
type TaskResultDecisionInput struct {
	TenantID           uuid.UUID
	ProjectID          uuid.UUID
	ProjectTaskID      uuid.UUID
	AttemptID          *uuid.UUID
	TaskResultID       uuid.UUID
	ExecutionSummaryID *uuid.UUID
	ResultStatus       project.TaskResultStatus
	Decision           project.TaskResultDecision
}

type TaskResultDecisionOutput struct {
	ReadyTaskIDs       []uuid.UUID
	RevisionTaskID     uuid.UUID
	DecisionRequestID  uuid.UUID
	DemandSummaryID    uuid.UUID
	TerminalDemand     bool
	DispatchErrors     []string
}
```

Add activity method declarations to `activities.go`:

```go
ApplyTaskResultDecision(ctx context.Context, input TaskResultDecisionInput) (TaskResultDecisionOutput, error)
```

- [ ] **Step 4: Implement decision application**

Create `apps/control-plane/internal/workflow/projectcoordination/task_result_decision.go`:

```go
package projectcoordination

import (
	"context"
	"errors"

	"github.com/google/uuid"
	project "github.com/superteam/control-plane/internal/project"
)

func (s *ProjectStore) ApplyTaskResultDecision(ctx context.Context, input TaskResultDecisionInput) (TaskResultDecisionOutput, error) {
	switch input.Decision {
	case project.TaskResultDecisionCompleteAccepted:
		return s.applyCompletedTaskResult(ctx, input)
	case project.TaskResultDecisionWaitingHumanReview:
		return s.applyResultHumanReview(ctx, input)
	case project.TaskResultDecisionRevisionAttempt:
		return s.applyRevisionAttempt(ctx, input)
	case project.TaskResultDecisionRevisionTask:
		return s.applyRevisionTask(ctx, input)
	case project.TaskResultDecisionBlockedWaitingHuman:
		return s.applyBlockedTaskResult(ctx, input)
	case project.TaskResultDecisionFailedRetryable:
		return s.applyFailedRetryableResult(ctx, input)
	case project.TaskResultDecisionFailedRecovery:
		return s.applyFailedRecoveryResult(ctx, input)
	case project.TaskResultDecisionCancelledTerminal:
		return s.applyCancelledResult(ctx, input)
	case project.TaskResultDecisionReplanRequested:
		return s.applyReplanRequestedResult(ctx, input)
	default:
		return TaskResultDecisionOutput{}, project.ErrInvalidProject
	}
}

func (s *ProjectStore) applyCompletedTaskResult(ctx context.Context, input TaskResultDecisionInput) (TaskResultDecisionOutput, error) {
	if _, err := s.repository.UpdateProjectTaskStatus(ctx, input.TenantID, input.ProjectTaskID, "completed", nil, []string{"queued", "running", "waiting_human"}); err != nil {
		return TaskResultDecisionOutput{}, err
	}
	ready, err := s.ResolveReadyDownstream(ctx, ResolveReadyDownstreamInput{
		TenantID:        input.TenantID,
		ProjectID:       input.ProjectID,
		CompletedTaskID: input.ProjectTaskID,
	})
	if err != nil {
		return TaskResultDecisionOutput{}, err
	}
	output := TaskResultDecisionOutput{ReadyTaskIDs: ready}
	for _, taskID := range ready {
		if err := s.DispatchProjectTask(ctx, DispatchProjectTaskInput{
			TenantID:       input.TenantID,
			ProjectID:      input.ProjectID,
			TaskID:         taskID,
			DispatchReason: DispatchReasonDependencyUnlocked,
		}); err != nil {
			output.DispatchErrors = append(output.DispatchErrors, err.Error())
		}
	}
	// generateFinalSummaryIfDemandTerminal MUST return ErrDemandGraphNotTerminal
	// (not a generic error) when the demand graph is not yet terminal. A mid-graph
	// completion is the common case and must NOT fail the decision.
	summary, err := s.generateFinalSummaryIfDemandTerminal(ctx, input)
	switch {
	case err == nil && summary.ID != uuid.Nil:
		output.DemandSummaryID = summary.ID
		output.TerminalDemand = true
	case errors.Is(err, ErrDemandGraphNotTerminal):
		// demand graph not terminal yet: no final summary, this is not an error
	case err != nil:
		return TaskResultDecisionOutput{}, err
	}
	return output, nil
}
```

Implement the other methods with these exact effects:

- `applyResultHumanReview`: create a `result_review` approval and `project_decision_requests` record, link it to `project_task_results`, move task to `waiting_human`, and do not release downstream.
- `applyRevisionAttempt`: mark current attempt terminal, move task back to `planned`, increment through the next dispatch only after Phase 3 gate, and do not create a new task.
- `applyRevisionTask`: create a new `ProjectTask` with `revision_of_task_id` set to the original task, blocked by the original task's blockers and replacing the original in downstream dependencies through `RewireProjectTaskDependencies`; link `project_task_results.revision_task_id`; dispatch the revision only through Phase 3 gate.
- `applyBlockedTaskResult`: create a `blocked_result_resolution` human decision request, move task to `waiting_human`, keep downstream blocked.
- `applyFailedRetryableResult`: set retry metadata and move the task back to `planned`; dispatch through Phase 3 gate only when retry time is due.
- `applyFailedRecoveryResult`: use existing `HoldDownstreamForFailure` with the result summary; keep downstream blocked and request human recovery.
- `applyCancelledResult`: mark task `cancelled`, cancel downstream only when the dependency policy in the result contract demands cancellation, otherwise request replan.
- `applyReplanRequestedResult`: create `replan_decision`, mark affected downstream blocked, and invoke the existing plan revision generation path so the new PlanRevision goes through Phase 2 review.

- [ ] **Step 5: Wire workflow signal**

In `apps/control-plane/internal/workflow/projectcoordination/workflow.go`, handle `EmployeeTaskResultDecisionSignal`:

```go
func handleEmployeeTaskResultDecision(ctx workflow.Context, state *projectCoordinatorState, signal project.EmployeeTaskResultDecisionSignal) error {
	var output TaskResultDecisionOutput
	err := workflow.ExecuteActivity(ctx, activityNames.ApplyTaskResultDecision, TaskResultDecisionInput{
		TenantID:           signal.TenantID,
		ProjectID:          signal.ProjectID,
		ProjectTaskID:      signal.ProjectTaskID,
		AttemptID:          signal.AttemptID,
		TaskResultID:       signal.TaskResultID,
		ExecutionSummaryID: signal.ExecutionSummaryID,
		ResultStatus:       signal.ResultStatus,
		Decision:           signal.Decision,
	}).Get(ctx, &output)
	if err != nil {
		return err
	}
	state.LastTaskResultID = signal.TaskResultID
	return nil
}
```

Keep the old `EmployeeTaskCompleted` and `EmployeeTaskFailed` handlers for backward compatibility, but route new Runtime writebacks through `EmployeeTaskResultDecision`.

- [ ] **Step 6: Run coordinator tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestApplyTaskResultDecision|TestProjectCoordinator.*ResultDecision|TestProjectStore.*Result' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination
git commit -m "feat: apply project task result decisions in coordinator"
```

### Task 6: Human Review, Revision, And Replan Recovery Paths

**Files:**
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/service_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`

- [ ] **Step 1: Write failing human decision tests**

Add tests that resolve result review decisions:

```go
func TestResolveResultReviewAcceptsResultAndUnlocksDownstream(t *testing.T) {
	store := newProjectStoreWithResultReviewFixture(t)
	decision := store.seedResultReviewDecision("accept_result")

	output, err := store.ResolveResultReviewDecision(context.Background(), ResultReviewDecisionInput{
		TenantID:          decision.TenantID,
		ProjectID:         decision.ProjectID,
		DecisionRequestID: decision.ID,
		Decision:          "accept_result",
		ResponseSummary:   "结果可接受",
	})

	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{store.downstreamTaskID}, output.ReadyTaskIDs)
	require.Contains(t, store.projectEventsOfType(project.ProjectEventDecisionSubmitted), "结果可接受")
}

func TestResolveResultReviewRequestRevisionCreatesRevisionDecision(t *testing.T) {
	store := newProjectStoreWithResultReviewFixture(t)
	decision := store.seedResultReviewDecision("request_revision")

	output, err := store.ResolveResultReviewDecision(context.Background(), ResultReviewDecisionInput{
		TenantID:          decision.TenantID,
		ProjectID:         decision.ProjectID,
		DecisionRequestID: decision.ID,
		Decision:          "request_revision",
		ResponseSummary:   "补充真实接口验证",
	})

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, output.RevisionTaskID)
	require.Equal(t, store.taskID, store.createdRevision.RevisionOfTaskID)
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestResolveResultReview' -count=1
```

Expected: FAIL because result-review resolution helpers do not exist.

- [ ] **Step 3: Implement result-review resolution mapping**

Add `ResultReviewDecisionInput` and `ResultReviewDecisionOutput`:

```go
type ResultReviewDecisionInput struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DecisionRequestID uuid.UUID
	Decision          string
	ResponseSummary   string
	ContextRefs       []any
}

type ResultReviewDecisionOutput struct {
	ReadyTaskIDs      []uuid.UUID
	RevisionTaskID    uuid.UUID
	ReplanRequestedID uuid.UUID
}
```

Implement decision mapping:

```go
func resultReviewDecisionToAction(decision string) (project.TaskResultDecision, error) {
	switch decision {
	case "accept_result":
		return project.TaskResultDecisionCompleteAccepted, nil
	case "request_revision":
		return project.TaskResultDecisionRevisionTask, nil
	case "mark_blocked":
		return project.TaskResultDecisionBlockedWaitingHuman, nil
	case "request_replan":
		return project.TaskResultDecisionReplanRequested, nil
	case "cancel_task":
		return project.TaskResultDecisionCancelledTerminal, nil
	default:
		return project.TaskResultDecisionValidationFailed, project.ErrInvalidProject
	}
}
```

Wire this into `handleHumanDecisionSubmitted` so `decision_type` values `result_review`, `blocked_result_resolution`, and `replan_decision` resume the correct result decision path instead of only resolving legacy waits.

- [ ] **Step 4: Implement append-only replan creation**

When a result produces `request_replan`, call the existing planner path with a PlanningSnapshot that includes:

```go
map[string]any{
	"replan_reason": result.Contract.ReplanRequest.Reason,
	"task_result_id": result.ID.String(),
	"failed_project_task_id": result.ProjectTaskID.String(),
	"preserve_history": true,
	"current_task_statuses": terminalTaskSnapshot,
	"artifact_refs": collectedArtifactRefs,
	"evidence_refs": collectedEvidenceRefs,
	"human_decisions": collectedHumanDecisionSummaries,
}
```

Persist the new PlanRevision with:

- `revision_number` incremented from the latest revision for the same demand
- `status = pending_review` when human review is required
- `status = accepted` only when the existing policy allows automatic low-risk acceptance
- old tasks kept as completed, failed, blocked, waiting_human, cancelled, or historical

- [ ] **Step 5: Run tests**

Run:

```bash
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/workflow/projectcoordination -run 'TestResolveResultReview|TestApplyTaskResultDecisionRequestsReplan|TestProjectCoordinator.*HumanDecision' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/project apps/control-plane/internal/workflow/projectcoordination
git commit -m "feat: support result review revision and replan recovery"
```

### Task 7: Final Demand Summary

**Files:**
- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/pg_repository_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/task_result_decision.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`

- [ ] **Step 1: Write failing summary tests**

Add to `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`:

```go
func TestProjectStoreGeneratesFinalDemandSummaryWhenAllRequiredTasksAccepted(t *testing.T) {
	store := newProjectStoreWithCompletedDemandGraph(t)

	summary, err := store.GenerateFinalDemandSummary(context.Background(), GenerateFinalDemandSummaryInput{
		TenantID:  store.tenantID,
		ProjectID: store.projectID,
		DemandID:  store.demandID,
	})

	require.NoError(t, err)
	require.Equal(t, "completed", summary.Status)
	require.Contains(t, summary.SummaryPayload["final_conclusion"], "完成")
	require.NotEmpty(t, summary.SummaryPayload["completed_tasks"])
	require.NotEmpty(t, summary.SummaryPayload["evidence_refs"])
	require.NotEmpty(t, summary.SummaryPayload["risks"])
}

func TestProjectStoreDoesNotGenerateSummaryWhenRequiredTaskStillRunning(t *testing.T) {
	store := newProjectStoreWithRunningDemandGraph(t)

	_, err := store.GenerateFinalDemandSummary(context.Background(), GenerateFinalDemandSummaryInput{
		TenantID:  store.tenantID,
		ProjectID: store.projectID,
		DemandID:  store.demandID,
	})

	require.ErrorIs(t, err, ErrDemandGraphNotTerminal)
}
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStoreGeneratesFinalDemandSummary|TestProjectStoreDoesNotGenerateSummary' -count=1
```

Expected: FAIL because final summary generation is not implemented.

- [ ] **Step 3: Add summary generation types**

Add:

```go
type GenerateFinalDemandSummaryInput struct {
	TenantID  uuid.UUID
	ProjectID uuid.UUID
	DemandID  uuid.UUID
}

type FinalDemandSummaryPayload struct {
	OriginalGoal       string                     `json:"original_goal"`
	FinalConclusion    string                     `json:"final_conclusion"`
	CompletedTasks     []FinalDemandSummaryTask   `json:"completed_tasks"`
	UnfinishedTasks    []FinalDemandSummaryTask   `json:"unfinished_tasks"`
	EvidenceRefs       []TaskResultRef            `json:"evidence_refs"`
	ArtifactRefs       []TaskResultRef            `json:"artifact_refs"`
	HumanDecisions     []FinalDemandHumanDecision `json:"human_decisions"`
	ChangesMade        []TaskResultChange         `json:"changes_made"`
	Verification       []TaskResultVerification   `json:"verification"`
	Risks              []TaskResultRisk           `json:"risks"`
	RecommendedNext    []TaskResultFollowUpRequest `json:"recommended_next"`
}
```

- [ ] **Step 4: Implement final summary policy**

The summary trigger is valid when one of these is true:

```go
func demandGraphSummaryAllowed(tasks []project.ProjectTask, humanStopDecision bool) bool {
	for _, task := range tasks {
		switch task.Status {
		case "completed", "cancelled":
			continue
		case "failed", "blocked", "waiting_human":
			if humanStopDecision {
				continue
			}
			return false
		default:
			return false
		}
	}
	return true
}
```

Build payload sections from `project_task_results`, `project_execution_summaries`, `project_evidence_refs`, `project_artifact_refs`, and `project_decision_requests`. Persist a `project_report_refs` row with:

```go
CreateReportRefRequest{
	TenantID:        input.TenantID,
	ProjectID:       input.ProjectID,
	ReportType:      "final_demand_summary",
	Title:           "最终需求总结",
	Summary:         payload.FinalConclusion,
	ObjectRef:       "project-demand-summary://" + input.DemandID.String(),
	Format:          "json",
	GeneratedByType: "system",
	CreatedEventID:  &event.ID,
}
```

Then create `project_demand_summaries` with the same idempotency key:

```go
idempotencyKey := "final-demand-summary:" + input.DemandID.String() + ":graph-terminal"
```

- [ ] **Step 5: Run tests**

Run:

```bash
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStoreGeneratesFinalDemandSummary|TestProjectStoreDoesNotGenerateSummary|TestPgRepository.*DemandSummary' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add apps/control-plane/internal/project apps/control-plane/internal/workflow/projectcoordination
git commit -m "feat: generate final project demand summaries"
```

### Task 8: Read APIs And Web Visibility

**Files:**
- Modify: `apps/control-plane/internal/project/handler.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Modify: `apps/control-plane/internal/project/handler_test.go`
- Modify: `apps/control-plane/internal/api/project_routes_test.go`
- Modify: `contracts/control-plane/openapi.yaml`
- Modify generated client files
- Modify: `apps/web/src/lib/api/projects.ts`
- Modify: `apps/web/src/lib/api/projects.test.ts`
- Modify: `apps/web/src/features/projects/index.tsx`
- Modify: `apps/web/src/features/projects/components/project-operational-detail.tsx`
- Modify: `apps/web/src/features/projects/index.test.tsx`

- [ ] **Step 1: Write failing Web and API tests**

Add Web API tests:

```ts
it("fetches project task results", async () => {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({
      items: [
        {
          id: "result-1",
          project_task_id: "task-1",
          result_status: "revision_needed",
          decision: "revision_task",
          contract: { status: "revision_needed", summary: "需要补测试" },
          created_at: "2026-06-22T09:00:00Z",
        },
      ],
    }),
  });

  const results = await listProjectTaskResults("project-1", "task-1", { fetcher: fetchMock });

  expect(fetchMock).toHaveBeenCalledWith("/api/v1/projects/project-1/tasks/task-1/results?limit=20&offset=0", expect.any(Object));
  expect(results[0].decision).toBe("revision_task");
});
```

Add UI test:

```tsx
it("renders latest task result and final demand summary", async () => {
  render(<ProjectOperationalDetail {...detailPropsWithTaskResultAndDemandSummary()} />);

  expect(await screen.findByText("结构化结果")).toBeInTheDocument();
  expect(screen.getByText("revision_needed")).toBeInTheDocument();
  expect(screen.getByText("最终需求总结")).toBeInTheDocument();
  expect(screen.getByText("剩余风险")).toBeInTheDocument();
});
```

- [ ] **Step 2: Run tests and verify they fail**

Run:

```bash
corepack pnpm --filter ./apps/web run test -- src/lib/api/projects.test.ts src/features/projects/index.test.tsx
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/api -run 'TestListProjectTaskResults|TestGetProjectDemandSummary|TestProjectRoutes.*Results' -count=1
```

Expected: FAIL because routes, fetchers, and UI rendering are missing.

- [ ] **Step 3: Add routes and schemas**

Add OpenAPI paths:

```yaml
  /api/v1/projects/{projectId}/tasks/{taskId}/results:
    get:
      operationId: listProjectTaskResults
      summary: List structured ProjectTask results
      parameters:
        - $ref: "#/components/parameters/ProjectId"
        - $ref: "#/components/parameters/ProjectTaskId"
        - $ref: "#/components/parameters/Limit"
        - $ref: "#/components/parameters/Offset"
      responses:
        "200":
          description: ProjectTask result list
  /api/v1/projects/{projectId}/demands/{demandId}/summary:
    get:
      operationId: getProjectDemandSummary
      summary: Get latest final demand summary
      parameters:
        - $ref: "#/components/parameters/ProjectId"
        - name: demandId
          in: path
          required: true
          schema:
            type: string
            format: uuid
      responses:
        "200":
          description: Final demand summary
```

Register routes in `apps/control-plane/internal/api/server.go` beside existing project routes.

- [ ] **Step 4: Add Web fetchers and rendering**

In `apps/web/src/lib/api/projects.ts`:

```ts
export type ProjectTaskResult = {
  id: string;
  project_id: string;
  project_task_id: string;
  attempt_id?: string;
  execution_summary_id?: string;
  result_status: "completed" | "revision_needed" | "blocked" | "failed" | "cancelled";
  validation_status: "accepted" | "rejected";
  decision: string;
  contract: TaskResultContract;
  validation_errors: string[];
  decision_request_id?: string;
  revision_task_id?: string;
  created_at: string;
};

export type ProjectDemandSummary = {
  id: string;
  project_id: string;
  demand_id: string;
  status: string;
  conclusion: string;
  summary_payload: Record<string, unknown>;
  report_ref_id?: string;
  acceptance_required: boolean;
  created_at: string;
};

export async function listProjectTaskResults(projectId: string, taskId: string, options: ApiOptions = {}) {
  const response = await apiFetch<{ items: ProjectTaskResult[] }>(
    `/api/v1/projects/${projectId}/tasks/${taskId}/results?limit=20&offset=0`,
    options,
  );
  return response.items;
}

export async function getProjectDemandSummary(projectId: string, demandId: string, options: ApiOptions = {}) {
  return apiFetch<ProjectDemandSummary>(
    `/api/v1/projects/${projectId}/demands/${demandId}/summary`,
    options,
  );
}
```

In `project-operational-detail.tsx`, add a compact panel titled `结构化结果` in the existing side column and a `最终需求总结` panel near execution summaries. Show status, decision, top evidence refs, risks, and validation errors. Keep the existing restrained console layout and do not introduce a new page.

- [ ] **Step 5: Run tests**

Run:

```bash
corepack pnpm generate:control-plane
corepack pnpm verify:contracts
corepack pnpm --filter ./apps/web run test -- src/lib/api/projects.test.ts src/features/projects/index.test.tsx
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/api -run 'TestListProjectTaskResults|TestGetProjectDemandSummary|TestProjectRoutes.*Results' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add contracts/control-plane/openapi.yaml apps/control-plane/internal/api apps/control-plane/internal/project apps/web/src/lib/api/projects.ts apps/web/src/lib/api/projects.test.ts apps/web/src/features/projects/index.tsx apps/web/src/features/projects/components/project-operational-detail.tsx apps/web/src/features/projects/index.test.tsx
git commit -m "feat: expose project task results and final summaries"
```

### Task 9: Integration Verification And Changelog

**Files:**
- Modify: `CHANGELOG.md`
- No code files unless verification exposes a defect.

- [ ] **Step 1: Run backend and contract verification**

Run:

```bash
corepack pnpm verify:contracts
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/workflow/projectcoordination ./apps/control-plane/internal/api ./apps/control-plane/internal/storage -count=1
```

Expected: PASS.

- [ ] **Step 2: Run Runtime Agent verification**

Run:

```bash
cargo test --manifest-path apps/runtime-agent/Cargo.toml project_task -- --nocapture
```

Expected: PASS, including structured result contract writeback tests.

- [ ] **Step 3: Run Web verification**

Run:

```bash
corepack pnpm --filter ./apps/web run test
corepack pnpm --filter ./apps/web run typecheck
```

Expected: PASS.

- [ ] **Step 4: Add changelog entry**

Run:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Add the returned timestamp to `CHANGELOG.md`:

```markdown
## 2026-06-22 HH:MM

- Added Phase 4 dynamic project planning result handling: structured ProjectTask result contracts, append-only result records, revision/replan decision paths, final demand summaries, Runtime result writeback support, and project console visibility.
```

Use the exact `HH:MM` printed by the date command.

- [ ] **Step 5: Run hygiene**

Run:

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add CHANGELOG.md
git commit -m "chore: document dynamic planning phase 4 result handling"
```

### Task 10: Real End-To-End Smoke

**Files:**
- No source edits unless the smoke exposes a defect.

- [ ] **Step 1: Confirm services and restart changed surfaces**

Run:

```bash
scripts/dev-services.sh status
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh restart runtime-agent
scripts/dev-services.sh restart web
scripts/dev-services.sh status
```

Expected:

- Temporal is running
- Control Plane is running current branch code
- Runtime Agent is running current branch code
- Web is running current branch code

- [ ] **Step 2: Apply migrations against the intended local development database**

Run only after confirming `DATABASE_URL` points at the intended local development database:

```bash
DATABASE_URL="$DATABASE_URL" make -C apps/control-plane migrate-status
DATABASE_URL="$DATABASE_URL" make -C apps/control-plane migrate-up
DATABASE_URL="$DATABASE_URL" make -C apps/control-plane migrate-status
```

Expected:

- migration `034_project_task_results.sql` is applied
- `project_task_results` and `project_demand_summaries` exist

- [ ] **Step 3: Execute a real dependent-task smoke**

Create or use a test project demand that decomposes into at least two dependent tasks:

1. root task has acceptance criterion `输出结论`
2. downstream task depends on root task
3. Runtime Agent receives the root task through the existing project-task dispatch path
4. Provider output includes:

```json
{
  "result_contract": {
    "status": "completed",
    "summary": "根任务完成，输出可复核结论。",
    "acceptance_results": [
      {
        "criterion": "输出结论",
        "status": "passed",
        "evidence_refs": ["artifact:root-result"]
      }
    ],
    "evidence_refs": [
      {
        "type": "runtime_artifact",
        "ref": "artifact:root-result",
        "summary": "根任务结果"
      }
    ],
    "artifact_refs": [],
    "changes_made": [],
    "verification": [
      {
        "type": "runtime_provider_smoke",
        "status": "passed",
        "summary": "真实 Runtime/Provider 写回成功"
      }
    ],
    "risks": [],
    "follow_up_requests": []
  }
}
```

Expected:

- Control Plane accepts the structured result
- `project_task_results` has one accepted completed result for the root task
- root task transitions to `completed`
- downstream task moves through Phase 3 `PreDispatchGate`
- downstream task is not released from free-text summary alone

- [ ] **Step 4: Execute revision/replan smoke**

Submit a second result for a safe test task with:

```json
{
  "result_contract": {
    "status": "revision_needed",
    "summary": "缺少浏览器真实验证证据。",
    "acceptance_results": [
      {
        "criterion": "真实浏览器验证",
        "status": "failed",
        "evidence_refs": []
      }
    ],
    "evidence_refs": [],
    "artifact_refs": [],
    "changes_made": [],
    "verification": [
      {
        "type": "browser_smoke",
        "status": "failed",
        "summary": "页面没有加载当前数据"
      }
    ],
    "risks": [
      {
        "level": "medium",
        "description": "未完成真实浏览器验证"
      }
    ],
    "follow_up_requests": [],
    "revision_request": {
      "reason": "补充真实浏览器验证",
      "contract_changed": true,
      "recommended_task_title": "补充浏览器验证",
      "recommended_task_summary": "打开真实 Web 并验证当前 Control Plane 数据"
    }
  }
}
```

Expected:

- downstream remains blocked
- append-only revision task is created with `revision_of_task_id`
- old task/result/attempt remain readable
- no old task contract is overwritten

- [ ] **Step 5: Verify final summary**

Complete all required smoke tasks or choose the human stop decision for blocked/failed/cancelled test tasks. Then verify:

```bash
curl -sS "$CONTROL_PLANE_URL/api/v1/projects/$PROJECT_ID/demands/$DEMAND_ID/summary" \
  -H "Authorization: Bearer $TOKEN" | jq .
```

Expected:

- response status is 200
- `summary_payload.original_goal` matches the demand
- `summary_payload.completed_tasks` includes completed tasks
- `summary_payload.unfinished_tasks` includes blocked/failed/cancelled tasks only when a human stop decision exists
- `summary_payload.evidence_refs` includes `artifact:root-result`
- `summary_payload.risks` includes remaining risks

- [ ] **Step 6: Browser verification**

Use the Chrome plug/browser automation against the running Web:

1. Open the project operational detail page.
2. Confirm the task list shows latest result status.
3. Confirm `结构化结果` shows the result decision and evidence refs.
4. Confirm `最终需求总结` shows conclusion, task status groups, evidence, and risks.
5. Confirm no panel is stuck loading or rendering mock-only data.

Expected: visible data comes from the real Control Plane and current database.

- [ ] **Step 7: Final hygiene**

Run:

```bash
git diff --check
git status --short
```

Expected:

- `git diff --check` has no output
- `git status --short` only shows intended implementation files

If all local and real-chain checks pass, the branch is ready for code review. If real Runtime/Provider smoke cannot run because credentials, auth, service startup, or safe workspace setup is missing, stop and report the blocker; do not claim Phase 4 is usable.

## Self-Review Checklist

- Spec coverage:
  - `TaskResultContract` is covered in Tasks 1, 3, and 4.
  - Acceptance criterion validation is covered in Task 1 and Task 4.
  - Completed downstream unlock through Phase 3 gate is covered in Task 5 and Task 10.
  - Revision task and same-task retry paths are covered in Tasks 5 and 6.
  - Blocked, failed, cancelled, and human review paths are covered in Tasks 4, 5, and 6.
  - Append-only plan-level replan is covered in Task 6.
  - Final demand summary is covered in Task 7 and Task 10.
  - Web visibility is covered in Task 8.
  - Real Runtime/Provider smoke is covered in Task 10.
- Forbidden-marker scan:
  - The plan uses concrete file paths, commands, schema snippets, request structs, tests, and expected outcomes.
  - The plan avoids future-fill markers and undefined implementation slots.
- Type consistency:
  - Result status values match the phase spec: `completed`, `revision_needed`, `blocked`, `failed`, `cancelled`.
  - Result decisions are consistently named as `TaskResultDecision*`.
  - Persistence names use `project_task_results` and `project_demand_summaries`.
  - Runtime payload field is consistently named `result_contract`.

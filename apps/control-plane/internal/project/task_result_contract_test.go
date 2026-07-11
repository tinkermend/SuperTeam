package project

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestValidateTaskResultContract(t *testing.T) {
	t.Run("completed result with all acceptance criteria passes", func(t *testing.T) {
		validation := ValidateTaskResultContract(taskResultContractTask(), completeTaskResultContract())

		require.True(t, validation.Valid)
		require.Empty(t, validation.Errors)
		require.Equal(t, TaskResultDecisionCompleteAccepted, validation.Decision)
	})

	t.Run("completed result missing acceptance criterion fails with acceptance_result_missing:说明剩余风险 and TaskResultDecisionValidationFailed", func(t *testing.T) {
		result := completeTaskResultContract()
		result.AcceptanceResults = result.AcceptanceResults[:1]

		validation := ValidateTaskResultContract(taskResultContractTask(), result)

		require.False(t, validation.Valid)
		require.Contains(t, validation.Errors, "acceptance_result_missing:说明剩余风险")
		require.Equal(t, TaskResultDecisionValidationFailed, validation.Decision)
	})

	t.Run("completed result requires evidence on every required acceptance criterion result", func(t *testing.T) {
		result := completeTaskResultContract()
		result.AcceptanceResults[1].EvidenceRefs = nil

		validation := ValidateTaskResultContract(taskResultContractTask(), result)

		require.False(t, validation.Valid)
		require.Contains(t, validation.Errors, "acceptance_result_evidence_missing:说明剩余风险")
		require.Equal(t, TaskResultDecisionValidationFailed, validation.Decision)
	})

	t.Run("completed result rejects blank top-level evidence refs", func(t *testing.T) {
		result := completeTaskResultContract()
		result.EvidenceRefs = []TaskResultRef{
			{Kind: "trace", Ref: "   "},
		}

		validation := ValidateTaskResultContract(taskResultContractTask(), result)

		require.False(t, validation.Valid)
		require.Contains(t, validation.Errors, "expected_output_missing:evidence_refs")
		require.Equal(t, TaskResultDecisionValidationFailed, validation.Decision)
	})

	t.Run("completed result rejects mixed valid and blank top-level evidence refs", func(t *testing.T) {
		result := completeTaskResultContract()
		result.EvidenceRefs = []TaskResultRef{
			{Kind: "trace", Ref: "evidence://task/trace"},
			{Kind: "trace", Ref: "   "},
		}

		validation := ValidateTaskResultContract(taskResultContractTask(), result)

		require.False(t, validation.Valid)
		require.Contains(t, validation.Errors, "evidence_ref_blank")
		require.Equal(t, TaskResultDecisionValidationFailed, validation.Decision)
	})

	t.Run("completed result rejects blank artifact refs when artifacts are required", func(t *testing.T) {
		result := completeTaskResultContract()
		result.ArtifactRefs = []TaskResultRef{
			{Kind: "report", Ref: "   "},
		}

		validation := ValidateTaskResultContract(taskResultContractTask(), result)

		require.False(t, validation.Valid)
		require.Contains(t, validation.Errors, "expected_output_missing:artifact_refs")
		require.Equal(t, TaskResultDecisionValidationFailed, validation.Decision)
	})

	t.Run("completed result rejects mixed valid and blank artifact refs", func(t *testing.T) {
		result := completeTaskResultContract()
		result.ArtifactRefs = []TaskResultRef{
			{Kind: "report", Ref: "artifact://task/report"},
			{Kind: "report", Ref: "   "},
		}

		validation := ValidateTaskResultContract(taskResultContractTask(), result)

		require.False(t, validation.Valid)
		require.Contains(t, validation.Errors, "artifact_ref_blank")
		require.Equal(t, TaskResultDecisionValidationFailed, validation.Decision)
	})

	t.Run("completed result rejects blank acceptance criterion evidence refs", func(t *testing.T) {
		result := completeTaskResultContract()
		result.AcceptanceResults[1].EvidenceRefs = []string{"   "}

		validation := ValidateTaskResultContract(taskResultContractTask(), result)

		require.False(t, validation.Valid)
		require.Contains(t, validation.Errors, "acceptance_result_evidence_missing:说明剩余风险")
		require.Equal(t, TaskResultDecisionValidationFailed, validation.Decision)
	})

	t.Run("completed result rejects mixed valid and blank acceptance criterion evidence refs", func(t *testing.T) {
		result := completeTaskResultContract()
		result.AcceptanceResults[1].EvidenceRefs = []string{"evidence://risk-summary", "   "}

		validation := ValidateTaskResultContract(taskResultContractTask(), result)

		require.False(t, validation.Valid)
		require.Contains(t, validation.Errors, "acceptance_result_evidence_blank:说明剩余风险")
		require.Equal(t, TaskResultDecisionValidationFailed, validation.Decision)
	})

	t.Run("completed result rejects unknown verification status", func(t *testing.T) {
		result := completeTaskResultContract()
		result.Verification = []TaskResultVerification{
			{Status: TaskResultVerificationStatus("bogus")},
		}

		validation := ValidateTaskResultContract(taskResultContractTask(), result)

		require.False(t, validation.Valid)
		require.Contains(t, validation.Errors, "verification_status_invalid:bogus")
		require.Equal(t, TaskResultDecisionValidationFailed, validation.Decision)
	})

	t.Run("completed result rejects failed verification", func(t *testing.T) {
		result := completeTaskResultContract()
		result.Verification = []TaskResultVerification{
			{Status: TaskResultVerificationFailed},
		}

		validation := ValidateTaskResultContract(taskResultContractTask(), result)

		require.False(t, validation.Valid)
		require.Contains(t, validation.Errors, "verification_failed")
		require.Equal(t, TaskResultDecisionValidationFailed, validation.Decision)
	})

	t.Run("non-completed result rejects unknown verification status when verification is present", func(t *testing.T) {
		result := TaskResultContract{
			Status:  TaskResultStatusBlocked,
			Summary: "等待负责人补充凭据。",
			Blocker: &TaskResultBlocker{
				Reason:     "缺少凭据",
				RequiredBy: "human",
			},
			Verification: []TaskResultVerification{
				{Status: TaskResultVerificationStatus("bogus")},
			},
		}

		validation := ValidateTaskResultContract(ProjectTask{}, result)

		require.False(t, validation.Valid)
		require.Contains(t, validation.Errors, "verification_status_invalid:bogus")
		require.Equal(t, TaskResultDecisionValidationFailed, validation.Decision)
	})

	t.Run("non-completed result allows failed verification evidence", func(t *testing.T) {
		result := TaskResultContract{
			Status:  TaskResultStatusBlocked,
			Summary: "等待负责人补充凭据。",
			Blocker: &TaskResultBlocker{
				Reason:     "缺少凭据",
				RequiredBy: "human",
			},
			Verification: []TaskResultVerification{
				{Status: TaskResultVerificationFailed},
			},
		}

		validation := ValidateTaskResultContract(ProjectTask{}, result)

		require.True(t, validation.Valid)
		require.Empty(t, validation.Errors)
		require.Equal(t, TaskResultDecisionBlockedWaitingHuman, validation.Decision)
	})

	t.Run("acceptance criteria maps with required false are optional", func(t *testing.T) {
		task := taskResultContractTask()
		task.HandoffContract = map[string]any{
			"acceptance_criteria": []any{
				map[string]any{
					"name":     "可选截图",
					"required": false,
				},
				map[string]any{
					"name": "说明剩余风险",
				},
			},
		}
		result := completeTaskResultContract()
		result.AcceptanceResults = []TaskResultAcceptanceResult{
			{
				Criterion:    "说明剩余风险",
				Status:       TaskResultCriterionPassed,
				EvidenceRefs: []string{"evidence://risk-summary"},
			},
		}

		validation := ValidateTaskResultContract(task, result)

		require.True(t, validation.Valid)
		require.Empty(t, validation.Errors)
	})

	t.Run("revision_needed requires a revision reason", func(t *testing.T) {
		result := TaskResultContract{
			Status:  TaskResultStatusRevisionNeeded,
			Summary: "实现与验收标准存在偏差，需要补充一次修订。",
			RevisionRequest: &TaskResultRevisionRequest{
				ContractChanged: false,
			},
		}

		validation := ValidateTaskResultContract(ProjectTask{}, result)

		require.False(t, validation.Valid)
		require.Contains(t, validation.Errors, "revision_reason_required")
		require.Equal(t, TaskResultDecisionValidationFailed, validation.Decision)
	})

	t.Run("blocked with blocker and human review maps to TaskResultDecisionBlockedWaitingHuman", func(t *testing.T) {
		result := TaskResultContract{
			Status:  TaskResultStatusBlocked,
			Summary: "缺少生产只读凭据，无法核对最终状态。",
			Blocker: &TaskResultBlocker{
				Reason:           "需要人类提供生产只读凭据",
				RequiredBy:       "human",
				ResolutionPrompt: "请提供生产只读凭据或确认改用脱敏导出",
			},
			HumanReviewRequest: &TaskResultHumanReviewRequest{
				Reason:     "请负责人确认是否提供凭据或改用脱敏导出",
				Prompt:     "是否提供凭据？",
				Options:    []string{"提供凭据", "使用脱敏导出"},
				RequiredBy: "human",
			},
		}

		validation := ValidateTaskResultContract(ProjectTask{}, result)

		require.True(t, validation.Valid)
		require.Equal(t, TaskResultDecisionBlockedWaitingHuman, validation.Decision)
	})

	t.Run("failed retryable maps to TaskResultDecisionFailedRetryable when retry budget remains", func(t *testing.T) {
		retryable := true
		result := TaskResultContract{
			Status:  TaskResultStatusFailed,
			Summary: "Provider 会话中断，当前尝试没有产出完整结果。",
			Failure: &TaskResultFailure{
				ErrorFamily:            "provider_interrupted",
				Retryable:              &retryable,
				RecoveryRecommendation: "重新领取任务并复用已有上下文重试",
			},
		}
		task := ProjectTask{AttemptCount: 1, MaxAttempts: int32Ptr(3)}

		validation := ValidateTaskResultContract(task, result)

		require.True(t, validation.Valid)
		require.Equal(t, TaskResultDecisionFailedRetryable, validation.Decision)
	})
}

func TestFailureContractAdapter(t *testing.T) {
	t.Run("legacy failure adapter infers retryable false for unknown family when retryable is nil", func(t *testing.T) {
		contract := TaskResultContractFromFailure(FailProjectTaskAttemptRequest{
			FailureSummary: "Provider exited before final result.",
			FailureFamily:  "provider_interrupted",
		})

		require.Equal(t, TaskResultStatusFailed, contract.Status)
		require.NotNil(t, contract.Failure)
		require.NotNil(t, contract.Failure.Retryable)
		require.False(t, *contract.Failure.Retryable)

		validation := ValidateTaskResultContract(ProjectTask{}, contract)
		require.True(t, validation.Valid)
		require.Equal(t, TaskResultDecisionFailedRecovery, validation.Decision)
	})

	t.Run("legacy failure adapter infers retryable true for transient family when retryable is nil", func(t *testing.T) {
		contract := TaskResultContractFromFailure(FailProjectTaskAttemptRequest{
			FailureSummary: "Runtime lease was interrupted.",
			FailureFamily:  FailureFamilyTransientRuntime,
		})

		require.Equal(t, TaskResultStatusFailed, contract.Status)
		require.NotNil(t, contract.Failure)
		require.NotNil(t, contract.Failure.Retryable)
		require.True(t, *contract.Failure.Retryable)

		validation := ValidateTaskResultContract(ProjectTask{AttemptCount: 1, MaxAttempts: int32Ptr(3)}, contract)
		require.True(t, validation.Valid)
		require.Equal(t, TaskResultDecisionFailedRetryable, validation.Decision)
	})

	t.Run("failed retryable with exhausted task budget maps to failed recovery", func(t *testing.T) {
		retryable := true
		contract := TaskResultContractFromFailure(FailProjectTaskAttemptRequest{
			FailureSummary: "Provider timed out.",
			FailureFamily:  FailureFamilyTimeout,
			Retryable:      &retryable,
		})

		validation := ValidateTaskResultContract(ProjectTask{AttemptCount: 3, MaxAttempts: int32Ptr(3)}, contract)

		require.True(t, validation.Valid)
		require.Equal(t, TaskResultDecisionFailedRecovery, validation.Decision)
	})
}

func TestLegacyCompletionContractAdapter(t *testing.T) {
	t.Run("legacy completion adapter produces completed contract with evidence/artifact refs and follow-up request", func(t *testing.T) {
		contract := TaskResultContractFromLegacyCompletion(CompleteProjectTaskAttemptRequest{
			Conclusion: "已完成数据库核对并产出复盘报告。",
			EvidenceRefs: []any{
				"evidence://project-task/trace-1",
			},
			ArtifactRefs: []any{
				map[string]any{
					"kind": "report",
					"ref":  "artifact://reports/task-1",
				},
			},
			RecommendedNextAction: "请负责人确认是否接受剩余风险。",
			RequiresHumanReview:   true,
		})

		require.Equal(t, TaskResultStatusCompleted, contract.Status)
		require.Equal(t, "已完成数据库核对并产出复盘报告。", contract.Summary)
		require.Len(t, contract.EvidenceRefs, 1)
		require.Equal(t, "evidence", contract.EvidenceRefs[0].Kind)
		require.Equal(t, "evidence://project-task/trace-1", contract.EvidenceRefs[0].Ref)
		require.Len(t, contract.ArtifactRefs, 1)
		require.Equal(t, "report", contract.ArtifactRefs[0].Kind)
		require.Equal(t, "artifact://reports/task-1", contract.ArtifactRefs[0].Ref)
		require.Len(t, contract.FollowUpRequests, 1)
		require.Equal(t, "请负责人确认是否接受剩余风险。", contract.FollowUpRequests[0].Summary)
		require.NotNil(t, contract.HumanReviewRequest)
	})
}

func TestCompletedVerificationRequiresRuntimeAttestationRef(t *testing.T) {
	task := ProjectTask{
		ID:              uuid.New(),
		ExpectedOutputs: []any{"verification"},
		HandoffContract: map[string]any{
			"requires_runtime_attestation": true,
		},
	}
	result := TaskResultContract{
		Status:  TaskResultStatusCompleted,
		Summary: "tests passed",
		Verification: []TaskResultVerification{{
			Status:  TaskResultVerificationStatusPassed,
			Type:    "unit_test",
			Summary: "go test passed",
		}},
	}

	validation := ValidateTaskResultContract(task, result)

	require.False(t, validation.Valid)
	require.Contains(t, validation.Errors, "verification_attestation_ref_required")
}

func TestCompletedVerificationAcceptsRuntimeAttestationRef(t *testing.T) {
	task := ProjectTask{
		ID:              uuid.New(),
		ExpectedOutputs: []any{"verification"},
		HandoffContract: map[string]any{
			"requires_runtime_attestation": true,
		},
	}
	result := TaskResultContract{
		Status:  TaskResultStatusCompleted,
		Summary: "tests passed",
		Verification: []TaskResultVerification{{
			Status:  TaskResultVerificationStatusPassed,
			Type:    "unit_test",
			Summary: "go test passed",
			EvidenceRefs: []TaskResultRef{{
				Kind: "attestation",
				Type: "runtime_command",
				Ref:  "attestation:123",
			}},
		}},
	}

	validation := ValidateTaskResultContract(task, result)

	require.True(t, validation.Valid, "unexpected errors: %#v", validation.Errors)
}

func TestTaskResultContractReviewRequiredExports(t *testing.T) {
	var validation TaskResultValidationResult = ValidateTaskResultContract(ProjectTask{}, TaskResultContract{
		Status:  "unknown",
		Summary: "invalid status should surface as a typed validation error",
	})
	require.False(t, validation.Valid)
	require.NotEmpty(t, validation.Errors)

	var validationError TaskResultValidationError = validation.Errors[0]
	require.Equal(t, TaskResultValidationError("status_invalid:unknown"), validationError)

	contract := AdaptCompletionEvidenceToResultContract(CompleteProjectTaskAttemptRequest{
		Conclusion:   "legacy completion evidence was normalized",
		EvidenceRefs: []any{"evidence://legacy/ref"},
	})
	require.Equal(t, TaskResultStatusCompleted, contract.Status)
	require.Equal(t, "evidence://legacy/ref", contract.EvidenceRefs[0].Ref)

	require.Equal(t, TaskResultDecisionCompleteAccepted, NormalizeTaskResultDecision(ProjectTask{}, completeTaskResultContract()))
}

func TestProjectTaskResultEventConstants(t *testing.T) {
	require.Equal(t, ProjectEventType("project_task.result.submitted"), ProjectEventTaskResultSubmitted)
	require.Equal(t, ProjectEventType("project_task.result.accepted"), ProjectEventTaskResultAccepted)
	require.Equal(t, ProjectEventType("project_task.result.rejected"), ProjectEventTaskResultRejected)
	require.Equal(t, ProjectEventType("project_task.result.blocked"), ProjectEventTaskResultBlocked)
	require.Equal(t, ProjectEventType("project_task.result.retryable_failed"), ProjectEventTaskResultRetryableFailed)
}

func TestTaskResultContractPlanShapeCompatibility(t *testing.T) {
	require.Equal(t, TaskResultCriterionStatusPassed, TaskResultCriterionPassed)
	require.Equal(t, TaskResultCriterionStatusFailed, TaskResultCriterionFailed)
	require.Equal(t, TaskResultCriterionStatusNeedsHuman, TaskResultCriterionNeedsHuman)
	require.Equal(t, TaskResultCriterionStatusNotApplicable, TaskResultCriterionNotApplicable)
	require.Equal(t, TaskResultCriterionStatusHumanOverridden, TaskResultCriterionHumanOverridden)
	require.Equal(t, TaskResultVerificationStatusPassed, TaskResultVerificationPassed)
	require.Equal(t, TaskResultVerificationStatusFailed, TaskResultVerificationFailed)
	require.Equal(t, TaskResultVerificationStatusSkipped, TaskResultVerificationSkipped)

	contract := TaskResultContract{
		Status:  TaskResultStatusCompleted,
		Summary: "shape check",
		ChangesMade: []TaskResultChange{
			{Type: "code", Ref: "git://task-result-contract.go", Summary: "added task result validation"},
		},
		EvidenceRefs: []TaskResultRef{
			{Type: "trace", Summary: "trace evidence", ID: "evidence-1"},
		},
		Risks: []TaskResultRisk{
			{Level: "low", Description: "known residual risk"},
		},
		Verification: []TaskResultVerification{
			{Status: TaskResultVerificationPassed, Type: "go_test", Ref: "go-test://focused", Summary: "focused test passed"},
		},
		HumanReviewRequest: &TaskResultHumanReviewRequest{
			Prompt:  "Accept residual risk?",
			Options: []string{"accept", "request_revision"},
		},
		FollowUpRequests: []TaskResultFollowUpRequest{
			{Type: "human_acceptance", Summary: "review residual risk"},
		},
		RevisionRequest: &TaskResultRevisionRequest{
			Reason:                 "needs scoped correction",
			RecommendedTaskTitle:   "Revise task result contract",
			RecommendedTaskSummary: "Add missing public plan fields",
		},
		ReplanRequest: &TaskResultReplanRequest{
			Reason:      "plan constraint changed",
			Constraints: []string{"preserve runtime scope"},
		},
		Blocker: &TaskResultBlocker{
			ResolutionPrompt: "Provide missing credential or approve sanitized evidence.",
		},
	}

	payload, err := json.Marshal(contract)
	require.NoError(t, err)
	require.JSONEq(t, `{
		"status": "completed",
		"summary": "shape check",
		"changes_made": [{"type": "code", "ref": "git://task-result-contract.go", "summary": "added task result validation"}],
		"evidence_refs": [{"id": "evidence-1", "type": "trace", "summary": "trace evidence"}],
		"risks": [{"level": "low", "description": "known residual risk"}],
		"verification": [{"status": "passed", "type": "go_test", "ref": "go-test://focused", "summary": "focused test passed"}],
		"human_review_request": {"prompt": "Accept residual risk?", "options": ["accept", "request_revision"]},
		"follow_up_requests": [{"type": "human_acceptance", "summary": "review residual risk"}],
		"revision_request": {"reason": "needs scoped correction", "recommended_task_title": "Revise task result contract", "recommended_task_summary": "Add missing public plan fields"},
		"replan_request": {"reason": "plan constraint changed", "constraints": ["preserve runtime scope"]},
		"blocker": {"resolution_prompt": "Provide missing credential or approve sanitized evidence."}
	}`, string(payload))
	require.NotContains(t, string(payload), `"changes"`)

	_, hasLegacyChangesField := reflect.TypeOf(TaskResultContract{}).FieldByName("Changes")
	require.False(t, hasLegacyChangesField)

	retryable := false
	partialPayload, err := json.Marshal(TaskResultContract{
		Status:  TaskResultStatusFailed,
		Summary: "partial contract",
		Failure: &TaskResultFailure{
			ErrorFamily:            "provider_interrupted",
			Retryable:              &retryable,
			RecoveryRecommendation: "manual_recovery_required",
		},
	})
	require.NoError(t, err)
	require.NotContains(t, string(partialPayload), "changes_made")
	require.NotContains(t, string(partialPayload), "verification")
}

func TestTaskResultContractUnmarshalAcceptsSingularEvidenceRef(t *testing.T) {
	var contract TaskResultContract
	err := json.Unmarshal([]byte(`{
		"status": "completed",
		"summary": "runtime completed",
		"evidence_ref": {"type": "runtime_command", "ref": "runtime-command://cmd-1"},
		"acceptance_results": [
			{"criterion": "输出结论", "status": "passed", "evidence_ref": "runtime-command://cmd-1"}
		],
		"verification": [
			{"type": "runtime_smoke", "status": "passed", "evidence_ref": {"type": "runtime_command", "ref": "runtime-command://cmd-1"}}
		]
	}`), &contract)
	require.NoError(t, err)
	require.Len(t, contract.EvidenceRefs, 1)
	require.Equal(t, "runtime-command://cmd-1", contract.EvidenceRefs[0].Ref)
	require.Equal(t, []string{"runtime-command://cmd-1"}, contract.AcceptanceResults[0].EvidenceRefs)
	require.Len(t, contract.Verification[0].EvidenceRefs, 1)
	require.Equal(t, "runtime-command://cmd-1", contract.Verification[0].EvidenceRefs[0].Ref)
}

func taskResultContractTask() ProjectTask {
	return ProjectTask{
		ExpectedOutputs: []any{
			"execution_summary",
			"evidence_refs",
			"artifact_refs",
			"verification",
		},
		HandoffContract: map[string]any{
			"acceptance_criteria": []any{
				"SQL 核对",
				"说明剩余风险",
			},
		},
	}
}

func TestMapTaskResultDecisionBlockedResolvableWhenMissingInputsKnown(t *testing.T) {
	task := ProjectTask{
		InputRequirements: map[string]any{"required_inputs": []any{"load_test_report"}},
	}
	result := TaskResultContract{
		Status:  TaskResultStatusBlocked,
		Summary: "no load test report",
		Blocker: &TaskResultBlocker{
			Reason:        "no load test report",
			MissingInputs: []string{"load_test_report"},
		},
	}

	decision := mapTaskResultDecision(task, result)
	require.Equal(t, TaskResultDecisionBlockedResolvableUpstream, decision)
}

func TestMapTaskResultDecisionBlockedFallsBackToHumanWhenInputNotDeclared(t *testing.T) {
	task := ProjectTask{
		InputRequirements: map[string]any{"required_inputs": []any{"load_test_report"}},
	}
	result := TaskResultContract{
		Status:  TaskResultStatusBlocked,
		Summary: "need something else",
		Blocker: &TaskResultBlocker{
			Reason:        "need something else",
			MissingInputs: []string{"undisclosed_thing"},
		},
	}

	decision := mapTaskResultDecision(task, result)
	require.Equal(t, TaskResultDecisionBlockedWaitingHuman, decision)
}

func completeTaskResultContract() TaskResultContract {
	return TaskResultContract{
		Status:  TaskResultStatusCompleted,
		Summary: "已完成数据库核对，列出剩余低风险项并给出负责人验收建议。",
		AcceptanceResults: []TaskResultAcceptanceResult{
			{
				Criterion:    "SQL 核对",
				Status:       TaskResultCriterionStatusPassed,
				EvidenceRefs: []string{"evidence://sql/project-count"},
			},
			{
				Criterion:    "说明剩余风险",
				Status:       TaskResultCriterionStatusPassed,
				EvidenceRefs: []string{"evidence://risk-summary"},
			},
		},
		EvidenceRefs: []TaskResultRef{
			{Kind: "trace", Ref: "evidence://task/trace"},
		},
		ArtifactRefs: []TaskResultRef{
			{Kind: "report", Ref: "artifact://task/report"},
		},
		Verification: []TaskResultVerification{
			{
				Status:  TaskResultVerificationStatusPassed,
				Type:    "go_test",
				Ref:     "go-test://focused",
				Summary: "go test focused package passed",
			},
		},
	}
}

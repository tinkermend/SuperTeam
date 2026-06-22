package project

import (
	"testing"

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
				Reason:     "需要人类提供生产只读凭据",
				RequiredBy: "human",
			},
			HumanReviewRequest: &TaskResultHumanReviewRequest{
				Reason:     "请负责人确认是否提供凭据或改用脱敏导出",
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

func completeTaskResultContract() TaskResultContract {
	return TaskResultContract{
		Status:  TaskResultStatusCompleted,
		Summary: "已完成数据库核对，列出剩余低风险项并给出负责人验收建议。",
		AcceptanceResults: []TaskResultAcceptanceResult{
			{
				Criterion: "SQL 核对",
				Status:    TaskResultCriterionStatusPassed,
				EvidenceRefs: []TaskResultRef{
					{Kind: "query", Ref: "evidence://sql/project-count"},
				},
			},
			{
				Criterion: "说明剩余风险",
				Status:    TaskResultCriterionStatusPassed,
				EvidenceRefs: []TaskResultRef{
					{Kind: "report", Ref: "evidence://risk-summary"},
				},
			},
		},
		EvidenceRefs: []TaskResultRef{
			{Kind: "trace", Ref: "evidence://task/trace"},
		},
		ArtifactRefs: []TaskResultRef{
			{Kind: "report", Ref: "artifact://task/report"},
		},
		Verification: TaskResultVerification{
			Status:  TaskResultVerificationStatusPassed,
			Summary: "go test focused package passed",
		},
	}
}

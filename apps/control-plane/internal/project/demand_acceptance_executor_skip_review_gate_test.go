package project

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestExecutorSkipsReviewGate: the executor verdict projection must NOT write a
// demand_criterion_verdicts row for a review_gate criterion — the detector gate
// owns that channel (review-flagged cleanliness/safety). A control automated_test
// criterion on the same task IS projected, proving the loop otherwise runs; only
// the review_gate branch is skipped.
func TestExecutorSkipsReviewGate(t *testing.T) {
	repo := newMemoryRepository()
	service, err := NewService(repo)
	require.NoError(t, err)

	tenantID, projectID := uuid.New(), uuid.New()
	demandID, revisionID := uuid.New(), uuid.New()
	employeeID := uuid.New()
	taskKey := "impl_task"

	// Both criteria are satisfied_by the SAME task key, so criteriaSatisfiedByTask
	// scopes both onto this task.
	rgCriterion := DemandAcceptanceCriterion{
		TenantID: tenantID, ProjectID: projectID, DemandID: demandID, PlanRevisionID: revisionID,
		CriterionID:        "crit_rg",
		Statement:          "交付未引入安全违规",
		VerificationMethod: demandCriterionVerificationMethodReviewGate,
		Severity:           demandAcceptanceCriterionSeverityBlocking,
		SatisfiedBy:        []string{taskKey},
	}
	atCriterion := DemandAcceptanceCriterion{
		TenantID: tenantID, ProjectID: projectID, DemandID: demandID, PlanRevisionID: revisionID,
		CriterionID:        "crit_at",
		Statement:          "单元测试全部通过",
		VerificationMethod: "automated_test",
		Severity:           demandAcceptanceCriterionSeverityBlocking,
		SatisfiedBy:        []string{taskKey},
	}
	repo.demandAcceptanceCriteria = append(repo.demandAcceptanceCriteria, rgCriterion, atCriterion)

	key := taskKey
	task := ProjectTask{
		ID:                        uuid.New(),
		TenantID:                  tenantID,
		ProjectID:                 projectID,
		DemandID:                  &demandID,
		AcceptedPlanRevisionID:    &revisionID,
		PlannedTaskKey:            &key,
		AssignedDigitalEmployeeID: &employeeID,
	}

	// The executor self-reports BOTH criteria. The review_gate self-report must be
	// ignored; the automated_test not_applicable self-report projects (not_applicable
	// avoids the satisfied-attestation lookup, keeping the control simple).
	contract := TaskResultContract{
		Status: TaskResultStatusCompleted,
		AcceptanceResults: []TaskResultAcceptanceResult{
			{CriterionID: "crit_rg", Status: TaskResultCriterionStatusPassed, Summary: "执行者自报通过（应被忽略）"},
			{CriterionID: "crit_at", Status: TaskResultCriterionStatusNotApplicable, Summary: "不适用"},
		},
	}

	require.NoError(t, service.projectDemandCriterionVerdicts(context.Background(), task, ProjectTaskAttemptRuntimeRequest{}, contract))

	verdicts, err := repo.ListDemandCriterionVerdicts(context.Background(), tenantID, demandID, revisionID)
	require.NoError(t, err)

	var rgRows, atRows int
	for _, v := range verdicts {
		switch v.CriterionID {
		case "crit_rg":
			rgRows++
		case "crit_at":
			atRows++
		}
	}
	require.Zero(t, rgRows, "executor must not write a verdict for a review_gate criterion")
	require.Equal(t, 1, atRows, "control automated_test criterion should still be projected")
}

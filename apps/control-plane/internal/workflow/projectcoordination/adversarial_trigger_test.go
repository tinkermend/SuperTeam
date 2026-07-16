package projectcoordination

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/superteam/control-plane/internal/project"
)

// fakeAdversarialStore is the narrow store seam for AdversarialReviewForTask
// Activity tests. It embeds ActivityStore (nil) only to satisfy assignment to
// Activities.store; the trigger path only ever calls the two methods below.
type fakeAdversarialStore struct {
	ActivityStore
	plan       AdversarialReviewPlan
	prepareErr error
	persistErr error
	persisted  []PersistAdversarialOutcomeInput
}

func (f *fakeAdversarialStore) PrepareAdversarialReview(ctx context.Context, input PrepareAdversarialReviewInput) (AdversarialReviewPlan, error) {
	return f.plan, f.prepareErr
}

func (f *fakeAdversarialStore) PersistAdversarialOutcome(ctx context.Context, input PersistAdversarialOutcomeInput) error {
	if f.persistErr != nil {
		return f.persistErr
	}
	f.persisted = append(f.persisted, input)
	return nil
}

func adversarialItem(criterionID string, budgetExhausted bool) AdversarialReviewItem {
	return AdversarialReviewItem{
		DemandID:       uuid.New(),
		PlanRevisionID: uuid.New(),
		Input: RunAdversarialReviewInput{
			CriterionID:     criterionID,
			ReviewedTaskID:  uuid.New(),
			Assertion:       "登录接口 p95 延迟低于 200ms",
			EvidenceSummary: "压测报告 p95=180ms",
			BudgetExhausted: budgetExhausted,
		},
	}
}

// stubAdversarialRepo is a minimal project.Repository for PrepareAdversarialReview
// unit tests: it answers only GetProjectTask / ListDemandAcceptanceCriteria /
// GetProject and inherits nil (panic-on-call) for everything else via the
// embedded interface — none of which PrepareAdversarialReview reaches for a task
// with no LatestTaskResultID / CoordinationJobID.
type stubAdversarialRepo struct {
	project.Repository
	task     project.ProjectTask
	criteria []project.DemandAcceptanceCriterion
}

func (r *stubAdversarialRepo) GetProjectTask(ctx context.Context, tenantID, projectTaskID uuid.UUID) (project.ProjectTask, error) {
	return r.task, nil
}

func (r *stubAdversarialRepo) ListDemandAcceptanceCriteria(ctx context.Context, tenantID, demandID, planRevisionID uuid.UUID) ([]project.DemandAcceptanceCriterion, error) {
	return r.criteria, nil
}

func (r *stubAdversarialRepo) GetProject(ctx context.Context, tenantID, projectID uuid.UUID) (project.Project, error) {
	return project.Project{TenantID: tenantID, ID: projectID, CoordinationPolicy: map[string]any{}}, nil
}

func stubAdversarialTask(tenantID, projectID, demandID, planRevisionID uuid.UUID, plannedKey string) project.ProjectTask {
	key := plannedKey
	return project.ProjectTask{
		ID:                     uuid.New(),
		TenantID:               tenantID,
		ProjectID:              projectID,
		DemandID:               &demandID,
		AcceptedPlanRevisionID: &planRevisionID,
		PlannedTaskKey:         &key,
	}
}

func adversarialCriterionRow(demandID, planRevisionID uuid.UUID, id, method, satisfiedBy string) project.DemandAcceptanceCriterion {
	return project.DemandAcceptanceCriterion{
		DemandID:           demandID,
		PlanRevisionID:     planRevisionID,
		CriterionID:        id,
		Statement:          "断言 " + id,
		VerificationMethod: method,
		Severity:           CriterionSeverityBlocking,
		SatisfiedBy:        []string{satisfiedBy},
	}
}

// TestAdversarialTriggerOnlyForReviewedTask: PrepareAdversarialReview scopes to
// adversarial_review criteria the completed task's planned KEY satisfies. A task
// on the hook for such a criterion is Reviewed with exactly that criterion; a
// task that satisfies no adversarial_review criterion is not Reviewed.
func TestAdversarialTriggerOnlyForReviewedTask(t *testing.T) {
	tenantID, projectID := uuid.New(), uuid.New()
	demandID, planRevisionID := uuid.New(), uuid.New()

	criteria := []project.DemandAcceptanceCriterion{
		adversarialCriterionRow(demandID, planRevisionID, "crit_adv", VerificationMethodAdversarialReview, "perf_task"),
		adversarialCriterionRow(demandID, planRevisionID, "crit_other", VerificationMethodAdversarialReview, "other_task"),
		adversarialCriterionRow(demandID, planRevisionID, "crit_ht", "automated_test", "perf_task"),
	}

	// Reviewed task: planned key "perf_task" matches exactly one adversarial_review criterion.
	reviewed := &stubAdversarialRepo{
		task:     stubAdversarialTask(tenantID, projectID, demandID, planRevisionID, "perf_task"),
		criteria: criteria,
	}
	store := &ProjectStore{repository: reviewed}
	plan, err := store.PrepareAdversarialReview(context.Background(), PrepareAdversarialReviewInput{
		TenantID: tenantID, ProjectID: projectID, CompletedTaskID: reviewed.task.ID,
	})
	require.NoError(t, err)
	require.True(t, plan.Reviewed)
	require.Len(t, plan.Items, 1)
	require.Equal(t, "crit_adv", plan.Items[0].Input.CriterionID)
	require.Equal(t, "断言 crit_adv", plan.Items[0].Input.Assertion)

	// Non-satisfied_by task: planned key "unrelated" matches no adversarial criterion.
	unrelated := &stubAdversarialRepo{
		task:     stubAdversarialTask(tenantID, projectID, demandID, planRevisionID, "unrelated"),
		criteria: criteria,
	}
	storeUnrelated := &ProjectStore{repository: unrelated}
	plan, err = storeUnrelated.PrepareAdversarialReview(context.Background(), PrepareAdversarialReviewInput{
		TenantID: tenantID, ProjectID: projectID, CompletedTaskID: unrelated.task.ID,
	})
	require.NoError(t, err)
	require.False(t, plan.Reviewed)
	require.Empty(t, plan.Items)

	// And the orchestrating Activity turns "not reviewed" into a no-op result:
	// the judge is never called, nothing is persisted.
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{{content: acceptedJSON("ok")}}}
	fake := &fakeAdversarialStore{plan: AdversarialReviewPlan{Reviewed: false}}
	activities := &Activities{store: fake, judgeClient: client}
	result, err := activities.AdversarialReviewForTask(context.Background(), AdversarialReviewForTaskInput{
		TenantID: tenantID, ProjectID: projectID, CompletedTaskID: uuid.New(),
	})
	require.NoError(t, err)
	require.False(t, result.Reviewed)
	require.Empty(t, fake.persisted)
	require.Equal(t, 0, client.calls)
}

// TestAdversarialReviewForTaskProjectsAggregateAndDetails: a satisfied review
// persists an aggregate verdict (satisfied) plus per-lens detail rows, and the
// summary reports AllSatisfied.
func TestAdversarialReviewForTaskProjectsAggregateAndDetails(t *testing.T) {
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{
		{content: acceptedJSON("满足")},
		{content: acceptedJSON("满足")},
		{content: acceptedJSON("满足")},
	}}
	fake := &fakeAdversarialStore{plan: AdversarialReviewPlan{Reviewed: true, Items: []AdversarialReviewItem{adversarialItem("crit_adv", false)}}}
	activities := &Activities{store: fake, judgeClient: client, judgeModel: "judge-model"}

	result, err := activities.AdversarialReviewForTask(context.Background(), AdversarialReviewForTaskInput{
		TenantID: uuid.New(), ProjectID: uuid.New(), CompletedTaskID: uuid.New(),
	})
	require.NoError(t, err)
	require.True(t, result.Reviewed)
	require.True(t, result.AllSatisfied)
	require.False(t, result.AnyEscalated)
	require.Len(t, fake.persisted, 1)
	require.Equal(t, AdversarialAggregateSatisfied, fake.persisted[0].Aggregate)
	require.Len(t, fake.persisted[0].Judgements, 3)
	require.Equal(t, "judge-model", client.lastModel())
}

// TestAdversarialEscalateHumanOnBudget: an over-budget task short-circuits to an
// escalate_human aggregate WITHOUT calling any judge; it is persisted (so the
// gate holds) and the summary flags AnyEscalated / not AllSatisfied.
func TestAdversarialEscalateHumanOnBudget(t *testing.T) {
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{{content: acceptedJSON("ok")}}}
	fake := &fakeAdversarialStore{plan: AdversarialReviewPlan{Reviewed: true, Items: []AdversarialReviewItem{adversarialItem("crit_adv", true)}}}
	activities := &Activities{store: fake, judgeClient: client}

	result, err := activities.AdversarialReviewForTask(context.Background(), AdversarialReviewForTaskInput{
		TenantID: uuid.New(), ProjectID: uuid.New(), CompletedTaskID: uuid.New(),
	})
	require.NoError(t, err)
	require.True(t, result.Reviewed)
	require.False(t, result.AllSatisfied)
	require.True(t, result.AnyEscalated)
	require.Len(t, fake.persisted, 1)
	require.Equal(t, AdversarialAggregateEscalateHuman, fake.persisted[0].Aggregate)
	require.Equal(t, 0, client.calls, "budget short-circuit must not call the judge")
}

// TestAdversarialEngineErrorDoesNotReleaseGate: when the judge transport errors,
// the Activity propagates and persists NOTHING for the criterion. The blocking
// adversarial_review criterion therefore stays verdict-less, and the convergence
// gate holds the demand — a review that cannot complete never releases the gate.
func TestAdversarialEngineErrorDoesNotReleaseGate(t *testing.T) {
	transportErr := errors.New("llm unavailable")
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{{err: transportErr}}}
	fake := &fakeAdversarialStore{plan: AdversarialReviewPlan{Reviewed: true, Items: []AdversarialReviewItem{adversarialItem("crit_adv", false)}}}
	activities := &Activities{store: fake, judgeClient: client}

	result, err := activities.AdversarialReviewForTask(context.Background(), AdversarialReviewForTaskInput{
		TenantID: uuid.New(), ProjectID: uuid.New(), CompletedTaskID: uuid.New(),
	})
	require.Error(t, err)
	require.False(t, result.AllSatisfied)
	require.Empty(t, fake.persisted, "an engine error must not persist any verdict")

	// With no persisted adversarial verdict, the gate holds the blocking criterion.
	criterion := project.DemandAcceptanceCriterion{
		CriterionID:        "crit_adv",
		VerificationMethod: VerificationMethodAdversarialReview,
		Severity:           CriterionSeverityBlocking,
	}
	pending := project.ResolveUnsatisfiedBlockingCriteria([]project.DemandAcceptanceCriterion{criterion}, nil)
	require.Equal(t, []string{"crit_adv"}, pending)
}

func TestAdversarialJudgeCountPolicy(t *testing.T) {
	require.Equal(t, 0, adversarialJudgeCount(nil))
	require.Equal(t, 0, adversarialJudgeCount(map[string]any{}))
	require.Equal(t, 5, adversarialJudgeCount(map[string]any{"adversarial_review_judges": 5}))
	require.Equal(t, 5, adversarialJudgeCount(map[string]any{"adversarial_review_judges": float64(5)}))
	require.Equal(t, 0, adversarialJudgeCount(map[string]any{"adversarial_review_judges": 0}))
	require.Equal(t, 0, adversarialJudgeCount(map[string]any{"adversarial_review_judges": "bad"}))
}

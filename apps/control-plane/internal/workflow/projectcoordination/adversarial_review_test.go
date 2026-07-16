package projectcoordination

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// scriptedChatCompletionClient returns a canned response per call (in order) and
// records the system prompt of each call so tests can assert the refute-not-confirm
// framing and per-lens perspective diversity. When more calls arrive than scripted
// responses, the last response is reused.
type scriptedChatCompletionClient struct {
	responses []scriptedResponse
	calls     int
	systems   []string
	users     []string
	models    []string
}

func (c *scriptedChatCompletionClient) lastModel() string {
	if len(c.models) == 0 {
		return ""
	}
	return c.models[len(c.models)-1]
}

type scriptedResponse struct {
	content string
	err     error
}

func (c *scriptedChatCompletionClient) CreateChatCompletion(ctx context.Context, req OpenAICompatibleChatRequest) (string, error) {
	_ = ctx
	c.systems = append(c.systems, req.System)
	c.users = append(c.users, req.User)
	c.models = append(c.models, req.Model)
	idx := c.calls
	c.calls++
	if idx >= len(c.responses) {
		idx = len(c.responses) - 1
	}
	if idx < 0 {
		return "", nil
	}
	r := c.responses[idx]
	return r.content, r.err
}

func acceptedJSON(reason string) string {
	return `{"verdict":"accepted","reason":"` + reason + `"}`
}

func refutedJSON(reason string) string {
	return `{"verdict":"refuted","reason":"` + reason + `"}`
}

func adversarialTestInput() RunAdversarialReviewInput {
	return RunAdversarialReviewInput{
		CriterionID:     "crit_perf",
		ReviewedTaskID:  uuid.New(),
		Assertion:       "登录接口 p95 延迟低于 200ms",
		EvidenceSummary: "压测报告显示 p95=180ms",
		Deliverables:    []string{"load_test_report"},
		EvidenceRefs:    []string{"artifact://report/1"},
	}
}

// Test 1: 3 judges, 2 refute → unsatisfied, RefutedCount=2.
func TestAdversarialMajorityRefuteKills(t *testing.T) {
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{
		{content: refutedJSON("边界未覆盖")},
		{content: refutedJSON("无复现证据")},
		{content: acceptedJSON("看起来满足")},
	}}
	lenses := resolveAdversarialLenses(3)
	result, err := runAdversarialReview(context.Background(), client, lenses, adversarialTestInput())
	require.NoError(t, err)
	require.Equal(t, 3, result.JudgeCount)
	require.Equal(t, 2, result.RefutedCount)
	require.Equal(t, AdversarialAggregateUnsatisfied, result.Aggregate)
	require.Len(t, result.Judgements, 3)
}

// Test 2: 3 judges, 1 refute → satisfied.
func TestAdversarialMinorityRefutePasses(t *testing.T) {
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{
		{content: refutedJSON("边界未覆盖")},
		{content: acceptedJSON("满足")},
		{content: acceptedJSON("满足")},
	}}
	lenses := resolveAdversarialLenses(3)
	result, err := runAdversarialReview(context.Background(), client, lenses, adversarialTestInput())
	require.NoError(t, err)
	require.Equal(t, 3, result.JudgeCount)
	require.Equal(t, 1, result.RefutedCount)
	require.Equal(t, AdversarialAggregateSatisfied, result.Aggregate)
}

// Test 3: a judge returns non-JSON → that judge is recorded refuted + parse_failed.
func TestAdversarialParseFailureConservativeRefute(t *testing.T) {
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{
		{content: "I think this is fine, no JSON here"},
		{content: acceptedJSON("满足")},
		{content: acceptedJSON("满足")},
	}}
	lenses := resolveAdversarialLenses(3)
	result, err := runAdversarialReview(context.Background(), client, lenses, adversarialTestInput())
	require.NoError(t, err)
	require.Equal(t, AdversarialVerdictRefuted, result.Judgements[0].Verdict)
	require.Equal(t, adversarialReasonParseFailed, result.Judgements[0].Reason)
	// One parse-failed refute out of 3 is a minority → still satisfied.
	require.Equal(t, 1, result.RefutedCount)
	require.Equal(t, AdversarialAggregateSatisfied, result.Aggregate)
}

// Test 4: N=2, any single refute → unsatisfied (conservative, ceil(2/2)=1).
func TestAdversarialTwoJudgeAnyRefuteKills(t *testing.T) {
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{
		{content: refutedJSON("边界未覆盖")},
		{content: acceptedJSON("满足")},
	}}
	lenses := resolveAdversarialLenses(2)
	require.Len(t, lenses, 2)
	result, err := runAdversarialReview(context.Background(), client, lenses, adversarialTestInput())
	require.NoError(t, err)
	require.Equal(t, 2, result.JudgeCount)
	require.Equal(t, 1, result.RefutedCount)
	require.Equal(t, AdversarialAggregateUnsatisfied, result.Aggregate)
}

// Test 5: policy value is capped at the hard limit of 7; unset (0) → default 3.
func TestAdversarialJudgeCountCappedAndPolicyRead(t *testing.T) {
	require.Equal(t, 7, resolveJudgeCount(10), "policy=10 must cap at 7")
	require.Equal(t, 3, resolveJudgeCount(0), "unset policy must default to 3")
	require.Equal(t, 3, resolveJudgeCount(-2), "negative policy falls back to default 3")
	require.Equal(t, 5, resolveJudgeCount(5), "in-range value is honored")
	require.Equal(t, 1, resolveJudgeCount(1), "single judge is honored")

	// The resolved lens set matches the resolved count and stays capped.
	require.Len(t, resolveAdversarialLenses(resolveJudgeCount(10)), 7)
	require.Len(t, resolveAdversarialLenses(resolveJudgeCount(0)), 3)
}

// Test 6: every judge call carries a refute (证伪) instruction and the three
// default lenses use distinct perspectives.
func TestAdversarialEachJudgeGetsRefutePrompt(t *testing.T) {
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{
		{content: acceptedJSON("满足")},
	}}
	lenses := resolveAdversarialLenses(3)
	_, err := runAdversarialReview(context.Background(), client, lenses, adversarialTestInput())
	require.NoError(t, err)
	require.Len(t, client.systems, 3)

	for i, system := range client.systems {
		require.Truef(t, strings.Contains(system, "证伪"), "judge %d system prompt must instruct to refute: %q", i, system)
		require.Truef(t, strings.Contains(system, "refuted"), "judge %d system prompt must define the refuted verdict token", i)
	}
	// Perspectives differ across the three default lenses.
	require.NotEqual(t, client.systems[0], client.systems[1])
	require.NotEqual(t, client.systems[1], client.systems[2])
	require.NotEqual(t, client.systems[0], client.systems[2])
	require.Equal(t, "correctness", lenses[0].Key)
	require.Equal(t, "security", lenses[1].Key)
	require.Equal(t, "reproducibility", lenses[2].Key)
}

// Activity-level: budget exhaustion short-circuits to the human-escalation result
// without calling any judge.
func TestRunAdversarialReviewBudgetExhaustedEscalates(t *testing.T) {
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{
		{content: refutedJSON("should never be called")},
	}}
	a := WithJudgeClient(&Activities{}, client)
	input := adversarialTestInput()
	input.BudgetExhausted = true
	result, err := a.RunAdversarialReview(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, AdversarialAggregateEscalateHuman, result.Aggregate)
	require.Equal(t, 0, result.JudgeCount)
	require.Equal(t, 0, result.RefutedCount)
	require.Equal(t, input.CriterionID, result.CriterionID)
	require.Equal(t, 0, client.calls, "no judge should be called once budget is exhausted")
}

// Activity-level: a nil judge client is a clear, typed error (mirrors ErrRoutePlannerRequired).
func TestRunAdversarialReviewRequiresJudgeClient(t *testing.T) {
	a := &Activities{}
	_, err := a.RunAdversarialReview(context.Background(), adversarialTestInput())
	require.ErrorIs(t, err, ErrJudgeClientRequired)
}

// Activity-level: the happy path resolves the policy count and runs the engine.
func TestRunAdversarialReviewRunsEngine(t *testing.T) {
	client := &scriptedChatCompletionClient{responses: []scriptedResponse{
		{content: refutedJSON("边界未覆盖")},
		{content: refutedJSON("无复现证据")},
		{content: acceptedJSON("满足")},
	}}
	a := WithJudgeClient(&Activities{}, client)
	input := adversarialTestInput()
	input.JudgeCountPolicy = 3
	result, err := a.RunAdversarialReview(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 3, result.JudgeCount)
	require.Equal(t, 2, result.RefutedCount)
	require.Equal(t, AdversarialAggregateUnsatisfied, result.Aggregate)
	require.Equal(t, input.ReviewedTaskID, result.ReviewedTaskID)
}

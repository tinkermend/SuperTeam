package employee

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestEvaluateSessionResume(t *testing.T) {
	node := uuid.New()
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)

	t.Run("resumes fresh same-node session", func(t *testing.T) {
		decision := evaluateSessionResume(ProviderSessionResumeCandidate{
			SessionID:         "sess-1",
			RuntimeNodeID:     node,
			LastRuntimeSeenAt: now.Add(-time.Hour),
		}, node, now, DefaultSessionResumeMaxIdle)
		require.True(t, decision.Resumed())
		require.Equal(t, "sess-1", decision.SessionID)
		require.Empty(t, decision.SkipReason)
	})

	t.Run("no candidate is not a skip", func(t *testing.T) {
		decision := evaluateSessionResume(ProviderSessionResumeCandidate{}, node, now, DefaultSessionResumeMaxIdle)
		require.False(t, decision.Resumed())
		require.Empty(t, decision.SkipReason)
	})

	t.Run("node mismatch skips", func(t *testing.T) {
		decision := evaluateSessionResume(ProviderSessionResumeCandidate{
			SessionID:     "sess-other-node",
			RuntimeNodeID: uuid.New(),
		}, node, now, DefaultSessionResumeMaxIdle)
		require.False(t, decision.Resumed())
		require.Equal(t, SessionResumeSkipReasonNodeMismatch, decision.SkipReason)
	})

	t.Run("stale skips", func(t *testing.T) {
		decision := evaluateSessionResume(ProviderSessionResumeCandidate{
			SessionID:         "sess-stale",
			RuntimeNodeID:     node,
			LastRuntimeSeenAt: now.Add(-DefaultSessionResumeMaxIdle - time.Minute),
		}, node, now, DefaultSessionResumeMaxIdle)
		require.False(t, decision.Resumed())
		require.Equal(t, SessionResumeSkipReasonStale, decision.SkipReason)
	})

	t.Run("last_active_at fresher than runtime seen keeps session", func(t *testing.T) {
		decision := evaluateSessionResume(ProviderSessionResumeCandidate{
			SessionID:         "sess-active",
			RuntimeNodeID:     node,
			LastRuntimeSeenAt: now.Add(-DefaultSessionResumeMaxIdle - time.Hour),
			LastActiveAt:      now.Add(-time.Hour),
		}, node, now, DefaultSessionResumeMaxIdle)
		require.True(t, decision.Resumed())
	})

	t.Run("missing timestamps do not skip", func(t *testing.T) {
		decision := evaluateSessionResume(ProviderSessionResumeCandidate{
			SessionID:     "sess-legacy",
			RuntimeNodeID: node,
		}, node, now, DefaultSessionResumeMaxIdle)
		require.True(t, decision.Resumed())
	})

	t.Run("nil target node does not force mismatch", func(t *testing.T) {
		decision := evaluateSessionResume(ProviderSessionResumeCandidate{
			SessionID:     "sess-1",
			RuntimeNodeID: node,
		}, uuid.Nil, now, DefaultSessionResumeMaxIdle)
		require.True(t, decision.Resumed())
	})
}

func TestSessionResumeUserSummaryLocked(t *testing.T) {
	require.Equal(t, "已接上该员工上次会话继续执行", SessionResumeUserSummary(SessionResumeStatusResumed, ""))
	require.Equal(t,
		"原会话超过7天未活跃，已主动开新会话（未沿用旧会话）",
		SessionResumeUserSummary(SessionResumeStatusSkipped, SessionResumeSkipReasonStale),
	)
	require.Equal(t,
		"原会话在其他运行节点，已主动开新会话",
		SessionResumeUserSummary(SessionResumeStatusSkipped, SessionResumeSkipReasonNodeMismatch),
	)
	require.Empty(t, SessionResumeUserSummary(SessionResumeStatusNone, ""))
}

func TestSessionResumeTraceLabelLocked(t *testing.T) {
	require.Equal(t, "已接上上次会话", SessionResumeTraceLabel(SessionResumeStatusResumed, ""))
	require.Equal(t, "已开新会话 · 原会话过期", SessionResumeTraceLabel(SessionResumeStatusSkipped, SessionResumeSkipReasonStale))
	require.Equal(t, "已开新会话 · 原会话不在本节点", SessionResumeTraceLabel(SessionResumeStatusSkipped, SessionResumeSkipReasonNodeMismatch))
	require.Equal(t, "新会话", SessionResumeTraceLabel(SessionResumeStatusNone, ""))
}

func TestSessionResumeOutcomeFromDecision(t *testing.T) {
	resumed := SessionResumeOutcomeFromDecision(SessionResumeDecision{SessionID: "s1"}, "s1", true)
	require.Equal(t, SessionResumeStatusResumed, resumed.Status)
	require.True(t, resumed.ShouldEmitContinuity())
	require.Equal(t, "已接上该员工上次会话继续执行", resumed.Summary)

	skipped := SessionResumeOutcomeFromDecision(SessionResumeDecision{SkipReason: SessionResumeSkipReasonStale}, "old", true)
	require.Equal(t, SessionResumeStatusSkipped, skipped.Status)
	require.Equal(t, "old", skipped.SessionID)
	require.True(t, skipped.ShouldEmitContinuity())

	none := SessionResumeOutcomeFromDecision(SessionResumeDecision{}, "", true)
	require.Equal(t, SessionResumeStatusNone, none.Status)
	require.False(t, none.ShouldEmitContinuity())

	notAttempted := SessionResumeOutcomeFromDecision(SessionResumeDecision{}, "", false)
	require.Empty(t, notAttempted.Status)
	require.False(t, notAttempted.ShouldEmitContinuity())
}

func TestApplyAndRoundTripSessionResumeMetadata(t *testing.T) {
	outcome := SessionResumeOutcomeFromDecision(SessionResumeDecision{SkipReason: SessionResumeSkipReasonNodeMismatch}, "sess-x", true)
	meta := map[string]any{}
	ApplySessionResumeOutcomeMetadata(meta, outcome)
	AttachSessionResumeOutcomeToMap(meta, outcome)

	require.Equal(t, SessionResumeStatusSkipped, meta["session_resume_status"])
	require.Equal(t, SessionResumeSkipReasonNodeMismatch, meta["session_resume_skipped"])
	require.Equal(t, "sess-x", meta["session_resume_skipped_session_id"])

	got := SessionResumeOutcomeFromMetadata(meta)
	require.Equal(t, outcome.Status, got.Status)
	require.Equal(t, outcome.SkipReason, got.SkipReason)
	require.Equal(t, outcome.SessionID, got.SessionID)
	require.Equal(t, outcome.Summary, got.Summary)
	require.Equal(t, outcome.Label, got.Label)
}

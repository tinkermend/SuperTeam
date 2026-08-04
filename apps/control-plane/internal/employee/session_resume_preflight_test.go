package employee

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// resume 预检的全部判据（spec 2026-08-01 §6.1）。这条路径的价值就在于
// "宁可开新会话，也不要带着必失败的 session id 把整单拖挂"。
func TestEvaluateSessionResume(t *testing.T) {
	node := uuid.New()
	otherNode := uuid.New()
	now := time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)

	t.Run("新鲜且同节点：续", func(t *testing.T) {
		decision := evaluateSessionResume(ProviderSessionResumeCandidate{
			SessionID:         "sess-1",
			RuntimeNodeID:     node,
			LastRuntimeSeenAt: now.Add(-time.Hour),
		}, node, now, DefaultSessionResumeMaxIdle)

		require.True(t, decision.Resumed())
		require.Equal(t, "sess-1", decision.SessionID)
		require.Empty(t, decision.SkipReason)
	})

	t.Run("无候选：开新会话且不留痕（不是降级）", func(t *testing.T) {
		decision := evaluateSessionResume(ProviderSessionResumeCandidate{}, node, now, DefaultSessionResumeMaxIdle)

		require.False(t, decision.Resumed())
		require.Empty(t, decision.SkipReason, "本来就没会话，不该记成一次降级")
	})

	t.Run("跨节点：放弃并留痕", func(t *testing.T) {
		decision := evaluateSessionResume(ProviderSessionResumeCandidate{
			SessionID:         "sess-1",
			RuntimeNodeID:     otherNode,
			LastRuntimeSeenAt: now,
		}, node, now, DefaultSessionResumeMaxIdle)

		require.False(t, decision.Resumed(), "会话文件在原机器上，换节点后必失败")
		require.Equal(t, SessionResumeSkipReasonNodeMismatch, decision.SkipReason)
	})

	t.Run("过期：放弃并留痕", func(t *testing.T) {
		decision := evaluateSessionResume(ProviderSessionResumeCandidate{
			SessionID:         "sess-1",
			RuntimeNodeID:     node,
			LastRuntimeSeenAt: now.Add(-DefaultSessionResumeMaxIdle - time.Minute),
		}, node, now, DefaultSessionResumeMaxIdle)

		require.False(t, decision.Resumed())
		require.Equal(t, SessionResumeSkipReasonStale, decision.SkipReason)
	})

	t.Run("两个时间取更近的一个", func(t *testing.T) {
		// runtime 很久没见到，但控制平面侧刚活跃过 → 仍认为可能还在，试着续。
		decision := evaluateSessionResume(ProviderSessionResumeCandidate{
			SessionID:         "sess-1",
			RuntimeNodeID:     node,
			LastRuntimeSeenAt: now.Add(-DefaultSessionResumeMaxIdle - time.Hour),
			LastActiveAt:      now.Add(-time.Minute),
		}, node, now, DefaultSessionResumeMaxIdle)

		require.True(t, decision.Resumed())
	})

	t.Run("两个时间都缺：不据此拒绝", func(t *testing.T) {
		// 历史数据可能没有这两列；缺一列就把所有老会话判死太粗暴。
		decision := evaluateSessionResume(ProviderSessionResumeCandidate{
			SessionID:     "sess-1",
			RuntimeNodeID: node,
		}, node, now, DefaultSessionResumeMaxIdle)

		require.True(t, decision.Resumed())
	})

	t.Run("候选未绑节点：不按节点拒绝", func(t *testing.T) {
		decision := evaluateSessionResume(ProviderSessionResumeCandidate{
			SessionID:    "sess-1",
			LastActiveAt: now,
		}, node, now, DefaultSessionResumeMaxIdle)

		require.True(t, decision.Resumed())
	})
}

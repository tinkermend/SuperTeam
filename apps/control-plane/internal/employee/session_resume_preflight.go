package employee

import (
	"time"

	"github.com/google/uuid"
)

// resume 预检（spec 2026-08-01-demand-continuation-design §6.1）。
//
// 为什么必须有：`--resume <失效 id>` 会让 provider 进程直接失败，于是**整个
// 接续单失败**。而接续的典型场景恰恰是"隔了几天回来接着做"——正是会话最可能
// 已经过期、或 runtime 节点已经换掉的时刻。人主动来接续，换来一次莫名其妙的
// 失败，是最坏的失败模式。
//
// 判据放在控制平面：runtime 侧做 resume 兜底属于改 provider 管道（基线 §4.8
// 越界）。这里主动放弃 resume、正常开新会话，并**留痕**——不留痕的降级等于
// 静默丢上下文，事后无法区分"该续没续"与"本就不该续"。

const (
	// DefaultSessionResumeMaxIdle 是会话可续的最长静置时间。provider 侧的
	// 会话文件不是永久资产（磁盘清理、家目录 LRU 裁剪都会动它），过旧的 id
	// 拿去 resume 基本是必失败。
	DefaultSessionResumeMaxIdle = 7 * 24 * time.Hour

	SessionResumeSkipReasonStale        = "session_stale"
	SessionResumeSkipReasonNodeMismatch = "session_node_mismatch"
)

// SessionResumeDecision 是"这次派发要不要带上旧会话"的结论。
type SessionResumeDecision struct {
	SessionID string
	// SkipReason 非空表示找到了会话但主动放弃续用，值是原因码（留痕用）。
	SkipReason string
}

// Resumed 报告本次派发是否真的带上了旧会话。
func (d SessionResumeDecision) Resumed() bool { return d.SessionID != "" }

// evaluateSessionResume 判定一条候选会话能否续用。
//
// now 由调用方注入，便于测试；targetNodeID 是本次派发实际落到的 runtime 节点。
func evaluateSessionResume(
	candidate ProviderSessionResumeCandidate,
	targetNodeID uuid.UUID,
	now time.Time,
	maxIdle time.Duration,
) SessionResumeDecision {
	if candidate.SessionID == "" {
		// 没有候选：正常开新会话，不算降级，不留痕。
		return SessionResumeDecision{}
	}
	// 会话文件在原机器上，换节点后那个 id 在本机不存在。
	if candidate.RuntimeNodeID != uuid.Nil && targetNodeID != uuid.Nil &&
		candidate.RuntimeNodeID != targetNodeID {
		return SessionResumeDecision{SkipReason: SessionResumeSkipReasonNodeMismatch}
	}
	if maxIdle > 0 {
		// 取两个时间里更近的一个：last_runtime_seen_at 是 runtime 视角的活性，
		// last_active_at 是控制平面视角；任一新鲜即认为会话可能还在。
		lastSeen := candidate.LastRuntimeSeenAt
		if candidate.LastActiveAt.After(lastSeen) {
			lastSeen = candidate.LastActiveAt
		}
		// 两个时间都缺（历史数据）时不据此拒绝：宁可试一次 resume，
		// 也不因为缺一列就把所有老会话判死。
		if !lastSeen.IsZero() && now.Sub(lastSeen) > maxIdle {
			return SessionResumeDecision{SkipReason: SessionResumeSkipReasonStale}
		}
	}
	return SessionResumeDecision{SessionID: candidate.SessionID}
}

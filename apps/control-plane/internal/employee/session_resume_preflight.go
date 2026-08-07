package employee

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// resume 预检（spec 2026-08-01-demand-continuation-design §6.1）与
// 可观测性（spec 2026-08-07-session-resume-observability-design）。
//
// 为什么必须有：`--resume <失效 id>` 会让 provider 进程直接失败，于是**整个
// 接续单失败**。而接续的典型场景恰恰是"隔了几天回来接着做"——正是会话最可能
// 已经过期、或 runtime 节点已经换掉的时刻。人主动来接续，换来一次莫名其妙的
// 失败，是最坏的失败模式。
//
// 判据放在控制平面：runtime 侧做 resume 兜底属于改 provider 管道（基线 §4.8
// 越界）。这里主动放弃 resume、正常开新会话，并**留痕**——不留痕的降级等于
// 静默丢上下文，事后无法区分"该续没续"与"本就不该续"。
//
// 2026-08-07：留痕从 run metadata 提升到卷宗时间线；本文件是中文文案与
// outcome 结构的唯一源。

const (
	// DefaultSessionResumeMaxIdle 是会话可续的最长静置时间。provider 侧的
	// 会话文件不是永久资产（磁盘清理、家目录 LRU 裁剪都会动它），过旧的 id
	// 拿去 resume 基本是必失败。
	DefaultSessionResumeMaxIdle = 7 * 24 * time.Hour

	SessionResumeSkipReasonStale        = "session_stale"
	SessionResumeSkipReasonNodeMismatch = "session_node_mismatch"

	SessionResumeStatusResumed = "resumed"
	SessionResumeStatusSkipped = "skipped"
	SessionResumeStatusNone    = "none"
)

// SessionResumeDecision 是"这次派发要不要带上旧会话"的结论。
type SessionResumeDecision struct {
	SessionID string
	// SkipReason 非空表示找到了会话但主动放弃续用，值是原因码（留痕用）。
	SkipReason string
}

// Resumed 报告本次派发是否真的带上了旧会话。
func (d SessionResumeDecision) Resumed() bool { return d.SessionID != "" }

// SessionResumeOutcome 是派发期会话接续的完整结论（机器字段 + 中文文案）。
// 跨包通过 StartProjectTaskRunResult 传递；projectcoordination 只消费，不重算。
type SessionResumeOutcome struct {
	Status     string // resumed | skipped | none
	SkipReason string
	SessionID  string // resumed=续上的 id；skipped=被放弃的 id；none=空
	Summary    string // 卷宗时间线中文一句；none 为空
	Label      string // 执行轨迹短中文；none 为空
}

// ShouldEmitContinuity 报告是否应写入卷宗 continuity 事件。
func (o SessionResumeOutcome) ShouldEmitContinuity() bool {
	return o.Status == SessionResumeStatusResumed || o.Status == SessionResumeStatusSkipped
}

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

// SessionResumeOutcomeFromDecision 把预检决策收成 outcome（含中文）。
// attempted 为 false 表示政策禁止 resume，不写 status=none 语义上的「尝试过查找」。
func SessionResumeOutcomeFromDecision(decision SessionResumeDecision, candidateSessionID string, attempted bool) SessionResumeOutcome {
	switch {
	case decision.Resumed():
		return SessionResumeOutcome{
			Status:    SessionResumeStatusResumed,
			SessionID: decision.SessionID,
			Summary:   SessionResumeUserSummary(SessionResumeStatusResumed, ""),
			Label:     SessionResumeTraceLabel(SessionResumeStatusResumed, ""),
		}
	case decision.SkipReason != "":
		sessionID := strings.TrimSpace(candidateSessionID)
		if sessionID == "" {
			sessionID = strings.TrimSpace(decision.SessionID)
		}
		return SessionResumeOutcome{
			Status:     SessionResumeStatusSkipped,
			SkipReason: decision.SkipReason,
			SessionID:  sessionID,
			Summary:    SessionResumeUserSummary(SessionResumeStatusSkipped, decision.SkipReason),
			Label:      SessionResumeTraceLabel(SessionResumeStatusSkipped, decision.SkipReason),
		}
	case attempted:
		return SessionResumeOutcome{
			Status: SessionResumeStatusNone,
			// none 默认不进卷宗；Summary/Label 留空。
		}
	default:
		return SessionResumeOutcome{}
	}
}

// SessionResumeUserSummary 是卷宗时间线中文一句的唯一源（spec §3.2 / §5.6）。
func SessionResumeUserSummary(status, skipReason string) string {
	switch status {
	case SessionResumeStatusResumed:
		return "已接上该员工上次会话继续执行"
	case SessionResumeStatusSkipped:
		switch skipReason {
		case SessionResumeSkipReasonStale:
			return fmt.Sprintf("原会话超过%s未活跃，已主动开新会话（未沿用旧会话）", formatSessionResumeMaxIdle(DefaultSessionResumeMaxIdle))
		case SessionResumeSkipReasonNodeMismatch:
			return "原会话在其他运行节点，已主动开新会话"
		default:
			if strings.TrimSpace(skipReason) == "" {
				return "已主动开新会话（未沿用旧会话）"
			}
			return "已主动开新会话（未沿用旧会话）"
		}
	default:
		return ""
	}
}

// SessionResumeTraceLabel 是执行轨迹短中文（spec §6.2）。
func SessionResumeTraceLabel(status, skipReason string) string {
	switch status {
	case SessionResumeStatusResumed:
		return "已接上上次会话"
	case SessionResumeStatusSkipped:
		switch skipReason {
		case SessionResumeSkipReasonStale:
			return "已开新会话 · 原会话过期"
		case SessionResumeSkipReasonNodeMismatch:
			return "已开新会话 · 原会话不在本节点"
		default:
			return "已开新会话"
		}
	case SessionResumeStatusNone:
		return "新会话"
	default:
		return ""
	}
}

func formatSessionResumeMaxIdle(d time.Duration) string {
	if d <= 0 {
		return "限定时间"
	}
	days := int(d / (24 * time.Hour))
	if days > 0 && d%(24*time.Hour) == 0 {
		return fmt.Sprintf("%d天", days)
	}
	hours := int(d / time.Hour)
	if hours > 0 && d%time.Hour == 0 {
		return fmt.Sprintf("%d小时", hours)
	}
	return d.String()
}

// ApplySessionResumeOutcomeMetadata 把 outcome 写入 run metadata（对称留痕）。
// attempted 为 true 时才会写 status=none。
func ApplySessionResumeOutcomeMetadata(metadata map[string]any, outcome SessionResumeOutcome) {
	if metadata == nil {
		return
	}
	switch outcome.Status {
	case SessionResumeStatusResumed:
		metadata["session_resume_status"] = SessionResumeStatusResumed
		if outcome.SessionID != "" {
			metadata["provider_session_id"] = outcome.SessionID
			metadata["session_resume_session_id"] = outcome.SessionID
		}
	case SessionResumeStatusSkipped:
		metadata["session_resume_status"] = SessionResumeStatusSkipped
		if outcome.SkipReason != "" {
			metadata["session_resume_skipped"] = outcome.SkipReason
		}
		if outcome.SessionID != "" {
			metadata["session_resume_skipped_session_id"] = outcome.SessionID
		}
	case SessionResumeStatusNone:
		metadata["session_resume_status"] = SessionResumeStatusNone
	}
}

// SessionResumeOutcomeFromMetadata 从 run/packet metadata 反构 outcome（activity 重试自愈用）。
func SessionResumeOutcomeFromMetadata(metadata map[string]any) SessionResumeOutcome {
	if metadata == nil {
		return SessionResumeOutcome{}
	}
	status, _ := metadata["session_resume_status"].(string)
	status = strings.TrimSpace(status)
	skipReason, _ := metadata["session_resume_skipped"].(string)
	skipReason = strings.TrimSpace(skipReason)
	sessionID := ""
	switch status {
	case SessionResumeStatusResumed:
		if v, ok := metadata["provider_session_id"].(string); ok {
			sessionID = strings.TrimSpace(v)
		}
		if sessionID == "" {
			if v, ok := metadata["session_resume_session_id"].(string); ok {
				sessionID = strings.TrimSpace(v)
			}
		}
	case SessionResumeStatusSkipped:
		if v, ok := metadata["session_resume_skipped_session_id"].(string); ok {
			sessionID = strings.TrimSpace(v)
		}
	}
	// 优先用写入时固化的文案，避免文案演进后历史 packet 被重算。
	summary, _ := metadata["session_resume_summary"].(string)
	label, _ := metadata["session_resume_label"].(string)
	outcome := SessionResumeOutcome{
		Status:     status,
		SkipReason: skipReason,
		SessionID:  sessionID,
		Summary:    strings.TrimSpace(summary),
		Label:      strings.TrimSpace(label),
	}
	if outcome.Summary == "" {
		outcome.Summary = SessionResumeUserSummary(outcome.Status, outcome.SkipReason)
	}
	if outcome.Label == "" {
		outcome.Label = SessionResumeTraceLabel(outcome.Status, outcome.SkipReason)
	}
	return outcome
}

// AttachSessionResumeOutcomeToMap 把 outcome 固化进 execution_context_packet 等 map。
func AttachSessionResumeOutcomeToMap(target map[string]any, outcome SessionResumeOutcome) {
	if target == nil || outcome.Status == "" {
		return
	}
	target["session_resume_status"] = outcome.Status
	if outcome.SkipReason != "" {
		target["session_resume_skip_reason"] = outcome.SkipReason
		// 与 run metadata 字段名对齐，便于 SessionResumeOutcomeFromMetadata 反构。
		target["session_resume_skipped"] = outcome.SkipReason
	}
	if outcome.SessionID != "" {
		target["session_resume_session_id"] = outcome.SessionID
		if outcome.Status == SessionResumeStatusResumed {
			target["provider_session_id"] = outcome.SessionID
		}
		if outcome.Status == SessionResumeStatusSkipped {
			target["session_resume_skipped_session_id"] = outcome.SessionID
		}
	}
	if outcome.Summary != "" {
		target["session_resume_summary"] = outcome.Summary
	}
	if outcome.Label != "" {
		target["session_resume_label"] = outcome.Label
	}
}

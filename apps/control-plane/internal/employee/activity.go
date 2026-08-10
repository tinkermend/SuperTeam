package employee

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ActivityEventPresentation 是 task_events.event_type 到中文标签与状态语义的唯一映射来源，
// 供跨员工动态流端点与 overview recent_events 共用；未收录类型按启发式兜底后原文透出。
func ActivityEventPresentation(eventType string) (label string, status string) {
	switch eventType {
	case "run_dispatched":
		return "命令已下发", "running"
	case "session_started":
		return "会话已启动", "running"
	case "text_delta":
		return "输出内容更新", "running"
	case "tool_started":
		return "开始调用工具", "running"
	case "tool_completed":
		return "工具调用完成", "running"
	case "turn_completed":
		return "回合完成", "running"
	case "turn_error":
		return "回合失败", "failed"
	case "native_unmapped":
		return "未映射的原生事件", "running"
	case "run_completed":
		return "运行完成", "completed"
	case "run_failed":
		return "运行失败", "failed"
	case "run_cancelled":
		return "运行已取消", "cancelled"
	case "run_reaped_stale":
		return "运行超时回收", "failed"
	case "stop_requested":
		return "已请求停止", "cancelled"
	case "task.started":
		return "任务开始", "running"
	case "task.progress":
		return "任务进展", "running"
	}
	lower := strings.ToLower(eventType)
	switch {
	case strings.Contains(lower, "fail"):
		return "执行失败", "failed"
	case strings.Contains(lower, "cancel"):
		return "已取消", "cancelled"
	case strings.Contains(lower, "complete"):
		return "等待结果回写", "completed"
	case strings.Contains(lower, "provider"):
		return "Provider 输出中", "running"
	}
	return eventType, "running"
}

type GetDigitalEmployeeActivityRequest struct {
	TenantID       uuid.UUID
	SinceCreatedAt *time.Time
	SinceID        *uuid.UUID
	Limit          int32
}

type DigitalEmployeeActivityItem struct {
	EventID             uuid.UUID
	EventType           string
	Label               string
	Status              string
	OccurredAt          *time.Time
	RunID               uuid.UUID
	TaskID              uuid.UUID
	TaskTitle           string
	DigitalEmployeeID   uuid.UUID
	DigitalEmployeeName string
	TeamID              *uuid.UUID
	ProjectID           *uuid.UUID
	ProjectName         string
}

type DigitalEmployeeActivity struct {
	Items []DigitalEmployeeActivityItem
	// NextSince：下次增量拉取用的游标（最新一条事件的位置）；无新事件时回显请求游标。
	NextSince string
}

// 游标格式：<created_at RFC3339Nano>|<event_id>，对客户端不透明。
func encodeActivityCursor(occurredAt time.Time, eventID uuid.UUID) string {
	return fmt.Sprintf("%s|%s", occurredAt.UTC().Format(time.RFC3339Nano), eventID)
}

func decodeActivityCursor(cursor string) (*time.Time, *uuid.UUID, error) {
	trimmed := strings.TrimSpace(cursor)
	if trimmed == "" {
		return nil, nil, nil
	}
	parts := strings.SplitN(trimmed, "|", 2)
	if len(parts) != 2 {
		return nil, nil, fmt.Errorf("invalid cursor")
	}
	occurredAt, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid cursor timestamp")
	}
	eventID, err := uuid.Parse(parts[1])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid cursor id")
	}
	return &occurredAt, &eventID, nil
}

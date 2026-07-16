package employee

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestActivityEventPresentationMapsKnownAndFallbackTypes(t *testing.T) {
	cases := []struct {
		eventType string
		label     string
		status    string
	}{
		{"run_dispatched", "命令已下发", "running"},
		{"text_delta", "输出内容更新", "running"},
		{"tool_started", "开始调用工具", "running"},
		{"run_completed", "运行完成", "completed"},
		{"run_failed", "运行失败", "failed"},
		{"run_cancelled", "运行已取消", "cancelled"},
		{"run_reaped_stale", "运行超时回收", "failed"},
		// 未收录类型走启发式兜底。
		{"provider_stream_chunk", "Provider 输出中", "running"},
		{"custom_step_failed", "执行失败", "failed"},
		// 完全未知类型原文透出，不再裸奔到前端以外的语义。
		{"totally_unknown", "totally_unknown", "running"},
	}
	for _, tc := range cases {
		label, status := ActivityEventPresentation(tc.eventType)
		require.Equal(t, tc.label, label, tc.eventType)
		require.Equal(t, tc.status, status, tc.eventType)
	}
}

func TestActivityCursorRoundTrip(t *testing.T) {
	occurredAt := time.Date(2026, 7, 16, 8, 30, 15, 123456789, time.UTC)
	eventID := uuid.New()
	cursor := encodeActivityCursor(occurredAt, eventID)

	parsedAt, parsedID, err := decodeActivityCursor(cursor)
	require.NoError(t, err)
	require.NotNil(t, parsedAt)
	require.NotNil(t, parsedID)
	require.True(t, occurredAt.Equal(*parsedAt))
	require.Equal(t, eventID, *parsedID)

	emptyAt, emptyID, err := decodeActivityCursor("  ")
	require.NoError(t, err)
	require.Nil(t, emptyAt)
	require.Nil(t, emptyID)

	_, _, err = decodeActivityCursor("not-a-cursor")
	require.Error(t, err)

	_, _, err = decodeActivityCursor("2026-07-16T08:30:15Z|not-a-uuid")
	require.Error(t, err)
}

func TestServiceGetActivityValidatesAndBuildsNextSince(t *testing.T) {
	repo := newMemoryRepository()
	occurredAt := time.Date(2026, 7, 16, 8, 30, 0, 0, time.UTC)
	newestID := uuid.New()
	repo.digitalEmployeeActivity = []DigitalEmployeeActivityItem{
		{EventID: newestID, EventType: "run_dispatched", Label: "命令已下发", Status: "running", OccurredAt: &occurredAt},
		{EventID: uuid.New(), EventType: "text_delta", Label: "输出内容更新", Status: "running", OccurredAt: &occurredAt},
	}
	service, err := NewService(repo)
	require.NoError(t, err)

	_, err = service.GetActivity(context.Background(), GetDigitalEmployeeActivityRequest{})
	require.ErrorIs(t, err, ErrInvalidInput)

	tenantID := uuid.New()
	activity, err := service.GetActivity(context.Background(), GetDigitalEmployeeActivityRequest{TenantID: tenantID, Limit: 500})
	require.NoError(t, err)
	require.Len(t, activity.Items, 2)
	require.Equal(t, encodeActivityCursor(occurredAt, newestID), activity.NextSince)
	// limit 超上限被钳制到 100。
	require.Equal(t, int32(100), repo.lastActivityRequest.Limit)

	repo.digitalEmployeeActivity = nil
	empty, err := service.GetActivity(context.Background(), GetDigitalEmployeeActivityRequest{TenantID: tenantID})
	require.NoError(t, err)
	require.Empty(t, empty.Items)
	require.Empty(t, empty.NextSince)
	require.Equal(t, int32(20), repo.lastActivityRequest.Limit)
}

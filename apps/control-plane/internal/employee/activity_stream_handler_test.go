package employee

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestStreamActivityPushesEventsInChronologicalOrderAndAdvancesCursor(t *testing.T) {
	tenantID := uuid.New()
	userID := uuid.New()
	newer := time.Date(2026, 7, 16, 9, 0, 5, 0, time.UTC)
	older := time.Date(2026, 7, 16, 9, 0, 1, 0, time.UTC)
	service := &fakeTemplateHandlerService{
		activityBatches: [][]DigitalEmployeeActivityItem{
			{
				// 服务返回时间倒序（新在前）。
				{EventID: uuid.New(), EventType: "run_completed", Label: "运行完成", Status: "completed", OccurredAt: &newer, DigitalEmployeeName: "乙员工"},
				{EventID: uuid.New(), EventType: "run_dispatched", Label: "命令已下发", Status: "running", OccurredAt: &older, DigitalEmployeeName: "甲员工"},
			},
		},
	}
	handler := newTemplateTestHandler(service, true)
	handler.activityStreamInterval = 5 * time.Millisecond

	req := templateTestRequest(http.MethodGet, "/api/v1/digital-employees/activity/stream", "", tenantID, userID, nil)
	ctx, cancel := context.WithTimeout(req.Context(), 120*time.Millisecond)
	defer cancel()
	recorder := httptest.NewRecorder()
	handler.StreamActivity(recorder, req.WithContext(ctx))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	body := recorder.Body.String()
	require.Equal(t, 2, strings.Count(body, "event: activity"), body)
	// 推送按时间正序：旧事件（甲员工）先于新事件（乙员工）。
	require.Less(t, strings.Index(body, "甲员工"), strings.Index(body, "乙员工"), body)
	require.Contains(t, body, `"label":"命令已下发"`)
	// 首轮空游标从"现在"起播；推送后游标推进到最新事件位置，后续增量请求携带该游标。
	require.NotNil(t, service.lastActivityRequest.SinceCreatedAt)
	require.True(t, service.lastActivityRequest.SinceCreatedAt.Equal(newer))
}

func TestStreamActivityRejectsInvalidCursor(t *testing.T) {
	service := &fakeTemplateHandlerService{}
	handler := newTemplateTestHandler(service, true)
	handler.activityStreamInterval = 5 * time.Millisecond

	req := templateTestRequest(http.MethodGet, "/api/v1/digital-employees/activity/stream?since=broken", "", uuid.New(), uuid.New(), nil)
	recorder := httptest.NewRecorder()
	handler.StreamActivity(recorder, req)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestStreamActivitySendsKeepaliveWhenIdle(t *testing.T) {
	service := &fakeTemplateHandlerService{}
	handler := newTemplateTestHandler(service, true)
	handler.activityStreamInterval = 2 * time.Millisecond

	req := templateTestRequest(http.MethodGet, "/api/v1/digital-employees/activity/stream", "", uuid.New(), uuid.New(), nil)
	ctx, cancel := context.WithTimeout(req.Context(), 100*time.Millisecond)
	defer cancel()
	recorder := httptest.NewRecorder()
	handler.StreamActivity(recorder, req.WithContext(ctx))

	require.Contains(t, recorder.Body.String(), ": keepalive")
}

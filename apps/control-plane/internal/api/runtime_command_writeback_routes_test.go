package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/handlers"
	"github.com/superteam/control-plane/internal/employee"
	"github.com/superteam/control-plane/internal/runtime"
)

func TestRuntimeCommandWritebackRoutesUseRuntimeSessionAuth(t *testing.T) {
	runtimeService := &routeRuntimeService{}
	writebackService := &routeRuntimeCommandWritebackService{}
	server := NewServerWithRuntimeSessionAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(runtimeService, &routeTaskService{}, &routePoller{}),
		&routeRuntimeAuthService{},
		runtimeService,
	)
	server.SetRuntimeCommandWritebackHandler(handlers.NewRuntimeCommandWritebackHandler(writebackService))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/commands/cmd-1/events", strings.NewReader(`{"event_type":"text_delta","sequence_number":7,"payload":{"text":"hello"}}`))
	req.Header.Set("Authorization", "Bearer session-token")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusAccepted {
		t.Fatalf("expected runtime session authenticated writeback to return 202, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(writebackService.calls) != 1 {
		t.Fatalf("expected one writeback service call, got %#v", writebackService.calls)
	}
	call := writebackService.calls[0]
	if call.method != "event" || call.tenantID != runtime.DefaultTenantID || call.commandID != "cmd-1" {
		t.Fatalf("expected tenant/command from runtime session route, got %#v", call)
	}
}

func TestRuntimeCommandWritebackRoutesRejectMissingRuntimeSessionAuth(t *testing.T) {
	runtimeService := &routeRuntimeService{}
	writebackService := &routeRuntimeCommandWritebackService{}
	server := NewServerWithRuntimeSessionAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(runtimeService, &routeTaskService{}, &routePoller{}),
		&routeRuntimeAuthService{},
		runtimeService,
	)
	server.SetRuntimeCommandWritebackHandler(handlers.NewRuntimeCommandWritebackHandler(writebackService))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/commands/cmd-1/events", strings.NewReader(`{"event_type":"text_delta","sequence_number":7}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing runtime session auth to return 401, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(writebackService.calls) != 0 {
		t.Fatalf("expected unauthorized route not to call writeback service, got %#v", writebackService.calls)
	}
}

type routeRuntimeCommandWritebackService struct {
	calls []routeRuntimeCommandWritebackCall
}

type routeRuntimeCommandWritebackCall struct {
	method    string
	tenantID  uuid.UUID
	commandID string
}

func (s *routeRuntimeCommandWritebackService) RecordEvent(ctx context.Context, tenantID uuid.UUID, commandID string, event employee.RuntimeCommandEventWriteback) error {
	s.calls = append(s.calls, routeRuntimeCommandWritebackCall{method: "event", tenantID: tenantID, commandID: commandID})
	return nil
}

func (s *routeRuntimeCommandWritebackService) Complete(ctx context.Context, tenantID uuid.UUID, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error {
	s.calls = append(s.calls, routeRuntimeCommandWritebackCall{method: "complete", tenantID: tenantID, commandID: commandID})
	return nil
}

func (s *routeRuntimeCommandWritebackService) Fail(ctx context.Context, tenantID uuid.UUID, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error {
	s.calls = append(s.calls, routeRuntimeCommandWritebackCall{method: "fail", tenantID: tenantID, commandID: commandID})
	return nil
}

func (s *routeRuntimeCommandWritebackService) Cancel(ctx context.Context, tenantID uuid.UUID, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error {
	s.calls = append(s.calls, routeRuntimeCommandWritebackCall{method: "cancel", tenantID: tenantID, commandID: commandID})
	return nil
}

func (s *routeRuntimeCommandWritebackService) TimedOut(ctx context.Context, tenantID uuid.UUID, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error {
	s.calls = append(s.calls, routeRuntimeCommandWritebackCall{method: "timed_out", tenantID: tenantID, commandID: commandID})
	return nil
}

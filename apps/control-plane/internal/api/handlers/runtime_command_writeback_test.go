package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/employee"
)

func TestRuntimeCommandWritebackEndpointsDecodeBodyAndReturnAccepted(t *testing.T) {
	tenantID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	tests := []struct {
		name       string
		body       string
		call       func(*RuntimeCommandWritebackHandler, http.ResponseWriter, *http.Request)
		wantMethod string
		wantStatus employee.DigitalEmployeeRunStatus
	}{
		{
			name:       "events",
			body:       `{"event_type":"text_delta","sequence_number":7,"payload":{"text":"hello"},"provider_session_external_id":"provider-session-1"}`,
			call:       (*RuntimeCommandWritebackHandler).RecordEvent,
			wantMethod: "event",
		},
		{
			name:       "complete",
			body:       `{"status":"completed","summary":"done","result":{"summary":"done"}}`,
			call:       (*RuntimeCommandWritebackHandler).Complete,
			wantMethod: "complete",
			wantStatus: employee.DigitalEmployeeRunStatusCompleted,
		},
		{
			name:       "fail",
			body:       `{"status":"failed","error_message":"provider failed","error_code":"exit_1"}`,
			call:       (*RuntimeCommandWritebackHandler).Fail,
			wantMethod: "fail",
			wantStatus: employee.DigitalEmployeeRunStatusFailed,
		},
		{
			name:       "cancelled",
			body:       `{"status":"cancelled"}`,
			call:       (*RuntimeCommandWritebackHandler).Cancel,
			wantMethod: "cancel",
			wantStatus: employee.DigitalEmployeeRunStatusCancelled,
		},
		{
			name:       "timed-out",
			body:       `{"status":"timed_out","timed_out":true}`,
			call:       (*RuntimeCommandWritebackHandler).TimedOut,
			wantMethod: "timed_out",
			wantStatus: employee.DigitalEmployeeRunStatusTimedOut,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := &fakeRuntimeCommandWritebackService{}
			handler := NewRuntimeCommandWritebackHandler(service)
			req := runtimeCommandWritebackRequest(http.MethodPost, "/api/v1/runtime/commands/cmd-1/"+tt.name, "cmd-1", tenantID, tt.body)
			resp := httptest.NewRecorder()

			tt.call(handler, resp, req)

			if resp.Code != http.StatusAccepted {
				t.Fatalf("expected 202, got %d: %s", resp.Code, resp.Body.String())
			}
			if len(service.calls) != 1 {
				t.Fatalf("expected one service call, got %#v", service.calls)
			}
			call := service.calls[0]
			if call.method != tt.wantMethod || call.tenantID != tenantID || call.commandID != "cmd-1" {
				t.Fatalf("unexpected service call: %#v", call)
			}
			if tt.wantMethod == "event" {
				if call.event.EventType != "text_delta" || call.event.SequenceNumber != 7 {
					t.Fatalf("unexpected event writeback: %#v", call.event)
				}
				if call.event.Payload["text"] != "hello" {
					t.Fatalf("expected decoded payload text, got %#v", call.event.Payload)
				}
			} else if call.terminal.Status != tt.wantStatus {
				t.Fatalf("expected terminal status %s, got %#v", tt.wantStatus, call.terminal)
			}
		})
	}
}

func TestRuntimeCommandWritebackConflictMapsTo409(t *testing.T) {
	service := &fakeRuntimeCommandWritebackService{err: employee.ErrConflict}
	handler := NewRuntimeCommandWritebackHandler(service)
	req := runtimeCommandWritebackRequest(http.MethodPost, "/api/v1/runtime/commands/cmd-1/complete", "cmd-1", runtimeCommandWritebackTenantID, `{"status":"completed"}`)
	resp := httptest.NewRecorder()

	handler.Complete(resp, req)

	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestRuntimeCommandWritebackInvalidJSONMapsTo400(t *testing.T) {
	service := &fakeRuntimeCommandWritebackService{}
	handler := NewRuntimeCommandWritebackHandler(service)
	req := runtimeCommandWritebackRequest(http.MethodPost, "/api/v1/runtime/commands/cmd-1/events", "cmd-1", runtimeCommandWritebackTenantID, `{"event_type":`)
	resp := httptest.NewRecorder()

	handler.RecordEvent(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(service.calls) != 0 {
		t.Fatalf("expected invalid json not to call service, got %#v", service.calls)
	}
}

func TestRuntimeCommandWritebackMissingTenantMapsToUnauthorized(t *testing.T) {
	service := &fakeRuntimeCommandWritebackService{}
	handler := NewRuntimeCommandWritebackHandler(service)
	req := runtimeCommandWritebackRequest(http.MethodPost, "/api/v1/runtime/commands/cmd-1/events", "cmd-1", uuid.Nil, `{"event_type":"text_delta","sequence_number":1}`)
	resp := httptest.NewRecorder()

	handler.RecordEvent(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(service.calls) != 0 {
		t.Fatalf("expected missing tenant not to call service, got %#v", service.calls)
	}
}

func runtimeCommandWritebackRequest(method, path, commandID string, tenantID uuid.UUID, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("commandId", commandID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, routeContext)
	if tenantID != uuid.Nil {
		ctx = context.WithValue(ctx, middleware.TenantIDKey, tenantID)
	}
	return req.WithContext(ctx)
}

var runtimeCommandWritebackTenantID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

type fakeRuntimeCommandWritebackService struct {
	err   error
	calls []runtimeCommandWritebackCall
}

type runtimeCommandWritebackCall struct {
	method    string
	tenantID  uuid.UUID
	commandID string
	event     employee.RuntimeCommandEventWriteback
	terminal  employee.RuntimeCommandTerminalWriteback
}

func (f *fakeRuntimeCommandWritebackService) RecordEvent(ctx context.Context, tenantID uuid.UUID, commandID string, event employee.RuntimeCommandEventWriteback) error {
	f.calls = append(f.calls, runtimeCommandWritebackCall{method: "event", tenantID: tenantID, commandID: commandID, event: event})
	return f.err
}

func (f *fakeRuntimeCommandWritebackService) Complete(ctx context.Context, tenantID uuid.UUID, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error {
	f.calls = append(f.calls, runtimeCommandWritebackCall{method: "complete", tenantID: tenantID, commandID: commandID, terminal: terminal})
	return f.err
}

func (f *fakeRuntimeCommandWritebackService) Fail(ctx context.Context, tenantID uuid.UUID, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error {
	f.calls = append(f.calls, runtimeCommandWritebackCall{method: "fail", tenantID: tenantID, commandID: commandID, terminal: terminal})
	return f.err
}

func (f *fakeRuntimeCommandWritebackService) Cancel(ctx context.Context, tenantID uuid.UUID, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error {
	f.calls = append(f.calls, runtimeCommandWritebackCall{method: "cancel", tenantID: tenantID, commandID: commandID, terminal: terminal})
	return f.err
}

func (f *fakeRuntimeCommandWritebackService) TimedOut(ctx context.Context, tenantID uuid.UUID, commandID string, terminal employee.RuntimeCommandTerminalWriteback) error {
	f.calls = append(f.calls, runtimeCommandWritebackCall{method: "timed_out", tenantID: tenantID, commandID: commandID, terminal: terminal})
	return f.err
}

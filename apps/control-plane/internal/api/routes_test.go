package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/handlers"
	"github.com/superteam/control-plane/internal/auth"
	"github.com/superteam/control-plane/internal/authz"
	"github.com/superteam/control-plane/internal/authzcenter"
	"github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/task"
	"nhooyr.io/websocket"
)

const routeTaskID = "11111111-1111-1111-1111-111111111111"

func TestRuntimeRoutesAreRegistered(t *testing.T) {
	server := NewServer(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
	)

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/v1/runtime/register", body: `{"node_id":"node-1","name":"node 1","supported_providers":["codex"],"max_slots":1}`},
		{method: http.MethodPost, path: "/api/v1/runtime/heartbeat", body: `{"current_load":0}`},
		{method: http.MethodPost, path: "/api/v1/runtime/tasks/claim"},
		{method: http.MethodPost, path: "/api/v1/runtime/tasks/" + routeTaskID + "/events", body: `{"events":[]}`},
		{method: http.MethodPost, path: "/api/v1/runtime/tasks/" + routeTaskID + "/complete", body: `{"result":{}}`},
		{method: http.MethodPost, path: "/api/v1/runtime/tasks/" + routeTaskID + "/fail", body: `{"error":"failed"}`},
		{method: http.MethodPost, path: "/api/v1/runtime/tasks/" + routeTaskID + "/lease"},
		{method: http.MethodGet, path: "/api/v1/runtime/nodes"},
		{method: http.MethodGet, path: "/api/v1/runtime/nodes/node-1"},
		{method: http.MethodPost, path: "/api/v1/runtime/enrollments/hello", body: `{"node_id":"node-1","bootstrap_key":"bootstrap-secret","name":"node 1","supported_providers":["codex"],"max_slots":1}`},
		{method: http.MethodGet, path: "/api/v1/runtime/enrollments"},
		{method: http.MethodGet, path: "/api/v1/runtime/overview"},
		{method: http.MethodGet, path: "/api/v1/runtime/events"},
		{method: http.MethodGet, path: "/api/v1/runtime/nodes/node-1/capabilities"},
		{method: http.MethodPost, path: "/api/v1/runtime/enrollments/11111111-1111-1111-1111-111111111111/approve"},
		{method: http.MethodPost, path: "/api/v1/runtime/enrollments/11111111-1111-1111-1111-111111111111/reject", body: `{"reason":"not allowed"}`},
		{method: http.MethodPost, path: "/api/v1/runtime/enrollments/11111111-1111-1111-1111-111111111111/revoke", body: `{"reason":"rotated"}`},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			server.ServeHTTP(rr, req)

			if rr.Code == http.StatusNotFound {
				t.Fatalf("expected runtime route to be registered, got 404")
			}
		})
	}
}

func TestRuntimeEnrollmentHelloUsesCurrentContractPathWithoutRuntimeSessionAuth(t *testing.T) {
	service := &routeRuntimeService{}
	server := NewServer(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{}),
		&routeRuntimeAuthService{},
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/enrollments/hello", strings.NewReader(`{"node_id":"node-hello","bootstrap_key":"bootstrap-secret","name":"node hello","version":"1.2.3","supported_providers":["codex"],"max_slots":2,"metadata":{"region":"local"},"capabilities":[{"capability_type":"provider","capability_key":"codex","provider_type":"codex","available":true}]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected enrollment hello to be public and accepted, got %d: %s", rr.Code, rr.Body.String())
	}
	if service.enrollHelloReq.NodeID != "node-hello" || service.enrollHelloReq.BootstrapKey != "bootstrap-secret" {
		t.Fatalf("expected enrollment hello request to reach runtime service, got %#v", service.enrollHelloReq)
	}
	var body struct {
		Enrollment struct {
			NodeID string `json:"node_id"`
			Status string `json:"status"`
		} `json:"enrollment"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode enrollment hello response: %v", err)
	}
	if body.Enrollment.NodeID != "node-hello" || body.Enrollment.Status != "pending" {
		t.Fatalf("unexpected enrollment hello response: %#v", body.Enrollment)
	}
}

func TestRuntimeEnrollmentManagementRoutesRequireConsoleUserAuth(t *testing.T) {
	service := &routeRuntimeService{}
	server := NewServer(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{}),
	)

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/v1/runtime/enrollments"},
		{method: http.MethodPost, path: "/api/v1/runtime/enrollments/" + routeTaskID + "/approve"},
		{method: http.MethodPost, path: "/api/v1/runtime/enrollments/" + routeTaskID + "/reject", body: `{"reason":"no"}`},
		{method: http.MethodPost, path: "/api/v1/runtime/enrollments/" + routeTaskID + "/revoke", body: `{"reason":"rotated"}`},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			resp := httptest.NewRecorder()

			server.ServeHTTP(resp, req)

			if resp.Code != http.StatusUnauthorized {
				t.Fatalf("expected unauthenticated enrollment management route to return 401, got %d: %s", resp.Code, resp.Body.String())
			}
		})
	}
	if service.listEnrollmentsCalled || service.approveEnrollmentCalled || service.rejectEnrollmentCalled || service.revokeEnrollmentCalled {
		t.Fatalf("expected unauthenticated management routes not to call runtime service: %#v", service)
	}
}

func TestRuntimeOverviewRoutesRequireConsoleAuth(t *testing.T) {
	service := &routeRuntimeService{}
	server := NewServer(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{}),
	)

	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/api/v1/runtime/overview"},
		{method: http.MethodGet, path: "/api/v1/runtime/events"},
		{method: http.MethodGet, path: "/api/v1/runtime/nodes/node-1/capabilities"},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			resp := httptest.NewRecorder()

			server.ServeHTTP(resp, req)

			if resp.Code != http.StatusUnauthorized {
				t.Fatalf("expected unauthenticated runtime overview route to return 401, got %d: %s", resp.Code, resp.Body.String())
			}
		})
	}
	if service.getOverviewCalled || service.listEventsCalled || service.listCapabilitiesCalled {
		t.Fatalf("expected unauthenticated runtime overview routes not to call runtime service: %#v", service)
	}
}

func TestRuntimeEnrollmentManagementRoutesUseConsoleUserAuth(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeRuntimeService{}
	authorizer := &routeAuthorizer{allowed: true}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		authorizer,
	)
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"admin"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp := httptest.NewRecorder()
	server.ServeHTTP(loginResp, loginReq)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("expected login to succeed, got %d: %s", loginResp.Code, loginResp.Body.String())
	}
	cookie := loginResp.Result().Cookies()[0]

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/enrollments", nil)
	listReq.AddCookie(cookie)
	listResp := httptest.NewRecorder()
	server.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected authenticated list enrollments to succeed, got %d: %s", listResp.Code, listResp.Body.String())
	}
	if !service.listEnrollmentsCalled {
		t.Fatalf("expected list enrollments to call runtime service")
	}

	overviewReq := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/overview", nil)
	overviewReq.AddCookie(cookie)
	overviewResp := httptest.NewRecorder()
	server.ServeHTTP(overviewResp, overviewReq)
	if overviewResp.Code != http.StatusOK {
		t.Fatalf("expected authenticated runtime overview to succeed, got %d: %s", overviewResp.Code, overviewResp.Body.String())
	}
	if !service.getOverviewCalled {
		t.Fatalf("expected runtime overview to call runtime service")
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/events?limit=25&offset=5&event_type=capability_degraded&severity=warning&node_id=node-1&provider_type=codex", nil)
	eventsReq.AddCookie(cookie)
	eventsResp := httptest.NewRecorder()
	server.ServeHTTP(eventsResp, eventsReq)
	if eventsResp.Code != http.StatusOK {
		t.Fatalf("expected authenticated runtime events to succeed, got %d: %s", eventsResp.Code, eventsResp.Body.String())
	}
	if !service.listEventsCalled {
		t.Fatalf("expected runtime events to call runtime service")
	}
	if service.eventsFilter.Limit != 25 || service.eventsFilter.Offset != 5 {
		t.Fatalf("expected runtime events pagination filter, got %#v", service.eventsFilter)
	}
	if service.eventsFilter.EventType == nil || *service.eventsFilter.EventType != runtime.RuntimeEventCapabilityDegraded {
		t.Fatalf("expected runtime events event_type filter, got %#v", service.eventsFilter.EventType)
	}
	if service.eventsFilter.Severity == nil || *service.eventsFilter.Severity != runtime.RuntimeEventSeverity("warning") {
		t.Fatalf("expected runtime events severity filter, got %#v", service.eventsFilter.Severity)
	}
	if service.eventsFilter.NodeID == nil || *service.eventsFilter.NodeID != "node-1" {
		t.Fatalf("expected runtime events node_id filter, got %#v", service.eventsFilter.NodeID)
	}
	if service.eventsFilter.ProviderType == nil || *service.eventsFilter.ProviderType != "codex" {
		t.Fatalf("expected runtime events provider_type filter, got %#v", service.eventsFilter.ProviderType)
	}

	capabilitiesReq := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/nodes/node-1/capabilities", nil)
	capabilitiesReq.AddCookie(cookie)
	capabilitiesResp := httptest.NewRecorder()
	server.ServeHTTP(capabilitiesResp, capabilitiesReq)
	if capabilitiesResp.Code != http.StatusOK {
		t.Fatalf("expected authenticated runtime node capabilities to succeed, got %d: %s", capabilitiesResp.Code, capabilitiesResp.Body.String())
	}
	if !service.listCapabilitiesCalled || service.capabilitiesNodeID != "node-1" {
		t.Fatalf("expected runtime node capabilities service call for node-1, got called=%v node=%q", service.listCapabilitiesCalled, service.capabilitiesNodeID)
	}

	approveReq := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/enrollments/"+routeTaskID+"/approve", nil)
	approveReq.AddCookie(cookie)
	approveResp := httptest.NewRecorder()
	server.ServeHTTP(approveResp, approveReq)
	if approveResp.Code != http.StatusOK {
		t.Fatalf("expected authenticated approve enrollment to succeed, got %d: %s", approveResp.Code, approveResp.Body.String())
	}
	if !service.approveEnrollmentCalled || service.approvedEnrollmentID.String() != routeTaskID {
		t.Fatalf("expected approve enrollment service call, got called=%v id=%s", service.approveEnrollmentCalled, service.approvedEnrollmentID)
	}

	rejectReq := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/enrollments/"+routeTaskID+"/reject", strings.NewReader(`{"reason":"bad bootstrap"}`))
	rejectReq.Header.Set("Content-Type", "application/json")
	rejectReq.AddCookie(cookie)
	rejectResp := httptest.NewRecorder()
	server.ServeHTTP(rejectResp, rejectReq)
	if rejectResp.Code != http.StatusOK {
		t.Fatalf("expected authenticated reject enrollment to succeed, got %d: %s", rejectResp.Code, rejectResp.Body.String())
	}
	if !service.rejectEnrollmentCalled || service.rejectedReason != "bad bootstrap" {
		t.Fatalf("expected reject enrollment service call with reason, got called=%v reason=%q", service.rejectEnrollmentCalled, service.rejectedReason)
	}

	revokeReq := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/enrollments/"+routeTaskID+"/revoke", strings.NewReader(`{"reason":"rotated"}`))
	revokeReq.Header.Set("Content-Type", "application/json")
	revokeReq.AddCookie(cookie)
	revokeResp := httptest.NewRecorder()
	server.ServeHTTP(revokeResp, revokeReq)
	if revokeResp.Code != http.StatusOK {
		t.Fatalf("expected authenticated revoke enrollment to succeed, got %d: %s", revokeResp.Code, revokeResp.Body.String())
	}
	if !service.revokeEnrollmentCalled || service.revokedReason != "rotated" {
		t.Fatalf("expected revoke enrollment service call with reason, got called=%v reason=%q", service.revokeEnrollmentCalled, service.revokedReason)
	}
}

func TestRuntimeOverviewAndEventsResponsesUseRuntimeConsoleShape(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	zeroNodeEvent := runtime.RuntimeEvent{
		ID:            uuid.MustParse("77777777-7777-7777-7777-777777777777"),
		TenantID:      runtime.DefaultTenantID,
		EventType:     runtime.RuntimeEventCommandEvent,
		Severity:      runtime.RuntimeEventSeverityInfo,
		Source:        runtime.RuntimeEventSourceRuntimeCommand,
		Title:         "command event",
		ProviderType:  "codex",
		CorrelationID: "cmd-1",
		CreatedAt:     time.Date(2026, 6, 5, 7, 0, 0, 0, time.UTC),
	}
	nodeEvent := runtime.RuntimeEvent{
		ID:            uuid.MustParse("88888888-8888-8888-8888-888888888888"),
		TenantID:      runtime.DefaultTenantID,
		RuntimeNodeID: uuid.MustParse("44444444-4444-4444-4444-444444444444"),
		NodeID:        "node-1",
		EventType:     runtime.RuntimeEventCapabilityReported,
		Severity:      runtime.RuntimeEventSeveritySuccess,
		Source:        runtime.RuntimeEventSourceCapability,
		Title:         "capability reported",
		CreatedAt:     time.Date(2026, 6, 5, 7, 1, 0, 0, time.UTC),
	}
	service := &routeRuntimeService{
		runtimeOverview: &runtime.RuntimeOverview{
			Summary: runtime.RuntimeOverviewSummary{
				OnlineNodes:            1,
				TotalNodes:             2,
				PendingEnrollments:     3,
				ActiveProviderSessions: 4,
				BlockedEvents:          5,
			},
			ProviderCapabilities: []runtime.RuntimeProviderCapabilitySummary{{
				ProviderType:   "codex",
				NodeCount:      2,
				AvailableCount: 1,
				HealthyCount:   1,
				LastSeenAt:     time.Date(2026, 6, 5, 7, 2, 0, 0, time.UTC),
			}},
			RecentEvents: []runtime.RuntimeEvent{zeroNodeEvent, nodeEvent},
		},
		runtimeEvents: []runtime.RuntimeEvent{zeroNodeEvent, nodeEvent},
	}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
	)
	cookie := routeLogin(t, server, "admin", "admin")

	overviewReq := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/overview", nil)
	overviewReq.AddCookie(cookie)
	overviewResp := httptest.NewRecorder()
	server.ServeHTTP(overviewResp, overviewReq)
	if overviewResp.Code != http.StatusOK {
		t.Fatalf("expected runtime overview to succeed, got %d: %s", overviewResp.Code, overviewResp.Body.String())
	}
	var overview map[string]any
	if err := json.NewDecoder(overviewResp.Body).Decode(&overview); err != nil {
		t.Fatalf("decode overview response: %v", err)
	}
	summary, ok := overview["summary"].(map[string]any)
	if !ok || summary["online_nodes"] != float64(1) || summary["active_provider_sessions"] != float64(4) {
		t.Fatalf("expected snake_case overview summary, got %#v", overview["summary"])
	}
	if _, ok := overview["provider_capabilities"]; !ok {
		t.Fatalf("expected provider_capabilities key in overview response: %#v", overview)
	}
	overviewEvents, ok := overview["recent_events"].([]any)
	if !ok || len(overviewEvents) != 2 {
		t.Fatalf("expected two recent_events in overview response, got %#v", overview["recent_events"])
	}
	assertRuntimeEventNodeIdentityOmitted(t, overviewEvents[0])
	assertRuntimeEventNodeIdentityPresent(t, overviewEvents[1], "44444444-4444-4444-4444-444444444444", "node-1")

	eventsReq := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/events?limit=2&offset=1", nil)
	eventsReq.AddCookie(cookie)
	eventsResp := httptest.NewRecorder()
	server.ServeHTTP(eventsResp, eventsReq)
	if eventsResp.Code != http.StatusOK {
		t.Fatalf("expected runtime events to succeed, got %d: %s", eventsResp.Code, eventsResp.Body.String())
	}
	var eventsBody map[string]any
	if err := json.NewDecoder(eventsResp.Body).Decode(&eventsBody); err != nil {
		t.Fatalf("decode events response: %v", err)
	}
	if eventsBody["limit"] != float64(2) || eventsBody["offset"] != float64(1) {
		t.Fatalf("expected events pagination keys, got %#v", eventsBody)
	}
	items, ok := eventsBody["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected events items array, got %#v", eventsBody["items"])
	}
	assertRuntimeEventNodeIdentityOmitted(t, items[0])
	assertRuntimeEventNodeIdentityPresent(t, items[1], "44444444-4444-4444-4444-444444444444", "node-1")
	firstEvent, ok := items[0].(map[string]any)
	if !ok || firstEvent["event_type"] != string(runtime.RuntimeEventCommandEvent) || firstEvent["provider_type"] != "codex" {
		t.Fatalf("expected snake_case runtime event fields, got %#v", items[0])
	}
}

func TestRuntimeEventsRejectInvalidFilters(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "event type", path: "/api/v1/runtime/events?event_type=capability.changed"},
		{name: "severity", path: "/api/v1/runtime/events?severity=urgent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authService, err := auth.NewService(newRouteAuthRepo())
			if err != nil {
				t.Fatalf("new auth service: %v", err)
			}
			if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
				t.Fatalf("create user: %v", err)
			}
			service := &routeRuntimeService{}
			server := NewServerWithAuthz(
				handlers.NewTaskHandler(&routeTaskService{}),
				handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{}),
				authService,
				nil,
				&routeAuthorizer{allowed: true},
			)
			cookie := routeLogin(t, server, "admin", "admin")
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.AddCookie(cookie)
			resp := httptest.NewRecorder()

			server.ServeHTTP(resp, req)

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected invalid runtime event filter to return 400, got %d: %s", resp.Code, resp.Body.String())
			}
			if service.listEventsCalled {
				t.Fatalf("expected invalid runtime event filter not to call runtime service")
			}
		})
	}
}

func assertRuntimeEventNodeIdentityOmitted(t *testing.T, raw any) {
	t.Helper()
	event, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("expected runtime event object, got %#v", raw)
	}
	if _, ok := event["runtime_node_id"]; ok {
		t.Fatalf("expected zero runtime_node_id to be omitted, got %#v", event)
	}
	if _, ok := event["node_id"]; ok {
		t.Fatalf("expected empty node_id to be omitted, got %#v", event)
	}
}

func assertRuntimeEventNodeIdentityPresent(t *testing.T, raw any, runtimeNodeID string, nodeID string) {
	t.Helper()
	event, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("expected runtime event object, got %#v", raw)
	}
	if event["runtime_node_id"] != runtimeNodeID || event["node_id"] != nodeID {
		t.Fatalf("expected runtime event node identity %s/%s, got %#v", runtimeNodeID, nodeID, event)
	}
}

func TestRuntimeEnrollmentManagementRoutesRequireAuthorization(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeRuntimeService{}
	authorizer := &routeAuthorizer{allowed: false}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		authorizer,
	)
	cookie := routeLogin(t, server, "admin", "admin")

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/api/v1/runtime/enrollments"},
		{method: http.MethodGet, path: "/api/v1/runtime/overview"},
		{method: http.MethodGet, path: "/api/v1/runtime/events"},
		{method: http.MethodGet, path: "/api/v1/runtime/nodes/node-1/capabilities"},
		{method: http.MethodPost, path: "/api/v1/runtime/enrollments/" + routeTaskID + "/approve"},
		{method: http.MethodPost, path: "/api/v1/runtime/enrollments/" + routeTaskID + "/reject", body: `{"reason":"denied"}`},
		{method: http.MethodPost, path: "/api/v1/runtime/enrollments/" + routeTaskID + "/revoke", body: `{"reason":"denied"}`},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req.AddCookie(cookie)
			resp := httptest.NewRecorder()

			server.ServeHTTP(resp, req)

			if resp.Code != http.StatusForbidden {
				t.Fatalf("expected denied enrollment management route to return 403, got %d: %s", resp.Code, resp.Body.String())
			}
		})
	}
	if service.listEnrollmentsCalled || service.getOverviewCalled || service.listEventsCalled || service.listCapabilitiesCalled || service.approveEnrollmentCalled || service.rejectEnrollmentCalled || service.revokeEnrollmentCalled {
		t.Fatalf("expected denied management routes not to call runtime service: %#v", service)
	}
	if len(authorizer.checks) != len(tests) {
		t.Fatalf("expected one authz check per management request, got %#v", authorizer.checks)
	}
	for _, check := range authorizer.checks {
		if check.Actor.Type != authz.ActorUser {
			t.Fatalf("expected user actor, got %#v", check)
		}
		if check.Action != authz.ActionRuntimeScopeManage {
			t.Fatalf("expected runtime scope manage action, got %#v", check)
		}
		if check.Resource.Type != authz.ResourceTenant || check.Resource.ID != auth.DefaultTenantID {
			t.Fatalf("expected tenant resource %s, got %#v", auth.DefaultTenantID, check)
		}
	}
}

func TestRuntimeEnrollmentManagementRoutesPassTenantAndActorToRuntimeService(t *testing.T) {
	authRepo := newRouteAuthRepo()
	authService, err := auth.NewService(authRepo)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	user, err := authService.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	service := &routeRuntimeService{}
	authorizer := &routeAuthorizer{allowed: true}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		authorizer,
	)
	cookie := routeLogin(t, server, "admin", "admin")

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/enrollments", nil)
	listReq.AddCookie(cookie)
	listResp := httptest.NewRecorder()
	server.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list enrollment to succeed, got %d: %s", listResp.Code, listResp.Body.String())
	}
	if service.listEnrollmentsTenantID.String() != auth.DefaultTenantID {
		t.Fatalf("expected list tenant %s, got %s", auth.DefaultTenantID, service.listEnrollmentsTenantID)
	}

	approveReq := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/enrollments/"+routeTaskID+"/approve", nil)
	approveReq.AddCookie(cookie)
	approveResp := httptest.NewRecorder()
	server.ServeHTTP(approveResp, approveReq)
	if approveResp.Code != http.StatusOK {
		t.Fatalf("expected approve enrollment to succeed, got %d: %s", approveResp.Code, approveResp.Body.String())
	}
	if service.approveTenantID.String() != auth.DefaultTenantID || service.approvedBy != user.ID {
		t.Fatalf("expected approve tenant/user %s/%s, got %s/%s", auth.DefaultTenantID, user.ID, service.approveTenantID, service.approvedBy)
	}
	if len(authorizer.checks) < 2 {
		t.Fatalf("expected authz checks, got %#v", authorizer.checks)
	}
	check := authorizer.checks[len(authorizer.checks)-1]
	if check.Actor.ID != user.ID.String() || check.AuditReason != "runtime enrollment approve" {
		t.Fatalf("expected approve authz check with actor and audit reason, got %#v", check)
	}
}

func TestLegacyRuntimeClaimRouteIsNotRegistered(t *testing.T) {
	server := NewServer(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/claim", nil)
	rr := httptest.NewRecorder()

	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected legacy runtime claim route to be removed, got %d", rr.Code)
	}
}

func TestAuthRoutesAreRegistered(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	server := NewServerWithAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
	)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"admin"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp := httptest.NewRecorder()

	server.ServeHTTP(loginResp, loginReq)

	if loginResp.Code != http.StatusOK {
		t.Fatalf("expected login to succeed, got %d: %s", loginResp.Code, loginResp.Body.String())
	}
	cookies := loginResp.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != auth.SessionCookieName {
		t.Fatalf("expected session cookie, got %#v", cookies)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.AddCookie(cookies[0])
	meResp := httptest.NewRecorder()

	server.ServeHTTP(meResp, meReq)

	if meResp.Code != http.StatusOK {
		t.Fatalf("expected current user to succeed, got %d: %s", meResp.Code, meResp.Body.String())
	}

	logsReq := httptest.NewRequest(http.MethodGet, "/api/auth/login-logs?limit=10&offset=0", nil)
	logsReq.AddCookie(cookies[0])
	logsResp := httptest.NewRecorder()

	server.ServeHTTP(logsResp, logsReq)

	if logsResp.Code != http.StatusOK {
		t.Fatalf("expected login logs to succeed, got %d: %s", logsResp.Code, logsResp.Body.String())
	}
	var logsBody struct {
		Items []struct {
			EventType string `json:"event_type"`
			Username  string `json:"username"`
			Result    string `json:"result"`
		} `json:"items"`
	}
	if err := json.NewDecoder(logsResp.Body).Decode(&logsBody); err != nil {
		t.Fatalf("decode login logs response: %v", err)
	}
	if len(logsBody.Items) != 1 {
		t.Fatalf("expected one login log, got %#v", logsBody.Items)
	}
	if logsBody.Items[0].EventType != auth.LoginEventSucceeded {
		t.Fatalf("expected login succeeded log, got %#v", logsBody.Items[0])
	}
}

func TestCurrentUserRequiresConsoleAuthorization(t *testing.T) {
	authRepo := newRouteAuthRepo()
	authService, err := auth.NewService(authRepo)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	user, err := authService.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	authorizer := &routeAuthorizer{allowed: false}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		authorizer,
	)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"admin"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp := httptest.NewRecorder()
	server.ServeHTTP(loginResp, loginReq)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("expected login to succeed, got %d: %s", loginResp.Code, loginResp.Body.String())
	}

	meReq := httptest.NewRequest(http.MethodGet, "/api/auth/me", nil)
	meReq.AddCookie(loginResp.Result().Cookies()[0])
	meResp := httptest.NewRecorder()
	server.ServeHTTP(meResp, meReq)

	if meResp.Code != http.StatusForbidden {
		t.Fatalf("expected current user to be forbidden, got %d: %s", meResp.Code, meResp.Body.String())
	}
	if len(authorizer.checks) != 1 {
		t.Fatalf("expected one authorization check, got %#v", authorizer.checks)
	}
	check := authorizer.checks[0]
	if check.Action != authz.ActionConsoleAccess {
		t.Fatalf("expected console access action, got %q", check.Action)
	}
	if check.Resource.Type != authz.ResourceConsole {
		t.Fatalf("expected console resource type, got %q", check.Resource.Type)
	}
	if check.Resource.ID != "web" {
		t.Fatalf("expected web console resource ID, got %q", check.Resource.ID)
	}
	if check.Actor.Type != authz.ActorUser {
		t.Fatalf("expected user actor type, got %q", check.Actor.Type)
	}
	if check.Actor.ID != user.ID.String() {
		t.Fatalf("expected actor ID %q, got %q", user.ID.String(), check.Actor.ID)
	}
	if check.TenantID.String() != auth.DefaultTenantID {
		t.Fatalf("expected default tenant ID %q, got %q", auth.DefaultTenantID, check.TenantID.String())
	}
	if check.TeamID != nil {
		t.Fatalf("expected nil team ID, got %v", check.TeamID)
	}
	if check.AuditReason != "current user console access" {
		t.Fatalf("expected current user audit reason, got %q", check.AuditReason)
	}
}

func TestServerWithAuthzGatesRuntimeClaim(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	tenantID := uuid.MustParse(auth.DefaultTenantID)
	taskID := uuid.MustParse(routeTaskID)
	taskService := &routeTaskService{
		tasks: []*task.Task{{
			ID:           taskID,
			TenantID:     tenantID,
			ProviderType: "codex",
		}},
	}
	authorizer := &routeAuthorizer{allowed: false}
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(taskService),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, taskService, &routePoller{}),
		authService,
		&routeRuntimeAuthService{},
		authorizer,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/tasks/claim?timeout=1", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("X-Node-ID", "node-1")
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected denied runtime claim to return 204, got %d: %s", resp.Code, resp.Body.String())
	}
	if taskService.assignedTaskID != uuid.Nil {
		t.Fatalf("expected denied runtime claim not to assign task, got %s", taskService.assignedTaskID)
	}
	if len(authorizer.checks) != 1 {
		t.Fatalf("expected one runtime authz check, got %#v", authorizer.checks)
	}
	check := authorizer.checks[0]
	if check.Actor.Type != authz.ActorRuntimeNode || check.Actor.ID != "node-1" {
		t.Fatalf("expected runtime node actor, got %#v", check.Actor)
	}
	if check.Action != authz.ActionTaskClaim {
		t.Fatalf("expected task claim action, got %q", check.Action)
	}
	if check.Resource.Type != authz.ResourceTask || check.Resource.ID != taskID.String() {
		t.Fatalf("expected task resource %s, got %#v", taskID, check.Resource)
	}
	if check.TenantID != tenantID {
		t.Fatalf("expected tenant %s, got %s", tenantID, check.TenantID)
	}
}

func TestAuthUserManagementRoutesAreRegistered(t *testing.T) {
	authRepo := newRouteAuthRepo()
	authService, err := auth.NewService(authRepo)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	server := NewServerWithAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
	)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"admin","password":"admin"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp := httptest.NewRecorder()
	server.ServeHTTP(loginResp, loginReq)
	if loginResp.Code != http.StatusOK {
		t.Fatalf("expected login to succeed, got %d: %s", loginResp.Code, loginResp.Body.String())
	}
	cookie := loginResp.Result().Cookies()[0]

	createReq := httptest.NewRequest(http.MethodPost, "/api/auth/users", strings.NewReader(`{"username":"operator","password":"secret","avatar":{"provider":"dicebear","style":"adventurer","seed":"operator-avatar"}}`))
	createReq.Header.Set("Content-Type", "application/json")
	createReq.AddCookie(cookie)
	createResp := httptest.NewRecorder()
	server.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected create user to succeed, got %d: %s", createResp.Code, createResp.Body.String())
	}
	var createBody struct {
		User struct {
			ID     string `json:"id"`
			Avatar struct {
				Provider string `json:"provider"`
				Style    string `json:"style"`
				Seed     string `json:"seed"`
			} `json:"avatar"`
		} `json:"user"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&createBody); err != nil {
		t.Fatalf("decode created user response: %v", err)
	}
	operatorID, err := uuid.Parse(createBody.User.ID)
	if err != nil {
		t.Fatalf("expected created user ID to be UUID, got %q: %v", createBody.User.ID, err)
	}
	if createBody.User.Avatar.Provider != "dicebear" || createBody.User.Avatar.Style != "adventurer" || createBody.User.Avatar.Seed != "operator-avatar" {
		t.Fatalf("expected created user avatar to round-trip, got %#v", createBody.User.Avatar)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/auth/users?q=operator&status=active&limit=10&offset=0", nil)
	listReq.AddCookie(cookie)
	listResp := httptest.NewRecorder()
	server.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected list users to succeed, got %d: %s", listResp.Code, listResp.Body.String())
	}
	var listBody struct {
		Items []struct {
			Username string `json:"username"`
			Status   string `json:"status"`
			Avatar   struct {
				Provider string `json:"provider"`
				Style    string `json:"style"`
				Seed     string `json:"seed"`
			} `json:"avatar"`
		} `json:"items"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode list users response: %v", err)
	}
	if authRepo.lastListUsersFilter.Q != "operator" || authRepo.lastListUsersFilter.Status != auth.UserStatusActive {
		t.Fatalf("expected user list q/status operator/active, got %#v", authRepo.lastListUsersFilter)
	}
	if len(listBody.Items) != 1 || listBody.Items[0].Username != "operator" {
		t.Fatalf("expected only operator user, got %#v", listBody.Items)
	}
	if listBody.Items[0].Avatar.Seed != "operator-avatar" {
		t.Fatalf("expected listed user avatar seed to be preserved, got %#v", listBody.Items[0].Avatar)
	}

	statusReq := httptest.NewRequest(http.MethodPatch, "/api/auth/users/"+operatorID.String()+"/status", strings.NewReader(`{"status":"disabled"}`))
	statusReq.Header.Set("Content-Type", "application/json")
	statusReq.AddCookie(cookie)
	statusResp := httptest.NewRecorder()
	server.ServeHTTP(statusResp, statusReq)
	if statusResp.Code != http.StatusOK {
		t.Fatalf("expected status update to succeed, got %d: %s", statusResp.Code, statusResp.Body.String())
	}

	resetReq := httptest.NewRequest(http.MethodPost, "/api/auth/users/"+operatorID.String()+"/reset-password", strings.NewReader(`{"password":"new-secret"}`))
	resetReq.Header.Set("Content-Type", "application/json")
	resetReq.AddCookie(cookie)
	resetResp := httptest.NewRecorder()
	server.ServeHTTP(resetResp, resetReq)
	if resetResp.Code != http.StatusOK {
		t.Fatalf("expected password reset to succeed, got %d: %s", resetResp.Code, resetResp.Body.String())
	}

	if len(authRepo.operationLogs) != 3 {
		t.Fatalf("expected three operation logs, got %#v", authRepo.operationLogs)
	}
	if authRepo.operationLogs[0].Action != auth.OperationActionUserCreate {
		t.Fatalf("expected first operation to create user, got %#v", authRepo.operationLogs[0])
	}
	if authRepo.operationLogs[1].Action != auth.OperationActionUserDisable {
		t.Fatalf("expected second operation to disable user, got %#v", authRepo.operationLogs[1])
	}
	if authRepo.operationLogs[2].Action != auth.OperationActionUserResetPassword {
		t.Fatalf("expected third operation to reset password, got %#v", authRepo.operationLogs[2])
	}
}

func TestAuthUserManagementRejectsUnauthenticatedRequests(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	server := NewServerWithAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/users", nil)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected users route to reject unauthenticated request, got %d", resp.Code)
	}
}

func TestLoginLogsRejectUnauthenticatedRequests(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	server := NewServerWithAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/auth/login-logs", nil)
	resp := httptest.NewRecorder()

	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected login logs to reject unauthenticated request, got %d", resp.Code)
	}
}

func TestAuthzCenterOverviewRejectsUnauthenticatedRequests(t *testing.T) {
	authService, err := auth.NewService(newRouteAuthRepo())
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	repo := &routeAuthzCenterRepo{}
	service := authzcenter.NewService(repo, &routeAuthorizer{allowed: true})
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
		authzcenter.NewHandler(service, authService),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/authz/overview", nil)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected overview to reject unauthenticated request, got %d", resp.Code)
	}
}

func TestAuthzCenterOverviewAllowsAuthenticatedAdmin(t *testing.T) {
	authRepo := newRouteAuthRepo()
	authService, err := auth.NewService(authRepo)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "admin", "admin"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	repo := &routeAuthzCenterRepo{}
	service := authzcenter.NewService(repo, &routeAuthorizer{allowed: true})
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		&routeAuthorizer{allowed: true},
		authzcenter.NewHandler(service, authService),
	)
	cookie := routeLogin(t, server, "admin", "admin")

	req := httptest.NewRequest(http.MethodGet, "/api/authz/overview", nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected overview to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	var body authzcenter.AuthzOverviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode overview response: %v", err)
	}
	if body.Engine.Engine != "db" {
		t.Fatalf("expected db engine, got %#v", body.Engine)
	}
	if repo.lastTenantID != uuid.MustParse(auth.DefaultTenantID) {
		t.Fatalf("expected overview repository calls to use default tenant, got %s", repo.lastTenantID)
	}
}

func TestAuthzCenterOverviewDeniedAuthorizationReturnsForbidden(t *testing.T) {
	authRepo := newRouteAuthRepo()
	authService, err := auth.NewService(authRepo)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	if _, err := authService.CreateUser(context.Background(), "viewer", "viewer"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	repo := &routeAuthzCenterRepo{}
	authorizer := &routeAuthorizer{allowed: false}
	service := authzcenter.NewService(repo, authorizer)
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		authorizer,
		authzcenter.NewHandler(service, authService),
	)
	cookie := routeLogin(t, server, "viewer", "viewer")

	req := httptest.NewRequest(http.MethodGet, "/api/authz/overview", nil)
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected denied read to return forbidden, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(authorizer.checks) != 1 || authorizer.checks[0].Action != authz.ActionAuthzCenterRead {
		t.Fatalf("expected one authz center read check, got %#v", authorizer.checks)
	}
}

func TestAuthzCenterRuntimeScopeCreateRecordsCheckAndOperationLog(t *testing.T) {
	authRepo := newRouteAuthRepo()
	authService, err := auth.NewService(authRepo)
	if err != nil {
		t.Fatalf("new auth service: %v", err)
	}
	user, err := authService.CreateUser(context.Background(), "admin", "admin")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	repo := &routeAuthzCenterRepo{}
	authorizer := &routeAuthorizer{allowed: true}
	service := authzcenter.NewService(repo, authorizer)
	server := NewServerWithAuthz(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		authService,
		nil,
		authorizer,
		authzcenter.NewHandler(service, authService),
	)
	cookie := routeLogin(t, server, "admin", "admin")
	tenantID := uuid.MustParse(auth.DefaultTenantID)
	nodeID := uuid.MustParse("00000000-0000-0000-0000-000000000201")

	req := httptest.NewRequest(http.MethodPost, "/api/authz/runtime-scopes", strings.NewReader(`{
		"tenant_id":"`+tenantID.String()+`",
		"runtime_node_id":"`+nodeID.String()+`",
		"scope_type":"tenant",
		"scope_value":"`+tenantID.String()+`"
	}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected runtime scope create to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(authorizer.checks) != 1 {
		t.Fatalf("expected one authorization check, got %#v", authorizer.checks)
	}
	check := authorizer.checks[0]
	if check.Action != authz.ActionRuntimeScopeManage {
		t.Fatalf("expected runtime scope manage check, got %q", check.Action)
	}
	if check.Actor.ID != user.ID.String() || check.Resource.Type != authz.ResourceTenant || check.Resource.ID != tenantID.String() {
		t.Fatalf("unexpected authorization check: %#v", check)
	}
	if len(repo.operationLogs) != 1 {
		t.Fatalf("expected one operation log, got %#v", repo.operationLogs)
	}
	if repo.operationLogs[0].Action != authzcenter.OperationActionRuntimeScopeCreate {
		t.Fatalf("expected runtime scope create operation, got %#v", repo.operationLogs[0])
	}
}

func TestRuntimeRoutesUseAuthenticatedNodeIdentity(t *testing.T) {
	server := NewServer(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		&routeRuntimeAuthService{},
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/heartbeat", strings.NewReader(`{"current_load":2}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-ID", "node-1")
	rr := httptest.NewRecorder()

	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected authenticated heartbeat to reach handler, got %d: %s", rr.Code, rr.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode heartbeat response: %v", err)
	}
	if body["node_id"] != "node-1" {
		t.Fatalf("expected node_id from auth context, got %#v", body["node_id"])
	}
}

func TestRuntimeRoutesAcceptRuntimeSessionTokenIdentity(t *testing.T) {
	service := &routeRuntimeService{}
	server := NewServerWithRuntimeSessionAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{}),
		&routeRuntimeAuthService{},
		service,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/heartbeat", strings.NewReader(`{"current_load":2}`))
	req.Header.Set("Authorization", "Bearer session-token")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected session-authenticated heartbeat to reach handler, got %d: %s", rr.Code, rr.Body.String())
	}
	if service.heartbeatReq.NodeID != "node-session" {
		t.Fatalf("expected heartbeat node_id from runtime session validation, got %#v", service.heartbeatReq)
	}
	if service.validatedSessionToken != "session-token" {
		t.Fatalf("expected runtime session token to be validated, got %q", service.validatedSessionToken)
	}
}

func TestRuntimeRoutesFallbackToLegacyWhenSessionTokenInvalid(t *testing.T) {
	service := &routeRuntimeService{}
	server := NewServerWithRuntimeSessionAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{}),
		&routeRuntimeAuthService{},
		service,
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/heartbeat", strings.NewReader(`{"current_load":2}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("X-Node-ID", "node-1")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected legacy runtime auth fallback to reach heartbeat, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.validatedSessionToken != "test-token" {
		t.Fatalf("expected session auth to be attempted first, got token %q", service.validatedSessionToken)
	}
	if service.heartbeatReq.NodeID != "node-1" {
		t.Fatalf("expected heartbeat to use legacy X-Node-ID identity, got %#v", service.heartbeatReq)
	}
}

func TestRuntimeRoutesRejectMissingRuntimeAuth(t *testing.T) {
	server := NewServer(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		&routeRuntimeAuthService{},
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/heartbeat", strings.NewReader(`{"current_load":2}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing runtime auth to be rejected, got %d", rr.Code)
	}
}

func TestRuntimeBootstrapHelloBodyCannotAccessProtectedRuntimeRoutes(t *testing.T) {
	service := &routeRuntimeService{}
	server := NewServerWithRuntimeSessionAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{}),
		&routeRuntimeAuthService{},
		service,
	)

	heartbeatReq := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/heartbeat", strings.NewReader(`{"node_id":"node-hello","bootstrap_key":"bootstrap-secret","current_load":2}`))
	heartbeatReq.Header.Set("Content-Type", "application/json")
	heartbeatResp := httptest.NewRecorder()
	server.ServeHTTP(heartbeatResp, heartbeatReq)
	if heartbeatResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected heartbeat to reject bootstrap body without bearer auth, got %d: %s", heartbeatResp.Code, heartbeatResp.Body.String())
	}

	capReq := httptest.NewRequest(http.MethodPut, "/api/v1/runtime/nodes/node-hello/capabilities", strings.NewReader(`{"capabilities":[{"capability_type":"provider","capability_key":"codex","provider_type":"codex","available":true}]}`))
	capReq.Header.Set("Content-Type", "application/json")
	capResp := httptest.NewRecorder()
	server.ServeHTTP(capResp, capReq)
	if capResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected capabilities to reject bootstrap body without session bearer auth, got %d: %s", capResp.Code, capResp.Body.String())
	}
}

func TestRuntimeSessionRenewRequiresSessionAuthAndReturnsBareSession(t *testing.T) {
	service := &routeRuntimeService{}
	server := NewServerWithRuntimeSessionAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{}),
		&routeRuntimeAuthService{},
		service,
	)

	missingReq := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/session/renew", nil)
	missingResp := httptest.NewRecorder()
	server.ServeHTTP(missingResp, missingReq)
	if missingResp.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing session auth to be rejected, got %d", missingResp.Code)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/session/renew", nil)
	req.Header.Set("Authorization", "Bearer session-token")
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected session renew to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	if service.renewedSessionToken != "session-token" {
		t.Fatalf("expected renew to use bearer session token, got %q", service.renewedSessionToken)
	}
	var body struct {
		ID        string `json:"id"`
		NodeID    string `json:"node_id"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode renew response: %v", err)
	}
	if body.ID != "33333333-3333-3333-3333-333333333333" || body.NodeID != "node-session" || body.ExpiresAt == "" {
		t.Fatalf("expected bare renewed session response, got %#v", body)
	}
}

func TestRuntimeSessionCanonicalRenewValidatesPathSessionID(t *testing.T) {
	service := &routeRuntimeService{}
	server := NewServerWithRuntimeSessionAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{}),
		&routeRuntimeAuthService{},
		service,
	)

	mismatchReq := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/sessions/55555555-5555-5555-5555-555555555555/renew", nil)
	mismatchReq.Header.Set("Authorization", "Bearer session-token")
	mismatchResp := httptest.NewRecorder()
	server.ServeHTTP(mismatchResp, mismatchReq)
	if mismatchResp.Code != http.StatusForbidden {
		t.Fatalf("expected canonical renew path session mismatch to be rejected, got %d: %s", mismatchResp.Code, mismatchResp.Body.String())
	}
	if service.renewedSessionToken != "" {
		t.Fatalf("expected mismatch not to call renew service, got token %q", service.renewedSessionToken)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/sessions/33333333-3333-3333-3333-333333333333/renew", nil)
	req.Header.Set("Authorization", "Bearer session-token")
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected canonical renew to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode canonical renew response: %v", err)
	}
	if body["id"] != "33333333-3333-3333-3333-333333333333" || body["expires_at"] == nil {
		t.Fatalf("expected canonical renew to return bare runtime session, got %#v", body)
	}
	if _, ok := body["session"]; ok {
		t.Fatalf("did not expect wrapped session response, got %#v", body)
	}
}

func TestRuntimeCapabilitiesRejectPathNodeMismatch(t *testing.T) {
	service := &routeRuntimeService{}
	server := NewServerWithRuntimeSessionAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{}),
		&routeRuntimeAuthService{},
		service,
	)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/runtime/nodes/other-node/capabilities", strings.NewReader(`{"capabilities":[{"capability_type":"provider","capability_key":"codex","provider_type":"codex","available":true}]}`))
	req.Header.Set("Authorization", "Bearer session-token")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected path node mismatch to be forbidden, got %d: %s", resp.Code, resp.Body.String())
	}
	if len(service.upsertedCapabilities) != 0 {
		t.Fatalf("expected mismatched node path not to upsert capabilities, got %#v", service.upsertedCapabilities)
	}
}

func TestRuntimeCapabilitiesSuccessReturnsTopLevelArray(t *testing.T) {
	service := &routeRuntimeService{}
	server := NewServerWithRuntimeSessionAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{}),
		&routeRuntimeAuthService{},
		service,
	)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/runtime/nodes/node-session/capabilities", strings.NewReader(`{"capabilities":[{"capability_type":"provider","capability_key":"codex","provider_type":"codex","available":true}]}`))
	req.Header.Set("Authorization", "Bearer session-token")
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected capabilities upsert to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	var body []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("expected top-level capability array response: %v; body=%s", err, resp.Body.String())
	}
	if len(body) != 1 || body[0]["capability_type"] != "provider" || body[0]["capability_key"] != "codex" {
		t.Fatalf("unexpected capabilities response: %#v", body)
	}
}

func TestRuntimeWebSocketRequiresRuntimeSessionAuth(t *testing.T) {
	service := &routeRuntimeService{}
	server := NewServerWithRuntimeSessionAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{}),
		&routeRuntimeAuthService{},
		service,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/ws", nil)
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected websocket route to require runtime session auth, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestRuntimeWebSocketRejectsLegacyRuntimeToken(t *testing.T) {
	service := &routeRuntimeService{}
	server := NewServerWithRuntimeSessionAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{}),
		&routeRuntimeAuthService{},
		service,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/ws", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("X-Node-ID", "node-1")
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected websocket route to reject legacy runtime token, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestRuntimeWebSocketReturnsServiceUnavailableWhenRegistryMissing(t *testing.T) {
	service := &routeRuntimeService{}
	server := NewServerWithRuntimeSessionAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{}),
		&routeRuntimeAuthService{},
		service,
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runtime/ws", nil)
	req.Header.Set("Authorization", "Bearer session-token")
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected websocket route without registry to return 503, got %d: %s", resp.Code, resp.Body.String())
	}
}

func TestRuntimeWebSocketSendsDispatchedCommand(t *testing.T) {
	service := &routeRuntimeService{}
	registry := runtime.NewConnectionRegistry()
	runtimeHandler := handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{})
	runtimeHandler.SetConnectionRegistry(registry)
	server := NewServerWithRuntimeSessionAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		runtimeHandler,
		&routeRuntimeAuthService{},
		service,
	)
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/api/v1/runtime/ws"
	headers := http.Header{}
	headers.Set("Authorization", "Bearer session-token")
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatalf("dial runtime websocket: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "test done")
	waitForRuntimeConnection(t, registry, "node-session")

	command := runtime.RuntimeCommand{
		ID:      "cmd-1",
		Type:    "task.claim",
		Payload: json.RawMessage(`{"task_id":"task-1"}`),
	}
	if err := registry.Dispatch(ctx, "node-session", command); err != nil {
		t.Fatalf("dispatch command: %v", err)
	}

	messageType, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read websocket command: %v", err)
	}
	if messageType != websocket.MessageText {
		t.Fatalf("expected text websocket message, got %v", messageType)
	}
	var got runtime.RuntimeCommand
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode websocket command: %v; data=%s", err, string(data))
	}
	if got.ID != command.ID || got.Type != command.Type || string(got.Payload) != string(command.Payload) {
		t.Fatalf("unexpected websocket command: %#v", got)
	}
}

func TestRuntimeWebSocketRejectsCrossOriginHandshake(t *testing.T) {
	service := &routeRuntimeService{}
	registry := runtime.NewConnectionRegistry()
	runtimeHandler := handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{})
	runtimeHandler.SetConnectionRegistry(registry)
	server := NewServerWithRuntimeSessionAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		runtimeHandler,
		&routeRuntimeAuthService{},
		service,
	)
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/api/v1/runtime/ws"
	headers := http.Header{}
	headers.Set("Authorization", "Bearer session-token")
	headers.Set("Origin", "https://console.example.com")

	_, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: headers})
	if err == nil {
		t.Fatal("expected cross-origin runtime websocket handshake to be rejected")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected forbidden cross-origin websocket handshake, got %v", err)
	}
}

func TestRuntimeWebSocketClientCloseUnregistersConnection(t *testing.T) {
	service := &routeRuntimeService{}
	registry := runtime.NewConnectionRegistry()
	runtimeHandler := handlers.NewRuntimeHandler(service, &routeTaskService{}, &routePoller{})
	runtimeHandler.SetConnectionRegistry(registry)
	server := NewServerWithRuntimeSessionAuth(
		handlers.NewTaskHandler(&routeTaskService{}),
		runtimeHandler,
		&routeRuntimeAuthService{},
		service,
	)
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/api/v1/runtime/ws"
	headers := http.Header{}
	headers.Set("Authorization", "Bearer session-token")
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatalf("dial runtime websocket: %v", err)
	}
	waitForRuntimeConnection(t, registry, "node-session")
	if err := registry.Dispatch(ctx, "node-session", runtime.RuntimeCommand{ID: "cmd-before-close", Type: "noop"}); err != nil {
		t.Fatalf("expected connected runtime to accept dispatch before close: %v", err)
	}

	if err := conn.Close(websocket.StatusNormalClosure, "test done"); err != nil {
		t.Fatalf("close runtime websocket: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		dispatchCtx, dispatchCancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		err := registry.Dispatch(dispatchCtx, "node-session", runtime.RuntimeCommand{ID: "cmd-after-close", Type: "noop"})
		dispatchCancel()
		if errors.Is(err, runtime.ErrRuntimeNotConnected) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected runtime websocket close to unregister connection")
}

func waitForRuntimeConnection(t *testing.T, registry *runtime.ConnectionRegistry, nodeID string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if registry.IsConnected(nodeID) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for runtime websocket connection %q", nodeID)
}

func TestRuntimeRegisterRejectsMismatchedAuthenticatedNodeIdentity(t *testing.T) {
	server := NewServer(
		handlers.NewTaskHandler(&routeTaskService{}),
		handlers.NewRuntimeHandler(&routeRuntimeService{}, &routeTaskService{}, &routePoller{}),
		&routeRuntimeAuthService{},
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/runtime/register", strings.NewReader(`{"node_id":"node-2","name":"node 2","supported_providers":["codex"],"max_slots":1}`))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Node-ID", "node-1")
	rr := httptest.NewRecorder()

	server.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected mismatched runtime node identity to be rejected, got %d: %s", rr.Code, rr.Body.String())
	}
}

type routeRuntimeService struct {
	enrollHelloReq          runtime.EnrollHelloRequest
	heartbeatReq            runtime.UpdateHeartbeatRequest
	validatedSessionToken   string
	renewedSessionToken     string
	upsertedCapabilities    []runtime.RuntimeCapabilityInput
	listEnrollmentsCalled   bool
	getOverviewCalled       bool
	listEventsCalled        bool
	listCapabilitiesCalled  bool
	approveEnrollmentCalled bool
	rejectEnrollmentCalled  bool
	revokeEnrollmentCalled  bool
	overviewTenantID        uuid.UUID
	eventsFilter            runtime.ListRuntimeEventsFilter
	capabilitiesTenantID    uuid.UUID
	capabilitiesNodeID      string
	runtimeOverview         *runtime.RuntimeOverview
	runtimeEvents           []runtime.RuntimeEvent
	approvedEnrollmentID    uuid.UUID
	rejectedReason          string
	revokedReason           string
	listEnrollmentsTenantID uuid.UUID
	approveTenantID         uuid.UUID
	rejectTenantID          uuid.UUID
	revokeTenantID          uuid.UUID
	approvedBy              uuid.UUID
	rejectedBy              uuid.UUID
	revokedBy               uuid.UUID
}

func (s *routeRuntimeService) RegisterNode(ctx context.Context, req runtime.RegisterNodeRequest) (*runtime.Node, error) {
	return &runtime.Node{NodeID: req.NodeID, Name: req.Name, SupportedProviders: req.SupportedProviders, MaxSlots: req.MaxSlots}, nil
}

func (s *routeRuntimeService) UpdateHeartbeat(ctx context.Context, req runtime.UpdateHeartbeatRequest) (*runtime.Node, error) {
	s.heartbeatReq = req
	return &runtime.Node{NodeID: req.NodeID, CurrentLoad: req.CurrentLoad}, nil
}

func (s *routeRuntimeService) GetNode(ctx context.Context, nodeID string) (*runtime.Node, error) {
	return &runtime.Node{NodeID: nodeID, SupportedProviders: []string{"codex"}}, nil
}

func (s *routeRuntimeService) ListNodes(ctx context.Context, filter runtime.ListNodesFilter) ([]*runtime.Node, error) {
	return []*runtime.Node{}, nil
}

func (s *routeRuntimeService) EnrollHello(ctx context.Context, req runtime.EnrollHelloRequest) (*runtime.EnrollHelloResponse, error) {
	s.enrollHelloReq = req
	return &runtime.EnrollHelloResponse{
		Enrollment: runtime.RuntimeEnrollment{
			ID:             uuid.MustParse(routeTaskID),
			TenantID:       runtime.DefaultTenantID,
			NodeID:         req.NodeID,
			BootstrapKeyID: uuid.MustParse("22222222-2222-2222-2222-222222222222"),
			Status:         runtime.RuntimeEnrollmentStatusPending,
			RequestPayload: map[string]interface{}{},
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
			LastHelloAt:    time.Now(),
		},
	}, nil
}

func (s *routeRuntimeService) ListRuntimeEnrollments(ctx context.Context, filter runtime.ListRuntimeEnrollmentsFilter) ([]*runtime.RuntimeEnrollment, error) {
	s.listEnrollmentsCalled = true
	s.listEnrollmentsTenantID = filter.TenantID
	return []*runtime.RuntimeEnrollment{}, nil
}

func (s *routeRuntimeService) GetOverview(ctx context.Context, filter runtime.RuntimeOverviewFilter) (*runtime.RuntimeOverview, error) {
	s.getOverviewCalled = true
	s.overviewTenantID = filter.TenantID
	if s.runtimeOverview != nil {
		return s.runtimeOverview, nil
	}
	return &runtime.RuntimeOverview{}, nil
}

func (s *routeRuntimeService) ListRuntimeEvents(ctx context.Context, filter runtime.ListRuntimeEventsFilter) ([]runtime.RuntimeEvent, error) {
	s.listEventsCalled = true
	s.eventsFilter = filter
	if s.runtimeEvents != nil {
		return s.runtimeEvents, nil
	}
	return []runtime.RuntimeEvent{}, nil
}

func (s *routeRuntimeService) ListRuntimeCapabilitiesForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]runtime.RuntimeCapability, error) {
	s.listCapabilitiesCalled = true
	s.capabilitiesTenantID = tenantID
	s.capabilitiesNodeID = nodeID
	return []runtime.RuntimeCapability{}, nil
}

func (s *routeRuntimeService) ApproveEnrollment(ctx context.Context, req runtime.ApproveEnrollmentRequest) (*runtime.RuntimeEnrollment, error) {
	s.approveEnrollmentCalled = true
	s.approvedEnrollmentID = req.EnrollmentID
	s.approveTenantID = req.TenantID
	s.approvedBy = req.ApprovedBy
	return &runtime.RuntimeEnrollment{ID: req.EnrollmentID, TenantID: runtime.DefaultTenantID, Status: runtime.RuntimeEnrollmentStatusApproved}, nil
}

func (s *routeRuntimeService) RejectEnrollment(ctx context.Context, req runtime.RejectEnrollmentRequest) (*runtime.RuntimeEnrollment, error) {
	s.rejectEnrollmentCalled = true
	s.rejectedReason = req.Reason
	s.rejectTenantID = req.TenantID
	s.rejectedBy = req.RejectedBy
	return &runtime.RuntimeEnrollment{ID: req.EnrollmentID, TenantID: runtime.DefaultTenantID, Status: runtime.RuntimeEnrollmentStatusRejected, RejectReason: &req.Reason}, nil
}

func (s *routeRuntimeService) RevokeEnrollment(ctx context.Context, req runtime.RevokeEnrollmentRequest) (*runtime.RuntimeEnrollment, error) {
	s.revokeEnrollmentCalled = true
	s.revokedReason = req.Reason
	s.revokeTenantID = req.TenantID
	s.revokedBy = req.RevokedBy
	return &runtime.RuntimeEnrollment{ID: req.EnrollmentID, TenantID: runtime.DefaultTenantID, Status: runtime.RuntimeEnrollmentStatusRevoked, RevokeReason: &req.Reason}, nil
}

func (s *routeRuntimeService) ValidateRuntimeSession(ctx context.Context, token string) (*runtime.RuntimeSessionValidation, error) {
	s.validatedSessionToken = token
	if token != "session-token" {
		return nil, context.Canceled
	}
	return &runtime.RuntimeSessionValidation{
		SessionID:     uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		TenantID:      runtime.DefaultTenantID,
		RuntimeNodeID: uuid.MustParse("44444444-4444-4444-4444-444444444444"),
		NodeID:        "node-session",
		ExpiresAt:     time.Now().Add(time.Hour),
	}, nil
}

func (s *routeRuntimeService) RenewRuntimeSession(ctx context.Context, token string) (*runtime.RuntimeSession, error) {
	s.renewedSessionToken = token
	return &runtime.RuntimeSession{
		ID:            uuid.MustParse("33333333-3333-3333-3333-333333333333"),
		TenantID:      runtime.DefaultTenantID,
		RuntimeNodeID: uuid.MustParse("44444444-4444-4444-4444-444444444444"),
		NodeID:        "node-session",
		ExpiresAt:     time.Now().Add(time.Hour),
		LastSeenAt:    time.Now(),
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}, nil
}

func (s *routeRuntimeService) UpsertCapabilities(ctx context.Context, token string, capabilities []runtime.RuntimeCapabilityInput) ([]runtime.RuntimeCapability, error) {
	s.upsertedCapabilities = capabilities
	return []runtime.RuntimeCapability{{
		ID:             uuid.MustParse("66666666-6666-6666-6666-666666666666"),
		TenantID:       runtime.DefaultTenantID,
		RuntimeNodeID:  uuid.MustParse("44444444-4444-4444-4444-444444444444"),
		CapabilityType: capabilities[0].CapabilityType,
		CapabilityKey:  capabilities[0].CapabilityKey,
		ProviderType:   capabilities[0].ProviderType,
		Available:      capabilities[0].Available,
		Status:         "active",
		HealthStatus:   "ok",
		LastSeenAt:     time.Now(),
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}}, nil
}

func (s *fakeRuntimeService) GetOverview(ctx context.Context, filter runtime.RuntimeOverviewFilter) (*runtime.RuntimeOverview, error) {
	return &runtime.RuntimeOverview{}, nil
}

func (s *fakeRuntimeService) ListRuntimeEvents(ctx context.Context, filter runtime.ListRuntimeEventsFilter) ([]runtime.RuntimeEvent, error) {
	return []runtime.RuntimeEvent{}, nil
}

func (s *fakeRuntimeService) ListRuntimeCapabilitiesForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]runtime.RuntimeCapability, error) {
	return []runtime.RuntimeCapability{}, nil
}

type routeTaskService struct {
	tasks          []*task.Task
	assignedTaskID uuid.UUID
}

func (s *routeTaskService) CreateTask(ctx context.Context, req task.CreateTaskRequest) (*task.Task, error) {
	return &task.Task{ID: uuid.New(), Title: req.Title, ProviderType: req.ProviderType}, nil
}

func (s *routeTaskService) GetTask(ctx context.Context, taskID uuid.UUID) (*task.Task, error) {
	return &task.Task{ID: taskID, ProviderType: "codex"}, nil
}

func (s *routeTaskService) ListTasks(ctx context.Context, filter task.ListTasksFilter) ([]*task.Task, error) {
	if s.tasks == nil {
		return []*task.Task{}, nil
	}
	tasks := make([]*task.Task, 0, len(s.tasks))
	for _, t := range s.tasks {
		if filter.ProviderType != nil && t.ProviderType != *filter.ProviderType {
			continue
		}
		if filter.Status != nil && t.Status != "" && t.Status != *filter.Status {
			continue
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (s *routeTaskService) AppendTaskEvent(ctx context.Context, req task.AppendTaskEventRequest) (*task.TaskEvent, error) {
	return &task.TaskEvent{TaskID: req.TaskID, EventType: req.EventType, Payload: req.Payload}, nil
}

func (s *routeTaskService) UpdateTaskStatus(ctx context.Context, req task.UpdateTaskStatusRequest) (*task.Task, error) {
	return &task.Task{ID: req.TaskID, Status: req.NewStatus}, nil
}

func (s *routeTaskService) CancelTask(ctx context.Context, taskID uuid.UUID, cancelledBy *string, reason *string) (*task.Task, error) {
	return &task.Task{ID: taskID, Status: task.TaskStatusCancelled}, nil
}

func (s *routeTaskService) AssignTask(ctx context.Context, req task.AssignTaskRequest) (*task.Task, error) {
	s.assignedTaskID = req.TaskID
	return &task.Task{ID: req.TaskID, AssignedNodeID: &req.AssignedNodeID}, nil
}

type routePoller struct{}

func (p *routePoller) WaitForTask(ctx context.Context, nodeID string) (*task.Task, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type routeRuntimeAuthService struct{}

func (s *routeRuntimeAuthService) ValidateRuntimeToken(ctx context.Context, nodeID, token string) error {
	if nodeID != "node-1" || token != "test-token" {
		return context.Canceled
	}
	return nil
}

type routeAuthRepo struct {
	users               map[string]*auth.User
	usersByID           map[uuid.UUID]*auth.User
	sessions            map[string]*auth.Session
	loginLogs           []auth.LoginLog
	operationLogs       []auth.CreateOperationLogParams
	lastListUsersFilter auth.ListUsersFilter
}

func newRouteAuthRepo() *routeAuthRepo {
	return &routeAuthRepo{
		users:         map[string]*auth.User{},
		usersByID:     map[uuid.UUID]*auth.User{},
		sessions:      map[string]*auth.Session{},
		loginLogs:     []auth.LoginLog{},
		operationLogs: []auth.CreateOperationLogParams{},
	}
}

func (r *routeAuthRepo) CreateUser(ctx context.Context, username, passwordHash string, avatar auth.UserAvatarConfig) (*auth.User, error) {
	user := &auth.User{ID: uuid.New(), Username: username, PasswordHash: passwordHash, Status: "active", Avatar: avatar}
	r.users[username] = user
	r.usersByID[user.ID] = user
	return user, nil
}

func (r *routeAuthRepo) GetUserByUsername(ctx context.Context, username string) (*auth.User, error) {
	user, ok := r.users[username]
	if !ok {
		return nil, auth.ErrInvalidCredentials
	}
	return user, nil
}

func (r *routeAuthRepo) ListUsers(ctx context.Context, filter auth.ListUsersFilter) ([]*auth.User, error) {
	r.lastListUsersFilter = filter
	users := make([]*auth.User, 0, len(r.usersByID))
	for _, user := range r.usersByID {
		if filter.Status != "" && user.Status != filter.Status {
			continue
		}
		if filter.Q != "" && !strings.Contains(user.Username, filter.Q) {
			continue
		}
		users = append(users, user)
	}
	return users, nil
}

func (r *routeAuthRepo) GetUserByID(ctx context.Context, id uuid.UUID) (*auth.User, error) {
	user, ok := r.usersByID[id]
	if !ok {
		return nil, auth.ErrUnauthorized
	}
	return user, nil
}

func (r *routeAuthRepo) UpdateUserStatus(ctx context.Context, userID uuid.UUID, status string) (*auth.User, error) {
	user, ok := r.usersByID[userID]
	if !ok {
		return nil, auth.ErrUnauthorized
	}
	user.Status = status
	return user, nil
}

func (r *routeAuthRepo) UpdateUserPassword(ctx context.Context, userID uuid.UUID, passwordHash string) (*auth.User, error) {
	user, ok := r.usersByID[userID]
	if !ok {
		return nil, auth.ErrUnauthorized
	}
	user.PasswordHash = passwordHash
	return user, nil
}

func (r *routeAuthRepo) UpdateUserProfile(ctx context.Context, userID uuid.UUID, input auth.UpdateUserProfileInput) (*auth.User, error) {
	user, ok := r.usersByID[userID]
	if !ok {
		return nil, auth.ErrUnauthorized
	}
	user.DisplayName = input.DisplayName
	user.Email = input.Email
	user.Avatar = input.Avatar
	return user, nil
}

func (r *routeAuthRepo) CreateRuntimeToken(ctx context.Context, nodeID, tokenHash string, expiresAt time.Time) error {
	return nil
}

func (r *routeAuthRepo) GetRuntimeTokenByNodeID(ctx context.Context, nodeID string) (*auth.RuntimeToken, error) {
	return nil, auth.ErrInvalidToken
}

func (r *routeAuthRepo) CreateSession(ctx context.Context, session *auth.Session, tokenHash string) error {
	if session.ID == uuid.Nil {
		session.ID = uuid.New()
	}
	r.sessions[tokenHash] = session
	return nil
}

func (r *routeAuthRepo) GetSessionByTokenHash(ctx context.Context, tokenHash string) (*auth.Session, error) {
	session, ok := r.sessions[tokenHash]
	if !ok {
		return nil, auth.ErrSessionNotFound
	}
	return session, nil
}

func (r *routeAuthRepo) DeleteSession(ctx context.Context, tokenHash string) error {
	delete(r.sessions, tokenHash)
	return nil
}

func (r *routeAuthRepo) UpdateSessionLastSeen(ctx context.Context, tokenHash string, lastSeenAt time.Time) error {
	session, ok := r.sessions[tokenHash]
	if !ok {
		return auth.ErrSessionNotFound
	}
	session.LastSeenAt = lastSeenAt
	return nil
}

func (r *routeAuthRepo) CreateLoginLog(ctx context.Context, params auth.CreateLoginLogParams) error {
	now := time.Now().UTC()
	r.loginLogs = append([]auth.LoginLog{
		{
			ID:            uuid.New(),
			EventType:     params.EventType,
			UserID:        params.UserID,
			Username:      params.Username,
			SessionID:     params.SessionID,
			ClientIP:      params.ClientIP,
			UserAgent:     params.UserAgent,
			Result:        params.Result,
			FailureReason: params.FailureReason,
			CreatedAt:     now,
		},
	}, r.loginLogs...)
	return nil
}

func (r *routeAuthRepo) ListLoginLogs(ctx context.Context, filter auth.ListLoginLogsFilter) ([]auth.LoginLog, error) {
	logs := r.loginLogs
	if filter.UserID != nil {
		filtered := make([]auth.LoginLog, 0, len(logs))
		for _, log := range logs {
			if log.UserID != nil && *log.UserID == *filter.UserID {
				filtered = append(filtered, log)
			}
		}
		logs = filtered
	}
	start := int(filter.Offset)
	if start >= len(logs) {
		return []auth.LoginLog{}, nil
	}
	end := start + int(filter.Limit)
	if end > len(logs) {
		end = len(logs)
	}
	return append([]auth.LoginLog{}, logs[start:end]...), nil
}

func (r *routeAuthRepo) CreateOperationLog(ctx context.Context, params auth.CreateOperationLogParams) error {
	r.operationLogs = append(r.operationLogs, params)
	return nil
}

func routeLogin(t *testing.T, server *Server, username, password string) *http.Cookie {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"username":"`+username+`","password":"`+password+`"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	server.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected login to succeed, got %d: %s", resp.Code, resp.Body.String())
	}
	cookies := resp.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("expected session cookie")
	}
	return cookies[0]
}

type routeAuthorizer struct {
	allowed     bool
	denyActions map[string]bool
	err         error
	checks      []authz.CheckRequest
}

func (a *routeAuthorizer) Check(ctx context.Context, req authz.CheckRequest) (authz.Decision, error) {
	a.checks = append(a.checks, req)
	if a.err != nil {
		return authz.Decision{}, a.err
	}
	if a.denyActions[req.Action] {
		return authz.Decision{Allowed: false, Reason: authz.ReasonNoMembership, RequiresAudit: true}, nil
	}
	if a.allowed {
		return authz.Decision{Allowed: true, Reason: authz.ReasonAllowed, MatchedRule: "test.allow"}, nil
	}
	return authz.Decision{Allowed: false, Reason: authz.ReasonNoMembership, RequiresAudit: true}, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type routeAuthzCenterRepo struct {
	operationLogs []authzcenter.OperationLogInput
	lastTenantID  uuid.UUID
}

func (r *routeAuthzCenterRepo) CountDecisionsSince(ctx context.Context, tenantID uuid.UUID, since time.Time) (authzcenter.DecisionTotals, error) {
	r.lastTenantID = tenantID
	return authzcenter.DecisionTotals{}, nil
}

func (r *routeAuthzCenterRepo) ListTopDeniedActionsSince(ctx context.Context, tenantID uuid.UUID, since time.Time, limit int32) ([]authzcenter.ActionCount, error) {
	r.lastTenantID = tenantID
	return []authzcenter.ActionCount{}, nil
}

func (r *routeAuthzCenterRepo) ListDecisions(ctx context.Context, filter authzcenter.DecisionFilter) ([]authzcenter.DecisionRecord, error) {
	return []authzcenter.DecisionRecord{}, nil
}

func (r *routeAuthzCenterRepo) ListRuntimeScopeNodes(ctx context.Context, tenantID uuid.UUID) ([]authzcenter.RuntimeScopeNodeRecord, error) {
	r.lastTenantID = tenantID
	return []authzcenter.RuntimeScopeNodeRecord{}, nil
}

func (r *routeAuthzCenterRepo) CreateRuntimeScope(ctx context.Context, input authzcenter.RuntimeScopeInput) (authzcenter.RuntimeScopeRecord, error) {
	now := time.Now().UTC()
	return authzcenter.RuntimeScopeRecord{
		ID:            uuid.New(),
		TenantID:      input.TenantID,
		RuntimeNodeID: input.RuntimeNodeID,
		TeamID:        input.TeamID,
		ScopeType:     authzcenter.RuntimeScopeScopeType(input.ScopeType),
		ScopeValue:    input.ScopeValue,
		Status:        authzcenter.RuntimeScopeStatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}

func (r *routeAuthzCenterRepo) UpdateRuntimeScopeStatus(ctx context.Context, tenantID uuid.UUID, scopeID uuid.UUID, status string) (authzcenter.RuntimeScopeRecord, error) {
	now := time.Now().UTC()
	return authzcenter.RuntimeScopeRecord{
		ID:        scopeID,
		TenantID:  tenantID,
		Status:    authzcenter.RuntimeScopeStatus(status),
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

func (r *routeAuthzCenterRepo) ListMembers(ctx context.Context, filter authzcenter.MemberFilter) ([]authzcenter.MemberRecord, error) {
	return []authzcenter.MemberRecord{}, nil
}

func (r *routeAuthzCenterRepo) RecordOperationLog(ctx context.Context, input authzcenter.OperationLogInput) error {
	r.operationLogs = append(r.operationLogs, input)
	return nil
}

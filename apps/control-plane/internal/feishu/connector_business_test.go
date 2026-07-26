package feishu

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/api/middleware"
)

// stubProjectGateway 只为 ResolveDecision 枚举门测试服务：记录透传值，其余方法不实现。
type stubProjectGateway struct {
	resolvedDecision string
	resolveCalled    bool
}

func (s *stubProjectGateway) ListProjectsForHumanMember(context.Context, uuid.UUID, uuid.UUID) ([]ProjectRef, error) {
	return nil, nil
}

func (s *stubProjectGateway) SubmitDemand(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, string, string) (uuid.UUID, string, error) {
	return uuid.Nil, "", nil
}

func (s *stubProjectGateway) ResolveDecision(_ context.Context, _, _, _, _ uuid.UUID, decision, _ string) (bool, error) {
	s.resolveCalled = true
	s.resolvedDecision = decision
	return false, nil
}

func (s *stubProjectGateway) DecisionCardSnapshot(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (map[string]any, error) {
	return nil, nil
}

func (s *stubProjectGateway) SignDemandCriterion(context.Context, SignCriterionGatewayRequest) (*SignCriterionOutcome, error) {
	return nil, nil
}

func connectorResolveRequestFor(t *testing.T, decision string) *http.Request {
	t.Helper()
	decisionID := uuid.New()
	body := `{"project_id":"` + uuid.NewString() + `","decision":"` + decision + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/connector/decisions/"+decisionID.String()+"/resolve", strings.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.TenantIDKey, uuid.New())
	ctx = context.WithValue(ctx, middleware.UserIDKey, uuid.New())
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("decisionId", decisionID.String())
	ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	return req.WithContext(ctx)
}

// 契约外值必须被枚举门拦下（与 Web resolve 腿同一 ResolveDecisionValue 值域），
// 不得到达业务网关。
func TestConnectorResolveDecisionRejectsNonContractDecision(t *testing.T) {
	gateway := &stubProjectGateway{}
	handler := &ConnectorHTTPHandler{}
	handler.SetProjectGateway(gateway)

	rec := httptest.NewRecorder()
	handler.ResolveDecision(rec, connectorResolveRequestFor(t, "totally_made_up"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "invalid decision") {
		t.Fatalf("expected invalid decision message, got %s", rec.Body.String())
	}
	if gateway.resolveCalled {
		t.Fatal("business gateway must not be called for non-contract decision")
	}
}

// 新纳入契约的动词要能穿过枚举门原值到达业务网关。
func TestConnectorResolveDecisionAcceptsExpandedEnumValues(t *testing.T) {
	for _, decision := range []string{"retry", "cancel_downstream", "reassign", "retry_planning", "close_demand"} {
		gateway := &stubProjectGateway{}
		handler := &ConnectorHTTPHandler{}
		handler.SetProjectGateway(gateway)

		rec := httptest.NewRecorder()
		handler.ResolveDecision(rec, connectorResolveRequestFor(t, decision))

		if rec.Code != http.StatusOK {
			t.Fatalf("decision %q: expected 200, got %d body=%s", decision, rec.Code, rec.Body.String())
		}
		if !gateway.resolveCalled || gateway.resolvedDecision != decision {
			t.Fatalf("decision %q: expected passthrough to gateway, got called=%v value=%q", decision, gateway.resolveCalled, gateway.resolvedDecision)
		}
	}
}

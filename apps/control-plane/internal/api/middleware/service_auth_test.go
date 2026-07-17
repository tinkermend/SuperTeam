package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

type fakeServiceAuth struct {
	identity ServiceIdentity
	err      error
	lastName string
	lastTok  string
}

func (f *fakeServiceAuth) ValidateServiceToken(_ context.Context, serviceName, token string) (ServiceIdentity, error) {
	f.lastName, f.lastTok = serviceName, token
	if f.err != nil {
		return ServiceIdentity{}, f.err
	}
	return f.identity, nil
}

type fakeOBOResolver struct {
	err        error
	lastUserID uuid.UUID
	lastOpenID string
}

func (f *fakeOBOResolver) VerifyOnBehalfOf(_ context.Context, _ uuid.UUID, actingUserID uuid.UUID, openID string) error {
	f.lastUserID, f.lastOpenID = actingUserID, openID
	return f.err
}

func runServiceAuth(t *testing.T, auth ServiceAuthService, resolver OnBehalfOfResolver, mutate func(*http.Request)) (*httptest.ResponseRecorder, context.Context) {
	t.Helper()
	var captured context.Context
	handler := ServiceAuth(auth, resolver)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.Context()
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/connector/outbox", nil)
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec, captured
}

func TestServiceAuthRejectsMissingCredentials(t *testing.T) {
	auth := &fakeServiceAuth{identity: ServiceIdentity{TokenID: uuid.New(), TenantID: uuid.New()}}

	rec, _ := runServiceAuth(t, auth, nil, nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without bearer, got %d", rec.Code)
	}

	rec, _ = runServiceAuth(t, auth, nil, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer svc_x")
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without service name, got %d", rec.Code)
	}
}

func TestServiceAuthRejectsInvalidToken(t *testing.T) {
	auth := &fakeServiceAuth{err: errors.New("nope")}
	rec, _ := runServiceAuth(t, auth, nil, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer svc_bad")
		r.Header.Set("X-Service-Name", "feishu-connector")
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid token, got %d", rec.Code)
	}
}

func TestServiceAuthInjectsTenantWithoutOnBehalfOf(t *testing.T) {
	tenantID := uuid.New()
	auth := &fakeServiceAuth{identity: ServiceIdentity{TokenID: uuid.New(), TenantID: tenantID}}
	rec, ctx := runServiceAuth(t, auth, nil, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer svc_good")
		r.Header.Set("X-Service-Name", "feishu-connector")
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if GetTenantID(ctx) != tenantID {
		t.Fatalf("expected tenant injected")
	}
	if GetServiceName(ctx) != "feishu-connector" {
		t.Fatalf("expected service name injected")
	}
	if GetUserID(ctx) != uuid.Nil {
		t.Fatalf("expected no acting user without on-behalf-of")
	}
}

func TestServiceAuthOnBehalfOfVerifiedAndInjected(t *testing.T) {
	tenantID := uuid.New()
	actingID := uuid.New()
	auth := &fakeServiceAuth{identity: ServiceIdentity{TokenID: uuid.New(), TenantID: tenantID}}
	resolver := &fakeOBOResolver{}
	rec, ctx := runServiceAuth(t, auth, resolver, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer svc_good")
		r.Header.Set("X-Service-Name", "feishu-connector")
		r.Header.Set("X-On-Behalf-Of", actingID.String())
		r.Header.Set("X-Feishu-Open-Id", "ou_abc")
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if GetUserID(ctx) != actingID {
		t.Fatalf("expected acting user injected as UserID")
	}
	if GetActingOpenID(ctx) != "ou_abc" {
		t.Fatalf("expected open id injected")
	}
	if resolver.lastUserID != actingID || resolver.lastOpenID != "ou_abc" {
		t.Fatalf("expected resolver called with claim, got %s/%s", resolver.lastUserID, resolver.lastOpenID)
	}
}

func TestServiceAuthOnBehalfOfMismatchForbidden(t *testing.T) {
	auth := &fakeServiceAuth{identity: ServiceIdentity{TokenID: uuid.New(), TenantID: uuid.New()}}
	resolver := &fakeOBOResolver{err: errors.New("mismatch")}
	rec, _ := runServiceAuth(t, auth, resolver, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer svc_good")
		r.Header.Set("X-Service-Name", "feishu-connector")
		r.Header.Set("X-On-Behalf-Of", uuid.New().String())
		r.Header.Set("X-Feishu-Open-Id", "ou_evil")
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for identity mismatch, got %d", rec.Code)
	}
}

func TestServiceAuthOnBehalfOfRequiresOpenID(t *testing.T) {
	auth := &fakeServiceAuth{identity: ServiceIdentity{TokenID: uuid.New(), TenantID: uuid.New()}}
	resolver := &fakeOBOResolver{}
	rec, _ := runServiceAuth(t, auth, resolver, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer svc_good")
		r.Header.Set("X-Service-Name", "feishu-connector")
		r.Header.Set("X-On-Behalf-Of", uuid.New().String())
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without open id, got %d", rec.Code)
	}
}

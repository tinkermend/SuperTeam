package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestHealthEndpointReturnsControlPlaneStatus(t *testing.T) {
	router := NewHealthOnlyRouter()
	assertHealthResponse(t, router)
}

func TestProductServerHealthEndpointReturnsControlPlaneStatus(t *testing.T) {
	server := NewServer(nil, nil)
	assertHealthResponse(t, server)
}

func assertHealthResponse(t *testing.T, handler http.Handler) {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON health response: %v", err)
	}

	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", body["status"])
	}

	if body["service"] != "control-plane" {
		t.Fatalf("expected service control-plane, got %q", body["service"])
	}
}

func TestServerListenAndServeReturnsCleanlyWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	server := &Server{router: chi.NewRouter()}

	if err := server.ListenAndServe(ctx, "127.0.0.1:0"); err != nil {
		t.Fatalf("expected clean shutdown after context cancellation, got %v", err)
	}
}

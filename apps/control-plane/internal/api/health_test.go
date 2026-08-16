package api

import (
	"context"
	"encoding/json"
	"errors"
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

	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON health response: %v", err)
	}

	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %#v", body["status"])
	}

	if body["service"] != "control-plane" {
		t.Fatalf("expected service control-plane, got %#v", body["service"])
	}
	pc, ok := body["provider_contract"].(map[string]any)
	if !ok {
		t.Fatalf("expected provider_contract object, got %#v", body["provider_contract"])
	}
	if _, ok := pc["schema_violation_count"]; !ok {
		t.Fatalf("expected schema_violation_count in provider_contract: %#v", pc)
	}
	if engine, _ := pc["schema_engine"].(string); engine != "jsonschema" && engine != "structural" {
		t.Fatalf("unexpected schema_engine: %#v", pc["schema_engine"])
	}
}

func TestProductServerHealthProbeFailureReturns503(t *testing.T) {
	server := NewServer(nil, nil)
	server.SetObjectStoreHealthProbe(func(context.Context) error {
		return errors.New("head bucket missing")
	}, "missing-bucket")

	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v", err)
	}
	if body["status"] != "degraded" {
		t.Fatalf("status=%#v", body["status"])
	}
	os, _ := body["object_store"].(map[string]any)
	if os["status"] != "error" || os["bucket"] != "missing-bucket" {
		t.Fatalf("object_store=%#v", os)
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

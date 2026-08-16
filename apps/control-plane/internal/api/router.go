package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/superteam/control-plane/internal/employee"
)

type healthResponse struct {
	Status           string                            `json:"status"`
	Service          string                            `json:"service"`
	ProviderContract *employee.ProviderContractMetrics `json:"provider_contract,omitempty"`
	ObjectStore      *objectStoreHealth                `json:"object_store,omitempty"`
}

type objectStoreHealth struct {
	Status string `json:"status"`
	Bucket string `json:"bucket,omitempty"`
	Error  string `json:"error,omitempty"`
}

func writeHealthResponse(w http.ResponseWriter) {
	writeHealthResponseWithProbe(w, nil, "", false)
}

func writeHealthResponseWithProbe(w http.ResponseWriter, probe func(context.Context) error, bucket string, deep bool) {
	metrics := employee.SnapshotProviderContractMetrics()
	body := healthResponse{
		Status:           "ok",
		Service:          "control-plane",
		ProviderContract: &metrics,
	}
	code := http.StatusOK
	if probe != nil {
		osHealth := &objectStoreHealth{Status: "ok", Bucket: bucket}
		if err := probe(context.Background()); err != nil {
			osHealth.Status = "error"
			osHealth.Error = err.Error()
			body.Status = "degraded"
			code = http.StatusServiceUnavailable
		}
		body.ObjectStore = osHealth
	}
	_ = deep
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func NewHealthOnlyRouter() http.Handler {
	router := chi.NewRouter()

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeHealthResponse(w)
	})

	return router
}

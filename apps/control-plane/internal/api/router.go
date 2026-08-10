package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/superteam/control-plane/internal/employee"
)

type healthResponse struct {
	Status           string                          `json:"status"`
	Service          string                          `json:"service"`
	ProviderContract *employee.ProviderContractMetrics `json:"provider_contract,omitempty"`
}

func writeHealthResponse(w http.ResponseWriter) {
	metrics := employee.SnapshotProviderContractMetrics()
	w.Header().Set("content-type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(healthResponse{
		Status:           "ok",
		Service:          "control-plane",
		ProviderContract: &metrics,
	})
}

func NewHealthOnlyRouter() http.Handler {
	router := chi.NewRouter()

	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeHealthResponse(w)
	})

	return router
}

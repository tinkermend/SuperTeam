package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/superteam/control-plane/internal/runtime"
)

func (h *RuntimeHandler) ListProviderNativeConfigs(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := h.authorizeRuntimeEnrollmentManagement(w, r, "runtime provider native config list")
	if !ok {
		return
	}
	nodeID := strings.TrimSpace(chi.URLParam(r, "nodeId"))
	if nodeID == "" {
		http.Error(w, "nodeId is required", http.StatusBadRequest)
		return
	}
	items, err := h.runtimeService.ListProviderNativeConfigs(r.Context(), tenantID, nodeID)
	if err != nil {
		writeProviderNativeConfigError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(items)
}

func (h *RuntimeHandler) PullProviderNativeConfig(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorizeRuntimeEnrollmentManagement(w, r, "runtime provider native config pull")
	if !ok {
		return
	}
	nodeID := strings.TrimSpace(chi.URLParam(r, "nodeId"))
	if nodeID == "" {
		http.Error(w, "nodeId is required", http.StatusBadRequest)
		return
	}
	var req struct {
		ProviderType string `json:"provider_type"`
		ConfigKey    string `json:"config_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	detail, err := h.runtimeService.PullProviderNativeConfig(
		r.Context(), tenantID, userID, nodeID, strings.TrimSpace(req.ProviderType), strings.TrimSpace(req.ConfigKey),
	)
	if err != nil {
		writeProviderNativeConfigError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(detail)
}

func (h *RuntimeHandler) GetProviderNativeConfig(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := h.authorizeRuntimeEnrollmentManagement(w, r, "runtime provider native config get")
	if !ok {
		return
	}
	nodeID := strings.TrimSpace(chi.URLParam(r, "nodeId"))
	providerType := strings.TrimSpace(chi.URLParam(r, "providerType"))
	configKey := strings.TrimSpace(chi.URLParam(r, "configKey"))
	if nodeID == "" || providerType == "" || configKey == "" {
		http.Error(w, "nodeId, providerType and configKey are required", http.StatusBadRequest)
		return
	}
	detail, err := h.runtimeService.GetProviderNativeConfigSnapshot(
		r.Context(), tenantID, nodeID, providerType, configKey,
	)
	if err != nil {
		writeProviderNativeConfigError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(detail)
}

func (h *RuntimeHandler) PushProviderNativeConfig(w http.ResponseWriter, r *http.Request) {
	tenantID, userID, ok := h.authorizeRuntimeEnrollmentManagement(w, r, "runtime provider native config push")
	if !ok {
		return
	}
	nodeID := strings.TrimSpace(chi.URLParam(r, "nodeId"))
	providerType := strings.TrimSpace(chi.URLParam(r, "providerType"))
	configKey := strings.TrimSpace(chi.URLParam(r, "configKey"))
	if nodeID == "" || providerType == "" || configKey == "" {
		http.Error(w, "nodeId, providerType and configKey are required", http.StatusBadRequest)
		return
	}
	var req struct {
		Values                   map[string]any `json:"values"`
		ExpectedFileContentHash  string         `json:"expected_file_content_hash"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	detail, err := h.runtimeService.PushProviderNativeConfig(
		r.Context(),
		tenantID,
		userID,
		nodeID,
		providerType,
		configKey,
		req.Values,
		strings.TrimSpace(req.ExpectedFileContentHash),
	)
	if err != nil {
		writeProviderNativeConfigError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(detail)
}

func writeProviderNativeConfigError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, runtime.ErrNodeNotFound), errors.Is(err, runtime.ErrProviderNativeConfigNotFound):
		http.Error(w, err.Error(), http.StatusNotFound)
	case errors.Is(err, runtime.ErrProviderNativeConfigConflict),
		errors.Is(err, runtime.ErrProviderNativeConfigUnmanageable),
		errors.Is(err, runtime.ErrProviderNativeConfigOffline):
		http.Error(w, err.Error(), http.StatusConflict)
	case errors.Is(err, runtime.ErrProviderNativeConfigValidation):
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

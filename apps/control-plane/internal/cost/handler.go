package cost

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
)

type HandlerService interface {
	GetTenantCostSummary(ctx context.Context, tenantID uuid.UUID, period string) (TenantCostSummary, error)
	GetEmployeeCostList(ctx context.Context, tenantID uuid.UUID, period string) (EmployeeCostList, error)
}

type HTTPHandler struct {
	service    HandlerService
	authorizer authz.Authorizer
}

func NewHTTPHandler(service HandlerService) *HTTPHandler {
	return &HTTPHandler{service: service}
}

func (h *HTTPHandler) SetAuthorizer(a authz.Authorizer) {
	h.authorizer = a
}

// GET /api/v1/costs/summary?period=7d|30d|90d
func (h *HTTPHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := consoleIdentity(w, r)
	if !ok {
		return
	}
	period := r.URL.Query().Get("period")
	summary, err := h.service.GetTenantCostSummary(r.Context(), tenantID, period)
	if err != nil {
		if errors.Is(err, ErrInvalidPeriod) {
			writeJSONError(w, http.StatusBadRequest, "period must be one of 7d, 30d, 90d")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, costSummaryResponse(summary))
}

// GET /api/v1/costs/employees?period=7d|30d|90d
func (h *HTTPHandler) ListEmployees(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := consoleIdentity(w, r)
	if !ok {
		return
	}
	period := r.URL.Query().Get("period")
	list, err := h.service.GetEmployeeCostList(r.Context(), tenantID, period)
	if err != nil {
		if errors.Is(err, ErrInvalidPeriod) {
			writeJSONError(w, http.StatusBadRequest, "period must be one of 7d, 30d, 90d")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	writeJSON(w, http.StatusOK, employeeCostListResponse(list))
}

// --- response shapes ---

type employeeSummaryItem struct {
	EmployeeID   string `json:"employee_id"`
	EmployeeName string `json:"employee_name"`
	ProviderType string `json:"provider_type"`
	RunCount     int32  `json:"run_count"`
	TotalTokens  int64  `json:"total_tokens"`
}

type dailyTrendItem struct {
	Day          string `json:"day"`
	ProviderType string `json:"provider_type"`
	RunCount     int32  `json:"run_count"`
	TotalTokens  int64  `json:"total_tokens"`
}

type costSummaryResp struct {
	PeriodStart string               `json:"period_start"`
	PeriodEnd   string               `json:"period_end"`
	TotalTokens int64                `json:"total_tokens"`
	TotalRuns   int32                `json:"total_runs"`
	ByEmployee  []employeeSummaryItem `json:"by_employee"`
	ByProvider  map[string]int64     `json:"by_provider"`
	DailyTrend  []dailyTrendItem     `json:"daily_trend"`
}

type employeeCostListResp struct {
	PeriodStart string               `json:"period_start"`
	PeriodEnd   string               `json:"period_end"`
	Items       []employeeSummaryItem `json:"items"`
}

func costSummaryResponse(s TenantCostSummary) costSummaryResp {
	employees := make([]employeeSummaryItem, len(s.ByEmployee))
	for i, e := range s.ByEmployee {
		employees[i] = toEmployeeItem(e)
	}
	trend := make([]dailyTrendItem, len(s.DailyTrend))
	for i, d := range s.DailyTrend {
		trend[i] = dailyTrendItem{
			Day:          d.Day.Format(time.DateOnly),
			ProviderType: d.ProviderType,
			RunCount:     d.RunCount,
			TotalTokens:  d.TotalTokens,
		}
	}
	byProvider := s.ByProvider
	if byProvider == nil {
		byProvider = map[string]int64{}
	}
	return costSummaryResp{
		PeriodStart: s.PeriodStart.Format(time.RFC3339),
		PeriodEnd:   s.PeriodEnd.Format(time.RFC3339),
		TotalTokens: s.TotalTokens,
		TotalRuns:   s.TotalRuns,
		ByEmployee:  employees,
		ByProvider:  byProvider,
		DailyTrend:  trend,
	}
}

func employeeCostListResponse(l EmployeeCostList) employeeCostListResp {
	items := make([]employeeSummaryItem, len(l.Items))
	for i, e := range l.Items {
		items[i] = toEmployeeItem(e)
	}
	return employeeCostListResp{
		PeriodStart: l.PeriodStart.Format(time.RFC3339),
		PeriodEnd:   l.PeriodEnd.Format(time.RFC3339),
		Items:       items,
	}
}

func toEmployeeItem(e EmployeeSummaryRow) employeeSummaryItem {
	return employeeSummaryItem{
		EmployeeID:   e.DigitalEmployeeID.String(),
		EmployeeName: e.EmployeeName,
		ProviderType: e.ProviderType,
		RunCount:     e.RunCount,
		TotalTokens:  e.TotalTokens,
	}
}

// --- HTTP helpers (same pattern as inbox/handler.go) ---

func consoleIdentity(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		writeJSONError(w, http.StatusForbidden, "console identity not found in context")
		return uuid.Nil, uuid.Nil, false
	}
	return tenantID, userID, true
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

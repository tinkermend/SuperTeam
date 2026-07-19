package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
)

type HandlerService interface {
	ListItems(ctx context.Context, req ListItemsRequest) (ListItemsResult, error)
	GetBadge(ctx context.Context, tenantID, actorUserID uuid.UUID, includeTeam bool) (Badge, error)
	ExecuteAction(ctx context.Context, req ExecuteActionRequest) (Item, SourceActionResult, error)
	PeekChanges(ctx context.Context, req PeekChangeRequest) (*ChangeCursor, error)
}

type HTTPHandler struct {
	service    HandlerService
	authorizer authz.Authorizer
	// streamInterval 覆盖 SSE 探测间隔,零值用 defaultStreamInterval;测试注入用。
	streamInterval time.Duration
}

const defaultStreamInterval = 2 * time.Second
const streamKeepaliveEvery = 8 // 每 N 个空探测发一次注释行保活(2s 间隔 ≈ 16s 一次)
// 流启动游标回拨量:updated_at 由 DB NOW() 生成,与应用时钟可能有偏差。宁可多推一次
// 脏通知(客户端只是多一次 refetch),不漏掉建流瞬间落库的变更。
const streamCursorSlack = 5 * time.Second

func NewHandler(service HandlerService) *HTTPHandler {
	return &HTTPHandler{service: service}
}

func (h *HTTPHandler) SetAuthorizer(authorizer authz.Authorizer) {
	h.authorizer = authorizer
}

func (h *HTTPHandler) ListItems(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, ok := consoleIdentity(w, r)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	req, ok := listItemsRequestFromQuery(w, r, tenantID, actorID)
	if !ok {
		return
	}
	if req.View == ViewTeam {
		allowed, err := h.canReadTeamInbox(r.Context(), tenantID, actorID, "inbox team view read")
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "internal server error")
			return
		}
		if !allowed {
			writeJSONError(w, http.StatusForbidden, "forbidden")
			return
		}
		req.TeamViewAllowed = true
	}
	result, err := service.ListItems(r.Context(), req)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, listResponseFromDomain(result))
}

func (h *HTTPHandler) GetBadge(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, ok := consoleIdentity(w, r)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	includeTeam, err := h.canReadTeamInbox(r.Context(), tenantID, actorID, "inbox badge team count read")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	badge, err := service.GetBadge(r.Context(), tenantID, actorID, includeTeam)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, badgeResponse{
		MineOpenCount: badge.MineOpenCount,
		TeamOpenCount: badge.TeamOpenCount,
		HighRiskCount: badge.HighRiskCount,
	})
}

// StreamItems 以 SSE 推送收件箱脏通知:服务端按短间隔用 (updated_at, id) 游标探测 actor
// 可见范围内是否有变更,有则写一帧 event: inbox-changed(客户端收到后走既有 ListItems 重拉,
// 授权与筛选不在流内复刻);空探测期间定期发注释行保活。团队可见范围在建流时判权一次,
// 存续期内不重判(断线重连后生效,与页面打开期间不重判权限的现状一致)。
func (h *HTTPHandler) StreamItems(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, ok := consoleIdentity(w, r)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	teamViewAllowed, err := h.canReadTeamInbox(r.Context(), tenantID, actorID, "inbox change stream read")
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	flusher, canFlush := w.(http.Flusher)
	if !canFlush {
		writeJSONError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	interval := h.streamInterval
	if interval <= 0 {
		interval = defaultStreamInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	cursorAt := time.Now().UTC().Add(-streamCursorSlack)
	cursorID := uuid.Nil
	idleTicks := 0
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
		change, err := service.PeekChanges(r.Context(), PeekChangeRequest{
			TenantID:        tenantID,
			ActorUserID:     actorID,
			TeamViewAllowed: teamViewAllowed,
			CursorUpdatedAt: cursorAt,
			CursorID:        cursorID,
		})
		if err != nil {
			// 探测失败(含连接被取消)即结束流,客户端 EventSource 会自动重连。
			return
		}
		if change == nil {
			idleTicks++
			if idleTicks >= streamKeepaliveEvery {
				idleTicks = 0
				if _, writeErr := w.Write([]byte(": keepalive\n\n")); writeErr != nil {
					return
				}
				flusher.Flush()
			}
			continue
		}
		idleTicks = 0
		cursorAt, cursorID = change.UpdatedAt, change.ID
		payload, marshalErr := json.Marshal(map[string]string{
			"updated_at": change.UpdatedAt.UTC().Format(time.RFC3339Nano),
		})
		if marshalErr != nil {
			continue
		}
		if _, writeErr := w.Write([]byte("event: inbox-changed\ndata: " + string(payload) + "\n\n")); writeErr != nil {
			return
		}
		flusher.Flush()
	}
}

func (h *HTTPHandler) ExecuteAction(w http.ResponseWriter, r *http.Request) {
	tenantID, actorID, ok := consoleIdentity(w, r)
	if !ok {
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	itemID, ok := itemIDFromRequest(w, r)
	if !ok {
		return
	}
	var body executeActionBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	item, result, err := service.ExecuteAction(r.Context(), ExecuteActionRequest{
		TenantID:    tenantID,
		ActorUserID: actorID,
		ItemID:      itemID,
		Action:      body.Action,
		Comment:     body.Comment,
		Payload:     body.Payload,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, executeActionResponse{
		Item: itemResponseFromDomain(item),
		SourceResult: sourceActionResultResponse{
			SourceType: result.SourceType,
			SourceID:   result.SourceID.String(),
			Status:     result.Status,
		},
	})
}

func (h *HTTPHandler) serviceFromRequest(w http.ResponseWriter) (HandlerService, bool) {
	if h == nil || h.service == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "inbox service is not configured")
		return nil, false
	}
	return h.service, true
}

func (h *HTTPHandler) canReadTeamInbox(ctx context.Context, tenantID, actorID uuid.UUID, auditReason string) (bool, error) {
	if h == nil || h.authorizer == nil {
		return false, nil
	}
	decision, err := h.authorizer.Check(ctx, authz.CheckRequest{
		Actor: authz.ActorRef{
			Type: authz.ActorUser,
			ID:   actorID.String(),
		},
		Action: authz.ActionTeamRead,
		Resource: authz.ResourceRef{
			Type: authz.ResourceTenant,
			ID:   tenantID.String(),
		},
		TenantID:    tenantID,
		AuditReason: auditReason,
	})
	if err != nil {
		return false, err
	}
	return decision.Allowed, nil
}

func listItemsRequestFromQuery(w http.ResponseWriter, r *http.Request, tenantID, actorID uuid.UUID) (ListItemsRequest, bool) {
	query := r.URL.Query()
	req := ListItemsRequest{
		TenantID:    tenantID,
		ActorUserID: actorID,
		View:        View(query.Get("view")),
	}
	// Team view stays disabled until ListItems authorizes the tenant-level team read.
	req.TeamViewAllowed = false
	if raw := query.Get("status"); raw != "" && raw != "all" {
		status := Status(raw)
		req.Status = &status
	}
	if raw := query.Get("item_type"); raw != "" {
		itemType := ItemType(raw)
		req.ItemType = &itemType
	}
	if raw := query.Get("risk_level"); raw != "" {
		req.RiskLevel = &raw
	}
	if raw := query.Get("project_id"); raw != "" {
		projectID, err := uuid.Parse(raw)
		if err != nil || projectID == uuid.Nil {
			writeJSONError(w, http.StatusBadRequest, "invalid project_id")
			return ListItemsRequest{}, false
		}
		req.ProjectID = &projectID
	}
	if raw := query.Get("target_user_id"); raw != "" {
		targetUserID, err := uuid.Parse(raw)
		if err != nil || targetUserID == uuid.Nil {
			writeJSONError(w, http.StatusBadRequest, "invalid target_user_id")
			return ListItemsRequest{}, false
		}
		req.TargetUserID = &targetUserID
	}
	limit, ok := int32QueryParam(w, r, "limit")
	if !ok {
		return ListItemsRequest{}, false
	}
	offset, ok := int32QueryParam(w, r, "offset")
	if !ok {
		return ListItemsRequest{}, false
	}
	req.Limit = limit
	req.Offset = offset
	return req, true
}

func consoleIdentity(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		writeJSONError(w, http.StatusForbidden, "console identity not found in context")
		return uuid.Nil, uuid.Nil, false
	}
	return tenantID, userID, true
}

func itemIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	itemID, err := uuid.Parse(chi.URLParam(r, "itemId"))
	if err != nil || itemID == uuid.Nil {
		writeJSONError(w, http.StatusBadRequest, "invalid inbox item id")
		return uuid.Nil, false
	}
	return itemID, true
}

func int32QueryParam(w http.ResponseWriter, r *http.Request, name string) (int32, bool) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, true
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, name+" must be an integer")
		return 0, false
	}
	return int32(value), true
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}

func writeHandlerError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidItem), errors.Is(err, ErrInvalidAction):
		writeJSONError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrViewForbidden), errors.Is(err, ErrActionForbidden):
		writeJSONError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, ErrItemNotFound):
		writeJSONError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrSourceUnavailable):
		writeJSONError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, ErrProjectionNotApplied):
		writeJSONError(w, http.StatusInternalServerError, err.Error())
	default:
		writeJSONError(w, http.StatusInternalServerError, "internal server error")
	}
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

type listResponse struct {
	Items      []itemResponse      `json:"items"`
	Pagination paginationResponse  `json:"pagination"`
	Summary    listSummaryResponse `json:"summary"`
}

type paginationResponse struct {
	Limit   int32 `json:"limit"`
	Offset  int32 `json:"offset"`
	HasMore bool  `json:"has_more"`
}

type listSummaryResponse struct {
	OpenCount     int64 `json:"open_count"`
	HighRiskCount int64 `json:"high_risk_count"`
	BlockedCount  int64 `json:"blocked_count"`
}

type badgeResponse struct {
	MineOpenCount int64 `json:"mine_open_count"`
	TeamOpenCount int64 `json:"team_open_count"`
	HighRiskCount int64 `json:"high_risk_count"`
}

type executeActionBody struct {
	Action  string         `json:"action"`
	Comment string         `json:"comment"`
	Payload map[string]any `json:"payload"`
}

type executeActionResponse struct {
	Item         itemResponse               `json:"item"`
	SourceResult sourceActionResultResponse `json:"source_result"`
}

type sourceActionResultResponse struct {
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
	Status     string `json:"status"`
}

type itemResponse struct {
	ID                      string         `json:"id"`
	TenantID                string         `json:"tenant_id"`
	TeamID                  *string        `json:"team_id,omitempty"`
	TargetUserID            string         `json:"target_user_id"`
	ItemType                ItemType       `json:"item_type"`
	SourceType              SourceType     `json:"source_type"`
	SourceID                string         `json:"source_id"`
	SourceProjectID         *string        `json:"source_project_id,omitempty"`
	SourceTaskID            *string        `json:"source_task_id,omitempty"`
	SourceApprovalRequestID *string        `json:"source_approval_request_id,omitempty"`
	SourceProjectName       *string        `json:"source_project_name,omitempty"`
	SourceTaskName          *string        `json:"source_task_name,omitempty"`
	Title                   string         `json:"title"`
	Summary                 *string        `json:"summary,omitempty"`
	RiskLevel               *string        `json:"risk_level,omitempty"`
	Priority                *string        `json:"priority,omitempty"`
	Status                  Status         `json:"status"`
	Actions                 []Action       `json:"actions"`
	Context                 map[string]any `json:"context"`
	DeepLink                map[string]any `json:"deep_link"`
	LastActivityAt          string         `json:"last_activity_at"`
	CreatedAt               string         `json:"created_at"`
	UpdatedAt               string         `json:"updated_at"`
	ResolvedAt              *string        `json:"resolved_at,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func listResponseFromDomain(result ListItemsResult) listResponse {
	items := make([]itemResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, itemResponseFromDomain(item))
	}
	return listResponse{
		Items:      items,
		Pagination: paginationResponse{Limit: result.Limit, Offset: result.Offset, HasMore: result.HasMore},
		Summary:    listSummaryResponse{OpenCount: result.OpenCount, HighRiskCount: result.HighRiskCount},
	}
}

func itemResponseFromDomain(item Item) itemResponse {
	return itemResponse{
		ID:                      item.ID.String(),
		TenantID:                item.TenantID.String(),
		TeamID:                  optionalUUIDString(item.TeamID),
		TargetUserID:            item.TargetUserID.String(),
		ItemType:                item.ItemType,
		SourceType:              item.SourceType,
		SourceID:                item.SourceID.String(),
		SourceProjectID:         optionalUUIDString(item.SourceProjectID),
		SourceTaskID:            optionalUUIDString(item.SourceTaskID),
		SourceApprovalRequestID: optionalUUIDString(item.SourceApprovalRequestID),
		SourceProjectName:       item.SourceProjectName,
		SourceTaskName:          item.SourceTaskName,
		Title:                   item.Title,
		Summary:                 item.Summary,
		RiskLevel:               item.RiskLevel,
		Priority:                item.Priority,
		Status:                  item.Status,
		Actions:                 actionsOrEmpty(item.Actions),
		Context:                 mapOrEmpty(item.ContextPayload),
		DeepLink:                mapOrEmpty(item.DeepLink),
		LastActivityAt:          timeValue(item.LastActivityAt),
		CreatedAt:               timeValue(item.CreatedAt),
		UpdatedAt:               timeValue(item.UpdatedAt),
		ResolvedAt:              optionalTimeString(item.ResolvedAt),
	}
}

func actionsOrEmpty(actions []Action) []Action {
	if actions == nil {
		return []Action{}
	}
	return append([]Action(nil), actions...)
}

func optionalUUIDString(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}

func optionalTimeString(value *time.Time) *string {
	if value == nil {
		return nil
	}
	text := timeValue(*value)
	return &text
}

func timeValue(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

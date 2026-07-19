package project

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/api/middleware"
	"github.com/superteam/control-plane/internal/authz"
)

// artifactStorageService 是 presign/content 端点对 service 的窄依赖面;
// 用断言而非扩 HandlerService,避免波及全部测试替身。
type artifactStorageService interface {
	PresignRuntimeArtifactUpload(ctx context.Context, req PresignRuntimeArtifactRequest) (PresignRuntimeArtifactResult, error)
	PresignRuntimeRawLogUpload(ctx context.Context, req PresignRuntimeRawLogRequest) (PresignRuntimeRawLogResult, error)
	GetArtifactRef(ctx context.Context, tenantID, artifactRefID uuid.UUID) (ProjectArtifactRef, error)
	PresignArtifactContent(ctx context.Context, ref ProjectArtifactRef) (string, error)
	// 下载 presign TTL 经系统配置中心可配,format=json 响应的 expires_at 与之对齐。
	artifactContentGetTTL(ctx context.Context, tenantID uuid.UUID) time.Duration
}

func (h *HTTPHandler) artifactStorageServiceFromRequest(w http.ResponseWriter) (artifactStorageService, bool) {
	service, ok := h.service.(artifactStorageService)
	if !ok {
		http.Error(w, "artifact storage is not configured", http.StatusNotImplemented)
		return nil, false
	}
	return service, true
}

func runtimeTenantFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		http.Error(w, "tenant_id not found in context", http.StatusUnauthorized)
		return uuid.Nil, false
	}
	return tenantID, true
}

type presignRuntimeArtifactBody struct {
	Sha256      string `json:"sha256"`
	SizeBytes   int64  `json:"size_bytes"`
	ContentType string `json:"content_type"`
}

type presignUploadResponse struct {
	ObjectKey     string `json:"object_key"`
	UploadURL     string `json:"upload_url,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`
	AlreadyExists bool   `json:"already_exists"`
}

// PresignRuntimeArtifact POST /api/v1/runtime/artifacts/presign(RuntimeSessionAuth)。
// tenant 取自 runtime 会话身份,请求体不含也不信任租户字段(spec §4.2)。
func (h *HTTPHandler) PresignRuntimeArtifact(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := runtimeTenantFromRequest(w, r)
	if !ok {
		return
	}
	service, ok := h.artifactStorageServiceFromRequest(w)
	if !ok {
		return
	}
	var body presignRuntimeArtifactBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	result, err := service.PresignRuntimeArtifactUpload(r.Context(), PresignRuntimeArtifactRequest{
		TenantID:    tenantID,
		Sha256:      body.Sha256,
		SizeBytes:   body.SizeBytes,
		ContentType: body.ContentType,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, presignUploadResponse{
		ObjectKey:     result.ObjectKey,
		UploadURL:     result.UploadURL,
		ExpiresAt:     formatPresignExpiry(result.ExpiresAt),
		AlreadyExists: result.AlreadyExists,
	})
}

type presignRuntimeRawLogBody struct {
	AttemptID uuid.UUID `json:"attempt_id"`
	Object    string    `json:"object"`
	PartIndex *int32    `json:"part_index"`
	SizeBytes int64     `json:"size_bytes"`
}

// PresignRuntimeRawLog POST /api/v1/runtime/raw-logs/presign(RuntimeSessionAuth)。
func (h *HTTPHandler) PresignRuntimeRawLog(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := runtimeTenantFromRequest(w, r)
	if !ok {
		return
	}
	service, ok := h.artifactStorageServiceFromRequest(w)
	if !ok {
		return
	}
	var body presignRuntimeRawLogBody
	if !decodeJSONBody(w, r, &body) {
		return
	}
	result, err := service.PresignRuntimeRawLogUpload(r.Context(), PresignRuntimeRawLogRequest{
		TenantID:  tenantID,
		AttemptID: body.AttemptID,
		Object:    body.Object,
		PartIndex: body.PartIndex,
		SizeBytes: body.SizeBytes,
	})
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, presignUploadResponse{
		ObjectKey: result.ObjectKey,
		UploadURL: result.UploadURL,
		ExpiresAt: formatPresignExpiry(result.ExpiresAt),
	})
}

// GetArtifactContent GET /api/v1/artifacts/{artifactRefId}/content(console auth)。
// 校验调用方对 artifact 所属项目的读权限后 302 到 presigned GET,
// 字节不经控制平面(spec §4.3)。
func (h *HTTPHandler) GetArtifactContent(w http.ResponseWriter, r *http.Request) {
	if h.authorizer == nil {
		http.Error(w, "project authorization is not configured", http.StatusForbidden)
		return
	}
	tenantID := middleware.GetTenantID(r.Context())
	userID := middleware.GetUserID(r.Context())
	if tenantID == uuid.Nil || userID == uuid.Nil {
		http.Error(w, "console identity not found in context", http.StatusForbidden)
		return
	}
	artifactRefID, err := uuid.Parse(chi.URLParam(r, "artifactRefId"))
	if err != nil {
		http.Error(w, "artifactRefId must be a valid uuid", http.StatusBadRequest)
		return
	}
	service, ok := h.artifactStorageServiceFromRequest(w)
	if !ok {
		return
	}

	ref, err := service.GetArtifactRef(r.Context(), tenantID, artifactRefID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	decision, err := h.authorizer.Check(r.Context(), authz.CheckRequest{
		Actor:    authz.ActorRef{Type: authz.ActorUser, ID: userID.String()},
		Action:   authz.ActionProjectRead,
		Resource: authz.ResourceRef{Type: authz.ResourceProject, ID: ref.ProjectID.String()},
		TenantID: tenantID,
	})
	if err != nil {
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !decision.Allowed {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	url, err := service.PresignArtifactContent(r.Context(), ref)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	// format=json:返回 presigned URL 本体供浏览器两步取回。302 的 fetch
	// 跨域重定向会把 Origin 置为 null(redirect taint),迫使对象存储 CORS
	// 放行 null origin;两步取回下第二跳请求 Origin 干净,桶只需常规放行。
	if r.URL.Query().Get("format") == "json" {
		writeJSON(w, http.StatusOK, artifactContentLocationResponse{
			URL:       url,
			ExpiresAt: time.Now().Add(service.artifactContentGetTTL(r.Context(), ref.TenantID)).UTC().Format(time.RFC3339),
		})
		return
	}
	http.Redirect(w, r, url, http.StatusFound)
}

type artifactContentLocationResponse struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

func formatPresignExpiry(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

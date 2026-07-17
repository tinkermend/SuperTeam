package skill

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/superteam/control-plane/internal/api/middleware"
)

// skillArchivePresigner 是 presign 端点对 service 的窄依赖面;用断言而非扩
// HandlerService,避免波及测试替身。
type skillArchivePresigner interface {
	PresignArchiveDownload(ctx context.Context, tenantID uuid.UUID, archiveObjectRef string) (string, time.Time, error)
}

type presignSkillArchiveBody struct {
	ArchiveObjectRef string `json:"archive_object_ref"`
}

type presignSkillArchiveResponse struct {
	DownloadURL string `json:"download_url"`
	ExpiresAt   string `json:"expires_at"`
}

// PresignRuntimeSkillArchive POST /api/v1/runtime/skills/presign
// (RuntimeSessionAuth)。tenant 取自 runtime 会话身份;service 侧强制
// skills/{tenant}/ 前缀,跨租户引用一律 400。
func (h *HTTPHandler) PresignRuntimeSkillArchive(w http.ResponseWriter, r *http.Request) {
	tenantID := middleware.GetTenantID(r.Context())
	if tenantID == uuid.Nil {
		http.Error(w, "tenant_id not found in context", http.StatusUnauthorized)
		return
	}
	presigner, ok := h.service.(skillArchivePresigner)
	if !ok {
		http.Error(w, "skill archive presign is not configured", http.StatusNotImplemented)
		return
	}
	var body presignSkillArchiveBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	url, expiresAt, err := presigner.PresignArchiveDownload(r.Context(), tenantID, strings.TrimSpace(body.ArchiveObjectRef))
	if err != nil {
		if errors.Is(err, ErrInvalidInput) {
			http.Error(w, err.Error(), http.StatusBadRequest)
		} else {
			http.Error(w, "internal server error", http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusOK, presignSkillArchiveResponse{
		DownloadURL: url,
		ExpiresAt:   expiresAt.UTC().Format(time.RFC3339),
	})
}

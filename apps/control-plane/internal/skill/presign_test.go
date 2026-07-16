package skill

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestPresignArchiveDownloadEnforcesTenantSkillsPrefix(t *testing.T) {
	repo := &serviceTestRepository{}
	service := newTestService(repo)
	tenantID := uuid.New()

	// s3:// URI 形态(ArchiveObjectRef 的存量存储形态)按 key 提取后签发。
	uri := fmt.Sprintf("s3://superteam/skills/%s/diagnose/abc.zip", tenantID)
	url, expiresAt, err := service.PresignArchiveDownload(context.Background(), tenantID, uri)
	if err != nil {
		t.Fatalf("presign tenant archive: %v", err)
	}
	if !strings.Contains(url, fmt.Sprintf("skills/%s/diagnose/abc.zip", tenantID)) {
		t.Fatalf("presigned url must target the extracted key, got %s", url)
	}
	if expiresAt.IsZero() {
		t.Fatalf("expected expiry timestamp")
	}

	// 纯 key 形态同样接受。
	if _, _, err := service.PresignArchiveDownload(context.Background(), tenantID, fmt.Sprintf("skills/%s/x/y.zip", tenantID)); err != nil {
		t.Fatalf("presign bare key: %v", err)
	}

	// 他租户前缀 / 非 skills 前缀一律 400。
	rejected := []string{
		fmt.Sprintf("s3://superteam/skills/%s/other/pkg.zip", uuid.New()),
		fmt.Sprintf("runs/%s/attempt/raw.part-0001.jsonl", tenantID),
		"",
	}
	for _, ref := range rejected {
		if _, _, err := service.PresignArchiveDownload(context.Background(), tenantID, ref); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("expected ErrInvalidInput for %q, got %v", ref, err)
		}
	}
}

package project

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// 证据地基(spec 2026-07-09 §3-4):对象字节走 runtime ↔ 对象存储数据面,
// 控制面只负责授权(presign)、元数据与读模型。runtime 不持有对象存储凭证。

// ArtifactObjectStore 是控制平面对象存储的最小依赖面:存在性探测与 URL 签发。
// 用原始返回值而非 storage 包类型,避免 project 层耦合基础设施包。
type ArtifactObjectStore interface {
	StatObject(ctx context.Context, key string) (exists bool, sizeBytes int64, err error)
	PresignPut(ctx context.Context, key, contentType string, ttl time.Duration) (string, error)
	PresignGet(ctx context.Context, key string, ttl time.Duration) (string, error)
}

const (
	// ArtifactMaxFileSizeBytes 单文件上限,与 runtime 采集侧限额一致(spec §4.1)。
	ArtifactMaxFileSizeBytes = int64(10 * 1024 * 1024)
	// rawLogMaxPartSizeBytes raw 分段按 8MiB 轮转,留少量余量。
	rawLogMaxPartSizeBytes = int64(9 * 1024 * 1024)

	artifactPresignTTL       = 15 * time.Minute
	artifactContentGetTTL    = 5 * time.Minute
	artifactObjectKeyPrefix  = "artifacts/"
	rawLogObjectKeyPrefix    = "runs/"
	rawLogPresignObjectPart  = "part"
	rawLogPresignObjectindex = "manifest"
)

var sha256HexPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// SetArtifactObjectStore 注入对象存储;未注入时 presign/content 端点返回配置错误。
func (s *Service) SetArtifactObjectStore(store ArtifactObjectStore) {
	s.artifactObjectStore = store
}

type PresignRuntimeArtifactRequest struct {
	TenantID    uuid.UUID
	Sha256      string
	SizeBytes   int64
	ContentType string
}

type PresignRuntimeArtifactResult struct {
	ObjectKey     string
	UploadURL     string
	ExpiresAt     time.Time
	AlreadyExists bool
}

// PresignRuntimeArtifactUpload 为内容寻址 artifact 签发直传 URL。
// key = artifacts/{tenant}/sha256/{hex};tenant 取自 runtime 身份,不信任请求体。
// 对象已存在时返回 already_exists,跳过上传(内容寻址天然幂等)。
func (s *Service) PresignRuntimeArtifactUpload(ctx context.Context, req PresignRuntimeArtifactRequest) (PresignRuntimeArtifactResult, error) {
	if s.artifactObjectStore == nil {
		return PresignRuntimeArtifactResult{}, fmt.Errorf("artifact object store is not configured")
	}
	if req.TenantID == uuid.Nil {
		return PresignRuntimeArtifactResult{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidProject)
	}
	sha := strings.ToLower(strings.TrimSpace(req.Sha256))
	if !sha256HexPattern.MatchString(sha) {
		return PresignRuntimeArtifactResult{}, fmt.Errorf("%w: sha256 must be 64 lowercase hex chars", ErrInvalidProject)
	}
	if req.SizeBytes <= 0 || req.SizeBytes > ArtifactMaxFileSizeBytes {
		return PresignRuntimeArtifactResult{}, fmt.Errorf("%w: size_bytes must be within (0, %d]", ErrInvalidProject, ArtifactMaxFileSizeBytes)
	}

	key := fmt.Sprintf("%s%s/sha256/%s", artifactObjectKeyPrefix, req.TenantID, sha)
	exists, _, err := s.artifactObjectStore.StatObject(ctx, key)
	if err != nil {
		return PresignRuntimeArtifactResult{}, fmt.Errorf("stat artifact object: %w", err)
	}
	if exists {
		return PresignRuntimeArtifactResult{ObjectKey: key, AlreadyExists: true}, nil
	}

	url, err := s.artifactObjectStore.PresignPut(ctx, key, strings.TrimSpace(req.ContentType), artifactPresignTTL)
	if err != nil {
		return PresignRuntimeArtifactResult{}, fmt.Errorf("presign artifact put: %w", err)
	}
	return PresignRuntimeArtifactResult{
		ObjectKey: key,
		UploadURL: url,
		ExpiresAt: time.Now().Add(artifactPresignTTL),
	}, nil
}

type PresignRuntimeRawLogRequest struct {
	TenantID  uuid.UUID
	AttemptID uuid.UUID
	// Object: "part"(需 PartIndex)或 "manifest"。
	Object    string
	PartIndex *int32
	SizeBytes int64
}

type PresignRuntimeRawLogResult struct {
	ObjectKey string
	UploadURL string
	ExpiresAt time.Time
}

// PresignRuntimeRawLogUpload 为 raw transcript 分段/清单签发直传 URL,
// 接替 runtime 直连凭证上传(spec §8 修订 1)。key 服务端派生:
// runs/{tenant}/{attempt}/raw.part-NNNN.jsonl | manifest.json。
// attempt 必须存在且属于调用方租户——这是跨租户写入的唯一闸门。
func (s *Service) PresignRuntimeRawLogUpload(ctx context.Context, req PresignRuntimeRawLogRequest) (PresignRuntimeRawLogResult, error) {
	if s.artifactObjectStore == nil {
		return PresignRuntimeRawLogResult{}, fmt.Errorf("artifact object store is not configured")
	}
	if req.TenantID == uuid.Nil {
		return PresignRuntimeRawLogResult{}, fmt.Errorf("%w: tenant_id is required", ErrInvalidProject)
	}
	if req.AttemptID == uuid.Nil {
		return PresignRuntimeRawLogResult{}, fmt.Errorf("%w: attempt_id is required", ErrInvalidProject)
	}
	if req.SizeBytes < 0 || req.SizeBytes > rawLogMaxPartSizeBytes {
		return PresignRuntimeRawLogResult{}, fmt.Errorf("%w: size_bytes must be within [0, %d]", ErrInvalidProject, rawLogMaxPartSizeBytes)
	}
	if _, err := s.repository.GetProjectTaskAttempt(ctx, req.TenantID, req.AttemptID); err != nil {
		return PresignRuntimeRawLogResult{}, fmt.Errorf("resolve raw log attempt: %w", err)
	}

	var key, contentType string
	switch req.Object {
	case rawLogPresignObjectPart:
		if req.PartIndex == nil || *req.PartIndex < 0 || *req.PartIndex > 9999 {
			return PresignRuntimeRawLogResult{}, fmt.Errorf("%w: part_index must be within [0, 9999]", ErrInvalidProject)
		}
		key = fmt.Sprintf("%s%s/%s/raw.part-%04d.jsonl", rawLogObjectKeyPrefix, req.TenantID, req.AttemptID, *req.PartIndex)
		contentType = "application/x-ndjson"
	case rawLogPresignObjectindex:
		key = fmt.Sprintf("%s%s/%s/manifest.json", rawLogObjectKeyPrefix, req.TenantID, req.AttemptID)
		contentType = "application/json"
	default:
		return PresignRuntimeRawLogResult{}, fmt.Errorf("%w: object must be part or manifest", ErrInvalidProject)
	}

	url, err := s.artifactObjectStore.PresignPut(ctx, key, contentType, artifactPresignTTL)
	if err != nil {
		return PresignRuntimeRawLogResult{}, fmt.Errorf("presign raw log put: %w", err)
	}
	return PresignRuntimeRawLogResult{
		ObjectKey: key,
		UploadURL: url,
		ExpiresAt: time.Now().Add(artifactPresignTTL),
	}, nil
}

// GetArtifactRef 按租户取回单条 artifact 引用(内容取回端点用)。
func (s *Service) GetArtifactRef(ctx context.Context, tenantID, artifactRefID uuid.UUID) (ProjectArtifactRef, error) {
	if tenantID == uuid.Nil || artifactRefID == uuid.Nil {
		return ProjectArtifactRef{}, ErrInvalidProject
	}
	return s.repository.GetArtifactRef(ctx, tenantID, artifactRefID)
}

// PresignArtifactContent 为已物化的 artifact 签发短时下载 URL。
// 只对本租户前缀内的对象 key 签发;外部引用/遗留 ref 不可取回。
func (s *Service) PresignArtifactContent(ctx context.Context, ref ProjectArtifactRef) (string, error) {
	if s.artifactObjectStore == nil {
		return "", fmt.Errorf("artifact object store is not configured")
	}
	expectedPrefix := fmt.Sprintf("%s%s/", artifactObjectKeyPrefix, ref.TenantID)
	if !strings.HasPrefix(ref.ObjectRef, expectedPrefix) {
		return "", fmt.Errorf("%w: artifact content is not retrievable (external or legacy object_ref)", ErrInvalidProjectEvidence)
	}
	url, err := s.artifactObjectStore.PresignGet(ctx, ref.ObjectRef, artifactContentGetTTL)
	if err != nil {
		return "", fmt.Errorf("presign artifact get: %w", err)
	}
	return url, nil
}

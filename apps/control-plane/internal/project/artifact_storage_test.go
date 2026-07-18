package project

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type fakeArtifactObjectStore struct {
	existing map[string]int64
	putURLs  map[string]string
	getURLs  map[string]string
	statErr  error
}

func (f *fakeArtifactObjectStore) StatObject(_ context.Context, key string) (bool, int64, error) {
	if f.statErr != nil {
		return false, 0, f.statErr
	}
	size, ok := f.existing[key]
	return ok, size, nil
}

func (f *fakeArtifactObjectStore) PresignPut(_ context.Context, key, _ string, _ time.Duration) (string, error) {
	if url, ok := f.putURLs[key]; ok {
		return url, nil
	}
	return "https://signed.local/put/" + key, nil
}

func (f *fakeArtifactObjectStore) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	if url, ok := f.getURLs[key]; ok {
		return url, nil
	}
	return "https://signed.local/get/" + key, nil
}

func newArtifactStorageTestService(t *testing.T, store ArtifactObjectStore) (*Service, *governanceMemoryRepository) {
	t.Helper()
	repo := newGovernanceMemoryRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	service.SetArtifactObjectStore(store)
	return service, repo
}

func TestPresignRuntimeArtifactUploadDerivesTenantScopedKey(t *testing.T) {
	tenantID := uuid.New()
	sha := strings.Repeat("ab", 32)
	store := &fakeArtifactObjectStore{}
	service, _ := newArtifactStorageTestService(t, store)

	result, err := service.PresignRuntimeArtifactUpload(context.Background(), PresignRuntimeArtifactRequest{
		TenantID:    tenantID,
		Sha256:      strings.ToUpper(sha), // 大小写归一
		SizeBytes:   1024,
		ContentType: "application/x-ndjson",
	})
	if err != nil {
		t.Fatalf("presign artifact: %v", err)
	}
	wantKey := fmt.Sprintf("artifacts/%s/sha256/%s", tenantID, sha)
	if result.ObjectKey != wantKey {
		t.Fatalf("expected key %s, got %s", wantKey, result.ObjectKey)
	}
	if result.AlreadyExists || result.UploadURL == "" {
		t.Fatalf("expected signable upload, got %#v", result)
	}
}

func TestPresignRuntimeArtifactUploadRejectsInvalidInput(t *testing.T) {
	store := &fakeArtifactObjectStore{}
	service, _ := newArtifactStorageTestService(t, store)
	tenantID := uuid.New()
	valid := strings.Repeat("ab", 32)

	cases := []struct {
		name string
		req  PresignRuntimeArtifactRequest
	}{
		{"非法 sha256", PresignRuntimeArtifactRequest{TenantID: tenantID, Sha256: "not-hex", SizeBytes: 1}},
		{"sha256 长度不足", PresignRuntimeArtifactRequest{TenantID: tenantID, Sha256: "abcd", SizeBytes: 1}},
		{"size 为零", PresignRuntimeArtifactRequest{TenantID: tenantID, Sha256: valid, SizeBytes: 0}},
		{"size 超单文件上限", PresignRuntimeArtifactRequest{TenantID: tenantID, Sha256: valid, SizeBytes: ArtifactMaxFileSizeBytes + 1}},
		{"缺 tenant", PresignRuntimeArtifactRequest{Sha256: valid, SizeBytes: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := service.PresignRuntimeArtifactUpload(context.Background(), tc.req); !errors.Is(err, ErrInvalidProject) {
				t.Fatalf("expected ErrInvalidProject, got %v", err)
			}
		})
	}
}

func TestPresignRuntimeArtifactUploadShortCircuitsOnExistingContent(t *testing.T) {
	tenantID := uuid.New()
	sha := strings.Repeat("cd", 32)
	key := fmt.Sprintf("artifacts/%s/sha256/%s", tenantID, sha)
	store := &fakeArtifactObjectStore{existing: map[string]int64{key: 42}}
	service, _ := newArtifactStorageTestService(t, store)

	result, err := service.PresignRuntimeArtifactUpload(context.Background(), PresignRuntimeArtifactRequest{
		TenantID:  tenantID,
		Sha256:    sha,
		SizeBytes: 42,
	})
	if err != nil {
		t.Fatalf("presign artifact: %v", err)
	}
	if !result.AlreadyExists {
		t.Fatalf("expected already_exists for stored content")
	}
	if result.UploadURL != "" {
		t.Fatalf("already-existing content must not get an upload URL")
	}
}

func TestPresignRuntimeRawLogUploadValidatesAttemptTenancy(t *testing.T) {
	store := &fakeArtifactObjectStore{}
	service, _ := newArtifactStorageTestService(t, store)
	partIndex := int32(3)

	// governanceMemoryRepository 无该 attempt → ErrProjectNotFound → 拒绝签发。
	_, err := service.PresignRuntimeRawLogUpload(context.Background(), PresignRuntimeRawLogRequest{
		TenantID:  uuid.New(),
		AttemptID: uuid.New(),
		Object:    "part",
		PartIndex: &partIndex,
	})
	if err == nil {
		t.Fatalf("expected error for unknown attempt")
	}
}

func TestPresignArtifactContentRejectsForeignObjectRefs(t *testing.T) {
	store := &fakeArtifactObjectStore{}
	service, _ := newArtifactStorageTestService(t, store)
	tenantID := uuid.New()

	cases := []string{
		"runtime-command://cmd-123",                                                 // 遗留自报引用,无内容
		"s3://bucket/artifacts/" + tenantID.String(),                                // 非纯 key 形态
		fmt.Sprintf("artifacts/%s/sha256/%s", uuid.New(), strings.Repeat("ef", 32)), // 他租户前缀
	}
	for _, objectRef := range cases {
		if _, err := service.PresignArtifactContent(context.Background(), ProjectArtifactRef{
			TenantID:  tenantID,
			ObjectRef: objectRef,
		}); err == nil {
			t.Fatalf("expected rejection for object_ref %q", objectRef)
		}
	}

	key := fmt.Sprintf("artifacts/%s/sha256/%s", tenantID, strings.Repeat("ef", 32))
	url, err := service.PresignArtifactContent(context.Background(), ProjectArtifactRef{
		TenantID:  tenantID,
		ObjectRef: key,
	})
	if err != nil || url == "" {
		t.Fatalf("expected presigned url for tenant-scoped key, got %q %v", url, err)
	}
}

func TestParseUploadedArtifactRefRecognizesContentAddressedEntries(t *testing.T) {
	sha := strings.Repeat("aa", 32)
	entry := map[string]any{
		"type":         "execution_transcript",
		"name":         "raw.jsonl",
		"sha256":       sha,
		"size_bytes":   float64(2048),
		"content_type": "application/x-ndjson",
		"truncated":    true,
		"is_evidence":  true,
	}
	parsed, ok := parseUploadedArtifactRef(entry)
	if !ok {
		t.Fatalf("expected uploaded artifact to parse")
	}
	if parsed.Sha256 != sha || parsed.SizeBytes != 2048 || !parsed.IsEvidence || !parsed.Truncated {
		t.Fatalf("unexpected parse result: %#v", parsed)
	}

	if _, ok := parseUploadedArtifactRef("bare-string-ref"); ok {
		t.Fatalf("bare string must not parse as uploaded artifact")
	}
	if _, ok := parseUploadedArtifactRef(map[string]any{"ref": "x", "sha256": "zz"}); ok {
		t.Fatalf("invalid sha256 must not parse as uploaded artifact")
	}
}

func TestEvidenceRowForArtifactTypeMapping(t *testing.T) {
	cases := []struct {
		artifactType string
		isEvidence   bool
		wantType     string
		wantStatus   EvidenceVerificationStatus
	}{
		{"execution_transcript", true, "execution_transcript", EvidenceVerificationStatusSubmitted},
		{"diff", true, "code_change", EvidenceVerificationStatusSubmitted},
		{"declared", true, "declared_output", EvidenceVerificationStatusSubmitted},
		{"conclusion", false, "self_report", EvidenceVerificationStatusUnverified},
		{"custom_tool_output", true, "custom_tool_output", EvidenceVerificationStatusSubmitted},
	}
	for _, tc := range cases {
		gotType, gotStatus := evidenceRowForArtifactType(tc.artifactType, tc.isEvidence)
		if gotType != tc.wantType || gotStatus != tc.wantStatus {
			t.Fatalf("mapping %s/%v: got (%s,%s), want (%s,%s)", tc.artifactType, tc.isEvidence, gotType, gotStatus, tc.wantType, tc.wantStatus)
		}
	}
}

func TestTaskResultRefsToAnyCarriesUploadFields(t *testing.T) {
	sha := strings.Repeat("bb", 32)
	values := taskResultRefsToAny([]TaskResultRef{{
		Type:        "execution_transcript",
		Ref:         "artifacts/t/sha256/" + sha,
		Name:        "raw.jsonl",
		Sha256:      sha,
		SizeBytes:   99,
		ContentType: "application/x-ndjson",
		Truncated:   false,
		IsEvidence:  true,
	}})
	if len(values) != 1 {
		t.Fatalf("expected one value")
	}
	entry, ok := values[0].(map[string]any)
	if !ok {
		t.Fatalf("expected map entry")
	}
	if entry["sha256"] != sha || entry["is_evidence"] != true || entry["name"] != "raw.jsonl" {
		t.Fatalf("upload fields must pass through: %#v", entry)
	}
	parsed, ok := parseUploadedArtifactRef(values[0])
	if !ok || parsed.Sha256 != sha {
		t.Fatalf("round-trip through materializer parser failed: %#v", parsed)
	}
}

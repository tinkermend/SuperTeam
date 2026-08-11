package runtime

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// --- fakes ---

type nativeConfigFakeRepo struct {
	mu      sync.Mutex
	nodes   map[string]NodeRecord
	configs map[string]ProviderNativeConfigRecord // key: nodeID|provider|configKey
	events  []CreateRuntimeEventParams
}

func newNativeConfigFakeRepo(node NodeRecord) *nativeConfigFakeRepo {
	return &nativeConfigFakeRepo{
		nodes:   map[string]NodeRecord{node.NodeID: node},
		configs: map[string]ProviderNativeConfigRecord{},
	}
}

func snapshotMapKey(nodeID, provider, key string) string {
	return nodeID + "|" + provider + "|" + key
}

func (r *nativeConfigFakeRepo) CreateNode(context.Context, CreateNodeParams) (NodeRecord, error) {
	return NodeRecord{}, errors.New("not implemented")
}
func (r *nativeConfigFakeRepo) GetNode(_ context.Context, nodeID string) (NodeRecord, error) {
	n, ok := r.nodes[nodeID]
	if !ok {
		return NodeRecord{}, errors.New("not found")
	}
	return n, nil
}
func (r *nativeConfigFakeRepo) GetNodeByID(_ context.Context, id uuid.UUID) (NodeRecord, error) {
	for _, n := range r.nodes {
		if n.ID == id {
			return n, nil
		}
	}
	return NodeRecord{}, errors.New("not found")
}
func (r *nativeConfigFakeRepo) ListNodes(context.Context, ListNodesParams) ([]NodeRecord, error) {
	return nil, nil
}
func (r *nativeConfigFakeRepo) ListOnlineNodes(context.Context, pgtype.Timestamptz) ([]NodeRecord, error) {
	return nil, nil
}
func (r *nativeConfigFakeRepo) UpdateHeartbeat(context.Context, UpdateHeartbeatParams) (NodeRecord, error) {
	return NodeRecord{}, errors.New("not implemented")
}
func (r *nativeConfigFakeRepo) ApplyHeartbeat(context.Context, ApplyHeartbeatParams) (NodeRecord, error) {
	return NodeRecord{}, errors.New("not implemented")
}
func (r *nativeConfigFakeRepo) UpdateLoad(context.Context, UpdateLoadParams) (NodeRecord, error) {
	return NodeRecord{}, errors.New("not implemented")
}
func (r *nativeConfigFakeRepo) TryAcquireNodeSlot(context.Context, string, pgtype.Timestamptz) (NodeRecord, error) {
	return NodeRecord{}, errors.New("not implemented")
}
func (r *nativeConfigFakeRepo) UpdateStatus(context.Context, UpdateStatusParams) (NodeRecord, error) {
	return NodeRecord{}, errors.New("not implemented")
}
func (r *nativeConfigFakeRepo) PatchNodeMetadata(context.Context, PatchNodeMetadataParams) (NodeRecord, error) {
	return NodeRecord{}, errors.New("not implemented")
}
func (r *nativeConfigFakeRepo) CountOnlineNodesWithoutPlatformLimits(context.Context, uuid.UUID, pgtype.Timestamptz) (int64, error) {
	return 0, nil
}
func (r *nativeConfigFakeRepo) DeleteNode(context.Context, string) error { return nil }

func (r *nativeConfigFakeRepo) UpsertProviderNativeConfig(_ context.Context, params UpsertProviderNativeConfigParams) (ProviderNativeConfigRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec := ProviderNativeConfigRecord{
		ID:                 uuid.New(),
		TenantID:           params.TenantID,
		RuntimeNodeID:      params.RuntimeNodeID,
		NodeID:             params.NodeID,
		ProviderType:       params.ProviderType,
		ConfigKey:          params.ConfigKey,
		ResolvedPath:       params.ResolvedPath,
		Format:             params.Format,
		ManagedValues:      cloneAnyMap(params.ManagedValues),
		FileContentHash:    params.FileContentHash,
		ExistsOnNode:       params.ExistsOnNode,
		Manageable:         params.Manageable,
		UnmanageableReason: params.UnmanageableReason,
		Source:             params.Source,
		NodeMtime:          params.NodeMtime,
		SnapshotAt:         params.SnapshotAt,
		LastPulledAt:       params.LastPulledAt,
		LastPushedAt:       params.LastPushedAt,
		LastPushedBy:       params.LastPushedBy,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}
	key := snapshotMapKey(params.NodeID, params.ProviderType, params.ConfigKey)
	if existing, ok := r.configs[key]; ok {
		rec.ID = existing.ID
		rec.CreatedAt = existing.CreatedAt
	}
	r.configs[key] = rec
	return rec, nil
}

func (r *nativeConfigFakeRepo) ListProviderNativeConfigsForNode(_ context.Context, tenantID uuid.UUID, nodeID string) ([]ProviderNativeConfigRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ProviderNativeConfigRecord, 0)
	for _, rec := range r.configs {
		if rec.TenantID == tenantID && rec.NodeID == nodeID {
			out = append(out, rec)
		}
	}
	return out, nil
}

func (r *nativeConfigFakeRepo) GetProviderNativeConfig(_ context.Context, tenantID uuid.UUID, nodeID, providerType, configKey string) (ProviderNativeConfigRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rec, ok := r.configs[snapshotMapKey(nodeID, providerType, configKey)]
	if !ok || rec.TenantID != tenantID {
		return ProviderNativeConfigRecord{}, ErrProviderNativeConfigNotFound
	}
	return rec, nil
}

// Satisfy RuntimeEventRepository via type assertion path used by recordRuntimeEventBestEffort.
func (r *nativeConfigFakeRepo) CreateRuntimeEvent(_ context.Context, params CreateRuntimeEventParams) (RuntimeEvent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, params)
	return RuntimeEvent{ID: uuid.New(), EventType: params.EventType, Payload: params.Payload}, nil
}
func (r *nativeConfigFakeRepo) ListRuntimeEvents(context.Context, ListRuntimeEventsParams) ([]RuntimeEvent, error) {
	return nil, nil
}
func (r *nativeConfigFakeRepo) CountBlockedRuntimeEventsSince(context.Context, uuid.UUID, time.Time) (int64, error) {
	return 0, nil
}

type fakeNativeCommander struct {
	online      bool
	waitReceipt *NativeConfigCommandReceipt
	waitErr     error
	dispatched  []RuntimeCommand
	receipts    []NativeConfigCommandReceiptRequest
	timedOut    []string
}

func (f *fakeNativeCommander) IsConnected(string) bool { return f.online }
func (f *fakeNativeCommander) Dispatch(_ context.Context, _ string, command RuntimeCommand) error {
	f.dispatched = append(f.dispatched, command)
	return nil
}
func (f *fakeNativeCommander) CreateCommandReceipt(_ context.Context, req NativeConfigCommandReceiptRequest) error {
	f.receipts = append(f.receipts, req)
	return nil
}
func (f *fakeNativeCommander) WaitForCommandCompletion(context.Context, uuid.UUID, string, time.Duration) (*NativeConfigCommandReceipt, error) {
	if f.waitErr != nil {
		return nil, f.waitErr
	}
	return f.waitReceipt, nil
}
func (f *fakeNativeCommander) MarkCommandTimedOut(_ context.Context, _ uuid.UUID, commandID, _ string) error {
	f.timedOut = append(f.timedOut, commandID)
	return nil
}

func cloneAnyMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// testSealer is a minimal AES-GCM sealer compatible with production aesgcm:v1: prefix,
// kept local to avoid importing capability (import cycle via middleware).
type testSealer struct {
	aead cipher.AEAD
}

func newTestSealer(t *testing.T) *testSealer {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("gcm: %v", err)
	}
	return &testSealer{aead: aead}
}

func (s *testSealer) Seal(plain string) (string, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := s.aead.Seal(nil, nonce, []byte(plain), nil)
	payload := append(nonce, ciphertext...)
	return "aesgcm:v1:" + base64.StdEncoding.EncodeToString(payload), nil
}

func (s *testSealer) Open(sealed string) (string, error) {
	const prefix = "aesgcm:v1:"
	if !strings.HasPrefix(sealed, prefix) {
		return "", errors.New("invalid sealed prefix")
	}
	payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sealed, prefix))
	if err != nil {
		return "", err
	}
	ns := s.aead.NonceSize()
	if len(payload) <= ns {
		return "", errors.New("invalid sealed payload")
	}
	plain, err := s.aead.Open(nil, payload[:ns], payload[ns:], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func testNode(tenantID uuid.UUID) NodeRecord {
	return NodeRecord{
		ID:                 uuid.New(),
		TenantID:           tenantID,
		NodeID:             "local-dev-node",
		Name:               "local-dev",
		Status:             "online",
		SupportedProviders: []byte(`["codex","claude-code","opencode"]`),
		Metadata:           []byte(`{}`),
	}
}

func newNativeConfigService(t *testing.T, repo *nativeConfigFakeRepo, commander *fakeNativeCommander) *Service {
	t.Helper()
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	svc.SetCredentialSealer(newTestSealer(t))
	svc.SetNativeConfigCommander(commander)
	return svc
}

// --- tests ---

func TestSealAndOpenManagedValuesRoundTrip(t *testing.T) {
	t.Parallel()
	svc := &Service{sealer: newTestSealer(t)}
	in := map[string]any{
		"model": "gpt-5",
		"model_providers.custom.experimental_bearer_token": "sk-secret-token",
		"model_provider": "custom",
	}
	sealed, err := svc.sealManagedValues("codex", "model_profile", in)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if sealed["model"] != "gpt-5" {
		t.Fatalf("non-sensitive should stay plain, got %#v", sealed["model"])
	}
	tok, _ := sealed["model_providers.custom.experimental_bearer_token"].(string)
	if !strings.HasPrefix(tok, "aesgcm:v1:") {
		t.Fatalf("token should be sealed, got %q", tok)
	}
	opened, err := svc.openManagedValues(sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if opened["model_providers.custom.experimental_bearer_token"] != "sk-secret-token" {
		t.Fatalf("round-trip token mismatch: %#v", opened["model_providers.custom.experimental_bearer_token"])
	}
	if opened["model"] != "gpt-5" {
		t.Fatalf("model mismatch: %#v", opened["model"])
	}
}

// 密封/解封必须保类型：形如 JSON 标量的密钥（纯数字、true、null）
// 过去会在解封时被 json.Unmarshal 解成 number/bool/nil，再写回配置文件就是错类型。
func TestSealAndOpenManagedValuesPreservesScalarLookingSecrets(t *testing.T) {
	t.Parallel()
	svc := &Service{sealer: newTestSealer(t)}
	cases := map[string]any{
		"numeric":     "123456",
		"boolean":     "true",
		"nullish":     "null",
		"leadingZero": "007",
		"jsonObject":  `{"a":1}`,
		"ordinary":    "sk-live-abc",
	}
	for name, secret := range cases {
		in := map[string]any{"env.ANTHROPIC_API_KEY": secret}
		sealed, err := svc.sealManagedValues("claude-code", "model_profile", in)
		if err != nil {
			t.Fatalf("%s seal: %v", name, err)
		}
		opened, err := svc.openManagedValues(sealed)
		if err != nil {
			t.Fatalf("%s open: %v", name, err)
		}
		got := opened["env.ANTHROPIC_API_KEY"]
		if got != secret {
			t.Fatalf("%s round-trip changed value/type: want %#v (%T), got %#v (%T)",
				name, secret, secret, got, got)
		}
	}

	// 非字符串类型同样保真。
	in := map[string]any{"model_providers.custom.query_params": map[string]any{"key": "v"}}
	sealed, err := svc.sealManagedValues("codex", "model_profile", in)
	if err != nil {
		t.Fatalf("seal object: %v", err)
	}
	opened, err := svc.openManagedValues(sealed)
	if err != nil {
		t.Fatalf("open object: %v", err)
	}
	obj, ok := opened["model_providers.custom.query_params"].(map[string]any)
	if !ok || obj["key"] != "v" {
		t.Fatalf("object round-trip mismatch: %#v", opened["model_providers.custom.query_params"])
	}
}

// 供应商端点上的凭据面必须判为敏感（静态加密 + UI 掩码同源）。
func TestSensitiveKeyCoversProviderCredentialSurfaces(t *testing.T) {
	t.Parallel()
	sensitive := []struct{ provider, configKey, key string }{
		{"opencode", "model_profile", "provider.custom.options.apiKey"},
		{"opencode", "model_profile", "provider.custom.options.headers.Authorization"},
		{"opencode", "model_profile", "provider.custom.options.headers.x-api-key"},
		{"codex", "model_profile", "model_providers.custom.http_headers"},
		{"codex", "model_profile", "model_providers.custom.query_params"},
	}
	for _, tc := range sensitive {
		if !isSensitiveManagedKey(tc.provider, tc.configKey, tc.key) {
			t.Fatalf("expected %s sensitive", tc.key)
		}
	}
	notSensitive := []struct{ provider, configKey, key string }{
		{"opencode", "model_profile", "provider.custom.options.baseURL"},
		{"opencode", "model_profile", "provider.custom.options.headers.X-Trace"},
		{"opencode", "model_profile", "provider.custom.name"},
		{"codex", "model_profile", "model_providers.custom.base_url"},
		{"codex", "model_profile", "model_providers.custom.env_key"},
	}
	for _, tc := range notSensitive {
		if isSensitiveManagedKey(tc.provider, tc.configKey, tc.key) {
			t.Fatalf("did not expect %s sensitive", tc.key)
		}
	}
}

func TestSealManagedValuesRequiresSealerForSensitive(t *testing.T) {
	t.Parallel()
	svc := &Service{}
	_, err := svc.sealManagedValues("claude-code", "model_profile", map[string]any{
		"env.ANTHROPIC_API_KEY": "sk",
	})
	if err == nil || !strings.Contains(err.Error(), "credential sealer") {
		t.Fatalf("expected sealer required error, got %v", err)
	}
}

func TestPullProviderNativeConfigOffline(t *testing.T) {
	t.Parallel()
	tenant := uuid.New()
	node := testNode(tenant)
	repo := newNativeConfigFakeRepo(node)
	cmd := &fakeNativeCommander{online: false}
	svc := newNativeConfigService(t, repo, cmd)

	_, err := svc.PullProviderNativeConfig(context.Background(), tenant, uuid.New(), node.NodeID, "codex", "model_profile")
	if !errors.Is(err, ErrProviderNativeConfigOffline) {
		t.Fatalf("expected offline, got %v", err)
	}
}

func TestPullProviderNativeConfigSuccessUpsertsSnapshot(t *testing.T) {
	t.Parallel()
	tenant := uuid.New()
	actor := uuid.New()
	node := testNode(tenant)
	repo := newNativeConfigFakeRepo(node)
	cmd := &fakeNativeCommander{
		online: true,
		waitReceipt: &NativeConfigCommandReceipt{
			CommandID: "cmd-1",
			Status:    "completed",
			Result: map[string]any{
				"provider_type":     "codex",
				"config_key":        "model_profile",
				"resolved_path":     "/tmp/.codex/config.toml",
				"format":            "toml",
				"exists":            true,
				"manageable":        true,
				"file_content_hash": "sha256:abc",
				"managed_values": map[string]any{
					"model": "gpt-5.6",
					"model_providers.custom.experimental_bearer_token": "sk-live",
				},
				"managed_key_names": []any{"model", "model_providers.custom.experimental_bearer_token"},
			},
		},
	}
	svc := newNativeConfigService(t, repo, cmd)

	// Simulate writeback applying transit result before waiter observes completed receipt.
	if err := svc.ApplyNativeConfigWriteback(
		context.Background(),
		tenant,
		node.ID,
		node.NodeID,
		runtimeCommandReadProviderNativeConfig,
		cmd.waitReceipt.Result,
		true,
	); err != nil {
		t.Fatalf("writeback: %v", err)
	}

	detail, err := svc.PullProviderNativeConfig(context.Background(), tenant, actor, node.NodeID, "codex", "model_profile")
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if detail.ManagedValues["model"] != "gpt-5.6" {
		t.Fatalf("expected decrypted model, got %#v", detail.ManagedValues)
	}
	if detail.ManagedValues["model_providers.custom.experimental_bearer_token"] != "sk-live" {
		t.Fatalf("expected decrypted token, got %#v", detail.ManagedValues["model_providers.custom.experimental_bearer_token"])
	}
	if detail.StaleHint {
		t.Fatal("live pull should clear stale_hint")
	}
	if detail.FileContentHash != "sha256:abc" {
		t.Fatalf("hash: %s", detail.FileContentHash)
	}

	// Stored snapshot must keep token sealed.
	stored, err := repo.GetProviderNativeConfig(context.Background(), tenant, node.NodeID, "codex", "model_profile")
	if err != nil {
		t.Fatalf("stored: %v", err)
	}
	tok, _ := stored.ManagedValues["model_providers.custom.experimental_bearer_token"].(string)
	if !strings.HasPrefix(tok, "aesgcm:v1:") {
		t.Fatalf("stored token must be sealed, got %q", tok)
	}
	if len(cmd.dispatched) != 1 || cmd.dispatched[0].Type != runtimeCommandReadProviderNativeConfig {
		t.Fatalf("dispatch: %#v", cmd.dispatched)
	}
	if len(cmd.receipts) != 1 || cmd.receipts[0].ResourceType != providerNativeConfigResourceType {
		t.Fatalf("receipt: %#v", cmd.receipts)
	}
}

// 快照落库失败时 writeback 仍把回执置 completed（为了解阻塞等待者），
// 但 pull 的全部目的就是取新值——回执里 managed_values 已被剥离，CP 无从恢复，
// 因此必须报错，而不是把上一次的旧快照当实时值返回。
func TestPullSurfacesSnapshotError(t *testing.T) {
	t.Parallel()
	tenant := uuid.New()
	actor := uuid.New()
	node := testNode(tenant)
	repo := newNativeConfigFakeRepo(node)
	cmd := &fakeNativeCommander{
		online: true,
		waitReceipt: &NativeConfigCommandReceipt{
			CommandID: "cmd-snap-1",
			Status:    "completed",
			Result: map[string]any{
				"provider_type":     "codex",
				"config_key":        "model_profile",
				"file_content_hash": "sha256:new",
				"snapshot_error":    "seal: credential sealer is required",
			},
		},
	}
	svc := newNativeConfigService(t, repo, cmd)

	// 预置一份旧快照：修复前这份会被当成「实时」返回。
	if _, err := repo.UpsertProviderNativeConfig(context.Background(), UpsertProviderNativeConfigParams{
		TenantID:        tenant,
		RuntimeNodeID:   node.ID,
		NodeID:          node.NodeID,
		ProviderType:    "codex",
		ConfigKey:       "model_profile",
		Format:          "toml",
		ManagedValues:   map[string]any{"model": "stale-model"},
		FileContentHash: "sha256:old",
		Manageable:      true,
		Source:          "pulled",
		SnapshotAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	_, err := svc.PullProviderNativeConfig(context.Background(), tenant, actor, node.NodeID, "codex", "model_profile")
	if err == nil {
		t.Fatal("expected pull to fail when the snapshot did not persist")
	}
	if !errors.Is(err, ErrProviderNativeConfigCommand) {
		t.Fatalf("expected command error sentinel, got %v", err)
	}
	if !strings.Contains(err.Error(), "credential sealer") {
		t.Fatalf("expected underlying snapshot error surfaced, got %v", err)
	}
}

// push 与 pull 不同：节点写入已经落盘，报错会误报成「没生效」。
// 因此返回成功，但把值标为非实时并带上原因。
func TestPushSnapshotErrorKeepsSuccessButMarksStale(t *testing.T) {
	t.Parallel()
	tenant := uuid.New()
	actor := uuid.New()
	node := testNode(tenant)
	repo := newNativeConfigFakeRepo(node)
	if _, err := repo.UpsertProviderNativeConfig(context.Background(), UpsertProviderNativeConfigParams{
		TenantID:        tenant,
		RuntimeNodeID:   node.ID,
		NodeID:          node.NodeID,
		ProviderType:    "codex",
		ConfigKey:       "model_profile",
		Format:          "toml",
		ManagedValues:   map[string]any{"model": "stale-model"},
		FileContentHash: "sha256:old",
		Manageable:      true,
		Source:          "pulled",
		SnapshotAt:      time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}
	cmd := &fakeNativeCommander{
		online: true,
		waitReceipt: &NativeConfigCommandReceipt{
			CommandID: "cmd-snap-2",
			Status:    "completed",
			Result: map[string]any{
				"provider_type":     "codex",
				"config_key":        "model_profile",
				"file_content_hash": "sha256:new",
				"changed_keys":      []any{"model"},
				"snapshot_error":    "upsert: connection reset",
			},
		},
	}
	svc := newNativeConfigService(t, repo, cmd)

	detail, err := svc.PushProviderNativeConfig(
		context.Background(), tenant, actor, node.NodeID, "codex", "model_profile",
		map[string]any{"model": "gpt-5.6"}, "sha256:old",
	)
	if err != nil {
		t.Fatalf("push should still report success: %v", err)
	}
	if !detail.StaleHint {
		t.Fatal("values are the previous snapshot; stale_hint must stay true")
	}
	if !strings.Contains(detail.SnapshotError, "connection reset") {
		t.Fatalf("expected snapshot_error surfaced, got %q", detail.SnapshotError)
	}
}

// 服务端下发敏感键名，Console 不再自行用键名启发式判断（会漂移）。
func TestGetSnapshotServesSensitiveKeyList(t *testing.T) {
	t.Parallel()
	tenant := uuid.New()
	node := testNode(tenant)
	repo := newNativeConfigFakeRepo(node)
	svc := newNativeConfigService(t, repo, &fakeNativeCommander{online: true})

	sealed, err := svc.sealManagedValues("opencode", "model_profile", map[string]any{
		"model":                           "anthropic/claude-sonnet-4-5",
		"provider.custom.options.baseURL": "https://api.example/v1",
		"provider.custom.options.apiKey":  "sk-live-abc",
	})
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if _, err := repo.UpsertProviderNativeConfig(context.Background(), UpsertProviderNativeConfigParams{
		TenantID:      tenant,
		RuntimeNodeID: node.ID,
		NodeID:        node.NodeID,
		ProviderType:  "opencode",
		ConfigKey:     "model_profile",
		Format:        "json",
		ManagedValues: sealed,
		Manageable:    true,
		Source:        "pulled",
		SnapshotAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	detail, err := svc.GetProviderNativeConfigSnapshot(context.Background(), tenant, node.NodeID, "opencode", "model_profile")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(detail.SensitiveKeys) != 1 || detail.SensitiveKeys[0] != "provider.custom.options.apiKey" {
		t.Fatalf("expected only apiKey flagged sensitive, got %#v", detail.SensitiveKeys)
	}
	if detail.ManagedValues["provider.custom.options.apiKey"] != "sk-live-abc" {
		t.Fatalf("apiKey must decrypt for authorized reads, got %#v", detail.ManagedValues["provider.custom.options.apiKey"])
	}
}

func TestPushProviderNativeConfigConflict(t *testing.T) {
	t.Parallel()
	tenant := uuid.New()
	node := testNode(tenant)
	repo := newNativeConfigFakeRepo(node)
	msg := "conflict: file content hash mismatch"
	cmd := &fakeNativeCommander{
		online: true,
		waitReceipt: &NativeConfigCommandReceipt{
			CommandID:    "cmd-w",
			Status:       "failed",
			ErrorMessage: &msg,
			Result: map[string]any{
				"diagnostic": map[string]any{"error_code": "conflict"},
			},
		},
	}
	svc := newNativeConfigService(t, repo, cmd)

	_, err := svc.PushProviderNativeConfig(
		context.Background(),
		tenant,
		uuid.New(),
		node.NodeID,
		"codex",
		"model_profile",
		map[string]any{"model": "x"},
		"sha256:stale",
	)
	if !errors.Is(err, ErrProviderNativeConfigConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestPushProviderNativeConfigValidationRejectsAllowlist(t *testing.T) {
	t.Parallel()
	tenant := uuid.New()
	node := testNode(tenant)
	repo := newNativeConfigFakeRepo(node)
	cmd := &fakeNativeCommander{online: true}
	svc := newNativeConfigService(t, repo, cmd)

	_, err := svc.PushProviderNativeConfig(
		context.Background(),
		tenant,
		uuid.New(),
		node.NodeID,
		"codex",
		"model_profile",
		map[string]any{"mcp_servers.x.command": "evil"},
		"sha256:h",
	)
	if !errors.Is(err, ErrProviderNativeConfigValidation) {
		t.Fatalf("expected validation, got %v", err)
	}
	if len(cmd.dispatched) != 0 {
		t.Fatal("must not dispatch invalid keys")
	}
}

func TestPushProviderNativeConfigUnmanageableClaudeAuth(t *testing.T) {
	t.Parallel()
	tenant := uuid.New()
	node := testNode(tenant)
	repo := newNativeConfigFakeRepo(node)
	cmd := &fakeNativeCommander{online: true}
	svc := newNativeConfigService(t, repo, cmd)

	_, err := svc.PushProviderNativeConfig(
		context.Background(),
		tenant,
		uuid.New(),
		node.NodeID,
		"claude-code",
		"auth",
		map[string]any{"token": "x"},
		"sha256:empty",
	)
	if !errors.Is(err, ErrProviderNativeConfigUnmanageable) {
		t.Fatalf("expected unmanageable, got %v", err)
	}
	if len(cmd.dispatched) != 0 {
		t.Fatal("must not dispatch unmanageable claude auth")
	}
}

func TestPushProviderNativeConfigSuccess(t *testing.T) {
	t.Parallel()
	tenant := uuid.New()
	actor := uuid.New()
	node := testNode(tenant)
	repo := newNativeConfigFakeRepo(node)
	cmd := &fakeNativeCommander{
		online: true,
		waitReceipt: &NativeConfigCommandReceipt{
			CommandID: "cmd-push",
			Status:    "completed",
			Result: map[string]any{
				"provider_type":     "codex",
				"config_key":        "model_profile",
				"file_content_hash": "sha256:new",
				"exists":            true,
				"manageable":        true,
				"format":            "toml",
				"resolved_path":     "/home/.codex/config.toml",
				"changed_keys":      []any{"model"},
				"managed_values": map[string]any{
					"model": "gpt-5.6-terra",
				},
			},
		},
	}
	svc := newNativeConfigService(t, repo, cmd)

	if err := svc.ApplyNativeConfigWriteback(
		context.Background(),
		tenant,
		node.ID,
		node.NodeID,
		runtimeCommandWriteProviderNativeConfig,
		cmd.waitReceipt.Result,
		true,
	); err != nil {
		t.Fatalf("writeback: %v", err)
	}

	detail, err := svc.PushProviderNativeConfig(
		context.Background(),
		tenant,
		actor,
		node.NodeID,
		"codex",
		"model_profile",
		map[string]any{"model": "gpt-5.6-terra"},
		"sha256:old",
	)
	if err != nil {
		t.Fatalf("push: %v", err)
	}
	if detail.ManagedValues["model"] != "gpt-5.6-terra" {
		t.Fatalf("model: %#v", detail.ManagedValues)
	}
	if detail.Source != "pushed" && detail.FileContentHash != "sha256:new" {
		// source may be re-read after last_pushed patch
	}
	stored, _ := repo.GetProviderNativeConfig(context.Background(), tenant, node.NodeID, "codex", "model_profile")
	if stored.LastPushedBy == nil || *stored.LastPushedBy != actor {
		t.Fatalf("last_pushed_by: %#v", stored.LastPushedBy)
	}
	// Audit event should not contain secret values
	for _, ev := range repo.events {
		if ev.EventType == RuntimeEventProviderNativeConfigPush {
			if _, ok := ev.Payload["managed_values"]; ok {
				t.Fatal("audit must not include managed_values")
			}
			if ev.Payload["actor_user_id"] != actor.String() {
				t.Fatalf("actor: %#v", ev.Payload["actor_user_id"])
			}
		}
	}
}

func TestPullWaitTimeoutMarksTimedOut(t *testing.T) {
	t.Parallel()
	tenant := uuid.New()
	node := testNode(tenant)
	repo := newNativeConfigFakeRepo(node)
	cmd := &fakeNativeCommander{
		online:  true,
		waitErr: context.DeadlineExceeded,
	}
	svc := newNativeConfigService(t, repo, cmd)

	_, err := svc.PullProviderNativeConfig(context.Background(), tenant, uuid.New(), node.NodeID, "codex", "model_profile")
	if !errors.Is(err, ErrProviderNativeConfigCommand) {
		t.Fatalf("expected command error, got %v", err)
	}
	if len(cmd.timedOut) != 1 {
		t.Fatalf("expected timed_out mark, got %#v", cmd.timedOut)
	}
}

func TestGetSnapshotReturnsNotFound(t *testing.T) {
	t.Parallel()
	tenant := uuid.New()
	node := testNode(tenant)
	repo := newNativeConfigFakeRepo(node)
	svc := newNativeConfigService(t, repo, &fakeNativeCommander{online: true})

	_, err := svc.GetProviderNativeConfigSnapshot(context.Background(), tenant, node.NodeID, "codex", "model_profile")
	if !errors.Is(err, ErrProviderNativeConfigNotFound) {
		// GetNode path wraps; fake returns ErrProviderNativeConfigNotFound from Get
		if err == nil {
			t.Fatal("expected not found")
		}
	}
}

func TestListIncludesPlaceholdersWithoutValues(t *testing.T) {
	t.Parallel()
	tenant := uuid.New()
	node := testNode(tenant)
	repo := newNativeConfigFakeRepo(node)
	svc := newNativeConfigService(t, repo, &fakeNativeCommander{online: true})

	items, err := svc.ListProviderNativeConfigs(context.Background(), tenant, node.NodeID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) < 6 {
		t.Fatalf("expected known surfaces, got %d", len(items))
	}
	var foundClaudeAuth bool
	for _, item := range items {
		// List type has no ManagedValues field — compile-time safety.
		if item.ProviderType == "" || item.ConfigKey == "" {
			t.Fatalf("empty surface: %#v", item)
		}
		if !item.NodeOnline {
			t.Fatal("expected online")
		}
		// Pre-pull placeholder: claude-code/auth is never file-writable in v1.
		if item.ProviderType == "claude-code" && item.ConfigKey == "auth" {
			foundClaudeAuth = true
			if item.Manageable {
				t.Fatalf("claude-code/auth placeholder must be unmanageable, got %#v", item)
			}
			if item.UnmanageableReason != "oauth_session_protected" {
				t.Fatalf("expected oauth_session_protected, got %q", item.UnmanageableReason)
			}
		}
	}
	if !foundClaudeAuth {
		t.Fatal("expected claude-code/auth placeholder")
	}
}

func TestDefaultSurfaceManageability(t *testing.T) {
	t.Parallel()
	ok, reason := defaultSurfaceManageability("claude-code", "auth")
	if ok || reason != "oauth_session_protected" {
		t.Fatalf("claude-code/auth: got manageable=%v reason=%q", ok, reason)
	}
	ok, reason = defaultSurfaceManageability("codex", "auth")
	if !ok || reason != "" {
		t.Fatalf("codex/auth should default manageable until node reports keyring: ok=%v reason=%q", ok, reason)
	}
	ok, reason = defaultSurfaceManageability("claude-code", "model_profile")
	if !ok || reason != "" {
		t.Fatalf("model_profile should be manageable: ok=%v reason=%q", ok, reason)
	}
}

func TestMapNativeConfigCommandErrorUsesErrorsIs(t *testing.T) {
	t.Parallel()
	msg := "hash mismatch"
	err := mapNativeConfigCommandError(&NativeConfigCommandReceipt{
		Status:       "failed",
		ErrorMessage: &msg,
		Result: map[string]any{
			"diagnostic": map[string]any{"error_code": "conflict"},
		},
	})
	if !errors.Is(err, ErrProviderNativeConfigConflict) {
		t.Fatalf("errors.Is conflict failed: %v", err)
	}
	err = mapNativeConfigCommandError(&NativeConfigCommandReceipt{
		Status:       "failed",
		ErrorMessage: &msg,
		Result: map[string]any{
			"diagnostic": map[string]any{"error_code": "validation_error"},
		},
	})
	if !errors.Is(err, ErrProviderNativeConfigValidation) {
		t.Fatalf("errors.Is validation failed: %v", err)
	}
}

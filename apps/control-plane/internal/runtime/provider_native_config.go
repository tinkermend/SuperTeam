package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	runtimeCommandReadProviderNativeConfig  = "read_provider_native_config"
	runtimeCommandWriteProviderNativeConfig = "write_provider_native_config"
	providerNativeConfigResourceType        = "provider_native_config"
	defaultProviderNativeConfigTimeout      = 30 * time.Second
	defaultProviderNativeConfigPoll         = 250 * time.Millisecond
	sensitiveSealedPrefix                   = "aesgcm:v1:"
)

var (
	ErrProviderNativeConfigNotFound   = errors.New("provider native config snapshot not found")
	ErrProviderNativeConfigOffline    = errors.New("runtime node is not connected")
	ErrProviderNativeConfigConflict   = errors.New("provider native config conflict")
	ErrProviderNativeConfigValidation = errors.New("provider native config validation failed")
	ErrProviderNativeConfigUnmanageable = errors.New("provider native config surface is unmanageable")
	ErrProviderNativeConfigCommand    = errors.New("provider native config command failed")
)

// CredentialSealer encrypts sensitive managed key values at rest.
type CredentialSealer interface {
	Seal(plain string) (string, error)
	Open(sealed string) (string, error)
}

// NativeConfigCommander dispatches read/write commands to a connected Runtime Agent.
type NativeConfigCommander interface {
	IsConnected(nodeID string) bool
	Dispatch(ctx context.Context, nodeID string, command RuntimeCommand) error
	CreateCommandReceipt(ctx context.Context, req NativeConfigCommandReceiptRequest) error
	WaitForCommandCompletion(ctx context.Context, tenantID uuid.UUID, commandID string, interval time.Duration) (*NativeConfigCommandReceipt, error)
	// MarkCommandTimedOut flips a still-pending receipt so waiters and audits do not leave zombies.
	MarkCommandTimedOut(ctx context.Context, tenantID uuid.UUID, commandID, message string) error
}

type NativeConfigCommandReceiptRequest struct {
	TenantID      uuid.UUID
	CommandID     string
	CommandType   string
	RuntimeNodeID uuid.UUID
	NodeID        string
	ResourceType  string
	ResourceID    uuid.UUID
	Status        string
	Payload       map[string]any
	DispatchedAt  *time.Time
}

type NativeConfigCommandReceipt struct {
	CommandID    string
	Status       string
	ErrorMessage *string
	Result       map[string]any
}

func (s *Service) SetCredentialSealer(sealer CredentialSealer) {
	s.sealer = sealer
}

func (s *Service) SetNativeConfigCommander(commander NativeConfigCommander) {
	s.nativeConfigCommander = commander
}

// ListProviderNativeConfigs returns snapshot metadata without managed_values.
func (s *Service) ListProviderNativeConfigs(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]ProviderNativeConfigListItem, error) {
	if _, err := s.requireNodeForTenant(ctx, tenantID, nodeID); err != nil {
		return nil, err
	}
	repo, ok := s.repository.(ProviderNativeConfigRepository)
	if !ok {
		return nil, errors.New("provider native config repository is required")
	}
	records, err := repo.ListProviderNativeConfigsForNode(ctx, tenantID, nodeID)
	if err != nil {
		return nil, err
	}
	online := s.nativeConfigCommander != nil && s.nativeConfigCommander.IsConnected(nodeID)
	items := make([]ProviderNativeConfigListItem, 0, len(records))
	for _, rec := range records {
		snapshotAt := rec.SnapshotAt
		items = append(items, ProviderNativeConfigListItem{
			ProviderType:       rec.ProviderType,
			ConfigKey:          rec.ConfigKey,
			ResolvedPath:       rec.ResolvedPath,
			Format:             rec.Format,
			FileContentHash:    rec.FileContentHash,
			ExistsOnNode:       rec.ExistsOnNode,
			Manageable:         rec.Manageable,
			UnmanageableReason: rec.UnmanageableReason,
			Source:             rec.Source,
			SnapshotAt:         &snapshotAt,
			LastPulledAt:       rec.LastPulledAt,
			LastPushedAt:       rec.LastPushedAt,
			NodeOnline:         online,
		})
	}
	// Ensure known surfaces appear even without snapshots (empty placeholders).
	seen := map[string]struct{}{}
	for _, item := range items {
		seen[item.ProviderType+"/"+item.ConfigKey] = struct{}{}
	}
	for _, surface := range knownNativeConfigSurfaces() {
		key := surface.providerType + "/" + surface.configKey
		if _, ok := seen[key]; ok {
			continue
		}
		manageable, reason := defaultSurfaceManageability(surface.providerType, surface.configKey)
		items = append(items, ProviderNativeConfigListItem{
			ProviderType:       surface.providerType,
			ConfigKey:          surface.configKey,
			Format:             surface.format,
			Manageable:         manageable,
			UnmanageableReason: reason,
			Source:             "",
			NodeOnline:         online,
		})
	}
	return items, nil
}

// GetProviderNativeConfigSnapshot returns decrypted managed_values from CP snapshot (may be stale).
func (s *Service) GetProviderNativeConfigSnapshot(ctx context.Context, tenantID uuid.UUID, nodeID, providerType, configKey string) (*ProviderNativeConfigDetail, error) {
	if err := validateNativeConfigSurface(providerType, configKey); err != nil {
		return nil, err
	}
	if _, err := s.requireNodeForTenant(ctx, tenantID, nodeID); err != nil {
		return nil, err
	}
	repo, ok := s.repository.(ProviderNativeConfigRepository)
	if !ok {
		return nil, errors.New("provider native config repository is required")
	}
	rec, err := repo.GetProviderNativeConfig(ctx, tenantID, nodeID, providerType, configKey)
	if err != nil {
		return nil, err
	}
	values, err := s.openManagedValues(rec.ManagedValues)
	if err != nil {
		return nil, err
	}
	values = filterAllowlistedManagedValues(rec.ProviderType, rec.ConfigKey, values)
	sensitive := make([]string, 0, len(values))
	for k := range values {
		if isSensitiveManagedKey(rec.ProviderType, rec.ConfigKey, k) {
			sensitive = append(sensitive, k)
		}
	}
	sort.Strings(sensitive)
	online := s.nativeConfigCommander != nil && s.nativeConfigCommander.IsConnected(nodeID)
	return &ProviderNativeConfigDetail{
		SensitiveKeys:      sensitive,
		ProviderType:       rec.ProviderType,
		ConfigKey:          rec.ConfigKey,
		ResolvedPath:       rec.ResolvedPath,
		Format:             rec.Format,
		ManagedValues:      values,
		FileContentHash:    rec.FileContentHash,
		ExistsOnNode:       rec.ExistsOnNode,
		Manageable:         rec.Manageable,
		UnmanageableReason: rec.UnmanageableReason,
		Source:             rec.Source,
		SnapshotAt:         rec.SnapshotAt,
		StaleHint:          true, // snapshot path is always non-live
		NodeOnline:         online,
		LastPulledAt:       rec.LastPulledAt,
		LastPushedAt:       rec.LastPushedAt,
	}, nil
}

// PullProviderNativeConfig reads live config from the node and upserts snapshot.
func (s *Service) PullProviderNativeConfig(ctx context.Context, tenantID, actorID uuid.UUID, nodeID, providerType, configKey string) (*ProviderNativeConfigDetail, error) {
	if err := validateNativeConfigSurface(providerType, configKey); err != nil {
		return nil, err
	}
	node, err := s.requireNodeForTenant(ctx, tenantID, nodeID)
	if err != nil {
		return nil, err
	}
	if s.nativeConfigCommander == nil || !s.nativeConfigCommander.IsConnected(nodeID) {
		return nil, ErrProviderNativeConfigOffline
	}

	payload := map[string]any{
		"provider_type": providerType,
		"config_key":    configKey,
	}
	receipt, err := s.dispatchNativeConfigCommand(ctx, tenantID, node, runtimeCommandReadProviderNativeConfig, payload)
	if err != nil {
		return nil, err
	}
	if receipt.Status != "completed" {
		return nil, mapNativeConfigCommandError(receipt)
	}
	// The writeback path keeps the receipt terminal even when the snapshot upsert
	// fails, so waiters unblock. A pull whose whole purpose is fresh values has
	// nothing to return in that case — the receipt carries no managed_values
	// (stripped before persist), so we cannot recover them here.
	if snapErr := stringFromResult(receipt.Result, "snapshot_error"); snapErr != "" {
		return nil, fmt.Errorf("%w: 快照落库失败，未取到节点最新值: %s", ErrProviderNativeConfigCommand, snapErr)
	}

	// Writeback hook may have already upserted; re-load or upsert from receipt transit fields.
	// Prefer re-loading snapshot after writeback; if missing, upsert from result here.
	detail, err := s.GetProviderNativeConfigSnapshot(ctx, tenantID, nodeID, providerType, configKey)
	if err == nil {
		detail.StaleHint = false
		_ = actorID
		s.recordNativeConfigEventBestEffort(ctx, tenantID, node, providerType, configKey, RuntimeEventProviderNativeConfigPull, RuntimeEventSeveritySuccess, "从节点拉取 Provider 原生配置", map[string]any{
			"managed_key_names": receiptKeyNames(receipt.Result),
			"file_content_hash": stringFromResult(receipt.Result, "file_content_hash"),
			"source":            "pulled",
		})
		return detail, nil
	}

	// Fallback: apply from receipt if writeback didn't persist (e.g. no managed_values in receipt after strip).
	return nil, fmt.Errorf("%w: snapshot missing after pull", ErrProviderNativeConfigCommand)
}

// PushProviderNativeConfig writes managed keys to the node and updates snapshot.
func (s *Service) PushProviderNativeConfig(ctx context.Context, tenantID, actorID uuid.UUID, nodeID, providerType, configKey string, values map[string]any, expectedHash string) (*ProviderNativeConfigDetail, error) {
	if err := validateNativeConfigSurface(providerType, configKey); err != nil {
		return nil, err
	}
	// v1: claude-code auth is never file-manageable (keychain / OAuth session).
	if providerType == "claude-code" && configKey == "auth" {
		return nil, fmt.Errorf("%w: platform_keychain or oauth_session_protected", ErrProviderNativeConfigUnmanageable)
	}
	if strings.TrimSpace(expectedHash) == "" {
		return nil, fmt.Errorf("%w: expected_file_content_hash is required", ErrProviderNativeConfigValidation)
	}
	if err := validatePushKeys(providerType, configKey, values); err != nil {
		return nil, err
	}
	node, err := s.requireNodeForTenant(ctx, tenantID, nodeID)
	if err != nil {
		return nil, err
	}
	if s.nativeConfigCommander == nil || !s.nativeConfigCommander.IsConnected(nodeID) {
		return nil, ErrProviderNativeConfigOffline
	}

	payload := map[string]any{
		"provider_type":              providerType,
		"config_key":                 configKey,
		"values":                     values,
		"expected_file_content_hash": expectedHash,
	}
	receipt, err := s.dispatchNativeConfigCommand(ctx, tenantID, node, runtimeCommandWriteProviderNativeConfig, payload)
	if err != nil {
		return nil, err
	}
	if receipt.Status != "completed" {
		return nil, mapNativeConfigCommandError(receipt)
	}

	// Update last_pushed_by on the snapshot if present.
	repo, ok := s.repository.(ProviderNativeConfigRepository)
	if ok {
		if rec, getErr := repo.GetProviderNativeConfig(ctx, tenantID, nodeID, providerType, configKey); getErr == nil {
			now := time.Now().UTC()
			rec.LastPushedAt = &now
			rec.LastPushedBy = &actorID
			rec.Source = "pushed"
			rec.SnapshotAt = now
			_, _ = repo.UpsertProviderNativeConfig(ctx, upsertParamsFromRecord(rec))
		}
	}

	detail, err := s.GetProviderNativeConfigSnapshot(ctx, tenantID, nodeID, providerType, configKey)
	if err != nil {
		return nil, fmt.Errorf("%w: snapshot missing after push", ErrProviderNativeConfigCommand)
	}
	// Unlike pull, the node write already landed — failing the call would misreport
	// it as not applied. Report success but keep the values marked non-live so the
	// UI does not present the previous snapshot as the node's current state.
	if snapErr := stringFromResult(receipt.Result, "snapshot_error"); snapErr != "" {
		detail.StaleHint = true
		detail.SnapshotError = snapErr
	} else {
		detail.StaleHint = false
	}
	changedKeys := stringSliceFromResult(receipt.Result, "changed_keys")
	if len(changedKeys) == 0 {
		for k := range values {
			changedKeys = append(changedKeys, k)
		}
	}
	s.recordNativeConfigEventBestEffort(ctx, tenantID, node, providerType, configKey, RuntimeEventProviderNativeConfigPush, RuntimeEventSeveritySuccess, "下发 Provider 原生配置", map[string]any{
		"changed_keys":      changedKeys,
		"file_content_hash": stringFromResult(receipt.Result, "file_content_hash"),
		"actor_user_id":     actorID.String(),
		"source":            "pushed",
	})
	return detail, nil
}

// ApplyNativeConfigWriteback upserts encrypted snapshot from a command terminal result.
// managed_values must be present in result for successful reads/writes; they are stripped from the stored receipt by the writeback path.
func (s *Service) ApplyNativeConfigWriteback(ctx context.Context, tenantID uuid.UUID, runtimeNodeID uuid.UUID, nodeID string, commandType string, result map[string]any, success bool) error {
	if !success || result == nil {
		return nil
	}
	providerType := stringFromResult(result, "provider_type")
	configKey := stringFromResult(result, "config_key")
	if providerType == "" || configKey == "" {
		return nil
	}
	managedValues := filterAllowlistedManagedValues(providerType, configKey, mapFromResult(result, "managed_values"))
	sealed, err := s.sealManagedValues(providerType, configKey, managedValues)
	if err != nil {
		return err
	}
	repo, ok := s.repository.(ProviderNativeConfigRepository)
	if !ok {
		return errors.New("provider native config repository is required")
	}
	now := time.Now().UTC()
	source := "pulled"
	var lastPulled, lastPushed *time.Time
	if commandType == runtimeCommandWriteProviderNativeConfig {
		source = "pushed"
		lastPushed = &now
	} else {
		lastPulled = &now
	}
	format := stringFromResult(result, "format")
	if format == "" {
		format = "json"
	}
	params := UpsertProviderNativeConfigParams{
		TenantID:           tenantID,
		RuntimeNodeID:      runtimeNodeID,
		NodeID:             nodeID,
		ProviderType:       providerType,
		ConfigKey:          configKey,
		ResolvedPath:       stringFromResult(result, "resolved_path"),
		Format:             format,
		ManagedValues:      sealed,
		FileContentHash:    stringFromResult(result, "file_content_hash"),
		ExistsOnNode:       boolFromResult(result, "exists"),
		Manageable:         boolFromResultDefault(result, "manageable", true),
		UnmanageableReason: stringFromResult(result, "unmanageable_reason"),
		Source:             source,
		SnapshotAt:         now,
		LastPulledAt:       lastPulled,
		LastPushedAt:       lastPushed,
	}
	if mtime := stringFromResult(result, "node_mtime"); mtime != "" {
		if t, parseErr := time.Parse(time.RFC3339, mtime); parseErr == nil {
			params.NodeMtime = &t
		}
	}
	_, err = repo.UpsertProviderNativeConfig(ctx, params)
	return err
}

func (s *Service) requireNodeForTenant(ctx context.Context, tenantID uuid.UUID, nodeID string) (*Node, error) {
	record, err := s.repository.GetNode(ctx, nodeID)
	if err != nil {
		return nil, ErrNodeNotFound
	}
	if record.TenantID != tenantID {
		return nil, ErrNodeNotFound
	}
	return s.recordToNode(ctx, record)
}

func (s *Service) dispatchNativeConfigCommand(
	ctx context.Context,
	tenantID uuid.UUID,
	node *Node,
	commandType string,
	payload map[string]any,
) (*NativeConfigCommandReceipt, error) {
	commandID := uuid.NewString()
	now := time.Now().UTC()
	if err := s.nativeConfigCommander.CreateCommandReceipt(ctx, NativeConfigCommandReceiptRequest{
		TenantID:      tenantID,
		CommandID:     commandID,
		CommandType:   commandType,
		RuntimeNodeID: node.ID,
		NodeID:        node.NodeID,
		ResourceType:  providerNativeConfigResourceType,
		ResourceID:    node.ID,
		Status:        "pending",
		Payload:       payload,
		DispatchedAt:  &now,
	}); err != nil {
		return nil, fmt.Errorf("create command receipt: %w", err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if err := s.nativeConfigCommander.Dispatch(ctx, node.NodeID, RuntimeCommand{
		ID:      commandID,
		Type:    commandType,
		Payload: raw,
	}); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProviderNativeConfigOffline, err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, defaultProviderNativeConfigTimeout)
	defer cancel()
	receipt, err := s.nativeConfigCommander.WaitForCommandCompletion(waitCtx, tenantID, commandID, defaultProviderNativeConfigPoll)
	if err != nil {
		// Best-effort: mark stuck pending receipts so operators do not see eternal "pending".
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			s.markNativeConfigCommandTimedOutBestEffort(ctx, tenantID, commandID, err)
		}
		return nil, fmt.Errorf("%w: %v", ErrProviderNativeConfigCommand, err)
	}
	if receipt == nil {
		return nil, fmt.Errorf("%w: empty receipt", ErrProviderNativeConfigCommand)
	}
	return receipt, nil
}

func (s *Service) markNativeConfigCommandTimedOutBestEffort(ctx context.Context, tenantID uuid.UUID, commandID string, waitErr error) {
	if s.nativeConfigCommander == nil {
		return
	}
	msg := "provider native config command wait timed out"
	if waitErr != nil {
		msg = fmt.Sprintf("%s: %v", msg, waitErr)
	}
	_ = s.nativeConfigCommander.MarkCommandTimedOut(ctx, tenantID, commandID, msg)
}

func (s *Service) sealManagedValues(providerType, configKey string, values map[string]any) (map[string]any, error) {
	if values == nil {
		return map[string]any{}, nil
	}
	out := make(map[string]any, len(values))
	for k, v := range values {
		if isSensitiveManagedKey(providerType, configKey, k) {
			// Seal the JSON encoding, not the bare string, so the original type
			// survives the round trip. Sealing `stringifyManagedValue` loses it:
			// a token that happens to look like a JSON scalar ("123456", "true",
			// "null") comes back as a number/bool/nil and would be written back
			// into the provider config with the wrong type.
			plain := jsonEncodeManagedValue(v)
			if s.sealer == nil {
				return nil, errors.New("credential sealer is required for sensitive managed values")
			}
			sealed, err := s.sealer.Seal(plain)
			if err != nil {
				return nil, err
			}
			out[k] = sealed
			continue
		}
		out[k] = v
	}
	return out, nil
}

func (s *Service) openManagedValues(values map[string]any) (map[string]any, error) {
	if values == nil {
		return map[string]any{}, nil
	}
	out := make(map[string]any, len(values))
	for k, v := range values {
		str, ok := v.(string)
		if ok && strings.HasPrefix(str, sensitiveSealedPrefix) {
			if s.sealer == nil {
				return nil, errors.New("credential sealer is required to open sealed managed values")
			}
			plain, err := s.sealer.Open(str)
			if err != nil {
				return nil, err
			}
			// Current rows hold a JSON encoding (see sealManagedValues), so this
			// restores the exact original type. Rows sealed before that change hold
			// a bare string; those fail to decode and fall back to the raw text.
			// (A legacy row whose value was a numeric-looking string still decodes
			// as a number — it self-heals on the next pull, which reseals it.)
			var decoded any
			if json.Unmarshal([]byte(plain), &decoded) == nil {
				out[k] = decoded
			} else {
				out[k] = plain
			}
			continue
		}
		out[k] = v
	}
	return out, nil
}

func (s *Service) recordNativeConfigEventBestEffort(
	ctx context.Context,
	tenantID uuid.UUID,
	node *Node,
	providerType, configKey string,
	eventType RuntimeEventType,
	severity RuntimeEventSeverity,
	title string,
	payload map[string]any,
) {
	s.recordRuntimeEventBestEffort(ctx, CreateRuntimeEventRequest{
		TenantID:        tenantID,
		RuntimeNodeID:   node.ID,
		NodeID:          node.NodeID,
		EventType:       eventType,
		Severity:        severity,
		Source:          RuntimeEventSourceProviderNativeConfig,
		Title:           title,
		ProviderType:    providerType,
		CorrelationType: "provider_native_config",
		CorrelationID:   providerType + "/" + configKey,
		Payload:         payload,
	})
}

// StripManagedValuesFromResult removes managed_values from a receipt result map (in place).
func StripManagedValuesFromResult(result map[string]any) map[string]any {
	if result == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(result))
	for k, v := range result {
		if k == "managed_values" {
			continue
		}
		out[k] = v
	}
	return out
}

type ProviderNativeConfigListItem struct {
	ProviderType       string     `json:"provider_type"`
	ConfigKey          string     `json:"config_key"`
	ResolvedPath       string     `json:"resolved_path,omitempty"`
	Format             string     `json:"format,omitempty"`
	FileContentHash    string     `json:"file_content_hash,omitempty"`
	ExistsOnNode       bool       `json:"exists_on_node"`
	Manageable         bool       `json:"manageable"`
	UnmanageableReason string     `json:"unmanageable_reason,omitempty"`
	Source             string     `json:"source,omitempty"`
	SnapshotAt         *time.Time `json:"snapshot_at,omitempty"`
	LastPulledAt       *time.Time `json:"last_pulled_at,omitempty"`
	LastPushedAt       *time.Time `json:"last_pushed_at,omitempty"`
	NodeOnline         bool       `json:"node_online"`
}

type ProviderNativeConfigDetail struct {
	ProviderType       string         `json:"provider_type"`
	ConfigKey          string         `json:"config_key"`
	ResolvedPath       string         `json:"resolved_path,omitempty"`
	Format             string         `json:"format,omitempty"`
	ManagedValues      map[string]any `json:"managed_values"`
	FileContentHash    string         `json:"file_content_hash,omitempty"`
	ExistsOnNode       bool           `json:"exists_on_node"`
	Manageable         bool           `json:"manageable"`
	UnmanageableReason string         `json:"unmanageable_reason,omitempty"`
	Source             string         `json:"source,omitempty"`
	SnapshotAt         time.Time      `json:"snapshot_at,omitempty"`
	StaleHint          bool           `json:"stale_hint"`
	NodeOnline         bool           `json:"node_online"`
	LastPulledAt       *time.Time     `json:"last_pulled_at,omitempty"`
	LastPushedAt       *time.Time     `json:"last_pushed_at,omitempty"`
	// SensitiveKeys names the managed keys the client must mask. Served from the
	// same predicate that decides encryption at rest, so the UI never has to
	// re-derive sensitivity from key names (which drifts — `options.apiKey` was
	// missed by exactly that kind of heuristic).
	SensitiveKeys []string `json:"sensitive_keys,omitempty"`
	// SnapshotError is set when the node command succeeded but persisting the
	// snapshot failed. The values shown are then the previous snapshot, not the
	// node's current state — callers must not present them as live.
	SnapshotError string `json:"snapshot_error,omitempty"`
}

type nativeSurface struct {
	providerType string
	configKey    string
	format       string
}

func knownNativeConfigSurfaces() []nativeSurface {
	return []nativeSurface{
		{"claude-code", "model_profile", "json"},
		{"claude-code", "auth", "json"},
		{"codex", "model_profile", "toml"},
		{"codex", "auth", "json"},
		{"opencode", "model_profile", "json"},
		{"opencode", "auth", "json"},
	}
}

// defaultSurfaceManageability is the CP-side default before a node pull reports
// platform-specific facts. Claude Code auth is never file-writable in v1
// (macOS keychain / Linux OAuth session); use oauth_session_protected as the
// stable pre-pull reason (node pull may refine to platform_keychain).
func defaultSurfaceManageability(providerType, configKey string) (manageable bool, reason string) {
	if providerType == "claude-code" && configKey == "auth" {
		return false, "oauth_session_protected"
	}
	return true, ""
}

func validateNativeConfigSurface(providerType, configKey string) error {
	for _, s := range knownNativeConfigSurfaces() {
		if s.providerType == providerType && s.configKey == configKey {
			return nil
		}
	}
	return fmt.Errorf("%w: unsupported provider_type/config_key", ErrProviderNativeConfigValidation)
}

func validatePushKeys(providerType, configKey string, values map[string]any) error {
	if values == nil {
		return fmt.Errorf("%w: values is required", ErrProviderNativeConfigValidation)
	}
	for key := range values {
		if !isManagedKeyAllowlisted(providerType, configKey, key) {
			return fmt.Errorf("%w: key not in allowlist: %s", ErrProviderNativeConfigValidation, key)
		}
	}
	return nil
}

func isManagedKeyAllowlisted(providerType, configKey, key string) bool {
	switch {
	case providerType == "claude-code" && configKey == "model_profile":
		// `apiKeyHelper` is deliberately absent: it is a shell command Claude Code
		// executes to mint auth values (design §5.3).
		if key == "model" || key == "fallbackModel" {
			return true
		}
		if strings.HasPrefix(key, "env.") {
			sub := strings.TrimPrefix(key, "env.")
			switch sub {
			case "ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY", "ANTHROPIC_MODEL", "ANTHROPIC_SMALL_FAST_MODEL":
				return true
			}
		}
		return false
	case providerType == "claude-code" && configKey == "auth":
		return false
	case providerType == "codex" && configKey == "model_profile":
		if key == "model" || key == "model_provider" {
			return true
		}
		if strings.HasPrefix(key, "model_providers.") {
			parts := strings.SplitN(strings.TrimPrefix(key, "model_providers."), ".", 2)
			if len(parts) == 2 {
				switch parts[1] {
				case "name", "base_url", "wire_api", "env_key", "requires_openai_auth", "experimental_bearer_token", "query_params", "http_headers":
					return true
				}
			}
		}
		return false
	case configKey == "auth" && (providerType == "codex" || providerType == "opencode"):
		return key != "" && !strings.Contains(key, "..") && !strings.HasPrefix(key, "/")
	case providerType == "opencode" && configKey == "model_profile":
		if key == "model" || key == "small_model" {
			return true
		}
		if strings.HasPrefix(key, "provider.") {
			return isOpencodeProviderKeyAllowlisted(strings.TrimPrefix(key, "provider."))
		}
		return false
	default:
		return false
	}
}

// isOpencodeProviderKeyAllowlisted allows data-only fields under provider.<name>.
//
// `npm` names an npm package that opencode loads to talk to the provider, and
// `models.<id>.provider.*` can override it per model — both are code-loading
// surfaces and stay out of the allowlist (design §5.3). A bare provider.<name>
// write is rejected because the object could carry `npm`.
func isOpencodeProviderKeyAllowlisted(rest string) bool {
	idx := strings.Index(rest, ".")
	if idx < 0 {
		return false
	}
	path := rest[idx+1:]
	if path == "name" {
		return true
	}
	if strings.HasPrefix(path, "options.") && len(path) > len("options.") {
		return true
	}
	if strings.HasPrefix(path, "models.") {
		modelRest := strings.TrimPrefix(path, "models.")
		j := strings.Index(modelRest, ".")
		if j < 0 {
			return false
		}
		field := modelRest[j+1:]
		if field == "name" {
			return true
		}
		return strings.HasPrefix(field, "limit.") && len(field) > len("limit.")
	}
	return false
}

// filterAllowlistedManagedValues drops keys that are no longer in the allowlist —
// e.g. snapshots pulled before an allowlist narrowing. Without this, a stale
// `apiKeyHelper` / `provider.*.npm` would still render as an editable field and be
// replayed into the next push payload, failing the whole save with 422.
//
// `auth` surfaces are excluded: they are single-purpose files whose read set is the
// whole file, which is deliberately wider than their write set (claude-code auth is
// readable but never writable — §5.5).
func filterAllowlistedManagedValues(providerType, configKey string, values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	if configKey == "auth" {
		return values
	}
	out := make(map[string]any, len(values))
	for k, v := range values {
		if isManagedKeyAllowlisted(providerType, configKey, k) {
			out[k] = v
		}
	}
	return out
}

// authHeaderNameHints match header/param names that customarily carry credentials.
var authHeaderNameHints = []string{"authorization", "api-key", "apikey", "api_key", "token", "secret"}

func isSensitiveManagedKey(providerType, configKey, key string) bool {
	if configKey == "auth" {
		return true
	}
	if strings.HasPrefix(key, "env.ANTHROPIC_AUTH_TOKEN") || strings.HasPrefix(key, "env.ANTHROPIC_API_KEY") {
		return true
	}
	if strings.Contains(key, "experimental_bearer_token") {
		return true
	}
	// opencode: provider.<name>.options.apiKey is a credential like any other.
	if strings.HasSuffix(key, ".options.apiKey") {
		return true
	}
	// codex writes http_headers / query_params as whole blobs; either can carry a
	// key, and we cannot inspect the value here — treat the blob as sensitive.
	if strings.HasSuffix(key, ".http_headers") || strings.HasSuffix(key, ".query_params") {
		return true
	}
	// opencode: individual provider headers, e.g. provider.x.options.headers.Authorization.
	if idx := strings.LastIndex(key, ".options.headers."); idx >= 0 {
		name := strings.ToLower(key[idx+len(".options.headers."):])
		for _, hint := range authHeaderNameHints {
			if strings.Contains(name, hint) {
				return true
			}
		}
	}
	_ = providerType
	return false
}

// jsonEncodeManagedValue encodes any managed value as JSON so seal/open is
// type-preserving. Strings become quoted JSON strings — that quoting is what
// distinguishes the token "123456" from the number 123456 on the way back.
func jsonEncodeManagedValue(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

func mapNativeConfigCommandError(receipt *NativeConfigCommandReceipt) error {
	code := ""
	if receipt.Result != nil {
		if diagnostic, ok := receipt.Result["diagnostic"].(map[string]any); ok {
			if c, ok := diagnostic["error_code"].(string); ok {
				code = c
			}
		}
		if code == "" {
			if c, ok := receipt.Result["error_code"].(string); ok {
				code = c
			}
		}
	}
	msg := ""
	if receipt.ErrorMessage != nil {
		msg = *receipt.ErrorMessage
	}
	switch code {
	case "conflict":
		return fmt.Errorf("%w: %s", ErrProviderNativeConfigConflict, msg)
	case "validation_error":
		return fmt.Errorf("%w: %s", ErrProviderNativeConfigValidation, msg)
	case "unmanageable":
		return fmt.Errorf("%w: %s", ErrProviderNativeConfigUnmanageable, msg)
	default:
		if msg == "" {
			msg = "command status=" + receipt.Status
		}
		return fmt.Errorf("%w: %s", ErrProviderNativeConfigCommand, msg)
	}
}

func stringFromResult(result map[string]any, key string) string {
	if result == nil {
		return ""
	}
	if v, ok := result[key].(string); ok {
		return v
	}
	return ""
}

func boolFromResult(result map[string]any, key string) bool {
	if result == nil {
		return false
	}
	if v, ok := result[key].(bool); ok {
		return v
	}
	return false
}

func boolFromResultDefault(result map[string]any, key string, def bool) bool {
	if result == nil {
		return def
	}
	if v, ok := result[key].(bool); ok {
		return v
	}
	return def
}

func mapFromResult(result map[string]any, key string) map[string]any {
	if result == nil {
		return map[string]any{}
	}
	if v, ok := result[key].(map[string]any); ok {
		return v
	}
	return map[string]any{}
}

func stringSliceFromResult(result map[string]any, key string) []string {
	if result == nil {
		return nil
	}
	raw, ok := result[key].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func receiptKeyNames(result map[string]any) []string {
	if names := stringSliceFromResult(result, "managed_key_names"); len(names) > 0 {
		return names
	}
	return stringSliceFromResult(result, "changed_keys")
}

func upsertParamsFromRecord(rec ProviderNativeConfigRecord) UpsertProviderNativeConfigParams {
	return UpsertProviderNativeConfigParams{
		TenantID:           rec.TenantID,
		RuntimeNodeID:      rec.RuntimeNodeID,
		NodeID:             rec.NodeID,
		ProviderType:       rec.ProviderType,
		ConfigKey:          rec.ConfigKey,
		ResolvedPath:       rec.ResolvedPath,
		Format:             rec.Format,
		ManagedValues:      rec.ManagedValues,
		FileContentHash:    rec.FileContentHash,
		ExistsOnNode:       rec.ExistsOnNode,
		Manageable:         rec.Manageable,
		UnmanageableReason: rec.UnmanageableReason,
		Source:             rec.Source,
		NodeMtime:          rec.NodeMtime,
		SnapshotAt:         rec.SnapshotAt,
		LastPulledAt:       rec.LastPulledAt,
		LastPushedAt:       rec.LastPushedAt,
		LastPushedBy:       rec.LastPushedBy,
	}
}

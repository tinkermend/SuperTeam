package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/superteam/control-plane/internal/platform"
)

func TestEnrollHelloCreatesPendingWithoutSession(t *testing.T) {
	ctx := context.Background()
	repo := newEnrollmentFake(t)
	service, err := NewService(repo)
	require.NoError(t, err)

	bootstrapSecret := "boot_test_pending"
	bootstrapHash, err := HashRuntimeSecret(bootstrapSecret)
	require.NoError(t, err)
	repo.bootstrapKeys = append(repo.bootstrapKeys, RuntimeBootstrapKeyRecord{
		ID:       runtimeTestUUID(10),
		TenantID: platform.DefaultTenantID,
		KeyHash:  bootstrapHash,
		Status:   "active",
	})

	resp, err := service.EnrollHello(ctx, EnrollHelloRequest{
		NodeID:             "runtime-dev-1",
		Name:               "Runtime Dev 1",
		BootstrapKey:       bootstrapSecret,
		SupportedProviders: []string{"claude-code", "codex"},
		MaxSlots:           3,
		Metadata:           map[string]interface{}{"region": "local"},
		Version:            "0.1.0",
	})
	require.NoError(t, err)
	require.Equal(t, RuntimeEnrollmentStatusPending, resp.Enrollment.Status)
	require.Equal(t, "runtime-dev-1", resp.Enrollment.NodeID)
	require.Nil(t, resp.Session)
	require.Empty(t, resp.SessionToken)
	require.Empty(t, repo.nodes)
	require.Len(t, repo.enrollmentsByNode, 1)
}

func TestEnrollHelloApprovedIssuesSession(t *testing.T) {
	ctx := context.Background()
	repo := newEnrollmentFake(t)
	service, err := NewService(repo)
	require.NoError(t, err)

	bootstrapSecret := "boot_test_approved"
	bootstrapHash, err := HashRuntimeSecret(bootstrapSecret)
	require.NoError(t, err)
	node := repo.seedNode(platform.DefaultTenantID, "runtime-approved", "Runtime Approved", []string{"codex"})
	enrollment := repo.seedEnrollment(platform.DefaultTenantID, "runtime-approved", RuntimeEnrollmentStatusApproved, node.ID, runtimeTestUUID(20), bootstrapHash)

	issuedAt := time.Now()
	resp, err := service.EnrollHello(ctx, EnrollHelloRequest{
		NodeID:             "runtime-approved",
		Name:               "Runtime Approved",
		BootstrapKey:       bootstrapSecret,
		SupportedProviders: []string{"codex"},
		MaxSlots:           2,
	})
	require.NoError(t, err)
	require.Equal(t, RuntimeEnrollmentStatusApproved, resp.Enrollment.Status)
	require.NotNil(t, resp.Session)
	require.NotEmpty(t, resp.SessionToken)
	require.Equal(t, enrollment.ID, resp.Session.EnrollmentID.UUID)
	requireRuntimeSessionExpiresNear(t, issuedAt, resp.Session.ExpiresAt)
	require.Contains(t, repo.sessionsByLookup, LookupRuntimeSessionTokenHash(resp.SessionToken))
}

func TestEnrollHelloRejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	repo := newEnrollmentFake(t)
	service, err := NewService(repo)
	require.NoError(t, err)

	for _, tt := range []struct {
		name string
		req  EnrollHelloRequest
		err  string
	}{
		{name: "empty node id", req: EnrollHelloRequest{BootstrapKey: "boot"}, err: "node_id is required"},
		{name: "empty bootstrap key", req: EnrollHelloRequest{NodeID: "runtime"}, err: "bootstrap_key is required"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := service.EnrollHello(ctx, tt.req)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.err)
		})
	}
}

func TestEnrollHelloRejectsInvalidBootstrapSecret(t *testing.T) {
	ctx := context.Background()
	repo := newEnrollmentFake(t)
	service, err := NewService(repo)
	require.NoError(t, err)

	bootstrapHash, err := HashRuntimeSecret("actual_bootstrap_secret")
	require.NoError(t, err)
	repo.bootstrapKeys = append(repo.bootstrapKeys, RuntimeBootstrapKeyRecord{
		ID:       runtimeTestUUID(30),
		TenantID: platform.DefaultTenantID,
		KeyHash:  bootstrapHash,
		Status:   "active",
	})

	_, err = service.EnrollHello(ctx, EnrollHelloRequest{
		NodeID:       "runtime-invalid-secret",
		Name:         "Runtime Invalid Secret",
		BootstrapKey: "wrong_secret",
		MaxSlots:     1,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid bootstrap key")
	require.Empty(t, repo.enrollmentsByNode)
}

func TestEnrollHelloRejectedOrRevokedDoesNotIssueSession(t *testing.T) {
	for _, status := range []RuntimeEnrollmentStatus{RuntimeEnrollmentStatusRejected, RuntimeEnrollmentStatusRevoked} {
		t.Run(string(status), func(t *testing.T) {
			ctx := context.Background()
			repo := newEnrollmentFake(t)
			service, err := NewService(repo)
			require.NoError(t, err)

			bootstrapSecret := "boot_test_terminal"
			bootstrapHash, err := HashRuntimeSecret(bootstrapSecret)
			require.NoError(t, err)
			repo.seedEnrollment(platform.DefaultTenantID, "runtime-terminal", status, uuid.Nil, runtimeTestUUID(40), bootstrapHash)

			resp, err := service.EnrollHello(ctx, EnrollHelloRequest{
				NodeID:       "runtime-terminal",
				Name:         "Runtime Terminal",
				BootstrapKey: bootstrapSecret,
				MaxSlots:     1,
			})
			require.NoError(t, err)
			require.Equal(t, status, resp.Enrollment.Status)
			require.Nil(t, resp.Session)
			require.Empty(t, resp.SessionToken)
			require.Empty(t, repo.nodes)
		})
	}
}

func TestApproveEnrollmentCreatesOrReusesRuntimeNode(t *testing.T) {
	ctx := context.Background()
	repo := newEnrollmentFake(t)
	service, err := NewService(repo)
	require.NoError(t, err)

	bootstrapHash, err := HashRuntimeSecret("boot_approve")
	require.NoError(t, err)
	enrollment := repo.seedEnrollment(platform.DefaultTenantID, "runtime-approve", RuntimeEnrollmentStatusPending, uuid.Nil, runtimeTestUUID(50), bootstrapHash)
	enrollment.RequestPayload = mustRuntimePayload(t, "runtime-approve", "Runtime Approve", []string{"codex"}, 4, map[string]interface{}{"region": "local"})
	repo.enrollmentsByID[enrollment.ID] = enrollment
	repo.enrollmentsByNode[repo.enrollmentKey(platform.DefaultTenantID, "runtime-approve")] = enrollment

	approved, err := service.ApproveEnrollment(ctx, ApproveEnrollmentRequest{
		TenantID:     platform.DefaultTenantID,
		EnrollmentID: enrollment.ID,
		ApprovedBy:   runtimeTestUUID(51),
	})
	require.NoError(t, err)
	require.Equal(t, RuntimeEnrollmentStatusApproved, approved.Status)
	require.NotEqual(t, uuid.Nil, approved.RuntimeNodeID)
	require.Contains(t, repo.nodes, repo.nodeKey(platform.DefaultTenantID, "runtime-approve"))

	// 审批不得伪造活性：新建节点必须是 offline 且没有心跳，
	// 在线状态只能由 agent 自己的心跳建立。
	created := repo.nodes[repo.nodeKey(platform.DefaultTenantID, "runtime-approve")]
	require.Equal(t, string(NodeStatusOffline), created.Status)
	require.False(t, created.LastHeartbeatAt.Valid)

	// Re-approving another pending enrollment with the same tenant/node should reuse the same runtime node.
	firstNodeID := approved.RuntimeNodeID
	// 给已有节点一个真实活性状态，审批复用时必须原样保留
	liveHeartbeat := timestamptzFromTime(time.Now())
	existingNode := repo.nodes[repo.nodeKey(platform.DefaultTenantID, "runtime-approve")]
	existingNode.Status = string(NodeStatusOnline)
	existingNode.LastHeartbeatAt = liveHeartbeat
	repo.nodes[repo.nodeKey(platform.DefaultTenantID, "runtime-approve")] = existingNode
	second := repo.seedEnrollment(platform.DefaultTenantID, "runtime-approve", RuntimeEnrollmentStatusPending, uuid.Nil, runtimeTestUUID(52), bootstrapHash)
	second.RequestPayload = mustRuntimePayload(t, "runtime-approve", "Runtime Approve Reused", []string{"codex"}, 2, nil)
	repo.enrollmentsByID[second.ID] = second
	repo.enrollmentsByNode[repo.enrollmentKey(platform.DefaultTenantID, "runtime-approve")] = second
	approvedAgain, err := service.ApproveEnrollment(ctx, ApproveEnrollmentRequest{
		TenantID:     platform.DefaultTenantID,
		EnrollmentID: second.ID,
		ApprovedBy:   runtimeTestUUID(53),
	})
	require.NoError(t, err)
	require.Equal(t, firstNodeID, approvedAgain.RuntimeNodeID)
	reused := repo.nodes[repo.nodeKey(platform.DefaultTenantID, "runtime-approve")]
	require.Equal(t, string(NodeStatusOnline), reused.Status)
	require.Equal(t, liveHeartbeat, reused.LastHeartbeatAt)
}

func TestApproveEnrollmentDoesNotTakeOverCrossTenantNodeID(t *testing.T) {
	ctx := context.Background()
	otherTenantID := runtimeTestUUID(910)
	repo := newEnrollmentFake(t)
	service, err := NewService(repo)
	require.NoError(t, err)

	bootstrapHash, err := HashRuntimeSecret("boot_cross_tenant")
	require.NoError(t, err)
	repo.seedNode(platform.DefaultTenantID, "runtime-shared", "Default Tenant Runtime", []string{"codex"})
	enrollment := repo.seedEnrollment(otherTenantID, "runtime-shared", RuntimeEnrollmentStatusPending, uuid.Nil, runtimeTestUUID(911), bootstrapHash)
	enrollment.RequestPayload = mustRuntimePayload(t, "runtime-shared", "Other Tenant Runtime", []string{"codex"}, 2, nil)
	repo.enrollmentsByID[enrollment.ID] = enrollment
	repo.enrollmentsByNode[repo.enrollmentKey(otherTenantID, "runtime-shared")] = enrollment

	_, err = service.ApproveEnrollment(ctx, ApproveEnrollmentRequest{
		TenantID:     otherTenantID,
		EnrollmentID: enrollment.ID,
		ApprovedBy:   runtimeTestUUID(912),
	})
	require.Error(t, err)
	require.Equal(t, RuntimeEnrollmentStatusPending, repo.enrollmentsByID[enrollment.ID].Status)
	require.Equal(t, 1, len(repo.nodes))
	require.Equal(t, platform.DefaultTenantID, repo.nodes["runtime-shared"].TenantID)
}

func TestApproveEnrollmentFailureDoesNotCreateRuntimeNode(t *testing.T) {
	ctx := context.Background()
	repo := newEnrollmentFake(t)
	service, err := NewService(repo)
	require.NoError(t, err)

	bootstrapHash, err := HashRuntimeSecret("boot_rejected_after_read")
	require.NoError(t, err)
	enrollment := repo.seedEnrollment(platform.DefaultTenantID, "runtime-rejected-after-read", RuntimeEnrollmentStatusPending, uuid.Nil, runtimeTestUUID(913), bootstrapHash)
	enrollment.RequestPayload = mustRuntimePayload(t, "runtime-rejected-after-read", "Runtime Rejected After Read", []string{"codex"}, 2, nil)
	repo.enrollmentsByID[enrollment.ID] = enrollment
	repo.enrollmentsByNode[repo.enrollmentKey(platform.DefaultTenantID, "runtime-rejected-after-read")] = enrollment
	repo.failApproveEnrollmentIDs[enrollment.ID] = true

	_, err = service.ApproveEnrollment(ctx, ApproveEnrollmentRequest{
		TenantID:     platform.DefaultTenantID,
		EnrollmentID: enrollment.ID,
		ApprovedBy:   runtimeTestUUID(914),
	})
	require.Error(t, err)
	require.Empty(t, repo.nodes)
}

func TestNonDefaultTenantEnrollmentSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	tenantID := runtimeTestUUID(900)
	repo := newEnrollmentFake(t)
	service, err := NewService(repo)
	require.NoError(t, err)

	bootstrapSecret := "boot_non_default"
	bootstrapHash, err := HashRuntimeSecret(bootstrapSecret)
	require.NoError(t, err)
	repo.bootstrapKeys = append(repo.bootstrapKeys, RuntimeBootstrapKeyRecord{
		ID:       runtimeTestUUID(901),
		TenantID: tenantID,
		KeyHash:  bootstrapHash,
		Status:   "active",
	})

	pending, err := service.EnrollHello(ctx, EnrollHelloRequest{
		TenantID:           tenantID,
		NodeID:             "tenant-runtime",
		Name:               "Tenant Runtime",
		BootstrapKey:       bootstrapSecret,
		SupportedProviders: []string{"codex"},
		MaxSlots:           5,
		Metadata:           map[string]interface{}{"zone": "tenant-a"},
	})
	require.NoError(t, err)
	require.Equal(t, RuntimeEnrollmentStatusPending, pending.Enrollment.Status)
	require.Equal(t, tenantID, pending.Enrollment.TenantID)
	require.Equal(t, uuid.Nil, pending.Enrollment.RuntimeNodeID)
	require.Empty(t, repo.nodes)

	approved, err := service.ApproveEnrollment(ctx, ApproveEnrollmentRequest{
		TenantID:     tenantID,
		EnrollmentID: pending.Enrollment.ID,
		ApprovedBy:   runtimeTestUUID(902),
	})
	require.NoError(t, err)
	require.Equal(t, RuntimeEnrollmentStatusApproved, approved.Status)
	require.NotEqual(t, uuid.Nil, approved.RuntimeNodeID)
	require.Equal(t, tenantID, repo.nodes[repo.nodeKey(tenantID, "tenant-runtime")].TenantID)

	issuedAt := time.Now()
	approvedHello, err := service.EnrollHello(ctx, EnrollHelloRequest{
		TenantID:     tenantID,
		NodeID:       "tenant-runtime",
		Name:         "Tenant Runtime",
		BootstrapKey: bootstrapSecret,
	})
	require.NoError(t, err)
	require.Equal(t, RuntimeEnrollmentStatusApproved, approvedHello.Enrollment.Status)
	require.NotNil(t, approvedHello.Session)
	require.NotEmpty(t, approvedHello.SessionToken)
	requireRuntimeSessionExpiresNear(t, issuedAt, approvedHello.Session.ExpiresAt)

	validation, err := service.ValidateRuntimeSession(ctx, approvedHello.SessionToken)
	require.NoError(t, err)
	require.Equal(t, tenantID, validation.TenantID)
	require.Equal(t, "tenant-runtime", validation.NodeID)

	before := approvedHello.Session.ExpiresAt
	renewedAt := time.Now()
	renewed, err := service.RenewRuntimeSession(ctx, approvedHello.SessionToken)
	require.NoError(t, err)
	require.Equal(t, tenantID, renewed.TenantID)
	require.True(t, renewed.ExpiresAt.After(before))
	requireRuntimeSessionExpiresNear(t, renewedAt, renewed.ExpiresAt)
}

func TestRenewRuntimeSessionRequiresValidTokenAndExtends(t *testing.T) {
	ctx := context.Background()
	repo := newEnrollmentFake(t)
	service, err := NewService(repo)
	require.NoError(t, err)

	token := repo.seedApprovedSession(t, "runtime-renew", "boot_renew", time.Now().Add(10*time.Minute))
	before := timeFromTimestamptz(repo.sessionsByLookup[LookupRuntimeSessionTokenHash(token)].ExpiresAt)

	renewedAt := time.Now()
	renewed, err := service.RenewRuntimeSession(ctx, token)
	require.NoError(t, err)
	require.True(t, renewed.ExpiresAt.After(before))
	requireRuntimeSessionExpiresNear(t, renewedAt, renewed.ExpiresAt)

	_, err = service.RenewRuntimeSession(ctx, token+"bad")
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid runtime session")
}

func TestValidateRuntimeSession(t *testing.T) {
	ctx := context.Background()
	repo := newEnrollmentFake(t)
	service, err := NewService(repo)
	require.NoError(t, err)

	token := repo.seedApprovedSession(t, "runtime-validate", "boot_validate", time.Now().Add(10*time.Minute))

	validation, err := service.ValidateRuntimeSession(ctx, token)
	require.NoError(t, err)
	require.Equal(t, "runtime-validate", validation.NodeID)
	require.NotEqual(t, uuid.Nil, validation.RuntimeNodeID)
	require.NotEqual(t, uuid.Nil, validation.SessionID)
	require.Equal(t, platform.DefaultTenantID, validation.TenantID)
	require.True(t, validation.ExpiresAt.After(time.Now()))
}

func TestRevokeEnrollmentInvalidatesSessions(t *testing.T) {
	ctx := context.Background()
	repo := newEnrollmentFake(t)
	service, err := NewService(repo)
	require.NoError(t, err)

	token := repo.seedApprovedSession(t, "runtime-revoke", "boot_revoke", time.Now().Add(10*time.Minute))
	enrollment := repo.enrollmentsByNode[repo.enrollmentKey(platform.DefaultTenantID, "runtime-revoke")]

	revoked, err := service.RevokeEnrollment(ctx, RevokeEnrollmentRequest{
		TenantID:     platform.DefaultTenantID,
		EnrollmentID: enrollment.ID,
		RevokedBy:    runtimeTestUUID(60),
		Reason:       "rotation",
	})
	require.NoError(t, err)
	require.Equal(t, RuntimeEnrollmentStatusRevoked, revoked.Status)

	_, err = service.ValidateRuntimeSession(ctx, token)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid runtime session")
}

func requireRuntimeSessionExpiresNear(t *testing.T, startedAt time.Time, expiresAt time.Time) {
	t.Helper()
	expectedTTL := 12 * time.Hour
	require.WithinRange(t, expiresAt, startedAt.Add(expectedTTL-5*time.Second), time.Now().Add(expectedTTL+5*time.Second))
}

type enrollmentFake struct {
	t                        *testing.T
	nodes                    map[string]NodeRecord
	bootstrapKeys            []RuntimeBootstrapKeyRecord
	enrollmentsByID          map[uuid.UUID]RuntimeEnrollmentRecord
	enrollmentsByNode        map[string]RuntimeEnrollmentRecord
	sessionsByLookup         map[string]RuntimeSessionRecord
	failApproveEnrollmentIDs map[uuid.UUID]bool
}

func newEnrollmentFake(t *testing.T) *enrollmentFake {
	t.Helper()
	return &enrollmentFake{
		t:                        t,
		nodes:                    map[string]NodeRecord{},
		enrollmentsByID:          map[uuid.UUID]RuntimeEnrollmentRecord{},
		enrollmentsByNode:        map[string]RuntimeEnrollmentRecord{},
		sessionsByLookup:         map[string]RuntimeSessionRecord{},
		failApproveEnrollmentIDs: map[uuid.UUID]bool{},
	}
}

func mustRuntimePayload(t *testing.T, nodeID, name string, providers []string, maxSlots int32, metadata map[string]interface{}) []byte {
	t.Helper()
	payload, err := json.Marshal(map[string]interface{}{
		"node_id":             nodeID,
		"name":                name,
		"supported_providers": providers,
		"max_slots":           maxSlots,
		"metadata":            metadata,
	})
	require.NoError(t, err)
	return payload
}

func (f *enrollmentFake) nodeKey(tenantID uuid.UUID, nodeID string) string {
	return nodeID
}

func (f *enrollmentFake) enrollmentKey(tenantID uuid.UUID, nodeID string) string {
	return tenantID.String() + "/" + nodeID
}

func (f *enrollmentFake) seedNode(tenantID uuid.UUID, nodeID, name string, providers []string) NodeRecord {
	providersJSON, err := json.Marshal(providers)
	require.NoError(f.t, err)
	record := NodeRecord{
		ID:                 runtimeTestUUID(len(f.nodes) + 100),
		TenantID:           tenantID,
		NodeID:             nodeID,
		Name:               name,
		SupportedProviders: providersJSON,
		MaxSlots:           2,
		CurrentLoad:        0,
		Status:             string(NodeStatusOnline),
		Metadata:           []byte("{}"),
		LastHeartbeatAt:    timestamptzFromTime(time.Now()),
		CreatedAt:          timestamptzFromTime(time.Now()),
		UpdatedAt:          timestamptzFromTime(time.Now()),
	}
	f.nodes[f.nodeKey(tenantID, nodeID)] = record
	return record
}

func (f *enrollmentFake) seedEnrollment(tenantID uuid.UUID, nodeID string, status RuntimeEnrollmentStatus, runtimeNodeID uuid.UUID, bootstrapKeyID uuid.UUID, bootstrapHash string) RuntimeEnrollmentRecord {
	f.bootstrapKeys = append(f.bootstrapKeys, RuntimeBootstrapKeyRecord{
		ID:       bootstrapKeyID,
		TenantID: tenantID,
		KeyHash:  bootstrapHash,
		Status:   "active",
	})
	record := RuntimeEnrollmentRecord{
		ID:             runtimeTestUUID(len(f.enrollmentsByID) + 200),
		TenantID:       tenantID,
		RuntimeNodeID:  runtimeNodeID,
		NodeID:         nodeID,
		BootstrapKeyID: bootstrapKeyID,
		Status:         status,
		RequestPayload: []byte("{}"),
		LastHelloAt:    timestamptzFromTime(time.Now()),
		CreatedAt:      timestamptzFromTime(time.Now()),
		UpdatedAt:      timestamptzFromTime(time.Now()),
	}
	f.enrollmentsByID[record.ID] = record
	f.enrollmentsByNode[f.enrollmentKey(tenantID, nodeID)] = record
	return record
}

func (f *enrollmentFake) seedApprovedSession(t *testing.T, nodeID, bootstrapSecret string, expiresAt time.Time) string {
	t.Helper()
	bootstrapHash, err := HashRuntimeSecret(bootstrapSecret)
	require.NoError(t, err)
	node := f.seedNode(platform.DefaultTenantID, nodeID, nodeID, []string{"codex"})
	enrollment := f.seedEnrollment(platform.DefaultTenantID, nodeID, RuntimeEnrollmentStatusApproved, node.ID, runtimeTestUUID(len(f.bootstrapKeys)+300), bootstrapHash)
	token, err := GenerateRuntimeSessionToken()
	require.NoError(t, err)
	secretHash, err := HashRuntimeSecret(token)
	require.NoError(t, err)
	session := RuntimeSessionRecord{
		ID:              runtimeTestUUID(len(f.sessionsByLookup) + 400),
		TenantID:        platform.DefaultTenantID,
		RuntimeNodeID:   node.ID,
		NodeID:          nodeID,
		EnrollmentID:    uuid.NullUUID{UUID: enrollment.ID, Valid: true},
		TokenLookupHash: LookupRuntimeSessionTokenHash(token),
		TokenSecretHash: secretHash,
		ExpiresAt:       timestamptzFromTime(expiresAt),
		CreatedAt:       timestamptzFromTime(time.Now()),
		UpdatedAt:       timestamptzFromTime(time.Now()),
	}
	f.sessionsByLookup[session.TokenLookupHash] = session
	return token
}

func (f *enrollmentFake) CreateNode(_ context.Context, params CreateNodeParams) (NodeRecord, error) {
	record := NodeRecord{
		ID:                 runtimeTestUUID(len(f.nodes) + 100),
		TenantID:           platform.DefaultTenantID,
		NodeID:             params.NodeID,
		Name:               params.Name,
		SupportedProviders: params.SupportedProviders,
		MaxSlots:           params.MaxSlots,
		CurrentLoad:        params.CurrentLoad,
		Status:             params.Status,
		Metadata:           params.Metadata,
		LastHeartbeatAt:    params.LastHeartbeatAt,
		CreatedAt:          timestamptzFromTime(time.Now()),
		UpdatedAt:          timestamptzFromTime(time.Now()),
	}
	f.nodes[f.nodeKey(platform.DefaultTenantID, params.NodeID)] = record
	return record, nil
}

func (f *enrollmentFake) GetNode(_ context.Context, nodeID string) (NodeRecord, error) {
	record, ok := f.nodes[f.nodeKey(platform.DefaultTenantID, nodeID)]
	if !ok {
		return NodeRecord{}, errors.New("not found")
	}
	return record, nil
}

func (f *enrollmentFake) GetNodeByID(_ context.Context, id uuid.UUID) (NodeRecord, error) {
	for _, record := range f.nodes {
		if record.ID == id {
			return record, nil
		}
	}
	return NodeRecord{}, errors.New("not found")
}

func (f *enrollmentFake) ListNodes(context.Context, ListNodesParams) ([]NodeRecord, error) {
	return nil, nil
}

func (f *enrollmentFake) ListOnlineNodes(context.Context, pgtype.Timestamptz) ([]NodeRecord, error) {
	return nil, nil
}

func (f *enrollmentFake) UpdateHeartbeat(_ context.Context, params UpdateHeartbeatParams) (NodeRecord, error) {
	key := f.nodeKey(platform.DefaultTenantID, params.NodeID)
	record, ok := f.nodes[key]
	if !ok {
		return NodeRecord{}, errors.New("not found")
	}
	record.LastHeartbeatAt = params.LastHeartbeatAt
	f.nodes[key] = record
	return record, nil
}

func (f *enrollmentFake) ApplyHeartbeat(_ context.Context, params ApplyHeartbeatParams) (NodeRecord, error) {
	key := f.nodeKey(platform.DefaultTenantID, params.NodeID)
	record, ok := f.nodes[key]
	if !ok {
		return NodeRecord{}, errors.New("not found")
	}
	record.LastHeartbeatAt = params.LastHeartbeatAt
	record.CurrentLoad = params.CurrentLoad
	record.Status = string(NodeStatusOnline)
	f.nodes[key] = record
	return record, nil
}

func (f *enrollmentFake) UpdateLoad(_ context.Context, params UpdateLoadParams) (NodeRecord, error) {
	key := f.nodeKey(platform.DefaultTenantID, params.NodeID)
	record := f.nodes[key]
	record.CurrentLoad = params.CurrentLoad
	f.nodes[key] = record
	return record, nil
}

func (f *enrollmentFake) TryAcquireNodeSlot(_ context.Context, nodeID string, threshold pgtype.Timestamptz) (NodeRecord, error) {
	key := f.nodeKey(platform.DefaultTenantID, nodeID)
	record, ok := f.nodes[key]
	if !ok {
		return NodeRecord{}, ErrNodeNotFound
	}
	if record.Status != string(NodeStatusOnline) {
		return NodeRecord{}, errors.New("node not online")
	}
	if record.LastHeartbeatAt.Valid && threshold.Valid && record.LastHeartbeatAt.Time.Before(threshold.Time) {
		return NodeRecord{}, errors.New("node heartbeat stale")
	}
	if record.CurrentLoad >= record.MaxSlots {
		return NodeRecord{}, errors.New("node at capacity")
	}
	record.CurrentLoad++
	f.nodes[key] = record
	return record, nil
}

func (f *enrollmentFake) UpdateStatus(_ context.Context, params UpdateStatusParams) (NodeRecord, error) {
	key := f.nodeKey(platform.DefaultTenantID, params.NodeID)
	record := f.nodes[key]
	record.Status = params.Status
	f.nodes[key] = record
	return record, nil
}

func (f *enrollmentFake) PatchNodeMetadata(_ context.Context, params PatchNodeMetadataParams) (NodeRecord, error) {
	key := f.nodeKey(platform.DefaultTenantID, params.NodeID)
	record := f.nodes[key]
	merged := map[string]json.RawMessage{}
	if len(record.Metadata) > 0 {
		_ = json.Unmarshal(record.Metadata, &merged)
	}
	var patch map[string]json.RawMessage
	if err := json.Unmarshal(params.Patch, &patch); err != nil {
		return NodeRecord{}, err
	}
	for k, v := range patch {
		merged[k] = v
	}
	data, err := json.Marshal(merged)
	if err != nil {
		return NodeRecord{}, err
	}
	record.Metadata = data
	f.nodes[key] = record
	return record, nil
}

func (f *enrollmentFake) CountOnlineNodesWithoutPlatformLimits(context.Context, uuid.UUID, pgtype.Timestamptz) (int64, error) {
	return 0, nil
}

func (f *enrollmentFake) DeleteNode(context.Context, string) error {
	return nil
}

func (f *enrollmentFake) ListActiveRuntimeBootstrapKeys(_ context.Context, tenantID uuid.UUID) ([]RuntimeBootstrapKeyRecord, error) {
	keys := make([]RuntimeBootstrapKeyRecord, 0, len(f.bootstrapKeys))
	for _, key := range f.bootstrapKeys {
		if key.TenantID == tenantID && key.Status == "active" {
			keys = append(keys, key)
		}
	}
	return keys, nil
}

func (f *enrollmentFake) UpsertRuntimeEnrollmentFromHello(_ context.Context, params UpsertRuntimeEnrollmentFromHelloParams) (RuntimeEnrollmentRecord, error) {
	key := f.enrollmentKey(params.TenantID, params.NodeID)
	if existing, ok := f.enrollmentsByNode[key]; ok {
		if existing.Status == RuntimeEnrollmentStatusApproved || existing.Status == RuntimeEnrollmentStatusRejected || existing.Status == RuntimeEnrollmentStatusRevoked {
			existing.LastHelloAt = params.LastHelloAt
			f.enrollmentsByNode[key] = existing
			f.enrollmentsByID[existing.ID] = existing
			return existing, nil
		}
	}
	record := RuntimeEnrollmentRecord{
		ID:             runtimeTestUUID(len(f.enrollmentsByID) + 200),
		TenantID:       params.TenantID,
		RuntimeNodeID:  uuid.Nil,
		NodeID:         params.NodeID,
		BootstrapKeyID: params.BootstrapKeyID,
		Status:         RuntimeEnrollmentStatusPending,
		RequestPayload: params.RequestPayload,
		LastHelloAt:    params.LastHelloAt,
		CreatedAt:      timestamptzFromTime(time.Now()),
		UpdatedAt:      timestamptzFromTime(time.Now()),
	}
	f.enrollmentsByID[record.ID] = record
	f.enrollmentsByNode[key] = record
	return record, nil
}

func (f *enrollmentFake) GetRuntimeEnrollment(_ context.Context, tenantID, enrollmentID uuid.UUID) (RuntimeEnrollmentRecord, error) {
	record, ok := f.enrollmentsByID[enrollmentID]
	if !ok || record.TenantID != tenantID {
		return RuntimeEnrollmentRecord{}, errors.New("not found")
	}
	return record, nil
}

func (f *enrollmentFake) ListRuntimeEnrollments(_ context.Context, params ListRuntimeEnrollmentsParams) ([]RuntimeEnrollmentRecord, error) {
	records := make([]RuntimeEnrollmentRecord, 0, len(f.enrollmentsByID))
	for _, record := range f.enrollmentsByID {
		if record.TenantID != params.TenantID {
			continue
		}
		if params.Status.Valid && string(record.Status) != params.Status.String {
			continue
		}
		records = append(records, record)
	}
	return records, nil
}

func (f *enrollmentFake) UpsertRuntimeNodeForTenant(_ context.Context, params UpsertRuntimeNodeForTenantParams) (NodeRecord, error) {
	key := f.nodeKey(params.TenantID, params.NodeID)
	if existing, ok := f.nodes[key]; ok {
		if existing.TenantID != params.TenantID {
			return NodeRecord{}, errors.New("runtime node belongs to another tenant")
		}
		existing.Name = params.Name
		existing.SupportedProviders = params.SupportedProviders
		existing.MaxSlots = params.MaxSlots
		existing.Metadata = params.Metadata
		existing.Status = params.Status
		existing.LastHeartbeatAt = params.LastHeartbeatAt
		f.nodes[key] = existing
		return existing, nil
	}
	record := NodeRecord{
		ID:                 runtimeTestUUID(len(f.nodes) + 100),
		TenantID:           params.TenantID,
		NodeID:             params.NodeID,
		Name:               params.Name,
		SupportedProviders: params.SupportedProviders,
		MaxSlots:           params.MaxSlots,
		CurrentLoad:        params.CurrentLoad,
		Status:             params.Status,
		Metadata:           params.Metadata,
		LastHeartbeatAt:    params.LastHeartbeatAt,
		CreatedAt:          timestamptzFromTime(time.Now()),
		UpdatedAt:          timestamptzFromTime(time.Now()),
	}
	f.nodes[key] = record
	return record, nil
}

func (f *enrollmentFake) ApproveRuntimeEnrollmentWithNode(_ context.Context, params ApproveRuntimeEnrollmentWithNodeParams) (RuntimeEnrollmentRecord, error) {
	record, ok := f.enrollmentsByID[params.EnrollmentID]
	if !ok || record.TenantID != params.TenantID || record.Status != RuntimeEnrollmentStatusPending {
		return RuntimeEnrollmentRecord{}, errors.New("not found")
	}
	if f.failApproveEnrollmentIDs[params.EnrollmentID] {
		record.Status = RuntimeEnrollmentStatusRejected
		f.enrollmentsByID[record.ID] = record
		f.enrollmentsByNode[f.enrollmentKey(record.TenantID, record.NodeID)] = record
		return RuntimeEnrollmentRecord{}, errors.New("approve failed")
	}
	key := f.nodeKey(params.TenantID, record.NodeID)
	node, ok := f.nodes[key]
	if ok {
		if node.TenantID != params.TenantID {
			return RuntimeEnrollmentRecord{}, errors.New("runtime node belongs to another tenant")
		}
		node.Name = params.Name
		node.SupportedProviders = params.SupportedProviders
		node.MaxSlots = params.MaxSlots
		node.CurrentLoad = params.CurrentLoad
		// 镜像真实 SQL 的 ON CONFLICT 语义：审批不覆盖已有节点的 status/心跳
		node.Metadata = params.Metadata
		f.nodes[key] = node
	} else {
		node = NodeRecord{
			ID:                 runtimeTestUUID(len(f.nodes) + 100),
			TenantID:           params.TenantID,
			NodeID:             record.NodeID,
			Name:               params.Name,
			SupportedProviders: params.SupportedProviders,
			MaxSlots:           params.MaxSlots,
			CurrentLoad:        params.CurrentLoad,
			Status:             params.NodeStatus,
			Metadata:           params.Metadata,
			LastHeartbeatAt:    params.LastHeartbeatAt,
			CreatedAt:          timestamptzFromTime(time.Now()),
			UpdatedAt:          timestamptzFromTime(time.Now()),
		}
		f.nodes[key] = node
	}
	record.RuntimeNodeID = node.ID
	record.Status = RuntimeEnrollmentStatusApproved
	record.ApprovedBy = uuid.NullUUID{UUID: params.ApprovedBy, Valid: params.ApprovedBy != uuid.Nil}
	record.ApprovedAt = timestamptzFromTime(time.Now())
	f.enrollmentsByID[record.ID] = record
	f.enrollmentsByNode[f.enrollmentKey(record.TenantID, record.NodeID)] = record
	return record, nil
}

func (f *enrollmentFake) ApproveRuntimeEnrollment(_ context.Context, params ApproveRuntimeEnrollmentParams) (RuntimeEnrollmentRecord, error) {
	record, ok := f.enrollmentsByID[params.EnrollmentID]
	if !ok || record.TenantID != params.TenantID || record.Status != RuntimeEnrollmentStatusPending {
		return RuntimeEnrollmentRecord{}, errors.New("not found")
	}
	if f.failApproveEnrollmentIDs[params.EnrollmentID] {
		record.Status = RuntimeEnrollmentStatusRejected
		f.enrollmentsByID[record.ID] = record
		f.enrollmentsByNode[f.enrollmentKey(record.TenantID, record.NodeID)] = record
		return RuntimeEnrollmentRecord{}, errors.New("approve failed")
	}
	nodeKey := f.nodeKey(params.TenantID, record.NodeID)
	node, ok := f.nodes[nodeKey]
	if !ok || node.ID != params.RuntimeNodeID {
		return RuntimeEnrollmentRecord{}, errors.New("runtime node not found")
	}
	record.RuntimeNodeID = params.RuntimeNodeID
	record.Status = RuntimeEnrollmentStatusApproved
	record.ApprovedBy = uuid.NullUUID{UUID: params.ApprovedBy, Valid: params.ApprovedBy != uuid.Nil}
	record.ApprovedAt = timestamptzFromTime(time.Now())
	f.enrollmentsByID[record.ID] = record
	f.enrollmentsByNode[f.enrollmentKey(record.TenantID, record.NodeID)] = record
	return record, nil
}

func (f *enrollmentFake) RejectRuntimeEnrollment(_ context.Context, params RejectRuntimeEnrollmentParams) (RuntimeEnrollmentRecord, error) {
	record := f.enrollmentsByID[params.EnrollmentID]
	record.Status = RuntimeEnrollmentStatusRejected
	record.RejectReason = textFromString(&params.Reason)
	f.enrollmentsByID[record.ID] = record
	f.enrollmentsByNode[f.enrollmentKey(record.TenantID, record.NodeID)] = record
	return record, nil
}

func (f *enrollmentFake) RevokeRuntimeEnrollment(_ context.Context, params RevokeRuntimeEnrollmentParams) (RuntimeEnrollmentRecord, error) {
	record := f.enrollmentsByID[params.EnrollmentID]
	record.Status = RuntimeEnrollmentStatusRevoked
	record.RevokeReason = textFromString(&params.Reason)
	record.RevokedAt = timestamptzFromTime(time.Now())
	f.enrollmentsByID[record.ID] = record
	f.enrollmentsByNode[f.enrollmentKey(record.TenantID, record.NodeID)] = record
	for lookup, session := range f.sessionsByLookup {
		if session.EnrollmentID.Valid && session.EnrollmentID.UUID == record.ID {
			session.RevokedAt = timestamptzFromTime(time.Now())
			session.RevokedReason = textFromString(&params.Reason)
			f.sessionsByLookup[lookup] = session
		}
	}
	return record, nil
}

func (f *enrollmentFake) CreateRuntimeSession(_ context.Context, params CreateRuntimeSessionParams) (RuntimeSessionRecord, error) {
	enrollment := f.enrollmentsByID[params.EnrollmentID]
	if enrollment.Status != RuntimeEnrollmentStatusApproved || enrollment.RuntimeNodeID != params.RuntimeNodeID {
		return RuntimeSessionRecord{}, errors.New("not found")
	}
	session := RuntimeSessionRecord{
		ID:              runtimeTestUUID(len(f.sessionsByLookup) + 400),
		TenantID:        params.TenantID,
		RuntimeNodeID:   params.RuntimeNodeID,
		NodeID:          enrollment.NodeID,
		EnrollmentID:    uuid.NullUUID{UUID: params.EnrollmentID, Valid: true},
		TokenLookupHash: params.TokenLookupHash,
		TokenSecretHash: params.TokenSecretHash,
		ExpiresAt:       params.ExpiresAt,
		CreatedAt:       timestamptzFromTime(time.Now()),
		UpdatedAt:       timestamptzFromTime(time.Now()),
	}
	f.sessionsByLookup[session.TokenLookupHash] = session
	return session, nil
}

func (f *enrollmentFake) GetActiveRuntimeSessionByLookupHash(_ context.Context, params GetActiveRuntimeSessionByLookupHashParams) (RuntimeSessionRecord, error) {
	session, ok := f.sessionsByLookup[params.TokenLookupHash]
	if !ok || session.RevokedAt.Valid || !session.ExpiresAt.Time.After(time.Now()) {
		return RuntimeSessionRecord{}, errors.New("not found")
	}
	enrollment := f.enrollmentsByID[session.EnrollmentID.UUID]
	if enrollment.Status != RuntimeEnrollmentStatusApproved || enrollment.RevokedAt.Valid {
		return RuntimeSessionRecord{}, errors.New("not found")
	}
	return session, nil
}

func (f *enrollmentFake) RenewRuntimeSession(_ context.Context, params RenewRuntimeSessionParams) (RuntimeSessionRecord, error) {
	for lookup, session := range f.sessionsByLookup {
		if session.ID == params.SessionID && session.TenantID == params.TenantID && !session.RevokedAt.Valid {
			session.ExpiresAt = params.ExpiresAt
			session.LastSeenAt = timestamptzFromTime(time.Now())
			f.sessionsByLookup[lookup] = session
			return session, nil
		}
	}
	return RuntimeSessionRecord{}, errors.New("not found")
}

func (f *enrollmentFake) TouchRuntimeSession(_ context.Context, params TouchRuntimeSessionParams) (RuntimeSessionRecord, error) {
	for lookup, session := range f.sessionsByLookup {
		if session.ID == params.SessionID && session.TenantID == params.TenantID && !session.RevokedAt.Valid {
			session.LastSeenAt = timestamptzFromTime(time.Now())
			f.sessionsByLookup[lookup] = session
			return session, nil
		}
	}
	return RuntimeSessionRecord{}, errors.New("not found")
}

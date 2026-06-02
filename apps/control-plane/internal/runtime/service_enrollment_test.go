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
		TenantID: DefaultTenantID,
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
	require.Contains(t, repo.nodes, "runtime-dev-1")
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
	node := repo.seedNode("runtime-approved", "Runtime Approved", []string{"codex"})
	enrollment := repo.seedEnrollment("runtime-approved", RuntimeEnrollmentStatusApproved, node.ID, runtimeTestUUID(20), bootstrapHash)

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
	require.True(t, resp.Session.ExpiresAt.After(time.Now()))
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
		TenantID: DefaultTenantID,
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
			node := repo.seedNode("runtime-terminal", "Runtime Terminal", []string{"codex"})
			repo.seedEnrollment("runtime-terminal", status, node.ID, runtimeTestUUID(40), bootstrapHash)

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
	node := repo.seedNode("runtime-approve", "Runtime Approve", []string{"codex"})
	enrollment := repo.seedEnrollment("runtime-approve", RuntimeEnrollmentStatusPending, node.ID, runtimeTestUUID(50), bootstrapHash)

	approved, err := service.ApproveEnrollment(ctx, ApproveEnrollmentRequest{
		TenantID:     DefaultTenantID,
		EnrollmentID: enrollment.ID,
		ApprovedBy:   runtimeTestUUID(51),
	})
	require.NoError(t, err)
	require.Equal(t, RuntimeEnrollmentStatusApproved, approved.Status)
	require.Equal(t, node.ID, approved.RuntimeNodeID)
	require.Contains(t, repo.nodes, "runtime-approve")
}

func TestRenewRuntimeSessionRequiresValidTokenAndExtends(t *testing.T) {
	ctx := context.Background()
	repo := newEnrollmentFake(t)
	service, err := NewService(repo)
	require.NoError(t, err)

	token := repo.seedApprovedSession(t, "runtime-renew", "boot_renew", time.Now().Add(10*time.Minute))
	before := timeFromTimestamptz(repo.sessionsByLookup[LookupRuntimeSessionTokenHash(token)].ExpiresAt)

	renewed, err := service.RenewRuntimeSession(ctx, token)
	require.NoError(t, err)
	require.True(t, renewed.ExpiresAt.After(before))

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
	require.Equal(t, DefaultTenantID, validation.TenantID)
	require.True(t, validation.ExpiresAt.After(time.Now()))
}

func TestRevokeEnrollmentInvalidatesSessions(t *testing.T) {
	ctx := context.Background()
	repo := newEnrollmentFake(t)
	service, err := NewService(repo)
	require.NoError(t, err)

	token := repo.seedApprovedSession(t, "runtime-revoke", "boot_revoke", time.Now().Add(10*time.Minute))
	enrollment := repo.enrollmentsByNode["runtime-revoke"]

	revoked, err := service.RevokeEnrollment(ctx, RevokeEnrollmentRequest{
		TenantID:     DefaultTenantID,
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

type enrollmentFake struct {
	t                 *testing.T
	nodes             map[string]NodeRecord
	bootstrapKeys     []RuntimeBootstrapKeyRecord
	enrollmentsByID   map[uuid.UUID]RuntimeEnrollmentRecord
	enrollmentsByNode map[string]RuntimeEnrollmentRecord
	sessionsByLookup  map[string]RuntimeSessionRecord
}

func newEnrollmentFake(t *testing.T) *enrollmentFake {
	t.Helper()
	return &enrollmentFake{
		t:                 t,
		nodes:             map[string]NodeRecord{},
		enrollmentsByID:   map[uuid.UUID]RuntimeEnrollmentRecord{},
		enrollmentsByNode: map[string]RuntimeEnrollmentRecord{},
		sessionsByLookup:  map[string]RuntimeSessionRecord{},
	}
}

func (f *enrollmentFake) seedNode(nodeID, name string, providers []string) NodeRecord {
	providersJSON, err := json.Marshal(providers)
	require.NoError(f.t, err)
	record := NodeRecord{
		ID:                 runtimeTestUUID(len(f.nodes) + 100),
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
	f.nodes[nodeID] = record
	return record
}

func (f *enrollmentFake) seedEnrollment(nodeID string, status RuntimeEnrollmentStatus, runtimeNodeID uuid.UUID, bootstrapKeyID uuid.UUID, bootstrapHash string) RuntimeEnrollmentRecord {
	f.bootstrapKeys = append(f.bootstrapKeys, RuntimeBootstrapKeyRecord{
		ID:       bootstrapKeyID,
		TenantID: DefaultTenantID,
		KeyHash:  bootstrapHash,
		Status:   "active",
	})
	record := RuntimeEnrollmentRecord{
		ID:             runtimeTestUUID(len(f.enrollmentsByID) + 200),
		TenantID:       DefaultTenantID,
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
	f.enrollmentsByNode[nodeID] = record
	return record
}

func (f *enrollmentFake) seedApprovedSession(t *testing.T, nodeID, bootstrapSecret string, expiresAt time.Time) string {
	t.Helper()
	bootstrapHash, err := HashRuntimeSecret(bootstrapSecret)
	require.NoError(t, err)
	node := f.seedNode(nodeID, nodeID, []string{"codex"})
	enrollment := f.seedEnrollment(nodeID, RuntimeEnrollmentStatusApproved, node.ID, runtimeTestUUID(len(f.bootstrapKeys)+300), bootstrapHash)
	token, err := GenerateRuntimeSessionToken()
	require.NoError(t, err)
	secretHash, err := HashRuntimeSecret(token)
	require.NoError(t, err)
	session := RuntimeSessionRecord{
		ID:              runtimeTestUUID(len(f.sessionsByLookup) + 400),
		TenantID:        DefaultTenantID,
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
	f.nodes[params.NodeID] = record
	return record, nil
}

func (f *enrollmentFake) GetNode(_ context.Context, nodeID string) (NodeRecord, error) {
	record, ok := f.nodes[nodeID]
	if !ok {
		return NodeRecord{}, errors.New("not found")
	}
	return record, nil
}

func (f *enrollmentFake) ListNodes(context.Context, ListNodesParams) ([]NodeRecord, error) {
	return nil, nil
}

func (f *enrollmentFake) ListOnlineNodes(context.Context, pgtype.Timestamptz) ([]NodeRecord, error) {
	return nil, nil
}

func (f *enrollmentFake) UpdateHeartbeat(_ context.Context, params UpdateHeartbeatParams) (NodeRecord, error) {
	record, ok := f.nodes[params.NodeID]
	if !ok {
		return NodeRecord{}, errors.New("not found")
	}
	record.LastHeartbeatAt = params.LastHeartbeatAt
	f.nodes[params.NodeID] = record
	return record, nil
}

func (f *enrollmentFake) UpdateLoad(_ context.Context, params UpdateLoadParams) (NodeRecord, error) {
	record := f.nodes[params.NodeID]
	record.CurrentLoad = params.CurrentLoad
	f.nodes[params.NodeID] = record
	return record, nil
}

func (f *enrollmentFake) UpdateStatus(_ context.Context, params UpdateStatusParams) (NodeRecord, error) {
	record := f.nodes[params.NodeID]
	record.Status = params.Status
	f.nodes[params.NodeID] = record
	return record, nil
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
	if existing, ok := f.enrollmentsByNode[params.NodeID]; ok {
		if existing.Status == RuntimeEnrollmentStatusApproved || existing.Status == RuntimeEnrollmentStatusRejected || existing.Status == RuntimeEnrollmentStatusRevoked {
			existing.LastHelloAt = params.LastHelloAt
			f.enrollmentsByNode[params.NodeID] = existing
			f.enrollmentsByID[existing.ID] = existing
			return existing, nil
		}
	}
	record := RuntimeEnrollmentRecord{
		ID:             runtimeTestUUID(len(f.enrollmentsByID) + 200),
		TenantID:       params.TenantID,
		RuntimeNodeID:  params.RuntimeNodeID,
		NodeID:         params.NodeID,
		BootstrapKeyID: params.BootstrapKeyID,
		Status:         RuntimeEnrollmentStatusPending,
		RequestPayload: params.RequestPayload,
		LastHelloAt:    params.LastHelloAt,
		CreatedAt:      timestamptzFromTime(time.Now()),
		UpdatedAt:      timestamptzFromTime(time.Now()),
	}
	f.enrollmentsByID[record.ID] = record
	f.enrollmentsByNode[record.NodeID] = record
	return record, nil
}

func (f *enrollmentFake) ApproveRuntimeEnrollment(_ context.Context, params ApproveRuntimeEnrollmentParams) (RuntimeEnrollmentRecord, error) {
	record, ok := f.enrollmentsByID[params.EnrollmentID]
	if !ok || record.TenantID != params.TenantID || record.Status != RuntimeEnrollmentStatusPending {
		return RuntimeEnrollmentRecord{}, errors.New("not found")
	}
	record.Status = RuntimeEnrollmentStatusApproved
	record.ApprovedBy = uuid.NullUUID{UUID: params.ApprovedBy, Valid: params.ApprovedBy != uuid.Nil}
	record.ApprovedAt = timestamptzFromTime(time.Now())
	f.enrollmentsByID[record.ID] = record
	f.enrollmentsByNode[record.NodeID] = record
	return record, nil
}

func (f *enrollmentFake) RejectRuntimeEnrollment(_ context.Context, params RejectRuntimeEnrollmentParams) (RuntimeEnrollmentRecord, error) {
	record := f.enrollmentsByID[params.EnrollmentID]
	record.Status = RuntimeEnrollmentStatusRejected
	record.RejectReason = textFromString(&params.Reason)
	f.enrollmentsByID[record.ID] = record
	f.enrollmentsByNode[record.NodeID] = record
	return record, nil
}

func (f *enrollmentFake) RevokeRuntimeEnrollment(_ context.Context, params RevokeRuntimeEnrollmentParams) (RuntimeEnrollmentRecord, error) {
	record := f.enrollmentsByID[params.EnrollmentID]
	record.Status = RuntimeEnrollmentStatusRevoked
	record.RevokeReason = textFromString(&params.Reason)
	record.RevokedAt = timestamptzFromTime(time.Now())
	f.enrollmentsByID[record.ID] = record
	f.enrollmentsByNode[record.NodeID] = record
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
	if !ok || session.TenantID != params.TenantID || session.RevokedAt.Valid || !session.ExpiresAt.Time.After(time.Now()) {
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

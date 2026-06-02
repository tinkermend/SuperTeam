package runtime

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// Repository defines the interface for runtime node data access
type Repository interface {
	// Node operations
	CreateNode(ctx context.Context, params CreateNodeParams) (NodeRecord, error)
	GetNode(ctx context.Context, nodeID string) (NodeRecord, error)
	ListNodes(ctx context.Context, params ListNodesParams) ([]NodeRecord, error)
	ListOnlineNodes(ctx context.Context, heartbeatThreshold pgtype.Timestamptz) ([]NodeRecord, error)
	UpdateHeartbeat(ctx context.Context, params UpdateHeartbeatParams) (NodeRecord, error)
	UpdateLoad(ctx context.Context, params UpdateLoadParams) (NodeRecord, error)
	UpdateStatus(ctx context.Context, params UpdateStatusParams) (NodeRecord, error)
	DeleteNode(ctx context.Context, nodeID string) error
}

type EnrollmentRepository interface {
	ListActiveRuntimeBootstrapKeys(ctx context.Context, tenantID uuid.UUID) ([]RuntimeBootstrapKeyRecord, error)
	UpsertRuntimeEnrollmentFromHello(ctx context.Context, params UpsertRuntimeEnrollmentFromHelloParams) (RuntimeEnrollmentRecord, error)
	GetRuntimeEnrollment(ctx context.Context, tenantID, enrollmentID uuid.UUID) (RuntimeEnrollmentRecord, error)
	UpsertRuntimeNodeForTenant(ctx context.Context, params UpsertRuntimeNodeForTenantParams) (NodeRecord, error)
	ApproveRuntimeEnrollment(ctx context.Context, params ApproveRuntimeEnrollmentParams) (RuntimeEnrollmentRecord, error)
	ApproveRuntimeEnrollmentWithNode(ctx context.Context, params ApproveRuntimeEnrollmentWithNodeParams) (RuntimeEnrollmentRecord, error)
	RejectRuntimeEnrollment(ctx context.Context, params RejectRuntimeEnrollmentParams) (RuntimeEnrollmentRecord, error)
	RevokeRuntimeEnrollment(ctx context.Context, params RevokeRuntimeEnrollmentParams) (RuntimeEnrollmentRecord, error)
	CreateRuntimeSession(ctx context.Context, params CreateRuntimeSessionParams) (RuntimeSessionRecord, error)
	GetActiveRuntimeSessionByLookupHash(ctx context.Context, params GetActiveRuntimeSessionByLookupHashParams) (RuntimeSessionRecord, error)
	RenewRuntimeSession(ctx context.Context, params RenewRuntimeSessionParams) (RuntimeSessionRecord, error)
	TouchRuntimeSession(ctx context.Context, params TouchRuntimeSessionParams) (RuntimeSessionRecord, error)
}

type CapabilityRepository interface {
	UpsertRuntimeCapability(ctx context.Context, params UpsertRuntimeCapabilityParams) (RuntimeCapability, error)
}

// CreateNodeParams represents parameters for creating a node
type CreateNodeParams struct {
	NodeID             string
	Name               string
	SupportedProviders []byte
	MaxSlots           int32
	CurrentLoad        int32
	Status             string
	Metadata           []byte
	LastHeartbeatAt    pgtype.Timestamptz
}

// NodeRecord represents a node record from the database
type NodeRecord struct {
	ID                 uuid.UUID
	TenantID           uuid.UUID
	NodeID             string
	Name               string
	SupportedProviders []byte
	MaxSlots           int32
	CurrentLoad        int32
	Status             string
	Metadata           []byte
	LastHeartbeatAt    pgtype.Timestamptz
	CreatedAt          pgtype.Timestamptz
	UpdatedAt          pgtype.Timestamptz
}

// ListNodesParams represents parameters for listing nodes
type ListNodesParams struct {
	Status pgtype.Text
	Offset int32
	Limit  int32
}

// UpdateHeartbeatParams represents parameters for updating heartbeat
type UpdateHeartbeatParams struct {
	NodeID          string
	LastHeartbeatAt pgtype.Timestamptz
}

// UpdateLoadParams represents parameters for updating load
type UpdateLoadParams struct {
	NodeID      string
	CurrentLoad int32
}

// UpdateStatusParams represents parameters for updating status
type UpdateStatusParams struct {
	NodeID string
	Status string
}

type UpsertRuntimeEnrollmentFromHelloParams struct {
	TenantID       uuid.UUID
	NodeID         string
	BootstrapKeyID uuid.UUID
	RequestPayload []byte
	LastHelloAt    pgtype.Timestamptz
}

type ApproveRuntimeEnrollmentParams struct {
	TenantID      uuid.UUID
	EnrollmentID  uuid.UUID
	RuntimeNodeID uuid.UUID
	ApprovedBy    uuid.UUID
}

type ApproveRuntimeEnrollmentWithNodeParams struct {
	TenantID           uuid.UUID
	EnrollmentID       uuid.UUID
	ApprovedBy         uuid.UUID
	Name               string
	SupportedProviders []byte
	MaxSlots           int32
	CurrentLoad        int32
	NodeStatus         string
	Metadata           []byte
	LastHeartbeatAt    pgtype.Timestamptz
}

type RejectRuntimeEnrollmentParams struct {
	TenantID     uuid.UUID
	EnrollmentID uuid.UUID
	RejectedBy   uuid.UUID
	Reason       string
}

type RevokeRuntimeEnrollmentParams struct {
	TenantID     uuid.UUID
	EnrollmentID uuid.UUID
	RevokedBy    uuid.UUID
	Reason       string
}

type CreateRuntimeSessionParams struct {
	TenantID        uuid.UUID
	RuntimeNodeID   uuid.UUID
	EnrollmentID    uuid.UUID
	TokenLookupHash string
	TokenSecretHash string
	ExpiresAt       pgtype.Timestamptz
}

type GetActiveRuntimeSessionByLookupHashParams struct {
	TokenLookupHash string
}

type UpsertRuntimeNodeForTenantParams struct {
	TenantID           uuid.UUID
	NodeID             string
	Name               string
	SupportedProviders []byte
	MaxSlots           int32
	CurrentLoad        int32
	Status             string
	Metadata           []byte
	LastHeartbeatAt    pgtype.Timestamptz
}

type RenewRuntimeSessionParams struct {
	TenantID  uuid.UUID
	SessionID uuid.UUID
	ExpiresAt pgtype.Timestamptz
}

type TouchRuntimeSessionParams struct {
	TenantID  uuid.UUID
	SessionID uuid.UUID
}

type UpsertRuntimeCapabilityParams struct {
	TenantID         uuid.UUID
	RuntimeNodeID    uuid.UUID
	CapabilityType   string
	CapabilityKey    string
	ProviderType     string
	ProviderVersion  *string
	BinaryPath       *string
	Available        bool
	WorkspaceBaseDir *string
	Capacity         []byte
	Labels           []byte
	Status           string
	Details          []byte
	HealthStatus     string
	Metadata         []byte
	LastSeenAt       pgtype.Timestamptz
}

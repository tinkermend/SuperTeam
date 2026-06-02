package runtime

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// NodeStatus represents the status of a runtime node
type NodeStatus string

const (
	NodeStatusOnline  NodeStatus = "online"
	NodeStatusOffline NodeStatus = "offline"
)

var DefaultTenantID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// RuntimeEnrollmentStatus represents the human approval state for runtime enrollment.
type RuntimeEnrollmentStatus string

const (
	RuntimeEnrollmentStatusPending  RuntimeEnrollmentStatus = "pending"
	RuntimeEnrollmentStatusApproved RuntimeEnrollmentStatus = "approved"
	RuntimeEnrollmentStatusRejected RuntimeEnrollmentStatus = "rejected"
	RuntimeEnrollmentStatusRevoked  RuntimeEnrollmentStatus = "revoked"
)

// RuntimeBootstrapKeyRecord is the repository shape for active enrollment bootstrap keys.
type RuntimeBootstrapKeyRecord struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	Name      string
	KeyHash   string
	Status    string
	ExpiresAt pgtype.Timestamptz
	CreatedAt pgtype.Timestamptz
	UpdatedAt pgtype.Timestamptz
}

// RuntimeEnrollmentRecord is the repository shape for runtime enrollment approvals.
type RuntimeEnrollmentRecord struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	RuntimeNodeID  uuid.UUID
	NodeID         string
	BootstrapKeyID uuid.UUID
	Status         RuntimeEnrollmentStatus
	RequestPayload []byte
	ApprovedBy     uuid.NullUUID
	ApprovedAt     pgtype.Timestamptz
	RejectedBy     uuid.NullUUID
	RejectedAt     pgtype.Timestamptz
	RejectReason   pgtype.Text
	RevokedBy      uuid.NullUUID
	RevokedAt      pgtype.Timestamptz
	RevokeReason   pgtype.Text
	LastHelloAt    pgtype.Timestamptz
	CreatedAt      pgtype.Timestamptz
	UpdatedAt      pgtype.Timestamptz
}

// RuntimeSessionRecord is the repository shape for short-lived runtime sessions.
type RuntimeSessionRecord struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	RuntimeNodeID   uuid.UUID
	NodeID          string
	EnrollmentID    uuid.NullUUID
	TokenLookupHash string
	TokenSecretHash string
	ExpiresAt       pgtype.Timestamptz
	LastSeenAt      pgtype.Timestamptz
	RevokedAt       pgtype.Timestamptz
	RevokedReason   pgtype.Text
	CreatedAt       pgtype.Timestamptz
	UpdatedAt       pgtype.Timestamptz
}

type RuntimeEnrollment struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	RuntimeNodeID  uuid.UUID
	NodeID         string
	BootstrapKeyID uuid.UUID
	Status         RuntimeEnrollmentStatus
	RequestPayload map[string]interface{}
	ApprovedBy     uuid.NullUUID
	ApprovedAt     time.Time
	RejectedBy     uuid.NullUUID
	RejectedAt     time.Time
	RejectReason   *string
	RevokedBy      uuid.NullUUID
	RevokedAt      time.Time
	RevokeReason   *string
	LastHelloAt    time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type RuntimeSession struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	RuntimeNodeID uuid.UUID
	NodeID        string
	EnrollmentID  uuid.NullUUID
	ExpiresAt     time.Time
	LastSeenAt    time.Time
	RevokedAt     time.Time
	RevokedReason *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type RuntimeCapabilityInput struct {
	CapabilityType   string
	CapabilityKey    string
	ProviderType     string
	ProviderVersion  *string
	BinaryPath       *string
	Available        bool
	WorkspaceBaseDir *string
	Capacity         map[string]interface{}
	Labels           map[string]interface{}
	Status           string
	Details          map[string]interface{}
	HealthStatus     string
	Metadata         map[string]interface{}
}

type RuntimeCapability struct {
	ID             uuid.UUID
	TenantID       uuid.UUID
	RuntimeNodeID  uuid.UUID
	CapabilityType string
	CapabilityKey  string
	ProviderType   string
	Available      bool
	Status         string
	HealthStatus   string
	LastSeenAt     time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type EnrollHelloRequest struct {
	TenantID           uuid.UUID
	NodeID             string
	Name               string
	BootstrapKey       string
	SupportedProviders []string
	MaxSlots           int32
	Metadata           map[string]interface{}
	Version            string
	Capabilities       []RuntimeCapabilityInput
}

type EnrollHelloResponse struct {
	Enrollment   RuntimeEnrollment
	Session      *RuntimeSession
	SessionToken string
}

type ApproveEnrollmentRequest struct {
	TenantID     uuid.UUID
	EnrollmentID uuid.UUID
	ApprovedBy   uuid.UUID
}

type RejectEnrollmentRequest struct {
	TenantID     uuid.UUID
	EnrollmentID uuid.UUID
	RejectedBy   uuid.UUID
	Reason       string
}

type RevokeEnrollmentRequest struct {
	TenantID     uuid.UUID
	EnrollmentID uuid.UUID
	RevokedBy    uuid.UUID
	Reason       string
}

type RuntimeSessionValidation struct {
	SessionID     uuid.UUID
	TenantID      uuid.UUID
	RuntimeNodeID uuid.UUID
	NodeID        string
	EnrollmentID  uuid.NullUUID
	ExpiresAt     time.Time
}

type ListRuntimeEnrollmentsFilter struct {
	TenantID uuid.UUID
	Status   *RuntimeEnrollmentStatus
	Limit    int32
	Offset   int32
}

// IsValid checks if the status is valid
func (s NodeStatus) IsValid() bool {
	switch s {
	case NodeStatusOnline, NodeStatusOffline:
		return true
	}
	return false
}

// Node represents a runtime node in the domain model
type Node struct {
	ID                 uuid.UUID
	NodeID             string
	Name               string
	SupportedProviders []string
	MaxSlots           int32
	CurrentLoad        int32
	Status             NodeStatus
	Metadata           map[string]interface{}
	LastHeartbeatAt    time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// IsOnline checks if the node is online based on heartbeat
// A node is considered online if it has sent a heartbeat within the last 60 seconds
func (n *Node) IsOnline() bool {
	return time.Since(n.LastHeartbeatAt) <= 60*time.Second
}

// HasCapacity checks if the node has available slots
func (n *Node) HasCapacity() bool {
	return n.CurrentLoad < n.MaxSlots
}

// SupportsProvider checks if the node supports a given provider type
func (n *Node) SupportsProvider(providerType string) bool {
	for _, p := range n.SupportedProviders {
		if p == providerType {
			return true
		}
	}
	return false
}

// RegisterNodeRequest represents a request to register a runtime node
type RegisterNodeRequest struct {
	NodeID             string
	Name               string
	SupportedProviders []string
	MaxSlots           int32
	Metadata           map[string]interface{}
}

// UpdateHeartbeatRequest represents a request to update node heartbeat
type UpdateHeartbeatRequest struct {
	NodeID      string
	CurrentLoad int32
}

// ListNodesFilter represents filters for listing nodes
type ListNodesFilter struct {
	Status *NodeStatus
	Limit  int32
	Offset int32
}

// Helper functions to convert between pgtype and domain types

func textFromString(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: *s, Valid: true}
}

func stringFromText(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

func timeFromTimestamptz(t pgtype.Timestamptz) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time
}

func timestamptzFromTime(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

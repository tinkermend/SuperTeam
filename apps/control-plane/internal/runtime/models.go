package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	CapabilityType   string                 `json:"capability_type"`
	CapabilityKey    string                 `json:"capability_key"`
	ProviderType     string                 `json:"provider_type"`
	ProviderVersion  *string                `json:"provider_version"`
	BinaryPath       *string                `json:"binary_path"`
	Available        bool                   `json:"available"`
	WorkspaceBaseDir *string                `json:"workspace_base_dir"`
	Capacity         map[string]interface{} `json:"capacity"`
	Labels           map[string]interface{} `json:"labels"`
	Status           string                 `json:"status"`
	Details          map[string]interface{} `json:"details"`
	HealthStatus     string                 `json:"health_status"`
	Metadata         map[string]interface{} `json:"metadata"`
}

type RuntimeCapability struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	RuntimeNodeID    uuid.UUID
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
	LastSeenAt       time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
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

type RuntimeEventType string
type RuntimeEventSeverity string
type RuntimeEventSource string

const (
	RuntimeEventEnrollmentRequested RuntimeEventType = "enrollment_requested"
	RuntimeEventEnrollmentApproved  RuntimeEventType = "enrollment_approved"
	RuntimeEventEnrollmentRejected  RuntimeEventType = "enrollment_rejected"
	RuntimeEventEnrollmentRevoked   RuntimeEventType = "enrollment_revoked"
	RuntimeEventNodeOnline          RuntimeEventType = "node_online"
	RuntimeEventNodeOffline         RuntimeEventType = "node_offline"
	RuntimeEventCapabilityReported  RuntimeEventType = "capability_reported"
	RuntimeEventCapabilityDegraded  RuntimeEventType = "capability_degraded"
	RuntimeEventCommandEvent        RuntimeEventType = "command_event"
	RuntimeEventCommandCompleted    RuntimeEventType = "command_completed"
	RuntimeEventCommandFailed       RuntimeEventType = "command_failed"
	RuntimeEventCommandCancelled    RuntimeEventType = "command_cancelled"
	RuntimeEventCommandTimedOut     RuntimeEventType = "command_timed_out"
)

const (
	RuntimeEventSeverityInfo    RuntimeEventSeverity = "info"
	RuntimeEventSeveritySuccess RuntimeEventSeverity = "success"
	RuntimeEventSeverityWarning RuntimeEventSeverity = "warning"
	RuntimeEventSeverityError   RuntimeEventSeverity = "error"
)

const (
	RuntimeEventSourceEnrollment     RuntimeEventSource = "runtime_enrollment"
	RuntimeEventSourceRuntimeNode    RuntimeEventSource = "runtime_node"
	RuntimeEventSourceCapability     RuntimeEventSource = "runtime_capability"
	RuntimeEventSourceRuntimeCommand RuntimeEventSource = "runtime_command"
	RuntimeEventSourceProvider       RuntimeEventSource = "provider_session"
)

func (t RuntimeEventType) IsValid() bool {
	switch t {
	case RuntimeEventEnrollmentRequested,
		RuntimeEventEnrollmentApproved,
		RuntimeEventEnrollmentRejected,
		RuntimeEventEnrollmentRevoked,
		RuntimeEventNodeOnline,
		RuntimeEventNodeOffline,
		RuntimeEventCapabilityReported,
		RuntimeEventCapabilityDegraded,
		RuntimeEventCommandEvent,
		RuntimeEventCommandCompleted,
		RuntimeEventCommandFailed,
		RuntimeEventCommandCancelled,
		RuntimeEventCommandTimedOut:
		return true
	}
	return false
}

func (s RuntimeEventSeverity) IsValid() bool {
	switch s {
	case RuntimeEventSeverityInfo,
		RuntimeEventSeveritySuccess,
		RuntimeEventSeverityWarning,
		RuntimeEventSeverityError:
		return true
	}
	return false
}

func (s RuntimeEventSource) IsValid() bool {
	switch s {
	case RuntimeEventSourceEnrollment,
		RuntimeEventSourceRuntimeNode,
		RuntimeEventSourceCapability,
		RuntimeEventSourceRuntimeCommand,
		RuntimeEventSourceProvider:
		return true
	}
	return false
}

type RuntimeEvent struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	RuntimeNodeID   uuid.UUID
	NodeID          string
	EventType       RuntimeEventType
	Severity        RuntimeEventSeverity
	Source          RuntimeEventSource
	Title           string
	Description     string
	ProviderType    string
	CorrelationType string
	CorrelationID   string
	Payload         map[string]interface{}
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type CreateRuntimeEventRequest struct {
	TenantID        uuid.UUID
	RuntimeNodeID   uuid.UUID
	NodeID          string
	EventType       RuntimeEventType
	Severity        RuntimeEventSeverity
	Source          RuntimeEventSource
	Title           string
	Description     string
	ProviderType    string
	CorrelationType string
	CorrelationID   string
	Payload         map[string]interface{}
}

type ListRuntimeEventsFilter struct {
	TenantID     uuid.UUID
	EventType    *RuntimeEventType
	Severity     *RuntimeEventSeverity
	NodeID       *string
	ProviderType *string
	Limit        int32
	Offset       int32
}

type RuntimeOverviewFilter struct {
	TenantID uuid.UUID
}

type RuntimeOverview struct {
	Summary              RuntimeOverviewSummary
	PendingEnrollments   []*RuntimeEnrollment
	Nodes                []*Node
	ProviderCapabilities []RuntimeProviderCapabilitySummary
	RecentEvents         []RuntimeEvent
}

type RuntimeOverviewSummary struct {
	OnlineNodes            int64
	TotalNodes             int64
	PendingEnrollments     int64
	ActiveProviderSessions int64
	BlockedEvents          int64
}

type RuntimeProviderCapabilitySummary struct {
	ProviderType   string
	NodeCount      int64
	AvailableCount int64
	HealthyCount   int64
	LastSeenAt     time.Time
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

type HeartbeatResponse struct {
	Node           *Node
	RequiredTools  []string
	PlatformLimits *PlatformLimits
}

// PlatformLimits 是经心跳下发给 runtime 的平台限额快照(P2 spec §3)。
// 值来源=系统配置中心生效值,由 app 装配层以 PlatformLimitsResolver 闭包注入
// (runtime 包被 api/middleware 反向依赖,不能 import systemconfig)。
type PlatformLimits struct {
	ArtifactMaxFileSizeBytes   int64
	AttachmentMaxFileSizeBytes int64
	AttachmentMaxCount         int64
	AttachmentTotalMaxBytes    int64
	SkillArchiveMaxBytes       int64
	SkillArchiveMaxFileCount   int64
	// WorkspaceBaseDir 是平台约定的工作区根(系统配置 runtime.workspace_base_dir);
	// 节点本地 config/env 仍可覆盖。空串表示未下发(agent 保持本地默认)。
	WorkspaceBaseDir string
}

// Fingerprint 返回对限额有序序列化的稳定指纹,runtime 据此判断"没变就不动"。
func (l PlatformLimits) Fingerprint() string {
	canonical := fmt.Sprintf(
		"artifact_max_file_size_bytes=%d;attachment_max_file_size_bytes=%d;attachment_max_count=%d;attachment_total_max_bytes=%d;skill_archive_max_bytes=%d;skill_archive_max_file_count=%d;workspace_base_dir=%s",
		l.ArtifactMaxFileSizeBytes,
		l.AttachmentMaxFileSizeBytes,
		l.AttachmentMaxCount,
		l.AttachmentTotalMaxBytes,
		l.SkillArchiveMaxBytes,
		l.SkillArchiveMaxFileCount,
		l.WorkspaceBaseDir,
	)
	sum := sha256.Sum256([]byte(canonical))
	return "plv1:sha256:" + hex.EncodeToString(sum[:])
}

// IsOnline checks if the node is online based on heartbeat
// A node is considered online if it has sent a heartbeat within HeartbeatTimeout.
// 需要按配置生效值判定时用 IsOnlineAt;本便捷形态固定用默认超时。
func (n *Node) IsOnline() bool {
	return n.IsOnlineAt(HeartbeatTimeout)
}

// IsOnlineAt 按调用方解析出的超时阈值判定在线;model 方法不做 IO,
// 阈值由调用方经 systemconfig(HeartbeatTimeoutResolver)解析后传入。
func (n *Node) IsOnlineAt(timeout time.Duration) bool {
	return time.Since(n.LastHeartbeatAt) <= timeout
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
	TenantID    uuid.UUID
	NodeID      string
	CurrentLoad int32
	// SupportsPlatformLimits 是节点能力自报(版本偏斜护栏 P2 spec §5):
	// 新 agent 声明自己会消费心跳下发的 platform_limits 快照。
	SupportsPlatformLimits bool
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

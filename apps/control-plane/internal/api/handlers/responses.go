package handlers

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/task"
)

type taskResponse struct {
	ID             string          `json:"id"`
	TenantID       string          `json:"tenant_id"`
	TeamID         *string         `json:"team_id,omitempty"`
	Title          string          `json:"title"`
	Description    *string         `json:"description,omitempty"`
	CreatorID      *string         `json:"creator_id,omitempty"`
	ProviderType   string          `json:"provider_type"`
	TargetNodeID   *string         `json:"target_node_id,omitempty"`
	AssignedNodeID *string         `json:"assigned_node_id,omitempty"`
	Status         task.TaskStatus `json:"status"`
	WorkspacePath  *string         `json:"workspace_path,omitempty"`
	Params         json.RawMessage `json:"params"`
	Priority       int32           `json:"priority"`
	CancelledAt    *string         `json:"cancelled_at,omitempty"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
}

type runtimeNodeResponse struct {
	RuntimeNodeID           string                 `json:"runtime_node_id,omitempty"`
	NodeID                  string                 `json:"node_id"`
	Name                    string                 `json:"name"`
	SupportedProviders      []string               `json:"supported_providers"`
	MaxSlots                int32                  `json:"max_slots"`
	CurrentLoad             int32                  `json:"current_load"`
	Status                  runtime.NodeStatus     `json:"status"`
	CommandChannelConnected *bool                  `json:"command_channel_connected,omitempty"`
	Metadata                map[string]interface{} `json:"metadata,omitempty"`
	LastHeartbeatAt         string                 `json:"last_heartbeat_at,omitempty"`
	CreatedAt               string                 `json:"created_at,omitempty"`
	UpdatedAt               string                 `json:"updated_at,omitempty"`
}

type runtimeEnrollmentResponse struct {
	ID             string                          `json:"id"`
	TenantID       string                          `json:"tenant_id"`
	RuntimeNodeID  string                          `json:"runtime_node_id,omitempty"`
	NodeID         string                          `json:"node_id"`
	BootstrapKeyID string                          `json:"bootstrap_key_id"`
	Status         runtime.RuntimeEnrollmentStatus `json:"status"`
	RequestPayload map[string]interface{}          `json:"request_payload,omitempty"`
	ApprovedBy     string                          `json:"approved_by,omitempty"`
	ApprovedAt     string                          `json:"approved_at,omitempty"`
	RejectedBy     string                          `json:"rejected_by,omitempty"`
	RejectedAt     string                          `json:"rejected_at,omitempty"`
	RejectReason   *string                         `json:"reject_reason,omitempty"`
	RevokedBy      string                          `json:"revoked_by,omitempty"`
	RevokedAt      string                          `json:"revoked_at,omitempty"`
	RevokeReason   *string                         `json:"revoke_reason,omitempty"`
	LastHelloAt    string                          `json:"last_hello_at,omitempty"`
	CreatedAt      string                          `json:"created_at,omitempty"`
	UpdatedAt      string                          `json:"updated_at,omitempty"`
}

type runtimeSessionResponse struct {
	ID            string  `json:"id"`
	TenantID      string  `json:"tenant_id"`
	RuntimeNodeID string  `json:"runtime_node_id"`
	NodeID        string  `json:"node_id,omitempty"`
	EnrollmentID  string  `json:"enrollment_id,omitempty"`
	ExpiresAt     string  `json:"expires_at,omitempty"`
	LastSeenAt    string  `json:"last_seen_at,omitempty"`
	RevokedAt     string  `json:"revoked_at,omitempty"`
	RevokedReason *string `json:"revoked_reason,omitempty"`
	CreatedAt     string  `json:"created_at,omitempty"`
	UpdatedAt     string  `json:"updated_at,omitempty"`
}

type runtimeCapabilityResponse struct {
	ID             string `json:"id"`
	TenantID       string `json:"tenant_id"`
	RuntimeNodeID  string `json:"runtime_node_id"`
	CapabilityType string `json:"capability_type"`
	CapabilityKey  string `json:"capability_key"`
	ProviderType   string `json:"provider_type"`
	Available      bool   `json:"available"`
	Status         string `json:"status"`
	HealthStatus   string `json:"health_status"`
	LastSeenAt     string `json:"last_seen_at,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
	UpdatedAt      string `json:"updated_at,omitempty"`
}

type runtimeEnrollmentHelloResponse struct {
	Enrollment   runtimeEnrollmentResponse `json:"enrollment"`
	Session      *runtimeSessionResponse   `json:"session,omitempty"`
	SessionToken string                    `json:"session_token,omitempty"`
}

type runtimeSessionRenewResponse struct {
	Session runtimeSessionResponse `json:"session"`
}

type runtimeCapabilitiesResponse struct {
	Capabilities []runtimeCapabilityResponse `json:"capabilities"`
}

func newTaskResponse(t *task.Task) taskResponse {
	return taskResponse{
		ID:             t.ID.String(),
		TenantID:       t.TenantID.String(),
		TeamID:         optionalUUIDString(t.TeamID),
		Title:          t.Title,
		Description:    t.Description,
		CreatorID:      optionalUUIDString(t.CreatorID),
		ProviderType:   t.ProviderType,
		TargetNodeID:   t.TargetNodeID,
		AssignedNodeID: t.AssignedNodeID,
		Status:         t.Status,
		WorkspacePath:  t.WorkspacePath,
		Params:         normalizeTaskParams(t.Params),
		Priority:       t.Priority,
		CancelledAt:    optionalTimeString(t.CancelledAt),
		CreatedAt:      t.CreatedAt.UTC().Format(timeRFC3339Nano),
		UpdatedAt:      t.UpdatedAt.UTC().Format(timeRFC3339Nano),
	}
}

func optionalUUIDString(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}

func optionalTimeString(value *time.Time) *string {
	if value == nil {
		return nil
	}
	text := value.UTC().Format(timeRFC3339Nano)
	return &text
}

func nullUUIDString(value uuid.NullUUID) string {
	if !value.Valid {
		return ""
	}
	return value.UUID.String()
}

func newTaskResponses(tasks []*task.Task) []taskResponse {
	responses := make([]taskResponse, 0, len(tasks))
	for _, t := range tasks {
		responses = append(responses, newTaskResponse(t))
	}
	return responses
}

func normalizeTaskParams(raw []byte) json.RawMessage {
	trimmed := json.RawMessage(`{}`)
	if len(raw) == 0 {
		return trimmed
	}

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return trimmed
	}
	object, ok := decoded.(map[string]any)
	if !ok {
		return trimmed
	}

	normalized, err := json.Marshal(object)
	if err != nil {
		return trimmed
	}
	return json.RawMessage(normalized)
}

func newRuntimeNodeResponse(node *runtime.Node) runtimeNodeResponse {
	response := runtimeNodeResponse{
		NodeID:             node.NodeID,
		Name:               node.Name,
		SupportedProviders: append([]string(nil), node.SupportedProviders...),
		MaxSlots:           node.MaxSlots,
		CurrentLoad:        node.CurrentLoad,
		Status:             node.Status,
		Metadata:           node.Metadata,
	}
	if node.ID != uuid.Nil {
		response.RuntimeNodeID = node.ID.String()
	}
	if !node.LastHeartbeatAt.IsZero() {
		response.LastHeartbeatAt = node.LastHeartbeatAt.UTC().Format(timeRFC3339Nano)
	}
	if !node.CreatedAt.IsZero() {
		response.CreatedAt = node.CreatedAt.UTC().Format(timeRFC3339Nano)
	}
	if !node.UpdatedAt.IsZero() {
		response.UpdatedAt = node.UpdatedAt.UTC().Format(timeRFC3339Nano)
	}
	return response
}

func newRuntimeNodeResponseWithCommandChannel(node *runtime.Node, connected bool) runtimeNodeResponse {
	response := newRuntimeNodeResponse(node)
	response.CommandChannelConnected = &connected
	return response
}

func newRuntimeNodeResponses(nodes []*runtime.Node) []runtimeNodeResponse {
	responses := make([]runtimeNodeResponse, 0, len(nodes))
	for _, node := range nodes {
		responses = append(responses, newRuntimeNodeResponse(node))
	}
	return responses
}

func newRuntimeEnrollmentResponse(enrollment *runtime.RuntimeEnrollment) runtimeEnrollmentResponse {
	response := runtimeEnrollmentResponse{
		ID:             enrollment.ID.String(),
		TenantID:       enrollment.TenantID.String(),
		NodeID:         enrollment.NodeID,
		BootstrapKeyID: enrollment.BootstrapKeyID.String(),
		Status:         enrollment.Status,
		RequestPayload: enrollment.RequestPayload,
		ApprovedBy:     nullUUIDString(enrollment.ApprovedBy),
		RejectedBy:     nullUUIDString(enrollment.RejectedBy),
		RejectReason:   enrollment.RejectReason,
		RevokedBy:      nullUUIDString(enrollment.RevokedBy),
		RevokeReason:   enrollment.RevokeReason,
	}
	if enrollment.RuntimeNodeID != uuid.Nil {
		response.RuntimeNodeID = enrollment.RuntimeNodeID.String()
	}
	if !enrollment.ApprovedAt.IsZero() {
		response.ApprovedAt = enrollment.ApprovedAt.UTC().Format(timeRFC3339Nano)
	}
	if !enrollment.RejectedAt.IsZero() {
		response.RejectedAt = enrollment.RejectedAt.UTC().Format(timeRFC3339Nano)
	}
	if !enrollment.RevokedAt.IsZero() {
		response.RevokedAt = enrollment.RevokedAt.UTC().Format(timeRFC3339Nano)
	}
	if !enrollment.LastHelloAt.IsZero() {
		response.LastHelloAt = enrollment.LastHelloAt.UTC().Format(timeRFC3339Nano)
	}
	if !enrollment.CreatedAt.IsZero() {
		response.CreatedAt = enrollment.CreatedAt.UTC().Format(timeRFC3339Nano)
	}
	if !enrollment.UpdatedAt.IsZero() {
		response.UpdatedAt = enrollment.UpdatedAt.UTC().Format(timeRFC3339Nano)
	}
	return response
}

func newRuntimeEnrollmentResponses(enrollments []*runtime.RuntimeEnrollment) []runtimeEnrollmentResponse {
	responses := make([]runtimeEnrollmentResponse, 0, len(enrollments))
	for _, enrollment := range enrollments {
		responses = append(responses, newRuntimeEnrollmentResponse(enrollment))
	}
	return responses
}

func newRuntimeSessionResponse(session *runtime.RuntimeSession) runtimeSessionResponse {
	response := runtimeSessionResponse{
		ID:            session.ID.String(),
		TenantID:      session.TenantID.String(),
		RuntimeNodeID: session.RuntimeNodeID.String(),
		NodeID:        session.NodeID,
		EnrollmentID:  nullUUIDString(session.EnrollmentID),
		RevokedReason: session.RevokedReason,
	}
	if !session.ExpiresAt.IsZero() {
		response.ExpiresAt = session.ExpiresAt.UTC().Format(timeRFC3339Nano)
	}
	if !session.LastSeenAt.IsZero() {
		response.LastSeenAt = session.LastSeenAt.UTC().Format(timeRFC3339Nano)
	}
	if !session.RevokedAt.IsZero() {
		response.RevokedAt = session.RevokedAt.UTC().Format(timeRFC3339Nano)
	}
	if !session.CreatedAt.IsZero() {
		response.CreatedAt = session.CreatedAt.UTC().Format(timeRFC3339Nano)
	}
	if !session.UpdatedAt.IsZero() {
		response.UpdatedAt = session.UpdatedAt.UTC().Format(timeRFC3339Nano)
	}
	return response
}

func newRuntimeCapabilityResponse(capability runtime.RuntimeCapability) runtimeCapabilityResponse {
	response := runtimeCapabilityResponse{
		ID:             capability.ID.String(),
		TenantID:       capability.TenantID.String(),
		RuntimeNodeID:  capability.RuntimeNodeID.String(),
		CapabilityType: capability.CapabilityType,
		CapabilityKey:  capability.CapabilityKey,
		ProviderType:   capability.ProviderType,
		Available:      capability.Available,
		Status:         capability.Status,
		HealthStatus:   capability.HealthStatus,
	}
	if !capability.LastSeenAt.IsZero() {
		response.LastSeenAt = capability.LastSeenAt.UTC().Format(timeRFC3339Nano)
	}
	if !capability.CreatedAt.IsZero() {
		response.CreatedAt = capability.CreatedAt.UTC().Format(timeRFC3339Nano)
	}
	if !capability.UpdatedAt.IsZero() {
		response.UpdatedAt = capability.UpdatedAt.UTC().Format(timeRFC3339Nano)
	}
	return response
}

func newRuntimeCapabilityResponses(capabilities []runtime.RuntimeCapability) []runtimeCapabilityResponse {
	responses := make([]runtimeCapabilityResponse, 0, len(capabilities))
	for _, capability := range capabilities {
		responses = append(responses, newRuntimeCapabilityResponse(capability))
	}
	return responses
}

func newRuntimeEnrollmentHelloResponse(resp *runtime.EnrollHelloResponse) runtimeEnrollmentHelloResponse {
	response := runtimeEnrollmentHelloResponse{
		Enrollment:   newRuntimeEnrollmentResponse(&resp.Enrollment),
		SessionToken: resp.SessionToken,
	}
	if resp.Session != nil {
		session := newRuntimeSessionResponse(resp.Session)
		response.Session = &session
	}
	return response
}

const timeRFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"

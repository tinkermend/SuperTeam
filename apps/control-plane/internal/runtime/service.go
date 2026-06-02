package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNodeNotFound          = errors.New("node not found")
	ErrNodeAlreadyExists     = errors.New("node already exists")
	ErrInvalidStatus         = errors.New("invalid node status")
	ErrEnrollmentUnsupported = errors.New("runtime enrollment repository is required")
	ErrInvalidBootstrapKey   = errors.New("invalid bootstrap key")
	ErrInvalidRuntimeSession = errors.New("invalid runtime session")
)

const (
	// HeartbeatTimeout is the duration after which a node is considered offline
	HeartbeatTimeout  = 60 * time.Second
	RuntimeSessionTTL = 12 * time.Hour
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, errors.New("runtime repository is required")
	}
	return &Service{
		repository: repository,
	}, nil
}

func (s *Service) EnrollHello(ctx context.Context, req EnrollHelloRequest) (*EnrollHelloResponse, error) {
	enrollmentRepo, err := s.enrollmentRepository()
	if err != nil {
		return nil, err
	}
	if req.NodeID == "" {
		return nil, errors.New("node_id is required")
	}
	if req.BootstrapKey == "" {
		return nil, errors.New("bootstrap_key is required")
	}

	tenantID := tenantOrDefault(req.TenantID)
	bootstrapKey, err := s.findBootstrapKey(ctx, enrollmentRepo, tenantID, req.BootstrapKey)
	if err != nil {
		return nil, err
	}

	nodeName := req.Name
	if nodeName == "" {
		nodeName = req.NodeID
	}
	supportedProviders := req.SupportedProviders
	if len(supportedProviders) == 0 {
		supportedProviders = []string{"runtime"}
	}
	maxSlots := req.MaxSlots
	if maxSlots <= 0 {
		maxSlots = 1
	}
	metadata := req.Metadata
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	if req.Version != "" {
		metadata["version"] = req.Version
	}

	payload, err := json.Marshal(map[string]interface{}{
		"node_id":             req.NodeID,
		"name":                nodeName,
		"supported_providers": supportedProviders,
		"max_slots":           maxSlots,
		"metadata":            metadata,
		"version":             req.Version,
		"capability_count":    len(req.Capabilities),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to serialize enrollment payload: %w", err)
	}

	enrollmentRecord, err := enrollmentRepo.UpsertRuntimeEnrollmentFromHello(ctx, UpsertRuntimeEnrollmentFromHelloParams{
		TenantID:       tenantID,
		NodeID:         req.NodeID,
		BootstrapKeyID: bootstrapKey.ID,
		RequestPayload: payload,
		LastHelloAt:    timestamptzFromTime(time.Now()),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to upsert runtime enrollment: %w", err)
	}

	enrollment, err := s.recordToRuntimeEnrollment(enrollmentRecord)
	if err != nil {
		return nil, err
	}
	resp := &EnrollHelloResponse{Enrollment: *enrollment}
	if enrollment.Status == RuntimeEnrollmentStatusApproved && enrollment.RuntimeNodeID != uuid.Nil {
		session, token, err := s.IssueRuntimeSession(ctx, enrollmentRecord)
		if err != nil {
			return nil, err
		}
		resp.Session = session
		resp.SessionToken = token
	}
	return resp, nil
}

func (s *Service) ApproveEnrollment(ctx context.Context, req ApproveEnrollmentRequest) (*RuntimeEnrollment, error) {
	enrollmentRepo, err := s.enrollmentRepository()
	if err != nil {
		return nil, err
	}
	if req.EnrollmentID == uuid.Nil {
		return nil, errors.New("enrollment_id is required")
	}
	tenantID := tenantOrDefault(req.TenantID)
	enrollment, err := enrollmentRepo.GetRuntimeEnrollment(ctx, tenantID, req.EnrollmentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get runtime enrollment: %w", err)
	}
	if enrollment.Status != RuntimeEnrollmentStatusPending {
		return nil, errors.New("runtime enrollment must be pending")
	}
	nodeRequest, err := parseRuntimeNodeRequest(enrollment)
	if err != nil {
		return nil, err
	}
	record, err := enrollmentRepo.ApproveRuntimeEnrollmentWithNode(ctx, ApproveRuntimeEnrollmentWithNodeParams{
		TenantID:           tenantID,
		EnrollmentID:       req.EnrollmentID,
		ApprovedBy:         req.ApprovedBy,
		Name:               nodeRequest.Name,
		SupportedProviders: nodeRequest.SupportedProviders,
		MaxSlots:           nodeRequest.MaxSlots,
		CurrentLoad:        0,
		NodeStatus:         string(NodeStatusOnline),
		Metadata:           nodeRequest.Metadata,
		LastHeartbeatAt:    timestamptzFromTime(time.Now()),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to approve runtime enrollment: %w", err)
	}
	return s.recordToRuntimeEnrollment(record)
}

func (s *Service) RejectEnrollment(ctx context.Context, req RejectEnrollmentRequest) (*RuntimeEnrollment, error) {
	enrollmentRepo, err := s.enrollmentRepository()
	if err != nil {
		return nil, err
	}
	if req.EnrollmentID == uuid.Nil {
		return nil, errors.New("enrollment_id is required")
	}
	record, err := enrollmentRepo.RejectRuntimeEnrollment(ctx, RejectRuntimeEnrollmentParams{
		TenantID:     tenantOrDefault(req.TenantID),
		EnrollmentID: req.EnrollmentID,
		RejectedBy:   req.RejectedBy,
		Reason:       req.Reason,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to reject runtime enrollment: %w", err)
	}
	return s.recordToRuntimeEnrollment(record)
}

func (s *Service) RevokeEnrollment(ctx context.Context, req RevokeEnrollmentRequest) (*RuntimeEnrollment, error) {
	enrollmentRepo, err := s.enrollmentRepository()
	if err != nil {
		return nil, err
	}
	if req.EnrollmentID == uuid.Nil {
		return nil, errors.New("enrollment_id is required")
	}
	record, err := enrollmentRepo.RevokeRuntimeEnrollment(ctx, RevokeRuntimeEnrollmentParams{
		TenantID:     tenantOrDefault(req.TenantID),
		EnrollmentID: req.EnrollmentID,
		RevokedBy:    req.RevokedBy,
		Reason:       req.Reason,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to revoke runtime enrollment: %w", err)
	}
	return s.recordToRuntimeEnrollment(record)
}

func (s *Service) IssueRuntimeSession(ctx context.Context, enrollment RuntimeEnrollmentRecord) (*RuntimeSession, string, error) {
	enrollmentRepo, err := s.enrollmentRepository()
	if err != nil {
		return nil, "", err
	}
	if enrollment.Status != RuntimeEnrollmentStatusApproved {
		return nil, "", errors.New("runtime enrollment must be approved")
	}
	if enrollment.RuntimeNodeID == uuid.Nil {
		return nil, "", errors.New("runtime enrollment has no attached runtime node")
	}
	token, err := GenerateRuntimeSessionToken()
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate runtime session token: %w", err)
	}
	secretHash, err := HashRuntimeSecret(token)
	if err != nil {
		return nil, "", fmt.Errorf("failed to hash runtime session token: %w", err)
	}
	session, err := enrollmentRepo.CreateRuntimeSession(ctx, CreateRuntimeSessionParams{
		TenantID:        tenantOrDefault(enrollment.TenantID),
		RuntimeNodeID:   enrollment.RuntimeNodeID,
		EnrollmentID:    enrollment.ID,
		TokenLookupHash: LookupRuntimeSessionTokenHash(token),
		TokenSecretHash: secretHash,
		ExpiresAt:       timestamptzFromTime(time.Now().Add(RuntimeSessionTTL)),
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to create runtime session: %w", err)
	}
	session.NodeID = enrollment.NodeID
	domainSession := s.recordToRuntimeSession(session)
	return domainSession, token, nil
}

func (s *Service) RenewRuntimeSession(ctx context.Context, token string) (*RuntimeSession, error) {
	validation, err := s.ValidateRuntimeSession(ctx, token)
	if err != nil {
		return nil, err
	}
	enrollmentRepo, err := s.enrollmentRepository()
	if err != nil {
		return nil, err
	}
	record, err := enrollmentRepo.RenewRuntimeSession(ctx, RenewRuntimeSessionParams{
		TenantID:  validation.TenantID,
		SessionID: validation.SessionID,
		ExpiresAt: timestamptzFromTime(time.Now().Add(RuntimeSessionTTL)),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRuntimeSession, err)
	}
	record.NodeID = validation.NodeID
	return s.recordToRuntimeSession(record), nil
}

func (s *Service) ValidateRuntimeSession(ctx context.Context, token string) (*RuntimeSessionValidation, error) {
	enrollmentRepo, err := s.enrollmentRepository()
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, errors.New("runtime session token is required")
	}
	session, err := enrollmentRepo.GetActiveRuntimeSessionByLookupHash(ctx, GetActiveRuntimeSessionByLookupHashParams{
		TokenLookupHash: LookupRuntimeSessionTokenHash(token),
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRuntimeSession, err)
	}
	if !VerifyRuntimeSecret(token, session.TokenSecretHash) {
		return nil, ErrInvalidRuntimeSession
	}
	touched, err := enrollmentRepo.TouchRuntimeSession(ctx, TouchRuntimeSessionParams{
		TenantID:  session.TenantID,
		SessionID: session.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRuntimeSession, err)
	}
	if touched.NodeID == "" {
		touched.NodeID = session.NodeID
	}
	return &RuntimeSessionValidation{
		SessionID:     touched.ID,
		TenantID:      touched.TenantID,
		RuntimeNodeID: touched.RuntimeNodeID,
		NodeID:        touched.NodeID,
		EnrollmentID:  touched.EnrollmentID,
		ExpiresAt:     timeFromTimestamptz(touched.ExpiresAt),
	}, nil
}

func (s *Service) UpsertCapabilities(ctx context.Context, token string, capabilities []RuntimeCapabilityInput) ([]RuntimeCapability, error) {
	validation, err := s.ValidateRuntimeSession(ctx, token)
	if err != nil {
		return nil, err
	}
	capabilityRepo, ok := s.repository.(CapabilityRepository)
	if !ok {
		return nil, errors.New("runtime capability repository is required")
	}
	results := make([]RuntimeCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		if capability.CapabilityType == "" {
			return nil, errors.New("capability_type is required")
		}
		if capability.CapabilityKey == "" {
			return nil, errors.New("capability_key is required")
		}
		if capability.ProviderType == "" {
			return nil, errors.New("provider_type is required")
		}
		status := capability.Status
		if status == "" {
			status = "active"
		}
		healthStatus := capability.HealthStatus
		if healthStatus == "" {
			healthStatus = "unknown"
		}
		capacity, err := json.Marshal(defaultMap(capability.Capacity))
		if err != nil {
			return nil, fmt.Errorf("failed to serialize capability capacity: %w", err)
		}
		labels, err := json.Marshal(defaultMap(capability.Labels))
		if err != nil {
			return nil, fmt.Errorf("failed to serialize capability labels: %w", err)
		}
		details, err := json.Marshal(defaultMap(capability.Details))
		if err != nil {
			return nil, fmt.Errorf("failed to serialize capability details: %w", err)
		}
		metadata, err := json.Marshal(defaultMap(capability.Metadata))
		if err != nil {
			return nil, fmt.Errorf("failed to serialize capability metadata: %w", err)
		}
		result, err := capabilityRepo.UpsertRuntimeCapability(ctx, UpsertRuntimeCapabilityParams{
			TenantID:         validation.TenantID,
			RuntimeNodeID:    validation.RuntimeNodeID,
			CapabilityType:   capability.CapabilityType,
			CapabilityKey:    capability.CapabilityKey,
			ProviderType:     capability.ProviderType,
			ProviderVersion:  capability.ProviderVersion,
			BinaryPath:       capability.BinaryPath,
			Available:        capability.Available,
			WorkspaceBaseDir: capability.WorkspaceBaseDir,
			Capacity:         capacity,
			Labels:           labels,
			Status:           status,
			Details:          details,
			HealthStatus:     healthStatus,
			Metadata:         metadata,
			LastSeenAt:       timestamptzFromTime(time.Now()),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to upsert runtime capability: %w", err)
		}
		results = append(results, result)
	}
	return results, nil
}

// RegisterNode registers a new runtime node or updates an existing one
func (s *Service) RegisterNode(ctx context.Context, req RegisterNodeRequest) (*Node, error) {
	// Validate request
	if req.NodeID == "" {
		return nil, errors.New("node_id is required")
	}
	if req.Name == "" {
		return nil, errors.New("name is required")
	}
	if req.MaxSlots <= 0 {
		return nil, errors.New("max_slots must be greater than 0")
	}
	if len(req.SupportedProviders) == 0 {
		return nil, errors.New("supported_providers is required")
	}

	// Serialize supported providers
	providersJSON, err := json.Marshal(req.SupportedProviders)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize supported_providers: %w", err)
	}

	// Serialize metadata
	var metadataJSON []byte
	if req.Metadata != nil {
		metadataJSON, err = json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize metadata: %w", err)
		}
	} else {
		metadataJSON = []byte("{}")
	}

	// Check if node already exists
	_, err = s.repository.GetNode(ctx, req.NodeID)
	if err == nil {
		// Node exists, update it
		// Update heartbeat
		_, err = s.repository.UpdateHeartbeat(ctx, UpdateHeartbeatParams{
			NodeID:          req.NodeID,
			LastHeartbeatAt: timestamptzFromTime(time.Now()),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to update heartbeat: %w", err)
		}

		// Update status to online
		record, err := s.repository.UpdateStatus(ctx, UpdateStatusParams{
			NodeID: req.NodeID,
			Status: string(NodeStatusOnline),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to update status: %w", err)
		}

		return s.recordToNode(record)
	}

	// Node doesn't exist, create it
	params := CreateNodeParams{
		NodeID:             req.NodeID,
		Name:               req.Name,
		SupportedProviders: providersJSON,
		MaxSlots:           req.MaxSlots,
		CurrentLoad:        0,
		Status:             string(NodeStatusOnline),
		Metadata:           metadataJSON,
		LastHeartbeatAt:    timestamptzFromTime(time.Now()),
	}

	record, err := s.repository.CreateNode(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to create node: %w", err)
	}

	return s.recordToNode(record)
}

// UpdateHeartbeat updates the heartbeat and load of a node
func (s *Service) UpdateHeartbeat(ctx context.Context, req UpdateHeartbeatRequest) (*Node, error) {
	// Validate request
	if req.NodeID == "" {
		return nil, errors.New("node_id is required")
	}
	if req.CurrentLoad < 0 {
		return nil, errors.New("current_load must be non-negative")
	}

	// Update heartbeat
	record, err := s.repository.UpdateHeartbeat(ctx, UpdateHeartbeatParams{
		NodeID:          req.NodeID,
		LastHeartbeatAt: timestamptzFromTime(time.Now()),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update heartbeat: %w", err)
	}

	// Update load
	record, err = s.repository.UpdateLoad(ctx, UpdateLoadParams{
		NodeID:      req.NodeID,
		CurrentLoad: req.CurrentLoad,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update load: %w", err)
	}

	// Determine status based on heartbeat
	node, err := s.recordToNode(record)
	if err != nil {
		return nil, err
	}

	// Update status if needed
	expectedStatus := NodeStatusOnline
	if !node.IsOnline() {
		expectedStatus = NodeStatusOffline
	}

	if node.Status != expectedStatus {
		record, err = s.repository.UpdateStatus(ctx, UpdateStatusParams{
			NodeID: req.NodeID,
			Status: string(expectedStatus),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to update status: %w", err)
		}
		node, err = s.recordToNode(record)
		if err != nil {
			return nil, err
		}
	}

	return node, nil
}

// GetNode retrieves a node by ID
func (s *Service) GetNode(ctx context.Context, nodeID string) (*Node, error) {
	if nodeID == "" {
		return nil, errors.New("node_id is required")
	}

	record, err := s.repository.GetNode(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	return s.recordToNode(record)
}

// ListNodes lists all nodes with optional filters
func (s *Service) ListNodes(ctx context.Context, filter ListNodesFilter) ([]*Node, error) {
	// Set default limit if not specified
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 100 {
		filter.Limit = 100 // Max limit
	}

	params := ListNodesParams{
		Status: s.statusToText(filter.Status),
		Offset: filter.Offset,
		Limit:  filter.Limit,
	}

	records, err := s.repository.ListNodes(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	nodes := make([]*Node, 0, len(records))
	for _, record := range records {
		node, err := s.recordToNode(record)
		if err != nil {
			// Skip invalid nodes
			continue
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// ListOnlineNodes lists all online nodes (heartbeat within threshold)
func (s *Service) ListOnlineNodes(ctx context.Context) ([]*Node, error) {
	threshold := time.Now().Add(-HeartbeatTimeout)
	records, err := s.repository.ListOnlineNodes(ctx, timestamptzFromTime(threshold))
	if err != nil {
		return nil, fmt.Errorf("failed to list online nodes: %w", err)
	}

	nodes := make([]*Node, 0, len(records))
	for _, record := range records {
		node, err := s.recordToNode(record)
		if err != nil {
			// Skip invalid nodes
			continue
		}
		nodes = append(nodes, node)
	}

	return nodes, nil
}

// Helper methods

func (s *Service) recordToNode(record NodeRecord) (*Node, error) {
	// Deserialize supported providers
	var supportedProviders []string
	if err := json.Unmarshal(record.SupportedProviders, &supportedProviders); err != nil {
		return nil, fmt.Errorf("failed to deserialize supported_providers: %w", err)
	}

	// Deserialize metadata
	var metadata map[string]interface{}
	if len(record.Metadata) > 0 {
		if err := json.Unmarshal(record.Metadata, &metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize metadata: %w", err)
		}
	}

	return &Node{
		ID:                 record.ID,
		NodeID:             record.NodeID,
		Name:               record.Name,
		SupportedProviders: supportedProviders,
		MaxSlots:           record.MaxSlots,
		CurrentLoad:        record.CurrentLoad,
		Status:             NodeStatus(record.Status),
		Metadata:           metadata,
		LastHeartbeatAt:    timeFromTimestamptz(record.LastHeartbeatAt),
		CreatedAt:          timeFromTimestamptz(record.CreatedAt),
		UpdatedAt:          timeFromTimestamptz(record.UpdatedAt),
	}, nil
}

func (s *Service) statusToText(status *NodeStatus) pgtype.Text {
	if status == nil {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: string(*status), Valid: true}
}

func (s *Service) enrollmentRepository() (EnrollmentRepository, error) {
	repo, ok := s.repository.(EnrollmentRepository)
	if !ok {
		return nil, ErrEnrollmentUnsupported
	}
	return repo, nil
}

func (s *Service) findBootstrapKey(ctx context.Context, repo EnrollmentRepository, tenantID uuid.UUID, secret string) (RuntimeBootstrapKeyRecord, error) {
	keys, err := repo.ListActiveRuntimeBootstrapKeys(ctx, tenantID)
	if err != nil {
		return RuntimeBootstrapKeyRecord{}, fmt.Errorf("failed to list active runtime bootstrap keys: %w", err)
	}
	for _, key := range keys {
		if VerifyRuntimeSecret(secret, key.KeyHash) {
			return key, nil
		}
	}
	return RuntimeBootstrapKeyRecord{}, ErrInvalidBootstrapKey
}

func (s *Service) recordToRuntimeEnrollment(record RuntimeEnrollmentRecord) (*RuntimeEnrollment, error) {
	var payload map[string]interface{}
	if len(record.RequestPayload) > 0 {
		if err := json.Unmarshal(record.RequestPayload, &payload); err != nil {
			return nil, fmt.Errorf("failed to deserialize enrollment payload: %w", err)
		}
	}
	return &RuntimeEnrollment{
		ID:             record.ID,
		TenantID:       record.TenantID,
		RuntimeNodeID:  record.RuntimeNodeID,
		NodeID:         record.NodeID,
		BootstrapKeyID: record.BootstrapKeyID,
		Status:         record.Status,
		RequestPayload: payload,
		ApprovedBy:     record.ApprovedBy,
		ApprovedAt:     timeFromTimestamptz(record.ApprovedAt),
		RejectedBy:     record.RejectedBy,
		RejectedAt:     timeFromTimestamptz(record.RejectedAt),
		RejectReason:   stringFromText(record.RejectReason),
		RevokedBy:      record.RevokedBy,
		RevokedAt:      timeFromTimestamptz(record.RevokedAt),
		RevokeReason:   stringFromText(record.RevokeReason),
		LastHelloAt:    timeFromTimestamptz(record.LastHelloAt),
		CreatedAt:      timeFromTimestamptz(record.CreatedAt),
		UpdatedAt:      timeFromTimestamptz(record.UpdatedAt),
	}, nil
}

func (s *Service) recordToRuntimeSession(record RuntimeSessionRecord) *RuntimeSession {
	return &RuntimeSession{
		ID:            record.ID,
		TenantID:      record.TenantID,
		RuntimeNodeID: record.RuntimeNodeID,
		NodeID:        record.NodeID,
		EnrollmentID:  record.EnrollmentID,
		ExpiresAt:     timeFromTimestamptz(record.ExpiresAt),
		LastSeenAt:    timeFromTimestamptz(record.LastSeenAt),
		RevokedAt:     timeFromTimestamptz(record.RevokedAt),
		RevokedReason: stringFromText(record.RevokedReason),
		CreatedAt:     timeFromTimestamptz(record.CreatedAt),
		UpdatedAt:     timeFromTimestamptz(record.UpdatedAt),
	}
}

func tenantOrDefault(tenantID uuid.UUID) uuid.UUID {
	if tenantID == uuid.Nil {
		return DefaultTenantID
	}
	return tenantID
}

func defaultMap(in map[string]interface{}) map[string]interface{} {
	if in == nil {
		return map[string]interface{}{}
	}
	return in
}

type runtimeNodeApprovalPayload struct {
	Name               string                 `json:"name"`
	SupportedProviders []string               `json:"supported_providers"`
	MaxSlots           int32                  `json:"max_slots"`
	Metadata           map[string]interface{} `json:"metadata"`
}

type parsedRuntimeNodeRequest struct {
	Name               string
	SupportedProviders []byte
	MaxSlots           int32
	Metadata           []byte
}

func parseRuntimeNodeRequest(enrollment RuntimeEnrollmentRecord) (parsedRuntimeNodeRequest, error) {
	var payload runtimeNodeApprovalPayload
	if len(enrollment.RequestPayload) > 0 {
		if err := json.Unmarshal(enrollment.RequestPayload, &payload); err != nil {
			return parsedRuntimeNodeRequest{}, fmt.Errorf("failed to deserialize enrollment payload: %w", err)
		}
	}
	if payload.Name == "" {
		payload.Name = enrollment.NodeID
	}
	if len(payload.SupportedProviders) == 0 {
		payload.SupportedProviders = []string{"runtime"}
	}
	if payload.MaxSlots <= 0 {
		payload.MaxSlots = 1
	}
	if payload.Metadata == nil {
		payload.Metadata = map[string]interface{}{}
	}
	providersJSON, err := json.Marshal(payload.SupportedProviders)
	if err != nil {
		return parsedRuntimeNodeRequest{}, fmt.Errorf("failed to serialize supported_providers: %w", err)
	}
	metadataJSON, err := json.Marshal(payload.Metadata)
	if err != nil {
		return parsedRuntimeNodeRequest{}, fmt.Errorf("failed to serialize metadata: %w", err)
	}
	return parsedRuntimeNodeRequest{
		Name:               payload.Name,
		SupportedProviders: providersJSON,
		MaxSlots:           payload.MaxSlots,
		Metadata:           metadataJSON,
	}, nil
}

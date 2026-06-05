package runtime

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgRepository struct {
	q *queries.Queries
}

func NewPgRepository(q *queries.Queries) Repository {
	return &PgRepository{q: q}
}

func (r *PgRepository) CreateNode(ctx context.Context, params CreateNodeParams) (NodeRecord, error) {
	node, err := r.q.CreateRuntimeNode(ctx, queries.CreateRuntimeNodeParams{
		NodeID:             params.NodeID,
		Name:               params.Name,
		SupportedProviders: params.SupportedProviders,
		MaxSlots:           params.MaxSlots,
		CurrentLoad:        params.CurrentLoad,
		Status:             params.Status,
		Metadata:           params.Metadata,
		LastHeartbeatAt:    params.LastHeartbeatAt,
	})
	if err != nil {
		return NodeRecord{}, err
	}
	return NodeRecord{
		ID:                 node.ID,
		TenantID:           node.TenantID,
		NodeID:             node.NodeID,
		Name:               node.Name,
		SupportedProviders: node.SupportedProviders,
		MaxSlots:           node.MaxSlots,
		CurrentLoad:        node.CurrentLoad,
		Status:             node.Status,
		Metadata:           node.Metadata,
		LastHeartbeatAt:    node.LastHeartbeatAt,
		CreatedAt:          node.CreatedAt,
		UpdatedAt:          node.UpdatedAt,
	}, nil
}

func (r *PgRepository) GetNode(ctx context.Context, nodeID string) (NodeRecord, error) {
	node, err := r.q.GetRuntimeNode(ctx, nodeID)
	if err != nil {
		return NodeRecord{}, err
	}
	return NodeRecord{
		ID:                 node.ID,
		TenantID:           node.TenantID,
		NodeID:             node.NodeID,
		Name:               node.Name,
		SupportedProviders: node.SupportedProviders,
		MaxSlots:           node.MaxSlots,
		CurrentLoad:        node.CurrentLoad,
		Status:             node.Status,
		Metadata:           node.Metadata,
		LastHeartbeatAt:    node.LastHeartbeatAt,
		CreatedAt:          node.CreatedAt,
		UpdatedAt:          node.UpdatedAt,
	}, nil
}

func (r *PgRepository) ListNodes(ctx context.Context, params ListNodesParams) ([]NodeRecord, error) {
	nodes, err := r.q.ListRuntimeNodes(ctx, queries.ListRuntimeNodesParams{
		Status: params.Status,
		Offset: params.Offset,
		Limit:  params.Limit,
	})
	if err != nil {
		return nil, err
	}
	records := make([]NodeRecord, len(nodes))
	for i, node := range nodes {
		records[i] = NodeRecord{
			ID:                 node.ID,
			TenantID:           node.TenantID,
			NodeID:             node.NodeID,
			Name:               node.Name,
			SupportedProviders: node.SupportedProviders,
			MaxSlots:           node.MaxSlots,
			CurrentLoad:        node.CurrentLoad,
			Status:             node.Status,
			Metadata:           node.Metadata,
			LastHeartbeatAt:    node.LastHeartbeatAt,
			CreatedAt:          node.CreatedAt,
			UpdatedAt:          node.UpdatedAt,
		}
	}
	return records, nil
}

func (r *PgRepository) ListRuntimeNodesForTenant(ctx context.Context, params ListRuntimeNodesForTenantParams) ([]NodeRecord, error) {
	nodes, err := r.q.ListRuntimeNodesForTenant(ctx, queries.ListRuntimeNodesForTenantParams{
		TenantID: params.TenantID,
		Status:   params.Status,
		Offset:   params.Offset,
		Limit:    params.Limit,
	})
	if err != nil {
		return nil, err
	}
	records := make([]NodeRecord, 0, len(nodes))
	for _, node := range nodes {
		records = append(records, NodeRecord{
			ID:                 node.ID,
			TenantID:           node.TenantID,
			NodeID:             node.NodeID,
			Name:               node.Name,
			SupportedProviders: node.SupportedProviders,
			MaxSlots:           node.MaxSlots,
			CurrentLoad:        node.CurrentLoad,
			Status:             node.Status,
			Metadata:           node.Metadata,
			LastHeartbeatAt:    node.LastHeartbeatAt,
			CreatedAt:          node.CreatedAt,
			UpdatedAt:          node.UpdatedAt,
		})
	}
	return records, nil
}

func (r *PgRepository) ListOnlineNodes(ctx context.Context, threshold pgtype.Timestamptz) ([]NodeRecord, error) {
	nodes, err := r.q.ListOnlineRuntimeNodes(ctx, threshold)
	if err != nil {
		return nil, err
	}
	records := make([]NodeRecord, len(nodes))
	for i, node := range nodes {
		records[i] = NodeRecord{
			ID:                 node.ID,
			TenantID:           node.TenantID,
			NodeID:             node.NodeID,
			Name:               node.Name,
			SupportedProviders: node.SupportedProviders,
			MaxSlots:           node.MaxSlots,
			CurrentLoad:        node.CurrentLoad,
			Status:             node.Status,
			Metadata:           node.Metadata,
			LastHeartbeatAt:    node.LastHeartbeatAt,
			CreatedAt:          node.CreatedAt,
			UpdatedAt:          node.UpdatedAt,
		}
	}
	return records, nil
}

func (r *PgRepository) UpdateHeartbeat(ctx context.Context, params UpdateHeartbeatParams) (NodeRecord, error) {
	node, err := r.q.UpdateRuntimeNodeHeartbeat(ctx, queries.UpdateRuntimeNodeHeartbeatParams{
		NodeID:          params.NodeID,
		LastHeartbeatAt: params.LastHeartbeatAt,
	})
	if err != nil {
		return NodeRecord{}, err
	}
	return NodeRecord{
		ID:                 node.ID,
		TenantID:           node.TenantID,
		NodeID:             node.NodeID,
		Name:               node.Name,
		SupportedProviders: node.SupportedProviders,
		MaxSlots:           node.MaxSlots,
		CurrentLoad:        node.CurrentLoad,
		Status:             node.Status,
		Metadata:           node.Metadata,
		LastHeartbeatAt:    node.LastHeartbeatAt,
		CreatedAt:          node.CreatedAt,
		UpdatedAt:          node.UpdatedAt,
	}, nil
}

func (r *PgRepository) UpdateLoad(ctx context.Context, params UpdateLoadParams) (NodeRecord, error) {
	node, err := r.q.UpdateRuntimeNodeLoad(ctx, queries.UpdateRuntimeNodeLoadParams{
		NodeID:      params.NodeID,
		CurrentLoad: params.CurrentLoad,
	})
	if err != nil {
		return NodeRecord{}, err
	}
	return NodeRecord{
		ID:                 node.ID,
		TenantID:           node.TenantID,
		NodeID:             node.NodeID,
		Name:               node.Name,
		SupportedProviders: node.SupportedProviders,
		MaxSlots:           node.MaxSlots,
		CurrentLoad:        node.CurrentLoad,
		Status:             node.Status,
		Metadata:           node.Metadata,
		LastHeartbeatAt:    node.LastHeartbeatAt,
		CreatedAt:          node.CreatedAt,
		UpdatedAt:          node.UpdatedAt,
	}, nil
}

func (r *PgRepository) UpdateStatus(ctx context.Context, params UpdateStatusParams) (NodeRecord, error) {
	node, err := r.q.UpdateRuntimeNodeStatus(ctx, queries.UpdateRuntimeNodeStatusParams{
		NodeID: params.NodeID,
		Status: params.Status,
	})
	if err != nil {
		return NodeRecord{}, err
	}
	return NodeRecord{
		ID:                 node.ID,
		TenantID:           node.TenantID,
		NodeID:             node.NodeID,
		Name:               node.Name,
		SupportedProviders: node.SupportedProviders,
		MaxSlots:           node.MaxSlots,
		CurrentLoad:        node.CurrentLoad,
		Status:             node.Status,
		Metadata:           node.Metadata,
		LastHeartbeatAt:    node.LastHeartbeatAt,
		CreatedAt:          node.CreatedAt,
		UpdatedAt:          node.UpdatedAt,
	}, nil
}

func (r *PgRepository) DeleteNode(ctx context.Context, nodeID string) error {
	return r.q.DeleteRuntimeNode(ctx, nodeID)
}

func (r *PgRepository) ListActiveRuntimeBootstrapKeys(ctx context.Context, tenantID uuid.UUID) ([]RuntimeBootstrapKeyRecord, error) {
	keys, err := r.q.ListActiveRuntimeBootstrapKeys(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	records := make([]RuntimeBootstrapKeyRecord, 0, len(keys))
	for _, key := range keys {
		records = append(records, RuntimeBootstrapKeyRecord{
			ID:        key.ID,
			TenantID:  key.TenantID,
			Name:      key.Name,
			KeyHash:   key.KeyHash,
			Status:    key.Status,
			ExpiresAt: key.ExpiresAt,
			CreatedAt: key.CreatedAt,
			UpdatedAt: key.UpdatedAt,
		})
	}
	return records, nil
}

func (r *PgRepository) UpsertRuntimeEnrollmentFromHello(ctx context.Context, params UpsertRuntimeEnrollmentFromHelloParams) (RuntimeEnrollmentRecord, error) {
	enrollment, err := r.q.UpsertRuntimeEnrollment(ctx, queries.UpsertRuntimeEnrollmentParams{
		NodeID:         params.NodeID,
		RequestPayload: params.RequestPayload,
		LastHelloAt:    params.LastHelloAt,
		BootstrapKeyID: params.BootstrapKeyID,
		TenantID:       params.TenantID,
	})
	if err != nil {
		return RuntimeEnrollmentRecord{}, err
	}
	return runtimeEnrollmentRecordFromQuery(enrollment), nil
}

func (r *PgRepository) GetRuntimeEnrollment(ctx context.Context, tenantID, enrollmentID uuid.UUID) (RuntimeEnrollmentRecord, error) {
	enrollment, err := r.q.GetRuntimeEnrollment(ctx, queries.GetRuntimeEnrollmentParams{
		TenantID: tenantID,
		ID:       enrollmentID,
	})
	if err != nil {
		return RuntimeEnrollmentRecord{}, err
	}
	return runtimeEnrollmentRecordFromQuery(enrollment), nil
}

func (r *PgRepository) ListRuntimeEnrollments(ctx context.Context, params ListRuntimeEnrollmentsParams) ([]RuntimeEnrollmentRecord, error) {
	enrollments, err := r.q.ListRuntimeEnrollments(ctx, queries.ListRuntimeEnrollmentsParams{
		TenantID: params.TenantID,
		Status:   params.Status,
		Offset:   params.Offset,
		Limit:    params.Limit,
	})
	if err != nil {
		return nil, err
	}
	records := make([]RuntimeEnrollmentRecord, 0, len(enrollments))
	for _, enrollment := range enrollments {
		records = append(records, runtimeEnrollmentRecordFromQuery(enrollment))
	}
	return records, nil
}

func (r *PgRepository) CountRuntimeEnrollmentsForTenant(ctx context.Context, tenantID uuid.UUID, status *RuntimeEnrollmentStatus) (int64, error) {
	return r.q.CountRuntimeEnrollmentsForTenant(ctx, queries.CountRuntimeEnrollmentsForTenantParams{
		TenantID: tenantID,
		Status:   enrollmentStatusToText(status),
	})
}

func (r *PgRepository) UpsertRuntimeNodeForTenant(ctx context.Context, params UpsertRuntimeNodeForTenantParams) (NodeRecord, error) {
	node, err := r.q.UpsertRuntimeNodeForTenant(ctx, queries.UpsertRuntimeNodeForTenantParams{
		Name:               params.Name,
		SupportedProviders: params.SupportedProviders,
		MaxSlots:           params.MaxSlots,
		CurrentLoad:        params.CurrentLoad,
		Status:             params.Status,
		Metadata:           params.Metadata,
		LastHeartbeatAt:    params.LastHeartbeatAt,
		TenantID:           params.TenantID,
		NodeID:             params.NodeID,
	})
	if err != nil {
		return NodeRecord{}, err
	}
	return NodeRecord{
		ID:                 node.ID,
		TenantID:           node.TenantID,
		NodeID:             node.NodeID,
		Name:               node.Name,
		SupportedProviders: node.SupportedProviders,
		MaxSlots:           node.MaxSlots,
		CurrentLoad:        node.CurrentLoad,
		Status:             node.Status,
		Metadata:           node.Metadata,
		LastHeartbeatAt:    node.LastHeartbeatAt,
		CreatedAt:          node.CreatedAt,
		UpdatedAt:          node.UpdatedAt,
	}, nil
}

func (r *PgRepository) ApproveRuntimeEnrollment(ctx context.Context, params ApproveRuntimeEnrollmentParams) (RuntimeEnrollmentRecord, error) {
	enrollment, err := r.q.ApproveRuntimeEnrollment(ctx, queries.ApproveRuntimeEnrollmentParams{
		RuntimeNodeID: params.RuntimeNodeID,
		ApprovedBy:    uuid.NullUUID{UUID: params.ApprovedBy, Valid: params.ApprovedBy != uuid.Nil},
		ID:            params.EnrollmentID,
		TenantID:      params.TenantID,
	})
	if err != nil {
		return RuntimeEnrollmentRecord{}, err
	}
	return runtimeEnrollmentRecordFromQuery(enrollment), nil
}

func (r *PgRepository) ApproveRuntimeEnrollmentWithNode(ctx context.Context, params ApproveRuntimeEnrollmentWithNodeParams) (RuntimeEnrollmentRecord, error) {
	enrollment, err := r.q.ApproveRuntimeEnrollmentWithNode(ctx, queries.ApproveRuntimeEnrollmentWithNodeParams{
		ApprovedBy:         uuid.NullUUID{UUID: params.ApprovedBy, Valid: params.ApprovedBy != uuid.Nil},
		ID:                 params.EnrollmentID,
		TenantID:           params.TenantID,
		Name:               params.Name,
		SupportedProviders: params.SupportedProviders,
		MaxSlots:           params.MaxSlots,
		CurrentLoad:        params.CurrentLoad,
		NodeStatus:         params.NodeStatus,
		Metadata:           params.Metadata,
		LastHeartbeatAt:    params.LastHeartbeatAt,
	})
	if err != nil {
		return RuntimeEnrollmentRecord{}, err
	}
	return runtimeEnrollmentRecordFromQuery(enrollment), nil
}

func (r *PgRepository) RejectRuntimeEnrollment(ctx context.Context, params RejectRuntimeEnrollmentParams) (RuntimeEnrollmentRecord, error) {
	enrollment, err := r.q.RejectRuntimeEnrollment(ctx, queries.RejectRuntimeEnrollmentParams{
		RejectedBy:   uuid.NullUUID{UUID: params.RejectedBy, Valid: params.RejectedBy != uuid.Nil},
		RejectReason: textFromString(&params.Reason),
		ID:           params.EnrollmentID,
		TenantID:     params.TenantID,
	})
	if err != nil {
		return RuntimeEnrollmentRecord{}, err
	}
	return runtimeEnrollmentRecordFromQuery(enrollment), nil
}

func (r *PgRepository) RevokeRuntimeEnrollment(ctx context.Context, params RevokeRuntimeEnrollmentParams) (RuntimeEnrollmentRecord, error) {
	enrollment, err := r.q.RevokeRuntimeEnrollment(ctx, queries.RevokeRuntimeEnrollmentParams{
		RevokedBy:    uuid.NullUUID{UUID: params.RevokedBy, Valid: params.RevokedBy != uuid.Nil},
		RevokeReason: textFromString(&params.Reason),
		ID:           params.EnrollmentID,
		TenantID:     params.TenantID,
	})
	if err != nil {
		return RuntimeEnrollmentRecord{}, err
	}
	return RuntimeEnrollmentRecord{
		ID:             enrollment.ID,
		TenantID:       enrollment.TenantID,
		RuntimeNodeID:  uuidFromNull(enrollment.RuntimeNodeID),
		NodeID:         enrollment.NodeID,
		BootstrapKeyID: enrollment.BootstrapKeyID,
		Status:         RuntimeEnrollmentStatus(enrollment.Status),
		RequestPayload: enrollment.RequestPayload,
		ApprovedBy:     enrollment.ApprovedBy,
		ApprovedAt:     enrollment.ApprovedAt,
		RejectedBy:     enrollment.RejectedBy,
		RejectedAt:     enrollment.RejectedAt,
		RejectReason:   enrollment.RejectReason,
		RevokedBy:      enrollment.RevokedBy,
		RevokedAt:      enrollment.RevokedAt,
		RevokeReason:   enrollment.RevokeReason,
		LastHelloAt:    enrollment.LastHelloAt,
		CreatedAt:      enrollment.CreatedAt,
		UpdatedAt:      enrollment.UpdatedAt,
	}, nil
}

func (r *PgRepository) CreateRuntimeSession(ctx context.Context, params CreateRuntimeSessionParams) (RuntimeSessionRecord, error) {
	session, err := r.q.CreateRuntimeSession(ctx, queries.CreateRuntimeSessionParams{
		TokenLookupHash: params.TokenLookupHash,
		TokenSecretHash: params.TokenSecretHash,
		ExpiresAt:       params.ExpiresAt,
		EnrollmentID:    uuid.NullUUID{UUID: params.EnrollmentID, Valid: params.EnrollmentID != uuid.Nil},
		TenantID:        params.TenantID,
		RuntimeNodeID:   params.RuntimeNodeID,
	})
	if err != nil {
		return RuntimeSessionRecord{}, err
	}
	return runtimeSessionRecordFromQuery(session, ""), nil
}

func (r *PgRepository) GetActiveRuntimeSessionByLookupHash(ctx context.Context, params GetActiveRuntimeSessionByLookupHashParams) (RuntimeSessionRecord, error) {
	session, err := r.q.GetActiveRuntimeSessionByLookupHash(ctx, params.TokenLookupHash)
	if err != nil {
		return RuntimeSessionRecord{}, err
	}
	return RuntimeSessionRecord{
		ID:              session.ID,
		TenantID:        session.TenantID,
		RuntimeNodeID:   session.RuntimeNodeID,
		NodeID:          session.NodeID,
		EnrollmentID:    session.EnrollmentID,
		TokenLookupHash: session.TokenLookupHash,
		TokenSecretHash: session.TokenSecretHash,
		ExpiresAt:       session.ExpiresAt,
		LastSeenAt:      session.LastSeenAt,
		RevokedAt:       session.RevokedAt,
		RevokedReason:   session.RevokedReason,
		CreatedAt:       session.CreatedAt,
		UpdatedAt:       session.UpdatedAt,
	}, nil
}

func (r *PgRepository) RenewRuntimeSession(ctx context.Context, params RenewRuntimeSessionParams) (RuntimeSessionRecord, error) {
	session, err := r.q.RenewRuntimeSession(ctx, queries.RenewRuntimeSessionParams{
		ExpiresAt: params.ExpiresAt,
		ID:        params.SessionID,
		TenantID:  params.TenantID,
	})
	if err != nil {
		return RuntimeSessionRecord{}, err
	}
	return runtimeSessionRecordFromQuery(session, ""), nil
}

func (r *PgRepository) TouchRuntimeSession(ctx context.Context, params TouchRuntimeSessionParams) (RuntimeSessionRecord, error) {
	session, err := r.q.TouchRuntimeSessionLastSeen(ctx, queries.TouchRuntimeSessionLastSeenParams{
		ID:       params.SessionID,
		TenantID: params.TenantID,
	})
	if err != nil {
		return RuntimeSessionRecord{}, err
	}
	return runtimeSessionRecordFromQuery(session, ""), nil
}

func (r *PgRepository) UpsertRuntimeCapability(ctx context.Context, params UpsertRuntimeCapabilityParams) (RuntimeCapability, error) {
	capability, err := r.q.UpsertRuntimeCapability(ctx, queries.UpsertRuntimeCapabilityParams{
		CapabilityType:   params.CapabilityType,
		CapabilityKey:    params.CapabilityKey,
		ProviderType:     params.ProviderType,
		ProviderVersion:  textFromString(params.ProviderVersion),
		BinaryPath:       textFromString(params.BinaryPath),
		Available:        params.Available,
		WorkspaceBaseDir: textFromString(params.WorkspaceBaseDir),
		Capacity:         params.Capacity,
		Labels:           params.Labels,
		Status:           params.Status,
		Details:          params.Details,
		HealthStatus:     params.HealthStatus,
		Metadata:         params.Metadata,
		LastSeenAt:       params.LastSeenAt,
		RuntimeNodeID:    params.RuntimeNodeID,
		TenantID:         params.TenantID,
	})
	if err != nil {
		return RuntimeCapability{}, err
	}
	return runtimeCapabilityFromQuery(capability), nil
}

func (r *PgRepository) CreateRuntimeEvent(ctx context.Context, params CreateRuntimeEventParams) (RuntimeEvent, error) {
	payload, err := json.Marshal(defaultMap(params.Payload))
	if err != nil {
		return RuntimeEvent{}, err
	}
	event, err := r.q.CreateRuntimeEvent(ctx, queries.CreateRuntimeEventParams{
		TenantID:        params.TenantID,
		RuntimeNodeID:   uuid.NullUUID{UUID: params.RuntimeNodeID, Valid: params.RuntimeNodeID != uuid.Nil},
		NodeID:          textFromValue(params.NodeID),
		EventType:       string(params.EventType),
		Severity:        string(params.Severity),
		Source:          string(params.Source),
		Title:           params.Title,
		Description:     textFromValue(params.Description),
		ProviderType:    textFromValue(params.ProviderType),
		CorrelationType: textFromValue(params.CorrelationType),
		CorrelationID:   textFromValue(params.CorrelationID),
		Payload:         payload,
	})
	if err != nil {
		return RuntimeEvent{}, err
	}
	return runtimeEventFromQuery(event), nil
}

func (r *PgRepository) ListRuntimeEvents(ctx context.Context, params ListRuntimeEventsParams) ([]RuntimeEvent, error) {
	events, err := r.q.ListRuntimeEvents(ctx, queries.ListRuntimeEventsParams{
		TenantID:     params.TenantID,
		EventType:    runtimeEventTypeToText(params.EventType),
		Severity:     runtimeEventSeverityToText(params.Severity),
		NodeID:       textFromString(params.NodeID),
		ProviderType: textFromString(params.ProviderType),
		Offset:       params.Offset,
		Limit:        params.Limit,
	})
	if err != nil {
		return nil, err
	}
	records := make([]RuntimeEvent, 0, len(events))
	for _, event := range events {
		records = append(records, runtimeEventFromQuery(event))
	}
	return records, nil
}

func (r *PgRepository) CountBlockedRuntimeEventsSince(ctx context.Context, tenantID uuid.UUID, since time.Time) (int64, error) {
	return r.q.CountBlockedRuntimeEventsSince(ctx, queries.CountBlockedRuntimeEventsSinceParams{
		TenantID:     tenantID,
		CreatedSince: timestamptzFromTime(since),
	})
}

func (r *PgRepository) CountRuntimeNodesForTenant(ctx context.Context, tenantID uuid.UUID) (int64, error) {
	return r.q.CountRuntimeNodesForTenant(ctx, tenantID)
}

func (r *PgRepository) CountOnlineRuntimeNodesForTenant(ctx context.Context, tenantID uuid.UUID, threshold time.Time) (int64, error) {
	return r.q.CountOnlineRuntimeNodesForTenant(ctx, queries.CountOnlineRuntimeNodesForTenantParams{
		TenantID:        tenantID,
		LastHeartbeatAt: timestamptzFromTime(threshold),
	})
}

func (r *PgRepository) CountActiveProviderSessionsForTenant(ctx context.Context, tenantID uuid.UUID) (int64, error) {
	return r.q.CountActiveProviderSessionsForTenant(ctx, tenantID)
}

func (r *PgRepository) ListRuntimeProviderCapabilitiesForTenant(ctx context.Context, tenantID uuid.UUID) ([]RuntimeProviderCapabilitySummary, error) {
	capabilities, err := r.q.ListRuntimeProviderCapabilitiesForTenant(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	summaries := make([]RuntimeProviderCapabilitySummary, 0, len(capabilities))
	for _, capability := range capabilities {
		summaries = append(summaries, RuntimeProviderCapabilitySummary{
			ProviderType:   capability.ProviderType,
			NodeCount:      capability.NodeCount,
			AvailableCount: capability.AvailableCount,
			HealthyCount:   capability.HealthyCount,
			LastSeenAt:     timeFromTimestamptz(capability.LastSeenAt),
		})
	}
	return summaries, nil
}

func (r *PgRepository) ListRuntimeCapabilitiesForNode(ctx context.Context, tenantID uuid.UUID, nodeID string) ([]RuntimeCapability, error) {
	capabilities, err := r.q.ListRuntimeCapabilitiesForNode(ctx, queries.ListRuntimeCapabilitiesForNodeParams{
		TenantID: tenantID,
		NodeID:   nodeID,
	})
	if err != nil {
		return nil, err
	}
	records := make([]RuntimeCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		records = append(records, runtimeCapabilityFromQuery(capability))
	}
	return records, nil
}

func runtimeEnrollmentRecordFromQuery(enrollment queries.RuntimeEnrollment) RuntimeEnrollmentRecord {
	return RuntimeEnrollmentRecord{
		ID:             enrollment.ID,
		TenantID:       enrollment.TenantID,
		RuntimeNodeID:  uuidFromNull(enrollment.RuntimeNodeID),
		NodeID:         enrollment.NodeID,
		BootstrapKeyID: enrollment.BootstrapKeyID,
		Status:         RuntimeEnrollmentStatus(enrollment.Status),
		RequestPayload: enrollment.RequestPayload,
		ApprovedBy:     enrollment.ApprovedBy,
		ApprovedAt:     enrollment.ApprovedAt,
		RejectedBy:     enrollment.RejectedBy,
		RejectedAt:     enrollment.RejectedAt,
		RejectReason:   enrollment.RejectReason,
		RevokedBy:      enrollment.RevokedBy,
		RevokedAt:      enrollment.RevokedAt,
		RevokeReason:   enrollment.RevokeReason,
		LastHelloAt:    enrollment.LastHelloAt,
		CreatedAt:      enrollment.CreatedAt,
		UpdatedAt:      enrollment.UpdatedAt,
	}
}

func uuidFromNull(id uuid.NullUUID) uuid.UUID {
	if !id.Valid {
		return uuid.Nil
	}
	return id.UUID
}

func runtimeSessionRecordFromQuery(session queries.RuntimeSession, nodeID string) RuntimeSessionRecord {
	return RuntimeSessionRecord{
		ID:              session.ID,
		TenantID:        session.TenantID,
		RuntimeNodeID:   session.RuntimeNodeID,
		NodeID:          nodeID,
		EnrollmentID:    session.EnrollmentID,
		TokenLookupHash: session.TokenLookupHash,
		TokenSecretHash: session.TokenSecretHash,
		ExpiresAt:       session.ExpiresAt,
		LastSeenAt:      session.LastSeenAt,
		RevokedAt:       session.RevokedAt,
		RevokedReason:   session.RevokedReason,
		CreatedAt:       session.CreatedAt,
		UpdatedAt:       session.UpdatedAt,
	}
}

func runtimeCapabilityFromQuery(capability queries.RuntimeCapability) RuntimeCapability {
	return RuntimeCapability{
		ID:               capability.ID,
		TenantID:         capability.TenantID,
		RuntimeNodeID:    capability.RuntimeNodeID,
		CapabilityType:   capability.CapabilityType,
		CapabilityKey:    capability.CapabilityKey,
		ProviderType:     capability.ProviderType,
		ProviderVersion:  stringFromText(capability.ProviderVersion),
		BinaryPath:       stringFromText(capability.BinaryPath),
		Available:        capability.Available,
		WorkspaceBaseDir: stringFromText(capability.WorkspaceBaseDir),
		Capacity:         jsonMapFromBytes(capability.Capacity),
		Labels:           jsonMapFromBytes(capability.Labels),
		Status:           capability.Status,
		Details:          jsonMapFromBytes(capability.Details),
		HealthStatus:     capability.HealthStatus,
		Metadata:         jsonMapFromBytes(capability.Metadata),
		LastSeenAt:       timeFromTimestamptz(capability.LastSeenAt),
		CreatedAt:        timeFromTimestamptz(capability.CreatedAt),
		UpdatedAt:        timeFromTimestamptz(capability.UpdatedAt),
	}
}

func runtimeEventFromQuery(event queries.RuntimeEvent) RuntimeEvent {
	return RuntimeEvent{
		ID:              event.ID,
		TenantID:        event.TenantID,
		RuntimeNodeID:   uuidFromNull(event.RuntimeNodeID),
		NodeID:          textValue(event.NodeID),
		EventType:       RuntimeEventType(event.EventType),
		Severity:        RuntimeEventSeverity(event.Severity),
		Source:          RuntimeEventSource(event.Source),
		Title:           event.Title,
		Description:     textValue(event.Description),
		ProviderType:    textValue(event.ProviderType),
		CorrelationType: textValue(event.CorrelationType),
		CorrelationID:   textValue(event.CorrelationID),
		Payload:         jsonMapFromBytes(event.Payload),
		CreatedAt:       timeFromTimestamptz(event.CreatedAt),
		UpdatedAt:       timeFromTimestamptz(event.UpdatedAt),
	}
}

func jsonMapFromBytes(payload []byte) map[string]interface{} {
	if len(payload) == 0 {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal(payload, &out); err != nil || out == nil {
		return map[string]interface{}{}
	}
	return out
}

func textFromValue(value string) pgtype.Text {
	if value == "" {
		return pgtype.Text{Valid: false}
	}
	return pgtype.Text{String: value, Valid: true}
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func runtimeEventTypeToText(eventType *RuntimeEventType) pgtype.Text {
	if eventType == nil {
		return pgtype.Text{Valid: false}
	}
	return textFromValue(string(*eventType))
}

func runtimeEventSeverityToText(severity *RuntimeEventSeverity) pgtype.Text {
	if severity == nil {
		return pgtype.Text{Valid: false}
	}
	return textFromValue(string(*severity))
}

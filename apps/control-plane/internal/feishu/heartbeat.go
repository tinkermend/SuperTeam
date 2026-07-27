package feishu

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/superteam/control-plane/internal/storage/queries"
)

// ConnectorHeartbeatTimeout 是 Console/看门狗判定通道失联的默认阈值。
// connector 默认 30s 上报一次,取 90s 允许偶发抖动。
const ConnectorHeartbeatTimeout = 90 * time.Second

const DefaultConnectorServiceName = "feishu-connector"

type ConnectorAppStatus struct {
	AppID         string     `json:"app_id"`
	ConfigID      string     `json:"config_id,omitempty"`
	WSStatus      string     `json:"ws_status"` // connected | reconnecting | stopped | unknown
	LastWSEventAt *time.Time `json:"last_ws_event_at,omitempty"`
}

type ConnectorHeartbeat struct {
	TenantID         uuid.UUID
	ServiceName      string
	Version          string
	LastHeartbeatAt  time.Time
	LastOutboxPollAt *time.Time
	Apps             []ConnectorAppStatus
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type UpsertConnectorHeartbeatInput struct {
	TenantID         uuid.UUID
	ServiceName      string
	Version          string
	LastOutboxPollAt *time.Time
	Apps             []ConnectorAppStatus
}

func (r *PgRepository) UpsertConnectorHeartbeat(ctx context.Context, input UpsertConnectorHeartbeatInput) (ConnectorHeartbeat, error) {
	serviceName := input.ServiceName
	if serviceName == "" {
		serviceName = DefaultConnectorServiceName
	}
	snapshot, err := json.Marshal(input.Apps)
	if err != nil {
		return ConnectorHeartbeat{}, err
	}
	var pollAt pgtype.Timestamptz
	if input.LastOutboxPollAt != nil {
		pollAt = pgtype.Timestamptz{Time: input.LastOutboxPollAt.UTC(), Valid: true}
	}
	row, err := r.q.UpsertFeishuConnectorHeartbeat(ctx, queries.UpsertFeishuConnectorHeartbeatParams{
		TenantID:         input.TenantID,
		ServiceName:      serviceName,
		Version:          input.Version,
		LastHeartbeatAt:  pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		LastOutboxPollAt: pollAt,
		AppsSnapshot:     snapshot,
	})
	if err != nil {
		return ConnectorHeartbeat{}, err
	}
	return connectorHeartbeatFromRow(row), nil
}

func (r *PgRepository) GetConnectorHeartbeat(ctx context.Context, tenantID uuid.UUID, serviceName string) (ConnectorHeartbeat, error) {
	if serviceName == "" {
		serviceName = DefaultConnectorServiceName
	}
	row, err := r.q.GetFeishuConnectorHeartbeat(ctx, queries.GetFeishuConnectorHeartbeatParams{
		TenantID:    tenantID,
		ServiceName: serviceName,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ConnectorHeartbeat{}, ErrAppConfigNotFound
		}
		return ConnectorHeartbeat{}, err
	}
	return connectorHeartbeatFromRow(row), nil
}

func (r *PgRepository) ListConnectorHeartbeats(ctx context.Context, tenantID uuid.UUID) ([]ConnectorHeartbeat, error) {
	rows, err := r.q.ListFeishuConnectorHeartbeats(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	out := make([]ConnectorHeartbeat, 0, len(rows))
	for _, row := range rows {
		out = append(out, connectorHeartbeatFromRow(row))
	}
	return out, nil
}

func (r *PgRepository) ListStaleConnectorHeartbeats(ctx context.Context, staleBefore time.Time) ([]ConnectorHeartbeat, error) {
	rows, err := r.q.ListStaleFeishuConnectorHeartbeats(ctx, pgtype.Timestamptz{Time: staleBefore.UTC(), Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]ConnectorHeartbeat, 0, len(rows))
	for _, row := range rows {
		out = append(out, connectorHeartbeatFromRow(row))
	}
	return out, nil
}

func (r *PgRepository) ListActiveTenantOwnersAndAdmins(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error) {
	return r.q.ListActiveTenantOwnersAndAdmins(ctx, tenantID)
}

func connectorHeartbeatFromRow(row queries.FeishuConnectorHeartbeat) ConnectorHeartbeat {
	hb := ConnectorHeartbeat{
		TenantID:        row.TenantID,
		ServiceName:     row.ServiceName,
		Version:         row.Version,
		LastHeartbeatAt: row.LastHeartbeatAt.Time,
		CreatedAt:       row.CreatedAt.Time,
		UpdatedAt:       row.UpdatedAt.Time,
		Apps:            []ConnectorAppStatus{},
	}
	if row.LastOutboxPollAt.Valid {
		t := row.LastOutboxPollAt.Time
		hb.LastOutboxPollAt = &t
	}
	if len(row.AppsSnapshot) > 0 {
		var apps []ConnectorAppStatus
		if err := json.Unmarshal(row.AppsSnapshot, &apps); err == nil {
			hb.Apps = apps
		}
	}
	return hb
}

// ChannelHealth 是 Console 健康摘要。
type ChannelHealth struct {
	ServiceName      string                `json:"service_name"`
	Version          string                `json:"version,omitempty"`
	Status           string                `json:"status"` // healthy | stale | missing
	LastHeartbeatAt  *time.Time            `json:"last_heartbeat_at,omitempty"`
	LastOutboxPollAt *time.Time            `json:"last_outbox_poll_at,omitempty"`
	AgeSeconds       *int64                `json:"age_seconds,omitempty"`
	Apps             []ConnectorAppStatus  `json:"apps"`
	TimeoutSeconds   int64                 `json:"timeout_seconds"`
}

func (s *Service) RecordConnectorHeartbeat(ctx context.Context, input UpsertConnectorHeartbeatInput) (ConnectorHeartbeat, error) {
	if input.TenantID == uuid.Nil {
		return ConnectorHeartbeat{}, ErrInvalidInput
	}
	return s.repoHeartbeat().UpsertConnectorHeartbeat(ctx, input)
}

func (s *Service) ListChannelAlertRecipients(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error) {
	if tenantID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	return s.repoHeartbeat().ListActiveTenantOwnersAndAdmins(ctx, tenantID)
}

func (s *Service) GetChannelHealth(ctx context.Context, tenantID uuid.UUID) (ChannelHealth, error) {
	if tenantID == uuid.Nil {
		return ChannelHealth{}, ErrInvalidInput
	}
	hb, err := s.repoHeartbeat().GetConnectorHeartbeat(ctx, tenantID, DefaultConnectorServiceName)
	timeout := ConnectorHeartbeatTimeout
	if err != nil {
		if errors.Is(err, ErrAppConfigNotFound) {
			return ChannelHealth{
				ServiceName:    DefaultConnectorServiceName,
				Status:         "missing",
				Apps:           []ConnectorAppStatus{},
				TimeoutSeconds: int64(timeout.Seconds()),
			}, nil
		}
		return ChannelHealth{}, err
	}
	age := time.Since(hb.LastHeartbeatAt)
	status := "healthy"
	if age > timeout {
		status = "stale"
	}
	ageSec := int64(age.Seconds())
	return ChannelHealth{
		ServiceName:      hb.ServiceName,
		Version:          hb.Version,
		Status:           status,
		LastHeartbeatAt:  &hb.LastHeartbeatAt,
		LastOutboxPollAt: hb.LastOutboxPollAt,
		AgeSeconds:       &ageSec,
		Apps:             hb.Apps,
		TimeoutSeconds:   int64(timeout.Seconds()),
	}, nil
}

func (s *Service) ListOperationalOutbox(ctx context.Context, tenantID uuid.UUID, statuses []string, limit, offset int32) ([]OutboxItem, int64, error) {
	if tenantID == uuid.Nil {
		return nil, 0, ErrInvalidInput
	}
	if len(statuses) == 0 {
		statuses = []string{"failed", "skipped_unbound"}
	}
	repo := s.repoOutbox()
	items, err := repo.ListOutboxByStatuses(ctx, tenantID, statuses, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	total, err := repo.CountOutboxByStatuses(ctx, tenantID, statuses)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (s *Service) RequeueFailedOutbox(ctx context.Context, tenantID, id uuid.UUID) (OutboxItem, error) {
	if tenantID == uuid.Nil || id == uuid.Nil {
		return OutboxItem{}, ErrInvalidInput
	}
	return s.repoOutbox().RequeueOutbox(ctx, tenantID, id)
}

// repoHeartbeat/repoOutbox 把 PgRepository 方法面暴露给 Service(生产 repo 是 *PgRepository)。
type heartbeatRepo interface {
	UpsertConnectorHeartbeat(ctx context.Context, input UpsertConnectorHeartbeatInput) (ConnectorHeartbeat, error)
	GetConnectorHeartbeat(ctx context.Context, tenantID uuid.UUID, serviceName string) (ConnectorHeartbeat, error)
	ListConnectorHeartbeats(ctx context.Context, tenantID uuid.UUID) ([]ConnectorHeartbeat, error)
	ListStaleConnectorHeartbeats(ctx context.Context, staleBefore time.Time) ([]ConnectorHeartbeat, error)
	ListActiveTenantOwnersAndAdmins(ctx context.Context, tenantID uuid.UUID) ([]uuid.UUID, error)
}

type outboxOpsRepo interface {
	ListOutboxByStatuses(ctx context.Context, tenantID uuid.UUID, statuses []string, limit, offset int32) ([]OutboxItem, error)
	CountOutboxByStatuses(ctx context.Context, tenantID uuid.UUID, statuses []string) (int64, error)
	RequeueOutbox(ctx context.Context, tenantID, id uuid.UUID) (OutboxItem, error)
}

func (s *Service) repoHeartbeat() heartbeatRepo {
	if r, ok := s.repo.(heartbeatRepo); ok {
		return r
	}
	// production always wires *PgRepository; tests that don't need heartbeat can ignore.
	return noopHeartbeatRepo{}
}

func (s *Service) repoOutbox() outboxOpsRepo {
	if r, ok := s.repo.(outboxOpsRepo); ok {
		return r
	}
	// Outbox ops live on *PgRepository which is also the OutboxRepository in production.
	if r, ok := any(s.repo).(outboxOpsRepo); ok {
		return r
	}
	return noopOutboxRepo{}
}

type noopHeartbeatRepo struct{}

func (noopHeartbeatRepo) UpsertConnectorHeartbeat(context.Context, UpsertConnectorHeartbeatInput) (ConnectorHeartbeat, error) {
	return ConnectorHeartbeat{}, ErrInvalidInput
}
func (noopHeartbeatRepo) GetConnectorHeartbeat(context.Context, uuid.UUID, string) (ConnectorHeartbeat, error) {
	return ConnectorHeartbeat{}, ErrAppConfigNotFound
}
func (noopHeartbeatRepo) ListConnectorHeartbeats(context.Context, uuid.UUID) ([]ConnectorHeartbeat, error) {
	return nil, nil
}
func (noopHeartbeatRepo) ListStaleConnectorHeartbeats(context.Context, time.Time) ([]ConnectorHeartbeat, error) {
	return nil, nil
}
func (noopHeartbeatRepo) ListActiveTenantOwnersAndAdmins(context.Context, uuid.UUID) ([]uuid.UUID, error) {
	return nil, nil
}

type noopOutboxRepo struct{}

func (noopOutboxRepo) ListOutboxByStatuses(context.Context, uuid.UUID, []string, int32, int32) ([]OutboxItem, error) {
	return nil, ErrInvalidInput
}
func (noopOutboxRepo) CountOutboxByStatuses(context.Context, uuid.UUID, []string) (int64, error) {
	return 0, ErrInvalidInput
}
func (noopOutboxRepo) RequeueOutbox(context.Context, uuid.UUID, uuid.UUID) (OutboxItem, error) {
	return OutboxItem{}, ErrOutboxNotFound
}

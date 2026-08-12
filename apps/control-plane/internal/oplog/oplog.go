// Package oplog writes web_operation_logs for console mutations.
//
// Audit events remain the durable business-fact chain. This package is the
// tenant-wide "who changed what" surface consumed by /logs/operation.
package oplog

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

const (
	ModuleAuth               = "auth"
	ModuleAuthz              = "authz"
	ModuleTeams              = "teams"
	ModuleEmployees          = "employees"
	ModuleProjects           = "projects"
	ModuleSkills             = "skills"
	ModuleSystemConfig       = "system_config"
	ModuleScenarioTemplates  = "scenario_templates"
	ResultSucceeded          = "succeeded"
	ResultFailed             = "failed"
)

// Querier is the sqlc surface this package needs.
type Querier interface {
	CreateWebOperationLog(ctx context.Context, arg queries.CreateWebOperationLogParams) (queries.WebOperationLog, error)
}

// Logger writes one operation-log row. Implementations must be nil-safe at the call site.
type Logger interface {
	Write(ctx context.Context, rec Record) error
}

// Record is one console operation log row.
type Record struct {
	TenantID     uuid.UUID
	UserID       uuid.UUID
	Username     string
	Module       string
	ResourceType string
	ResourceID   string
	Action       string
	Result       string
	RequestID    string
	ClientIP     string
	UserAgent    string
	Details      map[string]any
}

// PgLogger persists records via sqlc.
type PgLogger struct {
	Q Querier
}

func (l *PgLogger) Write(ctx context.Context, rec Record) error {
	if l == nil || l.Q == nil {
		return nil
	}
	return Write(ctx, l.Q, rec)
}

// Write inserts one row. Missing actor/IP fields are filled from request context.
func Write(ctx context.Context, q Querier, rec Record) error {
	if q == nil || strings.TrimSpace(rec.Module) == "" || strings.TrimSpace(rec.Action) == "" {
		return nil
	}
	rec = rec.withContext(ctx)
	if rec.Result == "" {
		rec.Result = ResultSucceeded
	}
	if rec.Details == nil {
		rec.Details = map[string]any{}
	}
	details, err := json.Marshal(rec.Details)
	if err != nil {
		return err
	}
	_, err = q.CreateWebOperationLog(ctx, queries.CreateWebOperationLogParams{
		TenantID:     uuid.NullUUID{UUID: rec.TenantID, Valid: rec.TenantID != uuid.Nil},
		UserID:       uuid.NullUUID{UUID: rec.UserID, Valid: rec.UserID != uuid.Nil},
		Username:     nullableText(rec.Username),
		Module:       rec.Module,
		ResourceType: nullableText(rec.ResourceType),
		ResourceID:   nullableText(rec.ResourceID),
		Action:       rec.Action,
		Result:       rec.Result,
		RequestID:    nullableText(rec.RequestID),
		ClientIp:     nullableText(rec.ClientIP),
		UserAgent:    nullableText(rec.UserAgent),
		Details:      details,
	})
	return err
}

// WriteBestEffort logs a warning instead of failing the caller.
func WriteBestEffort(ctx context.Context, log Logger, rec Record) {
	if log == nil {
		return
	}
	if err := log.Write(ctx, rec); err != nil {
		slog.Warn("operation log write failed", "error", err, "module", rec.Module, "action", rec.Action)
	}
}

// AuditMirror is the subset of an audit_events row that maps onto an operation log.
type AuditMirror struct {
	TenantID     uuid.UUID
	EventType    string
	ActorType    string
	ActorID      string
	ResourceType string
	ResourceID   string
	Action       string
	Details      []byte
}

// MirrorAudit dual-writes a console audit event into web_operation_logs.
// Unknown / run-lifecycle event types are ignored.
func MirrorAudit(ctx context.Context, q Querier, in AuditMirror) {
	module, ok := ModuleForAudit(in.EventType, in.Action)
	if !ok || q == nil {
		return
	}
	rec := Record{
		TenantID:     in.TenantID,
		Module:       module,
		ResourceType: in.ResourceType,
		ResourceID:   in.ResourceID,
		Action:       in.Action,
		Result:       ResultSucceeded,
		Details:      detailsMap(in.Details),
	}
	if in.ActorType == "user" {
		if id, err := uuid.Parse(strings.TrimSpace(in.ActorID)); err == nil {
			rec.UserID = id
		}
	}
	if err := Write(ctx, q, rec); err != nil {
		slog.Warn("operation log mirror failed", "error", err, "event_type", in.EventType, "action", in.Action)
	}
}

// MirrorCreateAudit dual-writes a sqlc CreateAuditEventParams row.
func MirrorCreateAudit(ctx context.Context, q Querier, params queries.CreateAuditEventParams) {
	MirrorAudit(ctx, q, AuditMirror{
		TenantID:     params.TenantID.UUID,
		EventType:    params.EventType,
		ActorType:    params.ActorType,
		ActorID:      params.ActorID,
		ResourceType: params.ResourceType.String,
		ResourceID:   params.ResourceID.String,
		Action:       params.Action,
		Details:      params.Details,
	})
}

// InsertAudit writes audit_events then mirrors console types into operation logs.
func InsertAudit(ctx context.Context, q AuditQuerier, params queries.CreateAuditEventParams) (queries.AuditEvent, error) {
	created, err := q.CreateAuditEvent(ctx, params)
	if err != nil {
		return queries.AuditEvent{}, err
	}
	MirrorCreateAudit(ctx, q, params)
	return created, nil
}

// AuditQuerier is the sqlc surface needed to persist an audit event and its operation-log mirror.
type AuditQuerier interface {
	CreateAuditEvent(ctx context.Context, arg queries.CreateAuditEventParams) (queries.AuditEvent, error)
	CreateWebOperationLog(ctx context.Context, arg queries.CreateWebOperationLogParams) (queries.WebOperationLog, error)
}

// ModuleForAudit maps audit event_type/action onto an operation-log module.
func ModuleForAudit(eventType, action string) (string, bool) {
	switch strings.TrimSpace(eventType) {
	case "team_management":
		if strings.HasPrefix(action, "team.skill.") {
			return ModuleSkills, true
		}
		return ModuleTeams, true
	case "digital_employee_management":
		return ModuleEmployees, true
	case "project_management":
		return ModuleProjects, true
	case "system_config":
		return ModuleSystemConfig, true
	case "scenario_template":
		return ModuleScenarioTemplates, true
	default:
		return "", false
	}
}

func (rec Record) withContext(ctx context.Context) Record {
	meta := MetaFromContext(ctx)
	if rec.TenantID == uuid.Nil {
		rec.TenantID = meta.TenantID
	}
	if rec.UserID == uuid.Nil {
		rec.UserID = meta.UserID
	}
	if rec.Username == "" {
		rec.Username = meta.Username
	}
	if rec.ClientIP == "" {
		rec.ClientIP = meta.ClientIP
	}
	if rec.UserAgent == "" {
		rec.UserAgent = meta.UserAgent
	}
	if rec.RequestID == "" {
		rec.RequestID = meta.RequestID
	}
	return rec
}

func detailsMap(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil || value == nil {
		return map[string]any{}
	}
	return value
}

func nullableText(value string) pgtype.Text {
	if strings.TrimSpace(value) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

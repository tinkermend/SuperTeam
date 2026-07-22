package automation

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgRepository struct {
	q *queries.Queries
}

func NewPgRepository(q *queries.Queries) *PgRepository {
	return &PgRepository{q: q}
}

func (r *PgRepository) CreateRule(ctx context.Context, rule Rule) (Rule, error) {
	row, err := r.q.CreateAutomationRule(ctx, queries.CreateAutomationRuleParams{
		TenantID:                rule.TenantID,
		TeamID:                  rule.TeamID,
		ProjectID:               rule.ProjectID,
		Name:                    rule.Name,
		Enabled:                 rule.Enabled,
		CoordinationMode:        rule.CoordinationMode,
		DemandTitleTemplate:     textPtr(rule.DemandTitleTemplate),
		DemandBodyTemplate:      textPtr(rule.DemandBodyTemplate),
		ScenarioTemplateKey:     textPtr(rule.ScenarioTemplateKey),
		DigitalEmployeeID:       uuidPtr(rule.DigitalEmployeeID),
		ChatObjectiveTemplate:   textPtr(rule.ChatObjectiveTemplate),
		ScheduleKind:            rule.ScheduleKind,
		CronExpr:                textPtr(rule.CronExpr),
		IntervalSeconds:         int4Ptr(rule.IntervalSeconds),
		Timezone:                rule.Timezone,
		OverlapPolicy:           rule.OverlapPolicy,
		ActorUserID:             rule.ActorUserID,
		DisabledReason:          textPtr(rule.DisabledReason),
		ConsecutiveFailureCount: rule.ConsecutiveFailureCount,
		TemporalScheduleID:      textPtr(rule.TemporalScheduleID),
	})
	if err != nil {
		return Rule{}, err
	}
	return ruleFromRow(row), nil
}

func (r *PgRepository) GetRule(ctx context.Context, tenantID, ruleID uuid.UUID) (Rule, error) {
	row, err := r.q.GetAutomationRule(ctx, queries.GetAutomationRuleParams{
		TenantID: tenantID,
		ID:       ruleID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Rule{}, ErrNotFound
		}
		return Rule{}, err
	}
	return ruleFromRow(row), nil
}

func (r *PgRepository) ListRules(ctx context.Context, req ListRulesRequest) ([]Rule, error) {
	rows, err := r.q.ListAutomationRules(ctx, queries.ListAutomationRulesParams{
		TenantID:    req.TenantID,
		ProjectID:   uuidPtr(req.ProjectID),
		Enabled:     boolPtr(req.Enabled),
		LimitCount:  req.Limit,
		OffsetCount: req.Offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Rule, 0, len(rows))
	for _, row := range rows {
		out = append(out, ruleFromRow(row))
	}
	return out, nil
}

func (r *PgRepository) UpdateRule(ctx context.Context, rule Rule) (Rule, error) {
	row, err := r.q.UpdateAutomationRule(ctx, queries.UpdateAutomationRuleParams{
		TenantID:              rule.TenantID,
		ID:                    rule.ID,
		Name:                  rule.Name,
		DemandTitleTemplate:   textPtr(rule.DemandTitleTemplate),
		DemandBodyTemplate:    textPtr(rule.DemandBodyTemplate),
		ScenarioTemplateKey:   textPtr(rule.ScenarioTemplateKey),
		DigitalEmployeeID:     uuidPtr(rule.DigitalEmployeeID),
		ChatObjectiveTemplate: textPtr(rule.ChatObjectiveTemplate),
		ScheduleKind:          rule.ScheduleKind,
		CronExpr:              textPtr(rule.CronExpr),
		IntervalSeconds:       int4Ptr(rule.IntervalSeconds),
		Timezone:              rule.Timezone,
		TemporalScheduleID:    textPtr(rule.TemporalScheduleID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Rule{}, ErrNotFound
		}
		return Rule{}, err
	}
	return ruleFromRow(row), nil
}

func (r *PgRepository) SetRuleEnabled(ctx context.Context, tenantID, ruleID uuid.UUID, enabled bool, disabledReason *string) (Rule, error) {
	row, err := r.q.SetAutomationRuleEnabled(ctx, queries.SetAutomationRuleEnabledParams{
		TenantID:       tenantID,
		ID:             ruleID,
		Enabled:        enabled,
		DisabledReason: textPtr(disabledReason),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Rule{}, ErrNotFound
		}
		return Rule{}, err
	}
	return ruleFromRow(row), nil
}

func (r *PgRepository) SetRuleScheduleID(ctx context.Context, tenantID, ruleID uuid.UUID, scheduleID *string) (Rule, error) {
	row, err := r.q.SetAutomationRuleScheduleID(ctx, queries.SetAutomationRuleScheduleIDParams{
		TenantID:           tenantID,
		ID:                 ruleID,
		TemporalScheduleID: textPtr(scheduleID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Rule{}, ErrNotFound
		}
		return Rule{}, err
	}
	return ruleFromRow(row), nil
}

func (r *PgRepository) IncrementFailureCount(ctx context.Context, tenantID, ruleID uuid.UUID) (Rule, error) {
	row, err := r.q.IncrementAutomationRuleFailureCount(ctx, queries.IncrementAutomationRuleFailureCountParams{
		TenantID: tenantID,
		ID:       ruleID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Rule{}, ErrNotFound
		}
		return Rule{}, err
	}
	return ruleFromRow(row), nil
}

func (r *PgRepository) ResetFailureCount(ctx context.Context, tenantID, ruleID uuid.UUID) (Rule, error) {
	row, err := r.q.ResetAutomationRuleFailureCount(ctx, queries.ResetAutomationRuleFailureCountParams{
		TenantID: tenantID,
		ID:       ruleID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Rule{}, ErrNotFound
		}
		return Rule{}, err
	}
	return ruleFromRow(row), nil
}

func (r *PgRepository) DisableRuleSystem(ctx context.Context, tenantID, ruleID uuid.UUID, reason string) (Rule, error) {
	row, err := r.q.DisableAutomationRuleSystem(ctx, queries.DisableAutomationRuleSystemParams{
		TenantID:       tenantID,
		ID:             ruleID,
		DisabledReason: reason,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Rule{}, ErrNotFound
		}
		return Rule{}, err
	}
	return ruleFromRow(row), nil
}

func (r *PgRepository) DeleteRule(ctx context.Context, tenantID, ruleID uuid.UUID) error {
	n, err := r.q.DeleteAutomationRule(ctx, queries.DeleteAutomationRuleParams{
		TenantID: tenantID,
		ID:       ruleID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *PgRepository) ListEnabledRulesByActor(ctx context.Context, tenantID, actorUserID uuid.UUID) ([]Rule, error) {
	rows, err := r.q.ListEnabledAutomationRulesByActor(ctx, queries.ListEnabledAutomationRulesByActorParams{
		TenantID:    tenantID,
		ActorUserID: actorUserID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Rule, 0, len(rows))
	for _, row := range rows {
		out = append(out, ruleFromRow(row))
	}
	return out, nil
}

func (r *PgRepository) ListEnabledRulesByActorOnProject(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID) ([]Rule, error) {
	rows, err := r.q.ListEnabledAutomationRulesByActorOnProject(ctx, queries.ListEnabledAutomationRulesByActorOnProjectParams{
		TenantID:    tenantID,
		ProjectID:   projectID,
		ActorUserID: actorUserID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Rule, 0, len(rows))
	for _, row := range rows {
		out = append(out, ruleFromRow(row))
	}
	return out, nil
}

func (r *PgRepository) CreateFire(ctx context.Context, fire Fire) (Fire, error) {
	row, err := r.q.CreateAutomationFire(ctx, queries.CreateAutomationFireParams{
		TenantID:        fire.TenantID,
		RuleID:          fire.RuleID,
		ScheduledFireAt: pgtype.Timestamptz{Time: fire.ScheduledFireAt, Valid: true},
		IdempotencyKey:  fire.IdempotencyKey,
		Status:          fire.Status,
		DemandID:        uuidPtr(fire.DemandID),
		RunID:           uuidPtr(fire.RunID),
		ErrorCode:       textPtr(fire.ErrorCode),
		ErrorMessage:    textPtr(fire.ErrorMessage),
	})
	if err != nil {
		return Fire{}, err
	}
	return fireFromRow(row), nil
}

func (r *PgRepository) GetFireByIdempotency(ctx context.Context, idempotencyKey string) (Fire, error) {
	row, err := r.q.GetAutomationFireByIdempotency(ctx, idempotencyKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Fire{}, ErrNotFound
		}
		return Fire{}, err
	}
	return fireFromRow(row), nil
}

func (r *PgRepository) UpdateFire(ctx context.Context, fire Fire) (Fire, error) {
	row, err := r.q.UpdateAutomationFire(ctx, queries.UpdateAutomationFireParams{
		ID:           fire.ID,
		TenantID:     fire.TenantID,
		Status:       fire.Status,
		DemandID:     uuidPtr(fire.DemandID),
		RunID:        uuidPtr(fire.RunID),
		ErrorCode:    textPtr(fire.ErrorCode),
		ErrorMessage: textPtr(fire.ErrorMessage),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Fire{}, ErrNotFound
		}
		return Fire{}, err
	}
	return fireFromRow(row), nil
}

func (r *PgRepository) ListFires(ctx context.Context, req ListFiresRequest) ([]Fire, error) {
	rows, err := r.q.ListAutomationFires(ctx, queries.ListAutomationFiresParams{
		TenantID:    req.TenantID,
		RuleID:      req.RuleID,
		LimitCount:  req.Limit,
		OffsetCount: req.Offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Fire, 0, len(rows))
	for _, row := range rows {
		out = append(out, fireFromRow(row))
	}
	return out, nil
}

func (r *PgRepository) GetLatestNonTerminalFire(ctx context.Context, tenantID, ruleID uuid.UUID) (Fire, error) {
	row, err := r.q.GetLatestNonTerminalAutomationFire(ctx, queries.GetLatestNonTerminalAutomationFireParams{
		TenantID: tenantID,
		RuleID:   ruleID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Fire{}, ErrNotFound
		}
		return Fire{}, err
	}
	return fireFromRow(row), nil
}

func ruleFromRow(row queries.AutomationRule) Rule {
	return Rule{
		ID:                      row.ID,
		TenantID:                row.TenantID,
		TeamID:                  row.TeamID,
		ProjectID:               row.ProjectID,
		Name:                    row.Name,
		Enabled:                 row.Enabled,
		CoordinationMode:        row.CoordinationMode,
		DemandTitleTemplate:     textFrom(row.DemandTitleTemplate),
		DemandBodyTemplate:      textFrom(row.DemandBodyTemplate),
		ScenarioTemplateKey:     textFrom(row.ScenarioTemplateKey),
		DigitalEmployeeID:       uuidFrom(row.DigitalEmployeeID),
		ChatObjectiveTemplate:   textFrom(row.ChatObjectiveTemplate),
		ScheduleKind:            row.ScheduleKind,
		CronExpr:                textFrom(row.CronExpr),
		IntervalSeconds:         int4From(row.IntervalSeconds),
		Timezone:                row.Timezone,
		OverlapPolicy:           row.OverlapPolicy,
		ActorUserID:             row.ActorUserID,
		DisabledReason:          textFrom(row.DisabledReason),
		ConsecutiveFailureCount: row.ConsecutiveFailureCount,
		TemporalScheduleID:      textFrom(row.TemporalScheduleID),
		CreatedAt:               row.CreatedAt.Time,
		UpdatedAt:               row.UpdatedAt.Time,
	}
}

func fireFromRow(row queries.AutomationFire) Fire {
	return Fire{
		ID:              row.ID,
		TenantID:        row.TenantID,
		RuleID:          row.RuleID,
		ScheduledFireAt: row.ScheduledFireAt.Time,
		IdempotencyKey:  row.IdempotencyKey,
		Status:          row.Status,
		DemandID:        uuidFrom(row.DemandID),
		RunID:           uuidFrom(row.RunID),
		ErrorCode:       textFrom(row.ErrorCode),
		ErrorMessage:    textFrom(row.ErrorMessage),
		CreatedAt:       row.CreatedAt.Time,
	}
}

func textPtr(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}

func textFrom(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}

func uuidPtr(v *uuid.UUID) uuid.NullUUID {
	if v == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *v, Valid: true}
}

func uuidFrom(v uuid.NullUUID) *uuid.UUID {
	if !v.Valid {
		return nil
	}
	id := v.UUID
	return &id
}

func int4Ptr(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}

func int4From(v pgtype.Int4) *int32 {
	if !v.Valid {
		return nil
	}
	n := v.Int32
	return &n
}

func boolPtr(v *bool) pgtype.Bool {
	if v == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *v, Valid: true}
}

package teamlending

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/storage/queries"
)

// PgRepository 基于 sqlc 的团队借调仓储实现。
type PgRepository struct {
	q *queries.Queries
}

// NewPgRepository 构造借调仓储。q 不能为空。
func NewPgRepository(q *queries.Queries) *PgRepository {
	return &PgRepository{q: q}
}

func (r *PgRepository) UpsertPolicy(ctx context.Context, params UpsertPolicyParams) (Policy, error) {
	budget, err := numericFromDecimalString(params.BudgetCeiling)
	if err != nil {
		return Policy{}, fmt.Errorf("encode budget_ceiling: %w", err)
	}
	capability, err := jsonbFromMap(params.CapabilityCeiling, "capability_ceiling")
	if err != nil {
		return Policy{}, err
	}
	match, err := jsonbFromMap(params.ProjectMatch, "project_match")
	if err != nil {
		return Policy{}, err
	}
	actor := nullUUIDFromPtr(params.ActorUserID)
	record, err := r.q.UpsertTeamLendingPolicy(ctx, queries.UpsertTeamLendingPolicyParams{
		TenantID:          params.TenantID,
		TeamID:            params.TeamID,
		AllowLending:      params.AllowLending,
		ApprovalMode:      string(params.ApprovalMode),
		BudgetCeiling:     budget,
		CapabilityCeiling: capability,
		ProjectMatch:      match,
		CreatedByUserID:   actor,
		UpdatedByUserID:   actor,
	})
	if err != nil {
		return Policy{}, mapConstraintError(err)
	}
	return policyFromQuery(record), nil
}

func (r *PgRepository) GetPolicy(ctx context.Context, tenantID, teamID uuid.UUID) (Policy, error) {
	record, err := r.q.GetTeamLendingPolicy(ctx, queries.GetTeamLendingPolicyParams{
		TenantID: tenantID,
		TeamID:   teamID,
	})
	if err != nil {
		return Policy{}, mapNoRows(err)
	}
	return policyFromQuery(record), nil
}

func (r *PgRepository) CreateRequest(ctx context.Context, params CreateRequestParams) (Request, error) {
	requestedBudget, err := numericFromDecimalString(params.RequestedBudget)
	if err != nil {
		return Request{}, fmt.Errorf("encode requested_budget: %w", err)
	}
	grantedBudget, err := numericFromDecimalString(params.GrantedBudget)
	if err != nil {
		return Request{}, fmt.Errorf("encode granted_budget: %w", err)
	}
	requestedCapability, err := jsonbFromMap(params.RequestedCapability, "requested_capability")
	if err != nil {
		return Request{}, err
	}
	grantedCapability, err := jsonbFromMap(params.GrantedCapability, "granted_capability")
	if err != nil {
		return Request{}, err
	}
	record, err := r.q.CreateTeamLendingRequest(ctx, queries.CreateTeamLendingRequestParams{
		TenantID:            params.TenantID,
		TeamID:              params.TeamID,
		ProjectID:           params.ProjectID,
		Status:              string(params.Status),
		RequestedByUserID:   params.RequestedByUserID,
		RequestReason:       params.RequestReason,
		RequestedBudget:     requestedBudget,
		RequestedCapability: requestedCapability,
		GrantedBudget:       grantedBudget,
		GrantedCapability:   grantedCapability,
		IsException:         params.IsException,
	})
	if err != nil {
		// (tenant, project, team) 上已存在有效借调 → 唯一索引冲突。
		return Request{}, mapConstraintError(err)
	}
	return requestFromQuery(record), nil
}

func (r *PgRepository) GetRequest(ctx context.Context, tenantID, requestID uuid.UUID) (Request, error) {
	record, err := r.q.GetTeamLendingRequest(ctx, queries.GetTeamLendingRequestParams{
		ID:       requestID,
		TenantID: tenantID,
	})
	if err != nil {
		return Request{}, mapNoRows(err)
	}
	return requestFromQuery(record), nil
}

func (r *PgRepository) ListRequestsByTeam(ctx context.Context, params ListTeamRequestsParams) ([]Request, error) {
	rows, err := r.q.ListTeamLendingRequestsByTeam(ctx, queries.ListTeamLendingRequestsByTeamParams{
		TenantID: params.TenantID,
		TeamID:   params.TeamID,
		Status:   textFromRequestStatus(params.Status),
		Limit:    params.Limit,
		Offset:   params.Offset,
	})
	if err != nil {
		return nil, err
	}
	return requestsFromQuery(rows), nil
}

func (r *PgRepository) ListRequestsByProject(ctx context.Context, params ListProjectRequestsParams) ([]Request, error) {
	rows, err := r.q.ListTeamLendingRequestsByProject(ctx, queries.ListTeamLendingRequestsByProjectParams{
		TenantID:  params.TenantID,
		ProjectID: params.ProjectID,
		Status:    textFromRequestStatus(params.Status),
		Limit:     params.Limit,
		Offset:    params.Offset,
	})
	if err != nil {
		return nil, err
	}
	return requestsFromQuery(rows), nil
}

func (r *PgRepository) ApproveRequest(ctx context.Context, params DecideRequestParams) (Request, error) {
	grantedBudget, err := numericFromDecimalString(params.GrantedBudget)
	if err != nil {
		return Request{}, fmt.Errorf("encode granted_budget: %w", err)
	}
	grantedCapability, err := optionalJSONBFromMap(params.GrantedCapability, "granted_capability")
	if err != nil {
		return Request{}, err
	}
	record, err := r.q.ApproveTeamLendingRequest(ctx, queries.ApproveTeamLendingRequestParams{
		ID:                params.RequestID,
		TenantID:          params.TenantID,
		TeamID:            params.TeamID,
		DecidedByUserID:   params.DecidedByUserID,
		DecisionReason:    params.DecisionReason,
		GrantedBudget:     grantedBudget,
		GrantedCapability: grantedCapability,
	})
	if err != nil {
		return Request{}, mapDecisionError(err)
	}
	return requestFromQuery(record), nil
}

func (r *PgRepository) RejectRequest(ctx context.Context, params DecideRequestParams) (Request, error) {
	record, err := r.q.RejectTeamLendingRequest(ctx, queries.RejectTeamLendingRequestParams{
		ID:              params.RequestID,
		TenantID:        params.TenantID,
		TeamID:          params.TeamID,
		DecidedByUserID: params.DecidedByUserID,
		DecisionReason:  params.DecisionReason,
	})
	if err != nil {
		return Request{}, mapDecisionError(err)
	}
	return requestFromQuery(record), nil
}

func (r *PgRepository) RevokeRequest(ctx context.Context, params DecideRequestParams) (Request, error) {
	record, err := r.q.RevokeTeamLendingRequest(ctx, queries.RevokeTeamLendingRequestParams{
		ID:              params.RequestID,
		TenantID:        params.TenantID,
		TeamID:          params.TeamID,
		DecidedByUserID: params.DecidedByUserID,
		DecisionReason:  params.DecisionReason,
	})
	if err != nil {
		return Request{}, mapDecisionError(err)
	}
	return requestFromQuery(record), nil
}

func (r *PgRepository) GetTeamOwnerUserIDs(ctx context.Context, tenantID, teamID uuid.UUID) ([]uuid.UUID, error) {
	team, err := r.q.GetTenantTeam(ctx, queries.GetTenantTeamParams{
		ID:       teamID,
		TenantID: tenantID,
	})
	if err != nil {
		return nil, mapNoRows(err)
	}
	owners := make([]uuid.UUID, 0, len(team.HumanOwnerUserIds))
	for _, owner := range team.HumanOwnerUserIds {
		if owner == uuid.Nil {
			continue
		}
		owners = append(owners, owner)
	}
	return owners, nil
}

func policyFromQuery(record queries.TeamLendingPolicy) Policy {
	budget, _ := numericToString(record.BudgetCeiling)
	capability, _ := mapFromJSONB(record.CapabilityCeiling, "capability_ceiling")
	match, _ := mapFromJSONB(record.ProjectMatch, "project_match")
	return Policy{
		ID:                record.ID,
		TenantID:          record.TenantID,
		TeamID:            record.TeamID,
		AllowLending:      record.AllowLending,
		ApprovalMode:      ApprovalMode(record.ApprovalMode),
		BudgetCeiling:     budget,
		CapabilityCeiling: capability,
		ProjectMatch:      match,
		Status:            record.Status,
		CreatedByUserID:   uuidPtrFromNull(record.CreatedByUserID),
		UpdatedByUserID:   uuidPtrFromNull(record.UpdatedByUserID),
		CreatedAt:         timeFromTimestamptz(record.CreatedAt),
		UpdatedAt:         timeFromTimestamptz(record.UpdatedAt),
	}
}

func requestFromQuery(record queries.TeamLendingRequest) Request {
	requestedBudget, _ := numericToString(record.RequestedBudget)
	grantedBudget, _ := numericToString(record.GrantedBudget)
	requestedCapability, _ := mapFromJSONB(record.RequestedCapability, "requested_capability")
	grantedCapability, _ := mapFromJSONB(record.GrantedCapability, "granted_capability")
	return Request{
		ID:                  record.ID,
		TenantID:            record.TenantID,
		TeamID:              record.TeamID,
		ProjectID:           record.ProjectID,
		Status:              RequestStatus(record.Status),
		RequestedByUserID:   record.RequestedByUserID,
		RequestReason:       record.RequestReason,
		RequestedBudget:     requestedBudget,
		RequestedCapability: requestedCapability,
		GrantedBudget:       grantedBudget,
		GrantedCapability:   grantedCapability,
		IsException:         record.IsException,
		DecidedByUserID:     uuidPtrFromNull(record.DecidedByUserID),
		DecidedAt:           timePtrFromTimestamptz(record.DecidedAt),
		DecisionReason:      record.DecisionReason,
		CreatedAt:           timeFromTimestamptz(record.CreatedAt),
		UpdatedAt:           timeFromTimestamptz(record.UpdatedAt),
	}
}

func requestsFromQuery(rows []queries.TeamLendingRequest) []Request {
	out := make([]Request, 0, len(rows))
	for _, row := range rows {
		out = append(out, requestFromQuery(row))
	}
	return out
}

// ---- 错误映射 ----

func mapNoRows(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func mapConstraintError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return fmt.Errorf("%w: active lending request already exists for this project and team", ErrDuplicateRequest)
	}
	return err
}

// mapDecisionError 把裁决类 UPDATE 的 no-rows（状态机不允许）映射为 ErrInvalidTransition。
func mapDecisionError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidTransition
	}
	return err
}

// ---- 编码/解码辅助（与 tenant/project 模块一致） ----

func numericFromDecimalString(value string) (pgtype.Numeric, error) {
	if value == "" {
		return pgtype.Numeric{}, nil
	}
	var numeric pgtype.Numeric
	if err := numeric.Scan(value); err != nil {
		return pgtype.Numeric{}, err
	}
	return numeric, nil
}

func numericToString(value pgtype.Numeric) (string, error) {
	if !value.Valid {
		return "", nil
	}
	encoded, err := value.Value()
	if err != nil {
		return "", err
	}
	if encoded == nil {
		return "", nil
	}
	return fmt.Sprint(encoded), nil
}

func jsonbFromMap(value map[string]any, field string) ([]byte, error) {
	encoded, err := json.Marshal(cloneMap(value))
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", field, err)
	}
	return encoded, nil
}

func optionalJSONBFromMap(value map[string]any, field string) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	return jsonbFromMap(value, field)
}

func mapFromJSONB(raw []byte, field string) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("decode %s: %w", field, err)
	}
	if decoded == nil {
		return map[string]any{}, nil
	}
	return decoded, nil
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(value))
	for key, val := range value {
		cloned[key] = val
	}
	return cloned
}

func timeFromTimestamptz(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time.UTC()
}

func timePtrFromTimestamptz(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time.UTC()
	return &t
}

func nullUUIDFromPtr(value uuid.UUID) uuid.NullUUID {
	if value == uuid.Nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: value, Valid: true}
}

func uuidPtrFromNull(value uuid.NullUUID) *uuid.UUID {
	if !value.Valid || value.UUID == uuid.Nil {
		return nil
	}
	copied := value.UUID
	return &copied
}

func textFromRequestStatus(status RequestStatus) pgtype.Text {
	if status == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: string(status), Valid: true}
}

package authzcenter

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/superteam/control-plane/internal/authz"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type PgQueryStore interface {
	CountAuthzDecisionsSince(ctx context.Context, since pgtype.Timestamptz) (queries.CountAuthzDecisionsSinceRow, error)
	ListTopDeniedAuthzActionsSince(ctx context.Context, params queries.ListTopDeniedAuthzActionsSinceParams) ([]queries.ListTopDeniedAuthzActionsSinceRow, error)
	ListAuthzDecisions(ctx context.Context, params queries.ListAuthzDecisionsParams) ([]queries.WebOperationLog, error)
	ListRuntimeNodesWithScopes(ctx context.Context) ([]queries.ListRuntimeNodesWithScopesRow, error)
	CreateRuntimeNodeScope(ctx context.Context, params queries.CreateRuntimeNodeScopeParams) (queries.RuntimeNodeScope, error)
	UpdateRuntimeNodeScopeStatus(ctx context.Context, params queries.UpdateRuntimeNodeScopeStatusParams) (queries.RuntimeNodeScope, error)
	ListAuthzMembers(ctx context.Context, params queries.ListAuthzMembersParams) ([]queries.ListAuthzMembersRow, error)
	CreateWebOperationLog(ctx context.Context, params queries.CreateWebOperationLogParams) (queries.WebOperationLog, error)
}

type PgRepository struct {
	q PgQueryStore
}

func NewPgRepository(q PgQueryStore) *PgRepository {
	return &PgRepository{q: q}
}

func (r *PgRepository) CountDecisionsSince(ctx context.Context, since time.Time) (DecisionTotals, error) {
	row, err := r.q.CountAuthzDecisionsSince(ctx, pgtype.Timestamptz{Time: since, Valid: true})
	if err != nil {
		return DecisionTotals{}, err
	}
	return DecisionTotals{Total: row.Total, Allowed: row.Allowed, Denied: row.Denied}, nil
}

func (r *PgRepository) ListTopDeniedActionsSince(ctx context.Context, since time.Time, limit int32) ([]ActionCount, error) {
	rows, err := r.q.ListTopDeniedAuthzActionsSince(ctx, queries.ListTopDeniedAuthzActionsSinceParams{
		Since: pgtype.Timestamptz{Time: since, Valid: true},
		Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	items := make([]ActionCount, 0, len(rows))
	for _, row := range rows {
		items = append(items, ActionCount{Action: row.Action, Count: row.Count})
	}
	return items, nil
}

func (r *PgRepository) ListDecisions(ctx context.Context, filter DecisionFilter) ([]DecisionRecord, error) {
	rows, err := r.q.ListAuthzDecisions(ctx, queries.ListAuthzDecisionsParams{
		Result:       nullableText(filter.Result),
		Action:       nullableText(filter.Action),
		ActorType:    nullableText(filter.ActorType),
		ActorID:      nullableText(filter.ActorID),
		ResourceType: nullableText(filter.ResourceType),
		ResourceID:   nullableText(filter.ResourceID),
		RequestID:    nullableText(filter.RequestID),
		Offset:       filter.Offset,
		Limit:        filter.Limit,
	})
	if err != nil {
		return nil, err
	}
	items := make([]DecisionRecord, 0, len(rows))
	for _, row := range rows {
		items = append(items, decisionFromOperationLog(row))
	}
	return items, nil
}

func (r *PgRepository) ListRuntimeScopeNodes(ctx context.Context) ([]RuntimeScopeNodeRecord, error) {
	rows, err := r.q.ListRuntimeNodesWithScopes(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]RuntimeScopeNodeRecord, 0, len(rows))
	nodeIndexes := map[uuid.UUID]int{}
	for _, row := range rows {
		index, ok := nodeIndexes[row.RuntimeNodeID]
		if !ok {
			index = len(nodes)
			nodeIndexes[row.RuntimeNodeID] = index
			nodes = append(nodes, RuntimeScopeNodeRecord{
				RuntimeNodeID:      row.RuntimeNodeID,
				TenantID:           row.RuntimeTenantID,
				NodeID:             row.NodeID,
				Name:               row.Name,
				SupportedProviders: parseSupportedProviders(row.SupportedProviders),
				MaxSlots:           row.MaxSlots,
				CurrentLoad:        row.CurrentLoad,
				Status:             row.RuntimeStatus,
				LastHeartbeatAt:    timePtrFromPg(row.LastHeartbeatAt),
				Scopes:             []RuntimeScopeRecord{},
			})
		}
		if row.ScopeID.Valid {
			nodes[index].Scopes = append(nodes[index].Scopes, runtimeScopeFromNodeRow(row))
		}
	}
	return nodes, nil
}

func (r *PgRepository) CreateRuntimeScope(ctx context.Context, input RuntimeScopeInput) (RuntimeScopeRecord, error) {
	scope, err := r.q.CreateRuntimeNodeScope(ctx, queries.CreateRuntimeNodeScopeParams{
		TenantID:      input.TenantID,
		RuntimeNodeID: input.RuntimeNodeID,
		TeamID:        nullUUID(input.TeamID),
		ScopeType:     input.ScopeType,
		ScopeValue:    input.ScopeValue,
	})
	if err != nil {
		return RuntimeScopeRecord{}, mapNotFoundError(err)
	}
	return runtimeScopeFromQuery(scope), nil
}

func (r *PgRepository) UpdateRuntimeScopeStatus(ctx context.Context, scopeID uuid.UUID, status string) (RuntimeScopeRecord, error) {
	scope, err := r.q.UpdateRuntimeNodeScopeStatus(ctx, queries.UpdateRuntimeNodeScopeStatusParams{
		ID:     scopeID,
		Status: status,
	})
	if err != nil {
		return RuntimeScopeRecord{}, mapNotFoundError(err)
	}
	return runtimeScopeFromQuery(scope), nil
}

func (r *PgRepository) ListMembers(ctx context.Context, filter MemberFilter) ([]MemberRecord, error) {
	rows, err := r.q.ListAuthzMembers(ctx, queries.ListAuthzMembersParams{
		Offset: filter.Offset,
		Limit:  filter.Limit,
	})
	if err != nil {
		return nil, err
	}
	members := make([]MemberRecord, 0, len(rows))
	indexes := map[uuid.UUID]int{}
	for _, row := range rows {
		index, ok := indexes[row.UserID]
		if !ok {
			index = len(members)
			indexes[row.UserID] = index
			members = append(members, MemberRecord{
				UserID:        row.UserID,
				Username:      row.UserUsername,
				DisplayName:   stringPtrFromPg(row.UserDisplayName),
				Email:         stringPtrFromPg(row.UserEmail),
				AccountStatus: row.AccountStatus,
				Memberships:   []MembershipRecord{},
			})
		}
		if row.TenantID.Valid && row.PrincipalID.Valid {
			membership := MembershipRecord{
				TenantID:      row.TenantID.UUID,
				TeamID:        uuidPtrFromNull(row.TeamID),
				PrincipalType: row.PrincipalType.String,
				PrincipalID:   row.PrincipalID.UUID,
				Role:          row.Role.String,
				Status:        row.MembershipStatus.String,
			}
			members[index].Memberships = append(members[index].Memberships, membership)
			if row.AccountStatus == "active" && membership.Status == "active" && roleAllowsConsoleAccess(membership.Role) {
				members[index].ConsoleAccess = true
			}
		}
	}
	return members, nil
}

func (r *PgRepository) RecordOperationLog(ctx context.Context, input OperationLogInput) error {
	details := input.Details
	if details == nil {
		details = map[string]any{}
	}
	payload, err := json.Marshal(details)
	if err != nil {
		return err
	}
	tenantID := input.TenantID
	if tenantID == uuid.Nil {
		tenantID = input.Actor.TenantID
	}
	_, err = r.q.CreateWebOperationLog(ctx, queries.CreateWebOperationLogParams{
		TenantID:     uuid.NullUUID{UUID: tenantID, Valid: tenantID != uuid.Nil},
		UserID:       uuid.NullUUID{UUID: input.Actor.UserID, Valid: input.Actor.UserID != uuid.Nil},
		Username:     nullableText(input.Actor.Username),
		Module:       input.Module,
		ResourceType: nullableText(input.ResourceType),
		ResourceID:   nullableText(input.ResourceID),
		Action:       input.Action,
		Result:       input.Result,
		RequestID:    nullableText(input.RequestID),
		ClientIp:     nullableText(input.ClientIP),
		UserAgent:    nullableText(input.UserAgent),
		Details:      payload,
	})
	return err
}

func decisionFromOperationLog(row queries.WebOperationLog) DecisionRecord {
	details := mapFromJSON(row.Details)
	return DecisionRecord{
		ID:           row.ID,
		TenantID:     row.TenantID,
		UserID:       uuidPtrFromNull(row.UserID),
		Username:     stringPtrFromPg(row.Username),
		Module:       row.Module,
		ResourceType: stringPtrFromPg(row.ResourceType),
		ResourceID:   stringPtrFromPg(row.ResourceID),
		Action:       row.Action,
		Result:       row.Result,
		RequestID:    stringPtrFromPg(row.RequestID),
		ActorType:    detailString(details, "actor_type", "actor", "type"),
		ActorID:      detailString(details, "actor_id", "actor", "id"),
		Engine:       detailString(details, "engine", "decision", "engine", "snapshot", "engine"),
		Reason:       detailString(details, "reason", "decision", "reason", "snapshot", "reason"),
		MatchedRule:  detailString(details, "matched_rule", "decision", "matched_rule", "snapshot", "matched_rule"),
		Details:      details,
		CreatedAt:    timeFromPg(row.CreatedAt),
	}
}

func runtimeScopeFromQuery(scope queries.RuntimeNodeScope) RuntimeScopeRecord {
	return RuntimeScopeRecord{
		ID:            scope.ID,
		TenantID:      scope.TenantID,
		RuntimeNodeID: scope.RuntimeNodeID,
		TeamID:        uuidPtrFromNull(scope.TeamID),
		ScopeType:     RuntimeScopeScopeType(scope.ScopeType),
		ScopeValue:    scope.ScopeValue,
		Status:        RuntimeScopeStatus(scope.Status),
		DisabledAt:    timePtrFromPg(scope.DisabledAt),
		CreatedAt:     timeFromPg(scope.CreatedAt),
		UpdatedAt:     timeFromPg(scope.UpdatedAt),
	}
}

func runtimeScopeFromNodeRow(row queries.ListRuntimeNodesWithScopesRow) RuntimeScopeRecord {
	tenantID := row.ScopeTenantID.UUID
	if !row.ScopeTenantID.Valid {
		tenantID = row.RuntimeTenantID
	}
	runtimeNodeID := row.ScopeRuntimeNodeID.UUID
	if !row.ScopeRuntimeNodeID.Valid {
		runtimeNodeID = row.RuntimeNodeID
	}
	return RuntimeScopeRecord{
		ID:            row.ScopeID.UUID,
		TenantID:      tenantID,
		RuntimeNodeID: runtimeNodeID,
		TeamID:        uuidPtrFromNull(row.ScopeTeamID),
		ScopeType:     RuntimeScopeScopeType(row.ScopeType.String),
		ScopeValue:    row.ScopeValue.String,
		Status:        RuntimeScopeStatus(row.ScopeStatus.String),
		DisabledAt:    timePtrFromPg(row.ScopeDisabledAt),
		CreatedAt:     timeFromPg(row.ScopeCreatedAt),
		UpdatedAt:     timeFromPg(row.ScopeUpdatedAt),
	}
}

func parseSupportedProviders(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var providers []string
	if err := json.Unmarshal(raw, &providers); err == nil {
		return providers
	}
	var wrapped struct {
		Providers []string `json:"providers"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil {
		return wrapped.Providers
	}
	return []string{}
}

func mapFromJSON(raw []byte) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var details map[string]any
	if err := json.Unmarshal(raw, &details); err != nil || details == nil {
		return map[string]any{}
	}
	return details
}

func detailString(details map[string]any, flatKey string, nestedPairs ...string) *string {
	if value, ok := details[flatKey].(string); ok && value != "" {
		return &value
	}
	for i := 0; i+1 < len(nestedPairs); i += 2 {
		nestedKey := nestedPairs[i]
		nestedField := nestedPairs[i+1]
		if nested, ok := details[nestedKey].(map[string]any); ok {
			if value, ok := nested[nestedField].(string); ok && value != "" {
				return &value
			}
		}
	}
	return nil
}

func roleAllowsConsoleAccess(role string) bool {
	switch role {
	case authz.RoleOwner, authz.RoleAdmin, authz.RoleMember, authz.RoleViewer:
		return true
	default:
		return false
	}
}

func nullUUID(value *uuid.UUID) uuid.NullUUID {
	if value == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: *value, Valid: true}
}

func uuidPtrFromNull(value uuid.NullUUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	id := value.UUID
	return &id
}

func nullableText(value string) pgtype.Text {
	return pgtype.Text{String: value, Valid: value != ""}
}

func stringPtrFromPg(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	text := value.String
	return &text
}

func timeFromPg(value pgtype.Timestamptz) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func timePtrFromPg(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}

func mapNotFoundError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

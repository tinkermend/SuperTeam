package project

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/storage/queries"
)

// PgDigitalEmployeeRoleSource reads employee roles and summaries for casting.
type PgDigitalEmployeeRoleSource struct {
	q *queries.Queries
}

func NewPgDigitalEmployeeRoleSource(q *queries.Queries) *PgDigitalEmployeeRoleSource {
	return &PgDigitalEmployeeRoleSource{q: q}
}

func (s *PgDigitalEmployeeRoleSource) ListEmployeesByRoleKey(ctx context.Context, tenantID uuid.UUID, roleKey string) ([]DigitalEmployeeRoleHolder, error) {
	rows, err := s.q.ListDigitalEmployeesByRoleKey(ctx, queries.ListDigitalEmployeesByRoleKeyParams{
		TenantID: tenantID,
		RoleKey:  roleKey,
	})
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(rows))
	seen := map[uuid.UUID]struct{}{}
	for _, row := range rows {
		if _, ok := seen[row.DigitalEmployeeID]; ok {
			continue
		}
		seen[row.DigitalEmployeeID] = struct{}{}
		ids = append(ids, row.DigitalEmployeeID)
	}
	summaries, err := s.ListEmployeeSummaries(ctx, tenantID, ids)
	if err != nil {
		return nil, err
	}
	out := make([]DigitalEmployeeRoleHolder, 0, len(ids))
	for _, id := range ids {
		sum := summaries[id]
		out = append(out, DigitalEmployeeRoleHolder{
			ID:     id,
			Name:   sum.Name,
			TeamID: sum.TeamID,
		})
	}
	return out, nil
}

func (s *PgDigitalEmployeeRoleSource) ListEmployeeRoleKeys(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
	if len(employeeIDs) == 0 {
		return map[uuid.UUID][]string{}, nil
	}
	rows, err := s.q.ListDigitalEmployeeRolesByEmployees(ctx, queries.ListDigitalEmployeeRolesByEmployeesParams{
		TenantID:           tenantID,
		DigitalEmployeeIds: employeeIDs,
	})
	if err != nil {
		return nil, err
	}
	out := map[uuid.UUID][]string{}
	for _, row := range rows {
		out[row.DigitalEmployeeID] = append(out[row.DigitalEmployeeID], row.RoleKey)
	}
	return out, nil
}

func (s *PgDigitalEmployeeRoleSource) ListEmployeeSummaries(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]DigitalEmployeeSummary, error) {
	out := map[uuid.UUID]DigitalEmployeeSummary{}
	if len(employeeIDs) == 0 {
		return out, nil
	}
	for _, id := range employeeIDs {
		row, err := s.q.GetDigitalEmployee(ctx, queries.GetDigitalEmployeeParams{
			TenantID: tenantID,
			ID:       id,
		})
		if err != nil {
			continue
		}
		sum := DigitalEmployeeSummary{
			ID:     row.ID,
			Name:   row.Name,
			Status: row.Status,
		}
		if row.TeamID.Valid {
			tid := row.TeamID.UUID
			sum.TeamID = &tid
			if team, err := s.q.GetTenantTeam(ctx, queries.GetTenantTeamParams{
				ID:       tid,
				TenantID: tenantID,
			}); err == nil {
				sum.TeamName = team.Name
			}
		}
		if cfg, err := s.q.GetCurrentDigitalEmployeeConfigRevision(ctx, queries.GetCurrentDigitalEmployeeConfigRevisionParams{
			TenantID:          tenantID,
			DigitalEmployeeID: id,
		}); err == nil && len(cfg.CapabilityBindings) > 0 {
			var bindings map[string]any
			if json.Unmarshal(cfg.CapabilityBindings, &bindings) == nil {
				sum.CapabilityBindings = bindings
			}
		}
		out[id] = sum
	}
	roles, err := s.ListEmployeeRoleKeys(ctx, tenantID, employeeIDs)
	if err != nil {
		return nil, err
	}
	for id, keys := range roles {
		sum := out[id]
		sum.RoleKeys = keys
		out[id] = sum
	}
	return out, nil
}

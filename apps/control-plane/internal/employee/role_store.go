package employee

import (
	"context"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/storage/queries"
)

// PgEmployeeRoleStore implements EmployeeRoleStore via sqlc.
type PgEmployeeRoleStore struct {
	q *queries.Queries
}

func NewPgEmployeeRoleStore(q *queries.Queries) *PgEmployeeRoleStore {
	return &PgEmployeeRoleStore{q: q}
}

func (s *PgEmployeeRoleStore) ListRoleKeys(ctx context.Context, tenantID, employeeID uuid.UUID) ([]string, error) {
	rows, err := s.q.ListDigitalEmployeeRoles(ctx, queries.ListDigitalEmployeeRolesParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.RoleKey)
	}
	return out, nil
}

func (s *PgEmployeeRoleStore) ReplaceRoleKeys(ctx context.Context, tenantID, employeeID uuid.UUID, roleKeys []string) error {
	if err := s.q.ReplaceDigitalEmployeeRolesDelete(ctx, queries.ReplaceDigitalEmployeeRolesDeleteParams{
		TenantID:          tenantID,
		DigitalEmployeeID: employeeID,
	}); err != nil {
		return err
	}
	for _, key := range roleKeys {
		if err := s.q.InsertDigitalEmployeeRole(ctx, queries.InsertDigitalEmployeeRoleParams{
			TenantID:          tenantID,
			DigitalEmployeeID: employeeID,
			RoleKey:           key,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *PgEmployeeRoleStore) ListRoleKeysByEmployees(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID][]string, error) {
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

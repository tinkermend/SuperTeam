package project

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
)

// CastingInvalidationNotifier fans out inbox alerts for cascade-invalidated castings.
// Implemented in app/ to keep project free of inbox package coupling.
type CastingInvalidationNotifier interface {
	NotifyCastingInvalidated(ctx context.Context, req CastingInvalidationNotifyRequest) error
	// ResolveCastingAlerts closes open 编制失效 alerts for a project. Alerts carry no
	// human verb (照 channel_alert 先例), so re-casting is the ONLY closer — without
	// this the card sits open forever and the inbox accumulates false alarms.
	ResolveCastingAlerts(ctx context.Context, tenantID, projectID uuid.UUID) error
}

// CastingInvalidationNotifyRequest carries one cascade batch for owner alerts.
type CastingInvalidationNotifyRequest struct {
	TenantID     uuid.UUID
	ProjectID    uuid.UUID
	ProjectName  string
	OwnerUserIDs []uuid.UUID
	// Trigger: "employee_role_removed" | "role_vocabulary_disabled"
	Trigger      string
	RoleKeys     []string
	EmployeeID   uuid.UUID
	EmployeeName string
	ActorUserID  uuid.UUID
	ActorSummary string
}

// SetCastingInvalidationNotifier wires optional inbox alerts for cascade removes.
func (s *Service) SetCastingInvalidationNotifier(n CastingInvalidationNotifier) {
	s.castingInvalidationNotifier = n
}

// ListEmployeeRoleImpact previews which castings would drop if roleKeys were removed
// from the employee. Empty roleKeys = impact of removing all currently held roles
// is not inferred here — callers pass the concrete removed set (or all keys).
func (s *Service) ListEmployeeRoleImpact(ctx context.Context, tenantID, employeeID uuid.UUID, roleKeys []string) (EmployeeRoleImpact, error) {
	if s.castingRepo == nil {
		return EmployeeRoleImpact{}, fmt.Errorf("casting repository not configured")
	}
	if tenantID == uuid.Nil || employeeID == uuid.Nil {
		return EmployeeRoleImpact{}, fmt.Errorf("%w: tenant_id and employee_id are required", ErrInvalidProject)
	}
	keys := uniqueStrings(roleKeys)
	rows, err := s.castingRepo.ListCastingsForEmployeeRoles(ctx, tenantID, employeeID, keys)
	if err != nil {
		return EmployeeRoleImpact{}, err
	}
	// Enrich employee names when role source is available.
	if s.employeeRoles != nil && len(rows) > 0 {
		if sums, err := s.employeeRoles.ListEmployeeSummaries(ctx, tenantID, []uuid.UUID{employeeID}); err == nil {
			if sum, ok := sums[employeeID]; ok {
				for i := range rows {
					rows[i].EmployeeName = sum.Name
				}
			}
		}
	}
	return EmployeeRoleImpact{AffectedCastings: rows, AffectedCount: len(rows)}, nil
}

// NotifyCascadedCastingInvalidation emits project events + owner alerts for rows
// already deleted in an external transaction (employee/rolevocab same-txn path).
func (s *Service) NotifyCascadedCastingInvalidation(
	ctx context.Context,
	tenantID, actorUserID uuid.UUID,
	rows []AffectedCastingRow,
	trigger string,
	employeeID uuid.UUID,
	employeeName string,
) {
	if len(rows) == 0 {
		return
	}
	s.emitCastingInvalidated(ctx, tenantID, actorUserID, rows, trigger, employeeID, employeeName)
}

// ResolveCastingInvalidationAlerts closes open 编制失效 alerts for a project.
// Called after a successful PutCasting: the human has re-cast, so the alert that
// said "编制已解除" is now false. Best effort — never blocks the write.
func (s *Service) ResolveCastingInvalidationAlerts(ctx context.Context, tenantID, projectID uuid.UUID) {
	if s == nil || s.castingInvalidationNotifier == nil {
		return
	}
	if err := s.castingInvalidationNotifier.ResolveCastingAlerts(ctx, tenantID, projectID); err != nil {
		slog.Warn("resolve casting invalidation alerts failed",
			"project_id", projectID.String(), "error", err)
	}
}

// CascadeInvalidateEmployeeRoles deletes castings for the removed role keys, writes
// project events, and notifies project owners. Caller must already have confirmed.
func (s *Service) CascadeInvalidateEmployeeRoles(
	ctx context.Context,
	tenantID, employeeID, actorUserID uuid.UUID,
	roleKeys []string,
	employeeName string,
) (EmployeeRoleImpact, error) {
	if s.castingRepo == nil {
		return EmployeeRoleImpact{}, fmt.Errorf("casting repository not configured")
	}
	keys := uniqueStrings(roleKeys)
	if len(keys) == 0 {
		return EmployeeRoleImpact{}, nil
	}
	impact, err := s.ListEmployeeRoleImpact(ctx, tenantID, employeeID, keys)
	if err != nil {
		return EmployeeRoleImpact{}, err
	}
	if impact.AffectedCount == 0 {
		return impact, nil
	}
	if err := s.castingRepo.DeleteCastingsForEmployeeRoles(ctx, tenantID, employeeID, keys); err != nil {
		return EmployeeRoleImpact{}, err
	}
	if employeeName == "" {
		for _, row := range impact.AffectedCastings {
			if row.EmployeeName != "" {
				employeeName = row.EmployeeName
				break
			}
		}
	}
	s.emitCastingInvalidated(ctx, tenantID, actorUserID, impact.AffectedCastings, "employee_role_removed", employeeID, employeeName)
	return impact, nil
}

// ListRoleKeyCastingImpact lists castings that would be removed if roleKey were disabled.
func (s *Service) ListRoleKeyCastingImpact(ctx context.Context, tenantID uuid.UUID, roleKey string) (EmployeeRoleImpact, error) {
	if s.castingRepo == nil {
		return EmployeeRoleImpact{}, fmt.Errorf("casting repository not configured")
	}
	roleKey = strings.TrimSpace(roleKey)
	if tenantID == uuid.Nil || roleKey == "" {
		return EmployeeRoleImpact{}, fmt.Errorf("%w: tenant_id and role_key are required", ErrInvalidProject)
	}
	rows, err := s.castingRepo.ListCastingsForRoleKey(ctx, tenantID, roleKey)
	if err != nil {
		return EmployeeRoleImpact{}, err
	}
	impact := EmployeeRoleImpact{AffectedCastings: rows, AffectedCount: len(rows)}
	if s.employeeRoles != nil && len(rows) > 0 {
		ids := make([]uuid.UUID, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.DigitalEmployeeID)
		}
		if sums, err := s.employeeRoles.ListEmployeeSummaries(ctx, tenantID, uniqueUUIDs(ids)); err == nil {
			for i := range impact.AffectedCastings {
				if sum, ok := sums[impact.AffectedCastings[i].DigitalEmployeeID]; ok {
					impact.AffectedCastings[i].EmployeeName = sum.Name
				}
			}
		}
	}
	return impact, nil
}

// CascadeInvalidateRoleKey deletes all castings for a vocabulary role and notifies owners.
func (s *Service) CascadeInvalidateRoleKey(
	ctx context.Context,
	tenantID uuid.UUID,
	roleKey string,
	actorUserID uuid.UUID,
) (EmployeeRoleImpact, error) {
	if s.castingRepo == nil {
		return EmployeeRoleImpact{}, fmt.Errorf("casting repository not configured")
	}
	roleKey = strings.TrimSpace(roleKey)
	if tenantID == uuid.Nil || roleKey == "" {
		return EmployeeRoleImpact{}, fmt.Errorf("%w: tenant_id and role_key are required", ErrInvalidProject)
	}
	rows, err := s.castingRepo.ListCastingsForRoleKey(ctx, tenantID, roleKey)
	if err != nil {
		return EmployeeRoleImpact{}, err
	}
	impact := EmployeeRoleImpact{AffectedCastings: rows, AffectedCount: len(rows)}
	if impact.AffectedCount == 0 {
		return impact, nil
	}
	// Enrich employee names.
	if s.employeeRoles != nil {
		ids := make([]uuid.UUID, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.DigitalEmployeeID)
		}
		if sums, err := s.employeeRoles.ListEmployeeSummaries(ctx, tenantID, uniqueUUIDs(ids)); err == nil {
			for i := range impact.AffectedCastings {
				if sum, ok := sums[impact.AffectedCastings[i].DigitalEmployeeID]; ok {
					impact.AffectedCastings[i].EmployeeName = sum.Name
				}
			}
		}
	}
	if err := s.castingRepo.DeleteCastingsForRoleKey(ctx, tenantID, roleKey); err != nil {
		return EmployeeRoleImpact{}, err
	}
	s.emitCastingInvalidated(ctx, tenantID, actorUserID, impact.AffectedCastings, "role_vocabulary_disabled", uuid.Nil, "")
	return impact, nil
}

func (s *Service) emitCastingInvalidated(
	ctx context.Context,
	tenantID, actorUserID uuid.UUID,
	rows []AffectedCastingRow,
	trigger string,
	employeeID uuid.UUID,
	employeeName string,
) {
	// Group by project for one event + one alert batch per project.
	type projGroup struct {
		name     string
		roleKeys []string
		rows     []AffectedCastingRow
	}
	byProject := map[uuid.UUID]*projGroup{}
	order := make([]uuid.UUID, 0)
	for _, row := range rows {
		g, ok := byProject[row.ProjectID]
		if !ok {
			g = &projGroup{name: row.ProjectName}
			byProject[row.ProjectID] = g
			order = append(order, row.ProjectID)
		}
		g.rows = append(g.rows, row)
		g.roleKeys = append(g.roleKeys, row.RoleKey)
	}
	actorType := "human_user"
	actorID := actorUserID.String()
	if actorUserID == uuid.Nil {
		actorType = "system"
		actorID = "system"
	}
	for _, projectID := range order {
		g := byProject[projectID]
		roleKeys := uniqueStrings(g.roleKeys)
		summary := "剧本编制已因角色变更解除"
		if trigger == "role_vocabulary_disabled" {
			summary = "剧本编制已因角色词表停用解除"
		}
		payload := map[string]any{
			"event":     "project.casting.invalidated",
			"trigger":   trigger,
			"role_keys": roleKeys,
		}
		if employeeID != uuid.Nil {
			payload["employee_id"] = employeeID.String()
		}
		if employeeName != "" {
			payload["employee_name"] = employeeName
		}
		if s.repository != nil {
			_, _ = s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
				TenantID:  tenantID,
				ProjectID: projectID,
				EventType: ProjectEventConfigChanged,
				ActorType: actorType,
				ActorID:   actorID,
				Summary:   summary,
				Payload:   payload,
			})
		}
		if s.castingInvalidationNotifier == nil {
			continue
		}
		owners := s.projectOwnerUserIDs(ctx, tenantID, projectID)
		if len(owners) == 0 {
			continue
		}
		_ = s.castingInvalidationNotifier.NotifyCastingInvalidated(ctx, CastingInvalidationNotifyRequest{
			TenantID:     tenantID,
			ProjectID:    projectID,
			ProjectName:  g.name,
			OwnerUserIDs: owners,
			Trigger:      trigger,
			RoleKeys:     roleKeys,
			EmployeeID:   employeeID,
			EmployeeName: employeeName,
			ActorUserID:  actorUserID,
		})
	}
}

func (s *Service) projectOwnerUserIDs(ctx context.Context, tenantID, projectID uuid.UUID) []uuid.UUID {
	if s == nil || s.repository == nil {
		return nil
	}
	record, err := s.repository.GetProject(ctx, tenantID, projectID)
	if err != nil {
		return nil
	}
	seen := map[uuid.UUID]struct{}{}
	var out []uuid.UUID
	add := func(id uuid.UUID) {
		if id == uuid.Nil {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	for _, id := range record.HumanOwnerUserIDs {
		add(id)
	}
	add(record.HumanOwnerUserID)
	return out
}

package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/superteam/control-plane/internal/automation"
	"github.com/superteam/control-plane/internal/employee"
	"github.com/superteam/control-plane/internal/inbox"
	"github.com/superteam/control-plane/internal/project"
	"github.com/superteam/control-plane/internal/rolevocab"
	"github.com/superteam/control-plane/internal/storage/queries"
)

type txBeginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

// castingInvalidationNotifierAdapter implements project.CastingInvalidationNotifier.
type castingInvalidationNotifierAdapter struct {
	inbox *inbox.Service
}

func newCastingInvalidationNotifier(inboxService *inbox.Service) *castingInvalidationNotifierAdapter {
	return &castingInvalidationNotifierAdapter{inbox: inboxService}
}

func (a *castingInvalidationNotifierAdapter) NotifyCastingInvalidated(ctx context.Context, req project.CastingInvalidationNotifyRequest) error {
	if a == nil || a.inbox == nil {
		return nil
	}
	if len(req.OwnerUserIDs) == 0 {
		return nil
	}
	title := "编制失效"
	if strings.TrimSpace(req.ProjectName) != "" {
		title = "编制失效：" + strings.TrimSpace(req.ProjectName)
	}
	roles := strings.Join(req.RoleKeys, ", ")
	summary := fmt.Sprintf("项目编制已解除（角色：%s）。", roles)
	switch req.Trigger {
	case "employee_role_removed":
		name := strings.TrimSpace(req.EmployeeName)
		if name == "" && req.EmployeeID != uuid.Nil {
			name = req.EmployeeID.String()
		}
		if name != "" {
			summary = fmt.Sprintf("数字员工 %s 的角色变更导致编制解除（角色：%s）。请重新编制或调整剧本。", name, roles)
		} else {
			summary = fmt.Sprintf("员工角色变更导致编制解除（角色：%s）。请重新编制或调整剧本。", roles)
		}
	case "role_vocabulary_disabled":
		summary = fmt.Sprintf("角色词表停用导致编制解除（角色：%s）。请重新编制或调整剧本。", roles)
	}
	now := time.Now().UTC()
	projectID := req.ProjectID
	for _, userID := range req.OwnerUserIDs {
		if _, err := a.inbox.UpsertItem(ctx, inbox.UpsertItemRequest{
			TenantID:        req.TenantID,
			TargetUserID:    userID,
			Scope:           "personal",
			ItemType:        inbox.ItemTypeCastingInvalidated,
			SourceType:      inbox.SourceTypeProjectCasting,
			SourceID:        req.ProjectID,
			SourceProjectID: &projectID,
			Title:           title,
			Summary:         summary,
			Priority:        "high",
			Status:          inbox.StatusOpen,
			Actions:         []inbox.Action{},
			DeepLink: map[string]any{
				"type":  "casting_invalidated",
				"route": fmt.Sprintf("/projects/%s/config?tab=casting", req.ProjectID.String()),
			},
			ContextPayload: map[string]any{
				"trigger":      req.Trigger,
				"role_keys":    req.RoleKeys,
				"employee_id":  req.EmployeeID.String(),
				"project_id":   req.ProjectID.String(),
				"project_name": req.ProjectName,
			},
			LastActivityAt: now,
		}); err != nil {
			return fmt.Errorf("upsert casting invalidated alert for user %s: %w", userID, err)
		}
	}
	return nil
}

func (a *castingInvalidationNotifierAdapter) ResolveCastingAlerts(ctx context.Context, tenantID, projectID uuid.UUID) error {
	if a == nil || a.inbox == nil {
		return nil
	}
	return a.inbox.ResolveOpenItemsBySource(ctx, tenantID, inbox.SourceTypeProjectCasting, projectID)
}

// automationAlertNotifierAdapter implements automation.AlertNotifier.
type automationAlertNotifierAdapter struct {
	inbox *inbox.Service
}

func newAutomationAlertNotifier(inboxService *inbox.Service) *automationAlertNotifierAdapter {
	return &automationAlertNotifierAdapter{inbox: inboxService}
}

func (a *automationAlertNotifierAdapter) OpenRuleFailureAlert(ctx context.Context, req automation.RuleFailureAlert) error {
	if a == nil || a.inbox == nil {
		return nil
	}
	if len(req.OwnerUserIDs) == 0 {
		return nil
	}
	title := "自动化规则执行失败"
	if strings.TrimSpace(req.RuleName) != "" {
		title = "自动化规则执行失败：" + strings.TrimSpace(req.RuleName)
	}
	summary := strings.TrimSpace(req.ErrorMessage)
	if summary == "" {
		summary = "规则执行失败，请检查编制与配置后重试。"
	}
	if strings.TrimSpace(req.ProjectName) != "" {
		summary = fmt.Sprintf("项目 %s · %s", req.ProjectName, summary)
	}
	now := time.Now().UTC()
	projectID := req.ProjectID
	for _, userID := range req.OwnerUserIDs {
		if _, err := a.inbox.UpsertItem(ctx, inbox.UpsertItemRequest{
			TenantID:        req.TenantID,
			TargetUserID:    userID,
			Scope:           "personal",
			ItemType:        inbox.ItemTypeAutomationAlert,
			SourceType:      inbox.SourceTypeAutomationRule,
			SourceID:        req.RuleID,
			SourceProjectID: &projectID,
			Title:           title,
			Summary:         summary,
			Priority:        "high",
			Status:          inbox.StatusOpen,
			Actions:         []inbox.Action{},
			DeepLink: map[string]any{
				"type":  "automation_fire_failed",
				"route": fmt.Sprintf("/automations?rule=%s", req.RuleID.String()),
			},
			ContextPayload: map[string]any{
				"rule_id":       req.RuleID.String(),
				"rule_name":     req.RuleName,
				"project_id":    req.ProjectID.String(),
				"project_name":  req.ProjectName,
				"error_code":    req.ErrorCode,
				"error_message": req.ErrorMessage,
				"fire_id":       req.FireID.String(),
			},
			LastActivityAt: now,
		}); err != nil {
			return fmt.Errorf("upsert automation alert for user %s: %w", userID, err)
		}
	}
	return nil
}

func (a *automationAlertNotifierAdapter) ResolveRuleAlerts(ctx context.Context, tenantID, ruleID uuid.UUID) error {
	if a == nil || a.inbox == nil {
		return nil
	}
	return a.inbox.ResolveOpenItemsBySource(ctx, tenantID, inbox.SourceTypeAutomationRule, ruleID)
}

// employeeCastingImpactAdapter bridges employee → project for role-impact/cascade.
// Role rewrite + casting deletes share one Postgres transaction; events/alerts after commit.
type employeeCastingImpactAdapter struct {
	projects *project.Service
	q        *queries.Queries
	db       txBeginner
}

func newEmployeeCastingImpactAdapter(projects *project.Service, q *queries.Queries, db txBeginner) *employeeCastingImpactAdapter {
	return &employeeCastingImpactAdapter{projects: projects, q: q, db: db}
}

func (a *employeeCastingImpactAdapter) ListEmployeeRoleImpact(ctx context.Context, tenantID, employeeID uuid.UUID, roleKeys []string) (employee.CastingRoleImpact, error) {
	if a == nil || a.projects == nil {
		return employee.CastingRoleImpact{}, nil
	}
	impact, err := a.projects.ListEmployeeRoleImpact(ctx, tenantID, employeeID, roleKeys)
	if err != nil {
		return employee.CastingRoleImpact{}, err
	}
	return toEmployeeImpact(impact), nil
}

func (a *employeeCastingImpactAdapter) CommitRoleReplaceWithCascade(ctx context.Context, req employee.RoleReplaceCascadeRequest) error {
	if a == nil || a.projects == nil || a.q == nil || a.db == nil {
		return fmt.Errorf("casting cascade dependencies not configured")
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin role-replace cascade tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := a.q.WithTx(tx)

	// 影响面快照必须与删除在同一事务视图内取:在事务外先查再删,两者之间新增的
	// 编制行会被删掉却不产生事件与告警——正是本批要封的静默失真。
	affected, err := qtx.ListCastingsForEmployeeRoles(ctx, queries.ListCastingsForEmployeeRolesParams{
		TenantID:          req.TenantID,
		DigitalEmployeeID: req.EmployeeID,
		RoleKeys:          req.RemovedKeys,
	})
	if err != nil {
		return err
	}
	impact := project.EmployeeRoleImpact{
		AffectedCastings: castingRowsFromEmployeeRoleQuery(affected),
	}
	impact.AffectedCount = len(impact.AffectedCastings)

	if err := qtx.ReplaceDigitalEmployeeRolesDelete(ctx, queries.ReplaceDigitalEmployeeRolesDeleteParams{
		TenantID:          req.TenantID,
		DigitalEmployeeID: req.EmployeeID,
	}); err != nil {
		return err
	}
	for _, key := range req.NewRoleKeys {
		if err := qtx.InsertDigitalEmployeeRole(ctx, queries.InsertDigitalEmployeeRoleParams{
			TenantID:          req.TenantID,
			DigitalEmployeeID: req.EmployeeID,
			RoleKey:           key,
		}); err != nil {
			return err
		}
	}
	if len(req.RemovedKeys) > 0 {
		if err := qtx.DeleteCastingsForEmployeeRoles(ctx, queries.DeleteCastingsForEmployeeRolesParams{
			TenantID:          req.TenantID,
			DigitalEmployeeID: req.EmployeeID,
			RoleKeys:          req.RemovedKeys,
		}); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit role-replace cascade tx: %w", err)
	}

	if impact.AffectedCount > 0 {
		a.projects.NotifyCascadedCastingInvalidation(
			ctx,
			req.TenantID,
			req.ActorUserID,
			impact.AffectedCastings,
			"employee_role_removed",
			req.EmployeeID,
			req.EmployeeName,
		)
	}
	return nil
}

func castingRowsFromEmployeeRoleQuery(rows []queries.ListCastingsForEmployeeRolesRow) []project.AffectedCastingRow {
	out := make([]project.AffectedCastingRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, project.AffectedCastingRow{
			ProjectID:           row.ProjectID,
			ProjectName:         row.ProjectName,
			ScenarioTemplateKey: row.ScenarioTemplateKey,
			TemplateName:        row.TemplateName,
			RoleKey:             row.RoleKey,
			DigitalEmployeeID:   row.DigitalEmployeeID,
		})
	}
	return out
}

func castingRowsFromRoleKeyQuery(rows []queries.ListCastingsForRoleKeyRow) []project.AffectedCastingRow {
	out := make([]project.AffectedCastingRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, project.AffectedCastingRow{
			ProjectID:           row.ProjectID,
			ProjectName:         row.ProjectName,
			ScenarioTemplateKey: row.ScenarioTemplateKey,
			TemplateName:        row.TemplateName,
			RoleKey:             row.RoleKey,
			DigitalEmployeeID:   row.DigitalEmployeeID,
		})
	}
	return out
}

func toEmployeeImpact(in project.EmployeeRoleImpact) employee.CastingRoleImpact {
	out := employee.CastingRoleImpact{
		AffectedCastings: make([]employee.CastingImpactRow, 0, len(in.AffectedCastings)),
		AffectedCount:    in.AffectedCount,
	}
	for _, row := range in.AffectedCastings {
		out.AffectedCastings = append(out.AffectedCastings, employee.CastingImpactRow{
			ProjectID:           row.ProjectID,
			ProjectName:         row.ProjectName,
			ScenarioTemplateKey: row.ScenarioTemplateKey,
			TemplateName:        row.TemplateName,
			RoleKey:             row.RoleKey,
		})
	}
	return out
}

// roleVocabCastingCascadeAdapter bridges rolevocab disable → casting cascade.
// Casting deletes + vocabulary patch share one transaction; events/alerts after commit.
type roleVocabCastingCascadeAdapter struct {
	projects *project.Service
	q        *queries.Queries
	db       txBeginner
}

func newRoleVocabCastingCascadeAdapter(projects *project.Service, q *queries.Queries, db txBeginner) *roleVocabCastingCascadeAdapter {
	return &roleVocabCastingCascadeAdapter{projects: projects, q: q, db: db}
}

func (a *roleVocabCastingCascadeAdapter) DisableRoleWithCascade(ctx context.Context, req rolevocab.PatchRequest) (rolevocab.Entry, error) {
	if a == nil || a.projects == nil || a.q == nil || a.db == nil {
		return rolevocab.Entry{}, fmt.Errorf("role vocabulary cascade dependencies not configured")
	}
	key := strings.TrimSpace(req.RoleKey)

	var title, description, status pgtype.Text
	if req.Title != nil {
		title = pgtype.Text{String: strings.TrimSpace(*req.Title), Valid: true}
	}
	if req.Description != nil {
		description = pgtype.Text{String: strings.TrimSpace(*req.Description), Valid: true}
	}
	if req.Status != nil {
		status = pgtype.Text{String: strings.TrimSpace(*req.Status), Valid: true}
	} else {
		status = pgtype.Text{String: "disabled", Valid: true}
	}

	tx, err := a.db.Begin(ctx)
	if err != nil {
		return rolevocab.Entry{}, fmt.Errorf("begin role-disable cascade tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := a.q.WithTx(tx)

	// 同事务快照,理由同 CommitRoleReplaceWithCascade。
	affected, err := qtx.ListCastingsForRoleKey(ctx, queries.ListCastingsForRoleKeyParams{
		TenantID: req.TenantID,
		RoleKey:  key,
	})
	if err != nil {
		return rolevocab.Entry{}, err
	}
	impact := project.EmployeeRoleImpact{
		AffectedCastings: castingRowsFromRoleKeyQuery(affected),
	}
	impact.AffectedCount = len(impact.AffectedCastings)

	if err := qtx.DeleteCastingsForRoleKey(ctx, queries.DeleteCastingsForRoleKeyParams{
		TenantID: req.TenantID,
		RoleKey:  key,
	}); err != nil {
		return rolevocab.Entry{}, err
	}
	row, err := qtx.UpdateRoleVocabulary(ctx, queries.UpdateRoleVocabularyParams{
		TenantID:    req.TenantID,
		RoleKey:     key,
		Title:       title,
		Description: description,
		Status:      status,
	})
	if err != nil {
		return rolevocab.Entry{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return rolevocab.Entry{}, fmt.Errorf("commit role-disable cascade tx: %w", err)
	}

	if impact.AffectedCount > 0 {
		a.projects.NotifyCascadedCastingInvalidation(
			ctx,
			req.TenantID,
			req.ActorUserID,
			impact.AffectedCastings,
			"role_vocabulary_disabled",
			uuid.Nil,
			"",
		)
	}

	entry := rolevocab.Entry{
		ID:          row.ID,
		TenantID:    row.TenantID,
		RoleKey:     row.RoleKey,
		Title:       row.Title,
		Description: row.Description,
		Status:      row.Status,
	}
	if row.CreatedAt.Valid {
		entry.CreatedAt = row.CreatedAt.Time
	}
	if row.UpdatedAt.Valid {
		entry.UpdatedAt = row.UpdatedAt.Time
	}
	return entry, nil
}

var (
	_ project.CastingInvalidationNotifier = (*castingInvalidationNotifierAdapter)(nil)
	_ automation.AlertNotifier            = (*automationAlertNotifierAdapter)(nil)
	_ employee.CastingImpactGateway       = (*employeeCastingImpactAdapter)(nil)
	_ rolevocab.CastingCascade            = (*roleVocabCastingCascadeAdapter)(nil)
)

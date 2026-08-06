package automation

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/superteam/control-plane/internal/project"
)

// AlertNotifier fans out automation fire failure alerts to project owners.
// Implemented in app/ against inbox.Service (channel_alert pattern).
type AlertNotifier interface {
	OpenRuleFailureAlert(ctx context.Context, req RuleFailureAlert) error
	ResolveRuleAlerts(ctx context.Context, tenantID, ruleID uuid.UUID) error
}

// RuleFailureAlert is one fire-failed notification payload.
type RuleFailureAlert struct {
	TenantID     uuid.UUID
	RuleID       uuid.UUID
	RuleName     string
	ProjectID    uuid.UUID
	ProjectName  string
	OwnerUserIDs []uuid.UUID
	ErrorCode    string
	ErrorMessage string
	FireID       uuid.UUID
}

type Service struct {
	repo      Repository
	projects  ProjectGateway
	demands   DemandSubmitter
	chats     ChatRunner
	schedules ScheduleSyncer
	alerts    AlertNotifier
	now       func() time.Time
}

func NewService(repo Repository, projects ProjectGateway, demands DemandSubmitter, chats ChatRunner, schedules ScheduleSyncer) *Service {
	return &Service{
		repo:      repo,
		projects:  projects,
		demands:   demands,
		chats:     chats,
		schedules: schedules,
		now:       time.Now,
	}
}

// SetAlertNotifier wires optional inbox alerts for fire failures.
func (s *Service) SetAlertNotifier(n AlertNotifier) {
	if s != nil {
		s.alerts = n
	}
}

func (s *Service) WithClock(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

func (s *Service) ListRules(ctx context.Context, req ListRulesRequest) ([]Rule, error) {
	req.Limit, req.Offset = normalizePagination(req.Limit, req.Offset)
	rules, err := s.repo.ListRules(ctx, req)
	if err != nil {
		return nil, err
	}
	for i := range rules {
		s.enrichRule(ctx, &rules[i])
	}
	return rules, nil
}

func (s *Service) GetRule(ctx context.Context, tenantID, ruleID uuid.UUID) (Rule, error) {
	if tenantID == uuid.Nil || ruleID == uuid.Nil {
		return Rule{}, ErrInvalidInput
	}
	rule, err := s.repo.GetRule(ctx, tenantID, ruleID)
	if err != nil {
		return Rule{}, err
	}
	s.enrichRule(ctx, &rule)
	return rule, nil
}

func (s *Service) enrichRule(ctx context.Context, rule *Rule) {
	if s == nil || rule == nil || s.projects == nil {
		return
	}
	if info, err := s.projects.GetProject(ctx, rule.TenantID, rule.ProjectID); err == nil {
		rule.ProjectName = strings.TrimSpace(info.Name)
	}
	if s.repo == nil {
		return
	}
	fires, err := s.repo.ListFires(ctx, ListFiresRequest{
		TenantID: rule.TenantID,
		RuleID:   rule.ID,
		Limit:    1,
		Offset:   0,
	})
	if err != nil || len(fires) == 0 {
		return
	}
	latest := fires[0]
	rule.LatestFire = &latest
}

func (s *Service) CreateRule(ctx context.Context, req CreateRuleRequest) (Rule, error) {
	if req.TenantID == uuid.Nil || req.ActorUserID == uuid.Nil || req.ProjectID == uuid.Nil {
		return Rule{}, ErrInvalidInput
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return Rule{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	mode := strings.TrimSpace(req.CoordinationMode)
	if mode == "" {
		mode = ModeLoop
	}
	if err := validateMode(mode); err != nil {
		return Rule{}, err
	}
	scheduleKind, cronExpr, intervalSeconds, timezone, err := normalizeSchedule(req.ScheduleKind, req.CronExpr, req.IntervalSeconds, req.Timezone)
	if err != nil {
		return Rule{}, err
	}
	if err := validateModeFields(mode, req); err != nil {
		return Rule{}, err
	}

	eligible, err := s.projects.IsEligibleInitiator(ctx, req.TenantID, req.ProjectID, req.ActorUserID)
	if err != nil {
		return Rule{}, err
	}
	if !eligible {
		return Rule{}, ErrActorNotEligible
	}
	projectInfo, err := s.projects.GetProject(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return Rule{}, err
	}
	if projectInfo.TeamID == uuid.Nil {
		return Rule{}, fmt.Errorf("%w: project has no team", ErrInvalidInput)
	}
	if err := s.validateCastingForTemplate(ctx, req.TenantID, req.ProjectID, req.ScenarioTemplateKey); err != nil {
		return Rule{}, err
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	rule := Rule{
		TenantID:              req.TenantID,
		TeamID:                projectInfo.TeamID,
		ProjectID:             req.ProjectID,
		Name:                  name,
		Enabled:               enabled,
		CoordinationMode:      mode,
		DemandTitleTemplate:   trimPtr(req.DemandTitleTemplate),
		DemandBodyTemplate:    trimPtr(req.DemandBodyTemplate),
		ScenarioTemplateKey:   trimPtr(req.ScenarioTemplateKey),
		DigitalEmployeeID:     req.DigitalEmployeeID,
		ChatObjectiveTemplate: trimPtr(req.ChatObjectiveTemplate),
		ScheduleKind:          scheduleKind,
		CronExpr:              cronExpr,
		IntervalSeconds:       intervalSeconds,
		Timezone:              timezone,
		OverlapPolicy:         OverlapSkip,
		ActorUserID:           req.ActorUserID,
	}
	created, err := s.repo.CreateRule(ctx, rule)
	if err != nil {
		return Rule{}, err
	}
	if err := s.syncScheduleCreate(ctx, &created); err != nil {
		return Rule{}, err
	}
	s.enrichRule(ctx, &created)
	return created, nil
}

func (s *Service) UpdateRule(ctx context.Context, req UpdateRuleRequest) (Rule, error) {
	if req.TenantID == uuid.Nil || req.RuleID == uuid.Nil || req.ActorUserID == uuid.Nil {
		return Rule{}, ErrInvalidInput
	}
	rule, err := s.repo.GetRule(ctx, req.TenantID, req.RuleID)
	if err != nil {
		return Rule{}, err
	}
	eligible, err := s.projects.IsEligibleInitiator(ctx, req.TenantID, rule.ProjectID, req.ActorUserID)
	if err != nil {
		return Rule{}, err
	}
	if !eligible {
		return Rule{}, ErrActorNotEligible
	}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return Rule{}, fmt.Errorf("%w: name is required", ErrInvalidInput)
		}
		rule.Name = name
	}
	if req.DemandTitleTemplate != nil {
		rule.DemandTitleTemplate = trimPtr(req.DemandTitleTemplate)
	}
	if req.DemandBodyTemplate != nil {
		rule.DemandBodyTemplate = trimPtr(req.DemandBodyTemplate)
	}
	if req.ScenarioTemplateKey != nil {
		rule.ScenarioTemplateKey = trimPtr(req.ScenarioTemplateKey)
	}
	if req.DigitalEmployeeID != nil {
		rule.DigitalEmployeeID = req.DigitalEmployeeID
	}
	if req.ChatObjectiveTemplate != nil {
		rule.ChatObjectiveTemplate = trimPtr(req.ChatObjectiveTemplate)
	}

	scheduleKind := rule.ScheduleKind
	cronExpr := rule.CronExpr
	intervalSeconds := rule.IntervalSeconds
	timezone := rule.Timezone
	if req.ScheduleKind != nil {
		scheduleKind = strings.TrimSpace(*req.ScheduleKind)
	}
	if req.CronExpr != nil {
		cronExpr = trimPtr(req.CronExpr)
	}
	if req.IntervalSeconds != nil {
		intervalSeconds = req.IntervalSeconds
	}
	if req.Timezone != nil {
		timezone = strings.TrimSpace(*req.Timezone)
	}
	normalizedKind, normalizedCron, normalizedInterval, normalizedTZ, err := normalizeSchedule(scheduleKind, cronExpr, intervalSeconds, timezone)
	if err != nil {
		return Rule{}, err
	}
	rule.ScheduleKind = normalizedKind
	rule.CronExpr = normalizedCron
	rule.IntervalSeconds = normalizedInterval
	rule.Timezone = normalizedTZ

	if err := validateModeFields(rule.CoordinationMode, CreateRuleRequest{
		CoordinationMode:      rule.CoordinationMode,
		DemandTitleTemplate:   rule.DemandTitleTemplate,
		DigitalEmployeeID:     rule.DigitalEmployeeID,
		ChatObjectiveTemplate: rule.ChatObjectiveTemplate,
	}); err != nil {
		return Rule{}, err
	}
	if err := s.validateCastingForTemplate(ctx, req.TenantID, rule.ProjectID, rule.ScenarioTemplateKey); err != nil {
		return Rule{}, err
	}

	updated, err := s.repo.UpdateRule(ctx, rule)
	if err != nil {
		return Rule{}, err
	}
	if err := s.syncScheduleUpdate(ctx, updated); err != nil {
		return Rule{}, err
	}
	s.enrichRule(ctx, &updated)
	return updated, nil
}

func (s *Service) DeleteRule(ctx context.Context, tenantID, ruleID, actorUserID uuid.UUID) error {
	if tenantID == uuid.Nil || ruleID == uuid.Nil || actorUserID == uuid.Nil {
		return ErrInvalidInput
	}
	rule, err := s.repo.GetRule(ctx, tenantID, ruleID)
	if err != nil {
		return err
	}
	eligible, err := s.projects.IsEligibleInitiator(ctx, tenantID, rule.ProjectID, actorUserID)
	if err != nil {
		return err
	}
	if !eligible {
		return ErrActorNotEligible
	}
	if rule.TemporalScheduleID != nil && *rule.TemporalScheduleID != "" && s.schedules != nil {
		if err := s.schedules.Delete(ctx, *rule.TemporalScheduleID); err != nil {
			slog.Warn("automation schedule delete failed", "schedule_id", *rule.TemporalScheduleID, "error", err)
		}
	}
	if err := s.repo.DeleteRule(ctx, tenantID, ruleID); err != nil {
		return err
	}
	// 规则没了，指向它的失败告警就是死链:告警无人类动词,只能由"下一次成功"关闭,
	// 而删掉的规则永远不会再成功——不在这里关就永久滞留。
	if s.alerts != nil {
		if err := s.alerts.ResolveRuleAlerts(ctx, tenantID, ruleID); err != nil {
			slog.Warn("automation resolve rule alerts on delete failed", "rule_id", ruleID, "error", err)
		}
	}
	return nil
}

func (s *Service) Enable(ctx context.Context, tenantID, ruleID, actorUserID uuid.UUID) (Rule, error) {
	if tenantID == uuid.Nil || ruleID == uuid.Nil || actorUserID == uuid.Nil {
		return Rule{}, ErrInvalidInput
	}
	rule, err := s.repo.GetRule(ctx, tenantID, ruleID)
	if err != nil {
		return Rule{}, err
	}
	eligible, err := s.projects.IsEligibleInitiator(ctx, tenantID, rule.ProjectID, rule.ActorUserID)
	if err != nil {
		return Rule{}, err
	}
	if !eligible {
		return Rule{}, ErrActorNotEligible
	}
	callerEligible, err := s.projects.IsEligibleInitiator(ctx, tenantID, rule.ProjectID, actorUserID)
	if err != nil {
		return Rule{}, err
	}
	if !callerEligible {
		return Rule{}, ErrActorNotEligible
	}
	updated, err := s.repo.SetRuleEnabled(ctx, tenantID, ruleID, true, nil)
	if err != nil {
		return Rule{}, err
	}
	if updated.TemporalScheduleID != nil && *updated.TemporalScheduleID != "" && s.schedules != nil {
		if err := s.schedules.Unpause(ctx, *updated.TemporalScheduleID, "enabled by user"); err != nil {
			return Rule{}, err
		}
	} else if err := s.syncScheduleCreate(ctx, &updated); err != nil {
		return Rule{}, err
	}
	s.enrichRule(ctx, &updated)
	return updated, nil
}

func (s *Service) Disable(ctx context.Context, tenantID, ruleID, actorUserID uuid.UUID) (Rule, error) {
	if tenantID == uuid.Nil || ruleID == uuid.Nil || actorUserID == uuid.Nil {
		return Rule{}, ErrInvalidInput
	}
	rule, err := s.repo.GetRule(ctx, tenantID, ruleID)
	if err != nil {
		return Rule{}, err
	}
	eligible, err := s.projects.IsEligibleInitiator(ctx, tenantID, rule.ProjectID, actorUserID)
	if err != nil {
		return Rule{}, err
	}
	if !eligible {
		return Rule{}, ErrActorNotEligible
	}
	reason := DisabledReasonUserDisabled
	updated, err := s.repo.SetRuleEnabled(ctx, tenantID, ruleID, false, &reason)
	if err != nil {
		return Rule{}, err
	}
	if updated.TemporalScheduleID != nil && *updated.TemporalScheduleID != "" && s.schedules != nil {
		if err := s.schedules.Pause(ctx, *updated.TemporalScheduleID, reason); err != nil {
			slog.Warn("automation schedule pause failed", "schedule_id", *updated.TemporalScheduleID, "error", err)
		}
	}
	s.enrichRule(ctx, &updated)
	return updated, nil
}

func (s *Service) Trigger(ctx context.Context, req TriggerRequest) (Fire, error) {
	if req.TenantID == uuid.Nil || req.RuleID == uuid.Nil || req.ActorUserID == uuid.Nil {
		return Fire{}, ErrInvalidInput
	}
	rule, err := s.repo.GetRule(ctx, req.TenantID, req.RuleID)
	if err != nil {
		return Fire{}, err
	}
	eligible, err := s.projects.IsEligibleInitiator(ctx, req.TenantID, rule.ProjectID, req.ActorUserID)
	if err != nil {
		return Fire{}, err
	}
	if !eligible {
		return Fire{}, ErrActorNotEligible
	}
	return s.Fire(ctx, req.TenantID, req.RuleID, s.now().UTC())
}

func (s *Service) Fire(ctx context.Context, tenantID, ruleID uuid.UUID, scheduledFireAt time.Time) (Fire, error) {
	if tenantID == uuid.Nil || ruleID == uuid.Nil || scheduledFireAt.IsZero() {
		return Fire{}, ErrInvalidInput
	}
	scheduledFireAt = scheduledFireAt.UTC().Truncate(time.Second)
	idempotencyKey := fmt.Sprintf("%s:%s", ruleID.String(), scheduledFireAt.Format(time.RFC3339))

	if existing, err := s.repo.GetFireByIdempotency(ctx, idempotencyKey); err == nil {
		return existing, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Fire{}, err
	}

	rule, err := s.repo.GetRule(ctx, tenantID, ruleID)
	if err != nil {
		return Fire{}, err
	}

	pending := Fire{
		TenantID:        tenantID,
		RuleID:          ruleID,
		ScheduledFireAt: scheduledFireAt,
		IdempotencyKey:  idempotencyKey,
		Status:          FireStatusPending,
	}
	fire, err := s.repo.CreateFire(ctx, pending)
	if err != nil {
		if isUniqueViolation(err) {
			existing, getErr := s.repo.GetFireByIdempotency(ctx, idempotencyKey)
			if getErr == nil {
				return existing, nil
			}
		}
		return Fire{}, err
	}

	if !rule.Enabled {
		return s.finishFire(ctx, fire, FireStatusSkippedDisabled, nil, nil, "rule_disabled", "rule is disabled")
	}

	eligible, err := s.projects.IsEligibleInitiator(ctx, tenantID, rule.ProjectID, rule.ActorUserID)
	if err != nil {
		return s.failFire(ctx, rule, fire, "eligibility_check_failed", err.Error())
	}
	if !eligible {
		_, _ = s.disableSystem(ctx, rule, DisabledReasonActorRemoved)
		return s.finishFire(ctx, fire, FireStatusSkippedDisabled, nil, nil, DisabledReasonActorRemoved, "actor no longer eligible")
	}

	if rule.OverlapPolicy == OverlapSkip {
		if _, err := s.repo.GetLatestNonTerminalFire(ctx, tenantID, ruleID); err == nil {
			return s.finishFire(ctx, fire, FireStatusSkippedOverlap, nil, nil, "overlap", "prior fire still non-terminal")
		} else if !errors.Is(err, ErrNotFound) {
			return s.failFire(ctx, rule, fire, "overlap_check_failed", err.Error())
		}
	}

	projectInfo, err := s.projects.GetProject(ctx, tenantID, rule.ProjectID)
	if err != nil {
		return s.failFire(ctx, rule, fire, "project_lookup_failed", err.Error())
	}
	// G8: 编制因运行期资源变化失效时允许失败但必须通知（写明原因）。
	if rule.ScenarioTemplateKey != nil && strings.TrimSpace(*rule.ScenarioTemplateKey) != "" {
		if missing, castErr := s.projects.MissingCastingRoles(ctx, tenantID, rule.ProjectID, *rule.ScenarioTemplateKey); castErr != nil {
			return s.failFire(ctx, rule, fire, "casting_check_failed", castErr.Error())
		} else if len(missing) > 0 {
			msg := "剧本编制不完整或编制员工不可用: " + project.FormatCastingInvalidations(missing)
			return s.failFire(ctx, rule, fire, "casting_incomplete", msg)
		}
	}

	switch rule.CoordinationMode {
	case ModePlan, ModeLoop:
		titleTmpl := ""
		if rule.DemandTitleTemplate != nil {
			titleTmpl = *rule.DemandTitleTemplate
		}
		bodyTmpl := ""
		if rule.DemandBodyTemplate != nil {
			bodyTmpl = *rule.DemandBodyTemplate
		}
		title, err := RenderTemplate(titleTmpl, scheduledFireAt, rule.Timezone, rule.Name, projectInfo.Name)
		if err != nil {
			return s.failFire(ctx, rule, fire, "template_render_failed", err.Error())
		}
		if strings.TrimSpace(title) == "" {
			return s.failFire(ctx, rule, fire, "template_empty_title", "rendered title is empty")
		}
		body, err := RenderTemplate(bodyTmpl, scheduledFireAt, rule.Timezone, rule.Name, projectInfo.Name)
		if err != nil {
			return s.failFire(ctx, rule, fire, "template_render_failed", err.Error())
		}
		if s.demands == nil {
			return s.failFire(ctx, rule, fire, "demand_submitter_missing", "demand submitter is not configured")
		}
		result, err := s.demands.SubmitDemand(ctx, DemandSubmitRequest{
			TenantID:            tenantID,
			ProjectID:           rule.ProjectID,
			SubmittedByUserID:   rule.ActorUserID,
			Title:               title,
			Content:             body,
			CoordinationMode:    rule.CoordinationMode,
			ScenarioTemplateKey: rule.ScenarioTemplateKey,
			SourceType:          string(project.DemandSourceAutomation),
			SourceRefs: map[string]any{
				"automation_rule_id": rule.ID.String(),
				"automation_fire_id": fire.ID.String(),
			},
		})
		if err != nil {
			return s.failFire(ctx, rule, fire, "submit_demand_failed", err.Error())
		}
		demandID := result.DemandID
		return s.succeedFire(ctx, rule, fire, &demandID, nil)

	case ModeChat:
		if rule.DigitalEmployeeID == nil || *rule.DigitalEmployeeID == uuid.Nil {
			return s.failFire(ctx, rule, fire, "chat_employee_missing", "digital_employee_id is required")
		}
		objectiveTmpl := ""
		if rule.ChatObjectiveTemplate != nil {
			objectiveTmpl = *rule.ChatObjectiveTemplate
		}
		objective, err := RenderTemplate(objectiveTmpl, scheduledFireAt, rule.Timezone, rule.Name, projectInfo.Name)
		if err != nil {
			return s.failFire(ctx, rule, fire, "template_render_failed", err.Error())
		}
		if strings.TrimSpace(objective) == "" {
			return s.failFire(ctx, rule, fire, "template_empty_objective", "rendered chat objective is empty")
		}
		if s.chats == nil {
			return s.failFire(ctx, rule, fire, "chat_runner_missing", "chat runner is not configured")
		}
		runID, err := s.chats.CreateChatRun(ctx, tenantID, *rule.DigitalEmployeeID, rule.ProjectID, rule.ActorUserID, objective, map[string]any{
			"source_type":        string(project.DemandSourceAutomation),
			"automation_rule_id": rule.ID.String(),
			"automation_fire_id": fire.ID.String(),
		})
		if err != nil {
			return s.failFire(ctx, rule, fire, "create_chat_run_failed", err.Error())
		}
		return s.succeedFire(ctx, rule, fire, nil, &runID)

	default:
		return s.failFire(ctx, rule, fire, "invalid_mode", "unsupported coordination mode")
	}
}

func (s *Service) ListFires(ctx context.Context, req ListFiresRequest) ([]Fire, error) {
	if req.TenantID == uuid.Nil || req.RuleID == uuid.Nil {
		return nil, ErrInvalidInput
	}
	if _, err := s.repo.GetRule(ctx, req.TenantID, req.RuleID); err != nil {
		return nil, err
	}
	req.Limit, req.Offset = normalizePagination(req.Limit, req.Offset)
	return s.repo.ListFires(ctx, req)
}

// validateCastingForTemplate enforces §6.3: automation rule save fails when
// the bound scenario template's casting is incomplete (G7).
func (s *Service) validateCastingForTemplate(ctx context.Context, tenantID, projectID uuid.UUID, templateKey *string) error {
	if templateKey == nil || strings.TrimSpace(*templateKey) == "" || s.projects == nil {
		return nil
	}
	missing, err := s.projects.MissingCastingRoles(ctx, tenantID, projectID, *templateKey)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: 剧本编制不完整，缺角色: %s", ErrInvalidInput, strings.Join(project.CastingInvalidationRoleKeys(missing), ", "))
	}
	return nil
}

func (s *Service) CascadeForProjectDeleted(ctx context.Context, tenantID, projectID uuid.UUID) error {
	if tenantID == uuid.Nil || projectID == uuid.Nil {
		return ErrInvalidInput
	}
	rules, err := s.repo.ListRulesByProject(ctx, tenantID, projectID)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if rule.TemporalScheduleID != nil && *rule.TemporalScheduleID != "" && s.schedules != nil {
			if err := s.schedules.Delete(ctx, *rule.TemporalScheduleID); err != nil {
				slog.Warn("automation schedule delete on project cascade failed",
					"schedule_id", *rule.TemporalScheduleID,
					"project_id", projectID.String(),
					"error", err,
				)
			}
		}
	}
	return s.repo.DeleteRulesForProject(ctx, tenantID, projectID)
}

func (s *Service) DisableForActorRemoved(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID) error {
	if tenantID == uuid.Nil || projectID == uuid.Nil || actorUserID == uuid.Nil {
		return ErrInvalidInput
	}
	rules, err := s.repo.ListEnabledRulesByActorOnProject(ctx, tenantID, projectID, actorUserID)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if _, err := s.disableSystem(ctx, rule, DisabledReasonActorRemoved); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) DisableForActorDeactivated(ctx context.Context, tenantID, actorUserID uuid.UUID) error {
	if tenantID == uuid.Nil || actorUserID == uuid.Nil {
		return ErrInvalidInput
	}
	rules, err := s.repo.ListEnabledRulesByActor(ctx, tenantID, actorUserID)
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if _, err := s.disableSystem(ctx, rule, DisabledReasonActorDeactivated); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) succeedFire(ctx context.Context, rule Rule, fire Fire, demandID, runID *uuid.UUID) (Fire, error) {
	finished, err := s.finishFire(ctx, fire, FireStatusSucceeded, demandID, runID, "", "")
	if err != nil {
		return Fire{}, err
	}
	if _, err := s.repo.ResetFailureCount(ctx, rule.TenantID, rule.ID); err != nil {
		slog.Warn("automation reset failure count failed", "rule_id", rule.ID, "error", err)
	}
	if s.alerts != nil {
		if err := s.alerts.ResolveRuleAlerts(ctx, rule.TenantID, rule.ID); err != nil {
			slog.Warn("automation resolve rule alerts failed", "rule_id", rule.ID, "error", err)
		}
	}
	return finished, nil
}

func (s *Service) failFire(ctx context.Context, rule Rule, fire Fire, code, message string) (Fire, error) {
	finished, err := s.finishFire(ctx, fire, FireStatusFailed, nil, nil, code, message)
	if err != nil {
		return Fire{}, err
	}
	s.openFailureAlert(ctx, rule, finished, code, message)
	updated, err := s.repo.IncrementFailureCount(ctx, rule.TenantID, rule.ID)
	if err != nil {
		return finished, nil
	}
	if updated.ConsecutiveFailureCount >= MaxConsecutiveFailures {
		_, _ = s.disableSystem(ctx, updated, DisabledReasonConsecutiveFireFailures)
	}
	return finished, nil
}

func (s *Service) openFailureAlert(ctx context.Context, rule Rule, fire Fire, code, message string) {
	if s == nil || s.alerts == nil || s.projects == nil {
		return
	}
	info, err := s.projects.GetProject(ctx, rule.TenantID, rule.ProjectID)
	if err != nil {
		slog.Warn("automation failure alert project lookup failed", "rule_id", rule.ID, "error", err)
		return
	}
	if len(info.OwnerUserIDs) == 0 {
		return
	}
	if err := s.alerts.OpenRuleFailureAlert(ctx, RuleFailureAlert{
		TenantID:     rule.TenantID,
		RuleID:       rule.ID,
		RuleName:     rule.Name,
		ProjectID:    rule.ProjectID,
		ProjectName:  info.Name,
		OwnerUserIDs: info.OwnerUserIDs,
		ErrorCode:    code,
		ErrorMessage: message,
		FireID:       fire.ID,
	}); err != nil {
		slog.Warn("automation open failure alert failed", "rule_id", rule.ID, "error", err)
	}
}

func (s *Service) finishFire(ctx context.Context, fire Fire, status string, demandID, runID *uuid.UUID, code, message string) (Fire, error) {
	fire.Status = status
	fire.DemandID = demandID
	fire.RunID = runID
	if code != "" {
		fire.ErrorCode = &code
	} else {
		fire.ErrorCode = nil
	}
	if message != "" {
		fire.ErrorMessage = &message
	} else {
		fire.ErrorMessage = nil
	}
	return s.repo.UpdateFire(ctx, fire)
}

func (s *Service) disableSystem(ctx context.Context, rule Rule, reason string) (Rule, error) {
	updated, err := s.repo.DisableRuleSystem(ctx, rule.TenantID, rule.ID, reason)
	if err != nil {
		return Rule{}, err
	}
	if updated.TemporalScheduleID != nil && *updated.TemporalScheduleID != "" && s.schedules != nil {
		if err := s.schedules.Pause(ctx, *updated.TemporalScheduleID, reason); err != nil {
			slog.Warn("automation schedule pause failed", "schedule_id", *updated.TemporalScheduleID, "error", err)
		}
	}
	return updated, nil
}

func (s *Service) syncScheduleCreate(ctx context.Context, rule *Rule) error {
	if s.schedules == nil || rule == nil {
		return nil
	}
	scheduleID, err := s.schedules.Create(ctx, *rule)
	if err != nil {
		return err
	}
	if scheduleID == "" {
		return nil
	}
	updated, err := s.repo.SetRuleScheduleID(ctx, rule.TenantID, rule.ID, &scheduleID)
	if err != nil {
		return err
	}
	*rule = updated
	if !rule.Enabled {
		_ = s.schedules.Pause(ctx, scheduleID, "created disabled")
	}
	return nil
}

func (s *Service) syncScheduleUpdate(ctx context.Context, rule Rule) error {
	if s.schedules == nil {
		return nil
	}
	if rule.TemporalScheduleID == nil || *rule.TemporalScheduleID == "" {
		return s.syncScheduleCreate(ctx, &rule)
	}
	return s.schedules.Update(ctx, rule)
}

func validateMode(mode string) error {
	switch mode {
	case ModePlan, ModeLoop, ModeChat:
		return nil
	default:
		return fmt.Errorf("%w: coordination_mode must be plan, loop, or chat", ErrInvalidInput)
	}
}

func validateModeFields(mode string, req CreateRuleRequest) error {
	switch mode {
	case ModePlan, ModeLoop:
		if req.DemandTitleTemplate == nil || strings.TrimSpace(*req.DemandTitleTemplate) == "" {
			return fmt.Errorf("%w: demand_title_template is required for plan/loop", ErrInvalidInput)
		}
	case ModeChat:
		if req.DigitalEmployeeID == nil || *req.DigitalEmployeeID == uuid.Nil {
			return fmt.Errorf("%w: digital_employee_id is required for chat", ErrInvalidInput)
		}
		if req.ChatObjectiveTemplate == nil || strings.TrimSpace(*req.ChatObjectiveTemplate) == "" {
			return fmt.Errorf("%w: chat_objective_template is required for chat", ErrInvalidInput)
		}
	}
	return nil
}

func normalizeSchedule(kind string, cronExpr *string, intervalSeconds *int32, timezone string) (string, *string, *int32, string, error) {
	kind = strings.TrimSpace(kind)
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		timezone = DefaultTimezone
	}
	if _, err := loadTimezone(timezone); err != nil {
		return "", nil, nil, "", err
	}
	switch kind {
	case ScheduleCron:
		if cronExpr == nil || strings.TrimSpace(*cronExpr) == "" {
			return "", nil, nil, "", fmt.Errorf("%w: cron_expr is required", ErrInvalidInput)
		}
		expr := strings.TrimSpace(*cronExpr)
		return ScheduleCron, &expr, nil, timezone, nil
	case ScheduleInterval:
		if intervalSeconds == nil || *intervalSeconds < MinIntervalSeconds {
			return "", nil, nil, "", fmt.Errorf("%w: interval_seconds must be >= %d", ErrInvalidInput, MinIntervalSeconds)
		}
		seconds := *intervalSeconds
		return ScheduleInterval, nil, &seconds, timezone, nil
	default:
		return "", nil, nil, "", fmt.Errorf("%w: schedule_kind must be cron or interval", ErrInvalidInput)
	}
}

func normalizePagination(limit, offset int32) (int32, int32) {
	if limit <= 0 {
		limit = DefaultListLimit
	}
	if limit > MaxListLimit {
		limit = MaxListLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func trimPtr(v *string) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return nil
	}
	return &s
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

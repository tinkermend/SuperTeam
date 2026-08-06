package project

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/superteam/control-plane/internal/scenariotemplate"
)

// CastingEntry is one role→employee assignment for a project×playbook.
type CastingEntry struct {
	ID                  uuid.UUID
	TenantID            uuid.UUID
	ProjectID           uuid.UUID
	ScenarioTemplateKey string
	RoleKey             string
	DigitalEmployeeID   uuid.UUID
	CastByUserID        uuid.UUID
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// CastingAssignment is the write shape for one role in a replace-casting call.
type CastingAssignment struct {
	RoleKey           string
	DigitalEmployeeID uuid.UUID
}

// Casting invalidation reasons for read-path self-checks and automation alerts.
const (
	CastingReasonNotCast             = "not_cast"
	CastingReasonEmployeeUnavailable = "employee_unavailable"
	CastingReasonRoleNotHeld         = "role_not_held"
)

// CastingInvalidation is one role that is not effectively cast for a playbook.
// Used by automation fire guards and failure notifications (原选谁 / 为何失效).
type CastingInvalidation struct {
	RoleKey      string
	EmployeeID   uuid.UUID // uuid.Nil when never cast
	EmployeeName string
	Reason       string // not_cast | employee_unavailable | role_not_held
}

// AffectedCastingRow is one casting row that would be (or was) cascade-removed.
type AffectedCastingRow struct {
	ProjectID           uuid.UUID
	ProjectName         string
	ScenarioTemplateKey string
	TemplateName        string
	RoleKey             string
	DigitalEmployeeID   uuid.UUID
	EmployeeName        string
}

// EmployeeRoleImpact is the preview for removing role keys from one employee.
type EmployeeRoleImpact struct {
	AffectedCastings []AffectedCastingRow
	AffectedCount    int
}

// PutCastingRequest replaces the entire casting set for one project×template.
type PutCastingRequest struct {
	TenantID            uuid.UUID
	ProjectID           uuid.UUID
	ActorUserID         uuid.UUID
	ScenarioTemplateKey string
	Assignments         []CastingAssignment
}

// RoleCandidate is one employee eligible for a role, with capability hints.
type RoleCandidate struct {
	DigitalEmployeeID   uuid.UUID
	Name                string
	TeamID              *uuid.UUID
	TeamName            string
	RoleKeys            []string
	MatchedCapabilities []string
	MissingCapabilities []string
	CapabilityFit       string // "matched" | "partial" | "missing"
}

// PlaybookReadiness is the deepest reachable exit for one template on a project.
type PlaybookReadiness struct {
	ScenarioTemplateKey string
	TemplateName        string
	Runnable            bool
	DeepestExit         *PlaybookExitReach
	NextExitNeedsRoles  []string
	MissingRolesForAny  []string
	Exits               []PlaybookExitReach
}

// PlaybookExitReach describes whether one exit is staffable under current casting/pool.
type PlaybookExitReach struct {
	Deliverable  string
	Label        string
	Reachable    bool
	RequiredRoles []string
	MissingRoles []string
}

// RoleVocabularyActiveKeys looks up active role keys (for casting validation).
type RoleVocabularyActiveKeys interface {
	UnknownKeys(ctx context.Context, tenantID uuid.UUID, keys []string) ([]string, error)
}

// RoleVocabularyRow is one active vocabulary entry for discoverer/planner prompts.
type RoleVocabularyRow struct {
	RoleKey     string
	Title       string
	Description string
}

// RoleVocabularyActiveLister lists active role rows (optional; discoverer skips when nil).
type RoleVocabularyActiveLister interface {
	ListActiveRoleRows(ctx context.Context, tenantID uuid.UUID) ([]RoleVocabularyRow, error)
}

// CastingGapRoleOption is one vocabulary option injected into the gap discoverer.
type CastingGapRoleOption struct {
	RoleKey     string
	Title       string
	Description string
}

// CastingGapInput is the DB-free input for the semantic casting gap discoverer.
type CastingGapInput struct {
	TaskTitle          string
	ConclusionSummary  string
	DeliverableNames   []string
	ActiveRoles        []CastingGapRoleOption
	ParticipatingRoles []string
	Model              string
}

// CastingGapSuggestion is the constrained discoverer output after R1–R3.
type CastingGapSuggestion struct {
	Needed   bool
	RoleKey  string
	External bool
	Reason   string
}

// CastingGapDiscoverer is the optional LLM seam for mid-execution semantic
// casting expansion (design 2026-08-05 §3). Unwired → silent skip.
type CastingGapDiscoverer interface {
	DiscoverCastingGap(ctx context.Context, in CastingGapInput) (CastingGapSuggestion, error)
}

// ScenarioTemplateSpecSource loads a parsed scenario template spec.
type ScenarioTemplateSpecSource interface {
	GetParsedSpec(ctx context.Context, tenantID uuid.UUID, key string) (scenariotemplate.SpecV2, string, error)
}

// DigitalEmployeeRoleSource lists employees by role and employee role sets.
type DigitalEmployeeRoleSource interface {
	ListEmployeesByRoleKey(ctx context.Context, tenantID uuid.UUID, roleKey string) ([]DigitalEmployeeRoleHolder, error)
	ListEmployeeRoleKeys(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID][]string, error)
	ListEmployeeSummaries(ctx context.Context, tenantID uuid.UUID, employeeIDs []uuid.UUID) (map[uuid.UUID]DigitalEmployeeSummary, error)
}

// DigitalEmployeeRoleHolder is a minimal employee row for casting candidates.
type DigitalEmployeeRoleHolder struct {
	ID     uuid.UUID
	Name   string
	TeamID *uuid.UUID
}

// DigitalEmployeeSummary carries display fields for casting/readiness.
type DigitalEmployeeSummary struct {
	ID                  uuid.UUID
	Name                string
	TeamID              *uuid.UUID
	TeamName            string
	Status              string
	CapabilityBindings  map[string]any
	RoleKeys            []string
}

// CastingRepository persists casting rows.
type CastingRepository interface {
	ListProjectCastings(ctx context.Context, tenantID, projectID uuid.UUID, templateKey *string) ([]CastingEntry, error)
	// ReplaceProjectCasting 整套替换编制,并在**同一事务**内把未入池的被编制
	// 员工补进项目成员池(spec §4.4)。displayNames 供入池时写显示名快照。
	//
	// 入池与编制必须同事务:分两步做时,若编制写入失败而入池已提交,项目成员池
	// 里会多出一个「没有编制却可被 planner 选中派活」的员工——治理泄漏。
	ReplaceProjectCasting(ctx context.Context, tenantID, projectID, actorUserID uuid.UUID, templateKey string, assignments []CastingAssignment, displayNames map[uuid.UUID]string) ([]CastingEntry, error)
	CountCastingsForEmployee(ctx context.Context, tenantID, projectID, employeeID uuid.UUID) (int, error)
	ListCastingsForEmployeeRoles(ctx context.Context, tenantID, employeeID uuid.UUID, roleKeys []string) ([]AffectedCastingRow, error)
	DeleteCastingsForEmployeeRoles(ctx context.Context, tenantID, employeeID uuid.UUID, roleKeys []string) error
	ListCastingsForRoleKey(ctx context.Context, tenantID uuid.UUID, roleKey string) ([]AffectedCastingRow, error)
	DeleteCastingsForRoleKey(ctx context.Context, tenantID uuid.UUID, roleKey string) error
}

var (
	ErrCastingIncomplete      = fmt.Errorf("%w: casting incomplete", ErrInvalidProject)
	ErrCastingEmployeeInUse   = fmt.Errorf("%w: employee still cast; change casting first", ErrInvalidProject)
	ErrCastingUnknownRole     = fmt.Errorf("%w: unknown role key", ErrInvalidProject)
	ErrCastingDuplicateRole   = fmt.Errorf("%w: duplicate role in casting", ErrInvalidProject)
	// ErrCastingRoleNotHeld:被编制的员工不持有该剧本角色。候选列表按角色过滤
	// 此前只在前端做,API 可绕 —— 绕过去写进来的编制会让「谁能干这个角色」这个
	// 承重判断失真(可达收口、缺员拦截、扩编候选全依赖它)。
	ErrCastingRoleNotHeld = fmt.Errorf("%w: employee does not hold the cast role", ErrInvalidProject)
)

func (s *Service) SetRoleVocabulary(v RoleVocabularyActiveKeys) {
	s.roleVocabulary = v
}

func (s *Service) SetRoleVocabularyLister(l RoleVocabularyActiveLister) {
	s.roleVocabularyLister = l
}

func (s *Service) SetCastingGapDiscoverer(d CastingGapDiscoverer) {
	s.castingGapDiscoverer = d
}

func (s *Service) SetScenarioTemplateSpecSource(src ScenarioTemplateSpecSource) {
	s.scenarioTemplateSpecs = src
}

func (s *Service) SetDigitalEmployeeRoleSource(src DigitalEmployeeRoleSource) {
	s.employeeRoles = src
}

func (s *Service) SetCastingRepository(repo CastingRepository) {
	s.castingRepo = repo
}

// ListCastings returns casting rows for a project, optionally filtered by template.
func (s *Service) ListCastings(ctx context.Context, tenantID, projectID uuid.UUID, templateKey string) ([]CastingEntry, error) {
	if s.castingRepo == nil {
		return nil, fmt.Errorf("casting repository not configured")
	}
	if _, err := s.repository.GetProject(ctx, tenantID, projectID); err != nil {
		return nil, err
	}
	var key *string
	if t := strings.TrimSpace(templateKey); t != "" {
		key = &t
	}
	return s.castingRepo.ListProjectCastings(ctx, tenantID, projectID, key)
}

// PutCasting replaces casting for one template and auto-joins employees to the member pool.
func (s *Service) PutCasting(ctx context.Context, req PutCastingRequest) ([]CastingEntry, error) {
	if s.castingRepo == nil {
		return nil, fmt.Errorf("casting repository not configured")
	}
	if req.TenantID == uuid.Nil || req.ProjectID == uuid.Nil || req.ActorUserID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	templateKey := strings.TrimSpace(req.ScenarioTemplateKey)
	if templateKey == "" {
		return nil, fmt.Errorf("%w: scenario_template_key is required", ErrInvalidProject)
	}
	if _, err := s.repository.GetProject(ctx, req.TenantID, req.ProjectID); err != nil {
		return nil, err
	}

	// Dedup role keys; one role one person.
	seen := map[string]struct{}{}
	assignments := make([]CastingAssignment, 0, len(req.Assignments))
	roleKeys := make([]string, 0, len(req.Assignments))
	employeeIDs := make([]uuid.UUID, 0, len(req.Assignments))
	for _, a := range req.Assignments {
		roleKey := strings.TrimSpace(a.RoleKey)
		if roleKey == "" || a.DigitalEmployeeID == uuid.Nil {
			return nil, fmt.Errorf("%w: each assignment needs role_key and digital_employee_id", ErrInvalidProject)
		}
		if _, ok := seen[roleKey]; ok {
			return nil, fmt.Errorf("%w: %s", ErrCastingDuplicateRole, roleKey)
		}
		seen[roleKey] = struct{}{}
		assignments = append(assignments, CastingAssignment{RoleKey: roleKey, DigitalEmployeeID: a.DigitalEmployeeID})
		roleKeys = append(roleKeys, roleKey)
		employeeIDs = append(employeeIDs, a.DigitalEmployeeID)
	}

	if s.roleVocabulary != nil && len(roleKeys) > 0 {
		unknown, err := s.roleVocabulary.UnknownKeys(ctx, req.TenantID, roleKeys)
		if err != nil {
			return nil, err
		}
		if len(unknown) > 0 {
			return nil, fmt.Errorf("%w: %s", ErrCastingUnknownRole, strings.Join(unknown, ", "))
		}
	}

	// 硬校验:被编制的人必须真的持有这个角色。前端候选列表已按角色过滤,但那只是
	// 便利;编制是「谁能干这个角色」的事实源,可达收口、缺员拦截、扩编候选全从这里
	// 读。允许 API 绕过去写,等于允许这些判断静默失真。
	if s.employeeRoles != nil && len(assignments) > 0 {
		heldByEmployee, err := s.employeeRoles.ListEmployeeRoleKeys(ctx, req.TenantID, uniqueUUIDs(employeeIDs))
		if err != nil {
			return nil, err
		}
		violations := make([]string, 0, len(assignments))
		for _, a := range assignments {
			held := false
			for _, key := range heldByEmployee[a.DigitalEmployeeID] {
				if strings.EqualFold(strings.TrimSpace(key), a.RoleKey) {
					held = true
					break
				}
			}
			if !held {
				violations = append(violations, fmt.Sprintf("%s←%s", a.RoleKey, a.DigitalEmployeeID))
			}
		}
		if len(violations) > 0 {
			return nil, fmt.Errorf("%w: %s", ErrCastingRoleNotHeld, strings.Join(violations, ", "))
		}
	}

	// Resolve display names for auto-join.
	names := map[uuid.UUID]string{}
	if s.employeeRoles != nil && len(employeeIDs) > 0 {
		summaries, err := s.employeeRoles.ListEmployeeSummaries(ctx, req.TenantID, uniqueUUIDs(employeeIDs))
		if err != nil {
			return nil, err
		}
		for id, sum := range summaries {
			names[id] = sum.Name
		}
	}

	// 入池与编制在**同一事务**内完成(spec §4.4):成员池 = 编制的并集 + 人工额外加的人。
	displayNames := make(map[uuid.UUID]string, len(assignments))
	for _, a := range assignments {
		display := names[a.DigitalEmployeeID]
		if display == "" {
			display = a.DigitalEmployeeID.String()
		}
		displayNames[a.DigitalEmployeeID] = display
	}

	entries, err := s.castingRepo.ReplaceProjectCasting(ctx, req.TenantID, req.ProjectID, req.ActorUserID, templateKey, assignments, displayNames)
	if err != nil {
		return nil, err
	}

	_, _ = s.repository.AppendProjectEvent(ctx, AppendProjectEventRequest{
		TenantID:  req.TenantID,
		ProjectID: req.ProjectID,
		EventType: ProjectEventConfigChanged,
		ActorType: "human_user",
		ActorID:   req.ActorUserID.String(),
		Summary:   "剧本编制已更新",
		Payload: map[string]any{
			"event":                 "project.casting.changed",
			"scenario_template_key": templateKey,
			"role_count":            len(entries),
		},
	})
	// 重新编制是「编制失效」告警的唯一关闭者:告警无人类动词,不关就永久滞留。
	s.ResolveCastingInvalidationAlerts(ctx, req.TenantID, req.ProjectID)
	return entries, nil
}

// ListRoleCandidates returns tenant-visible employees holding a role, annotated by required capabilities.
func (s *Service) ListRoleCandidates(ctx context.Context, tenantID, projectID uuid.UUID, roleKey string, requiredCapabilities []string) ([]RoleCandidate, error) {
	if s.employeeRoles == nil {
		return nil, fmt.Errorf("employee role source not configured")
	}
	roleKey = strings.TrimSpace(roleKey)
	if roleKey == "" {
		return nil, fmt.Errorf("%w: role_key is required", ErrInvalidProject)
	}
	if _, err := s.repository.GetProject(ctx, tenantID, projectID); err != nil {
		return nil, err
	}
	if s.roleVocabulary != nil {
		unknown, err := s.roleVocabulary.UnknownKeys(ctx, tenantID, []string{roleKey})
		if err != nil {
			return nil, err
		}
		if len(unknown) > 0 {
			return nil, fmt.Errorf("%w: %s", ErrCastingUnknownRole, roleKey)
		}
	}

	holders, err := s.employeeRoles.ListEmployeesByRoleKey(ctx, tenantID, roleKey)
	if err != nil {
		return nil, err
	}
	if len(holders) == 0 {
		return []RoleCandidate{}, nil
	}
	ids := make([]uuid.UUID, 0, len(holders))
	for _, h := range holders {
		ids = append(ids, h.ID)
	}
	summaries, err := s.employeeRoles.ListEmployeeSummaries(ctx, tenantID, ids)
	if err != nil {
		return nil, err
	}
	roleKeys, err := s.employeeRoles.ListEmployeeRoleKeys(ctx, tenantID, ids)
	if err != nil {
		return nil, err
	}

	candidates := make([]RoleCandidate, 0, len(holders))
	for _, h := range holders {
		sum := summaries[h.ID]
		caps := externalCapabilitiesFromBindings(sum.CapabilityBindings)
		matched, missing := diffCapabilities(requiredCapabilities, caps)
		fit := "matched"
		if len(requiredCapabilities) == 0 {
			fit = "matched"
		} else if len(matched) == 0 {
			fit = "missing"
		} else if len(missing) > 0 {
			fit = "partial"
		}
		name := sum.Name
		if name == "" {
			name = h.Name
		}
		candidates = append(candidates, RoleCandidate{
			DigitalEmployeeID:   h.ID,
			Name:                name,
			TeamID:              firstNonNilUUID(sum.TeamID, h.TeamID),
			TeamName:            sum.TeamName,
			RoleKeys:            roleKeys[h.ID],
			MatchedCapabilities: matched,
			MissingCapabilities: missing,
			CapabilityFit:       fit,
		})
	}
	// Sort: matched first, then partial, then missing; name within group.
	rank := map[string]int{"matched": 0, "partial": 1, "missing": 2}
	sort.SliceStable(candidates, func(i, j int) bool {
		ri, rj := rank[candidates[i].CapabilityFit], rank[candidates[j].CapabilityFit]
		if ri != rj {
			return ri < rj
		}
		return candidates[i].Name < candidates[j].Name
	})
	return candidates, nil
}

// GetPlaybookReadiness computes deepest reachable exit for each active template (or one key).
func (s *Service) GetPlaybookReadiness(ctx context.Context, tenantID, projectID uuid.UUID, templateKey string) ([]PlaybookReadiness, error) {
	if s.scenarioTemplateSpecs == nil || s.castingRepo == nil {
		return nil, fmt.Errorf("playbook readiness dependencies not configured")
	}
	if _, err := s.repository.GetProject(ctx, tenantID, projectID); err != nil {
		return nil, err
	}

	// Load casting and pool role coverage.
	castings, err := s.castingRepo.ListProjectCastings(ctx, tenantID, projectID, nil)
	if err != nil {
		return nil, err
	}
	castRolesByTemplate := map[string]map[string]uuid.UUID{}
	for _, c := range castings {
		m := castRolesByTemplate[c.ScenarioTemplateKey]
		if m == nil {
			m = map[string]uuid.UUID{}
			castRolesByTemplate[c.ScenarioTemplateKey] = m
		}
		m[c.RoleKey] = c.DigitalEmployeeID
	}

	// Pool: digital employee members + their roles.
	poolRoleHolders := map[string]map[uuid.UUID]struct{}{}
	if s.employeeRoles != nil {
		members, err := s.repository.ListProjectMembers(ctx, tenantID, projectID)
		if err != nil {
			return nil, err
		}
		var empIDs []uuid.UUID
		for _, m := range members {
			if m.PrincipalType == PrincipalTypeDigitalEmployee && m.Status == "active" {
				empIDs = append(empIDs, m.PrincipalID)
			}
		}
		if len(empIDs) > 0 {
			roleMap, err := s.employeeRoles.ListEmployeeRoleKeys(ctx, tenantID, empIDs)
			if err != nil {
				return nil, err
			}
			for empID, roles := range roleMap {
				for _, rk := range roles {
					if poolRoleHolders[rk] == nil {
						poolRoleHolders[rk] = map[uuid.UUID]struct{}{}
					}
					poolRoleHolders[rk][empID] = struct{}{}
				}
			}
		}
	}

	// Also treat tenant-wide role holders as "pool" for readiness when member pool is empty
	// per §5: "编制里有人，或池内存在具备该 role 的员工".
	// Spec §5 algorithm: casting OR pool. Pool = project members with role.
	// For empty member pool and no casting, we also consider tenant employees with the role
	// for "could we staff if we cast"? Spec says:
	//   满足 = 每个 role 在编制里有人，或池内存在具备该 role 的员工
	// "池" = project member pool, not tenant-wide. So empty pool + no casting = not staffable.
	// G2 expectation with empty pool: software_delivery can only reach "交付分支" if developer
	// exists in tenant? Re-read G2 / §10.3:
	//
	// "项目 P1 成员池为空时:
	//  软件开发 | 「交付分支」（只需 developer） | 再往深缺独立的 reviewer"
	//
	// That implies readiness considers tenant employees with roles, not only project members!
	// Otherwise with empty pool nothing is reachable. Spec §5:
	//   满足 = 每个 role 在编制里有人，或池内存在具备该 role 的员工
	// But §10.3 expectation contradicts empty-pool-only reading.
	//
	// Interpretation used for G2: when computing readiness for selector before casting,
	// "池" effectively means "who can be cast" = tenant employees holding the role
	// (candidate set). Casting, once set, takes precedence and is the operational truth.
	// After casting, readiness uses casting ∪ pool.
	//
	// Implementation: role is satisfied if casting has it OR any tenant employee holds it
	// (pre-cast feasibility). Post-cast, casting is authoritative for assigned roles;
	// unassigned roles still fall back to tenant holders so "next needs" stays informative.

	tenantRoleHolders := map[string]bool{}
	if s.employeeRoles != nil {
		// We'll query per role as needed via ListEmployeesByRoleKey during evaluation.
		_ = tenantRoleHolders
	}

	var templateKeys []string
	if k := strings.TrimSpace(templateKey); k != "" {
		templateKeys = []string{k}
	} else if s.scenarioTemplates != nil {
		// List via resolver is not available; readiness without filter requires
		// caller to pass keys or we use a list from scenarioTemplates package.
		// Fallback: only keys that appear in castings is incomplete.
		// Injected listTemplates below.
		if s.playbookTemplateLister != nil {
			keys, err := s.playbookTemplateLister.ListActiveTemplateKeys(ctx, tenantID)
			if err != nil {
				return nil, err
			}
			templateKeys = keys
		}
	}
	if len(templateKeys) == 0 && s.playbookTemplateLister != nil {
		keys, err := s.playbookTemplateLister.ListActiveTemplateKeys(ctx, tenantID)
		if err != nil {
			return nil, err
		}
		templateKeys = keys
	}

	results := make([]PlaybookReadiness, 0, len(templateKeys))
	for _, key := range templateKeys {
		spec, name, err := s.scenarioTemplateSpecs.GetParsedSpec(ctx, tenantID, key)
		if err != nil {
			// Skip missing/disabled templates quietly for list path.
			continue
		}
		results = append(results, computePlaybookReadiness(ctx, s, tenantID, key, name, spec, castRolesByTemplate[key], poolRoleHolders))
	}
	return results, nil
}

// PlaybookTemplateLister lists active scenario template keys for readiness.
type PlaybookTemplateLister interface {
	ListActiveTemplateKeys(ctx context.Context, tenantID uuid.UUID) ([]string, error)
}

func (s *Service) SetPlaybookTemplateLister(l PlaybookTemplateLister) {
	s.playbookTemplateLister = l
}

// ValidatePlaybookCastingComplete returns structured casting gaps for a project×template.
// Empty means casting covers every role referenced by the skeleton (all exits) with
// employees that still hold the role and are active/ready.
// Used by automation rule save (G7) and fire-time guards (G8).
//
// Read path self-checks holdings even if write-side cascade failed or rows were
// planted out-of-band (direct DB) — readiness/automation must not trust stale rows.
func (s *Service) ValidatePlaybookCastingComplete(ctx context.Context, tenantID, projectID uuid.UUID, templateKey string) (missing []CastingInvalidation, err error) {
	if s.castingRepo == nil || s.scenarioTemplateSpecs == nil {
		return nil, fmt.Errorf("casting dependencies not configured")
	}
	templateKey = strings.TrimSpace(templateKey)
	if templateKey == "" {
		return nil, nil
	}
	if _, err := s.repository.GetProject(ctx, tenantID, projectID); err != nil {
		return nil, err
	}
	spec, _, err := s.scenarioTemplateSpecs.GetParsedSpec(ctx, tenantID, templateKey)
	if err != nil {
		return nil, err
	}
	castings, err := s.castingRepo.ListProjectCastings(ctx, tenantID, projectID, &templateKey)
	if err != nil {
		return nil, err
	}
	cast := map[string]uuid.UUID{}
	for _, c := range castings {
		cast[c.RoleKey] = c.DigitalEmployeeID
	}
	// All skeleton roles (deepest full coverage) — automation needs full cast for the template.
	roles := distinctRolesFromSteps(spec.Skeleton)
	if len(roles) == 0 {
		for _, r := range spec.Roles {
			if k := strings.TrimSpace(r.Key); k != "" {
				roles = append(roles, k)
			}
		}
	}
	return evaluateCastingInvalidations(ctx, s, tenantID, roles, cast)
}

// evaluateCastingInvalidations checks cast rows for hold + availability.
func evaluateCastingInvalidations(
	ctx context.Context,
	s *Service,
	tenantID uuid.UUID,
	roles []string,
	cast map[string]uuid.UUID,
) ([]CastingInvalidation, error) {
	var miss []CastingInvalidation
	var needSummary []uuid.UUID
	for _, role := range roles {
		if empID, ok := cast[role]; ok && empID != uuid.Nil {
			needSummary = append(needSummary, empID)
		}
	}
	needSummary = uniqueUUIDs(needSummary)
	var sums map[uuid.UUID]DigitalEmployeeSummary
	if s != nil && s.employeeRoles != nil && len(needSummary) > 0 {
		var err error
		sums, err = s.employeeRoles.ListEmployeeSummaries(ctx, tenantID, needSummary)
		if err != nil {
			return nil, err
		}
	}
	for _, role := range roles {
		empID, ok := cast[role]
		if !ok || empID == uuid.Nil {
			miss = append(miss, CastingInvalidation{RoleKey: role, Reason: CastingReasonNotCast})
			continue
		}
		if s == nil || s.employeeRoles == nil {
			// Without role source, keep cast row as valid (test stubs / degraded).
			continue
		}
		sum, has := sums[empID]
		if !has {
			miss = append(miss, CastingInvalidation{
				RoleKey:    role,
				EmployeeID: empID,
				Reason:     CastingReasonEmployeeUnavailable,
			})
			continue
		}
		if sum.Status != "active" && sum.Status != "ready" {
			miss = append(miss, CastingInvalidation{
				RoleKey:      role,
				EmployeeID:   empID,
				EmployeeName: sum.Name,
				Reason:       CastingReasonEmployeeUnavailable,
			})
			continue
		}
		if !employeeHoldsRole(sum.RoleKeys, role) {
			miss = append(miss, CastingInvalidation{
				RoleKey:      role,
				EmployeeID:   empID,
				EmployeeName: sum.Name,
				Reason:       CastingReasonRoleNotHeld,
			})
			continue
		}
	}
	return miss, nil
}

// FormatCastingInvalidations builds a human-readable Chinese summary for alerts.
func FormatCastingInvalidations(items []CastingInvalidation) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		switch item.Reason {
		case CastingReasonNotCast:
			parts = append(parts, fmt.Sprintf("缺角色 %s（从未编制）", item.RoleKey))
		case CastingReasonEmployeeUnavailable:
			name := strings.TrimSpace(item.EmployeeName)
			if name == "" {
				name = item.EmployeeID.String()
			}
			parts = append(parts, fmt.Sprintf("角色 %s 原编制 %s，员工不可用", item.RoleKey, name))
		case CastingReasonRoleNotHeld:
			name := strings.TrimSpace(item.EmployeeName)
			if name == "" {
				name = item.EmployeeID.String()
			}
			parts = append(parts, fmt.Sprintf("角色 %s 原编制 %s，已不再持有该角色", item.RoleKey, name))
		default:
			parts = append(parts, fmt.Sprintf("角色 %s 编制失效（%s）", item.RoleKey, item.Reason))
		}
	}
	return strings.Join(parts, "；")
}

// CastingInvalidationRoleKeys extracts bare role keys (for callers that only need keys).
func CastingInvalidationRoleKeys(items []CastingInvalidation) []string {
	out := make([]string, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		k := strings.TrimSpace(item.RoleKey)
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}

func employeeHoldsRole(keys []string, role string) bool {
	role = strings.TrimSpace(role)
	for _, k := range keys {
		if strings.TrimSpace(k) == role {
			return true
		}
	}
	return false
}

func computePlaybookReadiness(
	ctx context.Context,
	s *Service,
	tenantID uuid.UUID,
	templateKey, templateName string,
	spec scenariotemplate.SpecV2,
	casting map[string]uuid.UUID,
	poolRoleHolders map[string]map[uuid.UUID]struct{},
) PlaybookReadiness {
	_ = ctx
	_ = tenantID
	out := PlaybookReadiness{
		ScenarioTemplateKey: templateKey,
		TemplateName:        templateName,
		Exits:               make([]PlaybookExitReach, 0, len(spec.Exits)),
	}
	if len(spec.Exits) == 0 {
		// No exits → treat as runnable if no skeleton roles, or all skeleton roles staffable.
		roles := distinctRolesFromSteps(spec.Skeleton)
		missing := missingRoles(roles, casting, poolRoleHolders, s, tenantID, ctx)
		out.Runnable = len(missing) == 0
		out.MissingRolesForAny = missing
		return out
	}

	var deepest *PlaybookExitReach
	var nextNeeds []string
	for _, exit := range spec.Exits {
		steps, err := scenariotemplate.PruneSkeletonForExit(spec, exit.Deliverable)
		if err != nil {
			reach := PlaybookExitReach{
				Deliverable: exit.Deliverable,
				Label:       exit.Label,
				Reachable:   false,
			}
			out.Exits = append(out.Exits, reach)
			continue
		}
		roles := distinctRolesFromSteps(steps)
		// role_independence: if constraint applies at this exit, need ≥2 distinct people for those roles when both cast.
		missing := missingRoles(roles, casting, poolRoleHolders, s, tenantID, ctx)
		// Check role_independence structural feasibility when casting collapses roles onto one person.
		if len(missing) == 0 {
			if !roleIndependenceSatisfied(spec, exit.Deliverable, casting, poolRoleHolders, roles) {
				// Treat as missing independent counterpart: report roles that require independence.
				for _, c := range spec.Constraints {
					if c.Kind != "role_independence" {
						continue
					}
					if !scenariotemplate.ExitCondMet(spec, c.When, exit.Deliverable) {
						continue
					}
					// If all independent roles map to same cast employee, not satisfied.
					missing = append(missing, c.Roles...)
				}
				missing = uniqueStrings(missing)
			}
		}
		reach := PlaybookExitReach{
			Deliverable:   exit.Deliverable,
			Label:         exit.Label,
			Reachable:     len(missing) == 0,
			RequiredRoles: roles,
			MissingRoles:  missing,
		}
		out.Exits = append(out.Exits, reach)
		if reach.Reachable {
			cp := reach
			deepest = &cp
			nextNeeds = nil
		} else if deepest != nil && nextNeeds == nil {
			nextNeeds = missing
		} else if deepest == nil && len(out.MissingRolesForAny) == 0 {
			out.MissingRolesForAny = missing
		}
	}
	out.DeepestExit = deepest
	out.NextExitNeedsRoles = nextNeeds
	out.Runnable = deepest != nil
	if deepest == nil && len(out.MissingRolesForAny) == 0 && len(out.Exits) > 0 {
		out.MissingRolesForAny = out.Exits[0].MissingRoles
	}
	return out
}

func roleIndependenceSatisfied(
	spec scenariotemplate.SpecV2,
	exit string,
	casting map[string]uuid.UUID,
	pool map[string]map[uuid.UUID]struct{},
	neededRoles []string,
) bool {
	needed := map[string]bool{}
	for _, r := range neededRoles {
		needed[r] = true
	}
	for _, c := range spec.Constraints {
		if c.Kind != "role_independence" {
			continue
		}
		if !scenariotemplate.ExitCondMet(spec, c.When, exit) {
			continue
		}
		// Collect cast employees for constraint roles that are in the pruned set.
		people := map[uuid.UUID]struct{}{}
		for _, rk := range c.Roles {
			if !needed[rk] {
				continue
			}
			if emp, ok := casting[rk]; ok {
				people[emp] = struct{}{}
				continue
			}
			// Uncast: if pool has ≥1 holder, count as potentially distinct unknown — allow.
			if holders := pool[rk]; len(holders) > 0 {
				for id := range holders {
					people[id] = struct{}{}
				}
			} else {
				// No one — already failed missingRoles; return true to avoid double-count.
				return true
			}
		}
		if len(people) < 2 {
			return false
		}
	}
	return true
}

func missingRoles(
	roles []string,
	casting map[string]uuid.UUID,
	pool map[string]map[uuid.UUID]struct{},
	s *Service,
	tenantID uuid.UUID,
	ctx context.Context,
) []string {
	// Self-check cast rows: a stale casting (role no longer held / employee down)
	// does not satisfy readiness. Pool/tenant holders remain the fallback only
	// when there is no usable cast row.
	var castEmpIDs []uuid.UUID
	for _, role := range roles {
		if empID, ok := casting[role]; ok && empID != uuid.Nil {
			castEmpIDs = append(castEmpIDs, empID)
		}
	}
	castEmpIDs = uniqueUUIDs(castEmpIDs)
	var sums map[uuid.UUID]DigitalEmployeeSummary
	if s != nil && s.employeeRoles != nil && len(castEmpIDs) > 0 {
		if got, err := s.employeeRoles.ListEmployeeSummaries(ctx, tenantID, castEmpIDs); err == nil {
			sums = got
		}
	}

	// 词表已停用的角色永远不可满足:`PutCasting` 走 UnknownKeys 会 400,员工身上
	// 的旧绑定还在,若放任 pool 回退命中,选择器会说「这档可达」而实际再也编制不上。
	inactive := map[string]struct{}{}
	if s != nil && s.roleVocabulary != nil && len(roles) > 0 {
		if unknown, err := s.roleVocabulary.UnknownKeys(ctx, tenantID, roles); err == nil {
			for _, key := range unknown {
				inactive[strings.TrimSpace(key)] = struct{}{}
			}
		}
	}

	var missing []string
	for _, role := range roles {
		if _, dead := inactive[strings.TrimSpace(role)]; dead {
			missing = append(missing, role)
			continue
		}
		if empID, ok := casting[role]; ok && empID != uuid.Nil {
			if castingRowSatisfies(role, empID, sums, s) {
				continue
			}
			// Cast row present but invalid — do not fall through to pool; the
			// selector must surface the gap so humans fix casting explicitly.
			missing = append(missing, role)
			continue
		}
		if holders := pool[role]; len(holders) > 0 {
			continue
		}
		// Tenant-wide fallback for pre-cast feasibility (G2 empty-pool expectations).
		if s != nil && s.employeeRoles != nil {
			holders, err := s.employeeRoles.ListEmployeesByRoleKey(ctx, tenantID, role)
			if err == nil && len(holders) > 0 {
				continue
			}
		}
		missing = append(missing, role)
	}
	return missing
}

func castingRowSatisfies(role string, empID uuid.UUID, sums map[uuid.UUID]DigitalEmployeeSummary, s *Service) bool {
	if s == nil || s.employeeRoles == nil {
		// No role source → cannot self-check; trust the row (tests / degraded).
		return true
	}
	if sums == nil {
		return false
	}
	sum, ok := sums[empID]
	if !ok {
		return false
	}
	if sum.Status != "active" && sum.Status != "ready" {
		return false
	}
	return employeeHoldsRole(sum.RoleKeys, role)
}

func distinctRolesFromSteps(steps []scenariotemplate.SpecSkeletonStep) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, step := range steps {
		r := strings.TrimSpace(step.Role)
		if r == "" {
			continue
		}
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		out = append(out, r)
	}
	sort.Strings(out)
	return out
}

func externalCapabilitiesFromBindings(bindings map[string]any) map[string]bool {
	out := map[string]bool{}
	if bindings == nil {
		return out
	}
	raw, ok := bindings["external_capabilities"]
	if !ok {
		return out
	}
	switch v := raw.(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				if t := strings.TrimSpace(s); t != "" {
					out[t] = true
				}
			}
		}
	case []string:
		for _, s := range v {
			if t := strings.TrimSpace(s); t != "" {
				out[t] = true
			}
		}
	}
	return out
}

func diffCapabilities(required []string, held map[string]bool) (matched, missing []string) {
	for _, r := range required {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		if held[r] {
			matched = append(matched, r)
		} else {
			missing = append(missing, r)
		}
	}
	return matched, missing
}

func uniqueUUIDs(ids []uuid.UUID) []uuid.UUID {
	seen := map[uuid.UUID]struct{}{}
	out := make([]uuid.UUID, 0, len(ids))
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func firstNonNilUUID(a, b *uuid.UUID) *uuid.UUID {
	if a != nil && *a != uuid.Nil {
		return a
	}
	if b != nil && *b != uuid.Nil {
		return b
	}
	return nil
}

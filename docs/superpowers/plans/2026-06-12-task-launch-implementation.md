# Task Launch Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first-version “任务发起” flow: users submit a project demand with reviewer preference, land on a launch detail page, and track real coordination facts.

**Architecture:** Keep `ProjectDemand` as the launch fact source. Persist reviewer preference inside demand metadata, add a demand-level launch detail read model, then add a focused web feature for launch creation/detail without expanding the existing project page.

**Tech Stack:** Go + chi/net/http + sqlc + OpenAPI for Control Plane; React + TanStack Router + TanStack Query + shadcn/ui + Vitest Browser for Web.

---

## File Structure

Backend domain and API:

- Modify `apps/control-plane/internal/project/types.go`: add reviewer preference fields to demand request/domain and add launch detail read-model types.
- Modify `apps/control-plane/internal/project/service.go`: add reviewer resolution, reviewer validation, demand metadata enrichment, and launch detail aggregation.
- Modify `apps/control-plane/internal/project/repository.go`: keep existing repository surface; no new SQL is required for V1 because reviewer preference persists in `project_demands.source_refs`.
- Modify `apps/control-plane/internal/project/pg_repository.go`: parse reviewer preference from `source_refs`.
- Modify `apps/control-plane/internal/project/handler.go`: decode reviewer fields, expose launch detail handler, and add response shapes.
- Modify `apps/control-plane/internal/api/server.go`: register `GET /api/v1/project-demands/{demandId}/launch-detail`.
- Modify `contracts/control-plane/openapi.yaml`: document reviewer fields and launch detail response.

Backend tests:

- Modify `apps/control-plane/internal/project/service_test.go`: cover reviewer defaults, invalid reviewer rejection, metadata persistence, and launch detail aggregation.
- Modify `apps/control-plane/internal/project/handler_test.go`: cover JSON request decoding and launch detail response shape.
- Modify `apps/control-plane/internal/api/project_routes_test.go`: cover the new route wiring.

Frontend API and routes:

- Modify `apps/web/src/lib/api/projects.ts`: add reviewer fields and `getProjectDemandLaunchDetail`.
- Modify `apps/web/src/lib/api/projects.test.ts`: cover request body and encoded demand detail URL.
- Modify `apps/web/src/components/layout/data/sidebar-data.ts`: replace “任务中心” with “任务发起”.
- Modify `apps/web/src/components/layout/sidebar-data.test.ts`: assert the new primary navigation.
- Create `apps/web/src/routes/_authenticated/task-launches/index.tsx`.
- Create `apps/web/src/routes/_authenticated/task-launches/$demandId.tsx`.
- Create `apps/web/src/features/task-launches/index.tsx`.
- Create `apps/web/src/features/task-launches/index.test.tsx`.
- Create `apps/web/src/features/task-launches/components/task-launch-shell.tsx`.
- Create `apps/web/src/features/task-launches/components/task-launch-form.tsx`.
- Create `apps/web/src/features/task-launches/components/task-launch-detail.tsx`.

Generated files and docs:

- Regenerate `apps/web/src/routeTree.gen.ts` by running the web test/typecheck path after adding routes.
- Regenerate Control Plane OpenAPI Go output with `pnpm generate:control-plane`.
- Modify `CHANGELOG.md` with an Asia/Shanghai timestamp from `TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'`.

## Task 1: Backend Reviewer Preference

**Files:**
- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Test: `apps/control-plane/internal/project/service_test.go`

- [ ] **Step 1: Write failing service tests for reviewer defaults**

Add tests near existing `SubmitDemand` tests in `apps/control-plane/internal/project/service_test.go`:

```go
func TestSubmitDemandPersistsDefaultReviewerPreference(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	reviewerID := uuid.New()
	repo := newMemoryRepository(tenantID, projectID, ownerID)
	repo.members = append(repo.members, ProjectMember{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
		PrincipalType: PrincipalTypeHumanUser, PrincipalID: reviewerID,
		ProjectRole: ProjectRoleReviewer, Status: "active",
	})
	service := mustProjectService(t, repo)

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "审查 PR", Content: "统计 PR 并分派审查",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}

	if demand.ReviewerPreference == nil {
		t.Fatalf("expected reviewer preference on demand: %#v", demand)
	}
	if demand.ReviewerPreference.ReviewerUserID != reviewerID {
		t.Fatalf("expected reviewer %s, got %#v", reviewerID, demand.ReviewerPreference)
	}
	if demand.ReviewerPreference.SelectionReason != ReviewerSelectionProjectReviewerDefault {
		t.Fatalf("unexpected reviewer reason: %#v", demand.ReviewerPreference)
	}
	if demand.SourceRefs["reviewer_user_id"] != reviewerID.String() {
		t.Fatalf("expected reviewer persisted in source refs: %#v", demand.SourceRefs)
	}
}

func TestSubmitDemandFallsBackToHumanOwnerWhenNoReviewer(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo := newMemoryRepository(tenantID, projectID, ownerID)
	service := mustProjectService(t, repo)

	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "补充证据",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}
	if demand.ReviewerPreference == nil || demand.ReviewerPreference.ReviewerUserID != ownerID {
		t.Fatalf("expected owner fallback preference: %#v", demand.ReviewerPreference)
	}
	if demand.ReviewerPreference.SelectionReason != ReviewerSelectionProjectHumanOwnerFallback {
		t.Fatalf("expected owner fallback reason, got %#v", demand.ReviewerPreference)
	}
}
```

- [ ] **Step 2: Write failing service tests for invalid reviewers**

Add:

```go
func TestSubmitDemandRejectsDigitalEmployeeReviewer(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	digitalEmployeeID := uuid.New()
	repo := newMemoryRepository(tenantID, projectID, ownerID)
	repo.members = append(repo.members, ProjectMember{
		ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
		PrincipalType: PrincipalTypeDigitalEmployee, PrincipalID: digitalEmployeeID,
		ProjectRole: ProjectRoleExecutor, Status: "active",
	})
	service := mustProjectService(t, repo)

	_, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "需要审核", ReviewerUserID: &digitalEmployeeID,
	})
	if !errors.Is(err, ErrInvalidProjectMember) {
		t.Fatalf("expected invalid project member, got %v", err)
	}
}

func TestSubmitDemandRequiresExplicitReviewerWhenMultipleReviewers(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo := newMemoryRepository(tenantID, projectID, ownerID)
	for range 2 {
		repo.members = append(repo.members, ProjectMember{
			ID: uuid.New(), TenantID: tenantID, ProjectID: projectID,
			PrincipalType: PrincipalTypeHumanUser, PrincipalID: uuid.New(),
			ProjectRole: ProjectRoleReviewer, Status: "active",
		})
	}
	service := mustProjectService(t, repo)

	_, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID,
		Title: "多审核人项目",
	})
	if !errors.Is(err, ErrInvalidProjectMember) {
		t.Fatalf("expected reviewer selection error, got %v", err)
	}
}
```

- [ ] **Step 3: Run focused tests and verify RED**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestSubmitDemand.*Reviewer' -count=1
```

Expected: FAIL because `ReviewerPreference`, `ReviewerUserID`, and reviewer selection constants do not exist.

- [ ] **Step 4: Add reviewer types to domain**

In `apps/control-plane/internal/project/types.go`, add:

```go
type ReviewerSelectionReason string

const (
	ReviewerSelectionProjectReviewerDefault     ReviewerSelectionReason = "project_reviewer_default"
	ReviewerSelectionProjectHumanOwnerFallback ReviewerSelectionReason = "project_human_owner_fallback"
	ReviewerSelectionUserSelected              ReviewerSelectionReason = "user_selected"
)

type ReviewerPreference struct {
	ReviewerUserID   uuid.UUID
	SelectionReason  ReviewerSelectionReason
	DisplayName      *string
	ProjectRole      ProjectRole
	ResolvedFromRule bool
}
```

Extend `ProjectDemand`:

```go
ReviewerPreference *ReviewerPreference
```

Extend `SubmitProjectDemandRequest`:

```go
ReviewerUserID          *uuid.UUID
ReviewerSelectionReason ReviewerSelectionReason
```

- [ ] **Step 5: Implement reviewer resolution in service**

In `apps/control-plane/internal/project/service.go`, add helpers near `SubmitDemand`:

```go
func (s *Service) resolveDemandReviewer(ctx context.Context, req SubmitProjectDemandRequest, project Project) (*ReviewerPreference, map[string]any, error) {
	members, err := s.repository.ListProjectMembers(ctx, req.TenantID, req.ProjectID)
	if err != nil {
		return nil, nil, err
	}
	selected, reason, resolvedFromRule, err := selectReviewer(req.ReviewerUserID, project, members)
	if err != nil {
		return nil, nil, err
	}
	preference := &ReviewerPreference{
		ReviewerUserID:   selected.PrincipalID,
		SelectionReason:  reason,
		DisplayName:      selected.DisplayNameSnapshot,
		ProjectRole:      selected.ProjectRole,
		ResolvedFromRule: resolvedFromRule,
	}
	return preference, map[string]any{
		"reviewer_user_id":          preference.ReviewerUserID.String(),
		"reviewer_selection_reason": string(preference.SelectionReason),
		"reviewer_project_role":     string(preference.ProjectRole),
		"reviewer_resolved_from_rule": preference.ResolvedFromRule,
	}, nil
}

func selectReviewer(explicit *uuid.UUID, project Project, members []ProjectMember) (ProjectMember, ReviewerSelectionReason, bool, error) {
	if explicit != nil {
		for _, member := range members {
			if member.PrincipalType == PrincipalTypeHumanUser && member.PrincipalID == *explicit && member.Status == "active" {
				return member, ReviewerSelectionUserSelected, false, nil
			}
		}
		return ProjectMember{}, "", false, ErrInvalidProjectMember
	}
	reviewers := make([]ProjectMember, 0, len(members))
	for _, member := range members {
		if member.PrincipalType == PrincipalTypeHumanUser && member.ProjectRole == ProjectRoleReviewer && member.Status == "active" {
			reviewers = append(reviewers, member)
		}
	}
	if len(reviewers) == 1 {
		return reviewers[0], ReviewerSelectionProjectReviewerDefault, true, nil
	}
	if len(reviewers) > 1 {
		return ProjectMember{}, "", false, ErrInvalidProjectMember
	}
	for _, member := range members {
		if member.PrincipalType == PrincipalTypeHumanUser && member.PrincipalID == project.HumanOwnerUserID && member.Status == "active" {
			return member, ReviewerSelectionProjectHumanOwnerFallback, true, nil
		}
	}
	return ProjectMember{}, "", false, ErrInvalidProjectMember
}

func mergeReviewerSourceRefs(sourceRefs map[string]any, reviewer map[string]any) map[string]any {
	merged := map[string]any{}
	for key, value := range sourceRefs {
		merged[key] = value
	}
	for key, value := range reviewer {
		merged[key] = value
	}
	return merged
}
```

In `SubmitDemand`, after archived-project check and before appending the event:

```go
preference, reviewerSourceRefs, err := s.resolveDemandReviewer(ctx, req, project)
if err != nil {
	return nil, err
}
req.SourceRefs = mergeReviewerSourceRefs(req.SourceRefs, reviewerSourceRefs)
```

After `CreateProjectDemand`:

```go
demand.ReviewerPreference = preference
```

Add reviewer fields to event payload:

```go
Payload: map[string]any{
	"title": req.Title,
	"reviewer_user_id": preference.ReviewerUserID.String(),
	"reviewer_selection_reason": string(preference.SelectionReason),
},
```

- [ ] **Step 6: Parse persisted reviewer preference in repository**

In `apps/control-plane/internal/project/pg_repository.go`, update `demandFromRecord` to populate `ReviewerPreference` from `sourceRefs`. Inside the existing returned `ProjectDemand` literal, set the new field:

```go
preference := reviewerPreferenceFromSourceRefs(sourceRefs)
// keep the existing ProjectDemand fields unchanged and add:
ReviewerPreference: preference,
```

Add helper:

```go
func reviewerPreferenceFromSourceRefs(sourceRefs map[string]any) *ReviewerPreference {
	rawReviewer, ok := sourceRefs["reviewer_user_id"].(string)
	if !ok || rawReviewer == "" {
		return nil
	}
	reviewerID, err := uuid.Parse(rawReviewer)
	if err != nil {
		return nil
	}
	reason := ReviewerSelectionReason("")
	if rawReason, ok := sourceRefs["reviewer_selection_reason"].(string); ok {
		reason = ReviewerSelectionReason(rawReason)
	}
	role := ProjectRole("")
	if rawRole, ok := sourceRefs["reviewer_project_role"].(string); ok {
		role = ProjectRole(rawRole)
	}
	resolved, _ := sourceRefs["reviewer_resolved_from_rule"].(bool)
	return &ReviewerPreference{
		ReviewerUserID:   reviewerID,
		SelectionReason:  reason,
		ProjectRole:      role,
		ResolvedFromRule: resolved,
	}
}
```

Update the memory repository `CreateProjectDemand` in `apps/control-plane/internal/project/service_test.go` to copy `req.ReviewerUserID` or the source refs into `ReviewerPreference`.

- [ ] **Step 7: Run focused tests and commit**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestSubmitDemand.*Reviewer' -count=1
```

Expected: PASS.

Commit:

```bash
git add apps/control-plane/internal/project/types.go apps/control-plane/internal/project/service.go apps/control-plane/internal/project/pg_repository.go apps/control-plane/internal/project/service_test.go
git commit -m "feat: add project demand reviewer preference"
```

## Task 2: Backend Launch Detail Read Model

**Files:**
- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/service.go`
- Modify: `apps/control-plane/internal/project/handler.go`
- Modify: `apps/control-plane/internal/api/server.go`
- Test: `apps/control-plane/internal/project/service_test.go`
- Test: `apps/control-plane/internal/project/handler_test.go`
- Test: `apps/control-plane/internal/api/project_routes_test.go`

- [ ] **Step 1: Write failing service test for launch detail aggregation**

Add to `apps/control-plane/internal/project/service_test.go`:

```go
func TestGetDemandLaunchDetailAggregatesDemandFacts(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	ownerID := uuid.New()
	repo := newMemoryRepository(tenantID, projectID, ownerID)
	service := mustProjectService(t, repo)
	demand, err := service.SubmitDemand(context.Background(), SubmitProjectDemandRequest{
		TenantID: tenantID, ProjectID: projectID, SubmittedByUserID: ownerID, Title: "审查 PR",
	})
	if err != nil {
		t.Fatalf("submit demand: %v", err)
	}
	job := CoordinationJob{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, TriggerEventID: demand.CreatedEventID, JobType: "demand_route", Status: "running"}
	repo.coordinationJobs = append(repo.coordinationJobs, job)
	task := ProjectTask{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, DemandID: &demand.ID, Title: "审查 PR", Status: "pending"}
	repo.tasks = append(repo.tasks, task)
	repo.routeDecisions = append(repo.routeDecisions, RouteDecision{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, CoordinationJobID: job.ID, DemandID: &demand.ID, Reason: "按能力分派"})
	repo.decisionRequests = append(repo.decisionRequests, DecisionRequest{ID: uuid.New(), TenantID: tenantID, ProjectID: projectID, CoordinationJobID: &job.ID, TargetUserID: ownerID, DecisionType: "route_review", TitleSnapshot: "确认路由", StatusSnapshot: "pending"})

	detail, err := service.GetDemandLaunchDetail(context.Background(), tenantID, demand.ID)
	if err != nil {
		t.Fatalf("launch detail: %v", err)
	}
	if detail.Demand.ID != demand.ID || detail.Project.ID != projectID {
		t.Fatalf("unexpected demand/project: %#v", detail)
	}
	if len(detail.CoordinationJobs) != 1 || len(detail.RouteDecisions) != 1 || len(detail.ProjectTasks) != 1 || len(detail.DecisionRequests) != 1 {
		t.Fatalf("expected related facts, got %#v", detail)
	}
}
```

- [ ] **Step 2: Add service interface types**

In `apps/control-plane/internal/project/types.go`, add:

```go
type DemandLaunchDetail struct {
	Demand          ProjectDemand
	Project         Project
	Reviewer        *ReviewerPreference
	CoordinationJobs []CoordinationJob
	RouteDecisions  []RouteDecision
	ProjectTasks    []ProjectTask
	DecisionRequests []DecisionRequest
	RecentEvents    []ProjectEvent
}
```

Add to `HandlerService` in `apps/control-plane/internal/project/handler.go`:

```go
GetDemandLaunchDetail(ctx context.Context, tenantID, demandID uuid.UUID) (*DemandLaunchDetail, error)
```

- [ ] **Step 3: Implement service aggregation**

In `apps/control-plane/internal/project/service.go`, add:

```go
func (s *Service) GetDemandLaunchDetail(ctx context.Context, tenantID, demandID uuid.UUID) (*DemandLaunchDetail, error) {
	if tenantID == uuid.Nil || demandID == uuid.Nil {
		return nil, ErrInvalidProject
	}
	demand, err := s.repository.GetProjectDemand(ctx, tenantID, demandID)
	if err != nil {
		return nil, err
	}
	project, err := s.repository.GetProject(ctx, tenantID, demand.ProjectID)
	if err != nil {
		return nil, err
	}
	jobs, err := s.repository.ListCoordinationJobs(ctx, tenantID, demand.ProjectID, 100, 0)
	if err != nil {
		return nil, err
	}
	routes, err := s.repository.ListRouteDecisions(ctx, tenantID, demand.ProjectID, 100, 0)
	if err != nil {
		return nil, err
	}
	tasks, err := s.repository.ListProjectTasks(ctx, tenantID, demand.ProjectID, nil, 100, 0)
	if err != nil {
		return nil, err
	}
	decisions, err := s.repository.ListDecisionRequests(ctx, tenantID, demand.ProjectID, 100, 0)
	if err != nil {
		return nil, err
	}
	events, err := s.repository.ListProjectEvents(ctx, tenantID, demand.ProjectID, 50, 0)
	if err != nil {
		return nil, err
	}
	filteredJobs := filterJobsForDemand(jobs, demand)
	filteredRoutes := filterRoutesForDemand(routes, demand.ID)
	filteredTasks := filterTasksForDemand(tasks, demand.ID)
	filteredDecisions := filterDecisionsForDemand(decisions, filteredJobs, filteredTasks)
	filteredEvents := filterEventsForDemand(events, demand, filteredTasks, filteredDecisions)
	return &DemandLaunchDetail{
		Demand: demand, Project: project, Reviewer: demand.ReviewerPreference,
		CoordinationJobs: filteredJobs, RouteDecisions: filteredRoutes,
		ProjectTasks: filteredTasks, DecisionRequests: filteredDecisions, RecentEvents: filteredEvents,
	}, nil
}
```

Add helpers with ID-set filtering:

```go
func filterJobsForDemand(jobs []CoordinationJob, demand ProjectDemand) []CoordinationJob {
	filtered := []CoordinationJob{}
	for _, job := range jobs {
		if demand.CreatedEventID != nil && job.TriggerEventID != nil && *job.TriggerEventID == *demand.CreatedEventID {
			filtered = append(filtered, job)
		}
	}
	return filtered
}

func filterRoutesForDemand(routes []RouteDecision, demandID uuid.UUID) []RouteDecision {
	filtered := []RouteDecision{}
	for _, route := range routes {
		if route.DemandID != nil && *route.DemandID == demandID {
			filtered = append(filtered, route)
		}
	}
	return filtered
}

func filterTasksForDemand(tasks []ProjectTask, demandID uuid.UUID) []ProjectTask {
	filtered := []ProjectTask{}
	for _, task := range tasks {
		if task.DemandID != nil && *task.DemandID == demandID {
			filtered = append(filtered, task)
		}
	}
	return filtered
}

func filterDecisionsForDemand(decisions []DecisionRequest, jobs []CoordinationJob, tasks []ProjectTask) []DecisionRequest {
	jobIDs := map[uuid.UUID]struct{}{}
	for _, job := range jobs {
		jobIDs[job.ID] = struct{}{}
	}
	taskIDs := map[uuid.UUID]struct{}{}
	for _, task := range tasks {
		taskIDs[task.ID] = struct{}{}
	}
	filtered := []DecisionRequest{}
	for _, decision := range decisions {
		if decision.CoordinationJobID != nil {
			if _, ok := jobIDs[*decision.CoordinationJobID]; ok {
				filtered = append(filtered, decision)
				continue
			}
		}
		if decision.ProjectTaskID != nil {
			if _, ok := taskIDs[*decision.ProjectTaskID]; ok {
				filtered = append(filtered, decision)
			}
		}
	}
	return filtered
}

func filterEventsForDemand(events []ProjectEvent, demand ProjectDemand, tasks []ProjectTask, decisions []DecisionRequest) []ProjectEvent {
	taskIDs := map[string]struct{}{}
	for _, task := range tasks {
		taskIDs[task.ID.String()] = struct{}{}
	}
	decisionIDs := map[string]struct{}{}
	for _, decision := range decisions {
		decisionIDs[decision.ID.String()] = struct{}{}
	}
	filtered := []ProjectEvent{}
	for _, event := range events {
		if demand.CreatedEventID != nil && event.ID == *demand.CreatedEventID {
			filtered = append(filtered, event)
			continue
		}
		if event.ResourceID != nil {
			if _, ok := taskIDs[*event.ResourceID]; ok {
				filtered = append(filtered, event)
				continue
			}
			if _, ok := decisionIDs[*event.ResourceID]; ok {
				filtered = append(filtered, event)
				continue
			}
		}
		if rawDemandID, ok := event.Payload["demand_id"].(string); ok && rawDemandID == demand.ID.String() {
			filtered = append(filtered, event)
		}
	}
	return filtered
}
```

- [ ] **Step 4: Write handler and route tests**

In `apps/control-plane/internal/project/handler_test.go`, add a test that calls:

```go
req := httptest.NewRequest(http.MethodGet, "/api/v1/project-demands/"+demandID.String()+"/launch-detail", nil)
```

Inject `middleware.TenantIDKey` and `middleware.UserIDKey`, call `handler.GetDemandLaunchDetail`, and assert:

```go
if body["demand"].(map[string]any)["id"] != demandID.String() {
	t.Fatalf("expected demand id in detail response: %#v", body)
}
if len(body["project_tasks"].([]any)) != 1 {
	t.Fatalf("expected project tasks in launch detail: %#v", body)
}
```

In `apps/control-plane/internal/api/project_routes_test.go`, add a route-level test for:

```text
GET /api/v1/project-demands/{demandId}/launch-detail
```

Expected: `routeProjectService.GetDemandLaunchDetail` receives the demand ID and the response status is `200`.

- [ ] **Step 5: Implement handler response**

In `apps/control-plane/internal/project/handler.go`, add:

```go
type demandLaunchDetailResponse struct {
	Demand           projectDemandResponse       `json:"demand"`
	Project          projectResponse             `json:"project"`
	Reviewer         *reviewerPreferenceResponse `json:"reviewer,omitempty"`
	CoordinationJobs []coordinationJobResponse   `json:"coordination_jobs"`
	RouteDecisions   []routeDecisionResponse     `json:"route_decisions"`
	ProjectTasks     []projectTaskResponse       `json:"project_tasks"`
	DecisionRequests []decisionRequestResponse   `json:"decision_requests"`
	RecentEvents     []projectEventResponse      `json:"recent_events"`
}

type reviewerPreferenceResponse struct {
	ReviewerUserID   string                  `json:"reviewer_user_id"`
	SelectionReason  ReviewerSelectionReason `json:"selection_reason"`
	DisplayName      *string                 `json:"display_name,omitempty"`
	ProjectRole      ProjectRole            `json:"project_role"`
	ResolvedFromRule bool                   `json:"resolved_from_rule"`
}
```

Add handler:

```go
func (h *HTTPHandler) GetDemandLaunchDetail(w http.ResponseWriter, r *http.Request) {
	tenantID, _, ok := consoleIdentity(w, r)
	if !ok {
		return
	}
	demandID, err := uuid.Parse(chi.URLParam(r, "demandId"))
	if err != nil {
		writeHandlerError(w, ErrInvalidProject)
		return
	}
	service, ok := h.serviceFromRequest(w)
	if !ok {
		return
	}
	detail, err := service.GetDemandLaunchDetail(r.Context(), tenantID, demandID)
	if err != nil {
		writeHandlerError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, demandLaunchDetailResponseFromDomain(*detail))
}
```

Register in `apps/control-plane/internal/api/server.go` inside project handler group:

```go
r.Get("/project-demands/{demandId}/launch-detail", s.projectHandler.GetDemandLaunchDetail)
```

- [ ] **Step 6: Run focused tests and commit**

Run:

```bash
go test ./apps/control-plane/internal/project -run 'TestGetDemandLaunchDetail|Test.*LaunchDetail' -count=1
go test ./apps/control-plane/internal/api -run 'TestProject.*LaunchDetail|Test.*ProjectDemand' -count=1
```

Expected: PASS.

Commit:

```bash
git add apps/control-plane/internal/project/types.go apps/control-plane/internal/project/service.go apps/control-plane/internal/project/handler.go apps/control-plane/internal/api/server.go apps/control-plane/internal/project/service_test.go apps/control-plane/internal/project/handler_test.go apps/control-plane/internal/api/project_routes_test.go
git commit -m "feat: add project demand launch detail"
```

## Task 3: Contracts and Web API Client

**Files:**
- Modify: `contracts/control-plane/openapi.yaml`
- Modify: `apps/control-plane/internal/project/handler.go`
- Modify: `apps/control-plane/internal/project/handler_test.go`
- Modify: `apps/web/src/lib/api/projects.ts`
- Modify: `apps/web/src/lib/api/projects.test.ts`
- Generated: Control Plane generated files from `pnpm generate:control-plane`

- [ ] **Step 1: Add failing web API tests**

In `apps/web/src/lib/api/projects.test.ts`, add:

```ts
it("submits demand with reviewer preference", async () => {
  const demand = {
    id: "55555555-5555-4555-8555-555555555555",
    tenant_id: "22222222-2222-4222-8222-222222222222",
    project_id: "11111111-1111-4111-8111-111111111111",
    submitted_by_user_id: "33333333-3333-4333-8333-333333333333",
    title: "审查 PR",
    content: "统计并审查 PR",
    source_type: "manual",
    source_refs: {},
    attachments: [],
    status: "planning_pending",
    reviewer: {
      reviewer_user_id: "33333333-3333-4333-8333-333333333333",
      selection_reason: "user_selected",
      project_role: "reviewer",
      resolved_from_rule: false,
    },
  }
  const fetcher = vi.fn(async () => new Response(JSON.stringify(demand), {
    headers: { "content-type": "application/json" },
    status: 201,
  }))
  const input = {
    title: "审查 PR",
    content: "统计并审查 PR",
    reviewer_user_id: "33333333-3333-4333-8333-333333333333",
    reviewer_selection_reason: "user_selected" as const,
  }

  await expect(submitProjectDemand({ baseUrl: "http://control-plane.local", fetcher }, "project-1", input)).resolves.toEqual(demand)

  expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/projects/project-1/demands", expect.objectContaining({
    body: JSON.stringify(input),
    method: "POST",
  }))
})

it("gets project demand launch detail with encoded demand id", async () => {
  const detail = {
    demand: baseDemand,
    project,
    reviewer: null,
    coordination_jobs: [],
    route_decisions: [],
    project_tasks: [],
    decision_requests: [],
    recent_events: [],
  }
  const fetcher = vi.fn(async () => new Response(JSON.stringify(detail), {
    headers: { "content-type": "application/json" },
    status: 200,
  }))

  await expect(getProjectDemandLaunchDetail({ baseUrl: "http://control-plane.local", fetcher }, "demand 1/primary")).resolves.toEqual(detail)

  expect(fetcher).toHaveBeenCalledWith("http://control-plane.local/api/v1/project-demands/demand%201%2Fprimary/launch-detail", {
    credentials: "include",
    headers: { accept: "application/json" },
    method: "GET",
  })
})
```

- [ ] **Step 2: Run web API tests and verify RED**

Run:

```bash
pnpm --filter @superteam/web test -- src/lib/api/projects.test.ts
```

Expected: FAIL because `getProjectDemandLaunchDetail` and reviewer input fields do not exist.

- [ ] **Step 3: Update OpenAPI**

In `contracts/control-plane/openapi.yaml`:

Add path:

```yaml
  /api/v1/project-demands/{demandId}/launch-detail:
    get:
      operationId: getProjectDemandLaunchDetail
      summary: Get project demand launch detail
      parameters:
        - name: demandId
          in: path
          required: true
          schema:
            type: string
            format: uuid
      responses:
        "200":
          description: Project demand launch detail
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ProjectDemandLaunchDetail"
```

Extend `SubmitProjectDemandRequest` with:

```yaml
        reviewer_user_id:
          type: string
          format: uuid
        reviewer_selection_reason:
          type: string
          enum:
            - project_reviewer_default
            - project_human_owner_fallback
            - user_selected
```

Add schemas:

```yaml
    ReviewerPreference:
      type: object
      required:
        - reviewer_user_id
        - selection_reason
        - project_role
        - resolved_from_rule
      properties:
        reviewer_user_id:
          type: string
          format: uuid
        selection_reason:
          type: string
        display_name:
          type: string
        project_role:
          $ref: "#/components/schemas/ProjectRole"
        resolved_from_rule:
          type: boolean
    ProjectDemandLaunchDetail:
      type: object
      required:
        - demand
        - project
        - coordination_jobs
        - route_decisions
        - project_tasks
        - decision_requests
        - recent_events
      properties:
        demand:
          $ref: "#/components/schemas/ProjectDemand"
        project:
          $ref: "#/components/schemas/Project"
        reviewer:
          $ref: "#/components/schemas/ReviewerPreference"
        coordination_jobs:
          type: array
          items:
            $ref: "#/components/schemas/ProjectCoordinationJob"
        route_decisions:
          type: array
          items:
            $ref: "#/components/schemas/ProjectRouteDecision"
        project_tasks:
          type: array
          items:
            $ref: "#/components/schemas/ProjectTask"
        decision_requests:
          type: array
          items:
            $ref: "#/components/schemas/ProjectDecisionRequest"
        recent_events:
          type: array
          items:
            $ref: "#/components/schemas/ProjectEvent"
```

- [ ] **Step 4: Update web API client**

Before the web client work, update the handwritten Control Plane handler surface:

- Extend `submitDemandBody` with `reviewer_user_id` and `reviewer_selection_reason`.
- Pass those fields into `SubmitProjectDemandRequest` in `HTTPHandler.SubmitDemand`.
- Ensure `demandResponseFromDomain` exposes `reviewer` as a stable JSON key with `null` when no reviewer preference exists.
- Add handler tests that post reviewer fields and assert the service receives them, and that the demand response includes the reviewer shape.

In `apps/web/src/lib/api/projects.ts`, add types:

```ts
export type ReviewerSelectionReason =
  | "project_reviewer_default"
  | "project_human_owner_fallback"
  | "user_selected";

export type ReviewerPreference = {
  reviewer_user_id: string;
  selection_reason: ReviewerSelectionReason;
  display_name?: string;
  project_role: ProjectRole;
  resolved_from_rule: boolean;
};
```

Extend `ProjectDemand`:

```ts
reviewer?: ReviewerPreference | null;
```

Extend `SubmitProjectDemandInput`:

```ts
reviewer_user_id?: string;
reviewer_selection_reason?: ReviewerSelectionReason;
```

Add:

```ts
export type ProjectDemandLaunchDetail = {
  demand: ProjectDemand;
  project: Project;
  reviewer?: ReviewerPreference | null;
  coordination_jobs: ProjectCoordinationJob[];
  route_decisions: ProjectRouteDecision[];
  project_tasks: ProjectTask[];
  decision_requests: ProjectDecisionRequest[];
  recent_events: ProjectEvent[];
};

export function getProjectDemandLaunchDetail(
  options: ApiClientOptions,
  demandId: string,
): Promise<ProjectDemandLaunchDetail> {
  return getJson<ProjectDemandLaunchDetail>(
    options,
    `/api/v1/project-demands/${encodeURIComponent(demandId)}/launch-detail`,
    "project demand launch detail",
  );
}
```

- [ ] **Step 5: Generate and verify contracts/client tests**

Run:

```bash
pnpm generate:control-plane
pnpm verify:contracts
pnpm --filter @superteam/web test -- src/lib/api/projects.test.ts
```

Expected: all PASS.

Commit:

```bash
git add contracts/control-plane/openapi.yaml apps/control-plane/internal/api apps/web/src/lib/api/projects.ts apps/web/src/lib/api/projects.test.ts
git commit -m "feat: expose task launch contract"
```

## Task 4: Navigation and Routes

**Files:**
- Modify: `apps/web/src/components/layout/data/sidebar-data.ts`
- Modify: `apps/web/src/components/layout/sidebar-data.test.ts`
- Create: `apps/web/src/routes/_authenticated/task-launches/index.tsx`
- Create: `apps/web/src/routes/_authenticated/task-launches/$demandId.tsx`
- Create: `apps/web/src/features/task-launches/index.tsx`
- Create: `apps/web/src/features/task-launches/components/task-launch-shell.tsx`
- Test: `apps/web/src/components/layout/sidebar-data.test.ts`

- [ ] **Step 1: Update sidebar test first**

In `apps/web/src/components/layout/sidebar-data.test.ts`, change expected workspace items to:

```ts
expect(workspaceItems?.map((item) => item.title)).toEqual([
  '工作台',
  '任务发起',
  '项目管理',
  '数字员工',
  '技能管理',
  '团队管理',
])
```

Change `expectedIconTones`:

```ts
['任务发起', 'task'],
```

Remove the `任务中心` entry.

- [ ] **Step 2: Run sidebar test and verify RED**

Run:

```bash
pnpm --filter @superteam/web test -- src/components/layout/sidebar-data.test.ts
```

Expected: FAIL because `sidebarData` still contains “任务中心”.

- [ ] **Step 3: Update sidebar data**

In `apps/web/src/components/layout/data/sidebar-data.ts`, replace `ClipboardList` task item with `SendHorizontal`:

```ts
import {
  Blocks,
  Bot,
  CircleDollarSign,
  FileClock,
  FolderKanban,
  GitBranch,
  KeyRound,
  LayoutDashboard,
  MessagesSquare,
  Puzzle,
  SendHorizontal,
  Server,
  ShieldCheck,
  Users,
  UsersRound,
} from "lucide-react";
```

Workspace item:

```ts
{
  title: "任务发起",
  url: "/task-launches",
  icon: SendHorizontal,
  iconTone: "task",
},
{
  title: "项目管理",
  url: "/projects",
  icon: FolderKanban,
  iconTone: "workflow",
},
```

Do not delete `apps/web/src/routes/_authenticated/tasks/index.tsx` in this task; it is no longer primary navigation but can remain as a technical route until a later cleanup.

- [ ] **Step 4: Add minimal routes and shell**

Create `apps/web/src/features/task-launches/components/task-launch-shell.tsx`:

```tsx
import type { ReactNode } from "react";
import { SendHorizontal } from "lucide-react";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import { SemanticIconTile } from "@/components/superteam";

export function TaskLaunchShell({
  children,
  description,
  title,
}: {
  children: ReactNode;
  description?: string;
  title: string;
}) {
  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        <div className="flex flex-col gap-5">
          <div className="flex min-w-0 items-center gap-3">
            <SemanticIconTile tone="primary" size="lg">
              <SendHorizontal />
            </SemanticIconTile>
            <div className="min-w-0">
              <h1 className="text-2xl font-bold tracking-normal">{title}</h1>
              {description ? <p className="text-sm text-muted-foreground">{description}</p> : null}
            </div>
          </div>
          {children}
        </div>
      </Main>
    </>
  );
}
```

Create `apps/web/src/features/task-launches/index.tsx` with placeholder components that will be replaced in later tasks:

```tsx
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { TaskLaunchShell } from "./components/task-launch-shell";

export function TaskLaunchPage() {
  return (
    <TaskLaunchShell title="任务发起" description="提交需求到项目，由项目协调线程编排后续任务">
      <div>任务发起表单加载中</div>
    </TaskLaunchShell>
  );
}

export function TaskLaunchDetailPage({ demandId }: { demandId: string }) {
  void resolveControlPlaneUrl();
  return (
    <TaskLaunchShell title="发起详情" description="查看一次任务发起触发的协调事实">
      <div>发起详情 {demandId}</div>
    </TaskLaunchShell>
  );
}
```

Create routes:

```tsx
// apps/web/src/routes/_authenticated/task-launches/index.tsx
import { createFileRoute } from "@tanstack/react-router";
import { TaskLaunchPage } from "@/features/task-launches";

export const Route = createFileRoute("/_authenticated/task-launches/")({
  component: TaskLaunchPage,
});
```

```tsx
// apps/web/src/routes/_authenticated/task-launches/$demandId.tsx
import { createFileRoute } from "@tanstack/react-router";
import { TaskLaunchDetailPage } from "@/features/task-launches";

export const Route = createFileRoute("/_authenticated/task-launches/$demandId")({
  component: TaskLaunchDetailRoute,
});

function TaskLaunchDetailRoute() {
  const { demandId } = Route.useParams();
  return <TaskLaunchDetailPage demandId={demandId} />;
}
```

- [ ] **Step 5: Run route/sidebar verification and commit**

Run:

```bash
pnpm --filter @superteam/web test -- src/components/layout/sidebar-data.test.ts
pnpm --filter @superteam/web typecheck
```

Expected: PASS and route tree generated.

Commit:

```bash
git add apps/web/src/components/layout/data/sidebar-data.ts apps/web/src/components/layout/sidebar-data.test.ts apps/web/src/routes/_authenticated/task-launches apps/web/src/features/task-launches apps/web/src/routeTree.gen.ts
git commit -m "feat: add task launch navigation"
```

## Task 5: Task Launch Create Page

**Files:**
- Modify: `apps/web/src/features/task-launches/index.tsx`
- Create: `apps/web/src/features/task-launches/components/task-launch-form.tsx`
- Test: `apps/web/src/features/task-launches/index.test.tsx`

- [ ] **Step 1: Write failing browser tests for form behavior**

Create `apps/web/src/features/task-launches/index.test.tsx` with header mocks following `apps/web/src/features/projects/index.test.tsx`. Add tests:

```tsx
it("defaults reviewer to the only project reviewer and submits demand", async () => {
  const fetcher = createTaskLaunchFetcher();
  const screen = await renderWithQueryClient(<TaskLaunchView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />);

  await userEvent.type(screen.getByLabelText("需求描述"), "审查这个开源项目的 PR，并按数量分配数字员工");

  await expect.element(screen.getByText("王审核 · reviewer")).toBeInTheDocument();
  await userEvent.click(screen.getByRole("button", { name: "发起任务" }));

  expect(fetcher).toHaveBeenCalledWith(
    "http://control-plane.local/api/v1/projects/project-1/demands",
    expect.objectContaining({
      body: JSON.stringify({
        title: "审查这个开源项目的 PR，并按数量分配数字员工",
        content: "审查这个开源项目的 PR，并按数量分配数字员工",
        source_type: "manual",
        source_refs: {},
        attachments: [],
        reviewer_user_id: "reviewer-1",
        reviewer_selection_reason: "project_reviewer_default",
      }),
    }),
  );
});

it("requires a reviewer when the project has multiple reviewers", async () => {
  const fetcher = createTaskLaunchFetcher({ multipleReviewers: true });
  const screen = await renderWithQueryClient(<TaskLaunchView apiBaseUrl="http://control-plane.local" fetcher={fetcher} />);

  await userEvent.type(screen.getByLabelText("需求描述"), "需要两位审核人项目确认");
  await userEvent.click(screen.getByRole("button", { name: "发起任务" }));

  await expect.element(screen.getByText("请选择审核人")).toBeInTheDocument();
});
```

The test fetcher should answer:

- `GET /api/v1/projects?limit=50&offset=0`
- `GET /api/v1/projects/project-1/members`
- `POST /api/v1/projects/project-1/demands`

- [ ] **Step 2: Run form tests and verify RED**

Run:

```bash
pnpm --filter @superteam/web test -- src/features/task-launches/index.test.tsx
```

Expected: FAIL because `TaskLaunchView` and form components do not exist.

- [ ] **Step 3: Implement reviewer default helper**

In `apps/web/src/features/task-launches/components/task-launch-form.tsx`, add:

```tsx
import { useMemo, useState } from "react";
import { ChevronDown, SendHorizontal } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { LiquidCard } from "@/components/superteam";
import type { Project, ProjectDemandSourceType, ProjectMember, ReviewerSelectionReason, SubmitProjectDemandInput } from "@/lib/api/projects";

export function resolveDefaultReviewer(project: Project | undefined, members: ProjectMember[]) {
  if (!project) return undefined;
  const humanMembers = members.filter((member) => member.principal_type === "human_user" && member.status === "active");
  const reviewers = humanMembers.filter((member) => member.project_role === "reviewer");
  if (reviewers.length === 1) {
    return { member: reviewers[0], reason: "project_reviewer_default" as ReviewerSelectionReason, requiresChoice: false };
  }
  if (reviewers.length > 1) {
    return { member: undefined, reason: "user_selected" as ReviewerSelectionReason, requiresChoice: true };
  }
  const owner = humanMembers.find((member) => member.principal_id === project.human_owner_user_id);
  return owner ? { member: owner, reason: "project_human_owner_fallback" as ReviewerSelectionReason, requiresChoice: false } : undefined;
}

function deriveTitle(content: string) {
  return content.trim().split(/\n+/)[0]?.slice(0, 80) ?? "";
}
```

- [ ] **Step 4: Implement `TaskLaunchForm`**

In the same file, export:

```tsx
export function TaskLaunchForm({
  isSubmitting,
  members,
  onSubmit,
  onProjectChange,
  projects,
  selectedProjectId,
}: {
  isSubmitting?: boolean;
  members: ProjectMember[];
  onProjectChange: (projectId: string) => void;
  onSubmit: (projectId: string, input: SubmitProjectDemandInput) => void;
  projects: Project[];
  selectedProjectId?: string;
}) {
  const activeProjects = projects.filter((project) => project.status !== "archived");
  const [content, setContent] = useState("");
  const [title, setTitle] = useState("");
  const [reviewerId, setReviewerId] = useState("");
  const [error, setError] = useState("");
  const projectId = selectedProjectId || activeProjects[0]?.id || "";
  const project = activeProjects.find((item) => item.id === projectId);
  const reviewerDefault = useMemo(() => resolveDefaultReviewer(project, members), [project, members]);
  const selectedReviewerId = reviewerId || reviewerDefault?.member?.principal_id || "";
  const selectedReason = reviewerId ? "user_selected" : reviewerDefault?.reason;

  function submit() {
    const resolvedTitle = title.trim() || deriveTitle(content);
    if (!content.trim()) {
      setError("需求描述不能为空");
      return;
    }
    if (!resolvedTitle) {
      setError("标题不能为空");
      return;
    }
    if (!projectId) {
      setError("请选择项目");
      return;
    }
    if (!selectedReviewerId || !selectedReason) {
      setError("请选择审核人");
      return;
    }
    setError("");
    onSubmit(projectId, {
      title: resolvedTitle,
      content: content.trim(),
      source_type: "manual" as ProjectDemandSourceType,
      source_refs: {},
      attachments: [],
      reviewer_user_id: selectedReviewerId,
      reviewer_selection_reason: selectedReason,
    });
  }

  return (
    <LiquidCard className="mx-auto w-full max-w-3xl rounded-xl p-5">
      <div className="grid gap-4">
        <Label className="grid gap-2">
          <span>需求描述</span>
          <Textarea aria-label="需求描述" value={content} onChange={(event) => setContent(event.target.value)} placeholder="描述你希望项目协调线程处理的需求" />
        </Label>
        <Label className="grid gap-2">
          <span>标题</span>
          <Input aria-label="标题" value={title} onChange={(event) => setTitle(event.target.value)} placeholder={deriveTitle(content) || "自动使用需求描述首行"} />
        </Label>
        <Label className="grid gap-2">
          <span>项目</span>
          <Select value={projectId} onValueChange={(value) => { onProjectChange(value); setReviewerId(""); }}>
            <SelectTrigger aria-label="项目"><SelectValue placeholder="选择项目" /></SelectTrigger>
            <SelectContent>{activeProjects.map((item) => <SelectItem key={item.id} value={item.id}>{item.name}</SelectItem>)}</SelectContent>
          </Select>
        </Label>
        <details className="rounded-lg border p-3">
          <summary className="flex cursor-pointer items-center gap-2 text-sm font-medium"><ChevronDown className="size-4" />高级选项</summary>
          <Label className="mt-3 grid gap-2">
            <span>审核人</span>
            <Select value={selectedReviewerId} onValueChange={setReviewerId}>
              <SelectTrigger aria-label="审核人"><SelectValue placeholder="选择审核人" /></SelectTrigger>
              <SelectContent>
                {members.filter((member) => member.principal_type === "human_user" && member.status === "active").map((member) => (
                  <SelectItem key={member.id} value={member.principal_id}>{member.display_name_snapshot || member.principal_id} · {member.project_role}</SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Label>
        </details>
        {error ? <p className="text-sm text-destructive">{error}</p> : null}
        <div className="flex justify-end">
          <Button type="button" disabled={isSubmitting} onClick={submit}><SendHorizontal data-icon="inline-start" />发起任务</Button>
        </div>
      </div>
    </LiquidCard>
  );
}
```

- [ ] **Step 5: Wire queries and mutation in `TaskLaunchView`**

In `apps/web/src/features/task-launches/index.tsx`, replace placeholder with:

```tsx
import { keepPreviousData, useMutation, useQuery } from "@tanstack/react-query";
import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { listProjectMembers, listProjects, submitProjectDemand, type SubmitProjectDemandInput } from "@/lib/api/projects";
import type { ApiClientOptions } from "@/lib/api/client";
import { TaskLaunchForm } from "./components/task-launch-form";
import { TaskLaunchShell } from "./components/task-launch-shell";

export function TaskLaunchPage() {
  return <TaskLaunchView apiBaseUrl={resolveControlPlaneUrl()} />;
}

export function TaskLaunchView({ apiBaseUrl, fetcher }: { apiBaseUrl: string; fetcher?: typeof fetch }) {
  const navigate = useNavigate();
  const apiOptions = useMemo<ApiClientOptions>(() => ({ baseUrl: apiBaseUrl, fetcher }), [apiBaseUrl, fetcher]);
  const [selectedProjectId, setSelectedProjectId] = useState("");
  const projectsQuery = useQuery({
    queryKey: ["task-launch-projects"],
    queryFn: () => listProjects(apiOptions, { limit: 50, offset: 0 }),
    placeholderData: keepPreviousData,
  });
  const firstActiveProjectId = projectsQuery.data?.find((project) => project.status !== "archived")?.id ?? "";
  useEffect(() => {
    if (!selectedProjectId && firstActiveProjectId) {
      setSelectedProjectId(firstActiveProjectId);
    }
  }, [firstActiveProjectId, selectedProjectId]);
  const membersQuery = useQuery({
    enabled: Boolean(selectedProjectId),
    queryKey: ["task-launch-project-members", selectedProjectId],
    queryFn: () => listProjectMembers(apiOptions, selectedProjectId),
    placeholderData: keepPreviousData,
  });
  const submitMutation = useMutation({
    mutationFn: ({ projectId, input }: { projectId: string; input: SubmitProjectDemandInput }) => submitProjectDemand(apiOptions, projectId, input),
    onSuccess: (demand) => navigate({ to: "/task-launches/$demandId", params: { demandId: demand.id } }),
  });

  return (
    <TaskLaunchShell title="任务发起" description="提交需求到项目，由项目协调线程编排后续任务">
      <TaskLaunchForm
        isSubmitting={submitMutation.isPending}
        members={membersQuery.data ?? []}
        onProjectChange={setSelectedProjectId}
        projects={projectsQuery.data ?? []}
        selectedProjectId={selectedProjectId}
        onSubmit={(projectId, input) => submitMutation.mutate({ projectId, input })}
      />
    </TaskLaunchShell>
  );
}
```

- [ ] **Step 6: Run tests and commit**

Run:

```bash
pnpm --filter @superteam/web test -- src/features/task-launches/index.test.tsx
pnpm --filter @superteam/web typecheck
```

Expected: PASS.

Commit:

```bash
git add apps/web/src/features/task-launches apps/web/src/routes/_authenticated/task-launches apps/web/src/routeTree.gen.ts
git commit -m "feat: add task launch form"
```

## Task 6: Task Launch Detail Page

**Files:**
- Modify: `apps/web/src/features/task-launches/index.tsx`
- Create: `apps/web/src/features/task-launches/components/task-launch-detail.tsx`
- Test: `apps/web/src/features/task-launches/index.test.tsx`

- [ ] **Step 1: Add failing detail test**

In `apps/web/src/features/task-launches/index.test.tsx`, add:

```tsx
it("renders launch detail coordination facts", async () => {
  const fetcher = createTaskLaunchFetcher({ launchDetail: true });
  const screen = await renderWithQueryClient(<TaskLaunchDetailView apiBaseUrl="http://control-plane.local" demandId="demand-1" fetcher={fetcher} />);

  await expect.element(screen.getByText("审查 PR")).toBeInTheDocument();
  await expect.element(screen.getByText("协调 Job")).toBeInTheDocument();
  await expect.element(screen.getByText("按能力分派")).toBeInTheDocument();
  await expect.element(screen.getByText("整理审查清单")).toBeInTheDocument();
  await expect.element(screen.getByText("确认路由")).toBeInTheDocument();
});

it("shows waiting state when coordination facts are empty", async () => {
  const fetcher = createTaskLaunchFetcher({ launchDetail: true, emptyFacts: true });
  const screen = await renderWithQueryClient(<TaskLaunchDetailView apiBaseUrl="http://control-plane.local" demandId="demand-1" fetcher={fetcher} />);

  await expect.element(screen.getByText("等待项目协调线程处理")).toBeInTheDocument();
});
```

- [ ] **Step 2: Run detail tests and verify RED**

Run:

```bash
pnpm --filter @superteam/web test -- src/features/task-launches/index.test.tsx
```

Expected: FAIL because `TaskLaunchDetailView` and detail component do not render the API data yet.

- [ ] **Step 3: Implement detail component**

Create `apps/web/src/features/task-launches/components/task-launch-detail.tsx`:

```tsx
import { Link } from "@tanstack/react-router";
import { CheckCircle2, FolderKanban, GitBranch, ShieldCheck, Workflow } from "lucide-react";
import { Button } from "@/components/ui/button";
import { LiquidCard, StatusBadge } from "@/components/superteam";
import type { ProjectDemandLaunchDetail } from "@/lib/api/projects";

export function TaskLaunchDetail({ detail }: { detail: ProjectDemandLaunchDetail }) {
  const hasFacts =
    detail.coordination_jobs.length > 0 ||
    detail.route_decisions.length > 0 ||
    detail.project_tasks.length > 0 ||
    detail.decision_requests.length > 0;
  return (
    <div className="grid items-start gap-4 xl:grid-cols-[360px_minmax(0,1fr)]">
      <LiquidCard className="rounded-xl p-4">
        <div className="grid gap-3">
          <div>
            <h2 className="text-lg font-semibold">{detail.demand.title}</h2>
            <p className="mt-1 text-sm text-muted-foreground">{detail.demand.content || "暂无需求正文"}</p>
          </div>
          <StatusBadge tone="info">{detail.demand.status}</StatusBadge>
          <div className="text-sm text-muted-foreground">项目：{detail.project.name}</div>
          <div className="text-sm text-muted-foreground">审核人：{detail.reviewer?.display_name || detail.reviewer?.reviewer_user_id || "未解析"}</div>
          <Button asChild variant="outline"><Link to="/projects/$projectId" params={{ projectId: detail.project.id }}><FolderKanban data-icon="inline-start" />进入项目</Link></Button>
        </div>
      </LiquidCard>
      <div className="grid gap-4">
        {!hasFacts ? <LiquidCard className="rounded-xl p-6 text-sm text-muted-foreground">等待项目协调线程处理</LiquidCard> : null}
        <FactList title="协调 Job" icon={<Workflow />} items={detail.coordination_jobs.map((job) => ({ id: job.id, title: job.job_type, subtitle: job.workflow_id, status: job.status }))} />
        <FactList title="路由决策" icon={<GitBranch />} items={detail.route_decisions.map((decision) => ({ id: decision.id, title: decision.reason, subtitle: decision.selected_digital_employee_ids.join(", "), status: decision.requires_human_review ? "需要人工确认" : "可分发" }))} />
        <FactList title="项目任务" icon={<CheckCircle2 />} items={detail.project_tasks.map((task) => ({ id: task.id, title: task.title, subtitle: task.summary || "暂无摘要", status: task.status }))} />
        <FactList title="人类决策请求" icon={<ShieldCheck />} items={detail.decision_requests.map((decision) => ({ id: decision.id, title: decision.title_snapshot, subtitle: decision.target_user_id, status: decision.status_snapshot }))} />
      </div>
    </div>
  );
}

function FactList({ icon, items, title }: { icon: React.ReactNode; items: Array<{ id: string; title: string; subtitle: string; status: string }>; title: string }) {
  if (items.length === 0) return null;
  return (
    <LiquidCard className="rounded-xl">
      <div className="flex items-center gap-2 border-b p-4 text-sm font-semibold">{icon}{title}</div>
      <div className="divide-y">
        {items.map((item) => (
          <div className="flex items-start justify-between gap-3 p-4" key={item.id}>
            <div className="min-w-0">
              <p className="truncate text-sm font-medium">{item.title}</p>
              <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">{item.subtitle}</p>
            </div>
            <StatusBadge tone="neutral">{item.status}</StatusBadge>
          </div>
        ))}
      </div>
    </LiquidCard>
  );
}
```

- [ ] **Step 4: Wire detail query**

In `apps/web/src/features/task-launches/index.tsx`, add:

```tsx
import { getProjectDemandLaunchDetail } from "@/lib/api/projects";
import { TaskLaunchDetail } from "./components/task-launch-detail";

export function TaskLaunchDetailPage({ demandId }: { demandId: string }) {
  return <TaskLaunchDetailView apiBaseUrl={resolveControlPlaneUrl()} demandId={demandId} />;
}

export function TaskLaunchDetailView({ apiBaseUrl, demandId, fetcher }: { apiBaseUrl: string; demandId: string; fetcher?: typeof fetch }) {
  const apiOptions: ApiClientOptions = { baseUrl: apiBaseUrl, fetcher };
  const detailQuery = useQuery({
    queryKey: ["task-launch-detail", demandId],
    queryFn: () => getProjectDemandLaunchDetail(apiOptions, demandId),
    placeholderData: keepPreviousData,
  });
  return (
    <TaskLaunchShell title="发起详情" description="查看一次任务发起触发的协调事实">
      {detailQuery.data ? <TaskLaunchDetail detail={detailQuery.data} /> : <div className="text-sm text-muted-foreground">正在加载发起详情</div>}
      {detailQuery.isError ? <div className="text-sm text-destructive">发起详情加载失败</div> : null}
    </TaskLaunchShell>
  );
}
```

- [ ] **Step 5: Run tests and commit**

Run:

```bash
pnpm --filter @superteam/web test -- src/features/task-launches/index.test.tsx
pnpm --filter @superteam/web typecheck
```

Expected: PASS.

Commit:

```bash
git add apps/web/src/features/task-launches apps/web/src/routes/_authenticated/task-launches apps/web/src/routeTree.gen.ts
git commit -m "feat: add task launch detail"
```

## Task 7: Changelog and Full Verification

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: Add changelog timestamp**

Run:

```bash
TZ=Asia/Shanghai date '+%Y-%m-%d %H:%M'
```

Use the exact command output in `CHANGELOG.md`. Add an entry:

```md
## YYYY-MM-DD HH:mm

- 新增任务发起一级入口设计落地：移除任务中心主导航，支持向项目提交需求、选择审核人偏好，并通过发起详情页追踪真实协调事实。
```

- [ ] **Step 2: Run backend verification**

Run:

```bash
go test ./apps/control-plane/...
```

Expected: PASS.

- [ ] **Step 3: Run contract verification**

Run:

```bash
pnpm verify:contracts
```

Expected: PASS.

- [ ] **Step 4: Run web verification**

Run:

```bash
pnpm --filter @superteam/web test
pnpm --filter @superteam/web typecheck
pnpm --filter @superteam/web build
```

Expected: PASS. If Vite reports a large chunk warning only, record it as informational.

- [ ] **Step 5: Browser smoke for navigation and pages**

Start the web app:

```bash
pnpm dev:web
```

Open `http://127.0.0.1:3000/task-launches` in the in-app Browser. Verify:

- Sidebar shows “任务发起”.
- Sidebar does not show “任务中心”.
- Task launch form fits desktop viewport without overlapping.
- Advanced reviewer selector opens and shows human members only.

If authenticated local data is not available, use the test suite evidence and record browser smoke as environment-blocked rather than inventing product success.

- [ ] **Step 6: Final commit**

Run:

```bash
git status --short
```

Confirm only intended implementation files are modified. Then:

```bash
git add CHANGELOG.md
git commit -m "docs: record task launch implementation"
```

If previous task commits already included all code, this commit should contain only `CHANGELOG.md`.

## Self-Review

Spec coverage:

- “任务发起” primary navigation is covered by Task 4.
- “任务中心” removal from primary navigation is covered by Task 4.
- Reviewer preference and default rules are covered by Task 1.
- Launch detail aggregation is covered by Task 2.
- OpenAPI and web client contract are covered by Task 3.
- Create page and detail page are covered by Tasks 5 and 6.
- No xyflow flow diagram is implemented; Task 6 explicitly renders real fact lists only.
- Runtime task API is not deleted; Task 4 explicitly keeps `/tasks` as a non-primary technical route.

Placeholder scan:

- The plan contains no TBD/TODO markers.
- Steps include exact file paths, commands, expected outcomes, and concrete code shapes.

Type consistency:

- `reviewer_user_id` and `reviewer_selection_reason` are used consistently in backend request, OpenAPI, and web client.
- `ReviewerPreference` maps to `reviewer` in JSON responses.
- `ProjectDemandLaunchDetail` uses `coordination_jobs`, `route_decisions`, `project_tasks`, `decision_requests`, and `recent_events` consistently across backend and frontend.

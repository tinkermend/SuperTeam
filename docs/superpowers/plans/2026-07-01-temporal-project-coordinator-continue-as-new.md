# Temporal ProjectCoordinator Continue-As-New Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make ProjectCoordinator workflows history-safe by continuing as new and routing human decisions from durable project decision facts instead of workflow-local pending maps.

**Architecture:** Add the minimum durable association needed for `plan_review` decisions (`project_decision_requests.plan_revision_id`), expose it through sqlc/domain/repository/store types, then route `HumanDecisionSubmitted` from `project_decision_requests` instead of in-memory pending maps. Keep Provider/Codex/Claude Code session recovery outside Temporal; Continue-As-New carries only coordinator identity and generation.

**Tech Stack:** Go Control Plane, Temporal Go SDK v1.44.1, PostgreSQL migrations managed by Atlas, sqlc generated queries, existing projectcoordination workflow tests.

---

## File Structure

- Modify `apps/control-plane/internal/storage/migrations/041_project_decision_plan_revision_resume.sql`: forward migration adding `project_decision_requests.plan_revision_id`.
- Modify `apps/control-plane/internal/storage/migrations/atlas.sum`: generated Atlas checksum.
- Modify `apps/control-plane/internal/storage/migrations_test.go`: schema contract test for the new column/index.
- Modify `apps/control-plane/internal/storage/queries/project.sql`: include `plan_revision_id` in decision request insert/select/returning paths.
- Modify generated sqlc files under `apps/control-plane/internal/storage/queries/`: produced by `sqlc generate`.
- Modify `apps/control-plane/internal/project/repository.go`: add `PlanRevisionID *uuid.UUID` to create request and domain decision.
- Modify `apps/control-plane/internal/project/types.go`: add `PlanRevisionID *uuid.UUID` to `DecisionRequest`.
- Modify `apps/control-plane/internal/project/pg_repository.go`: map `plan_revision_id`.
- Modify `apps/control-plane/internal/project/service_test.go`: update memory repository decision request fixture mapping.
- Modify `apps/control-plane/internal/workflow/projectcoordination/types.go`: add coordinator generation and human decision route DTOs.
- Modify `apps/control-plane/internal/workflow/projectcoordination/activities.go`: extend `ActivityStore` and activity wrapper for durable human-decision routing.
- Modify `apps/control-plane/internal/workflow/projectcoordination/project_store.go`: write `plan_revision_id` when requesting plan review and add route lookup logic.
- Modify `apps/control-plane/internal/workflow/projectcoordination/workflow.go`: remove long-lived pending maps, route decisions from store, add Continue-As-New.
- Modify `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`: red/green tests for store-routed decisions and Continue-As-New.
- Modify `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`: store-level coverage for `plan_revision_id` persistence and route resolution.
- Optional create `scripts/dev/clear-project-coordination-data.sql`: guarded development cleanup script if live DB cleanup is needed during execution.

## Task 1: Persist Plan Revision Link on Decision Requests

**Files:**
- Create: `apps/control-plane/internal/storage/migrations/041_project_decision_plan_revision_resume.sql`
- Modify: `apps/control-plane/internal/storage/migrations_test.go`
- Modify: `apps/control-plane/internal/storage/queries/project.sql`
- Modify generated: `apps/control-plane/internal/storage/queries/models.go`
- Modify generated: `apps/control-plane/internal/storage/queries/project.sql.go`
- Modify generated: `apps/control-plane/internal/storage/queries/querier.go`
- Modify generated: `apps/control-plane/internal/storage/migrations/atlas.sum`

- [ ] **Step 1: Add failing migration test**

Append this test to `apps/control-plane/internal/storage/migrations_test.go` near other project migration tests:

```go
func TestProjectDecisionRequestsPlanRevisionResumeMigration(t *testing.T) {
	body, err := os.ReadFile("migrations/041_project_decision_plan_revision_resume.sql")
	require.NoError(t, err)
	sql := string(body)

	for _, expected := range []string{
		"ALTER TABLE project_decision_requests",
		"ADD COLUMN plan_revision_id UUID",
		"idx_project_decision_requests_plan_revision",
		"ON project_decision_requests(tenant_id, project_id, plan_revision_id)",
		"WHERE plan_revision_id IS NOT NULL",
		"COMMENT ON COLUMN project_decision_requests.plan_revision_id IS",
	} {
		require.Contains(t, sql, expected)
	}
}
```

- [ ] **Step 2: Run migration test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestProjectDecisionRequestsPlanRevisionResumeMigration -count=1
```

Expected: FAIL because `migrations/041_project_decision_plan_revision_resume.sql` does not exist.

- [ ] **Step 3: Add migration**

Create `apps/control-plane/internal/storage/migrations/041_project_decision_plan_revision_resume.sql`:

```sql
ALTER TABLE project_decision_requests
    ADD COLUMN plan_revision_id UUID;

CREATE INDEX idx_project_decision_requests_plan_revision
    ON project_decision_requests(tenant_id, project_id, plan_revision_id)
    WHERE plan_revision_id IS NOT NULL;

COMMENT ON COLUMN project_decision_requests.plan_revision_id IS '该人类决策关联的计划版本ID，用于 ProjectCoordinator Continue-As-New 后恢复 plan_review 路由。';
```

- [ ] **Step 4: Update project decision request queries**

In `apps/control-plane/internal/storage/queries/project.sql`, update `CreateProjectDecisionRequest` to include `plan_revision_id`:

```sql
-- name: CreateProjectDecisionRequest :one
INSERT INTO project_decision_requests (
    tenant_id,
    project_id,
    approval_request_id,
    coordination_job_id,
    project_task_id,
    plan_revision_id,
    target_user_id,
    decision_type,
    title_snapshot,
    summary_snapshot,
    risk_level_snapshot,
    status_snapshot,
    created_event_id
) VALUES (
    sqlc.arg('tenant_id')::uuid,
    sqlc.arg('project_id')::uuid,
    sqlc.arg('approval_request_id')::uuid,
    sqlc.narg('coordination_job_id')::uuid,
    sqlc.narg('project_task_id')::uuid,
    sqlc.narg('plan_revision_id')::uuid,
    sqlc.arg('target_user_id')::uuid,
    sqlc.arg('decision_type')::varchar,
    sqlc.arg('title_snapshot')::varchar,
    sqlc.narg('summary_snapshot')::text,
    sqlc.narg('risk_level_snapshot')::varchar,
    sqlc.arg('status_snapshot')::varchar,
    sqlc.narg('created_event_id')::uuid
) RETURNING *;
```

Leave `SELECT *` queries as `SELECT *`; sqlc will include the new model field after generation.

- [ ] **Step 5: Generate sqlc and Atlas checksum**

Run:

```bash
cd apps/control-plane && sqlc generate
cd apps/control-plane && atlas migrate hash --dir file://internal/storage/migrations
```

Expected:
- `apps/control-plane/internal/storage/queries/models.go` includes `PlanRevisionID uuid.NullUUID` on `ProjectDecisionRequest`.
- `apps/control-plane/internal/storage/queries/project.sql.go` includes `PlanRevisionID` in `CreateProjectDecisionRequestParams` and scans.
- `apps/control-plane/internal/storage/migrations/atlas.sum` includes `041_project_decision_plan_revision_resume.sql`.

- [ ] **Step 6: Run tests**

Run:

```bash
go test ./apps/control-plane/internal/storage -run TestProjectDecisionRequestsPlanRevisionResumeMigration -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/storage/migrations/041_project_decision_plan_revision_resume.sql \
  apps/control-plane/internal/storage/migrations/atlas.sum \
  apps/control-plane/internal/storage/migrations_test.go \
  apps/control-plane/internal/storage/queries/project.sql \
  apps/control-plane/internal/storage/queries/models.go \
  apps/control-plane/internal/storage/queries/project.sql.go \
  apps/control-plane/internal/storage/queries/querier.go
git commit -m "feat: persist plan review decision link"
```

## Task 2: Expose PlanRevisionID Through Project Repository Types

**Files:**
- Modify: `apps/control-plane/internal/project/repository.go`
- Modify: `apps/control-plane/internal/project/types.go`
- Modify: `apps/control-plane/internal/project/pg_repository.go`
- Modify: `apps/control-plane/internal/project/service_test.go`

- [ ] **Step 1: Add failing repository test**

Append this test near decision request tests in `apps/control-plane/internal/project/service_test.go`:

```go
func TestMemoryRepositoryDecisionRequestCarriesPlanRevisionID(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	planRevisionID := uuid.New()
	repo := &memoryRepository{}

	decision, err := repo.CreateDecisionRequest(context.Background(), CreateDecisionRequestRequest{
		TenantID:          tenantID,
		ProjectID:         projectID,
		ApprovalRequestID: uuid.New(),
		PlanRevisionID:    &planRevisionID,
		TargetUserID:      uuid.New(),
		DecisionType:      "plan_review",
		TitleSnapshot:     "确认项目计划版本",
		StatusSnapshot:    "pending",
	})

	require.NoError(t, err)
	require.NotNil(t, decision.PlanRevisionID)
	require.Equal(t, planRevisionID, *decision.PlanRevisionID)
}
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestMemoryRepositoryDecisionRequestCarriesPlanRevisionID -count=1
```

Expected: FAIL because `PlanRevisionID` is not defined.

- [ ] **Step 3: Add domain and create request fields**

In `apps/control-plane/internal/project/repository.go`, update `CreateDecisionRequestRequest`:

```go
type CreateDecisionRequestRequest struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	ApprovalRequestID uuid.UUID
	CoordinationJobID *uuid.UUID
	ProjectTaskID     *uuid.UUID
	PlanRevisionID    *uuid.UUID
	TargetUserID      uuid.UUID
	DecisionType      string
	TitleSnapshot     string
	SummarySnapshot   string
	RiskLevelSnapshot string
	StatusSnapshot    string
	CreatedEventID    *uuid.UUID
}
```

In `apps/control-plane/internal/project/types.go`, update `DecisionRequest`:

```go
type DecisionRequest struct {
	ID                   uuid.UUID
	TenantID             uuid.UUID
	ProjectID            uuid.UUID
	ApprovalRequestID    uuid.UUID
	CoordinationJobID    *uuid.UUID
	ProjectTaskID        *uuid.UUID
	PlanRevisionID       *uuid.UUID
	TargetUserID         uuid.UUID
	DecisionType         string
	TitleSnapshot        string
	SummarySnapshot      *string
	RiskLevelSnapshot    *string
	StatusSnapshot       string
	CreatedEventID       *uuid.UUID
	ResolvedEventID      *uuid.UUID
	CreatedAt            time.Time
	UpdatedAt            time.Time
	ResolvedAt           *time.Time
	DispatchGateResultID *uuid.UUID
}
```

- [ ] **Step 4: Map Postgres repository field**

In `apps/control-plane/internal/project/pg_repository.go`, update `createDecisionRequestWithQueries`:

```go
row, err := q.CreateProjectDecisionRequest(ctx, queries.CreateProjectDecisionRequestParams{
	TenantID:          req.TenantID,
	ProjectID:         req.ProjectID,
	ApprovalRequestID: req.ApprovalRequestID,
	CoordinationJobID: nullUUID(req.CoordinationJobID),
	ProjectTaskID:     nullUUID(req.ProjectTaskID),
	PlanRevisionID:    nullUUID(req.PlanRevisionID),
	TargetUserID:      req.TargetUserID,
	DecisionType:      req.DecisionType,
	TitleSnapshot:     req.TitleSnapshot,
	SummarySnapshot:   textOrNull(req.SummarySnapshot),
	RiskLevelSnapshot: textOrNull(req.RiskLevelSnapshot),
	StatusSnapshot:    req.StatusSnapshot,
	CreatedEventID:    nullUUID(req.CreatedEventID),
})
```

Update `decisionRequestFromRecord`:

```go
func decisionRequestFromRecord(row queries.ProjectDecisionRequest) (DecisionRequest, error) {
	return DecisionRequest{
		ID:                   row.ID,
		TenantID:             row.TenantID,
		ProjectID:            row.ProjectID,
		ApprovalRequestID:    row.ApprovalRequestID,
		CoordinationJobID:    ptrUUID(row.CoordinationJobID),
		ProjectTaskID:        ptrUUID(row.ProjectTaskID),
		PlanRevisionID:       ptrUUID(row.PlanRevisionID),
		TargetUserID:         row.TargetUserID,
		DecisionType:         row.DecisionType,
		TitleSnapshot:        row.TitleSnapshot,
		SummarySnapshot:      ptrText(row.SummarySnapshot),
		RiskLevelSnapshot:    ptrText(row.RiskLevelSnapshot),
		StatusSnapshot:       row.StatusSnapshot,
		CreatedEventID:       ptrUUID(row.CreatedEventID),
		ResolvedEventID:      ptrUUID(row.ResolvedEventID),
		CreatedAt:            row.CreatedAt.Time,
		UpdatedAt:            row.UpdatedAt.Time,
		ResolvedAt:           ptrTime(row.ResolvedAt),
		DispatchGateResultID: ptrUUID(row.DispatchGateResultID),
	}, nil
}
```

- [ ] **Step 5: Update memory repository**

In `apps/control-plane/internal/project/service_test.go`, update `memoryRepository.CreateDecisionRequest`:

```go
decision := DecisionRequest{
	ID:                uuid.New(),
	TenantID:          req.TenantID,
	ProjectID:         req.ProjectID,
	ApprovalRequestID: req.ApprovalRequestID,
	CoordinationJobID: req.CoordinationJobID,
	ProjectTaskID:     req.ProjectTaskID,
	PlanRevisionID:    req.PlanRevisionID,
	TargetUserID:      req.TargetUserID,
	DecisionType:      req.DecisionType,
	TitleSnapshot:     req.TitleSnapshot,
	SummarySnapshot:   strPtrOrNil(req.SummarySnapshot),
	RiskLevelSnapshot: strPtrOrNil(req.RiskLevelSnapshot),
	StatusSnapshot:    req.StatusSnapshot,
	CreatedEventID:    req.CreatedEventID,
	CreatedAt:         time.Now().UTC(),
	UpdatedAt:         time.Now().UTC(),
}
```

- [ ] **Step 6: Run tests**

Run:

```bash
go test ./apps/control-plane/internal/project -run TestMemoryRepositoryDecisionRequestCarriesPlanRevisionID -count=1
go test ./apps/control-plane/internal/project -run 'TestResolveDecision|TestMemoryRepositoryDecisionRequestCarriesPlanRevisionID' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/project/repository.go \
  apps/control-plane/internal/project/types.go \
  apps/control-plane/internal/project/pg_repository.go \
  apps/control-plane/internal/project/service_test.go
git commit -m "feat: expose plan revision decision link"
```

## Task 3: Persist PlanRevisionID When Requesting Plan Review

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`

- [ ] **Step 1: Add failing store test**

Append this test to `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`:

```go
func TestProjectStoreRequestPlanRevisionReviewStoresPlanRevisionID(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	demandID := uuid.New()
	coordinationJobID := uuid.New()
	planRevisionID := uuid.New()
	targetUserID := uuid.New()
	repo := &projectStoreMemoryRepository{
		projectRecord: project.Project{
			ID:               projectID,
			TenantID:         tenantID,
			HumanOwnerUserID: targetUserID,
		},
	}
	approvals := &recordingApprovalCreator{approvalID: uuid.New()}
	store := NewProjectStoreWithApprovals(repo, approvals)

	decision, err := store.RequestPlanRevisionReview(context.Background(), RequestPlanRevisionReviewInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		CoordinationJobID: coordinationJobID,
		DemandID:          demandID,
		PlanRevisionID:    planRevisionID,
		PlanFingerprint:   "fingerprint",
		Payload: PlanRevisionPayload{
			Summary: "需要审核",
			RiskAssessment: PlanRiskAssessment{
				HighestRiskLevel: "high",
			},
		},
		CreatedEventID: uuid.New(),
	})

	require.NoError(t, err)
	require.NotEqual(t, uuid.Nil, decision.ID)
	require.Len(t, repo.decisionRequests, 1)
	require.NotNil(t, repo.decisionRequests[0].PlanRevisionID)
	require.Equal(t, planRevisionID, *repo.decisionRequests[0].PlanRevisionID)
}
```

If `recordingApprovalCreator` does not exist in this package, add this helper near other test fakes:

```go
type recordingApprovalCreator struct {
	approvalID uuid.UUID
}

func (r *recordingApprovalCreator) CreateRequest(ctx context.Context, input approval.CreateRequestInput) (*approval.ApprovalRequest, error) {
	id := r.approvalID
	if id == uuid.Nil {
		id = uuid.New()
	}
	return &approval.ApprovalRequest{
		ID:             id,
		TenantID:       input.TenantID,
		ResourceType:   input.ResourceType,
		ResourceID:     input.ResourceID,
		RequesterType:  input.RequesterType,
		TargetUserID:   input.TargetUserID,
		DecisionType:   input.DecisionType,
		Title:          input.Title,
		Summary:        input.Summary,
		RiskLevel:      input.RiskLevel,
		Status:         approval.ApprovalStatusPending,
		ContextPayload: input.ContextPayload,
	}, nil
}

func (r *recordingApprovalCreator) GetRequestByResource(ctx context.Context, tenantID uuid.UUID, resourceType string, resourceID uuid.UUID) (*approval.ApprovalRequest, error) {
	return nil, approval.ErrApprovalNotFound
}
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run TestProjectStoreRequestPlanRevisionReviewStoresPlanRevisionID -count=1
```

Expected: FAIL because `RequestPlanRevisionReview` does not set `PlanRevisionID`.

- [ ] **Step 3: Set plan revision in decision request**

In `apps/control-plane/internal/workflow/projectcoordination/project_store.go`, update `RequestPlanRevisionReview`:

```go
planRevisionID := input.PlanRevisionID
decision, err := s.repository.CreateDecisionRequest(ctx, project.CreateDecisionRequestRequest{
	TenantID:          input.TenantID,
	ProjectID:         input.ProjectID,
	ApprovalRequestID: approvalRequest.ID,
	CoordinationJobID: &coordinationJobID,
	PlanRevisionID:    &planRevisionID,
	TargetUserID:      targetUserID,
	DecisionType:      "plan_review",
	TitleSnapshot:     "确认项目计划版本",
	SummarySnapshot:   input.Payload.Summary,
	RiskLevelSnapshot: nonEmptyString(input.Payload.RiskAssessment.HighestRiskLevel, "medium"),
	StatusSnapshot:    "pending",
	CreatedEventID:    &event.ID,
})
```

- [ ] **Step 4: Run test**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run TestProjectStoreRequestPlanRevisionReviewStoresPlanRevisionID -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/project_store.go \
  apps/control-plane/internal/workflow/projectcoordination/project_store_test.go
git commit -m "feat: link plan review decisions"
```

## Task 4: Add Durable Human Decision Route Activity

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/types.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/activities.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`

- [ ] **Step 1: Add failing route resolution test**

Append to `apps/control-plane/internal/workflow/projectcoordination/project_store_test.go`:

```go
func TestProjectStoreLoadHumanDecisionRouteForPlanReview(t *testing.T) {
	tenantID := uuid.New()
	projectID := uuid.New()
	decisionID := uuid.New()
	planRevisionID := uuid.New()
	coordinationJobID := uuid.New()
	demandID := uuid.New()
	routeDecisionID := uuid.New()
	repo := &projectStoreMemoryRepository{
		decisionRequests: []project.DecisionRequest{{
			ID:                decisionID,
			TenantID:          tenantID,
			ProjectID:         projectID,
			CoordinationJobID: &coordinationJobID,
			PlanRevisionID:    &planRevisionID,
			DecisionType:      "plan_review",
			StatusSnapshot:    "resolved",
		}},
		planRevisions: []project.PlanRevision{{
			ID:                planRevisionID,
			TenantID:          tenantID,
			ProjectID:         projectID,
			DemandID:          demandID,
			CoordinationJobID: &coordinationJobID,
			RouteDecisionID:   &routeDecisionID,
			Status:            project.PlanRevisionStatusPendingReview,
			PlanFingerprint:   "fingerprint",
			Payload: map[string]any{
				"summary": "review me",
				"tasks": []any{},
			},
		}},
	}
	store := NewProjectStore(repo)

	route, err := store.LoadHumanDecisionRoute(context.Background(), LoadHumanDecisionRouteInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionID,
	})

	require.NoError(t, err)
	require.Equal(t, "plan_review", route.Decision.DecisionType)
	require.NotNil(t, route.PlanReview)
	require.Equal(t, planRevisionID, route.PlanReview.PlanRevisionID)
	require.Equal(t, demandID, route.PlanReview.DemandID)
	require.Equal(t, coordinationJobID, route.PlanReview.CoordinationJobID)
	require.Equal(t, routeDecisionID, route.PlanReview.RouteDecisionID)
}
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run TestProjectStoreLoadHumanDecisionRouteForPlanReview -count=1
```

Expected: FAIL because `LoadHumanDecisionRoute` types and method are missing.

- [ ] **Step 3: Define route DTOs**

In `apps/control-plane/internal/workflow/projectcoordination/types.go`, add:

```go
type LoadHumanDecisionRouteInput struct {
	TenantID          uuid.UUID
	ProjectID         uuid.UUID
	DecisionRequestID uuid.UUID
}

type HumanDecisionRouteResult struct {
	Decision   ProjectDecisionSnapshot
	PlanReview *PlanReviewRoute
}

type ProjectDecisionSnapshot struct {
	ID                   uuid.UUID
	ProjectID            uuid.UUID
	DecisionType         string
	StatusSnapshot       string
	CoordinationJobID    uuid.UUID
	ProjectTaskID        uuid.UUID
	PlanRevisionID       uuid.UUID
	DispatchGateResultID uuid.UUID
}

type PlanReviewRoute struct {
	ProjectID         uuid.UUID
	DemandID          uuid.UUID
	CoordinationJobID uuid.UUID
	RouteDecisionID   uuid.UUID
	PlanRevisionID    uuid.UUID
	PlanFingerprint   string
	Payload           PlanRevisionPayload
}
```

- [ ] **Step 4: Extend activity interface and wrapper**

In `apps/control-plane/internal/workflow/projectcoordination/activities.go`, add to `ActivityStore`:

```go
LoadHumanDecisionRoute(ctx context.Context, input LoadHumanDecisionRouteInput) (HumanDecisionRouteResult, error)
```

Add method:

```go
func (a *Activities) LoadHumanDecisionRoute(ctx context.Context, input LoadHumanDecisionRouteInput) (HumanDecisionRouteResult, error) {
	if a.store == nil {
		return HumanDecisionRouteResult{}, ErrActivityStoreRequired
	}
	return a.store.LoadHumanDecisionRoute(ctx, input)
}
```

- [ ] **Step 5: Implement store route lookup**

In `apps/control-plane/internal/workflow/projectcoordination/project_store.go`, add:

```go
func (s *ProjectStore) LoadHumanDecisionRoute(ctx context.Context, input LoadHumanDecisionRouteInput) (HumanDecisionRouteResult, error) {
	if s.repository == nil {
		return HumanDecisionRouteResult{}, ErrActivityStoreRequired
	}
	decision, err := s.repository.GetDecisionRequest(ctx, input.TenantID, input.ProjectID, input.DecisionRequestID)
	if err != nil {
		if errors.Is(err, project.ErrProjectNotFound) {
			return HumanDecisionRouteResult{}, nil
		}
		return HumanDecisionRouteResult{}, err
	}
	result := HumanDecisionRouteResult{
		Decision: ProjectDecisionSnapshot{
			ID:                   decision.ID,
			ProjectID:            decision.ProjectID,
			DecisionType:         decision.DecisionType,
			StatusSnapshot:       decision.StatusSnapshot,
			CoordinationJobID:    uuidValue(decision.CoordinationJobID),
			ProjectTaskID:        uuidValue(decision.ProjectTaskID),
			PlanRevisionID:       uuidValue(decision.PlanRevisionID),
			DispatchGateResultID: uuidValue(decision.DispatchGateResultID),
		},
	}
	if decision.DecisionType != "plan_review" {
		return result, nil
	}
	if decision.PlanRevisionID == nil || *decision.PlanRevisionID == uuid.Nil {
		return HumanDecisionRouteResult{}, project.ErrInvalidProject
	}
	revision, err := s.repository.GetPlanRevision(ctx, input.TenantID, input.ProjectID, *decision.PlanRevisionID)
	if err != nil {
		return HumanDecisionRouteResult{}, err
	}
	payload, err := planRevisionPayloadFromMap(revision.Payload)
	if err != nil {
		return HumanDecisionRouteResult{}, err
	}
	if revision.CoordinationJobID == nil || revision.RouteDecisionID == nil {
		return HumanDecisionRouteResult{}, project.ErrInvalidProject
	}
	result.PlanReview = &PlanReviewRoute{
		ProjectID:         revision.ProjectID,
		DemandID:          revision.DemandID,
		CoordinationJobID: *revision.CoordinationJobID,
		RouteDecisionID:   *revision.RouteDecisionID,
		PlanRevisionID:    revision.ID,
		PlanFingerprint:   revision.PlanFingerprint,
		Payload:           payload,
	}
	return result, nil
}

func uuidValue(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return *id
}
```

If `planRevisionPayloadFromMap` does not exist, add it next to `planRevisionPayloadMap`:

```go
func planRevisionPayloadFromMap(input map[string]any) (PlanRevisionPayload, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return PlanRevisionPayload{}, err
	}
	var payload PlanRevisionPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return PlanRevisionPayload{}, err
	}
	return payload, nil
}
```

- [ ] **Step 6: Update test fakes**

Add method to `recordingActivityStore` in `workflow_test.go` and to any package test fake that implements `ActivityStore`:

```go
func (s *recordingActivityStore) LoadHumanDecisionRoute(ctx context.Context, input LoadHumanDecisionRouteInput) (HumanDecisionRouteResult, error) {
	s.calls = append(s.calls, "LoadHumanDecisionRoute")
	route := s.humanDecisionRoutes[input.DecisionRequestID]
	return route, nil
}
```

Add fields to `recordingActivityStore`:

```go
humanDecisionRoutes map[uuid.UUID]HumanDecisionRouteResult
```

- [ ] **Step 7: Run tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run TestProjectStoreLoadHumanDecisionRouteForPlanReview -count=1
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/types.go \
  apps/control-plane/internal/workflow/projectcoordination/activities.go \
  apps/control-plane/internal/workflow/projectcoordination/project_store.go \
  apps/control-plane/internal/workflow/projectcoordination/project_store_test.go \
  apps/control-plane/internal/workflow/projectcoordination/workflow_test.go
git commit -m "feat: load coordinator decision routes"
```

## Task 5: Route Human Decisions From Store Instead of Pending Maps

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow.go`

- [ ] **Step 1: Add failing workflow test for plan review after workflow-local state is absent**

Append to `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`:

```go
func TestProjectCoordinatorRoutesPlanReviewFromStoreWithoutPendingMap(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	projectID := uuid.New()
	decisionRequestID := uuid.New()
	coordinationJobID := uuid.New()
	routeDecisionID := uuid.New()
	planRevisionID := uuid.New()
	demandID := uuid.New()
	readyTaskID := uuid.New()
	store := &recordingActivityStore{
		snapshot:                  CoordinationSnapshot{ProjectID: projectID},
		dispatchableTaskIDBatches: [][]uuid.UUID{{readyTaskID}},
		humanDecisionRoutes: map[uuid.UUID]HumanDecisionRouteResult{
			decisionRequestID: {
				Decision: ProjectDecisionSnapshot{
					ID:             decisionRequestID,
					ProjectID:      projectID,
					DecisionType:   "plan_review",
					StatusSnapshot: "resolved",
				},
				PlanReview: &PlanReviewRoute{
					ProjectID:         projectID,
					DemandID:          demandID,
					CoordinationJobID: coordinationJobID,
					RouteDecisionID:   routeDecisionID,
					PlanRevisionID:    planRevisionID,
					PlanFingerprint:   "fingerprint",
					Payload:           PlanRevisionPayload{Summary: "approved plan"},
				},
			},
		},
	}
	activities := newRawDispatchWorkflowActivities(store)
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalHumanDecisionSubmitted, HumanDecisionSubmitted{
			DecisionRequestID: decisionRequestID,
			Decision:          project.PlanReviewDecisionAccept,
			ResolvedEventID:   uuid.New(),
		})
	}, time.Millisecond)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalShutdown, ShutdownSignal{})
	}, 10*time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
	})

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.Equal(t, []string{
		"LoadHumanDecisionRoute",
		"ResolvePlanRevisionReview",
		"DecomposeAcceptedPlanRevision",
		"ListDispatchableTasks",
		"DispatchProjectTask",
		"FinishCoordinationJob",
	}, store.calls)
	require.Len(t, store.resolvePlanReviewInputs, 1)
	require.Equal(t, planRevisionID, store.resolvePlanReviewInputs[0].PlanRevisionID)
	require.Len(t, store.dispatchInputs, 1)
	require.Equal(t, readyTaskID, store.dispatchInputs[0].TaskID)
	require.Equal(t, project.DispatchReasonHumanResolved, store.dispatchInputs[0].DispatchReason)
}
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run TestProjectCoordinatorRoutesPlanReviewFromStoreWithoutPendingMap -count=1
```

Expected: FAIL because current workflow only knows plan review through `pendingReviews`.

- [ ] **Step 3: Replace long-lived pending maps for human decision routing**

In `apps/control-plane/internal/workflow/projectcoordination/workflow.go`, remove these long-lived maps:

```go
pendingReviews := map[string]pendingPlanRevisionReview{}
pendingFailureRecoveries := map[string]pendingTaskFailureRecovery{}
pendingAcceptance := map[string]pendingProjectAcceptance{}
```

Keep return values from `handleDemandSubmitted`, `handleEmployeeTaskCompleted`, and `handleEmployeeTaskFailed` only as activity outcomes, not as long-term routing state. The returned pending structs can be removed once all callers are updated.

Change the human signal handler to:

```go
selector.AddReceive(humanCh, func(c workflow.ReceiveChannel, more bool) {
	var signal HumanDecisionSubmitted
	c.Receive(ctx, &signal)
	workflowErr = handleHumanDecisionSubmitted(ctx, input, signal)
})
```

- [ ] **Step 4: Implement store-routed human decision handler**

Replace `handleHumanDecisionSubmitted` with:

```go
func handleHumanDecisionSubmitted(ctx workflow.Context, input ProjectCoordinatorInput, signal HumanDecisionSubmitted) error {
	route, err := loadHumanDecisionRoute(ctx, input.TenantID, input.ProjectID, signal.DecisionRequestID)
	if err != nil {
		return err
	}
	if route.Decision.ID == uuid.Nil {
		return appendSignalObservedEvent(ctx, input, "human decision submitted for unknown request")
	}
	switch route.Decision.DecisionType {
	case "plan_review":
		if route.PlanReview == nil {
			return temporal.NewNonRetryableApplicationError("plan review route missing", "ProjectCoordinatorRouteInvalid", project.ErrInvalidProject)
		}
		return handlePlanReviewDecision(ctx, input, signal, pendingPlanRevisionReview{
			DecisionRequestID: signal.DecisionRequestID,
			ProjectID:         route.PlanReview.ProjectID,
			DemandID:          route.PlanReview.DemandID,
			CoordinationJobID: route.PlanReview.CoordinationJobID,
			RouteDecisionID:   route.PlanReview.RouteDecisionID,
			PlanRevisionID:    route.PlanReview.PlanRevisionID,
			PlanFingerprint:   route.PlanReview.PlanFingerprint,
			Payload:           route.PlanReview.Payload,
		})
	case "task_failure_recovery":
		readyTaskIDs, err := applyFailureRecoveryDecision(ctx, input.TenantID, input.ProjectID, signal)
		if err != nil {
			return err
		}
		return dispatchProjectTasks(ctx, input.TenantID, input.ProjectID, readyTaskIDs, project.DispatchReasonRetry)
	case "project_acceptance":
		return applyProjectAcceptanceDecision(ctx, input.TenantID, input.ProjectID, signal)
	case "project_task_approval":
		readyTaskIDs, err := applyPreDispatchGateDecision(ctx, input.TenantID, input.ProjectID, signal)
		if err != nil {
			return err
		}
		if len(readyTaskIDs) > 0 {
			return dispatchProjectTasks(ctx, input.TenantID, input.ProjectID, readyTaskIDs, project.DispatchReasonHumanResolved)
		}
		return appendSignalObservedEvent(ctx, input, "human decision submitted")
	default:
		readyTaskIDs, err := applyPreDispatchGateDecision(ctx, input.TenantID, input.ProjectID, signal)
		if err != nil {
			return err
		}
		if len(readyTaskIDs) > 0 {
			return dispatchProjectTasks(ctx, input.TenantID, input.ProjectID, readyTaskIDs, project.DispatchReasonHumanResolved)
		}
		return appendSignalObservedEvent(ctx, input, "human decision submitted")
	}
}
```

Add helper:

```go
func loadHumanDecisionRoute(ctx workflow.Context, tenantID, projectID, decisionRequestID uuid.UUID) (HumanDecisionRouteResult, error) {
	var route HumanDecisionRouteResult
	err := workflow.ExecuteActivity(ctx, (*Activities).LoadHumanDecisionRoute, LoadHumanDecisionRouteInput{
		TenantID:          tenantID,
		ProjectID:         projectID,
		DecisionRequestID: decisionRequestID,
	}).Get(ctx, &route)
	return route, err
}
```

Keep `pendingPlanRevisionReview` as a local route DTO for `handlePlanReviewDecision` until a later cleanup. Remove `pendingFailureRecoveries` and `pendingAcceptance` arguments and map logic.

- [ ] **Step 5: Update existing workflow tests' expected calls**

Tests that signal `HumanDecisionSubmitted` now need `humanDecisionRoutes` populated and expected calls updated to start with `LoadHumanDecisionRoute`.

For `TestProjectCoordinatorDispatchesHumanResolvedTaskThroughGate`, add:

```go
humanDecisionRoutes: map[uuid.UUID]HumanDecisionRouteResult{
	decisionRequestID: {
		Decision: ProjectDecisionSnapshot{
			ID:             decisionRequestID,
			ProjectID:      store.snapshot.ProjectID,
			DecisionType:   "plan_review",
			StatusSnapshot: "resolved",
		},
		PlanReview: &PlanReviewRoute{
			ProjectID:         store.snapshot.ProjectID,
			DemandID:          store.snapshot.Demand.ID,
			CoordinationJobID: store.jobID,
			RouteDecisionID:   store.routeID,
			PlanRevisionID:    store.planRevisionID,
			PlanFingerprint:   "fingerprint",
			Payload:           PlanRevisionPayload{Summary: "approved plan"},
		},
	},
```

Set `store.planRevisionID = uuid.New()` in that test before constructing the route if needed.

Update expected calls for plan review human decisions to include `LoadHumanDecisionRoute` before `ResolvePlanRevisionReview`.

For `TestProjectCoordinatorDispatchesRetryReasonAfterHumanRecoveryDecision`, populate:

```go
humanDecisionRoutes: map[uuid.UUID]HumanDecisionRouteResult{
	decisionRequestID: {
		Decision: ProjectDecisionSnapshot{
			ID:             decisionRequestID,
			ProjectID:      projectID,
			DecisionType:   "task_failure_recovery",
			StatusSnapshot: "resolved",
			ProjectTaskID:  failedTaskID,
		},
	},
},
```

For `TestProjectCoordinatorRequestsAcceptanceReviewAndAppliesDecision`, populate route for `acceptanceID` with `DecisionType: "project_acceptance"`.

- [ ] **Step 6: Run workflow tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectCoordinator.*Human|TestProjectCoordinator.*PlanReview|TestProjectCoordinator.*Recovery|TestProjectCoordinator.*Acceptance|TestProjectCoordinatorRoutesPlanReviewFromStoreWithoutPendingMap' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/workflow.go \
  apps/control-plane/internal/workflow/projectcoordination/workflow_test.go
git commit -m "feat: route coordinator decisions from store"
```

## Task 6: Add Continue-As-New After Complete Signal Processing

**Files:**
- Modify: `apps/control-plane/internal/workflow/projectcoordination/types.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow.go`
- Modify: `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`

- [ ] **Step 1: Add failing Continue-As-New test**

Append to `apps/control-plane/internal/workflow/projectcoordination/workflow_test.go`:

```go
func TestProjectCoordinatorContinuesAsNewWhenSuggestedAfterSignal(t *testing.T) {
	var suite testsuite.WorkflowTestSuite
	env := suite.NewTestWorkflowEnvironment()
	env.SetContinueAsNewSuggested(true)
	projectID := uuid.New()
	store := &recordingActivityStore{snapshot: CoordinationSnapshot{ProjectID: projectID}, dispatchEvent: uuid.New()}
	activities := NewActivities(store, HeuristicRoutePlanner{})
	env.RegisterActivity(activities)
	env.RegisterDelayedCallback(func() {
		env.SignalWorkflow(SignalProjectPolicyChanged, ProjectPolicyChanged{
			ProjectID:        projectID,
			ConfigRevisionID: uuid.New(),
			ChangedEventID:   uuid.New(),
		})
	}, time.Millisecond)

	env.ExecuteWorkflow(ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   uuid.New(),
		ProjectID:  projectID,
		WorkflowID: "project-coordinator:" + projectID.String(),
		Generation: 3,
	})

	require.True(t, env.IsWorkflowCompleted())
	require.Error(t, env.GetWorkflowError())
	require.Contains(t, env.GetWorkflowError().Error(), "continue as new")
	require.Equal(t, []string{"AppendProjectEvent"}, store.calls)
}
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run TestProjectCoordinatorContinuesAsNewWhenSuggestedAfterSignal -count=1
```

Expected: FAIL because workflow currently keeps waiting instead of returning Continue-As-New.

- [ ] **Step 3: Add generation field**

In `apps/control-plane/internal/workflow/projectcoordination/types.go`, update input:

```go
type ProjectCoordinatorInput struct {
	TenantID   uuid.UUID
	ProjectID  uuid.UUID
	WorkflowID string
	Generation int
}
```

- [ ] **Step 4: Add Continue-As-New check**

In `apps/control-plane/internal/workflow/projectcoordination/workflow.go`, after `shouldStop` handling and before looping:

```go
if shouldStop {
	return nil
}
if shouldContinueAsNew(ctx) {
	return workflow.NewContinueAsNewError(ctx, ProjectCoordinatorWorkflow, ProjectCoordinatorInput{
		TenantID:   input.TenantID,
		ProjectID:  input.ProjectID,
		WorkflowID: input.WorkflowID,
		Generation: input.Generation + 1,
	})
}
```

Add helper near the bottom:

```go
func shouldContinueAsNew(ctx workflow.Context) bool {
	info := workflow.GetInfo(ctx)
	return info.GetContinueAsNewSuggested()
}
```

- [ ] **Step 5: Run Continue-As-New test**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run TestProjectCoordinatorContinuesAsNewWhenSuggestedAfterSignal -count=1
```

Expected: PASS with `continue as new` workflow error in test suite.

- [ ] **Step 6: Run workflow regression tests**

Run:

```bash
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectCoordinator' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add apps/control-plane/internal/workflow/projectcoordination/types.go \
  apps/control-plane/internal/workflow/projectcoordination/workflow.go \
  apps/control-plane/internal/workflow/projectcoordination/workflow_test.go
git commit -m "feat: continue coordinator workflows as new"
```

## Task 7: Add Development Cleanup Script

**Files:**
- Create: `scripts/dev/clear-project-coordination-data.sql`
- Optional Modify: `docs/superpowers/plans/2026-07-01-temporal-project-coordinator-continue-as-new.md` only if execution notes need updates.

- [ ] **Step 1: Create guarded cleanup script**

Create `scripts/dev/clear-project-coordination-data.sql`:

```sql
\echo 'This script clears SuperTeam development project coordination data.'
\echo 'It preserves auth_users, auth_sessions, web_login_logs, user_project_team_scopes, tenants, tenant teams, and login/account data.'
\echo 'Run only against the confirmed development database.'

BEGIN;

SELECT 'project_task_attestations' AS table_name, count(*) FROM project_task_attestations
UNION ALL SELECT 'project_placements', count(*) FROM project_placements
UNION ALL SELECT 'project_task_results', count(*) FROM project_task_results
UNION ALL SELECT 'project_task_dispatch_gate_results', count(*) FROM project_task_dispatch_gate_results
UNION ALL SELECT 'project_task_attempt_context_updates', count(*) FROM project_task_attempt_context_updates
UNION ALL SELECT 'project_task_attempts', count(*) FROM project_task_attempts
UNION ALL SELECT 'project_plan_decomposition_claims', count(*) FROM project_plan_decomposition_claims
UNION ALL SELECT 'project_plan_revisions', count(*) FROM project_plan_revisions
UNION ALL SELECT 'project_decision_requests', count(*) FROM project_decision_requests
UNION ALL SELECT 'project_transfer_requests', count(*) FROM project_transfer_requests
UNION ALL SELECT 'project_execution_summaries', count(*) FROM project_execution_summaries
UNION ALL SELECT 'project_route_decisions', count(*) FROM project_route_decisions
UNION ALL SELECT 'project_coordination_jobs', count(*) FROM project_coordination_jobs
UNION ALL SELECT 'project_acceptance_records', count(*) FROM project_acceptance_records
UNION ALL SELECT 'project_archive_snapshots', count(*) FROM project_archive_snapshots
UNION ALL SELECT 'project_budget_ledger', count(*) FROM project_budget_ledger
UNION ALL SELECT 'project_report_refs', count(*) FROM project_report_refs
UNION ALL SELECT 'project_artifact_refs', count(*) FROM project_artifact_refs
UNION ALL SELECT 'project_evidence_refs', count(*) FROM project_evidence_refs
UNION ALL SELECT 'project_task_dependencies', count(*) FROM project_task_dependencies
UNION ALL SELECT 'project_events', count(*) FROM project_events
UNION ALL SELECT 'project_demands', count(*) FROM project_demands
UNION ALL SELECT 'project_members', count(*) FROM project_members
UNION ALL SELECT 'project_tasks', count(*) FROM project_tasks
UNION ALL SELECT 'projects', count(*) FROM projects
UNION ALL SELECT 'project approvals', count(*) FROM approval_requests WHERE requester_type = 'project_coordinator'
UNION ALL SELECT 'project inbox items', count(*) FROM inbox_items WHERE source_project_id IS NOT NULL;

DELETE FROM inbox_items WHERE source_project_id IS NOT NULL;
DELETE FROM approval_decisions WHERE approval_request_id IN (SELECT id FROM approval_requests WHERE requester_type = 'project_coordinator');
DELETE FROM approval_requests WHERE requester_type = 'project_coordinator';

TRUNCATE TABLE
    project_task_attestations,
    project_placements,
    project_task_results,
    project_task_dispatch_gate_results,
    project_task_attempt_context_updates,
    project_task_attempts,
    project_plan_decomposition_claims,
    project_plan_revisions,
    project_decision_requests,
    project_transfer_requests,
    project_execution_summaries,
    project_route_decisions,
    project_coordination_jobs,
    project_acceptance_records,
    project_archive_snapshots,
    project_budget_ledger,
    project_report_refs,
    project_artifact_refs,
    project_evidence_refs,
    project_task_dependencies,
    project_events,
    project_demands,
    project_members,
    project_tasks,
    projects;

SELECT 'auth_users preserved' AS check_name, count(*) FROM auth_users
UNION ALL SELECT 'auth_sessions preserved', count(*) FROM auth_sessions
UNION ALL SELECT 'web_login_logs preserved', count(*) FROM web_login_logs
UNION ALL SELECT 'user_project_team_scopes preserved', count(*) FROM user_project_team_scopes;

COMMIT;
```

- [ ] **Step 2: Validate script table names statically**

Run:

```bash
rg -n "project_decision_requests|auth_users|TRUNCATE TABLE|requester_type = 'project_coordinator'" scripts/dev/clear-project-coordination-data.sql
```

Expected: output shows delete/truncate statements and preserved auth checks.

- [ ] **Step 3: Commit**

```bash
git add scripts/dev/clear-project-coordination-data.sql
git commit -m "chore: add project coordination dev cleanup"
```

## Task 8: Verification and Real-Chain Smoke

**Files:**
- No required source edits.
- May update implementation notes in this plan only if command output reveals an environment-specific blocker.

- [ ] **Step 1: Run focused unit tests**

Run:

```bash
go test ./apps/control-plane/internal/storage -run 'TestProjectDecisionRequestsPlanRevisionResumeMigration' -count=1
go test ./apps/control-plane/internal/project -run 'TestMemoryRepositoryDecisionRequestCarriesPlanRevisionID|TestResolveDecision' -count=1
go test ./apps/control-plane/internal/workflow/projectcoordination -run 'TestProjectStoreRequestPlanRevisionReviewStoresPlanRevisionID|TestProjectStoreLoadHumanDecisionRouteForPlanReview|TestProjectCoordinatorRoutesPlanReviewFromStoreWithoutPendingMap|TestProjectCoordinatorContinuesAsNewWhenSuggestedAfterSignal|TestProjectCoordinator' -count=1
```

Expected: all PASS.

- [ ] **Step 2: Run package tests for touched packages**

Run:

```bash
go test ./apps/control-plane/internal/project ./apps/control-plane/internal/workflow/projectcoordination ./apps/control-plane/internal/storage -count=1
```

Expected: PASS.

- [ ] **Step 3: Check migration status against confirmed development DB**

Only run after confirming `DATABASE_URL` points at the intended development database:

```bash
export DATABASE_URL="$(awk '/^postgres:/{f=1;next} f&&/^[[:space:]]*url:/{sub(/^[[:space:]]*url:[[:space:]]*/,""); gsub(/"/,""); print; exit}' apps/control-plane/config/config.yaml)"
test -n "$DATABASE_URL" || { echo "ERROR: could not extract postgres.url from config.yaml" >&2; exit 1; }
make -C apps/control-plane migrate-status
make -C apps/control-plane migrate-up
make -C apps/control-plane migrate-status
```

Expected: migration 041 applies. If `atlas` or DB connectivity is missing, stop and report blocked; do not claim DB migration verified.

- [ ] **Step 4: Optionally clear old development project data**

Only if the user still wants old project data removed and `DATABASE_URL` is confirmed as development:

```bash
psql "$DATABASE_URL" -f scripts/dev/clear-project-coordination-data.sql
```

Expected: script prints counts, clears project coordination data, and preserves auth counts.

- [ ] **Step 5: Restart affected services**

Run:

```bash
scripts/dev-services.sh status
scripts/dev-services.sh restart control-plane
scripts/dev-services.sh restart runtime-agent
scripts/dev-services.sh status
```

Expected: Temporal, Control Plane, Web, and Runtime Agent are running. If a service is down and cannot start, real-chain smoke is blocked.

- [ ] **Step 6: Run real coordinator smoke**

Use the current repo's real project creation and demand APIs or existing smoke helper. The smoke must prove:

1. Create a new Project.
2. Submit a demand that creates a plan review or task DAG.
3. Resolve a `plan_review` decision through the real `project.Service.ResolveDecision` / API path.
4. Confirm the coordinator receives `HumanDecisionSubmitted` and advances via DB route facts.
5. Confirm at least one ProjectTask exists or dispatch attempt/gate result is created.

If no existing smoke script covers this, use curl against the running Control Plane with real auth and record the exact endpoints and IDs in the final report. Do not invent a success state.

- [ ] **Step 7: Run hygiene checks**

Run:

```bash
git diff --check
git status --short
```

Expected: no whitespace errors. Only intended files changed.

- [ ] **Step 8: Confirm verification did not create unexpected edits**

Run:

```bash
git status --short
```

Expected: no new source edits from Task 8. If generated files, migrations, tests, or smoke scripts changed during verification, stop and review the diff before creating another commit; do not create an empty verification commit.

## Self-Review

- Spec coverage: migration and durable `plan_revision_id` are covered in Tasks 1-3; DB-routed human decisions are covered in Tasks 4-5; Continue-As-New is covered in Task 6; development cleanup is covered in Task 7; real-chain verification is covered in Task 8.
- No Provider session state is added to Temporal. Runtime/Provider recovery remains outside the coordinator.
- The plan accounts for the current `ResolveDecision` flow marking `project_decision_requests` resolved before signaling workflow, so workflow routing must not treat non-pending as automatic no-op.
- No placeholders are intentionally left; commands and expected results are explicit.

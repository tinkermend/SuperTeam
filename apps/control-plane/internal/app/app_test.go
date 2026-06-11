package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/superteam/control-plane/internal/api"
	"github.com/superteam/control-plane/internal/artifact"
	"github.com/superteam/control-plane/internal/config"
	"github.com/superteam/control-plane/internal/project"
	runtimepkg "github.com/superteam/control-plane/internal/runtime"
	"github.com/superteam/control-plane/internal/storage"
)

func TestHealthOnlyRouterIsExplicit(t *testing.T) {
	router := NewHealthOnlyRouter()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", response.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("expected JSON health response: %v", err)
	}

	if body["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", body["status"])
	}
	if body["service"] != "control-plane" {
		t.Fatalf("expected service control-plane, got %q", body["service"])
	}
}

func TestRunContainerClosesPollerWhenContextIsCanceled(t *testing.T) {
	poller := runtimepkg.NewPoller()
	container := &Container{
		Poller: poller,
		Server: api.NewServer(nil, nil),
	}

	waitErr := make(chan error, 1)
	go func() {
		_, err := poller.WaitForTask(context.Background(), "node-1")
		waitErr <- err
	}()

	waitForActivePoller(t, poller)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := runContainer(ctx, container, "127.0.0.1:0"); err != nil {
		t.Fatalf("expected clean shutdown, got %v", err)
	}

	if err := <-waitErr; !errors.Is(err, runtimepkg.ErrPollerClosed) {
		t.Fatalf("expected poller waiter to be closed, got %v", err)
	}
}

func TestRunContainerStartsAndStopsWorkflowWorker(t *testing.T) {
	poller := runtimepkg.NewPoller()
	worker := &recordingWorkflowWorker{}
	client := &recordingTemporalClient{}
	container := &Container{
		Poller:              poller,
		Server:              api.NewServer(nil, nil),
		CoordinationWorker:  worker,
		TemporalClientClose: client.Close,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := runContainer(ctx, container, "127.0.0.1:0"); err != nil {
		t.Fatalf("expected clean shutdown, got %v", err)
	}

	if worker.starts != 1 {
		t.Fatalf("expected workflow worker to start once, got %d", worker.starts)
	}
	if worker.stops != 1 {
		t.Fatalf("expected workflow worker to stop once, got %d", worker.stops)
	}
	if client.closes != 1 {
		t.Fatalf("expected temporal client to close once, got %d", client.closes)
	}
}

func TestRunContainerClosesTemporalClientWhenWorkerStartFails(t *testing.T) {
	startErr := errors.New("worker start failed")
	worker := &recordingWorkflowWorker{startErr: startErr}
	client := &recordingTemporalClient{}
	container := &Container{
		Poller:              runtimepkg.NewPoller(),
		Server:              api.NewServer(nil, nil),
		CoordinationWorker:  worker,
		TemporalClientClose: client.Close,
	}

	err := runContainer(context.Background(), container, "127.0.0.1:0")
	if !errors.Is(err, startErr) {
		t.Fatalf("expected worker start error, got %v", err)
	}
	if client.closes != 1 {
		t.Fatalf("expected temporal client to close on start failure, got %d", client.closes)
	}
	if worker.stops != 0 {
		t.Fatalf("worker must not be stopped when start fails, got %d", worker.stops)
	}
}

func TestNewContainerWithConfigWiresTemporalOnlyWhenEnabled(t *testing.T) {
	stores := newTestStorageClients(t)

	disabled, err := NewContainerWithConfig(stores, config.Config{})
	if err != nil {
		t.Fatalf("new disabled container: %v", err)
	}
	if disabled.CoordinationWorker != nil || disabled.TemporalClientClose != nil {
		t.Fatalf("expected Temporal lifecycle to be nil when disabled")
	}
	if disabled.ApprovalService == nil {
		t.Fatalf("expected approval service to be wired")
	}
	if disabled.ArtifactService == nil {
		t.Fatalf("expected artifact service to be wired")
	}
	if disabled.ProjectService == nil {
		t.Fatalf("expected project service to be wired")
	}

	enabled, err := NewContainerWithConfig(stores, config.Config{
		Temporal: config.TemporalConfig{
			Enabled:   true,
			Address:   "127.0.0.1:7233",
			Namespace: "default",
			TaskQueue: "superteam-project-coordination-test",
		},
	})
	if err != nil {
		t.Fatalf("new enabled container: %v", err)
	}
	if enabled.CoordinationWorker == nil {
		t.Fatalf("expected coordination worker when Temporal is enabled")
	}
	if enabled.TemporalClientClose == nil {
		t.Fatalf("expected Temporal client closer when Temporal is enabled")
	}
	if enabled.ArtifactService == nil {
		t.Fatalf("expected artifact service to be wired")
	}
	if enabled.ProjectService == nil {
		t.Fatalf("expected project service to be wired")
	}
	enabled.TemporalClientClose()
}

func TestProjectArtifactLockerReturnsRetentionEventID(t *testing.T) {
	artifactRepo := &fakeArtifactRepository{}
	artifactService, err := artifact.NewService(artifactRepo)
	if err != nil {
		t.Fatalf("new artifact service: %v", err)
	}
	eventID := uuid.New()
	eventAppender := &fakeProjectEventAppender{eventID: eventID}
	locker := projectArtifactLocker{artifactService: artifactService, projectEvents: eventAppender}
	tenantID := uuid.New()
	projectID := uuid.New()
	artifactID := uuid.New()

	result, err := locker.LockProjectArtifacts(context.Background(), tenantID, projectID, []uuid.UUID{artifactID})
	if err != nil {
		t.Fatalf("lock project artifacts: %v", err)
	}
	if result.EventID == nil || *result.EventID != eventID {
		t.Fatalf("expected retention event id %s, got %#v", eventID, result.EventID)
	}
	if len(result.HoldIDs) != 1 || result.HoldIDs[0] == uuid.Nil {
		t.Fatalf("expected hold id, got %#v", result.HoldIDs)
	}
	if len(result.ArtifactIDs) != 1 || result.ArtifactIDs[0] != artifactID {
		t.Fatalf("expected artifact id, got %#v", result.ArtifactIDs)
	}
	if eventAppender.last.EventType != project.ProjectEventArchiveRetentionPending {
		t.Fatalf("expected retention pending event, got %s", eventAppender.last.EventType)
	}
	if eventAppender.last.ActorType != "system" || eventAppender.last.ActorID != "project_archive_retention" {
		t.Fatalf("unexpected event actor: %#v", eventAppender.last)
	}
	if eventAppender.last.Payload["artifact_count"] != 1 {
		t.Fatalf("expected artifact count payload, got %#v", eventAppender.last.Payload)
	}
	if artifactRepo.last.TenantID != tenantID || artifactRepo.last.ArtifactID != artifactID || artifactRepo.last.ResourceID != projectID {
		t.Fatalf("expected artifact hold request to be recorded, got %#v", artifactRepo.last)
	}
}

func TestProjectArtifactLockerReturnsEventIDOnArtifactServiceError(t *testing.T) {
	artifactErr := errors.New("artifact hold failed")
	artifactRepo := &fakeArtifactRepository{err: artifactErr}
	artifactService, err := artifact.NewService(artifactRepo)
	if err != nil {
		t.Fatalf("new artifact service: %v", err)
	}
	eventID := uuid.New()
	locker := projectArtifactLocker{
		artifactService: artifactService,
		projectEvents:   &fakeProjectEventAppender{eventID: eventID},
	}

	result, err := locker.LockProjectArtifacts(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
	if !errors.Is(err, artifactErr) {
		t.Fatalf("expected artifact service error, got %v", err)
	}
	if result.EventID == nil || *result.EventID != eventID {
		t.Fatalf("expected retention event id on failure %s, got %#v", eventID, result.EventID)
	}
}

func waitForActivePoller(t *testing.T, poller *runtimepkg.Poller) {
	t.Helper()

	deadline := time.After(time.Second)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()

	for {
		if poller.ActiveWaiters() > 0 {
			return
		}

		select {
		case <-deadline:
			t.Fatal("timed out waiting for active poller waiter")
		case <-ticker.C:
		}
	}
}

func newTestStorageClients(t *testing.T) *storage.Clients {
	t.Helper()

	poolConfig, err := pgxpool.ParseConfig("postgres://superteam:superteam@127.0.0.1:1/superteam_test?sslmode=disable")
	if err != nil {
		t.Fatalf("parse pgx pool config: %v", err)
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		t.Fatalf("new pgx pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return &storage.Clients{Postgres: pool}
}

type recordingWorkflowWorker struct {
	starts   int
	stops    int
	startErr error
}

func (w *recordingWorkflowWorker) Start() error {
	w.starts++
	return w.startErr
}

func (w *recordingWorkflowWorker) Stop() {
	w.stops++
}

type recordingTemporalClient struct {
	closes int
}

func (c *recordingTemporalClient) Close() {
	c.closes++
}

type fakeProjectEventAppender struct {
	eventID uuid.UUID
	last    project.AppendProjectEventRequest
	err     error
}

func (a *fakeProjectEventAppender) AppendProjectEvent(ctx context.Context, event project.AppendProjectEventRequest) (project.ProjectEvent, error) {
	if a.err != nil {
		return project.ProjectEvent{}, a.err
	}
	a.last = event
	return project.ProjectEvent{
		ID:        a.eventID,
		TenantID:  event.TenantID,
		ProjectID: event.ProjectID,
		EventType: event.EventType,
		ActorType: event.ActorType,
		ActorID:   event.ActorID,
		Summary:   &event.Summary,
		Payload:   event.Payload,
	}, nil
}

type fakeArtifactRepository struct {
	last artifact.CreateRetentionHoldRequest
	err  error
}

func (r *fakeArtifactRepository) CreateRetentionHold(ctx context.Context, req artifact.CreateRetentionHoldRequest) (artifact.RetentionHold, error) {
	if r.err != nil {
		return artifact.RetentionHold{}, r.err
	}
	r.last = req
	return artifact.RetentionHold{
		ID:         uuid.New(),
		TenantID:   req.TenantID,
		ArtifactID: req.ArtifactID,
		Status:     "active",
	}, nil
}

func (r *fakeArtifactRepository) CountActiveRetentionHolds(ctx context.Context, tenantID, artifactID uuid.UUID) (int32, error) {
	return 0, nil
}

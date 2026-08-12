package project

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	runtimepkg "github.com/superteam/control-plane/internal/runtime"
)

// scriptedWorkspaceCommander records every runtime command and lets a test
// script the terminal receipt per command type — needed because probe results
// (directory facts) come back through the receipt Result map.
type scriptedWorkspaceCommander struct {
	tenantID uuid.UUID
	// dispatched keeps commands in order so a test can assert both "what ran"
	// and "what never ran" (e.g. attach must not mkdir).
	dispatched []runtimepkg.RuntimeCommand
	// results maps command type → receipt Result on success.
	results map[string]map[string]any
	// failures maps command type → error message (receipt status=failed).
	failures map[string]string

	typeByCommandID map[string]string
}

func newScriptedCommander(tenantID uuid.UUID) *scriptedWorkspaceCommander {
	return &scriptedWorkspaceCommander{
		tenantID:        tenantID,
		results:         map[string]map[string]any{},
		failures:        map[string]string{},
		typeByCommandID: map[string]string{},
	}
}

func (c *scriptedWorkspaceCommander) GetNodeByID(_ context.Context, id uuid.UUID) (runtimepkg.NodeRecord, error) {
	return runtimepkg.NodeRecord{ID: id, TenantID: c.tenantID, NodeID: "node-" + id.String()[:8], Name: "节点-" + id.String()[:4]}, nil
}

func (c *scriptedWorkspaceCommander) IsConnected(string) bool { return true }

func (c *scriptedWorkspaceCommander) Dispatch(_ context.Context, _ string, command runtimepkg.RuntimeCommand) error {
	c.dispatched = append(c.dispatched, command)
	c.typeByCommandID[command.ID] = command.Type
	return nil
}

func (c *scriptedWorkspaceCommander) CreateCommandReceipt(_ context.Context, _ WorkspaceCommandReceiptRequest) error {
	return nil
}

func (c *scriptedWorkspaceCommander) WaitForCommandCompletion(_ context.Context, _ uuid.UUID, commandID string, _ time.Duration) (*WorkspaceCommandReceipt, error) {
	commandType := c.typeByCommandID[commandID]
	if msg, ok := c.failures[commandType]; ok {
		return &WorkspaceCommandReceipt{CommandID: commandID, Status: "failed", ErrorMessage: &msg}, nil
	}
	return &WorkspaceCommandReceipt{
		CommandID: commandID,
		Status:    "completed",
		Result:    c.results[commandType],
	}, nil
}

func (c *scriptedWorkspaceCommander) dispatchedTypeList() []string {
	types := make([]string, 0, len(c.dispatched))
	for _, command := range c.dispatched {
		types = append(types, command.Type)
	}
	return types
}

func (c *scriptedWorkspaceCommander) payloadOf(t *testing.T, commandType string) map[string]any {
	t.Helper()
	for _, command := range c.dispatched {
		if command.Type != commandType {
			continue
		}
		var payload map[string]any
		require.NoError(t, json.Unmarshal(command.Payload, &payload))
		return payload
	}
	t.Fatalf("no %s command was dispatched (got %v)", commandType, c.dispatchedTypeList())
	return nil
}

func attachProbeFacts() map[string]any {
	return map[string]any{
		"exists":         true,
		"is_dir":         true,
		"is_symlink":     false,
		"is_git_repo":    true,
		"origin_url":     "git@example.com:acme/legacy.git",
		"current_branch": "main",
		"dirty":          false,
		"head_commit":    "0123456789abcdef",
	}
}

func newProvisionTestService(
	t *testing.T,
	repo *memoryRepository,
	commander RuntimeWorkspaceCommander,
	tenantID uuid.UUID,
	nodeIDs ...uuid.UUID,
) *Service {
	t.Helper()
	service, err := NewServiceWithCoordinator(repo, &fakeCoordinatorSignalClient{})
	require.NoError(t, err)
	service.SetRuntimeWorkspaceCommander(commander)
	records := make([]runtimepkg.NodeRecord, 0, len(nodeIDs))
	for _, id := range nodeIDs {
		records = append(records, onlineNode(id, tenantID, "node-"+id.String()[:8], 0, 4))
	}
	service.SetProjectRuntimeNodeReader(&fakeProjectRuntimeNodeReader{nodes: records})
	return service
}

func createAttachProjectRequest(tenantID, actorID, nodeID uuid.UUID) CreateProjectRequest {
	return CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      actorID,
		Name:             "存量资料库",
		Goal:             "纳管已有目录",
		DirectoryName:    "legacy-erp",
		SourceKind:       ProjectSourceKindAttach,
		HumanOwnerUserID: actorID,
		RuntimeNodeIDs:   []uuid.UUID{nodeID},
	}
}

// Attach 的承重语义：平台只探测，绝不建目录、不 clone(spec §5.1 / 不变量 3、5)。
func TestCreateProjectAttachProbesAndNeverCreatesDirectory(t *testing.T) {
	repo := newMemoryRepository()
	tenantID, actorID, nodeID := uuid.New(), uuid.New(), uuid.New()
	commander := newScriptedCommander(tenantID)
	commander.results[runtimeCommandProbeProjectDirectory] = attachProbeFacts()
	service := newProvisionTestService(t, repo, commander, tenantID, nodeID)

	result, err := service.CreateProject(context.Background(), createAttachProjectRequest(tenantID, actorID, nodeID))
	require.NoError(t, err)

	require.Equal(t, []string{runtimeCommandProbeProjectDirectory}, commander.dispatchedTypeList(),
		"attach 必须只探测:不得下发 ensure/clone")
	require.Equal(t, "legacy-erp", commander.payloadOf(t, runtimeCommandProbeProjectDirectory)["project_name"])

	require.Equal(t, WorkspaceOwnershipAttached, result.Project.WorkspaceOwnership)
	require.Equal(t, WorkspaceReadyStatusReady, result.Project.WorkspaceReadyStatus)

	nodes, err := repo.ListProjectRuntimeNodes(context.Background(), tenantID, result.Project.ID)
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	require.True(t, nodes[0].IsProvisioned())
	require.NotNil(t, nodes[0].ProvisionSource)
	require.Equal(t, "attach_probe", *nodes[0].ProvisionSource)
}

// 探测失败(目录不存在)必须整体失败并回滚,不得留下半成品项目。
func TestCreateProjectAttachRollsBackWhenProbeFails(t *testing.T) {
	repo := newMemoryRepository()
	tenantID, actorID, nodeID := uuid.New(), uuid.New(), uuid.New()
	commander := newScriptedCommander(tenantID)
	commander.failures[runtimeCommandProbeProjectDirectory] = "project directory missing"
	service := newProvisionTestService(t, repo, commander, tenantID, nodeID)

	_, err := service.CreateProject(context.Background(), createAttachProjectRequest(tenantID, actorID, nodeID))
	require.Error(t, err)
	require.ErrorIs(t, err, ErrProjectWorkspaceProvision)

	for _, project := range repo.projects {
		if project.DirectoryName == "legacy-erp" {
			require.NotNil(t, project.DeletedAt, "探测失败的 attach 项目必须回滚,不得留活行")
		}
	}
}

// Git 项目：mkdir 成功 ≠ 工作区可用。primary 要等 clone 回写才算已供给,
// 否则 `provisioned ⇒ 磁盘就绪` 这条不变量在创建路径上是假的。
func TestCreateProjectGitMarksPrimaryProvisionedOnlyAfterCloneWriteback(t *testing.T) {
	repo := newMemoryRepository()
	ctx := context.Background()
	tenantID, actorID, nodeID := uuid.New(), uuid.New(), uuid.New()
	commander := newScriptedCommander(tenantID)
	service := newProvisionTestService(t, repo, commander, tenantID, nodeID)

	result, err := service.CreateProject(ctx, CreateProjectRequest{
		TenantID:         tenantID,
		ActorUserID:      actorID,
		Name:             "带仓库的项目",
		Goal:             "接管代码",
		HumanOwnerUserID: actorID,
		RuntimeNodeIDs:   []uuid.UUID{nodeID},
		RepoBinding:      &ProjectRepoBindingInput{URL: "https://example.com/acme/app.git", DefaultBranch: "main"},
	})
	require.NoError(t, err)
	require.Equal(t, WorkspaceReadyStatusPending, result.Project.WorkspaceReadyStatus)

	nodes, err := repo.ListProjectRuntimeNodes(ctx, tenantID, result.Project.ID)
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	require.False(t, nodes[0].IsProvisioned(), "clone 还在飞时不得标为已供给")

	require.NoError(t, service.HandleCloneCommandTerminal(ctx, CloneCommandTerminal{
		TenantID:      tenantID,
		ProjectID:     result.Project.ID,
		RuntimeNodeID: nodeID,
		Success:       true,
	}))

	nodes, err = repo.ListProjectRuntimeNodes(ctx, tenantID, result.Project.ID)
	require.NoError(t, err)
	require.True(t, nodes[0].IsProvisioned(), "clone 落盘后才算已供给")
	require.NotNil(t, nodes[0].ProvisionSource)
	require.Equal(t, "clone", *nodes[0].ProvisionSource)
}

// 绑定 ≠ 供给：加节点是纯元数据，一条磁盘命令都不许发(不变量 5)。
func TestAddProjectRuntimeNodeBindsWithoutTouchingDisk(t *testing.T) {
	repo := newMemoryRepository()
	ctx := context.Background()
	tenantID, actorID, projectID := uuid.New(), uuid.New(), uuid.New()
	newNodeID := uuid.New()
	commander := newScriptedCommander(tenantID)
	service := newProvisionTestService(t, repo, commander, tenantID, newNodeID)
	repo.projects[projectID] = Project{
		ID: projectID, TenantID: tenantID, Name: "已有项目", DirectoryName: "existing-dir",
		Status: ProjectStatusRunning, HumanOwnerUserID: actorID,
		RepoBinding: ProjectRepoBinding{Status: ProjectRepoBindingStatusBound, URL: "https://example.com/a.git"},
	}

	node, err := service.AddProjectRuntimeNode(ctx, ModifyProjectRuntimeNodeRequest{
		TenantID: tenantID, ProjectID: projectID, RuntimeNodeID: newNodeID, ActorUserID: actorID,
	})
	require.NoError(t, err)
	require.Empty(t, commander.dispatchedTypeList(), "绑定节点不得下发任何磁盘命令")
	require.False(t, node.IsProvisioned())
}

func seedProvisionCandidate(t *testing.T, repo *memoryRepository, tenantID, projectID, actorID, nodeID uuid.UUID, binding ProjectRepoBinding, ownership WorkspaceOwnership) {
	t.Helper()
	repo.projects[projectID] = Project{
		ID: projectID, TenantID: tenantID, Name: "待供给项目", DirectoryName: "supply-dir",
		Status: ProjectStatusRunning, HumanOwnerUserID: actorID,
		RepoBinding: binding, WorkspaceOwnership: ownership,
		WorkspaceReadyStatus: WorkspaceReadyStatusReady,
	}
	_, err := repo.InsertProjectRuntimeNode(context.Background(), tenantID, projectID, nodeID, false, "")
	require.NoError(t, err)
}

// 供给确认：非 Git 项目 mkdir 成功后才标已供给。
func TestProvisionWorkspaceOnNodeMkdirMarksProvisioned(t *testing.T) {
	repo := newMemoryRepository()
	ctx := context.Background()
	tenantID, actorID, projectID, nodeID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	commander := newScriptedCommander(tenantID)
	service := newProvisionTestService(t, repo, commander, tenantID, nodeID)
	seedProvisionCandidate(t, repo, tenantID, projectID, actorID, nodeID, ProjectRepoBinding{Status: ProjectRepoBindingStatusUnbound}, WorkspaceOwnershipPlatformManaged)

	node, err := service.ProvisionWorkspaceOnNode(ctx, ModifyProjectRuntimeNodeRequest{
		TenantID: tenantID, ProjectID: projectID, RuntimeNodeID: nodeID, ActorUserID: actorID,
	})
	require.NoError(t, err)
	require.Equal(t, []string{runtimeCommandEnsureProjectDirectory}, commander.dispatchedTypeList())
	require.True(t, node.IsProvisioned())
	require.Equal(t, "confirm", *node.ProvisionSource)
}

// 供给确认：attached 项目只探测——平台不在备节点上凭空造目录。
func TestProvisionWorkspaceOnNodeAttachedOnlyProbes(t *testing.T) {
	repo := newMemoryRepository()
	ctx := context.Background()
	tenantID, actorID, projectID, nodeID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	commander := newScriptedCommander(tenantID)
	commander.results[runtimeCommandProbeProjectDirectory] = attachProbeFacts()
	service := newProvisionTestService(t, repo, commander, tenantID, nodeID)
	seedProvisionCandidate(t, repo, tenantID, projectID, actorID, nodeID, ProjectRepoBinding{Status: ProjectRepoBindingStatusUnbound}, WorkspaceOwnershipAttached)

	node, err := service.ProvisionWorkspaceOnNode(ctx, ModifyProjectRuntimeNodeRequest{
		TenantID: tenantID, ProjectID: projectID, RuntimeNodeID: nodeID, ActorUserID: actorID,
	})
	require.NoError(t, err)
	require.Equal(t, []string{runtimeCommandProbeProjectDirectory}, commander.dispatchedTypeList())
	require.Equal(t, "attach_probe", *node.ProvisionSource)
}

// attached 备节点上没有目录时，供给必须失败而不是"顺手建一个空的"。
func TestProvisionWorkspaceOnNodeAttachedFailsWhenDirectoryMissing(t *testing.T) {
	repo := newMemoryRepository()
	ctx := context.Background()
	tenantID, actorID, projectID, nodeID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	commander := newScriptedCommander(tenantID)
	commander.failures[runtimeCommandProbeProjectDirectory] = "attached project directory missing"
	service := newProvisionTestService(t, repo, commander, tenantID, nodeID)
	seedProvisionCandidate(t, repo, tenantID, projectID, actorID, nodeID, ProjectRepoBinding{Status: ProjectRepoBindingStatusUnbound}, WorkspaceOwnershipAttached)

	_, err := service.ProvisionWorkspaceOnNode(ctx, ModifyProjectRuntimeNodeRequest{
		TenantID: tenantID, ProjectID: projectID, RuntimeNodeID: nodeID, ActorUserID: actorID,
	})
	require.Error(t, err)
	require.NotContains(t, commander.dispatchedTypeList(), runtimeCommandEnsureProjectDirectory)

	nodes, err := repo.ListProjectRuntimeNodes(ctx, tenantID, projectID)
	require.NoError(t, err)
	require.False(t, nodes[0].IsProvisioned(), "探测失败不得留下 provisioned 假象")
}

func TestProvisionWorkspaceOnNodeIsIdempotentAndRejectsUnboundNode(t *testing.T) {
	repo := newMemoryRepository()
	ctx := context.Background()
	tenantID, actorID, projectID, nodeID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	commander := newScriptedCommander(tenantID)
	service := newProvisionTestService(t, repo, commander, tenantID, nodeID)
	seedProvisionCandidate(t, repo, tenantID, projectID, actorID, nodeID, ProjectRepoBinding{Status: ProjectRepoBindingStatusUnbound}, WorkspaceOwnershipPlatformManaged)

	_, err := service.ProvisionWorkspaceOnNode(ctx, ModifyProjectRuntimeNodeRequest{
		TenantID: tenantID, ProjectID: projectID, RuntimeNodeID: nodeID, ActorUserID: actorID,
	})
	require.NoError(t, err)
	require.Len(t, commander.dispatchedTypeList(), 1)

	// 二次供给是幂等 no-op，不再打一次磁盘。
	_, err = service.ProvisionWorkspaceOnNode(ctx, ModifyProjectRuntimeNodeRequest{
		TenantID: tenantID, ProjectID: projectID, RuntimeNodeID: nodeID, ActorUserID: actorID,
	})
	require.NoError(t, err)
	require.Len(t, commander.dispatchedTypeList(), 1)

	_, err = service.ProvisionWorkspaceOnNode(ctx, ModifyProjectRuntimeNodeRequest{
		TenantID: tenantID, ProjectID: projectID, RuntimeNodeID: uuid.New(), ActorUserID: actorID,
	})
	require.Error(t, err, "未绑定的节点不能被供给")
}

// attached 项目禁止 force reclone —— 那条路径会 remove_dir_all 用户的真实目录。
func TestRecloneRejectedForAttachedProject(t *testing.T) {
	repo := newMemoryRepository()
	ctx := context.Background()
	tenantID, actorID, projectID, nodeID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	commander := newScriptedCommander(tenantID)
	service := newProvisionTestService(t, repo, commander, tenantID, nodeID)
	repo.projects[projectID] = Project{
		ID: projectID, TenantID: tenantID, Name: "认领项目", DirectoryName: "attached-dir",
		Status: ProjectStatusRunning, HumanOwnerUserID: actorID,
		WorkspaceOwnership:   WorkspaceOwnershipAttached,
		WorkspaceReadyStatus: WorkspaceReadyStatusError,
		RepoBinding:          ProjectRepoBinding{Status: ProjectRepoBindingStatusBound, URL: "https://example.com/a.git"},
	}
	_, err := repo.InsertProjectRuntimeNode(ctx, tenantID, projectID, nodeID, true, "attach_probe")
	require.NoError(t, err)

	_, err = service.RecloneProjectWorkspace(ctx, WorkspaceManualActionRequest{
		TenantID: tenantID, ProjectID: projectID, ActorUserID: actorID,
	})
	require.Error(t, err)
	require.Empty(t, commander.dispatchedTypeList(), "被拒的 reclone 不得下发任何命令")
}

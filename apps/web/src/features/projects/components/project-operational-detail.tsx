import { lazy, Suspense, useEffect, useMemo, useState, type ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import {
  Activity,
  AlertTriangle,
  Archive,
  Bot,
  Trash2,
  ChevronDown,
  ClipboardList,
  FileCheck2,
  ExternalLink,
  FileText,
  GitBranch,
  MoreHorizontal,
  Settings2
} from "lucide-react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger
} from "@/components/ui/collapsible";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger
} from "@/components/ui/dropdown-menu";
import {
  IconTile,
  MasterDetailLayout,
  SoftCard,
  StatusPill,
  Button,
  DataTable,
  Td,
  Th,
  Tr,
  WorkSurface,
  type Tone,
  SoftTabs,
  SoftTabsList,
  SoftTabsTrigger,
  SoftTabsContent,
  LoadingState
} from "@/components/superteam";
import { cn } from "@/lib/utils";
import type {
  Project,
  ProjectAcceptanceRecord,
  ProjectArchivePreview,
  ProjectArchiveSnapshot,
  ProjectArtifactRef,
  ProjectBudgetLedgerEntry,
  ProjectBudgetSummary,
  ProjectCoordinationJob,
  CreateProjectArchiveSnapshotInput,
  CreateProjectEvidenceInput,
  DispatchGateResult,
  DispatchGateStatus,
  ProjectDecisionRequest,
  ProjectDemand,
  ProjectEvidenceRef,
  ProjectEvidenceVerificationStatus,
  ProjectEvent,
  ProjectExecutionTrace,
  ProjectExecutionSummary,
  ProjectMember,
  ProjectOverview,
  ProjectPlanRevision,
  ProjectReportRef,
  ProjectRouteDecision,
  ProjectStatus,
  ProjectTask,
  ProjectTaskGraph,
  ProjectTaskGraphBlockingFact,
  ProjectTransferRequest,
  WorkspaceReadyStatus
} from "@/lib/api/projects";
import {
  decisionStatusLabel,
  dispatchGateStatusLabel,
  projectStatusLabel,
  statusLabel,
  taskStatusLabel,
  workspaceReadyStatusLabel
} from "@/lib/status-labels";
import { compareIsoDesc, formatDateTime as formatAbsoluteDateTime, formatRelativeTime } from "@/lib/format-time";
import { taskIdFromNodeId, taskNodeId } from "@/features/flow-graph/flow-graph-adapter";
import { ProjectExecutionTracePanel } from "./project-execution-trace-panel";
import { ProjectAssetsPanel } from "./project-assets-panel";
import { ProjectGovernanceTabs } from "./project-governance-tabs";
import { ProjectOpsHome } from "./project-ops-home";
import { ProjectTaskDetailDialog } from "./project-task-detail-dialog";
import { ProjectOwnerAvatarStack } from "./project-owner-avatar-stack";
import {
  assetsInitialTabFromQuery,
  normalizeProjectDetailSection,
  type ProjectDetailSection
} from "../lib/project-detail-section";
import type { UserIdentityData } from "@/components/superteam/user-identity";

// 流程图权威画布依赖 @xyflow/react（重）。懒加载让它离开入口包——仅在有任务节点的
// 执行图区块渲染，首屏不需要（P1-D Step 1）。与流程编排详情共用同一画布组件，
// 节点可点击直达任务详情弹层（spec 2026-07-26 §4.2）。
const FlowGraphCanvas = lazy(() =>
  import("@/features/flow-graph/flow-graph-canvas").then((m) => ({
    default: m.FlowGraphCanvas
})),
);

type ProjectOperationalDetailProps = {
  acceptance?: ProjectAcceptanceRecord;
  archivePreview?: ProjectArchivePreview;
  archiveSnapshots?: ProjectArchiveSnapshot[];
  artifacts?: ProjectArtifactRef[];
  budgetLedger?: ProjectBudgetLedgerEntry[];
  budgetSummary?: ProjectBudgetSummary;
  coordinationJobs: ProjectCoordinationJob[];
  decisionRequests: ProjectDecisionRequest[];
  demands: ProjectDemand[];
  dispatchGateTaskTitle?: string;
  dispatchGates?: DispatchGateResult[];
  evidence?: ProjectEvidenceRef[];
  events: ProjectEvent[];
  executionTrace?: ProjectExecutionTrace;
  executionTraceErrorMessage?: string;
  executionTraceIsError?: boolean;
  executionTraceIsLoading?: boolean;
  executionSummaries: ProjectExecutionSummary[];
  /** 任务详情弹层按 demand 懒查执行图（页面只预载最新 demand 的图）。 */
  fetchTaskGraph?: (demandId: string) => Promise<ProjectTaskGraph>;
  focusDecisionId?: string;
  initialTab?: ProjectDetailSection | string;
  isArchived?: boolean;
  onArchiveProject: () => void;
  onDeleteProject?: () => void;
  onRecloneWorkspace?: () => void;
  onMarkWorkspaceReady?: () => void;
  workspaceActionPending?: boolean;
  onCreateArchiveSnapshot: (input: CreateProjectArchiveSnapshotInput) => void;
  onCreateEvidence: (input: CreateProjectEvidenceInput) => void;
  onPatchEvidence: (
    evidenceId: string,
    verificationStatus: ProjectEvidenceVerificationStatus,
  ) => void;
  onRetryExecutionTrace?: () => void;
  onResolveDecision: (
    decisionId: string,
    decision: string,
    targetExitDeliverable?: string,
  ) => void;
  onDismissTask?: (taskId: string) => void;
  dismissTaskPending?: boolean;
  onSubmitDemand: () => void;
  overview?: ProjectOverview;
  planRevisions: ProjectPlanRevision[];
  /** 成员快照缺名时回退到用户/数字员工真实名称，避免侧栏裸显 UUID。 */
  principalNamesById?: ReadonlyMap<string, string>;
  /** 人类负责人头像/身份补全（来自用户列表）。 */
  usersById?: ReadonlyMap<string, UserIdentityData>;
  project?: Project;
  reports?: ProjectReportRef[];
  routeDecisions: ProjectRouteDecision[];
  runtimePlacementPanel?: ReactNode;
  taskGraph?: ProjectTaskGraph;
  tasks: ProjectTask[];
  transferRequests: ProjectTransferRequest[];
};

const sectionTriggerClass =
  "h-auto flex-none rounded-none border-0 border-b-2 border-transparent bg-transparent px-3.5 pb-2.5 pt-1 text-[13px] font-semibold text-ink-2 shadow-none transition-colors data-[state=active]:border-brand data-[state=active]:bg-transparent data-[state=active]:text-brand-deep data-[state=active]:shadow-none data-[state=inactive]:hover:bg-transparent data-[state=inactive]:hover:text-ink";

export function ProjectOperationalDetail({
  acceptance,
  archivePreview,
  archiveSnapshots,
  artifacts,
  budgetLedger,
  budgetSummary,
  coordinationJobs,
  decisionRequests,
  demands,
  dispatchGateTaskTitle,
  dispatchGates,
  evidence,
  events,
  executionTrace,
  executionTraceErrorMessage,
  executionTraceIsError,
  executionTraceIsLoading,
  executionSummaries,
  fetchTaskGraph,
  focusDecisionId,
  initialTab = "workbench",
  isArchived,
  onArchiveProject,
  onDeleteProject,
  onRecloneWorkspace,
  onMarkWorkspaceReady,
  workspaceActionPending,
  onCreateArchiveSnapshot,
  onCreateEvidence,
  onPatchEvidence,
  onRetryExecutionTrace,
  onResolveDecision,
  onDismissTask,
  dismissTaskPending,
  onSubmitDemand: _onSubmitDemand,
  overview,
  planRevisions,
  principalNamesById,
  usersById,
  project,
  reports,
  routeDecisions,
  runtimePlacementPanel,
  taskGraph,
  tasks,
  transferRequests
}: ProjectOperationalDetailProps) {
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [detailTaskId, setDetailTaskId] = useState<string | undefined>();
  const initialSection = normalizeProjectDetailSection(
    typeof initialTab === "string" ? initialTab : undefined,
  );
  const [activeSection, setActiveSection] =
    useState<ProjectDetailSection>(initialSection);
  const assetsInitial = assetsInitialTabFromQuery(
    typeof initialTab === "string" ? initialTab : undefined,
  );

  useEffect(() => {
    setActiveSection(
      normalizeProjectDetailSection(
        typeof initialTab === "string" ? initialTab : undefined,
      ),
    );
  }, [initialTab, focusDecisionId]);

  // ?tab=trace 深链（任务详情弹层「查看执行轨迹」）：落在工作台并展开高级项目
  // 事实区，再定位到执行轨迹面板。等 Collapsible 展开渲染后再滚动。
  useEffect(() => {
    if (initialTab !== "trace") return;
    setAdvancedOpen(true);
    const timer = window.setTimeout(() => {
      document
        .getElementById("project-execution-trace")
        ?.scrollIntoView({ behavior: "smooth", block: "start" });
    }, 150);
    return () => window.clearTimeout(timer);
  }, [initialTab]);

  const latestPlanRevision = selectLatestPlanRevision(planRevisions);

  if (!project) {
    return (
      <SoftCard className="flex min-h-[460px] items-center justify-center p-8 text-sm text-ink-2">
        从左侧选择一个项目查看运行详情
      </SoftCard>
    );
  }

  const humanRoles = overview?.human_roles ?? [];
  const digitalPool = overview?.digital_employee_pool ?? [];
  const servicePool = digitalPool;
  const projectOwners = ownerMembers(humanRoles, project.human_owner_user_id);
  const latestDemand = demands[0];
  const pendingOwnerDecisions = decisionRequests.filter(
    (decision) => decision.status_snapshot === "pending",
  );
  const activeTasks = (overview?.active_tasks?.length ? overview.active_tasks : tasks).filter(
    (task) => !task.dismissed_at,
  );
  const latestPlanReviewDecision = decisionRequests.find(
    (decision) =>
      decision.decision_type === "plan_review" &&
      decision.status_snapshot === "pending" &&
      (!latestPlanRevision ||
        !latestPlanRevision.coordination_job_id ||
        decision.coordination_job_id === latestPlanRevision.coordination_job_id),
  );
  const canArchive =
    !isArchived &&
    (project.allowed_actions === undefined
      ? true
      : project.allowed_actions.includes("project.archive"));
  const canDelete =
    Boolean(onDeleteProject) && project.allowed_actions?.includes("project.delete");
  const canManageWorkspace =
    !isArchived &&
    (project.workspace_ready_status === "pending" ||
      project.workspace_ready_status === "error");
  const showReclone =
    canManageWorkspace &&
    Boolean(onRecloneWorkspace) &&
    Boolean(project.repo_binding && project.repo_binding.status === "bound");
  const showMarkReady = canManageWorkspace && Boolean(onMarkWorkspaceReady);

  return (
    <div className="grid min-w-0 gap-4">
      <SoftCard className="p-5">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex min-w-0 items-start gap-3">
            <IconTile tone="brand" size="lg">
              <Activity />
            </IconTile>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h2 className="truncate text-xl font-semibold tracking-normal text-ink">
                  {project.name}
                </h2>
                <StatusPill tone={projectStatusTone(project.status)}>
                  {projectStatusLabel(project.status)}
                </StatusPill>
                {project.workspace_ready_status &&
                project.workspace_ready_status !== "ready" ? (
                  <StatusPill tone={workspaceReadyTone(project.workspace_ready_status)}>
                    {workspaceReadyStatusLabel(project.workspace_ready_status)}
                  </StatusPill>
                ) : null}
              </div>
              {projectOwners.length > 0 ? (
                <ProjectOwnerAvatarStack
                  owners={projectOwners}
                  principalNamesById={principalNamesById}
                  usersById={usersById}
                />
              ) : null}
              <p className="mt-1 text-xs text-ink-3">
                <span>目录名</span>{" "}
                <span className="font-mono text-[12px] font-medium text-ink">
                  {project.directory_name || project.name}
                </span>
                {project.workspace_ready_error
                  ? ` · ${project.workspace_ready_error}`
                  : ""}
              </p>
              <div className="mt-3 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-ink-2">
                <HeroFactLink
                  label={`阻塞 ${pendingOwnerDecisions.length}`}
                  targetId="project-overview-pending"
                />
                <span aria-hidden className="text-ink-3">·</span>
                <HeroFactLink
                  label={`执行中 ${activeTasks.length}`}
                  targetId="project-overview-execution"
                />
                {latestDemand ? (
                  <>
                    <span aria-hidden className="text-ink-3">·</span>
                    <span className="max-w-56 truncate">
                      需求 {latestDemand.title}
                    </span>
                  </>
                ) : null}
              </div>
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            {isArchived ? (
              <Button disabled type="button">
                <FileText data-icon="inline-start" />
                提交需求
              </Button>
            ) : (
              <Button asChild>
                <Link
                  search={{ mode: "plan", project: project.id }}
                  to="/task-launches"
                >
                  <FileText data-icon="inline-start" />
                  提交需求
                </Link>
              </Button>
            )}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button
                  aria-label="更多项目操作"
                  size="icon"
                  type="button"
                  variant="outline"
                >
                  <MoreHorizontal />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem asChild>
                  <Link
                    params={{ projectId: project.id }}
                    to="/projects/$projectId/config"
                  >
                    <Settings2 />
                    配置项目
                  </Link>
                </DropdownMenuItem>
                {canArchive ? (
                  <DropdownMenuItem onSelect={onArchiveProject}>
                    <Archive />
                    归档项目
                  </DropdownMenuItem>
                ) : null}
                {showReclone ? (
                  <DropdownMenuItem
                    disabled={workspaceActionPending}
                    onSelect={onRecloneWorkspace}
                  >
                    重新 clone 工作区
                  </DropdownMenuItem>
                ) : null}
                {showMarkReady ? (
                  <DropdownMenuItem
                    disabled={workspaceActionPending}
                    onSelect={onMarkWorkspaceReady}
                  >
                    标记工作区已就绪
                  </DropdownMenuItem>
                ) : null}
                {canDelete ? (
                  <DropdownMenuItem variant="destructive" onSelect={onDeleteProject}>
                    <Trash2 />
                    删除项目
                  </DropdownMenuItem>
                ) : null}
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      </SoftCard>

      <SoftTabs
        className="grid min-w-0 gap-3"
        value={activeSection}
        onValueChange={(value) => setActiveSection(value as ProjectDetailSection)}
      >
        <div className="min-w-0 w-full border-b border-line">
          <SoftTabsList
            aria-label="项目工作区段"
            className="flex h-auto w-full min-w-0 max-w-none justify-start gap-0.5 overflow-x-auto rounded-none bg-transparent p-0 text-ink shadow-none"
          >
            <SoftTabsTrigger className={sectionTriggerClass} value="workbench">
              工作台
            </SoftTabsTrigger>
            <SoftTabsTrigger className={sectionTriggerClass} value="tasks">
              任务
            </SoftTabsTrigger>
            <SoftTabsTrigger className={sectionTriggerClass} value="approval">
              决策历史
            </SoftTabsTrigger>
            <SoftTabsTrigger className={sectionTriggerClass} value="assets">
              资产
            </SoftTabsTrigger>
          </SoftTabsList>
        </div>

        <SoftTabsContent className="m-0 grid min-w-0 gap-4" value="workbench">
          <ProjectOpsHome
            artifactsCount={artifacts?.length}
            budgetSummary={budgetSummary}
            decisionRequests={decisionRequests}
            demands={demands}
            events={events}
            isArchived={isArchived}
            onOpenTask={setDetailTaskId}
            onResolveDecision={onResolveDecision}
            onShowAllTasks={() => setActiveSection("tasks")}
            overview={overview}
            planRevisions={planRevisions}
            principalNamesById={principalNamesById}
            project={project}
            runtimePlacementPanel={runtimePlacementPanel}
            taskGraph={taskGraph}
            tasks={tasks}
          />

          <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
            <div
              className={cn(
                "overflow-hidden rounded-[12px] border border-dashed border-line",
                advancedOpen ? "border-solid bg-card shadow-card" : "bg-transparent",
              )}
            >
              <CollapsibleTrigger asChild>
                <button
                  aria-label={advancedOpen ? "收起高级项目事实" : "展开高级项目事实"}
                  className={cn(
                    "flex w-full items-center justify-between gap-3 px-3.5 py-2.5 text-left",
                    advancedOpen && "border-b border-line",
                  )}
                  type="button"
                >
                  <span className="min-w-0">
                    <span className="block text-[12.5px] font-semibold text-ink-2">
                      高级项目事实
                    </span>
                    {!advancedOpen ? (
                      <span className="mt-0.5 block text-[11px] text-ink-3">
                        计划图 · 需求 · 执行记录 · 治理
                      </span>
                    ) : null}
                  </span>
                  <span className="flex shrink-0 items-center gap-1.5 text-[11.5px] font-semibold text-ink-3">
                    {advancedOpen ? "收起" : "展开"}
                    <ChevronDown
                      className={cn("size-3.5 transition-transform", advancedOpen && "rotate-180")}
                    />
                  </span>
                </button>
              </CollapsibleTrigger>
              <CollapsibleContent>
                <MasterDetailLayout
                  className="p-4"
                  narrowDetail="stack"
                  rail="md"
                  master={
                    <section className="grid min-w-0 gap-4">
                      {taskGraph && taskGraph.nodes.length > 0 ? (
                        <section className="grid gap-2" data-testid="project-plan-graph-section">
                          <div className="flex items-center gap-2 px-1">
                            <ClipboardList className="size-4 text-ink-2" />
                            <h3 className="text-sm font-semibold tracking-normal">当前执行图</h3>
                          </div>
                          <Suspense fallback={<LoadingState />}>
                            <FlowGraphCanvas
                              graph={taskGraph}
                              onNodeOpen={(nodeId) => {
                                const taskId = taskIdFromNodeId(nodeId);
                                if (taskId) setDetailTaskId(taskId);
                              }}
                              onSelectedNodeChange={(nodeId) => {
                                if (!nodeId) setDetailTaskId(undefined);
                              }}
                              selectedNodeId={
                                detailTaskId ? taskNodeId(detailTaskId) : undefined
                              }
                            />
                          </Suspense>
                        </section>
                      ) : null}
                      <DispatchGateSummary
                        gates={dispatchGates ?? []}
                        taskTitle={dispatchGateTaskTitle}
                      />
                      <AdvancedRouteDecisions routeDecisions={routeDecisions} />
                      <div className="scroll-mt-20" id="project-execution-trace">
                        <ProjectExecutionTracePanel
                          errorMessage={executionTraceErrorMessage}
                          isError={executionTraceIsError}
                          isLoading={executionTraceIsLoading}
                          onRetry={onRetryExecutionTrace}
                          trace={executionTrace}
                        />
                      </div>
                      <ProjectGovernanceTabs
                        acceptance={acceptance}
                        archivePreview={archivePreview}
                        archiveSnapshots={archiveSnapshots}
                        artifacts={artifacts}
                        budgetLedger={budgetLedger}
                        budgetSummary={budgetSummary}
                        decisionRequestCount={decisionRequests.length}
                        demandCount={demands.length}
                        evidence={evidence}
                        executionSummaryCount={executionSummaries.length}
                        onCreateArchiveSnapshot={onCreateArchiveSnapshot}
                        onCreateEvidence={onCreateEvidence}
                        onPatchEvidence={onPatchEvidence}
                        reports={reports}
                        routeDecisionCount={routeDecisions.length}
                        taskCount={tasks.length}
                      />
                    </section>
                  }
                  detail={
                    <aside className="grid min-w-0 gap-4">
                      <AdvancedCoordinationJobs coordinationJobs={coordinationJobs} />
                      <AdvancedExecutionSummaries executionSummaries={executionSummaries} />
                      <AdvancedTransferRequests transferRequests={transferRequests} />
                      <AdvancedWorkflow project={project} overview={overview} />
                      <AdvancedDemands
                        blockingFact={taskGraph?.blocking_facts?.[0]}
                        demands={demands}
                      />
                    </aside>
                  }
                />
              </CollapsibleContent>
            </div>
          </Collapsible>
        </SoftTabsContent>

        <SoftTabsContent className="m-0" value="tasks">
          <ProjectTasksPanel
            decisionRequests={decisionRequests}
            dismissTaskPending={dismissTaskPending}
            onDismissTask={onDismissTask}
            onOpenTask={setDetailTaskId}
            principalNamesById={principalNamesById}
            tasks={tasks}
          />
        </SoftTabsContent>

        <SoftTabsContent className="m-0 grid min-w-0 gap-4" value="approval">

                      <SoftCard className="overflow-hidden scroll-mt-20" id="project-overview-plan">
                        <PanelHeader
                          icon={<GitBranch />}
                          title="计划确认"
                          meta={
                            latestPlanRevision
                              ? `v${latestPlanRevision.revision_number}`
                              : "暂无版本"
                          }
                        />
                        {latestPlanRevision ? (
                          <div className="grid gap-4 p-4">
                            <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
                              <div className="min-w-0">
                                <div className="flex flex-wrap items-center gap-2">
                                  <StatusPill tone={planRevisionTone(latestPlanRevision.status)}>
                                    {statusLabel(latestPlanRevision.status)}
                                  </StatusPill>
                                  {latestPlanRevision.review_required ? (
                                    <StatusPill tone="warn">需人工复核</StatusPill>
                                  ) : (
                                    <StatusPill tone="ok">自动接受</StatusPill>
                                  )}
                                </div>
                                <p className="mt-2 line-clamp-2 text-sm text-ink-2">
                                  {planRevisionSummary(latestPlanRevision)}
                                </p>
                              </div>
                              {latestPlanReviewDecision ? (
                                <div
                                  className="flex shrink-0 flex-col items-end gap-2"
                                  data-testid="plan-review-inbox-only"
                                >
                                  <StatusPill tone="warn">待收件箱处理</StatusPill>
                                  <p className="max-w-[220px] text-right text-[11px] leading-4 text-ink-3">
                                    计划确认请在收件箱处理（§6.3 决策历史只读）
                                  </p>
                                </div>
                              ) : null}
                            </div>
                            {planRevisionHasBoundTemplate(latestPlanRevision) &&
                            planRevisionTemplateKey(latestPlanRevision) ? (
                              <RuntimeMeta
                                label="场景模板"
                                value={planRevisionTemplateKey(latestPlanRevision)}
                              />
                            ) : null}
                            {planRevisionHasBoundTemplate(latestPlanRevision) &&
                            planRevisionExitLabel(latestPlanRevision) ? (
                              <RuntimeMeta
                                label="交付出口"
                                value={planRevisionExitLabel(latestPlanRevision)}
                              />
                            ) : null}
                            <div className="grid gap-3 md:grid-cols-3">
                              <FactTile
                                icon={<ClipboardList />}
                                label="计划任务"
                                value={`${planRevisionTasks(latestPlanRevision).length} 项`}
                              />
                              <FactTile
                                icon={<Bot />}
                                label="能力需求"
                                value={formatShortList(planRevisionCapabilityLabels(latestPlanRevision))}
                              />
                              <FactTile
                                icon={<FileCheck2 />}
                                label="风险等级"
                                value={formatShortList(planRevisionRiskLabels(latestPlanRevision))}
                              />
                            </div>
                            <div className="grid gap-3">
                              <div className="grid gap-2" data-testid="plan-dispatch-order">
                                <div className="flex items-center gap-2 px-1">
                                  <ClipboardList className="size-4 text-ink-2" />
                                  <h4 className="text-sm font-semibold text-ink">调度顺序</h4>
                                </div>
                                <div className="divide-y divide-line rounded-inner border border-line">
                                  {planRevisionTasksInDispatchOrder(latestPlanRevision).map(
                                    (task, index) => (
                                      <div
                                        className="grid gap-2 p-3"
                                        key={`${planRevisionTaskKey(task)}-${index}`}
                                      >
                                        <div className="flex items-start justify-between gap-3">
                                          <p className="min-w-0 text-sm font-medium text-ink">
                                            {planRevisionTaskTitle(task)}
                                          </p>
                                          <StatusPill tone="info">第 {index + 1} 步</StatusPill>
                                        </div>
                                        <RuntimeMeta
                                          label="执行员工"
                                          value={planRevisionTaskEmployee(
                                            task,
                                            servicePool,
                                            principalNamesById,
                                          )}
                                        />
                                        <RuntimeMeta
                                          label="选择原因"
                                          value={
                                            stringField(task, "employee_selection_reason") ||
                                            "未说明选择原因"
                                          }
                                        />
                                      </div>
                                    ),
                                  )}
                                  {planRevisionTasks(latestPlanRevision).length === 0 ? (
                                    <EmptyLine label="计划版本尚未包含可展示任务" />
                                  ) : null}
                                </div>
                              </div>

                              <div className="grid gap-2" data-testid="plan-acceptance-criteria">
                                <div className="flex items-center gap-2 px-1">
                                  <FileCheck2 className="size-4 text-ink-2" />
                                  <h4 className="text-sm font-semibold text-ink">验收判据</h4>
                                </div>
                                <div className="divide-y divide-line rounded-inner border border-line">
                                  {planRevisionAcceptanceCriteria(latestPlanRevision).map(
                                    (criterion, index) => {
                                      const criterionId =
                                        stringField(criterion, "id") || `criterion-${index}`;
                                      const method =
                                        planAcceptanceCriterionVerificationMethod(criterion);
                                      const severity = planAcceptanceCriterionSeverity(criterion);
                                      const isAmbiguous =
                                        planAcceptanceCriterionAmbiguityFlag(criterion);
                                      const evidenceHint =
                                        planAcceptanceCriterionEvidenceHint(criterion);

                                      return (
                                        <div className="grid gap-2 p-3" key={`${criterionId}-${index}`}>
                                          <div className="flex flex-wrap items-center gap-1.5">
                                            <StatusPill
                                              data-testid={`plan-acceptance-criterion-method-${criterionId}`}
                                              showDot={false}
                                              tone={method === "human_judgment" ? "info" : "mute"}
                                            >
                                              {method === "human_judgment" ? "人类判定" : "自动验证"}
                                            </StatusPill>
                                            {severity === "non_blocking" ? (
                                              <StatusPill
                                                data-testid={`plan-acceptance-criterion-severity-${criterionId}`}
                                                showDot={false}
                                                tone="mute"
                                              >
                                                非阻断
                                              </StatusPill>
                                            ) : null}
                                          </div>
                                          <PlanAcceptanceCriterionStatement
                                            criterionId={criterionId}
                                            statement={stringField(criterion, "statement")}
                                          />
                                          {isAmbiguous ? (
                                            <p
                                              className="flex items-center gap-1.5 text-xs font-medium text-warn-text"
                                              data-testid={`plan-acceptance-criterion-ambiguity-${criterionId}`}
                                            >
                                              <AlertTriangle className="size-3.5 shrink-0" />
                                              断言可能不可判定，请改写后再批准
                                            </p>
                                          ) : null}
                                          {evidenceHint ? (
                                            <p
                                              className="text-xs text-ink-3"
                                              data-testid={`plan-acceptance-criterion-evidence-hint-${criterionId}`}
                                            >
                                              证据提示：{evidenceHint}
                                            </p>
                                          ) : null}
                                          <RuntimeMeta
                                            label="满足任务"
                                            value={planRevisionCriterionSatisfiedLabel(
                                              criterion,
                                              latestPlanRevision,
                                            )}
                                          />
                                        </div>
                                      );
                                    },
                                  )}
                                  {planRevisionAcceptanceCriteria(latestPlanRevision).length === 0 ? (
                                    <EmptyLine label="本计划未声明验收判据" />
                                  ) : null}
                                </div>
                              </div>

                              {planRevisionConstraintNotes(latestPlanRevision).length > 0 ? (
                                <div className="grid gap-2" data-testid="plan-constraint-notes">
                                  <div className="flex items-center gap-2 px-1">
                                    <FileCheck2 className="size-4 text-ink-2" />
                                    <h4 className="text-sm font-semibold text-ink">约束说明</h4>
                                  </div>
                                  <div className="divide-y divide-line rounded-inner border border-line">
                                    {planRevisionConstraintNotes(latestPlanRevision).map(
                                      (note, index) => (
                                        <div
                                          className="flex items-start gap-2 p-3"
                                          key={`${note.kind}-${index}`}
                                        >
                                          <StatusPill tone={constraintNoteTone(note.kind)}>
                                            {constraintNoteKindLabel(note.kind)}
                                          </StatusPill>
                                          <p className="min-w-0 flex-1 text-xs text-ink-2">
                                            {note.message}
                                          </p>
                                        </div>
                                      ),
                                    )}
                                  </div>
                                </div>
                              ) : null}
                            </div>
                          </div>
                        ) : (
                          <EmptyLine label="暂无计划，提交需求后由系统生成下一步计划。" />
                        )}
                      </SoftCard>

          <ProjectApprovalPanel
            decisionRequests={decisionRequests}
            focusDecisionId={focusDecisionId}
          />
        </SoftTabsContent>

        <SoftTabsContent className="m-0" value="assets">
          <ProjectAssetsPanel
            acceptance={acceptance}
            artifacts={artifacts}
            budgetLedger={budgetLedger}
            budgetSummary={budgetSummary}
            initialTab={assetsInitial}
            reports={reports}
          />
        </SoftTabsContent>
      </SoftTabs>

      <ProjectTaskDetailDialog
        decisionRequests={decisionRequests}
        demands={demands}
        fetchTaskGraph={fetchTaskGraph}
        onOpenChange={(open) => {
          if (!open) setDetailTaskId(undefined);
        }}
        onResolveDecision={onResolveDecision}
        overview={overview}
        principalNamesById={principalNamesById}
        projectId={project.id}
        taskGraph={taskGraph}
        taskId={detailTaskId}
        tasks={tasks}
      />
    </div>
  );
}

function DispatchGateSummary({
  gates,
  taskTitle
}: {
  gates: DispatchGateResult[];
  taskTitle?: string;
}) {
  const latest = gates[0];
  if (!latest) {
    return null;
  }
  return (
    <SoftCard className="p-4">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-sm font-semibold text-ink">Dispatch gate 技术详情</h3>
          {taskTitle ? (
            <p className="mt-1 truncate text-xs text-ink-2">{taskTitle}</p>
          ) : null}
        </div>
        <StatusPill tone={dispatchGateTone(latest.status)}>
          {dispatchGateStatusLabel(latest.status)}
        </StatusPill>
      </div>
      {latest.blockers.length > 0 ? (
        <ul className="mt-3 space-y-2">
          {latest.blockers.map((blocker) => (
            <li
              className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-sm text-ink-2"
              key={`${latest.id}-${blocker.key}`}
            >
              <span className="min-w-0 break-all font-mono text-xs text-ink">
                {blocker.key}
              </span>
              <span className="text-xs">
                {blocker.retryable ? "可重试" : statusLabel(blocker.severity)}
              </span>
            </li>
          ))}
        </ul>
      ) : null}
      {latest.retry_after ? (
        <p className="mt-2 text-xs text-ink-2">
          {`下次重试 ${formatDateTime(latest.retry_after)}`}
        </p>
      ) : null}
    </SoftCard>
  );
}

function AdvancedRouteDecisions({
  routeDecisions
}: {
  routeDecisions: ProjectRouteDecision[];
}) {
  return (
    <SoftCard className="overflow-hidden">
      <PanelHeader
        icon={<GitBranch />}
        title="路由决策"
        meta={`${routeDecisions.length} 条`}
      />
      <div className="divide-y divide-line">
        {routeDecisions.length === 0 ? (
          <EmptyLine label="暂无路由决策" />
        ) : (
          routeDecisions.slice(0, 5).map((decision) => (
            <div className="grid gap-2 p-4" key={decision.id}>
              <div className="flex items-start justify-between gap-3">
                <p className="min-w-0 line-clamp-2 text-sm font-medium">
                  {decision.reason}
                </p>
                {decision.requires_human_review ? (
                  <StatusPill tone="warn">需人工复核</StatusPill>
                ) : (
                  <StatusPill tone="ok">已规划</StatusPill>
                )}
              </div>
              <RuntimeMeta
                label="已选数字员工"
                value={formatIdList(decision.selected_digital_employee_ids)}
              />
              <RuntimeMeta
                label="候选数字员工"
                value={formatIdList(decision.candidate_digital_employee_ids)}
              />
            </div>
          ))
        )}
      </div>
    </SoftCard>
  );
}

function AdvancedCoordinationJobs({
  coordinationJobs
}: {
  coordinationJobs: ProjectCoordinationJob[];
}) {
  return (
    <SoftCard className="overflow-hidden">
      <PanelHeader
        icon={<GitBranch />}
        title="协调任务"
        meta={`${coordinationJobs.length} 条`}
      />
      <div className="divide-y divide-line">
        {coordinationJobs.length === 0 ? (
          <EmptyLine label="暂无协调任务" />
        ) : (
          coordinationJobs.slice(0, 4).map((job) => (
            <div className="grid gap-2 p-4" key={job.id}>
              <div className="flex items-center justify-between gap-3">
                <p className="min-w-0 truncate text-sm font-medium">
                  {job.job_type}
                </p>
                <StatusPill tone={jobTone(job.status)}>{statusLabel(job.status)}</StatusPill>
              </div>
              <p className="truncate text-xs text-ink-2">
                {job.workflow_id}
              </p>
            </div>
          ))
        )}
      </div>
    </SoftCard>
  );
}

function AdvancedExecutionSummaries({
  executionSummaries
}: {
  executionSummaries: ProjectExecutionSummary[];
}) {
  return (
    <SoftCard className="overflow-hidden">
      <PanelHeader
        icon={<FileCheck2 />}
        title="执行摘要"
        meta={`${executionSummaries.length} 条`}
      />
      <div className="divide-y divide-line">
        {executionSummaries.length === 0 ? (
          <EmptyLine label="暂无执行回写摘要" />
        ) : (
          executionSummaries.slice(0, 4).map((summary) => (
            <div className="grid gap-2 p-4" key={summary.id}>
              <div className="flex items-start justify-between gap-3">
                <p className="min-w-0 line-clamp-2 text-sm font-medium">
                  {summary.conclusion}
                </p>
                {summary.requires_human_review ? (
                  <StatusPill tone="warn">需复核</StatusPill>
                ) : (
                  <StatusPill tone="ok">已回写</StatusPill>
                )}
              </div>
              <RuntimeMeta
                label="执行员工"
                value={summary.digital_employee_id}
              />
              {summary.recommended_next_action ? (
                <p className="line-clamp-2 text-xs text-ink-2">
                  {summary.recommended_next_action}
                </p>
              ) : null}
            </div>
          ))
        )}
      </div>
    </SoftCard>
  );
}

function AdvancedTransferRequests({
  transferRequests
}: {
  transferRequests: ProjectTransferRequest[];
}) {
  return (
    <SoftCard className="overflow-hidden">
      <PanelHeader
        icon={<Bot />}
        title="转派请求"
        meta={`${transferRequests.length} 条`}
      />
      <div className="divide-y divide-line">
        {transferRequests.length === 0 ? (
          <EmptyLine label="暂无转派请求" />
        ) : (
          transferRequests.slice(0, 4).map((request) => (
            <div className="grid gap-2 p-4" key={request.id}>
              <div className="flex items-start justify-between gap-3">
                <p className="min-w-0 line-clamp-2 text-sm font-medium">
                  {request.reason}
                </p>
                <StatusPill tone={requestTone(request.status)}>
                  {statusLabel(request.status)}
                </StatusPill>
              </div>
              <RuntimeMeta
                label="发起员工"
                value={request.requested_by_digital_employee_id}
              />
              <RuntimeMeta
                label="建议员工"
                value={formatIdList(request.suggested_digital_employee_ids)}
              />
            </div>
          ))
        )}
      </div>
    </SoftCard>
  );
}

function AdvancedWorkflow({
  project,
  overview
}: {
  project: Project;
  overview?: ProjectOverview;
}) {
  return (
    <SoftCard className="overflow-hidden">
      <PanelHeader
        icon={<GitBranch />}
        title="协调线程"
        meta={statusLabel(overview?.coordination_workflow.status || project.coordination_status)}
      />
      <div className="p-4">
        <p className="truncate text-sm font-medium">
          {project.coordination_workflow_id}
        </p>
        <p className="mt-1 text-xs text-ink-2">
          虚拟协调线程，仅作为项目 Workflow 元数据展示。
        </p>
      </div>
    </SoftCard>
  );
}

function AdvancedDemands({
  demands,
  blockingFact
}: {
  blockingFact?: ProjectTaskGraphBlockingFact;
  demands: ProjectDemand[];
}) {
  return (
    <SoftCard className="overflow-hidden">
      <PanelHeader
        icon={<FileText />}
        title="需求记录"
        meta={`${demands.length} 条`}
      />
      <div className="divide-y divide-line">
        {demands.length === 0 ? (
          <EmptyLine label="暂无提交到项目的需求" />
        ) : (
          demands.slice(0, 4).map((demand) => (
            <div className="grid gap-1 p-4" key={demand.id}>
              <div className="flex items-center justify-between gap-3">
                <p className="truncate text-sm font-medium">
                  {demand.title}
                </p>
                <StatusPill tone="mute">{demand.source_type}</StatusPill>
              </div>
              <p className="line-clamp-2 text-xs text-ink-2">
                {demand.content || "需求内容已记录"}
              </p>
              {demand.status === "failed" ? (
                <DemandFailureDiagnosis
                  demandId={demand.id}
                  fact={blockingFact}
                />
              ) : null}
            </div>
          ))
        )}
      </div>
    </SoftCard>
  );
}

function FactTile({
  icon,
  label,
  value
}: {
  icon: ReactNode;
  label: string;
  value: string;
}) {
  return (
    <div className="min-w-0 rounded-inner bg-card-soft p-3">
      <div className="flex items-center gap-2 text-xs text-ink-2">
        <IconTile tone="mute" size="sm" className="size-8 rounded-[10px] [&_svg]:size-3.5">
          {icon}
        </IconTile>
        {label}
      </div>
      <p className="mt-2 truncate text-sm font-semibold text-ink">{value}</p>
    </div>
  );
}

function HeroFactLink({
  className,
  label,
  targetId
}: {
  className?: string;
  label: string;
  targetId: string;
}) {
  return (
    <button
      className={cn(
        "rounded-sm font-medium underline-offset-2 transition-colors hover:text-ink hover:underline",
        className,
      )}
      type="button"
      onClick={() =>
        document
          .getElementById(targetId)
          ?.scrollIntoView({ behavior: "smooth", block: "start" })
      }
    >
      {label}
    </button>
  );
}

function PanelHeader({
  icon,
  meta,
  title
}: {
  icon: ReactNode;
  meta: string;
  title: string;
}) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-line p-4">
      <div className="flex items-center gap-2">
        <IconTile tone="brand" size="sm" className="size-8 rounded-[10px] [&_svg]:size-3.5">
          {icon}
        </IconTile>
        <h3 className="font-semibold text-ink">{title}</h3>
      </div>
      <StatusPill tone="mute" showDot={false}>{meta}</StatusPill>
    </div>
  );
}

export function MemberPanel({
  emptyLabel,
  icon,
  members,
  principalNamesById,
  title
}: {
  emptyLabel: string;
  icon: ReactNode;
  members: ProjectMember[];
  principalNamesById?: ReadonlyMap<string, string>;
  title: string;
}) {
  return (
    <SoftCard className="overflow-hidden">
      <PanelHeader icon={icon} title={title} meta={`${members.length} 个`} />
      <div className="divide-y divide-line">
        {members.length === 0 ? (
          <EmptyLine label={emptyLabel} />
        ) : (
          members
            .slice(0, 6)
            .map((member) => (
              <MemberRow
                key={member.id}
                member={member}
                principalNamesById={principalNamesById}
              />
            ))
        )}
      </div>
    </SoftCard>
  );
}

function MemberRow({
  member,
  principalNamesById
}: {
  member: ProjectMember;
  principalNamesById?: ReadonlyMap<string, string>;
}) {
  const name = resolvePrincipalLabel(
    member.principal_id,
    member.display_name_snapshot,
    principalNamesById,
  );
  const content = (
    <>
      <div className="min-w-0">
        <p className="truncate text-sm font-medium text-ink">{name}</p>
        <p className="truncate text-xs text-ink-2">
          {projectMemberBusinessLabel(member)}
        </p>
      </div>
      <ExternalLink className="size-3.5 text-ink-2" />
    </>
  );
  const href = projectMemberHref(member);

  if (!href) {
    return <div className="flex items-center justify-between gap-3 p-4">{content}</div>;
  }

  return (
    <Link className="flex items-center justify-between gap-3 p-4 hover:bg-card-soft" to={href}>
      {content}
    </Link>
  );
}

export function DemandTitle({ demand }: { demand: ProjectDemand }) {
  if (!demand.id) {
    return <p className="text-sm font-semibold text-ink">{demand.title}</p>;
  }

  return (
    <Link
      className="text-sm font-semibold text-brand-deep hover:text-brand"
      params={{ demandId: demand.id }}
      to="/workflows/$demandId"
    >
      {demand.title}
    </Link>
  );
}

/**
 * 失败需求的诊断摘要行：读取该需求 taskGraph 的第一条 blocking fact（已按 latestDemand.id
 * 取数，见调用方），给出诊断原因/下一步建议，并深链到 /workflows/{demandId} 的缺口处理面板
 * （规划缺口面板——补员/豁免/借调三个动作都在那）。没有 blocking fact 时仍给出深链，不留死胡同。
 */
function DemandFailureDiagnosis({
  demandId,
  fact
}: {
  demandId: string;
  fact?: ProjectTaskGraphBlockingFact;
}) {
  return (
    <div className="grid gap-1 rounded-[14px] border border-danger/25 bg-danger/5 p-3">
      <p className="text-xs leading-5 text-ink-2">
        {fact?.message ?? "规划已终止，需要人工处理。"}
      </p>
      {fact?.recommended_action ? (
        <p className="text-xs leading-5 text-ink-2">下一步：{fact.recommended_action}</p>
      ) : null}
      <Link
        className="text-xs font-semibold text-brand-deep hover:text-brand"
        params={{ demandId }}
        to="/workflows/$demandId"
      >
        查看缺口处理 →
      </Link>
    </div>
  );
}

function ProjectTaskLink({
  task,
  onOpen
}: {
  task: ProjectTask;
  onOpen?: (taskId: string) => void;
}) {
  if (onOpen) {
    return (
      <button
        className="block min-w-0 max-w-full truncate text-left text-sm font-medium text-brand-deep hover:text-brand"
        onClick={() => onOpen(task.id)}
        type="button"
      >
        {task.title}
      </button>
    );
  }

  if (task.assigned_digital_employee_id) {
    return (
      <Link
        className="min-w-0 truncate text-sm font-medium text-brand-deep hover:text-brand"
        params={{ employeeId: task.assigned_digital_employee_id }}
        to="/employees/$employeeId"
      >
        {task.title}
      </Link>
    );
  }

  return <p className="min-w-0 truncate text-sm font-medium">{task.title}</p>;
}

function ProjectTasksPanel({
  decisionRequests,
  dismissTaskPending,
  onDismissTask,
  onOpenTask,
  principalNamesById,
  tasks
}: {
  decisionRequests?: ProjectDecisionRequest[];
  dismissTaskPending?: boolean;
  onDismissTask?: (taskId: string) => void;
  onOpenTask?: (taskId: string) => void;
  principalNamesById?: ReadonlyMap<string, string>;
  tasks: ProjectTask[];
}) {
  const [statusFilter, setStatusFilter] = useState("all");
  const [employeeFilter, setEmployeeFilter] = useState("all");
  const visibleTasks = useMemo(
    () => tasks.filter((task) => !task.dismissed_at),
    [tasks],
  );
  const employeeIds = useMemo(
    () =>
      Array.from(
        new Set(
          visibleTasks
            .map((task) => task.assigned_digital_employee_id)
            .filter((value): value is string => Boolean(value)),
        ),
      ),
    [visibleTasks],
  );
  const statuses = useMemo(
    () => Array.from(new Set(visibleTasks.map((task) => task.status))).filter(Boolean),
    [visibleTasks],
  );
  const filteredTasks = useMemo(
    () =>
      visibleTasks
        .filter(
          (task) =>
            (statusFilter === "all" || task.status === statusFilter) &&
            (employeeFilter === "all" || task.assigned_digital_employee_id === employeeFilter),
        )
        .sort((left, right) =>
          compareIsoDesc(
            left.updated_at ?? left.created_at,
            right.updated_at ?? right.created_at,
          ),
        ),
    [employeeFilter, statusFilter, visibleTasks],
  );

  return (
    <WorkSurface className="min-w-0">
      <div className="flex flex-col gap-3 border-b border-line p-4 lg:flex-row lg:items-center lg:justify-between">
        <div className="min-w-0">
          <h3 className="text-sm font-semibold text-ink">项目任务</h3>
          <p className="mt-1 text-xs text-ink-2">
            基于当前 project tasks 数据展示状态、员工和项目上下文。
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <select
            aria-label="任务状态"
            className="h-9 rounded-inner border border-line bg-card px-3 text-xs font-semibold text-ink"
            value={statusFilter}
            onChange={(event) => setStatusFilter(event.target.value)}
          >
            <option value="all">全部状态</option>
            {statuses.map((status) => (
              <option key={status} value={status}>
                {taskStatusLabel(status)}
              </option>
            ))}
          </select>
          <select
            aria-label="执行员工"
            className="h-9 rounded-inner border border-line bg-card px-3 text-xs font-semibold text-ink"
            value={employeeFilter}
            onChange={(event) => setEmployeeFilter(event.target.value)}
          >
            <option value="all">全部员工</option>
            {employeeIds.map((employeeId) => (
              <option key={employeeId} value={employeeId}>
                {principalNamesById?.get(employeeId) ?? employeeId}
              </option>
            ))}
          </select>
        </div>
      </div>
      <DataTable>
        <thead>
          <tr>
            <Th className="min-w-[220px]">任务</Th>
            <Th>状态</Th>
            <Th>员工</Th>
            <Th>更新</Th>
            <Th>操作</Th>
          </tr>
        </thead>
        <tbody>
          {filteredTasks.length === 0 ? (
            <Tr>
              <Td colSpan={5}>
                <EmptyLine label="当前筛选下没有项目任务" />
              </Td>
            </Tr>
          ) : (
            filteredTasks.map((task) => {
              const activityAt = task.updated_at ?? task.created_at;
              const canDismiss =
                Boolean(onDismissTask) &&
                !task.dismissed_at &&
                (task.status === "failed" || task.status === "cancelled") &&
                !(decisionRequests ?? []).some(
                  (decision) =>
                    decision.project_task_id === task.id &&
                    ["pending", "requested", "waiting", "open"].includes(
                      (decision.status_snapshot ?? "").toLowerCase(),
                    ),
                );
              return (
              <Tr key={task.id}>
                <Td className="min-w-[220px]">
                  <ProjectTaskLink onOpen={onOpenTask} task={task} />
                  {task.summary ? (
                    <p className="mt-1 line-clamp-2 text-xs text-ink-2">{task.summary}</p>
                  ) : null}
                </Td>
                <Td>
                  <StatusPill tone="info">{taskStatusLabel(task.status)}</StatusPill>
                </Td>
                <Td className="text-xs text-ink-2">
                  {task.assigned_digital_employee_id ? (
                    <Link
                      className={cn(
                        "text-brand-deep hover:text-brand",
                        !principalNamesById?.get(task.assigned_digital_employee_id) &&
                          "font-mono",
                      )}
                      params={{ employeeId: task.assigned_digital_employee_id }}
                      to="/employees/$employeeId"
                    >
                      {principalNamesById?.get(task.assigned_digital_employee_id) ??
                        task.assigned_digital_employee_id}
                    </Link>
                  ) : (
                    "未分派"
                  )}
                </Td>
                <Td className="whitespace-nowrap tabular-nums text-xs text-ink-2">
                  {activityAt ? (
                    <time dateTime={activityAt} title={formatAbsoluteDateTime(activityAt)}>
                      {formatRelativeTime(activityAt)}
                    </time>
                  ) : (
                    "—"
                  )}
                </Td>
                <Td>
                  {canDismiss ? (
                    <Button
                      disabled={dismissTaskPending}
                      size="sm"
                      variant="outline"
                      onClick={() => {
                        if (
                          window.confirm(
                            `确认清理任务「${task.title}」？清理后不再出现在待处理与风险中，历史与审计仍保留。`,
                          )
                        ) {
                          onDismissTask?.(task.id);
                        }
                      }}
                    >
                      清理任务
                    </Button>
                  ) : (
                    <span className="text-xs text-ink-3">—</span>
                  )}
                </Td>
              </Tr>
              );
            })
          )}
        </tbody>
      </DataTable>
    </WorkSurface>
  );
}

function ProjectApprovalPanel({
  decisionRequests,
  focusDecisionId
}: {
  decisionRequests: ProjectDecisionRequest[];
  focusDecisionId?: string;
}) {
  const orderedDecisions = useMemo(
    () =>
      [...decisionRequests].sort((left, right) =>
        compareIsoDesc(left.created_at, right.created_at),
      ),
    [decisionRequests],
  );

  return (
    <WorkSurface className="min-w-0">
      <div className="border-b border-line p-4">
        <h3 className="text-sm font-semibold text-ink">决策历史</h3>
        <p className="mt-1 text-xs text-ink-2">
          只读汇总本项目决策记录；待办请在收件箱处理。
        </p>
      </div>
      <div className="divide-y divide-line">
        {orderedDecisions.length === 0 ? (
          <EmptyLine label="当前项目没有决策记录" />
        ) : (
          orderedDecisions.map((decision) => {
            const isResolved =
              decision.status_snapshot === "approved" ||
              decision.status_snapshot === "rejected" ||
              decision.status_snapshot === "needs_more_evidence" ||
              Boolean(decision.resolved_at);
            const timeLabel = isResolved && decision.resolved_at ? "决议" : "创建";
            const timeValue = isResolved && decision.resolved_at
              ? decision.resolved_at
              : decision.created_at;
            return (
            <div
              className={cn(
                "grid gap-3 p-4",
                decision.id === focusDecisionId && "bg-brand-soft shadow-[inset_3px_0_0_var(--brand)]",
              )}
              data-focused-decision={decision.id === focusDecisionId ? "true" : undefined}
              id={`decision-${decision.id}`}
              key={decision.id}
            >
              <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
                <div className="min-w-0">
                  <p className="text-sm font-semibold text-ink">{decision.title_snapshot}</p>
                  <p className="mt-1 line-clamp-3 text-xs leading-5 text-ink-2">
                    {decision.summary_snapshot && decision.summary_snapshot !== decision.title_snapshot
                      ? decision.summary_snapshot
                      : isResolved
                        ? "已处理"
                        : "待收件箱处理"}
                  </p>
                  <div className="mt-1 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[11px] text-ink-3">
                    {timeValue ? (
                      <time
                        className="tabular-nums"
                        dateTime={timeValue}
                        title={formatAbsoluteDateTime(timeValue)}
                      >
                        {timeLabel} {formatRelativeTime(timeValue)}
                      </time>
                    ) : null}
                    <span className="font-mono">{decision.id}</span>
                  </div>
                </div>
                <StatusPill tone={decisionTone(decision.status_snapshot)}>
                  {decisionStatusLabel(decision.status_snapshot)}
                </StatusPill>
              </div>
            </div>
            );
          })
        )}
      </div>
    </WorkSurface>
  );
}

function EmptyLine({ action, label }: { action?: ReactNode; label: string }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 px-4 py-4 text-sm text-ink-2">
      <span>{label}</span>
      {action}
    </div>
  );
}

const PLAN_ACCEPTANCE_STATEMENT_EXPAND_THRESHOLD = 80;

function PlanAcceptanceCriterionStatement({
  criterionId,
  statement
}: {
  criterionId: string;
  statement: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const text = statement.trim() || "未声明验收说明";
  const isExpandable = text.length > PLAN_ACCEPTANCE_STATEMENT_EXPAND_THRESHOLD;

  return (
    <div className="grid gap-1">
      <p
        className={cn(
          "break-words text-sm leading-6 text-ink",
          isExpandable && !expanded && "line-clamp-3",
        )}
        data-testid={`plan-acceptance-criterion-statement-${criterionId}`}
      >
        {text}
      </p>
      {isExpandable ? (
        <Button
          aria-label={
            expanded ? `收起验收判据 ${criterionId}` : `展开验收判据 ${criterionId}`
          }
          className="h-auto self-start px-0 py-0"
          size="sm"
          type="button"
          variant="ghost"
          onClick={() => setExpanded((value) => !value)}
        >
          {expanded ? "收起" : "展开"}
        </Button>
      ) : null}
    </div>
  );
}

function RuntimeMeta({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex min-w-0 items-center justify-between gap-3 text-xs">
      <span className="shrink-0 text-ink-2">{label}</span>
      <span className="min-w-0 truncate font-medium text-ink">{value}</span>
    </div>
  );
}

function formatIdList(ids: string[]) {
  return ids.length > 0 ? ids.join("、") : "未指定";
}

function projectMemberBusinessLabel(member: ProjectMember) {
  if (member.principal_type === "digital_employee") {
    const sourceTeam = stringFromUnknown(member.settings?.source_team_name);
    return sourceTeam ? `数字员工 · ${sourceTeam}` : "数字员工";
  }
  if (member.project_role === "owner") {
    return "项目负责人";
  }
  return "项目参与人";
}

function projectMemberHref(member: ProjectMember) {
  if (member.principal_type === "digital_employee") {
    return `/employees/${encodeURIComponent(member.principal_id)}`;
  }
  if (member.principal_type === "human_user") {
    return "/users";
  }
  return undefined;
}

export function projectEventDisplay(event: ProjectEvent) {
  const labels: Record<string, { summary: string; title: string }> = {
    "coordination_job.created": {
      summary: "系统已开始推进下一步项目工作。",
      title: "项目推进已启动"
},
    "decision.requested": {
      summary: "有事项需要项目负责人处理。",
      title: "等待负责人处理"
},
    "decision.submitted": {
      summary: "负责人处理结果已记录。",
      title: "负责人已处理"
},
    "demand.submitted": {
      summary: "新的项目需求已进入处理队列。",
      title: "需求已提交"
},
    "project.acceptance.submitted": {
      summary: "项目验收结论已提交。",
      title: "验收结论已提交"
},
    "project.archive.retention_pending": {
      summary: "项目归档前仍有保留事项待处理。",
      title: "归档保留事项待处理"
},
    "project.archive_snapshot.created": {
      summary: "项目归档快照已生成。",
      title: "归档快照已生成"
},
    "project.archived": {
      summary: "项目已归档关闭。",
      title: "项目已归档"
},
    "project.artifact.linked": {
      summary: "新的项目工件已关联。",
      title: "工件已关联"
},
    "project.budget.recorded": {
      summary: "项目预算记录已更新。",
      title: "预算记录已更新"
},
    "project.config.changed": {
      summary: "项目配置已更新。",
      title: "配置已更新"
},
    "project.created": {
      summary: "项目已创建。",
      title: "项目已创建"
},
    "project.evidence.linked": {
      summary: "新的项目证据已关联。",
      title: "证据已关联"
},
    "project.evidence.verified": {
      summary: "项目证据已完成核验。",
      title: "证据已核验"
},
    "project.report.linked": {
      summary: "项目报告已关联。",
      title: "报告已关联"
},
    "project_task.completed": {
      summary: "数字员工任务已完成并回写结果。",
      title: "执行任务已完成"
},
    "project_task.created": {
      summary: "新的执行任务已进入项目推进队列。",
      title: "执行任务已创建"
},
    "project_task.dispatched": {
      summary: "系统已安排数字员工执行任务。",
      title: "执行任务已分派"
},
    "project_task.dispatch_gate.blocked": {
      summary: "当前执行条件未满足，系统已记录阻塞原因。",
      title: "执行条件未满足"
},
    "project_task.dispatch_gate.checked": {
      summary: "系统已检查任务执行条件。",
      title: "执行条件已检查"
},
    "project_task.dispatch_gate.replan_required": {
      summary: "当前计划需要调整后继续推进。",
      title: "计划需要调整"
},
    "project_task.dispatch_gate.retry_later": {
      summary: "运行条件暂不可用，系统会稍后重试。",
      title: "稍后重试执行"
},
    "project_task.dispatch_gate.waiting_human": {
      summary: "当前执行需要负责人确认。",
      title: "等待负责人确认"
},
    "project_task.failed": {
      summary: "数字员工任务执行失败，系统已记录原因。",
      title: "执行任务失败"
},
    "route_decision.created": {
      summary: "系统已生成任务分派方案。",
      title: "任务分派方案已生成"
},
    "transfer.requested": {
      summary: "项目执行需要调整服务员工。",
      title: "服务员工调整待处理"
},
    "workflow.signaled": {
      summary: "项目推进状态已更新。",
      title: "项目推进已更新"
}
};
  const label = labels[event.event_type] ?? {
    summary: "项目状态已有新记录。",
    title: "项目动态已更新"
};
  return {
    resource: event.resource_id ? `项目对象 · ${shortIdentifier(event.resource_id)}` : undefined,
    summary: label.summary,
    title: label.title
};
}

export function projectEventActorLabel(
  event: ProjectEvent,
  principalNamesById?: ReadonlyMap<string, string>,
) {
  if (
    event.actor_type === "human_user" ||
    event.actor_type === "digital_employee"
  ) {
    // 无法补名时不显示，避免裸 UUID 出现在用户可见文本里。
    return resolvePrincipalName(event.actor_id, undefined, principalNamesById);
  }
  return "系统";
}

export function eventSummaryDuplicatesTitle(title: string, summary: string) {
  const normalize = (value: string) => value.trim().replace(/[。.!！]+$/u, "");
  return normalize(summary) === normalize(title);
}

function shortIdentifier(value: string) {
  return value.length > 12 ? `${value.slice(0, 8)}...${value.slice(-4)}` : value;
}

function stringFromUnknown(value: unknown) {
  return typeof value === "string" && value.trim() ? value : "";
}

function ownerMembers(members: ProjectMember[], fallbackOwnerID: string) {
  const owners = members.filter(
    (member) =>
      member.principal_type === "human_user" &&
      member.status === "active" &&
      (member.project_role === "owner" || member.principal_id === fallbackOwnerID),
  );
  if (owners.length > 0) {
    return owners;
  }
  if (!fallbackOwnerID) {
    return [];
  }
  return [
    {
      id: `owner-${fallbackOwnerID}`,
      principal_id: fallbackOwnerID,
      principal_type: "human_user" as const,
      project_id: "",
      project_role: "owner" as const,
      settings: {},
      status: "active",
      tenant_id: ""
},
  ];
}

function projectStatusTone(status: ProjectStatus | string): Tone {
  if (status === "running") return "ok";
  if (status === "archived") return "mute";
  if (status === "paused" || status === "acceptance") return "warn";
  if (status === "configuring" || status === "draft") return "info";
  return "mute";
}

function workspaceReadyTone(status: WorkspaceReadyStatus | string | undefined): Tone {
  if (status === "ready") return "ok";
  if (status === "error") return "danger";
  if (status === "pending") return "warn";
  return "mute";
}

function selectLatestPlanRevision(revisions: ProjectPlanRevision[]) {
  return [...revisions].sort((left, right) => {
    if (right.revision_number !== left.revision_number) {
      return right.revision_number - left.revision_number;
    }
    return (right.created_at ?? "").localeCompare(left.created_at ?? "");
  })[0];
}

function planRevisionSummary(revision: ProjectPlanRevision) {
  const summary = revision.payload.summary;
  return typeof summary === "string" && summary.trim()
    ? summary
    : "计划版本已生成，等待协调线程处理。";
}

function planRevisionTasks(revision: ProjectPlanRevision) {
  const tasks = revision.payload.tasks;
  return Array.isArray(tasks) ? tasks.filter(isRecord) : [];
}

function planRevisionTasksInDispatchOrder(revision: ProjectPlanRevision) {
  const tasks = planRevisionTasks(revision);
  const taskKeys = new Set(tasks.map(planRevisionTaskKey));
  const pending = [...tasks];
  const completed = new Set<string>();
  const ordered: Record<string, unknown>[] = [];

  while (pending.length > 0) {
    const nextTaskIndex = pending.findIndex((task) =>
      planRevisionTaskDependencies(task).every(
        (dependency) => !taskKeys.has(dependency) || completed.has(dependency),
      ),
    );

    if (nextTaskIndex === -1) {
      return [...ordered, ...pending];
    }

    const [nextTask] = pending.splice(nextTaskIndex, 1);
    ordered.push(nextTask);
    completed.add(planRevisionTaskKey(nextTask));
  }

  return ordered;
}

function planRevisionAcceptanceCriteria(revision: ProjectPlanRevision) {
  const criteria = revision.payload.plan_acceptance_criteria;
  return Array.isArray(criteria) ? criteria.filter(isRecord) : [];
}

function planRevisionCriterionTaskTitles(
  criterion: Record<string, unknown>,
  revision: ProjectPlanRevision,
) {
  const taskTitlesByKey = new Map(
    planRevisionTasks(revision).map((task) => [
      planRevisionTaskKey(task),
      planRevisionTaskTitle(task),
    ]),
  );

  return stringArrayField(criterion, "satisfied_by").map(
    (taskKey) => taskTitlesByKey.get(taskKey) ?? taskKey,
  );
}

function planAcceptanceCriterionVerificationMethod(
  criterion: Record<string, unknown>,
): "automated_test" | "human_judgment" {
  return stringField(criterion, "verification_method") === "human_judgment"
    ? "human_judgment"
    : "automated_test";
}

function planAcceptanceCriterionSeverity(
  criterion: Record<string, unknown>,
): "blocking" | "non_blocking" {
  return stringField(criterion, "severity") === "non_blocking" ? "non_blocking" : "blocking";
}

function planAcceptanceCriterionAmbiguityFlag(criterion: Record<string, unknown>): boolean {
  return criterion.ambiguity_flag === true;
}

function planAcceptanceCriterionEvidenceHint(criterion: Record<string, unknown>): string {
  return stringField(criterion, "evidence_hint");
}

function planRevisionCriterionSatisfiedLabel(
  criterion: Record<string, unknown>,
  revision: ProjectPlanRevision,
): string {
  const titles = planRevisionCriterionTaskTitles(criterion, revision);
  if (titles.length > 0) {
    return titles.join("、");
  }
  return planAcceptanceCriterionVerificationMethod(criterion) === "human_judgment"
    ? "需求级人类判定"
    : "";
}

function planRevisionCapabilityLabels(revision: ProjectPlanRevision) {
  return uniqueStrings(
    planRevisionTasks(revision).flatMap((task) => [
      ...stringArrayField(task, "required_capabilities"),
      ...stringArrayField(task, "matched_capabilities"),
    ]),
  ).slice(0, 4);
}

function planRevisionRiskLabels(revision: ProjectPlanRevision) {
  const riskAssessment = revision.payload.risk_assessment;
  const highest =
    isRecord(riskAssessment) && typeof riskAssessment.highest_risk_level === "string"
      ? riskAssessment.highest_risk_level
      : "";
  const taskRisks = uniqueStrings(
    planRevisionTasks(revision)
      .map((task) => stringField(task, "risk_level"))
      .filter(Boolean),
  );
  return uniqueStrings([highest, ...taskRisks].filter(Boolean)).slice(0, 3);
}

function planRevisionTaskDependencies(task: Record<string, unknown>) {
  const dependsOn = stringArrayField(task, "depends_on");
  if (dependsOn.length > 0) {
    return dependsOn;
  }
  return stringArrayField(task, "blocked_by_keys");
}

function planRevisionTaskKey(task: Record<string, unknown>) {
  return stringField(task, "planned_task_key") || stringField(task, "title") || "task";
}

function planRevisionTaskTitle(task: Record<string, unknown>) {
  return stringField(task, "title") || stringField(task, "objective") || "未命名任务";
}

function planRevisionTaskEmployee(
  task: Record<string, unknown>,
  members: ProjectMember[],
  principalNamesById?: ReadonlyMap<string, string>,
) {
  const employeeID = stringField(task, "selected_employee_id");
  if (!employeeID) {
    return "未指定";
  }
  const snapshot = members.find(
    (member) => member.principal_id === employeeID,
  )?.display_name_snapshot;
  return resolvePrincipalLabel(employeeID, snapshot, principalNamesById);
}

function resolvePrincipalName(
  id: string | undefined | null,
  snapshot?: string | null,
  principalNamesById?: ReadonlyMap<string, string>,
): string | undefined {
  const fromSnapshot = snapshot?.trim();
  if (fromSnapshot) {
    return fromSnapshot;
  }
  if (!id) {
    return undefined;
  }
  return principalNamesById?.get(id.trim())?.trim() || undefined;
}

function resolvePrincipalLabel(
  id: string | undefined | null,
  snapshot?: string | null,
  principalNamesById?: ReadonlyMap<string, string>,
): string {
  return resolvePrincipalName(id, snapshot, principalNamesById) || id?.trim() || "未指定";
}

function planRevisionTone(status: string): Tone {
  if (status === "accepted" || status === "decomposed") {
    return "ok";
  }
  if (status === "pending_review" || status === "decomposing") {
    return "warn";
  }
  if (status === "rejected" || status === "validation_failed") {
    return "danger";
  }
  return "mute";
}

function formatShortList(values: string[]) {
  return values.length > 0 ? values.join("、") : "未指定";
}

function stringField(record: Record<string, unknown>, key: string) {
  const value = record[key];
  return typeof value === "string" ? value : "";
}

function stringArrayField(record: Record<string, unknown>, key: string) {
  const value = record[key];
  return Array.isArray(value)
    ? value.filter((item): item is string => typeof item === "string" && item !== "")
    : [];
}

function uniqueStrings(values: string[]) {
  return Array.from(new Set(values.filter((value) => value.trim() !== "")));
}

export function planTaskGraphSummaryLabel(graph: ProjectTaskGraph): string {
  const stageCount = new Set(graph.nodes.map((node) => node.stage_index ?? -1)).size;
  const assignedEmployeeIds = new Set(
    graph.nodes.flatMap((node) =>
      node.assigned_digital_employee_id ? [node.assigned_digital_employee_id] : [],
    ),
  );
  const employeeCount = assignedEmployeeIds.size;
  const tasksByStage = new Map<number, string[]>();

  for (const node of graph.nodes) {
    const stage = node.stage_index ?? -1;
    const list = tasksByStage.get(stage) ?? [];
    list.push(node.assigned_digital_employee_id ?? "");
    tasksByStage.set(stage, list);
  }

  const maxParallel = Math.max(
    0,
    ...[...tasksByStage.values()].map((ids) => new Set(ids.filter(Boolean)).size),
  );

  return `${employeeCount} 位同事 · 分 ${stageCount} 个阶段协作 · 最多 ${maxParallel} 人同时进行`;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function decisionTone(status: string): Tone {
  if (status === "pending") {
    return "warn";
  }
  if (status === "approved") {
    return "ok";
  }
  if (status === "rejected") {
    return "danger";
  }
  return "mute";
}

function jobTone(status: string): Tone {
  if (status === "completed" || status === "succeeded") {
    return "ok";
  }
  if (status === "failed") {
    return "danger";
  }
  if (status === "running" || status === "started") {
    return "info";
  }
  return "mute";
}

function requestTone(status: string): Tone {
  if (status === "approved" || status === "resolved") {
    return "ok";
  }
  if (status === "rejected" || status === "failed") {
    return "danger";
  }
  return "warn";
}

function dispatchGateTone(status: DispatchGateStatus): Tone {
  if (status === "passed") {
    return "ok";
  }
  if (status === "retry_later" || status === "waiting_human") {
    return "warn";
  }
  if (status === "blocked" || status === "replan_required") {
    return "danger";
  }
  return "mute";
}

export function projectBusinessBlocker(gates: DispatchGateResult[]) {
  const latest = gates[0];
  if (!latest || latest.status === "passed") {
    return undefined;
  }
  const blockerKeys = latest.blockers.map((blocker) => blocker.key);
  if (blockerKeys.some((key) => key.includes("runtime"))) {
    return {
      description: "目标运行资源暂不可用。项目负责人无需处理，系统会等待平台资源恢复或稍后重试。",
      status: "等待平台处理",
      title: "运行节点暂不可用，系统会稍后重试"
};
  }
  if (latest.status === "waiting_human") {
    return {
      description: "当前任务需要负责人确认后才能继续推进。",
      status: "待负责人处理",
      title: "需要负责人确认"
};
  }
  if (latest.status === "replan_required") {
    return {
      description: "当前计划不再满足执行条件，需要重新编排后继续。",
      status: "需重新计划",
      title: "计划需要调整"
};
  }
  return {
    description: "当前执行条件尚未满足，系统已保留阻塞原因。",
    status: "待处理",
    title: "执行条件未满足"
};
}

function formatDateTime(value?: string | null) {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "short",
    timeStyle: "short"
}).format(date);
}

// planRevisionHasBoundTemplate reports whether the plan was generated against a
// real, bound scenario template. template_version is the authoritative binding
// marker: server governance stamps it only for bound plans and strips it for
// unbound/generic demands, so a planner-hallucinated template_key without a
// version never renders the 场景模板/交付出口 rows.
function planRevisionHasBoundTemplate(revision: ProjectPlanRevision): boolean {
  return typeof revision.payload?.["template_version"] === "number";
}

function planRevisionTemplateKey(revision: ProjectPlanRevision): string {
  const key = revision.payload?.["template_key"];
  if (typeof key !== "string" || !key) {
    return "";
  }
  const version = revision.payload?.["template_version"];
  return typeof version === "number" ? `${key}@v${version}` : key;
}

function planRevisionExitDeliverable(revision: ProjectPlanRevision): string {
  const value = revision.payload?.["exit_deliverable"];
  return typeof value === "string" ? value : "";
}

function planRevisionAvailableExits(
  revision: ProjectPlanRevision,
): { deliverable: string; label: string }[] {
  const value = revision.payload?.["available_exits"];
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map((item) => {
      if (!item || typeof item !== "object") {
        return null;
      }
      const record = item as Record<string, unknown>;
      const deliverable = stringField(record, "deliverable");
      if (!deliverable) {
        return null;
      }
      const label = stringField(record, "label");
      return { deliverable, label: label || deliverable };
    })
    .filter((item): item is { deliverable: string; label: string } => item !== null);
}

function planRevisionExitLabel(revision: ProjectPlanRevision): string {
  const deliverable = planRevisionExitDeliverable(revision);
  if (!deliverable) {
    return "";
  }
  const match = planRevisionAvailableExits(revision).find(
    (exit) => exit.deliverable === deliverable,
  );
  return match?.label || deliverable;
}

function planRevisionConstraintNotes(
  revision: ProjectPlanRevision,
): { kind: string; message: string }[] {
  const value = revision.payload?.["constraint_notes"];
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map((item) => {
      if (!item || typeof item !== "object") {
        return null;
      }
      const record = item as Record<string, unknown>;
      const message = stringField(record, "message");
      if (!message) {
        return null;
      }
      return { kind: stringField(record, "kind") || "constraint", message };
    })
    .filter((item): item is { kind: string; message: string } => item !== null);
}

function constraintNoteKindLabel(kind: string): string {
  if (kind === "human_gate") {
    return "强制人工审批";
  }
  if (kind === "collapse") {
    return "角色合并";
  }
  return "约束";
}

function constraintNoteTone(kind: string): Tone {
  if (kind === "human_gate") {
    return "warn";
  }
  return "mute";
}

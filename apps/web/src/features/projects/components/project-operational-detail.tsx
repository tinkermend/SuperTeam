import { useState, type ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import {
  Activity,
  Archive,
  Bot,
  ChevronDown,
  CircleDot,
  ClipboardList,
  FileCheck2,
  ExternalLink,
  FileText,
  GitBranch,
  History,
  Settings2,
  UserRound,
} from "lucide-react";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  IconTile,
  SoftCard,
  StatusPill,
  V3Button,
  type V3Tone,
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
  CreateProjectAcceptanceInput,
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
  ProjectTransferRequest,
} from "@/lib/api/projects";
import {
  decisionStatusLabel,
  dispatchGateStatusLabel,
  statusLabel,
  taskStatusLabel,
} from "@/lib/status-labels";
import { ProjectExecutionTracePanel } from "./project-execution-trace-panel";
import { ProjectGovernanceTabs } from "./project-governance-tabs";
import { PlanGraphCanvas } from "./plan-graph-canvas";

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
  isArchived?: boolean;
  onArchiveProject: () => void;
  onCreateAcceptance: (input: CreateProjectAcceptanceInput) => void;
  onCreateArchiveSnapshot: (input: CreateProjectArchiveSnapshotInput) => void;
  onCreateEvidence: (input: CreateProjectEvidenceInput) => void;
  onPatchEvidence: (
    evidenceId: string,
    verificationStatus: ProjectEvidenceVerificationStatus,
  ) => void;
  onRetryExecutionTrace?: () => void;
  onResolveDecision: (decisionId: string, decision: string) => void;
  onSubmitDemand: () => void;
  overview?: ProjectOverview;
  planRevisions: ProjectPlanRevision[];
  project?: Project;
  reports?: ProjectReportRef[];
  routeDecisions: ProjectRouteDecision[];
  taskGraph?: ProjectTaskGraph;
  tasks: ProjectTask[];
  transferRequests: ProjectTransferRequest[];
};

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
  isArchived,
  onArchiveProject,
  onCreateAcceptance,
  onCreateArchiveSnapshot,
  onCreateEvidence,
  onPatchEvidence,
  onRetryExecutionTrace,
  onResolveDecision,
  onSubmitDemand,
  overview,
  planRevisions,
  project,
  reports,
  routeDecisions,
  taskGraph,
  tasks,
  transferRequests,
}: ProjectOperationalDetailProps) {
  const [advancedOpen, setAdvancedOpen] = useState(false);

  if (!project) {
    return (
      <SoftCard className="flex min-h-[460px] items-center justify-center p-8 text-sm text-v3-ink-2">
        从左侧选择一个项目查看运行详情
      </SoftCard>
    );
  }

  const humanRoles = overview?.human_roles ?? [];
  const digitalPool = overview?.digital_employee_pool ?? [];
  const servicePool = digitalPool;
  const projectOwners = ownerMembers(humanRoles, project.human_owner_user_id);
  const latestDemand = demands[0];
  const latestResult = executionSummaries[0];
  const pendingOwnerDecisions = decisionRequests.filter(
    (decision) => decision.status_snapshot === "pending",
  );
  const pendingOwnerActionItems = pendingOwnerDecisions.filter(
    (decision) => decision.decision_type !== "plan_review",
  );
  const businessBlocker = projectBusinessBlocker(dispatchGates ?? []);
  const activeTasks = overview?.active_tasks?.length ? overview.active_tasks : tasks;
  const recentEvents = overview?.recent_events?.length ? overview.recent_events : events;
  const currentPhase = overview?.status_summary.current_phase || project.status;
  const latestPlanRevision = selectLatestPlanRevision(planRevisions);
  const latestPlanReviewDecision = decisionRequests.find(
    (decision) =>
      decision.decision_type === "plan_review" &&
      decision.status_snapshot === "pending" &&
      (!latestPlanRevision ||
        !latestPlanRevision.coordination_job_id ||
        decision.coordination_job_id === latestPlanRevision.coordination_job_id),
  );

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
                <p className="truncate text-xl font-semibold tracking-normal text-v3-ink">
                  {project.name}
                </p>
                <StatusPill tone={projectStatusTone(project.status)}>
                  {projectStatusLabel(project.status)}
                </StatusPill>
              </div>
              <p className="mt-1 max-w-3xl text-sm text-v3-ink-2">
                {project.goal}
              </p>
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            <V3Button
              disabled={isArchived}
              type="button"
              onClick={onSubmitDemand}
            >
              <FileText data-icon="inline-start" />
              提交需求
            </V3Button>
            <V3Button asChild variant="outline">
              <Link
                params={{ projectId: project.id }}
                to="/projects/$projectId/config"
              >
                <Settings2 data-icon="inline-start" />
                配置项目
              </Link>
            </V3Button>
          </div>
        </div>

        <div className="mt-5 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          <FactTile
            icon={<GitBranch />}
            label="当前阶段"
            value={projectPhaseLabel(currentPhase)}
          />
          <FactTile
            icon={<UserRound />}
            label="待负责人处理"
            value={`${pendingOwnerDecisions.length} 项`}
          />
          <FactTile
            icon={<FileText />}
            label="当前需求"
            value={latestDemand?.title ?? "暂无需求"}
          />
          <FactTile
            icon={<ClipboardList />}
            label="当前执行"
            value={`${activeTasks.length} 个任务`}
          />
        </div>
      </SoftCard>

      {taskGraph && taskGraph.nodes.length > 0 ? (
        <section className="grid gap-2" data-testid="project-plan-graph-section">
          <div className="flex items-center gap-2 px-1">
            <ClipboardList className="size-4 text-v3-ink-2" />
            <h3 className="text-sm font-semibold tracking-normal">当前执行</h3>
            <StatusPill tone="mute">
              {planTaskGraphSummaryLabel(taskGraph)}
            </StatusPill>
          </div>
          <PlanGraphCanvas graph={taskGraph} />
        </section>
      ) : null}

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.4fr)_minmax(320px,0.8fr)]">
        <section className="grid min-w-0 gap-4">
          <SoftCard className="overflow-hidden">
            <PanelHeader
              icon={<FileText />}
              title="当前需求"
              meta={latestDemand ? demandStatusLabel(latestDemand.status) : "暂无需求"}
            />
            {latestDemand ? (
              <div className="grid gap-2 p-4">
                <p className="text-sm font-semibold text-v3-ink">{latestDemand.title}</p>
                <p className="line-clamp-3 text-sm leading-6 text-v3-ink-2">
                  {latestDemand.content || "需求内容已记录，等待系统生成下一步计划。"}
                </p>
              </div>
            ) : (
              <EmptyLine label="暂无提交到项目的需求" />
            )}
          </SoftCard>

          <SoftCard className="overflow-hidden">
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
                    <p className="mt-2 line-clamp-2 text-sm text-v3-ink-2">
                      {planRevisionSummary(latestPlanRevision)}
                    </p>
                  </div>
                  {latestPlanReviewDecision ? (
                    <div className="flex shrink-0 flex-wrap gap-2">
                      <V3Button
                        aria-label={`批准计划版本 v${latestPlanRevision.revision_number}`}
                        size="sm"
                        type="button"
                        onClick={() =>
                          onResolveDecision(latestPlanReviewDecision.id, "approved")
                        }
                      >
                        批准
                      </V3Button>
                      <V3Button
                        aria-label={`要求修改计划版本 v${latestPlanRevision.revision_number}`}
                        size="sm"
                        type="button"
                        variant="outline"
                        onClick={() =>
                          onResolveDecision(
                            latestPlanReviewDecision.id,
                            "request_changes",
                          )
                        }
                      >
                        要求修改
                      </V3Button>
                      <V3Button
                        aria-label={`拒绝计划版本 v${latestPlanRevision.revision_number}`}
                        size="sm"
                        type="button"
                        variant="outline"
                        onClick={() =>
                          onResolveDecision(latestPlanReviewDecision.id, "rejected")
                        }
                      >
                        拒绝
                      </V3Button>
                    </div>
                  ) : null}
                </div>
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
                <div className="divide-y divide-v3-line rounded-v3-inner border border-v3-line">
                  {planRevisionTasks(latestPlanRevision)
                    .slice(0, 4)
                    .map((task, index) => (
                      <div
                        className="grid gap-2 p-3"
                        key={`${planRevisionTaskKey(task)}-${index}`}
                      >
                        <div className="flex items-start justify-between gap-3">
                          <p className="min-w-0 line-clamp-2 text-sm font-medium">
                            {planRevisionTaskTitle(task)}
                          </p>
                          <StatusPill tone={planRevisionTaskRiskTone(task)}>
                            {planRevisionTaskRisk(task)}
                          </StatusPill>
                        </div>
                        <RuntimeMeta
                          label="能力"
                          value={formatShortList(planRevisionTaskCapabilities(task))}
                        />
                        <RuntimeMeta
                          label="输出"
                          value={formatShortList(planRevisionTaskOutputs(task))}
                        />
                      </div>
                    ))}
                  {planRevisionTasks(latestPlanRevision).length === 0 ? (
                    <EmptyLine label="计划版本尚未包含可展示任务" />
                  ) : null}
                </div>
              </div>
            ) : (
              <EmptyLine label="暂无计划，提交需求后由系统生成下一步计划。" />
            )}
          </SoftCard>

          {taskGraph && taskGraph.nodes.length > 0 ? null : (
            <SoftCard className="overflow-hidden">
              <PanelHeader
                icon={<ClipboardList />}
                title="当前执行"
                meta={`${activeTasks.length} 项`}
              />
              <div className="divide-y divide-v3-line">
                {activeTasks.length === 0 ? (
                  <EmptyLine label="当前没有正在执行的数字员工任务" />
                ) : (
                  activeTasks.slice(0, 6).map((task) => (
                    <div className="grid gap-1 p-4" key={task.id}>
                      <div className="flex items-center justify-between gap-3">
                        <p className="min-w-0 truncate text-sm font-medium">
                          {task.title}
                        </p>
                        <StatusPill tone="info">{taskStatusLabel(task.status)}</StatusPill>
                      </div>
                      <p className="line-clamp-2 text-xs text-v3-ink-2">
                        {task.summary || "等待系统分派数字员工执行。"}
                      </p>
                    </div>
                  ))
                )}
              </div>
            </SoftCard>
          )}

          <SoftCard className="overflow-hidden">
            <PanelHeader
              icon={<FileCheck2 />}
              title="最新结果"
              meta={latestResult ? "已回写" : "暂无结果"}
            />
            {latestResult ? (
              <div className="grid gap-2 p-4">
                <p className="line-clamp-3 text-sm font-medium text-v3-ink">
                  {latestResult.conclusion}
                </p>
                {latestResult.recommended_next_action ? (
                  <p className="line-clamp-2 text-xs text-v3-ink-2">
                    {latestResult.recommended_next_action}
                  </p>
                ) : null}
                <RuntimeMeta label="执行员工" value={latestResult.digital_employee_id} />
              </div>
            ) : (
              <EmptyLine label="数字员工完成任务后会在这里回写结果" />
            )}
          </SoftCard>

          <SoftCard className="overflow-hidden">
            <PanelHeader
              icon={<UserRound />}
              title="待负责人处理"
              meta={`${pendingOwnerActionItems.length} 项`}
            />
            <div className="divide-y divide-v3-line">
              {pendingOwnerActionItems.length === 0 ? (
                <EmptyLine label="当前没有需要项目负责人处理的事项" />
              ) : (
                pendingOwnerActionItems.slice(0, 5).map((decision) => (
                  <div className="grid gap-3 p-4" key={decision.id}>
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium">
                          {decision.title_snapshot}
                        </p>
                        <p className="mt-1 line-clamp-2 text-xs text-v3-ink-2">
                          {decision.summary_snapshot &&
                          decision.summary_snapshot !== decision.title_snapshot
                            ? decision.summary_snapshot
                            : "等待负责人处理"}
                        </p>
                      </div>
                      <StatusPill tone={decisionTone(decision.status_snapshot)}>
                        {decisionStatusLabel(decision.status_snapshot)}
                      </StatusPill>
                    </div>
                    <DecisionRequestActions
                      decision={decision}
                      onResolveDecision={onResolveDecision}
                    />
                  </div>
                ))
              )}
            </div>
          </SoftCard>

          {businessBlocker ? (
            <SoftCard className="overflow-hidden">
              <PanelHeader icon={<CircleDot />} title="当前阻塞" meta={businessBlocker.status} />
              <div className="grid gap-2 p-4">
                <p className="text-sm font-semibold text-v3-ink">{businessBlocker.title}</p>
                <p className="text-xs leading-5 text-v3-ink-2">{businessBlocker.description}</p>
              </div>
            </SoftCard>
          ) : null}

          <SoftCard className="overflow-hidden">
            <PanelHeader
              icon={<History />}
              title="事件流"
              meta={`${recentEvents.length} 条`}
            />
            <div className="divide-y divide-v3-line">
              {recentEvents.length === 0 ? (
                <EmptyLine label="暂无项目事件" />
              ) : (
                recentEvents.slice(0, 8).map((event) => {
                  const eventDisplay = projectEventDisplay(event);
                  return (
                    <div className="flex gap-3 p-4" key={event.id}>
                      <span className="mt-1 size-2 rounded-full bg-v3-brand" />
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <p className="text-sm font-medium">{eventDisplay.title}</p>
                          <span className="text-xs text-v3-ink-2">
                            #{event.sequence_number}
                          </span>
                        </div>
                        <p className="mt-1 line-clamp-2 text-xs text-v3-ink-2">
                          {eventDisplay.summary}
                        </p>
                        {eventDisplay.resource ? (
                          <p className="mt-1 text-xs text-v3-ink-2">
                            {eventDisplay.resource}
                          </p>
                        ) : null}
                      </div>
                    </div>
                  );
                })
              )}
            </div>
          </SoftCard>
        </section>

        <aside className="grid min-w-0 gap-4">
          <MemberPanel
            emptyLabel="当前项目尚未设置项目负责人"
            icon={<UserRound />}
            members={projectOwners}
            title="项目负责人组"
          />
          <MemberPanel
            emptyLabel="当前项目服务池为空"
            icon={<Bot />}
            members={servicePool}
            title="项目服务池"
          />
        </aside>
      </div>

      <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
        <div className="overflow-hidden rounded-v3-card border border-v3-line bg-v3-card shadow-v3">
          <CollapsibleTrigger asChild>
            <button
              aria-label={advancedOpen ? "收起高级项目事实" : "展开高级项目事实"}
              className="flex w-full items-center justify-between gap-3 border-b border-v3-line p-4 text-left"
              type="button"
            >
              <span className="flex min-w-0 items-center gap-2">
                <IconTile tone="brand" size="sm" className="size-8 rounded-[10px] [&_svg]:size-3.5">
                  <GitBranch />
                </IconTile>
                <span className="min-w-0">
                  <span className="block font-semibold text-v3-ink">高级项目事实</span>
                  <span className="mt-0.5 block text-xs text-v3-ink-2">
                    计划历史、任务图、执行记录、治理、预算、归档和内部协调事实
                  </span>
                </span>
              </span>
              <span className="flex shrink-0 items-center gap-2 text-xs font-semibold text-v3-ink-2">
                {advancedOpen ? "收起" : "展开"}
                <ChevronDown
                  className={cn("size-4 transition-transform", advancedOpen && "rotate-180")}
                />
              </span>
            </button>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <div className="grid gap-4 p-4 xl:grid-cols-[minmax(0,1.3fr)_minmax(320px,0.7fr)]">
              <section className="grid min-w-0 gap-4">
                <DispatchGateSummary
                  gates={dispatchGates ?? []}
                  taskTitle={dispatchGateTaskTitle}
                />
                <AdvancedRouteDecisions routeDecisions={routeDecisions} />
                <ProjectExecutionTracePanel
                  errorMessage={executionTraceErrorMessage}
                  isError={executionTraceIsError}
                  isLoading={executionTraceIsLoading}
                  onRetry={onRetryExecutionTrace}
                  trace={executionTrace}
                />
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
                  onCreateAcceptance={onCreateAcceptance}
                  onCreateArchiveSnapshot={onCreateArchiveSnapshot}
                  onCreateEvidence={onCreateEvidence}
                  onPatchEvidence={onPatchEvidence}
                  reports={reports}
                  routeDecisionCount={routeDecisions.length}
                  taskCount={tasks.length}
                />
              </section>
              <aside className="grid min-w-0 gap-4">
                <AdvancedCoordinationJobs coordinationJobs={coordinationJobs} />
                <AdvancedExecutionSummaries executionSummaries={executionSummaries} />
                <AdvancedTransferRequests transferRequests={transferRequests} />
                <AdvancedWorkflow project={project} overview={overview} />
                <AdvancedDemands demands={demands} />
                <V3Button
                  disabled={isArchived}
                  type="button"
                  variant="outline"
                  onClick={onArchiveProject}
                >
                  <Archive data-icon="inline-start" />
                  归档项目
                </V3Button>
              </aside>
            </div>
          </CollapsibleContent>
        </div>
      </Collapsible>
    </div>
  );
}

function DispatchGateSummary({
  gates,
  taskTitle,
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
          <h3 className="text-sm font-semibold text-v3-ink">Dispatch gate 技术详情</h3>
          {taskTitle ? (
            <p className="mt-1 truncate text-xs text-v3-ink-2">{taskTitle}</p>
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
              className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-sm text-v3-ink-2"
              key={`${latest.id}-${blocker.key}`}
            >
              <span className="min-w-0 break-all font-mono text-xs text-v3-ink">
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
        <p className="mt-2 text-xs text-v3-ink-2">
          {`下次重试 ${formatDateTime(latest.retry_after)}`}
        </p>
      ) : null}
    </SoftCard>
  );
}

function AdvancedRouteDecisions({
  routeDecisions,
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
      <div className="divide-y divide-v3-line">
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
  coordinationJobs,
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
      <div className="divide-y divide-v3-line">
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
              <p className="truncate text-xs text-v3-ink-2">
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
  executionSummaries,
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
      <div className="divide-y divide-v3-line">
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
                <p className="line-clamp-2 text-xs text-v3-ink-2">
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
  transferRequests,
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
      <div className="divide-y divide-v3-line">
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
  overview,
}: {
  project: Project;
  overview?: ProjectOverview;
}) {
  return (
    <SoftCard className="overflow-hidden">
      <PanelHeader
        icon={<GitBranch />}
        title="协调线程"
        meta={overview?.coordination_workflow.status || project.coordination_status}
      />
      <div className="p-4">
        <p className="truncate text-sm font-medium">
          {project.coordination_workflow_id}
        </p>
        <p className="mt-1 text-xs text-v3-ink-2">
          虚拟协调线程，仅作为项目 Workflow 元数据展示。
        </p>
      </div>
    </SoftCard>
  );
}

function AdvancedDemands({ demands }: { demands: ProjectDemand[] }) {
  return (
    <SoftCard className="overflow-hidden">
      <PanelHeader
        icon={<FileText />}
        title="需求记录"
        meta={`${demands.length} 条`}
      />
      <div className="divide-y divide-v3-line">
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
              <p className="line-clamp-2 text-xs text-v3-ink-2">
                {demand.content || "需求内容已记录"}
              </p>
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
  value,
}: {
  icon: ReactNode;
  label: string;
  value: string;
}) {
  return (
    <div className="min-w-0 rounded-v3-inner bg-v3-card-soft p-3">
      <div className="flex items-center gap-2 text-xs text-v3-ink-2">
        <IconTile tone="mute" size="sm" className="size-8 rounded-[10px] [&_svg]:size-3.5">
          {icon}
        </IconTile>
        {label}
      </div>
      <p className="mt-2 truncate text-sm font-semibold text-v3-ink">{value}</p>
    </div>
  );
}

function PanelHeader({
  icon,
  meta,
  title,
}: {
  icon: ReactNode;
  meta: string;
  title: string;
}) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-v3-line p-4">
      <div className="flex items-center gap-2">
        <IconTile tone="brand" size="sm" className="size-8 rounded-[10px] [&_svg]:size-3.5">
          {icon}
        </IconTile>
        <h3 className="font-semibold text-v3-ink">{title}</h3>
      </div>
      <StatusPill tone="mute" showDot={false}>{meta}</StatusPill>
    </div>
  );
}

function MemberPanel({
  emptyLabel,
  icon,
  members,
  title,
}: {
  emptyLabel: string;
  icon: ReactNode;
  members: ProjectMember[];
  title: string;
}) {
  return (
    <SoftCard className="overflow-hidden">
      <PanelHeader icon={icon} title={title} meta={`${members.length} 个`} />
      <div className="divide-y divide-v3-line">
        {members.length === 0 ? (
          <EmptyLine label={emptyLabel} />
        ) : (
          members.slice(0, 6).map((member) => (
            <div className="flex items-center justify-between gap-3 p-4" key={member.id}>
              <div className="min-w-0">
                <p className="truncate text-sm font-medium text-v3-ink">
                  {member.display_name_snapshot || member.principal_id}
                </p>
                <p className="truncate text-xs text-v3-ink-2">
                  {projectMemberBusinessLabel(member)}
                </p>
              </div>
              <ExternalLink className="size-3.5 text-v3-ink-2" />
            </div>
          ))
        )}
      </div>
    </SoftCard>
  );
}

function EmptyLine({ label }: { label: string }) {
  return (
    <div className="flex min-h-24 items-center justify-center p-4 text-sm text-v3-ink-2">
      {label}
    </div>
  );
}

function RuntimeMeta({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex min-w-0 items-center justify-between gap-3 text-xs">
      <span className="shrink-0 text-v3-ink-2">{label}</span>
      <span className="min-w-0 truncate font-medium text-v3-ink">{value}</span>
    </div>
  );
}

function DecisionRequestActions({
  decision,
  onResolveDecision,
}: {
  decision: ProjectDecisionRequest;
  onResolveDecision: (decisionId: string, decision: string) => void;
}) {
  if (decision.status_snapshot !== "pending") {
    return null;
  }

  const actions = [
    { ariaLabel: `批准：${decision.title_snapshot}`, label: "批准", value: "approved" },
    {
      ariaLabel: `要求补证：${decision.title_snapshot}`,
      label: "要求补证",
      value: "needs_more_evidence",
    },
  ];

  return (
    <div className="flex flex-wrap gap-2">
      {actions.map((action) => (
        <V3Button
          aria-label={action.ariaLabel}
          key={action.value}
          size="sm"
          type="button"
          variant={action.value === "approved" ? "primary" : "outline"}
          onClick={() => onResolveDecision(decision.id, action.value)}
        >
          {action.label}
        </V3Button>
      ))}
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

function projectEventDisplay(event: ProjectEvent) {
  const labels: Record<string, { summary: string; title: string }> = {
    "coordination_job.created": {
      summary: "系统已开始推进下一步项目工作。",
      title: "项目推进已启动",
    },
    "decision.requested": {
      summary: "有事项需要项目负责人处理。",
      title: "等待负责人处理",
    },
    "decision.submitted": {
      summary: "负责人处理结果已记录。",
      title: "负责人已处理",
    },
    "demand.submitted": {
      summary: "新的项目需求已进入处理队列。",
      title: "需求已提交",
    },
    "project.acceptance.submitted": {
      summary: "项目验收结论已提交。",
      title: "验收结论已提交",
    },
    "project.archive.retention_pending": {
      summary: "项目归档前仍有保留事项待处理。",
      title: "归档保留事项待处理",
    },
    "project.archive_snapshot.created": {
      summary: "项目归档快照已生成。",
      title: "归档快照已生成",
    },
    "project.archived": {
      summary: "项目已归档关闭。",
      title: "项目已归档",
    },
    "project.artifact.linked": {
      summary: "新的项目工件已关联。",
      title: "工件已关联",
    },
    "project.budget.recorded": {
      summary: "项目预算记录已更新。",
      title: "预算记录已更新",
    },
    "project.config.changed": {
      summary: "项目配置已更新。",
      title: "配置已更新",
    },
    "project.created": {
      summary: "项目已创建。",
      title: "项目已创建",
    },
    "project.evidence.linked": {
      summary: "新的项目证据已关联。",
      title: "证据已关联",
    },
    "project.evidence.verified": {
      summary: "项目证据已完成核验。",
      title: "证据已核验",
    },
    "project.report.linked": {
      summary: "项目报告已关联。",
      title: "报告已关联",
    },
    "project_task.completed": {
      summary: "数字员工任务已完成并回写结果。",
      title: "执行任务已完成",
    },
    "project_task.created": {
      summary: "新的执行任务已进入项目推进队列。",
      title: "执行任务已创建",
    },
    "project_task.dispatched": {
      summary: "系统已安排数字员工执行任务。",
      title: "执行任务已分派",
    },
    "project_task.dispatch_gate.blocked": {
      summary: "当前执行条件未满足，系统已记录阻塞原因。",
      title: "执行条件未满足",
    },
    "project_task.dispatch_gate.checked": {
      summary: "系统已检查任务执行条件。",
      title: "执行条件已检查",
    },
    "project_task.dispatch_gate.replan_required": {
      summary: "当前计划需要调整后继续推进。",
      title: "计划需要调整",
    },
    "project_task.dispatch_gate.retry_later": {
      summary: "运行条件暂不可用，系统会稍后重试。",
      title: "稍后重试执行",
    },
    "project_task.dispatch_gate.waiting_human": {
      summary: "当前执行需要负责人确认。",
      title: "等待负责人确认",
    },
    "project_task.failed": {
      summary: "数字员工任务执行失败，系统已记录原因。",
      title: "执行任务失败",
    },
    "route_decision.created": {
      summary: "系统已生成任务分派方案。",
      title: "任务分派方案已生成",
    },
    "transfer.requested": {
      summary: "项目执行需要调整服务员工。",
      title: "服务员工调整待处理",
    },
    "workflow.signaled": {
      summary: "项目推进状态已更新。",
      title: "项目推进已更新",
    },
  };
  const label = labels[event.event_type] ?? {
    summary: "项目状态已有新记录。",
    title: "项目动态已更新",
  };
  return {
    resource: event.resource_id ? `项目对象 · ${shortIdentifier(event.resource_id)}` : undefined,
    summary: label.summary,
    title: label.title,
  };
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
      tenant_id: "",
    },
  ];
}

function projectStatusLabel(status: ProjectStatus | string) {
  const labels: Record<string, string> = {
    acceptance: "验收中",
    archived: "已归档",
    configuring: "配置中",
    draft: "草稿",
    paused: "已暂停",
    running: "运行中",
  };
  return labels[status] ?? status;
}

function projectPhaseLabel(phase: string) {
  const labels: Record<string, string> = {
    acceptance: "待确认结果",
    archived: "已关闭",
    configuring: "配置中",
    draft: "待配置",
    paused: "已暂停",
    running: "执行中",
  };
  return labels[phase] ?? phase;
}

function demandStatusLabel(status: string) {
  const labels: Record<string, string> = {
    cancelled: "已取消",
    completed: "已完成",
    executing: "执行中",
    failed: "失败",
    planned: "已计划",
    planning_pending: "待计划",
    recorded: "已记录",
    submitted: "待计划",
  };
  return labels[status] ?? status;
}

function projectStatusTone(status: ProjectStatus | string): V3Tone {
  if (status === "running") return "ok";
  if (status === "archived") return "mute";
  if (status === "paused" || status === "acceptance") return "warn";
  if (status === "configuring" || status === "draft") return "info";
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

function planRevisionTaskKey(task: Record<string, unknown>) {
  return stringField(task, "planned_task_key") || stringField(task, "title") || "task";
}

function planRevisionTaskTitle(task: Record<string, unknown>) {
  return stringField(task, "title") || stringField(task, "objective") || "未命名任务";
}

function planRevisionTaskCapabilities(task: Record<string, unknown>) {
  return uniqueStrings([
    ...stringArrayField(task, "required_capabilities"),
    ...stringArrayField(task, "matched_capabilities"),
  ]).slice(0, 4);
}

function planRevisionTaskOutputs(task: Record<string, unknown>) {
  return stringArrayField(task, "expected_outputs").slice(0, 3);
}

function planRevisionTaskRisk(task: Record<string, unknown>) {
  return stringField(task, "risk_level") || "normal";
}

function planRevisionTaskRiskTone(task: Record<string, unknown>): V3Tone {
  const risk = planRevisionTaskRisk(task);
  if (risk === "critical" || risk === "high") {
    return "danger";
  }
  if (risk === "medium") {
    return "warn";
  }
  return "mute";
}

function planRevisionTone(status: string): V3Tone {
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

function planTaskGraphSummaryLabel(graph: ProjectTaskGraph): string {
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

function decisionTone(status: string): V3Tone {
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

function jobTone(status: string): V3Tone {
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

function requestTone(status: string): V3Tone {
  if (status === "approved" || status === "resolved") {
    return "ok";
  }
  if (status === "rejected" || status === "failed") {
    return "danger";
  }
  return "warn";
}

function dispatchGateTone(status: DispatchGateStatus): V3Tone {
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

function projectBusinessBlocker(gates: DispatchGateResult[]) {
  const latest = gates[0];
  if (!latest || latest.status === "passed") {
    return undefined;
  }
  const blockerKeys = latest.blockers.map((blocker) => blocker.key);
  if (blockerKeys.some((key) => key.includes("runtime"))) {
    return {
      description: "目标运行资源暂不可用。项目负责人无需处理，系统会等待平台资源恢复或稍后重试。",
      status: "等待平台处理",
      title: "运行节点暂不可用，系统会稍后重试",
    };
  }
  if (latest.status === "waiting_human") {
    return {
      description: "当前任务需要负责人确认后才能继续推进。",
      status: "待负责人处理",
      title: "需要负责人确认",
    };
  }
  if (latest.status === "replan_required") {
    return {
      description: "当前计划不再满足执行条件，需要重新编排后继续。",
      status: "需重新计划",
      title: "计划需要调整",
    };
  }
  return {
    description: "当前执行条件尚未满足，系统已保留阻塞原因。",
    status: "待处理",
    title: "执行条件未满足",
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
    timeStyle: "short",
  }).format(date);
}

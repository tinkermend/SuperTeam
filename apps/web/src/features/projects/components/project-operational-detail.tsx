import type { ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import {
  Activity,
  Archive,
  Bot,
  ClipboardList,
  FileCheck2,
  FileArchive,
  ExternalLink,
  FileText,
  GitBranch,
  History,
  Settings2,
  UserRound,
} from "lucide-react";
import {
  IconTile,
  SoftCard,
  StatusPill,
  V3Button,
  type V3Tone,
} from "@/components/superteam";
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
import { ProjectExecutionTracePanel } from "./project-execution-trace-panel";
import { ProjectGovernanceTabs } from "./project-governance-tabs";
import { PlanTaskGraph } from "./plan-task-graph";

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
  if (!project) {
    return (
      <SoftCard className="flex min-h-[460px] items-center justify-center p-8 text-sm text-v3-ink-2">
        从左侧选择一个项目查看运行详情
      </SoftCard>
    );
  }

  const humanRoles = overview?.human_roles ?? [];
  const digitalPool = overview?.digital_employee_pool ?? [];
  const activeTasks = overview?.active_tasks?.length ? overview.active_tasks : tasks;
  const recentEvents = overview?.recent_events?.length ? overview.recent_events : events;
  const taskSummary = overview?.task_summary;
  const currentPhase = overview?.status_summary.current_phase || project.status;
  const evidencePolicyConfigured = Object.keys(project.evidence_policy ?? {}).length > 0;
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
                <h2 className="truncate text-xl font-semibold tracking-normal">
                  {project.name}
                </h2>
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
                配置
              </Link>
            </V3Button>
            <V3Button asChild variant="outline">
              <Link search={{ project_id: project.id }} to="/audit">
                <History data-icon="inline-start" />
                审计
              </Link>
            </V3Button>
            <V3Button asChild variant="outline">
              <Link search={{ project_id: project.id }} to="/costs">
                <FileCheck2 data-icon="inline-start" />
                成本
              </Link>
            </V3Button>
            <V3Button
              disabled={isArchived}
              type="button"
              variant="outline"
              onClick={onArchiveProject}
            >
              <Archive data-icon="inline-start" />
              归档
            </V3Button>
          </div>
        </div>

        <div className="mt-5 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          <FactTile
            icon={<GitBranch />}
            label="当前阶段"
            value={currentPhase}
          />
          <FactTile
            icon={<UserRound />}
            label="待人工处理"
            value={`${taskSummary?.pending_human_tasks ?? 0} 项`}
          />
          <FactTile
            icon={<FileArchive />}
            label="证据策略"
            value={evidencePolicyConfigured ? "已配置" : "未配置"}
          />
          <FactTile
            icon={<ClipboardList />}
            label="活跃任务"
            value={`${activeTasks.length} 个`}
          />
        </div>
      </SoftCard>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.4fr)_minmax(320px,0.8fr)]">
        <section className="grid min-w-0 gap-4">
          <SoftCard className="overflow-hidden">
            <PanelHeader
              icon={<GitBranch />}
              title="计划版本"
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
                        {latestPlanRevision.status}
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
              <EmptyLine label="暂无计划版本" />
            )}
          </SoftCard>

          {taskGraph && taskGraph.nodes.length > 0 ? (
            <div className="grid gap-2">
              <div className="flex items-center gap-2 px-1">
                <ClipboardList className="size-4 text-v3-ink-2" />
                <h3 className="text-sm font-semibold tracking-normal">任务计划</h3>
                <StatusPill tone="mute">{`${taskGraph.nodes.length} 项`}</StatusPill>
              </div>
              <PlanTaskGraph
                nodes={taskGraph.nodes}
                edges={taskGraph.edges}
                employees={taskGraph.employees}
                stageSummaries={taskGraph.stage_summaries}
              />
            </div>
          ) : (
            <SoftCard className="overflow-hidden">
              <PanelHeader
                icon={<ClipboardList />}
                title="任务计划"
                meta={`${activeTasks.length} 项`}
              />
              <div className="divide-y divide-v3-line">
                {activeTasks.length === 0 ? (
                  <EmptyLine label="当前项目暂无活跃任务" />
                ) : (
                  activeTasks.slice(0, 6).map((task) => (
                    <div className="grid gap-1 p-4" key={task.id}>
                      <div className="flex items-center justify-between gap-3">
                        <p className="min-w-0 truncate text-sm font-medium">
                          {task.title}
                        </p>
                        <StatusPill tone="info">{task.status}</StatusPill>
                      </div>
                      <p className="line-clamp-2 text-xs text-v3-ink-2">
                        {task.summary || "等待项目协调线程分派执行对象"}
                      </p>
                    </div>
                  ))
                )}
              </div>
            </SoftCard>
          )}

          <DispatchGateSummary
            gates={dispatchGates ?? []}
            taskTitle={dispatchGateTaskTitle}
          />

          <SoftCard className="overflow-hidden">
            <PanelHeader
              icon={<UserRound />}
              title="人类决策队列"
              meta={`${decisionRequests.length} 项`}
            />
            <div className="divide-y divide-v3-line">
              {decisionRequests.length === 0 ? (
                <EmptyLine label="当前没有待处理的人类决策" />
              ) : (
                decisionRequests.slice(0, 5).map((decision) => (
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
                        {decision.status_snapshot}
                      </StatusPill>
                    </div>
                    {decision.status_snapshot === "pending" ? (
                      <div className="flex flex-wrap gap-2">
                        <V3Button
                          aria-label={`批准：${decision.title_snapshot}`}
                          size="sm"
                          type="button"
                          onClick={() => onResolveDecision(decision.id, "approved")}
                        >
                          批准
                        </V3Button>
                        <V3Button
                          aria-label={`要求补证：${decision.title_snapshot}`}
                          size="sm"
                          type="button"
                          variant="outline"
                          onClick={() =>
                            onResolveDecision(decision.id, "needs_more_evidence")
                          }
                        >
                          要求补证
                        </V3Button>
                      </div>
                    ) : null}
                  </div>
                ))
              )}
            </div>
          </SoftCard>

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
                recentEvents.slice(0, 8).map((event) => (
                  <div className="flex gap-3 p-4" key={event.id}>
                    <span className="mt-1 size-2 rounded-full bg-v3-brand" />
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <p className="text-sm font-medium">{event.event_type}</p>
                        <span className="text-xs text-v3-ink-2">
                          #{event.sequence_number}
                        </span>
                      </div>
                      <p className="mt-1 line-clamp-2 text-xs text-v3-ink-2">
                        {event.summary || "项目事件已记录"}
                      </p>
                      {event.resource_type || event.resource_id ? (
                        <p className="mt-1 text-xs text-v3-ink-2">
                          {event.resource_type ?? "resource"} ·{" "}
                          {event.resource_id ?? "-"}
                        </p>
                      ) : null}
                    </div>
                  </div>
                ))
              )}
            </div>
          </SoftCard>
        </section>

        <aside className="grid min-w-0 gap-4">
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
                      <StatusPill tone={jobTone(job.status)}>{job.status}</StatusPill>
                    </div>
                    <p className="truncate text-xs text-v3-ink-2">
                      {job.workflow_id}
                    </p>
                  </div>
                ))
              )}
            </div>
          </SoftCard>

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
                        {request.status}
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

          <MemberPanel
            icon={<UserRound />}
            members={humanRoles}
            title="人类角色"
          />
          <MemberPanel
            icon={<Bot />}
            members={digitalPool}
            title="数字员工池"
          />
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
        </aside>
      </div>
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
  const statusLabel: Record<DispatchGateStatus, string> = {
    blocked: "Blocked",
    passed: "Passed",
    replan_required: "Replan required",
    retry_later: "Retry later",
    waiting_human: "Waiting human",
  };

  return (
    <SoftCard className="p-4">
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <h3 className="text-sm font-semibold text-v3-ink">Pre-dispatch gate</h3>
          {taskTitle ? (
            <p className="mt-1 truncate text-xs text-v3-ink-2">{taskTitle}</p>
          ) : null}
        </div>
        <StatusPill tone={dispatchGateTone(latest.status)}>
          {statusLabel[latest.status]}
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
                {blocker.retryable ? "retryable" : blocker.severity}
              </span>
            </li>
          ))}
        </ul>
      ) : null}
      {latest.retry_after ? (
        <p className="mt-2 text-xs text-v3-ink-2">
          Retry after {formatDateTime(latest.retry_after)}
        </p>
      ) : null}
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
  icon,
  members,
  title,
}: {
  icon: ReactNode;
  members: ProjectMember[];
  title: string;
}) {
  return (
    <SoftCard className="overflow-hidden">
      <PanelHeader icon={icon} title={title} meta={`${members.length} 个`} />
      <div className="divide-y divide-v3-line">
        {members.length === 0 ? (
          <EmptyLine label={`${title}为空`} />
        ) : (
          members.slice(0, 6).map((member) => (
            <div className="flex items-center justify-between gap-3 p-4" key={member.id}>
              <div className="min-w-0">
                <p className="truncate text-sm font-medium text-v3-ink">
                  {member.display_name_snapshot || member.principal_id}
                </p>
                <p className="truncate text-xs text-v3-ink-2">
                  {member.project_role} · {member.principal_type}
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

function formatIdList(ids: string[]) {
  return ids.length > 0 ? ids.join("、") : "未指定";
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

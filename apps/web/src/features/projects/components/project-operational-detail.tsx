import type { ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import {
  Activity,
  Archive,
  Bot,
  ClipboardList,
  FileArchive,
  ExternalLink,
  FileText,
  GitBranch,
  History,
  Settings2,
  UserRound,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { LiquidCard, SemanticIconTile, StatusBadge } from "@/components/superteam";
import type {
  Project,
  ProjectDemand,
  ProjectEvent,
  ProjectMember,
  ProjectOverview,
  ProjectTask,
} from "@/lib/api/projects";
import { statusLabel, statusTone } from "./project-switcher-pane";

type ProjectOperationalDetailProps = {
  demands: ProjectDemand[];
  events: ProjectEvent[];
  isArchived?: boolean;
  onArchiveProject: () => void;
  onSubmitDemand: () => void;
  overview?: ProjectOverview;
  project?: Project;
  tasks: ProjectTask[];
};

export function ProjectOperationalDetail({
  demands,
  events,
  isArchived,
  onArchiveProject,
  onSubmitDemand,
  overview,
  project,
  tasks,
}: ProjectOperationalDetailProps) {
  if (!project) {
    return (
      <LiquidCard className="flex min-h-[460px] items-center justify-center rounded-xl p-8 text-sm text-muted-foreground">
        从左侧选择一个项目查看运行详情
      </LiquidCard>
    );
  }

  const humanRoles = overview?.human_roles ?? [];
  const digitalPool = overview?.digital_employee_pool ?? [];
  const activeTasks = overview?.active_tasks?.length ? overview.active_tasks : tasks;
  const recentEvents = overview?.recent_events?.length ? overview.recent_events : events;
  const taskSummary = overview?.task_summary;
  const currentPhase = overview?.status_summary.current_phase || project.status;
  const evidencePolicyConfigured = Object.keys(project.evidence_policy ?? {}).length > 0;

  return (
    <div className="grid min-w-0 gap-4">
      <LiquidCard className="rounded-xl p-5">
        <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex min-w-0 items-start gap-3">
            <SemanticIconTile tone="primary" size="lg">
              <Activity />
            </SemanticIconTile>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <h2 className="truncate text-xl font-semibold tracking-normal">
                  {project.name}
                </h2>
                <StatusBadge tone={statusTone(project.status)}>
                  {statusLabel(project.status)}
                </StatusBadge>
              </div>
              <p className="mt-1 max-w-3xl text-sm text-muted-foreground">
                {project.goal}
              </p>
            </div>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              disabled={isArchived}
              type="button"
              onClick={onSubmitDemand}
            >
              <FileText data-icon="inline-start" />
              提交需求
            </Button>
            <Button asChild variant="outline">
              <Link
                params={{ projectId: project.id }}
                to="/projects/$projectId/config"
              >
                <Settings2 data-icon="inline-start" />
                配置
              </Link>
            </Button>
            <Button
              disabled={isArchived}
              type="button"
              variant="outline"
              onClick={onArchiveProject}
            >
              <Archive data-icon="inline-start" />
              归档
            </Button>
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
      </LiquidCard>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.4fr)_minmax(320px,0.8fr)]">
        <section className="grid min-w-0 gap-4">
          <LiquidCard className="rounded-xl">
            <PanelHeader
              icon={<ClipboardList />}
              title="活跃任务"
              meta={`${activeTasks.length} 项`}
            />
            <div className="divide-y">
              {activeTasks.length === 0 ? (
                <EmptyLine label="当前项目暂无活跃任务" />
              ) : (
                activeTasks.slice(0, 6).map((task) => (
                  <div className="grid gap-1 p-4" key={task.id}>
                    <div className="flex items-center justify-between gap-3">
                      <p className="min-w-0 truncate text-sm font-medium">
                        {task.title}
                      </p>
                      <StatusBadge tone="info">{task.status}</StatusBadge>
                    </div>
                    <p className="line-clamp-2 text-xs text-muted-foreground">
                      {task.summary || "等待项目协调线程分派执行对象"}
                    </p>
                  </div>
                ))
              )}
            </div>
          </LiquidCard>

          <LiquidCard className="rounded-xl">
            <PanelHeader
              icon={<UserRound />}
              title="人类决策队列"
              meta="V0"
            />
            <EmptyLine label="V0 暂未接入人类决策队列" />
          </LiquidCard>

          <LiquidCard className="rounded-xl">
            <PanelHeader
              icon={<History />}
              title="事件流"
              meta={`${recentEvents.length} 条`}
            />
            <div className="divide-y">
              {recentEvents.length === 0 ? (
                <EmptyLine label="暂无项目事件" />
              ) : (
                recentEvents.slice(0, 8).map((event) => (
                  <div className="flex gap-3 p-4" key={event.id}>
                    <span className="mt-1 size-2 rounded-full bg-primary" />
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <p className="text-sm font-medium">{event.event_type}</p>
                        <span className="text-xs text-muted-foreground">
                          #{event.sequence_number}
                        </span>
                      </div>
                      <p className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                        {event.summary || "项目事件已记录"}
                      </p>
                      {event.resource_type || event.resource_id ? (
                        <p className="mt-1 text-xs text-muted-foreground">
                          {event.resource_type ?? "resource"} ·{" "}
                          {event.resource_id ?? "-"}
                        </p>
                      ) : null}
                    </div>
                  </div>
                ))
              )}
            </div>
          </LiquidCard>
        </section>

        <aside className="grid min-w-0 gap-4">
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
          <LiquidCard className="rounded-xl">
            <PanelHeader
              icon={<GitBranch />}
              title="协调线程"
              meta={overview?.coordination_workflow.status || project.coordination_status}
            />
            <div className="p-4">
              <p className="truncate text-sm font-medium">
                {project.coordination_workflow_id}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">
                虚拟协调线程，仅作为项目 Workflow 元数据展示。
              </p>
            </div>
          </LiquidCard>
          <LiquidCard className="rounded-xl">
            <PanelHeader
              icon={<FileText />}
              title="需求记录"
              meta={`${demands.length} 条`}
            />
            <div className="divide-y">
              {demands.length === 0 ? (
                <EmptyLine label="暂无提交到项目的需求" />
              ) : (
                demands.slice(0, 4).map((demand) => (
                  <div className="grid gap-1 p-4" key={demand.id}>
                    <div className="flex items-center justify-between gap-3">
                      <p className="truncate text-sm font-medium">
                        {demand.title}
                      </p>
                      <StatusBadge tone="neutral">{demand.source_type}</StatusBadge>
                    </div>
                    <p className="line-clamp-2 text-xs text-muted-foreground">
                      {demand.content || "需求内容已记录"}
                    </p>
                  </div>
                ))
              )}
            </div>
          </LiquidCard>
        </aside>
      </div>
    </div>
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
    <div className="min-w-0 rounded-lg border bg-white/55 p-3">
      <div className="flex items-center gap-2 text-xs text-muted-foreground">
        <span className="[&_svg]:size-3.5">{icon}</span>
        {label}
      </div>
      <p className="mt-2 truncate text-sm font-semibold">{value}</p>
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
    <div className="flex items-center justify-between gap-3 border-b p-4">
      <div className="flex items-center gap-2">
        <span className="text-primary [&_svg]:size-4">{icon}</span>
        <h3 className="font-semibold">{title}</h3>
      </div>
      <span className="text-xs text-muted-foreground">{meta}</span>
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
    <LiquidCard className="rounded-xl">
      <PanelHeader icon={icon} title={title} meta={`${members.length} 个`} />
      <div className="divide-y">
        {members.length === 0 ? (
          <EmptyLine label={`${title}为空`} />
        ) : (
          members.slice(0, 6).map((member) => (
            <div className="flex items-center justify-between gap-3 p-4" key={member.id}>
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">
                  {member.display_name_snapshot || member.principal_id}
                </p>
                <p className="truncate text-xs text-muted-foreground">
                  {member.project_role} · {member.principal_type}
                </p>
              </div>
              <ExternalLink className="size-3.5 text-muted-foreground" />
            </div>
          ))
        )}
      </div>
    </LiquidCard>
  );
}

function EmptyLine({ label }: { label: string }) {
  return (
    <div className="flex min-h-24 items-center justify-center p-4 text-sm text-muted-foreground">
      {label}
    </div>
  );
}

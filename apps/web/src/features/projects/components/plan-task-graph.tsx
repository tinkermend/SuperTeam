import { Bot, GitBranch, ShieldCheck } from "lucide-react";
import {
  LiquidCard,
  SemanticIconTile,
  StatusBadge,
  type Tone,
} from "@/components/superteam";
import type {
  ProjectTask,
  ProjectTaskGraphEdge,
  ProjectTaskGraphEmployee,
  ProjectTaskGraphStageSummary,
} from "@/lib/api/projects";

/**
 * PlanTaskGraph renders a coordination plan as a stage-grouped task list with
 * dependency annotations. It degrades gracefully: `nodes` alone (grouped by
 * stage_index) is enough; `edges`, `employees` and `stageSummaries` are optional
 * enrichments. This lets both the project detail (full task-graph) and the task
 * launch detail (flat project_tasks) reuse the same surface.
 */
export type PlanTaskGraphProps = {
  nodes: ProjectTask[];
  edges?: ProjectTaskGraphEdge[];
  employees?: ProjectTaskGraphEmployee[];
  stageSummaries?: ProjectTaskGraphStageSummary[];
  emptyLabel?: string;
};

const UNSTAGED = Number.MAX_SAFE_INTEGER;

export function PlanTaskGraph({
  nodes,
  edges = [],
  employees = [],
  stageSummaries = [],
  emptyLabel = "暂无协调任务计划",
}: PlanTaskGraphProps) {
  if (nodes.length === 0) {
    return (
      <LiquidCard className="rounded-xl p-6 text-sm text-muted-foreground">
        {emptyLabel}
      </LiquidCard>
    );
  }

  const employeeName = new Map(
    employees.map((employee) => [employee.digital_employee_id, employee.display_name]),
  );
  const taskTitle = new Map(nodes.map((node) => [node.id, node.title]));
  const blockersByTask = new Map<string, string[]>();
  for (const edge of edges) {
    const list = blockersByTask.get(edge.dependent_task_id) ?? [];
    list.push(taskTitle.get(edge.blocker_task_id) ?? edge.blocker_task_id);
    blockersByTask.set(edge.dependent_task_id, list);
  }

  const stageTitle = new Map(
    stageSummaries.map((stage) => [stage.stage_index, stage.title]),
  );
  const groups = new Map<number, ProjectTask[]>();
  for (const node of nodes) {
    const stage = node.stage_index ?? UNSTAGED;
    const list = groups.get(stage) ?? [];
    list.push(node);
    groups.set(stage, list);
  }
  const orderedStages = [...groups.keys()].sort((a, b) => a - b);

  return (
    <div className="grid gap-4">
      {orderedStages.map((stage) => {
        const stageNodes = groups.get(stage) ?? [];
        const heading =
          stage === UNSTAGED
            ? "未分阶段"
            : stageTitle.get(stage) || `阶段 ${stage + 1}`;
        return (
          <LiquidCard className="rounded-xl" key={stage}>
            <div className="flex items-center justify-between gap-3 border-b p-4">
              <div className="flex min-w-0 items-center gap-2">
                <SemanticIconTile tone="artifact" size="sm">
                  <GitBranch />
                </SemanticIconTile>
                <div className="min-w-0">
                  <h3 className="text-sm font-semibold tracking-normal">{heading}</h3>
                  <p className="text-xs text-muted-foreground">
                    {stageNodes.length} 个任务
                  </p>
                </div>
              </div>
              <StatusBadge tone="neutral">{stageNodes.length}</StatusBadge>
            </div>
            <div className="divide-y">
              {stageNodes.map((node) => {
                const blockers = blockersByTask.get(node.id) ?? [];
                const assignee = node.assigned_digital_employee_id
                  ? employeeName.get(node.assigned_digital_employee_id) ||
                    node.assigned_digital_employee_id
                  : "未指派";
                return (
                  <div className="grid gap-2 p-4" key={node.id}>
                    <div className="flex items-start justify-between gap-3">
                      <p className="line-clamp-2 text-sm font-medium">{node.title}</p>
                      <StatusBadge tone={taskStatusTone(node.status)}>
                        {node.status}
                      </StatusBadge>
                    </div>
                    {node.summary ? (
                      <p className="line-clamp-2 text-xs text-muted-foreground">
                        {node.summary}
                      </p>
                    ) : null}
                    <div className="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                      <span className="inline-flex items-center gap-1">
                        <Bot className="size-3.5" />
                        {assignee}
                      </span>
                      {node.risk_level ? (
                        <StatusBadge tone={riskTone(node.risk_level)}>
                          {`风险：${node.risk_level}`}
                        </StatusBadge>
                      ) : null}
                      {node.requires_human_approval ? (
                        <StatusBadge tone="warning">
                          <ShieldCheck className="size-3.5" />
                          需审批
                        </StatusBadge>
                      ) : null}
                    </div>
                    {blockers.length > 0 ? (
                      <p className="text-xs text-muted-foreground">
                        {`依赖：${blockers.join("、")}`}
                      </p>
                    ) : null}
                  </div>
                );
              })}
            </div>
          </LiquidCard>
        );
      })}
    </div>
  );
}

function taskStatusTone(status: string): Tone {
  if (["completed", "accepted", "approved", "done", "success"].includes(status)) {
    return "success";
  }
  if (["failed", "rejected", "cancelled", "blocked"].includes(status)) {
    return "danger";
  }
  if (["pending", "waiting", "review_required", "planning_pending"].includes(status)) {
    return "warning";
  }
  if (["dispatchable", "running", "in_progress"].includes(status)) {
    return "info";
  }
  return "neutral";
}

function riskTone(risk: string): Tone {
  if (["high", "critical"].includes(risk)) {
    return "danger";
  }
  if (risk === "medium") {
    return "warning";
  }
  return "neutral";
}

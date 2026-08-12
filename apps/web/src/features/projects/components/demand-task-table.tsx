import { Link } from "@tanstack/react-router";
import {
  DataTable,
  EmptyState,
  SoftCard,
  StatusPill,
  Td,
  Th,
  Tr,
} from "@/components/superteam";
import { cn } from "@/lib/utils";
import type {
  ProjectDemand,
  ProjectDemandDossier,
  ProjectTaskGraph,
  ProjectTaskGraphNode,
} from "@/lib/api/projects";
import {
  demandStatusLabel,
  dispatchGateStatusLabel,
  humanTaskKindLabel,
  taskStatusLabel,
} from "@/lib/status-labels";
import { formatRelativeTime } from "@/lib/format-time";
import { isTerminalTaskStatus } from "@/lib/task-status";
import { demandStatusTone } from "./demand-dossier-header";

const OPEN_DECISION_STATUSES = new Set(["pending", "requested", "waiting", "open"]);

function isOpenDecision(status: string | undefined) {
  return OPEN_DECISION_STATUSES.has((status ?? "").trim().toLowerCase());
}

function taskCurrentHandling(task: ProjectTaskGraphNode, graph: ProjectTaskGraph) {
  const open = (graph.decision_requests ?? []).find(
    (decision) =>
      decision.project_task_id === task.id && isOpenDecision(decision.status_snapshot),
  );
  if (open) {
    return {
      href: true,
      label: `${humanTaskKindLabel(open.decision_type)} · 待决`,
    };
  }
  const gate = (graph.dispatch_gates ?? []).find(
    (item) =>
      item.project_task_id === task.id &&
      item.status !== "passed",
  );
  if (gate) {
    return { href: false, label: dispatchGateStatusLabel(gate.status) };
  }
  if (!isTerminalTaskStatus(task.status) && task.status === "waiting_human") {
    return { href: false, label: "等待人工" };
  }
  return { href: false, label: "—" };
}

function taskAssetLabel(taskId: string, dossier?: ProjectDemandDossier) {
  const railCount =
    dossier?.rail.slots.reduce((sum, slot) => {
      return (
        sum +
        slot.items.filter((item) => item.project_task_id === taskId && item.state === "delivered")
          .length
      );
    }, 0) ?? 0;
  const handoff = dossier?.handoff_summary.assessments.find(
    (item) => item.project_task_id === taskId,
  );
  const delivered =
    handoff?.deliverables.filter((item) => item.verdict === "delivered").length ?? 0;
  const count = Math.max(railCount, delivered);
  if (count <= 0) {
    return null;
  }
  return `工件 ${count}`;
}

type DemandTaskTableProps = {
  demand?: ProjectDemand;
  dossier?: ProjectDemandDossier;
  graph?: ProjectTaskGraph;
  onOpenTask: (taskId: string) => void;
  principalNamesById?: ReadonlyMap<string, string>;
  selectedTaskId?: string;
};

/**
 * 这一单的子任务表：主行是需求流程，子行是 graph nodes。
 * 当前处理 / 关联只连本图与卷宗，不用项目级 20 条任务窗去数。
 */
export function DemandTaskTable({
  demand,
  dossier,
  graph,
  onOpenTask,
  principalNamesById,
  selectedTaskId,
}: DemandTaskTableProps) {
  const nodes = (graph?.nodes ?? []).filter((node) => !node.dismissed_at);

  if (!demand) {
    return (
      <SoftCard className="p-8">
        <EmptyState
          description="提交需求后，这一单拆出的子任务会显示在这里。"
          title="暂无需求流程"
        />
      </SoftCard>
    );
  }

  return (
    <div className="min-w-0" data-testid="demand-task-table">
      <div className="border-b border-line px-4 py-3">
        <h3 className="text-sm font-semibold text-ink">子任务流转</h3>
        <p className="mt-1 text-xs text-ink-2">
          主行是需求流程，子行才是可执行任务。决策与资产挂在子行上。
        </p>
      </div>
      <DataTable>
        <thead>
          <tr>
            <Th className="min-w-[220px]">子任务</Th>
            <Th>状态</Th>
            <Th>员工</Th>
            <Th>当前处理</Th>
            <Th>关联</Th>
            <Th>更新</Th>
          </tr>
        </thead>
        <tbody>
          <Tr>
            <Td className="bg-card-soft" colSpan={6}>
              <span className="font-semibold text-ink">{demand.title}</span>
              <span className="ml-2 text-[12px] font-medium text-ink-3">
                需求流程 · {nodes.length} 条子任务
              </span>
              <StatusPill className="ml-2" tone={demandStatusTone(demand.status)}>
                {demandStatusLabel(demand.status)}
              </StatusPill>
            </Td>
          </Tr>
          {nodes.length === 0 ? (
            <Tr>
              <Td className="text-ink-3" colSpan={6}>
                这一单还没有拆出子任务（可能仍在规划中）。
              </Td>
            </Tr>
          ) : (
            nodes.map((task) => {
              const handling = taskCurrentHandling(task, graph!);
              const asset = taskAssetLabel(task.id, dossier);
              const activityAt = task.updated_at ?? task.created_at;
              const employeeName = task.assigned_digital_employee_id
                ? (principalNamesById?.get(task.assigned_digital_employee_id) ??
                  graph?.employees.find(
                    (employee) => employee.digital_employee_id === task.assigned_digital_employee_id,
                  )?.display_name)
                : undefined;
              return (
                <Tr
                  className={cn(selectedTaskId === task.id && "bg-brand-soft")}
                  data-testid={`demand-task-row-${task.id}`}
                  key={task.id}
                >
                  <Td className="min-w-[200px] pl-5">
                    <span className="mr-1.5 text-ink-3">↳</span>
                    <button
                      className="text-left font-semibold text-brand-deep hover:text-brand"
                      onClick={() => onOpenTask(task.id)}
                      type="button"
                    >
                      {task.title}
                    </button>
                  </Td>
                  <Td>
                    <StatusPill tone="info">{taskStatusLabel(task.status)}</StatusPill>
                  </Td>
                  <Td className="text-xs text-ink-2">
                    {task.assigned_digital_employee_id ? (
                      <Link
                        className={cn(
                          "text-brand-deep hover:text-brand",
                          !employeeName && "font-mono",
                        )}
                        params={{ employeeId: task.assigned_digital_employee_id }}
                        to="/employees/$employeeId"
                      >
                        {employeeName ?? task.assigned_digital_employee_id}
                      </Link>
                    ) : (
                      "未分派"
                    )}
                  </Td>
                  <Td className="text-xs">
                    {handling.label === "—" ? (
                      <span className="text-ink-3">—</span>
                    ) : handling.href ? (
                      <Link className="font-semibold text-brand-deep hover:text-brand" to="/inbox">
                        {handling.label}
                      </Link>
                    ) : (
                      <span className="text-ink-2">{handling.label}</span>
                    )}
                  </Td>
                  <Td>
                    {asset ? (
                      <span className="inline-flex h-5 items-center rounded-[7px] bg-artifact-soft px-1.5 text-[11px] font-bold text-artifact">
                        {asset}
                      </span>
                    ) : (
                      <span className="text-xs text-ink-3">—</span>
                    )}
                  </Td>
                  <Td className="whitespace-nowrap tabular-nums text-xs text-ink-2">
                    {activityAt ? (
                      <time dateTime={activityAt}>{formatRelativeTime(activityAt)}</time>
                    ) : (
                      "—"
                    )}
                  </Td>
                </Tr>
              );
            })
          )}
        </tbody>
      </DataTable>
    </div>
  );
}

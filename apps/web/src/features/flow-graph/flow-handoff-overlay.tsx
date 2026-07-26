import { ArrowRight, GitBranch } from "lucide-react";
import {
  SoftDialog,
  SoftDialogBody,
  SoftDialogContent,
  SoftDialogDescription,
  SoftDialogHeader,
  SoftDialogTitle,
  StatusPill,
} from "@/components/superteam";
import type { Tone } from "@/components/superteam";
import type {
  ProjectExecutionSummary,
  ProjectTaskGraph,
  ProjectTaskGraphHandoffAssessment,
  ProjectTaskGraphNode,
} from "@/lib/api/projects";
import { formatDateTime } from "@/lib/format-time";
import type { FlowLiveEdgeData } from "./flow-graph-adapter";
import { employeeNameForTask, formatValue } from "./inspector-primitives";

type FlowHandoffOverlayProps = {
  /** 选中的活性边（点击边打开）；undefined 时关闭。 */
  edge: FlowLiveEdgeData | undefined;
  graph: ProjectTaskGraph;
  onClose: () => void;
};

/**
 * 交接对照浮层（spec 2026-07-27 §1.3 + §5 P2-V）：左=交接契约（blocker 节点
 * expected_outputs / handoff_contract.acceptance_criteria），右=实际执行结论与
 * 已加载的产出引用（execution_summaries），叠加控制平面按声明交付物核对出的
 * 结构化交接 verdict（handoff_assessments，逐条 delivered/missing 徽章）。
 * 诚实边界：verdict 为 unknown（无声明数据）时维持"暂无"呈现与"不构成符合性
 * 判定"底注，不编造"不符"；不新增请求。
 */
export function FlowHandoffOverlay({ edge, graph, onClose }: FlowHandoffOverlayProps) {
  const blockerTask = edge
    ? graph.nodes.find((node) => node.id === edge.blockerTaskId)
    : undefined;
  const dependentTask = edge
    ? graph.nodes.find((node) => node.id === edge.dependentTaskId)
    : undefined;
  const open = Boolean(edge && blockerTask && dependentTask);
  const assessment =
    blockerTask ?
      graph.handoff_assessments?.find(
        (item) => item.project_task_id === blockerTask.id,
      )
    : undefined;
  const hasVerdict = Boolean(assessment && assessment.status !== "unknown");

  return (
    <SoftDialog onOpenChange={(nextOpen) => !nextOpen && onClose()} open={open}>
      <SoftDialogContent data-testid="flow-handoff-overlay" size="lg">
        {blockerTask && dependentTask ? (
          <>
            <SoftDialogHeader icon={<GitBranch />}>
              <SoftDialogTitle>交接对照</SoftDialogTitle>
              <SoftDialogDescription className="flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5">
                <span className="min-w-0 truncate font-semibold text-ink">
                  {handoffPartyLabel(graph, blockerTask)}
                </span>
                <ArrowRight aria-hidden className="size-3.5 shrink-0 text-brand" />
                <span className="min-w-0 truncate font-semibold text-ink">
                  {handoffPartyLabel(graph, dependentTask)}
                </span>
              </SoftDialogDescription>
            </SoftDialogHeader>
            <SoftDialogBody>
              <div className="grid gap-6 sm:grid-cols-2 sm:gap-8">
                <HandoffContractColumn task={blockerTask} />
                <HandoffActualColumn
                  assessment={hasVerdict ? assessment : undefined}
                  summaries={graph.execution_summaries.filter(
                    (summary) => summary.project_task_id === blockerTask.id,
                  )}
                />
              </div>
              <p className="mt-5 border-t border-line pt-3 text-[11.5px] leading-4 text-ink-3">
                {hasVerdict ?
                  "符合性由控制平面按声明交付物核对。"
                : "对照仅呈现交接契约与已回写的执行结论，不构成符合性判定。"}
              </p>
            </SoftDialogBody>
          </>
        ) : null}
      </SoftDialogContent>
    </SoftDialog>
  );
}

/** 交接方指称：任务名为主，负责数字员工名作补充。 */
function handoffPartyLabel(graph: ProjectTaskGraph, task: ProjectTaskGraphNode): string {
  const employeeName = employeeNameForTask(graph, task);
  return employeeName === "未分配" ? task.title : `${task.title}（${employeeName}）`;
}

function ColumnCaption({ label }: { label: string }) {
  return (
    <p className="text-[11.5px] font-bold uppercase leading-4 tracking-wider text-ink-3">
      {label}
    </p>
  );
}

function HandoffContractColumn({ task }: { task: ProjectTaskGraphNode }) {
  const expectedOutputs = task.expected_outputs;
  const acceptanceCriteria = toItemList(task.handoff_contract["acceptance_criteria"]);

  return (
    <section className="grid content-start gap-2" data-testid="handoff-contract-column">
      <ColumnCaption label="交接契约" />
      {expectedOutputs.length === 0 ? (
        <p className="text-[12.5px] text-ink-3">暂无预期输出</p>
      ) : (
        <ul className="grid gap-1.5">
          {expectedOutputs.map((output, index) => (
            <li
              className="rounded-[10px] border border-line bg-card-soft px-3 py-2 text-[12.5px] leading-5 text-ink"
              key={index}
            >
              {formatValue(output)}
            </li>
          ))}
        </ul>
      )}
      <p className="mt-2 text-[11.5px] font-bold uppercase leading-4 tracking-wider text-ink-3">
        验收判据
      </p>
      {acceptanceCriteria.length === 0 ? (
        <p className="text-[12.5px] text-ink-3">暂无验收判据</p>
      ) : (
        <ul className="grid gap-1">
          {acceptanceCriteria.map((criterion, index) => (
            <li
              className="border-l-2 border-brand/30 pl-2.5 text-[12.5px] leading-5 text-ink-2"
              key={index}
            >
              {formatValue(criterion)}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

/** 交付物 verdict 的中文词表与色调（组件内映射：新枚举仅此浮层消费）。 */
const DELIVERABLE_VERDICT_PRESENTATION: Record<string, { label: string; tone: Tone }> = {
  delivered: { label: "已交付", tone: "ok" },
  missing: { label: "缺失", tone: "danger" },
};

function HandoffDeliverableVerdicts({
  assessment,
}: {
  assessment: ProjectTaskGraphHandoffAssessment;
}) {
  return (
    <div className="grid gap-1.5" data-testid="handoff-deliverable-verdicts">
      <p className="text-[11.5px] font-bold uppercase leading-4 tracking-wider text-ink-3">
        声明交付物核对
      </p>
      <ul className="grid gap-1.5">
        {assessment.deliverables.map((deliverable, index) => {
          const presentation =
            DELIVERABLE_VERDICT_PRESENTATION[deliverable.verdict] ??
            DELIVERABLE_VERDICT_PRESENTATION.missing;
          return (
            <li
              className="flex min-w-0 items-center justify-between gap-2 rounded-[10px] border border-line bg-card-soft px-3 py-2"
              key={`${deliverable.name}-${index}`}
            >
              <span className="min-w-0 text-[12.5px] leading-5 text-ink">
                <span className="break-words font-medium">
                  {deliverable.name || "未命名交付物"}
                </span>
                {deliverable.summary ? (
                  <span className="ml-1.5 break-all text-[11.5px] text-ink-3">
                    {deliverable.summary}
                  </span>
                ) : null}
              </span>
              <StatusPill className="shrink-0" tone={presentation.tone}>
                {presentation.label}
              </StatusPill>
            </li>
          );
        })}
      </ul>
    </div>
  );
}

function HandoffActualColumn({
  assessment,
  summaries,
}: {
  assessment: ProjectTaskGraphHandoffAssessment | undefined;
  summaries: ProjectExecutionSummary[];
}) {
  return (
    <section className="grid content-start gap-2" data-testid="handoff-actual-column">
      <ColumnCaption label="实际执行" />
      {assessment ? <HandoffDeliverableVerdicts assessment={assessment} /> : null}
      {summaries.length === 0 ? (
        <p className="text-[12.5px] text-ink-3">暂无产出（上游任务尚未回写执行结论）</p>
      ) : (
        <ul className="grid gap-2">
          {summaries.map((summary) => {
            const artifactRefs = toItemList(summary.artifact_refs);
            return (
              <li
                className="rounded-[10px] border border-line bg-card-soft px-3 py-2.5"
                key={summary.id}
              >
                <blockquote className="border-l-2 border-brand/40 pl-2.5 text-[12.5px] leading-5 text-ink">
                  {summary.conclusion || "暂无执行结论"}
                </blockquote>
                {artifactRefs.length > 0 ? (
                  <p className="mt-1.5 break-words text-[11.5px] leading-4 text-ink-2">
                    产出：{artifactRefs.map((ref) => formatValue(ref)).join("、")}
                  </p>
                ) : (
                  <p className="mt-1.5 text-[11.5px] leading-4 text-ink-3">暂无产出引用</p>
                )}
                {summary.created_at ? (
                  <p className="mt-1 text-[11px] tabular-nums text-ink-3">
                    交付时间{" "}
                    <time dateTime={summary.created_at}>
                      {formatDateTime(summary.created_at)}
                    </time>
                  </p>
                ) : null}
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}

/** 未知形状的契约字段收敛为条目列表：数组展开、单值包装、空值为空列表。 */
function toItemList(value: unknown): unknown[] {
  if (Array.isArray(value)) return value;
  if (value === undefined || value === null || value === "") return [];
  return [value];
}

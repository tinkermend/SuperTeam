import { StatusPill } from "@/components/superteam";
import { cn } from "@/lib/utils";
import type { ProjectDemand, ProjectDemandDossier, ProjectTaskGraph } from "@/lib/api/projects";
import { demandStatusLabel } from "@/lib/status-labels";
import { demandStatusTone } from "./demand-dossier-header";

type DemandStageRiverProps = {
  acceptance?: ProjectDemandDossier["acceptance"];
  continueAction?: {
    available: boolean;
    reasonMessage?: string;
    onContinue: () => void;
  };
  demand?: ProjectDemand;
  exitLabel?: string | null;
  graph?: ProjectTaskGraph;
  onPlanClick?: () => void;
  planOpen?: boolean;
  playbookName?: string | null;
};

function stageClass(active: boolean, warn = false) {
  return cn(
    "min-w-0 rounded-[10px] border bg-card px-2.5 py-2",
    active && !warn && "border-brand shadow-[inset_3px_0_0_var(--brand)]",
    active && warn && "border-warn/40 shadow-[inset_3px_0_0_var(--warn)]",
    !active && "border-line",
  );
}

/**
 * 这一单的阶段河：需求 → 计划 → 执行 → 结果。
 * 只做导向，不替任务表。四格始终一行；接续入口挂在河下方。
 */
export function DemandStageRiver({
  acceptance,
  continueAction,
  demand,
  exitLabel,
  graph,
  onPlanClick,
  planOpen = false,
  playbookName,
}: DemandStageRiverProps) {
  if (!demand) {
    return null;
  }
  const status = demand.status;
  const demandDone = ![
    "submitted",
    "recorded",
  ].includes(status);
  const planDone = ![
    "submitted",
    "recorded",
    "planning_pending",
    "planning_failed",
  ].includes(status);
  const executing = status === "executing";
  const resultActive =
    status === "acceptance_pending" ||
    status === "completed" ||
    status === "failed" ||
    status === "cancelled";

  const stages = graph?.stage_summaries ?? [];
  const waiting = stages.reduce((sum, stage) => sum + (stage.waiting_human_nodes ?? 0), 0);
  const running = stages.reduce((sum, stage) => sum + (stage.running_nodes ?? 0), 0);
  const criteriaTotal = acceptance?.criteria_total ?? 0;
  const pendingHuman = acceptance?.pending_human_judgment ?? 0;
  const intent =
    demand.content && demand.content !== demand.title ? demand.content.trim() : "";

  return (
    <div className="border-b border-line bg-card-soft" data-testid="demand-stage-river">
      <section
        aria-label="本单阶段"
        className="grid grid-cols-4 gap-2 px-3 py-2.5"
      >
        <div className={stageClass(status === "submitted" || status === "recorded")}>
          <p className="text-[11px] font-bold tracking-wide text-ink-3">1 · 需求</p>
          <p className="mt-0.5 truncate text-[12.5px] font-semibold text-ink">
            {demandDone ? "已受理" : demandStatusLabel(status)}
          </p>
          <p className="mt-0.5 truncate text-[11px] text-ink-2">{demand.title}</p>
        </div>
        {onPlanClick ? (
          <button
            aria-expanded={planOpen}
            className={cn(stageClass(status === "planning_pending" || status === "planned", status === "planning_failed"), "text-left")}
            data-testid="demand-river-plan"
            onClick={onPlanClick}
            type="button"
          >
            <p className="text-[11px] font-bold tracking-wide text-ink-3">2 · 计划</p>
            <p className="mt-0.5 truncate text-[12.5px] font-semibold text-ink">
              {status === "planning_failed"
                ? "规划失败"
                : planDone
                  ? "计划已确认"
                  : demandStatusLabel(status)}
            </p>
            <p className="mt-0.5 text-[11px] text-ink-2">
              {planOpen ? "收起计划确认" : "展开计划确认"}
            </p>
          </button>
        ) : (
          <div className={stageClass(status === "planning_pending" || status === "planned", status === "planning_failed")}>
            <p className="text-[11px] font-bold tracking-wide text-ink-3">2 · 计划</p>
            <p className="mt-0.5 truncate text-[12.5px] font-semibold text-ink">
              {status === "planning_failed"
                ? "规划失败"
                : planDone
                  ? "计划已确认"
                  : demandStatusLabel(status)}
            </p>
            <StatusPill className="mt-1" tone={demandStatusTone(status)}>
              {demandStatusLabel(status)}
            </StatusPill>
          </div>
        )}
        <div className={stageClass(executing, waiting > 0)}>
          <p className="text-[11px] font-bold tracking-wide text-ink-3">3 · 执行</p>
          <p className="mt-0.5 truncate text-[12.5px] font-semibold text-ink">
            {executing
              ? waiting > 0
                ? `${running} 进行 / ${waiting} 等人`
                : running > 0
                  ? `${running} 个任务执行中`
                  : "执行中"
              : resultActive || planDone
                ? "执行阶段"
                : "尚未执行"}
          </p>
          <p className="mt-0.5 truncate text-[11px] text-ink-2">
            {waiting > 0 ? "有任务等待人工" : "子任务见下方任务表"}
          </p>
        </div>
        <div className={stageClass(resultActive)}>
          <p className="text-[11px] font-bold tracking-wide text-ink-3">4 · 结果</p>
          <p className="mt-0.5 truncate text-[12.5px] font-semibold text-ink">
            {status === "completed"
              ? "已验收"
              : status === "acceptance_pending"
                ? "待验收"
                : status === "failed"
                  ? "已失败"
                  : "验收未开始"}
          </p>
          <p className="mt-0.5 truncate text-[11px] text-ink-2">
            {criteriaTotal > 0
              ? `判据 ${Math.max(criteriaTotal - pendingHuman, 0)} / ${criteriaTotal}`
              : "判据见资产页签"}
          </p>
        </div>
      </section>
      {intent || playbookName || exitLabel || continueAction ? (
        <div className="flex flex-wrap items-start justify-between gap-2 border-t border-line px-3 py-2">
          <div className="min-w-0 flex-1">
            {intent ? (
              <p className="line-clamp-2 text-[12.5px] leading-5 text-ink-2">{intent}</p>
            ) : null}
            {playbookName || exitLabel ? (
              <p className="mt-1 flex flex-wrap gap-x-2 gap-y-1 text-[11px] text-ink-3">
                {playbookName ? <span>剧本 · {playbookName}</span> : null}
                {exitLabel ? <span>{exitLabel}</span> : null}
              </p>
            ) : null}
          </div>
          {continueAction?.available ? (
            <button
              className="shrink-0 rounded-[10px] border border-line-strong bg-card px-2.5 py-1.5 text-[12px] font-semibold text-ink hover:bg-card-soft"
              data-testid="demand-river-continue"
              onClick={continueAction.onContinue}
              type="button"
            >
              继续这一单
            </button>
          ) : continueAction && !continueAction.available && continueAction.reasonMessage ? (
            <span
              className="max-w-56 shrink-0 text-right text-[11px] leading-4 text-ink-3"
              data-testid="demand-river-continue-blocked"
            >
              {continueAction.reasonMessage}
            </span>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}

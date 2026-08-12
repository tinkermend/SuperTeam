import { Link } from "@tanstack/react-router";
import { ArrowUpRight, CornerDownRight, Inbox } from "lucide-react";

import { Button, Callout, SoftCard, StatusPill } from "@/components/superteam";
import { demandStatusLabel, decisionTypeLabel } from "@/lib/status-labels";
import type { ProjectDemandDossier } from "@/lib/api/projects";

export type DemandDossierView = "timeline" | "graph";

/** 需求状态 → pill tone。需求列表与单头共用，避免同一状态两种颜色。 */
export function demandStatusTone(status: string) {
  if (status === "completed") return "ok" as const;
  if (status === "failed" || status === "cancelled") return "danger" as const;
  if (status === "acceptance_pending") return "warn" as const;
  if (status === "executing" || status === "planned") return "info" as const;
  return "mute" as const;
}

/**
 * 本单收口的展示文案。收口是「这一单走多深」的唯一结构化表达，只显示剧本名
 * 等于只说了按哪套打法、没说打到哪一步。
 *
 * 待确认的计划必须标「拟」：把还没人点头的承诺显示成既成事实，正是这套治理
 * 要防的事。标签缺失时退回技术键——不编中文，也不吞掉收口。
 */
export function demandDossierExitText(playbook: {
  exit_deliverable?: string;
  exit_label?: string;
  exit_pending?: boolean;
}) {
  const deliverable = playbook.exit_deliverable?.trim();
  if (!deliverable) return null;
  const label = playbook.exit_label?.trim() || deliverable;
  return playbook.exit_pending
    ? { text: `拟收口 · ${label}`, title: `计划待确认，收口尚未生效：${label}` }
    : { text: `收口 · ${label}`, title: `本单收口：${label}` };
}

/**
 * 一单卷宗单头：身份 + 有效剧本 + 本单收口 + 接续链 + 待你处理。
 * 接续主按钮在阶段河；这里只保留链导航与待办提示。
 * 「驱动/巡检」密度切换已退役——时间线疏密由 signals 自动推导。
 */
export function DemandDossierHeader({
  dossier,
  hideViewToggle = false,
  onSelectDemand,
}: {
  dossier: ProjectDemandDossier;
  /** 时间线/流程图已拆成独立页签时关掉单头切换（遗留 prop，页签拆分后恒为 true）。 */
  hideViewToggle?: boolean;
  onSelectDemand?: (demandId: string) => void;
}) {
  void hideViewToggle;
  const demand = dossier.demand;
  const lineage = dossier.lineage;
  const chainLength = lineage?.chain_length ?? 1;
  const chainPosition = lineage?.chain_position ?? 1;
  const playbookName = dossier.effective_playbook.name?.trim();
  const showPlaybook =
    dossier.effective_playbook.source !== "none" && Boolean(playbookName);
  const pending = dossier.pending_actions ?? [];
  const exit = demandDossierExitText(dossier.effective_playbook);

  return (
    <SoftCard className="p-4" data-testid="demand-dossier-header">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="min-w-0 truncate text-base font-semibold text-ink">
              {demand.title}
            </h3>
            <StatusPill tone={demandStatusTone(demand.status)}>
              {demandStatusLabel(demand.status)}
            </StatusPill>
            {showPlaybook ? (
              <span className="rounded-full bg-card-soft px-2 py-0.5 text-[11.5px] text-ink-2">
                剧本 · {playbookName}
              </span>
            ) : null}
            {exit ? (
              <span
                className="rounded-full bg-card-soft px-2 py-0.5 text-[11.5px] text-ink-2"
                data-testid="demand-dossier-exit"
                title={exit.title}
              >
                {exit.text}
              </span>
            ) : null}
          </div>
          {chainLength > 1 ? (
            <nav
              aria-label="接续链"
              className="mt-1.5 flex flex-wrap items-center gap-1.5 text-[11.5px] text-ink-3"
              data-testid="demand-dossier-chain"
            >
              <CornerDownRight aria-hidden className="size-3 shrink-0" />
              <span>
                本单为第 {chainPosition} / {chainLength} 次
              </span>
              {lineage.chain.map((item) => (
                <button
                  aria-current={item.is_current ? "true" : undefined}
                  className={[
                    "rounded-full px-2 py-0.5",
                    item.is_current
                      ? "bg-accent-soft font-semibold text-ink"
                      : "bg-card-soft text-ink-2 hover:text-ink",
                  ].join(" ")}
                  disabled={item.is_current || !onSelectDemand}
                  key={item.demand_id}
                  onClick={() => onSelectDemand?.(item.demand_id)}
                  title={`${item.title}（${demandStatusLabel(item.status)}）`}
                  type="button"
                >
                  {item.is_current ? "本单" : item.title}
                </button>
              ))}
            </nav>
          ) : null}
          {demand.content && demand.content !== demand.title ? (
            <p className="mt-1.5 line-clamp-3 text-[13px] leading-6 text-ink-2">
              {demand.content}
            </p>
          ) : null}
        </div>
      </div>

      {pending.length > 0 ? (
        <Callout
          action={
            <Button asChild size="sm" variant="primary">
              <Link aria-label="去收件箱处理待你处理事项" to="/inbox">
                <Inbox className="size-3.5" />
                去处理
                <ArrowUpRight className="size-3.5" />
              </Link>
            </Button>
          }
          className="mt-3"
          data-testid="demand-dossier-pending"
          description={pending
            .slice(0, 2)
            .map((action) => action.title || decisionTypeLabel(action.kind))
            .join(" · ")}
          title={`待你处理 · ${pending.length}`}
          tone="warn"
        />
      ) : null}
    </SoftCard>
  );
}

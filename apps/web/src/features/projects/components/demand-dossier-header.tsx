import { Link } from "@tanstack/react-router";
import { ArrowUpRight, Inbox } from "lucide-react";

import { Button, Callout, Segmented, SoftCard, StatusPill } from "@/components/superteam";
import { demandStatusLabel, decisionTypeLabel, dossierDensityLabel } from "@/lib/status-labels";
import type { ProjectDemandDossier } from "@/lib/api/projects";

import type { DossierDensity } from "./demand-dossier-density";

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
 * 一单卷宗单头：身份 + 有效剧本 + 待你处理 + 视图/密度切换。
 *
 * 刻意**不放**「继续此任务」——同单接续能力属后续版本；放一颗禁用按钮不是留
 * 挂点，是留一颗点不动的承诺按钮。挂点留在契约与组件边界上。
 */
export function DemandDossierHeader({
  density,
  dossier,
  onDensityChange,
  onViewChange,
  view,
}: {
  density: DossierDensity;
  dossier: ProjectDemandDossier;
  onDensityChange: (density: DossierDensity) => void;
  onViewChange: (view: DemandDossierView) => void;
  view: DemandDossierView;
}) {
  const demand = dossier.demand;
  const playbookName = dossier.effective_playbook.name?.trim();
  const showPlaybook =
    dossier.effective_playbook.source !== "none" && Boolean(playbookName);
  const pending = dossier.pending_actions ?? [];

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
          </div>
          {demand.content && demand.content !== demand.title ? (
            <p className="mt-1.5 line-clamp-3 text-[13px] leading-6 text-ink-2">
              {demand.content}
            </p>
          ) : null}
        </div>

        <div className="flex shrink-0 flex-wrap items-center gap-2">
          <Segmented
            aria-label="切换视图"
            data-testid="demand-dossier-view-toggle"
            onChange={onViewChange}
            options={[
              { label: "时间线", value: "timeline" },
              { label: "流程图", value: "graph" },
            ]}
            value={view}
          />
          <Segmented
            aria-label="切换呈现密度"
            data-testid="demand-dossier-density-toggle"
            onChange={onDensityChange}
            options={[
              { label: dossierDensityLabel("drive"), value: "drive" },
              { label: dossierDensityLabel("inspect"), value: "inspect" },
            ]}
            value={density}
          />
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

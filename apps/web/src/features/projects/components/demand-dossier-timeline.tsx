import { useState } from "react";
import { Link } from "@tanstack/react-router";
import { History } from "lucide-react";

import {
  EmptyState,
  SoftCard,
  Timeline,
  type TimelineItemData,
} from "@/components/superteam";
import { formatRelativeTime } from "@/lib/format-time";
import type { DemandDossierSeverity, ProjectDemandDossier } from "@/lib/api/projects";

import {
  DOSSIER_INSPECT_TIMELINE_PREVIEW,
  type DossierDensity,
} from "./demand-dossier-density";

/** severity → Timeline tone。服务端已给语义，前端不再按 kind 猜颜色。 */
function severityTone(severity: DemandDossierSeverity | undefined) {
  switch (severity) {
    case "success":
      return "ok" as const;
    case "warn":
      return "warn" as const;
    case "danger":
      return "danger" as const;
    case "info":
      return "info" as const;
    default:
      return "mute" as const;
  }
}

/**
 * 一单卷宗中栏时间线。
 *
 * 诚实边界（spec §4.4，必做）：这是**协调叙事**——按关键节点归纳、噪音事件不入，
 * 不是完整审计流水。标题旁常显说明 + 「执行轨迹」出口，避免用户把它当成完整
 * 事件记录，进而在"少了一条"时怀疑整个平台的数据可信度。
 *
 * 文案一律用服务端已渲染的 `title`/`summary`，前端不解析原始 event_type。
 */
export function DemandDossierTimeline({
  density,
  onOpenTask,
  projectId,
  timeline,
}: {
  density: DossierDensity;
  onOpenTask: (taskId: string) => void;
  projectId: string;
  timeline: ProjectDemandDossier["timeline"];
}) {
  const [expanded, setExpanded] = useState(false);
  const items = timeline.items ?? [];
  const collapsed = density === "inspect" && !expanded;
  const visible = collapsed ? items.slice(0, DOSSIER_INSPECT_TIMELINE_PREVIEW) : items;

  const timelineItems: TimelineItemData[] = visible.map((item) => {
    const taskId = item.open_target?.type === "task_detail" ? item.open_target.task_id : undefined;
    const title = taskId ? (
      <button
        className="text-left text-brand hover:underline"
        onClick={() => onOpenTask(taskId)}
        type="button"
      >
        {item.title}
      </button>
    ) : (
      item.title
    );
    return {
      description: item.summary,
      id: item.id,
      meta: item.actor_display_name ? `处理人 · ${item.actor_display_name}` : undefined,
      time: item.occurred_at ? (
        <time dateTime={item.occurred_at} title={item.occurred_at}>
          {formatRelativeTime(item.occurred_at)}
        </time>
      ) : undefined,
      title,
      tone: severityTone(item.severity),
    };
  });

  return (
    <SoftCard className="p-4" data-testid="demand-dossier-timeline">
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <History className="size-4 text-ink-2" />
            <h3 className="text-sm font-semibold text-ink">协调时间线</h3>
          </div>
          <p className="mt-1 text-[11.5px] leading-5 text-ink-3">
            协调叙事视图，按关键节点归纳；完整执行事件见
            <Link
              className="mx-1 text-brand hover:underline"
              params={{ projectId }}
              search={{ tab: "trace" }}
              to="/projects/$projectId"
            >
              执行轨迹
            </Link>
            。
          </p>
        </div>
        {timeline.truncated ? (
          <span className="shrink-0 rounded-full bg-card-soft px-2 py-0.5 text-[11px] text-ink-3">
            仅显示最近若干条
          </span>
        ) : null}
      </div>

      {items.length === 0 ? (
        <div className="mt-3">
          <EmptyState
            description="需求已受理，协调尚未产生可展示节点；有进展后会按时间倒序出现在这里。"
            title="协调尚未产生可展示节点"
          />
        </div>
      ) : (
        <>
          <Timeline className="mt-4" items={timelineItems} />
          {collapsed && items.length > visible.length ? (
            <button
              className="mt-2 text-[12px] font-semibold text-brand hover:underline"
              data-testid="demand-dossier-timeline-expand"
              onClick={() => setExpanded(true)}
              type="button"
            >
              展开全部 {items.length} 条
            </button>
          ) : null}
        </>
      )}
    </SoftCard>
  );
}

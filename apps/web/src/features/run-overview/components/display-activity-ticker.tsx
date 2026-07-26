import { GlassCard } from "@/components/superteam";
import { formatTime } from "../formatters";
import type { RuntimeOverviewActivityItem } from "../runtime-overview-model";

const tickerDotClass: Record<string, string> = {
  cancelled: "bg-mute",
  completed: "bg-ok",
  failed: "bg-danger",
  running: "bg-info"
};

// 大屏底部动态流:最新活动单行滚动区(数据随 SSE/轮询更新,不做营销式跑马灯动效)。
export function DisplayActivityTicker({ items }: { items?: RuntimeOverviewActivityItem[] }) {
  if (!items || items.length === 0) return null;
  return (
    <GlassCard className="mt-4 p-3" data-display-activity-ticker>
      <ul className="flex items-center gap-6 overflow-hidden whitespace-nowrap">
        {items.slice(0, 6).map((item, index) => (
          <li key={`${item.employeeId}-${item.label}-${index}`} className="flex min-w-0 shrink-0 items-center gap-2 text-base">
            <span className={`size-2.5 shrink-0 rounded-full ${tickerDotClass[item.status] ?? "bg-mute"}`} aria-hidden />
            <span className="font-medium text-ink">{item.employeeName}</span>
            <span className="text-ink-2">
              {item.label}
              {item.taskTitle ? ` · ${item.taskTitle}` : ""}
            </span>
            {item.occurredAt ? <span className="text-sm tabular-nums text-ink-3">{formatTime(item.occurredAt)}</span> : null}
          </li>
        ))}
      </ul>
    </GlassCard>
  );
}

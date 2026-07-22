import { useMemo } from "react";
import { Link } from "@tanstack/react-router";
import { ArrowRight, CalendarClock, Inbox, Activity } from "lucide-react";
import { IconTile, StatusPill, V3EmptyState } from "@/components/superteam";
import type { AutomationFire, AutomationRule } from "@/lib/api/automations";
import type { InboxItem } from "@/lib/api/inbox";
import { formatRelativeFuture, formatRelativeTime } from "@/lib/format-time";
import { statusLabel } from "@/lib/status-labels";
import { cn } from "@/lib/utils";
import { automationFireTone } from "../fire-tone";

const PANEL_CLASS =
  "flex min-w-0 flex-col gap-2 rounded-[14px] border border-v3-line bg-v3-card p-4 shadow-sm";

const RAIL_UPCOMING_LIMIT = 8;

export type UpcomingFireRow = {
  rule: AutomationRule;
  nextAt: string;
};

export function buildUpcomingFires(
  rules: AutomationRule[],
  options?: {
    withinHours?: number;
    now?: Date;
    limit?: number;
    nextFireById?: Map<string, string>;
  },
): UpcomingFireRow[] {
  const withinHours = options?.withinHours ?? 72;
  const now = options?.now ?? new Date();
  const limit = options?.limit ?? RAIL_UPCOMING_LIMIT;
  const horizon = now.getTime() + withinHours * 3_600_000;
  const rows = rules
    .map((rule) => {
      const nextAt = options?.nextFireById?.get(rule.id) ?? null;
      return nextAt ? { rule, nextAt } : null;
    })
    .filter((row): row is UpcomingFireRow => {
      if (!row) return false;
      const ts = Date.parse(row.nextAt);
      return !Number.isNaN(ts) && ts <= horizon && ts > now.getTime();
    })
    .sort((a, b) => Date.parse(a.nextAt) - Date.parse(b.nextAt));

  if (limit === Number.POSITIVE_INFINITY || limit <= 0) return rows;
  return rows.slice(0, limit);
}

/** Uncapped count of enabled rules due within the horizon (for fact strip). */
export function countUpcomingFires(
  rules: AutomationRule[],
  nextFireById: Map<string, string>,
  withinHours = 72,
  now = new Date(),
): number {
  return buildUpcomingFires(rules, {
    withinHours,
    now,
    limit: Number.POSITIVE_INFINITY,
    nextFireById,
  }).length;
}

export function buildRecentFireActivity(rules: AutomationRule[], limit = 8) {
  return rules
    .filter((rule) => rule.latest_fire)
    .map((rule) => ({ rule, fire: rule.latest_fire as AutomationFire }))
    .sort(
      (a, b) =>
        Date.parse(b.fire.scheduled_fire_at) - Date.parse(a.fire.scheduled_fire_at),
    )
    .slice(0, limit);
}

type AutomationDashboardRailProps = {
  rules: AutomationRule[];
  nextFireById: Map<string, string>;
  decisionItems: InboxItem[];
  decisionOpenCount: number;
  decisionsLoading?: boolean;
  onSelectRule: (ruleId: string) => void;
};

export function AutomationDashboardRail({
  rules,
  nextFireById,
  decisionItems,
  decisionOpenCount,
  decisionsLoading,
  onSelectRule,
}: AutomationDashboardRailProps) {
  const upcoming = useMemo(
    () => buildUpcomingFires(rules, { nextFireById, limit: RAIL_UPCOMING_LIMIT }),
    [nextFireById, rules],
  );
  const recent = useMemo(() => buildRecentFireActivity(rules), [rules]);

  const showDecisionsLoading =
    Boolean(decisionsLoading) ||
    (decisionOpenCount > 0 && decisionItems.length === 0);

  return (
    <aside
      aria-label="自动化工作台右栏"
      className="flex min-w-0 flex-col gap-4 @4xl/master-detail:sticky @4xl/master-detail:top-4 @4xl/master-detail:max-h-[calc(100svh-2rem)] @4xl/master-detail:overflow-y-auto"
      data-testid="automations-dashboard-rail"
    >
      <section aria-label="即将触发" className={PANEL_CLASS}>
        <header className="flex items-center justify-between gap-2">
          <h3 className="text-sm font-extrabold text-v3-ink">即将触发 · 72h</h3>
          <CalendarClock aria-hidden className="size-3.5 text-v3-ink-3" />
        </header>
        {upcoming.length === 0 ? (
          <p className="text-[12px] leading-5 text-v3-ink-3">
            近三天没有启用中的日程，或规则尚未配置。
          </p>
        ) : (
          <ul className="flex min-w-0 flex-col gap-1.5">
            {upcoming.map(({ rule, nextAt }) => (
              <li key={rule.id}>
                <button
                  className="flex w-full min-w-0 items-start gap-2 rounded-[10px] border border-transparent px-2 py-1.5 text-left transition-colors hover:border-v3-line hover:bg-v3-soft"
                  type="button"
                  onClick={() => onSelectRule(rule.id)}
                >
                  <IconTile size="sm" tone="brand">
                    <CalendarClock />
                  </IconTile>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-[13px] font-medium text-v3-ink">
                      {rule.name}
                    </span>
                    <span className="block truncate text-[11px] text-v3-ink-3">
                      {rule.project_name?.trim() || "项目"} ·{" "}
                      {statusLabel(rule.coordination_mode)}
                    </span>
                  </span>
                  <time
                    className="shrink-0 text-[11px] tabular-nums text-v3-ink-2"
                    dateTime={nextAt}
                  >
                    {formatRelativeFuture(nextAt)}
                  </time>
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section aria-label="待你处理" className={PANEL_CLASS} data-testid="automations-gate-panel">
        <header className="flex items-center justify-between gap-2">
          <h3 className="text-sm font-extrabold text-v3-ink">
            待你处理{decisionOpenCount > 0 ? ` · ${decisionOpenCount}` : ""}
          </h3>
          <Link
            className="flex shrink-0 items-center gap-1 text-[12px] font-medium text-v3-brand hover:opacity-75"
            to="/inbox"
          >
            收件箱
            <ArrowRight aria-hidden className="size-3" />
          </Link>
        </header>
        <p className="text-[11px] leading-4 text-v3-ink-3">
          自动化到点后仍可能等人确认计划或验收；与手发需求同一收件箱 / 飞书投影。
        </p>
        {showDecisionsLoading ? (
          <p className="text-[12px] leading-5 text-v3-ink-3">正在加载决策事项…</p>
        ) : decisionItems.length === 0 ? (
          <V3EmptyState
            description="当前没有等待你处理的审批或人工决策。"
            icon={<Inbox />}
            title="闸门空闲"
          />
        ) : (
          <ul className="flex min-w-0 flex-col gap-1.5">
            {decisionItems.slice(0, 5).map((item) => (
              <li key={item.id}>
                <Link
                  className="flex min-w-0 items-start gap-2 rounded-[10px] border border-transparent px-2 py-1.5 transition-colors hover:border-v3-line hover:bg-v3-soft"
                  to="/inbox"
                >
                  <IconTile size="sm" tone={item.risk_level === "high" ? "danger" : "warn"}>
                    <Inbox />
                  </IconTile>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-[13px] font-medium text-v3-ink">
                      {item.title}
                    </span>
                    <span className="block truncate text-[11px] text-v3-ink-3">
                      {item.source_project_name?.trim() || "项目"} ·{" "}
                      {formatRelativeTime(item.last_activity_at)}
                    </span>
                  </span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section aria-label="最近触发" className={PANEL_CLASS}>
        <header className="flex items-center gap-2">
          <h3 className="text-sm font-extrabold text-v3-ink">最近触发</h3>
          <Activity aria-hidden className="size-3.5 text-v3-ink-3" />
        </header>
        {recent.length === 0 ? (
          <p className="text-[12px] leading-5 text-v3-ink-3">
            还没有触发记录。可在规则详情里点「立即试跑」。
          </p>
        ) : (
          <ul className="flex min-w-0 flex-col gap-1.5">
            {recent.map(({ rule, fire }) => {
              const tone = automationFireTone(fire.status);
              return (
                <li key={fire.id}>
                  <button
                    className="flex w-full min-w-0 items-start gap-2 rounded-[10px] border border-transparent px-2 py-1.5 text-left transition-colors hover:border-v3-line hover:bg-v3-soft"
                    type="button"
                    onClick={() => onSelectRule(rule.id)}
                  >
                    <span
                      aria-hidden
                      className={cn(
                        "mt-1.5 size-1.5 shrink-0 rounded-full",
                        tone === "ok" && "bg-v3-ok",
                        tone === "danger" && "bg-v3-danger",
                        tone === "warn" && "bg-v3-warn",
                        tone === "info" && "bg-v3-info",
                        tone === "mute" && "bg-v3-ink-3",
                      )}
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-[13px] font-medium text-v3-ink">
                        {rule.name}
                      </span>
                      <span className="flex flex-wrap items-center gap-1.5 text-[11px] text-v3-ink-3">
                        <StatusPill tone={tone}>{statusLabel(fire.status)}</StatusPill>
                        <time className="tabular-nums" dateTime={fire.scheduled_fire_at}>
                          {formatRelativeTime(fire.scheduled_fire_at)}
                        </time>
                      </span>
                    </span>
                  </button>
                </li>
              );
            })}
          </ul>
        )}
      </section>
    </aside>
  );
}

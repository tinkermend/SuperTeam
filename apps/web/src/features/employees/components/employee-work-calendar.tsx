import { useState } from "react";
import { addDays, format, isSameDay, isToday, startOfWeek } from "date-fns";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { Link } from "@tanstack/react-router";
import {
  StatusPill,
  Button,
  Chip,
  EmptyState,
  StateSurface,
  WorkSurface,
  type Tone
} from "@/components/superteam";
import type { DigitalEmployeeRunCalendarItem, DigitalEmployeeRunStatus } from "@/lib/api/employees";
import { runStatusLabel } from "@/lib/status-labels";

const PREVIEW_PER_DAY = 5;

const runStatusTone: Record<DigitalEmployeeRunStatus, Tone> = {
  queued: "mute",
  dispatching: "mute",
  running: "info",
  cancelling: "warn",
  completed: "ok",
  failed: "danger",
  cancelled: "warn",
  timed_out: "danger"
};

export type EmployeeWorkCalendarProps = {
  weekStart: Date;
  onWeekChange: (nextWeekStart: Date) => void;
  items: DigitalEmployeeRunCalendarItem[];
  totalCount: number;
  truncated?: boolean;
  isLoading?: boolean;
  isError?: boolean;
  error?: unknown;
  onRetry: () => void;
  onItemClick: (item: DigitalEmployeeRunCalendarItem) => void;
};

export function employeeWeekStart(anchor: Date = new Date()): Date {
  return startOfWeek(anchor, { weekStartsOn: 1 });
}

export function employeeWeekEndExclusive(weekStart: Date): Date {
  return addDays(weekStart, 7);
}

/**
 * Absolute [from, to) covering one local calendar week (Mon 00:00 → next Mon 00:00).
 * Callers must pass these ISO strings to the run-calendar API so timestamptz filters
 * align with local-day bucketing in the UI (not UTC-midnight day boundaries).
 */
export function employeeWeekQueryWindow(weekStart: Date): { from: string; to: string } {
  const from = employeeWeekStart(weekStart);
  const to = employeeWeekEndExclusive(from);
  return { from: from.toISOString(), to: to.toISOString() };
}

export function EmployeeWorkCalendar({
  weekStart,
  onWeekChange,
  items,
  totalCount,
  truncated = false,
  isLoading,
  isError,
  error,
  onRetry,
  onItemClick
}: EmployeeWorkCalendarProps) {
  const [expandedDays, setExpandedDays] = useState<Set<string>>(() => new Set());
  const days = Array.from({ length: 7 }, (_, index) => addDays(weekStart, index));
  const weekLabel = `${format(weekStart, "M/d")} - ${format(addDays(weekStart, 6), "M/d")}`;
  const byDay = groupItemsByLocalDay(items);
  const showEmptyWeek = !isLoading && !isError && totalCount === 0 && items.length === 0;

  return (
    <WorkSurface className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <div className="flex shrink-0 flex-col gap-2 border-b border-line px-3 py-2.5 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex flex-wrap items-center gap-2">
          <p className="text-sm font-semibold text-ink tabular-nums">{format(weekStart, "yyyy/MM/dd")}</p>
          <div className="flex items-center gap-1">
            <Button
              aria-label="上一周"
              onClick={() => onWeekChange(addDays(weekStart, -7))}
              size="sm"
              type="button"
              variant="outline"
            >
              <ChevronLeft className="size-4" />
            </Button>
            <span className="min-w-[7.5rem] text-center text-sm text-ink-2 tabular-nums">{weekLabel}</span>
            <Button
              aria-label="下一周"
              onClick={() => onWeekChange(addDays(weekStart, 7))}
              size="sm"
              type="button"
              variant="outline"
            >
              <ChevronRight className="size-4" />
            </Button>
          </div>
          <Chip active onClick={() => onWeekChange(employeeWeekStart(new Date()))} type="button">
            本周
          </Chip>
        </div>
        <p className="text-xs text-ink-3 tabular-nums">窗口内 {totalCount} 条</p>
      </div>

      {truncated ? (
        <p className="shrink-0 border-b border-line bg-warn-soft px-3 py-1.5 text-xs text-warn-text">
          本周条目较多，仅展示最新 500 条；其余请用列表视图筛选查看。
        </p>
      ) : null}

      <div className="flex min-h-0 flex-1 flex-col">
        <StateSurface empty={false} error={error} isError={isError} isLoading={isLoading} onRetry={onRetry}>
          {showEmptyWeek ? (
            <EmptyState
              action={
                <Button asChild size="sm" variant="outline">
                  <Link to="/">去任务中枢</Link>
                </Button>
              }
              className="min-h-[10rem] flex-1 justify-center py-10"
              description="在任务中枢发起对话或任务后，会按日出现在工作节奏里。"
              title="本周暂无运行记录"
            />
          ) : (
            <div className="min-h-0 flex-1 overflow-x-auto overflow-y-hidden">
              <div className="grid h-full min-h-[16rem] min-w-[52rem] grid-cols-7 divide-x divide-line">
                {days.map((day) => {
                  const dayKey = localDayKey(day);
                  const dayItems = byDay.get(dayKey) ?? [];
                  const expanded = expandedDays.has(dayKey);
                  const preview = expanded ? dayItems : dayItems.slice(0, PREVIEW_PER_DAY);
                  const overflow = dayItems.length - preview.length;
                  const today = isToday(day);
                  return (
                    <section
                      className={
                        today
                          ? "flex min-h-0 min-w-0 flex-col bg-brand-soft/40 px-2 py-2"
                          : "flex min-h-0 min-w-0 flex-col px-2 py-2"
                      }
                      key={dayKey}
                    >
                      <header className="mb-1.5 flex shrink-0 items-baseline justify-between gap-2 px-1">
                        <span className="text-xl font-bold tabular-nums text-ink-3">{format(day, "d")}</span>
                        <span className="text-[11px] text-ink-3">{weekdayLabel(day)}</span>
                      </header>
                      <ul className="flex min-h-0 flex-1 flex-col gap-1 overflow-y-auto">
                        {preview.length === 0 ? (
                          <li className="px-1 text-[11px] text-ink-3">无记录</li>
                        ) : (
                          preview.map((item) => (
                            <li key={item.id}>
                              <button
                                className="flex w-full flex-col gap-0.5 rounded-md px-1 py-1 text-left hover:bg-card-soft"
                                onClick={() => onItemClick(item)}
                                type="button"
                              >
                                <span className="flex items-center gap-1.5">
                                  <time
                                    className="shrink-0 text-[11px] tabular-nums text-ink-3"
                                    dateTime={item.created_at}
                                  >
                                    {formatRunTime(item.created_at)}
                                  </time>
                                  <StatusPill
                                    className="px-1.5 py-0 text-[10px] leading-4"
                                    showDot={false}
                                    tone={runStatusTone[item.status]}
                                  >
                                    {runStatusLabel(item.status)}
                                  </StatusPill>
                                </span>
                                <span className="line-clamp-2 break-words text-[12px] leading-snug text-ink">
                                  {item.task_title}
                                </span>
                              </button>
                            </li>
                          ))
                        )}
                        {overflow > 0 ? (
                          <li className="px-1 pt-0.5">
                            <button
                              className="text-[11px] font-medium text-brand hover:underline"
                              onClick={() =>
                                setExpandedDays((current) => {
                                  const next = new Set(current);
                                  next.add(dayKey);
                                  return next;
                                })
                              }
                              type="button"
                            >
                              还有 {overflow} 项
                            </button>
                          </li>
                        ) : null}
                        {expanded && dayItems.length > PREVIEW_PER_DAY ? (
                          <li className="px-1 pt-0.5">
                            <button
                              className="text-[11px] text-ink-3 hover:underline"
                              onClick={() =>
                                setExpandedDays((current) => {
                                  const next = new Set(current);
                                  next.delete(dayKey);
                                  return next;
                                })
                              }
                              type="button"
                            >
                              收起
                            </button>
                          </li>
                        ) : null}
                      </ul>
                    </section>
                  );
                })}
              </div>
            </div>
          )}
        </StateSurface>
      </div>
    </WorkSurface>
  );
}

function groupItemsByLocalDay(items: DigitalEmployeeRunCalendarItem[]): Map<string, DigitalEmployeeRunCalendarItem[]> {
  const map = new Map<string, DigitalEmployeeRunCalendarItem[]>();
  for (const item of items) {
    const day = new Date(item.created_at);
    if (Number.isNaN(day.getTime())) continue;
    const key = localDayKey(day);
    const bucket = map.get(key);
    if (bucket) {
      bucket.push(item);
    } else {
      map.set(key, [item]);
    }
  }
  for (const bucket of map.values()) {
    bucket.sort((a, b) => (a.created_at < b.created_at ? 1 : -1));
  }
  return map;
}

function localDayKey(day: Date): string {
  return format(day, "yyyy-MM-dd");
}

function weekdayLabel(day: Date): string {
  const labels = ["日", "一", "二", "三", "四", "五", "六"];
  return `周${labels[day.getDay()] ?? ""}`;
}

function formatRunTime(createdAt: string): string {
  const day = new Date(createdAt);
  if (Number.isNaN(day.getTime())) return "—";
  return format(day, "HH:mm");
}

/** Exported for tests: whether two anchors land on the same local calendar day. */
export function isSameLocalDay(a: Date, b: Date): boolean {
  return isSameDay(a, b);
}

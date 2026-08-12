import { type ReactNode, useEffect, useState } from "react";
import { X } from "lucide-react";
import {
  Button,
  Chip,
  IconButton,
  ListToolbar,
  MasterDetailLayout,
  Segmented,
  ToolbarSearch,
  WorkSurface,
  type Density,
} from "@/components/superteam";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { logResourceTypeLabel } from "@/lib/status-labels";
import { cn } from "@/lib/utils";

export const LOG_PAGE_SIZE = 20;

export const LOG_SINCE_OPTIONS = [
  { label: "24 小时", value: "24h" },
  { label: "7 天", value: "7d" },
  { label: "30 天", value: "30d" },
  { label: "全部时间", value: "all" },
] as const;

export type LogSinceWindow = (typeof LOG_SINCE_OPTIONS)[number]["value"];

export const DEFAULT_LOG_SINCE: LogSinceWindow = "24h";

export function sinceQueryValue(window: LogSinceWindow): string | undefined {
  if (window === "all") {
    return undefined;
  }
  const hours = window === "24h" ? 24 : window === "7d" ? 24 * 7 : 24 * 30;
  return new Date(Date.now() - hours * 3600 * 1000).toISOString();
}

export function formatLogDateTime(value?: string): string {
  if (!value) {
    return "-";
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }
  return new Intl.DateTimeFormat("zh-CN", {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "2-digit",
    second: "2-digit",
    year: "numeric",
  }).format(date);
}

const UUID_RE =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function shortenLogId(id: string | undefined): string {
  if (!id) {
    return "-";
  }
  if (UUID_RE.test(id)) {
    return `${id.slice(0, 8)}…`;
  }
  return id.length > 24 ? `${id.slice(0, 16)}…` : id;
}

export function resourceCaption(
  resourceType: string | undefined,
  resourceId: string | undefined,
  resourceName?: string | undefined,
): string {
  const named = resourceName?.trim();
  if (named) {
    return named;
  }
  if (resourceType === "system_config" && resourceId) {
    return resourceId;
  }
  const typeLabel = logResourceTypeLabel(resourceType);
  if (typeLabel && resourceId) {
    return `${typeLabel} ${shortenLogId(resourceId)}`;
  }
  return typeLabel || (resourceId ? shortenLogId(resourceId) : "");
}

export function sinceWindowLabel(window: LogSinceWindow): string {
  switch (window) {
    case "24h":
      return "近 24 小时";
    case "7d":
      return "近 7 天";
    case "30d":
      return "近 30 天";
    case "all":
      return "全部时间";
  }
}

/**
 * 空态文案：区分「时间窗内本来就没有」与「筛选把结果滤空」。
 * 有筛选且非全部时间时，提示先扩大时间窗，避免用户误以为类别不存在。
 */
export function logEmptyCopy(opts: {
  fallbackDescription: string;
  hasExtraFilter: boolean;
  noun: string;
  sinceWindow: LogSinceWindow;
}): { description: string; title: string } {
  const windowLabel = sinceWindowLabel(opts.sinceWindow);
  if (opts.hasExtraFilter) {
    if (opts.sinceWindow === "all") {
      return {
        title: `筛选后无${opts.noun}`,
        description: "当前筛选条件下没有匹配记录。可清除模块、状态或类型筛选后再看。",
      };
    }
    return {
      title: `${windowLabel}内无匹配的${opts.noun}`,
      description: `当前筛选在${windowLabel}内没有结果。可先切到「7 天」或「全部时间」；若仍为空，再清除筛选。`,
    };
  }
  if (opts.sinceWindow === "all") {
    return {
      title: `暂无${opts.noun}`,
      description: opts.fallbackDescription,
    };
  }
  return {
    title: `${windowLabel}暂无${opts.noun}`,
    description: `仅展示${windowLabel}内的记录。若要找更早的，可切换时间范围；新产生的记录会出现在这里。`,
  };
}

export function logRowClassName(opts: {
  failed?: boolean;
  selected?: boolean;
  warn?: boolean;
}): string {
  return cn(
    "cursor-pointer",
    opts.selected && "[&>td]:bg-brand-soft/55",
    opts.failed && "[&>td:first-child]:shadow-[inset_3px_0_0_var(--danger)]",
    !opts.failed && opts.warn && "[&>td:first-child]:shadow-[inset_3px_0_0_var(--warn)]",
  );
}

export function logTableDensityClass(density: Density): string | undefined {
  return density === "compact" ? "[&_td]:py-2 [&_th]:py-2" : undefined;
}

export function LogFilterChips({
  onValueChange,
  options,
  value,
}: {
  onValueChange: (value: string | undefined) => void;
  options: Array<{ label: string; value: string }>;
  value: string | undefined;
}) {
  return (
    <>
      <Chip active={!value} onClick={() => onValueChange(undefined)} type="button">
        全部
      </Chip>
      {options.map((opt) => (
        <Chip
          active={value === opt.value}
          key={opt.value}
          onClick={() => onValueChange(opt.value)}
          type="button"
        >
          {opt.label}
        </Chip>
      ))}
    </>
  );
}

/** 工具栏二级筛选：选项多时用下拉，避免与主维度 chips 抢注意力。 */
export function LogToolbarSelect({
  ariaLabel,
  groups,
  onValueChange,
  options,
  placeholder = "全部",
  value,
  widthClassName = "w-[10.5rem]",
}: {
  ariaLabel: string;
  groups?: Array<{ label: string; options: Array<{ label: string; value: string }> }>;
  onValueChange: (value: string | undefined) => void;
  options?: Array<{ label: string; value: string }>;
  placeholder?: string;
  value: string | undefined;
  widthClassName?: string;
}) {
  const flat = options ?? groups?.flatMap((g) => g.options) ?? [];
  return (
    <Select
      onValueChange={(next) => onValueChange(next === "all" ? undefined : next)}
      value={value ?? "all"}
    >
      <SelectTrigger
        aria-label={ariaLabel}
        className={`h-9 ${widthClassName} border-line bg-card-soft shadow-none`}
      >
        <SelectValue placeholder={placeholder} />
      </SelectTrigger>
      <SelectContent>
        <SelectItem value="all">{placeholder}</SelectItem>
        {groups
          ? groups.map((group) => (
              <SelectGroup key={group.label}>
                <SelectLabel>{group.label}</SelectLabel>
                {group.options.map((opt) => (
                  <SelectItem key={opt.value} value={opt.value}>
                    {opt.label}
                  </SelectItem>
                ))}
              </SelectGroup>
            ))
          : flat.map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </SelectItem>
            ))}
      </SelectContent>
    </Select>
  );
}

export function LogSinceSegmented({
  onValueChange,
  value,
}: {
  onValueChange: (value: LogSinceWindow) => void;
  value: LogSinceWindow;
}) {
  return (
    <Segmented
      aria-label="时间范围"
      onChange={onValueChange}
      options={[...LOG_SINCE_OPTIONS]}
      value={value}
    />
  );
}

export function LogDensityToggle({
  onChange,
  value,
}: {
  onChange: (value: Density) => void;
  value: Density;
}) {
  return (
    <Segmented
      aria-label="表格密度"
      onChange={onChange}
      options={[
        { label: "舒适", value: "comfortable" },
        { label: "紧凑", value: "compact" },
      ]}
      value={value}
    />
  );
}

export function LogToolbarSearch({
  id,
  onCommit,
  placeholder,
  value,
}: {
  id: string;
  onCommit: (value: string | undefined) => void;
  placeholder?: string;
  value?: string;
}) {
  const [draft, setDraft] = useState(value ?? "");

  useEffect(() => {
    setDraft(value ?? "");
  }, [value]);

  const commit = () => {
    const trimmed = draft.trim();
    onCommit(trimmed === "" ? undefined : trimmed);
  };

  return (
    <ToolbarSearch
      id={id}
      onBlur={commit}
      onChange={(event) => setDraft(event.target.value)}
      onKeyDown={(event) => {
        if (event.key === "Enter") {
          commit();
        }
      }}
      placeholder={placeholder ?? "输入后回车筛选"}
      value={draft}
    />
  );
}

export function LogPagination({
  isFetching,
  itemCount,
  offset,
  onOffsetChange,
  pageSize,
}: {
  isFetching: boolean;
  itemCount: number;
  offset: number;
  onOffsetChange: (offset: number) => void;
  pageSize: number;
}) {
  const currentPage = Math.floor(offset / pageSize) + 1;
  const canPrev = offset > 0;
  const canNext = itemCount >= pageSize;

  return (
    <div className="flex items-center justify-between gap-3 border-t border-line px-5 py-3">
      <p className="text-xs text-ink-2 tabular-nums">
        第 {currentPage} 页 · 本页 {itemCount} 条{isFetching ? " · 加载中…" : ""}
      </p>
      <div className="flex items-center gap-2">
        <Button
          disabled={!canPrev || isFetching}
          onClick={() => onOffsetChange(Math.max(0, offset - pageSize))}
          size="sm"
          variant="outline"
        >
          上一页
        </Button>
        <Button
          disabled={!canNext || isFetching}
          onClick={() => onOffsetChange(offset + pageSize)}
          size="sm"
          variant="outline"
        >
          下一页
        </Button>
      </div>
    </div>
  );
}

export function LogWorkbench({
  body,
  detail,
  detailLabel,
  onDetailDismiss,
  pagination,
  toolbar,
}: {
  body: ReactNode;
  detail?: ReactNode;
  detailLabel: string;
  onDetailDismiss: () => void;
  pagination: ReactNode;
  toolbar: ReactNode;
}) {
  return (
    <MasterDetailLayout
      data-layout="logs-workbench"
      detail={detail}
      detailLabel={detailLabel}
      onDetailDismiss={onDetailDismiss}
      rail="lg"
      master={
        <WorkSurface>
          {toolbar}
          {body}
          {pagination}
        </WorkSurface>
      }
    />
  );
}

export function LogListToolbar({
  actions,
  filters,
  search,
  since,
}: {
  actions?: ReactNode;
  filters?: ReactNode;
  search?: ReactNode;
  since: ReactNode;
}) {
  return (
    <ListToolbar
      actions={actions}
      filters={filters}
      search={search}
      segments={since}
    />
  );
}

export function LogDetailPanel({
  children,
  kicker,
  onClose,
  status,
  title,
}: {
  children: ReactNode;
  kicker?: ReactNode;
  onClose: () => void;
  status?: ReactNode;
  title: ReactNode;
}) {
  return (
    <div
      className="overflow-hidden rounded-card border border-line bg-card shadow-card"
      data-testid="log-detail"
    >
      <div className="flex items-start justify-between gap-3 border-b border-line px-4 py-3">
        <div className="min-w-0">
          {kicker ? <div className="mb-1.5 flex flex-wrap items-center gap-2">{kicker}</div> : null}
          <h3 className="text-sm font-semibold leading-snug text-ink">{title}</h3>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {status}
          <IconButton aria-label="关闭详情" onClick={onClose} type="button">
            <X className="size-4" />
          </IconButton>
        </div>
      </div>
      {children}
    </div>
  );
}

export function LogDetailSection({ children, title }: { children: ReactNode; title: string }) {
  return (
    <div className="border-b border-line px-5 py-4 last:border-b-0">
      <div className="mb-3 text-[11px] font-semibold tracking-wider text-ink-3">{title}</div>
      {children}
    </div>
  );
}

export function LogInfoRow({ children, label }: { children: ReactNode; label: string }) {
  return (
    <div className="flex items-start gap-3">
      <span className="w-16 shrink-0 text-xs text-ink-2">{label}</span>
      <span className="min-w-0 break-all text-xs text-ink">{children}</span>
    </div>
  );
}

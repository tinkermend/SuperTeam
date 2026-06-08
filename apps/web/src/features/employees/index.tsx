import { useEffect, useMemo, useState, type ChangeEvent } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  AlertTriangle,
  Bot,
  Check,
  ClipboardCheck,
  Link as LinkIcon,
  Plus,
  Search as SearchIcon,
  XCircle,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import {
  LiquidCard,
  MetricCard,
  SemanticIconTile,
  StatusBadge,
  type Tone,
} from "@/components/superteam";
import {
  getDigitalEmployeeOverview,
  type DigitalEmployeeOverview,
  type DigitalEmployeeOverviewFilters,
  type DigitalEmployeeOverviewItem,
  type DigitalEmployeeWorkbenchStatus,
  type OverviewFilterOption,
} from "@/lib/api/employees";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { cn } from "@/lib/utils";
import { EmployeeAvatar } from "./avatar";
import { overviewAvatarAsset } from "./avatar-library";

const DEFAULT_STATUS_OPTIONS: OverviewFilterOption[] = [
  { value: "active", label: "生效" },
  { value: "ready", label: "就绪" },
  { value: "draft", label: "草稿" },
  { value: "disabled", label: "已禁用" },
  { value: "error", label: "异常" },
];

type FilterKey = Exclude<keyof DigitalEmployeeOverviewFilters, "limit" | "offset">;

export function EmployeesPage() {
  const apiBaseUrl = resolveControlPlaneUrl();

  return <EmployeesView apiBaseUrl={apiBaseUrl} />;
}

type EmployeesViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

export function EmployeesView({ apiBaseUrl, fetcher }: EmployeesViewProps) {
  const [filters, setFilters] = useState<DigitalEmployeeOverviewFilters>({
    limit: 50,
    offset: 0,
  });
  const [selectedEmployeeId, setSelectedEmployeeId] = useState<string>();

  const overview = useQuery({
    queryKey: ["digital-employee-overview", filters],
    queryFn: () => getDigitalEmployeeOverview({ baseUrl: apiBaseUrl, fetcher }, filters),
    placeholderData: keepPreviousData,
  });

  const filterOptions = overview.data?.filters;
  const items = overview.data?.items ?? [];
  const selectedItem = useMemo(() => {
    if (items.length === 0) {
      return undefined;
    }

    return items.find((item) => item.identity_summary.id === selectedEmployeeId) ?? items[0];
  }, [items, selectedEmployeeId]);

  useEffect(() => {
    if (items.length === 0) {
      setSelectedEmployeeId(undefined);
      return;
    }

    if (!selectedEmployeeId || !items.some((item) => item.identity_summary.id === selectedEmployeeId)) {
      setSelectedEmployeeId(items[0].identity_summary.id);
    }
  }, [items, selectedEmployeeId]);

  const handleFilterChange = (key: FilterKey) => (value: string) => {
    setFilters((current) => updateFilter(current, key, value));
  };

  const handleSearchChange = (event: ChangeEvent<HTMLInputElement>) => {
    setFilters((current) => updateFilter(current, "q", event.target.value));
  };

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        <div className="flex flex-col gap-5">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div className="flex min-w-0 items-center gap-3">
              <SemanticIconTile tone="primary" size="lg">
                <Bot />
              </SemanticIconTile>
              <div className="min-w-0">
                <h1 className="text-2xl font-bold tracking-normal">数字员工</h1>
                <p className="text-sm text-muted-foreground">业务身份、执行实例和运行状态</p>
              </div>
            </div>
            <Button asChild>
              <Link to="/employees/new">
                <Plus data-icon="inline-start" />
                创建数字员工
              </Link>
            </Button>
          </div>

          {overview.data ? <WorkbenchMetrics overview={overview.data} /> : null}

          {overview.data ? (
            <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
              <div className="min-w-0 xl:col-start-1 xl:row-start-1">
                <EmployeeFilterPanel
                  filters={filters}
                  filterOptions={filterOptions}
                  onFilterChange={handleFilterChange}
                  onSearchChange={handleSearchChange}
                />
              </div>
              <div className="min-w-0 xl:col-start-2 xl:row-span-2 xl:row-start-1">
                <WorkbenchRail overview={overview.data} selectedItem={selectedItem} />
              </div>
              <div className="min-w-0 xl:col-start-1 xl:row-start-2">
                {items.length === 0 ? (
                  <LiquidCard className="rounded-xl p-6 text-sm text-muted-foreground">暂无数字员工</LiquidCard>
                ) : (
                  <div className="grid gap-4 md:grid-cols-2 2xl:grid-cols-3">
                    {items.map((item) => (
                      <EmployeeWorkbenchCard
                        key={item.identity_summary.id}
                        item={item}
                        selected={selectedItem?.identity_summary.id === item.identity_summary.id}
                        onSelect={() => setSelectedEmployeeId(item.identity_summary.id)}
                      />
                    ))}
                  </div>
                )}
              </div>
            </div>
          ) : null}
          {overview.isLoading ? (
            <LiquidCard className="rounded-xl p-6 text-sm text-muted-foreground">加载中...</LiquidCard>
          ) : null}
          {overview.isError ? (
            <LiquidCard className="flex flex-col gap-3 rounded-xl p-6">
              <p className="text-sm font-medium text-destructive">加载失败</p>
              <Button
                className="w-fit"
                size="sm"
                type="button"
                variant="outline"
                onClick={() => void overview.refetch()}
              >
                重试
              </Button>
            </LiquidCard>
          ) : null}
        </div>
      </Main>
    </>
  );
}

function EmployeeFilterPanel({
  filters,
  filterOptions,
  onFilterChange,
  onSearchChange,
}: {
  filters: DigitalEmployeeOverviewFilters;
  filterOptions?: DigitalEmployeeOverview["filters"];
  onFilterChange: (key: FilterKey) => (value: string) => void;
  onSearchChange: (event: ChangeEvent<HTMLInputElement>) => void;
}) {
  return (
    <LiquidCard className="rounded-xl">
      <div className="flex flex-col gap-4 p-4">
        <div className="grid gap-3 md:grid-cols-2 2xl:grid-cols-[minmax(220px,1.2fr)_repeat(4,minmax(132px,1fr))]">
          <label className="flex flex-col gap-1.5 text-sm font-medium text-foreground">
            搜索
            <span className="relative">
              <SearchIcon
                aria-hidden="true"
                className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
              />
              <Input
                className="pl-9"
                value={filters.q ?? ""}
                onChange={onSearchChange}
                placeholder="名称、角色、任务"
              />
            </span>
          </label>
          <FilterSelect
            label="状态"
            value={filters.status ?? "all"}
            options={filterOptions?.statuses ?? DEFAULT_STATUS_OPTIONS}
            onValueChange={onFilterChange("status")}
          />
          <FilterSelect
            label="团队"
            value={filters.team_id ?? "all"}
            options={filterOptions?.teams ?? []}
            onValueChange={onFilterChange("team_id")}
          />
          <FilterSelect
            label="Provider"
            value={filters.provider_type ?? "all"}
            options={filterOptions?.providers ?? []}
            onValueChange={onFilterChange("provider_type")}
          />
          <FilterSelect
            label="风险"
            value={filters.risk_level ?? "all"}
            options={filterOptions?.risk_levels ?? []}
            onValueChange={onFilterChange("risk_level")}
          />
        </div>
        <div className="grid gap-3 md:grid-cols-3">
          <FilterSelect
            label="员工类型"
            value={filters.employee_type ?? "all"}
            options={filterOptions?.employee_types ?? []}
            onValueChange={onFilterChange("employee_type")}
          />
          <FilterSelect
            label="执行"
            value={filters.execution_status ?? "all"}
            options={filterOptions?.execution_statuses ?? []}
            onValueChange={onFilterChange("execution_status")}
          />
          <FilterSelect
            label="最近任务"
            value={filters.run_status ?? "all"}
            options={filterOptions?.run_statuses ?? []}
            onValueChange={onFilterChange("run_status")}
          />
        </div>
      </div>
    </LiquidCard>
  );
}

function WorkbenchMetrics({ overview }: { overview: DigitalEmployeeOverview }) {
  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
      <MetricCard
        title="就绪"
        value={formatNumber(overview.summary.ready_count)}
        icon={<Check />}
        iconTone="success"
      />
      <MetricCard
        title="待绑定"
        value={formatNumber(overview.summary.pending_runtime_binding_count)}
        icon={<LinkIcon />}
        iconTone="warning"
      />
      <MetricCard
        title="异常"
        value={formatNumber(overview.summary.error_count)}
        icon={<AlertTriangle />}
        iconTone="danger"
      />
      <MetricCard
        title="配置待审批"
        value={formatNumber(overview.summary.pending_config_approval_count)}
        icon={<ClipboardCheck />}
        iconTone="artifact"
      />
      <MetricCard
        title="运行失败"
        value={formatNumber(overview.summary.failed_recent_run_count)}
        icon={<XCircle />}
        iconTone="danger"
      />
    </div>
  );
}

function EmployeeWorkbenchCard({
  item,
  selected,
  onSelect,
}: {
  item: DigitalEmployeeOverviewItem;
  selected: boolean;
  onSelect: () => void;
}) {
  const identity = item.identity_summary;
  const avatarAsset = overviewAvatarAsset(item);

  return (
    <article
      aria-label={`员工 ${identity.name}`}
      aria-selected={selected}
      onClick={onSelect}
      onKeyDown={(event) => {
        if (event.currentTarget === event.target && (event.key === "Enter" || event.key === " ")) {
          event.preventDefault();
          onSelect();
        }
      }}
      tabIndex={0}
      className={cn(
        "group relative flex min-h-[240px] cursor-pointer flex-col overflow-hidden rounded-lg border border-border/70 bg-card/90 p-4 text-left shadow-sm transition-all duration-300 hover:-translate-y-1 hover:border-superteam-menu-accent/50 hover:shadow-[var(--superteam-shadow-mid)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        selected &&
          "border-superteam-menu-accent bg-superteam-primary-soft/40 shadow-[var(--superteam-shadow-glow)]",
      )}
    >
      {selected ? <span className="absolute inset-y-0 left-0 w-1 bg-superteam-menu-accent" /> : null}
      <div className="flex items-start gap-3 pl-1">
        <EmployeeAvatar asset={avatarAsset} name={identity.name} size="md" />
        <div className="min-w-0 flex-1">
          <div className="flex items-start justify-between gap-2">
            <div className="min-w-0">
              <p className="truncate font-semibold text-foreground">{identity.name}</p>
              <p className="flex min-w-0 items-center gap-1 text-xs text-muted-foreground">
                <span className="truncate">{identity.employee_type_label || identity.role}</span>
                <span className="shrink-0">·</span>
                <span className="truncate">{identity.team_name || "未分组"}</span>
              </p>
            </div>
            <div className="flex shrink-0 flex-col items-end gap-2">
              <StatusBadge tone={workbenchTone(item.workbench_status)}>
                {workbenchStatusLabel(item.workbench_status)}
              </StatusBadge>
            </div>
          </div>
        </div>
      </div>
      <div className="mt-4 flex flex-col gap-3 text-sm">
        <div className="border-t pt-3 font-medium text-foreground">{runtimeProviderLine(item)}</div>
        <div>
          <p className="text-xs text-muted-foreground">最近运行</p>
          <p className={cn("mt-1 font-medium", latestRunToneClass(item.latest_run_summary?.status))}>
            {latestRunCompact(item)}
          </p>
        </div>
        <p className="text-xs text-muted-foreground">{governanceLine(item)}</p>
        <BudgetBar summary={item.budget_summary} />
      </div>
      <div className="mt-auto grid grid-cols-2 gap-2 border-t pt-3">
        <Button asChild size="sm" variant="ghost" onClick={(event) => event.stopPropagation()}>
          <Link params={{ employeeId: identity.id }} to="/employees/$employeeId">
            详情
          </Link>
        </Button>
        <Button asChild size="sm" variant="ghost" onClick={(event) => event.stopPropagation()}>
          <Link params={{ employeeId: identity.id }} to="/employees/$employeeId/config">
            配置
          </Link>
        </Button>
      </div>
    </article>
  );
}

function BudgetBar({ summary }: { summary: DigitalEmployeeOverviewItem["budget_summary"] }) {
  if (!summary.daily_token_limit) {
    return <p className="text-xs text-muted-foreground">Token 预算：无预算上限</p>;
  }

  const percent = Math.min(summary.usage_percent_today ?? 0, 100);

  return (
    <div className="flex flex-col gap-1.5">
      <div className="flex items-center justify-between gap-3 text-xs text-muted-foreground">
        <span>Token 预算</span>
        <span>
          {formatNumber(summary.usage_tokens_today)} / {formatNumber(summary.daily_token_limit)}
        </span>
      </div>
      <div className="h-1.5 overflow-hidden rounded-full bg-muted">
        <div
          className={cn(
            "h-full rounded-full",
            summary.limit_exceeded ? "bg-destructive" : "bg-[var(--superteam-menu-accent)]",
          )}
          style={{ width: `${percent}%` }}
        />
      </div>
    </div>
  );
}

function QueueRow({
  action,
  label,
  tone,
  value,
}: {
  action: string;
  label: string;
  tone: Tone;
  value: number;
}) {
  return (
    <div className="flex items-center justify-between gap-3 border-t py-3 first:border-t-0 first:pt-0">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <StatusBadge tone={tone}>{formatNumber(value)}</StatusBadge>
          <p className="truncate text-sm font-medium">{label}</p>
        </div>
        <p className="text-xs text-muted-foreground">{formatNumber(value)} 个数字员工</p>
      </div>
      <Button size="sm" variant="outline" type="button">
        {action}
      </Button>
    </div>
  );
}

function SelectedEmployeePanel({ item }: { item: DigitalEmployeeOverviewItem }) {
  const identity = item.identity_summary;
  const avatarAsset = overviewAvatarAsset(item);

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h2 className="font-semibold">选中员工</h2>
      </div>
      <div className="flex items-start gap-3">
        <EmployeeAvatar asset={avatarAsset} name={identity.name} size="lg" />
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <p className="truncate font-semibold">{identity.name}</p>
            <StatusBadge tone={workbenchTone(item.workbench_status)}>{workbenchStatusLabel(item.workbench_status)}</StatusBadge>
          </div>
          <p className="mt-1 text-xs text-muted-foreground">
            {identity.employee_type_label || identity.role} · {identity.team_name || "未分组"}
          </p>
        </div>
      </div>
      <div className="flex flex-col gap-1 text-sm">
        <p className="text-xs text-muted-foreground">绑定</p>
        <p className="font-medium">{runtimeProviderLine(item)}</p>
      </div>
      <div className="flex flex-col gap-3">
        <p className="text-xs text-muted-foreground">最新事件</p>
        {item.recent_events.length === 0 ? (
          <p className="text-sm text-muted-foreground">暂无最近事件</p>
        ) : (
          <ol className="flex flex-col gap-3">
            {item.recent_events.map((event, index) => (
              <li className="flex items-start gap-3" key={`${event.label}-${event.occurred_at ?? index}`}>
                <span
                  className={cn(
                    "mt-1 size-2 rounded-full",
                    event.status === "failed" ? "bg-destructive" : "bg-[var(--superteam-menu-accent)]",
                  )}
                />
                <div className="min-w-0 flex-1">
                  <p className="text-sm">{event.label}</p>
                  <p className="text-xs text-muted-foreground">
                    {event.occurred_at ? eventTimeLabel(event.occurred_at) : "-"}
                  </p>
                </div>
              </li>
            ))}
          </ol>
        )}
      </div>
      <Button className="w-full" type="button" variant="outline">
        查看审计
      </Button>
    </div>
  );
}

function WorkbenchRail({
  overview,
  selectedItem,
}: {
  overview: DigitalEmployeeOverview;
  selectedItem?: DigitalEmployeeOverviewItem;
}) {
  return (
    <aside className="flex min-w-0 flex-col gap-4">
      <LiquidCard className="rounded-xl p-4">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="font-semibold">待处理队列</h2>
        </div>
        <QueueRow
          label="待绑定 Runtime"
          value={overview.queue_summary.pending_runtime_binding_count}
          action="绑定"
          tone="warning"
        />
        <QueueRow label="配置过期" value={overview.queue_summary.stale_config_count} action="审批" tone="artifact" />
        <QueueRow
          label="最近运行失败"
          value={overview.queue_summary.failed_recent_run_count}
          action="查看"
          tone="danger"
        />
      </LiquidCard>
      <LiquidCard className="rounded-xl p-4">
        {selectedItem ? (
          <SelectedEmployeePanel item={selectedItem} />
        ) : (
          <p className="text-sm text-muted-foreground">暂无选中员工</p>
        )}
      </LiquidCard>
    </aside>
  );
}

type FilterSelectProps = {
  label: string;
  value: string;
  options: OverviewFilterOption[];
  onValueChange: (value: string) => void;
};

function FilterSelect({ label, value, options, onValueChange }: FilterSelectProps) {
  const selectId = `employees-filter-${label}`;

  return (
    <div className="flex flex-col gap-1.5">
      <label className="text-sm font-medium text-foreground" htmlFor={selectId}>
        {label}
      </label>
      <Select value={value} onValueChange={onValueChange}>
        <SelectTrigger
          id={selectId}
          aria-label={label}
          className="h-9 w-full rounded-full bg-background/70 shadow-none"
        >
          <SelectValue placeholder="全部" />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            <SelectItem value="all">全部</SelectItem>
            {options.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </div>
  );
}

function updateFilter(
  filters: DigitalEmployeeOverviewFilters,
  key: FilterKey,
  value: string,
): DigitalEmployeeOverviewFilters {
  const next: DigitalEmployeeOverviewFilters = { ...filters, offset: 0 };
  const normalized = value.trim();

  if (normalized === "" || normalized === "all") {
    delete next[key];
    return next;
  }

  next[key] = normalized as never;
  return next;
}

function formatNumber(value: number | undefined | null) {
  return new Intl.NumberFormat("en-US").format(value ?? 0);
}

function workbenchStatusLabel(status: DigitalEmployeeWorkbenchStatus) {
  return status === "ready" ? "就绪" : status === "pending_binding" ? "待绑定" : "异常";
}

function workbenchTone(status: DigitalEmployeeWorkbenchStatus): Tone {
  return status === "ready" ? "success" : status === "pending_binding" ? "warning" : "danger";
}

function runtimeProviderLine(item: DigitalEmployeeOverviewItem) {
  const execution = item.execution_summary;
  if (item.workbench_status === "pending_binding" || !execution.runtime_node_id) {
    return "等待绑定 Runtime Agent";
  }

  const runtime = execution.node_id || execution.runtime_name || "Runtime Agent";
  const provider = providerLabel(execution.provider_type);
  return `${runtime} · ${provider}`;
}

function providerLabel(value: string) {
  const normalized = value.trim().toLowerCase().replace(/-/g, "_");
  const labels: Record<string, string> = {
    claude_code: "Claude Code",
    claude: "Claude Code",
    opencode: "OpenCode",
    open_code: "OpenCode",
    codex: "Codex",
  };

  return labels[normalized] ?? value;
}

function latestRunCompact(item: DigitalEmployeeOverviewItem) {
  const run = item.latest_run_summary;
  if (!run || run.status === "none") {
    return "-";
  }

  if (run.status === "completed") {
    return `成功 · ${runTimeLabel(run)}`;
  }
  if (run.status === "failed" || run.status === "timed_out") {
    return `失败 · ${runTimeLabel(run)}`;
  }
  return "-";
}

function latestRunToneClass(status?: string) {
  if (status === "completed") {
    return "text-emerald-600";
  }
  if (status === "failed" || status === "timed_out") {
    return "text-destructive";
  }
  return "text-muted-foreground";
}

function governanceLine(item: DigitalEmployeeOverviewItem) {
  const governance = item.governance_summary;
  const revision = governance.employee_revision_number ?? governance.team_revision_number;
  const revisionText = revision ? `配置 v${revision}` : "配置";
  return `${revisionText} ${governanceStatusCompact(governance.status)} · skills ${formatNumber(governance.skills_count)} · MCP ${formatNumber(governance.mcp_servers_count)}`;
}

function governanceStatusCompact(status: string) {
  if (status === "approved") {
    return "已审批";
  }
  if (status === "pending_approval" || status === "draft" || status === "stale") {
    return "待审批";
  }
  return "未配置";
}

function runTimeLabel(run: NonNullable<DigitalEmployeeOverviewItem["latest_run_summary"]>) {
  const timestamp = run.finished_at ?? run.updated_at ?? run.started_at;
  if (!timestamp) {
    return run.duration_sec ? `${formatNumber(run.duration_sec)} 秒` : "时间未记录";
  }

  const date = new Date(timestamp);
  if (Number.isNaN(date.getTime())) {
    return timestamp;
  }

  const elapsedMs = Date.now() - date.getTime();
  if (elapsedMs >= 0) {
    const elapsedMinutes = Math.floor(elapsedMs / 60_000);
    if (elapsedMinutes < 1) {
      return "刚刚";
    }
    if (elapsedMinutes < 60) {
      return `${elapsedMinutes} 分钟前`;
    }

    const elapsedHours = Math.floor(elapsedMinutes / 60);
    if (elapsedHours < 24) {
      return `${elapsedHours} 小时前`;
    }

    const elapsedDays = Math.floor(elapsedHours / 24);
    if (elapsedDays < 7) {
      return `${elapsedDays} 天前`;
    }
  }

  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(date);
}

function eventTimeLabel(value: string) {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat("zh-CN", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(date);
}

import { useState, type ChangeEvent } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  Activity,
  AlertTriangle,
  Bot,
  Check,
  Clock,
  Gauge,
  Plus,
  Search as SearchIcon,
  Server,
  ShieldCheck,
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
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
  type DigitalEmployeeOverviewFilters,
  type DigitalEmployeeOverviewItem,
  type OverviewFilterOption,
} from "@/lib/api/employees";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

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

  const overview = useQuery({
    queryKey: ["digital-employee-overview", filters],
    queryFn: () => getDigitalEmployeeOverview({ baseUrl: apiBaseUrl, fetcher }, filters),
  });

  const filterOptions = overview.data?.filters;
  const items = overview.data?.items ?? [];

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

          {overview.data ? <SummaryMetrics summary={overview.data.summary} /> : null}

          <LiquidCard className="rounded-xl">
            <div className="flex flex-col gap-4 p-4">
              <div className="grid gap-3 lg:grid-cols-[minmax(220px,1.2fr)_repeat(4,minmax(150px,1fr))]">
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
                      onChange={handleSearchChange}
                      placeholder="名称、角色、任务"
                    />
                  </span>
                </label>
                <FilterSelect
                  label="状态"
                  value={filters.status ?? "all"}
                  options={filterOptions?.statuses ?? DEFAULT_STATUS_OPTIONS}
                  onValueChange={handleFilterChange("status")}
                />
                <FilterSelect
                  label="团队"
                  value={filters.team_id ?? "all"}
                  options={filterOptions?.teams ?? []}
                  onValueChange={handleFilterChange("team_id")}
                />
                <FilterSelect
                  label="Provider"
                  value={filters.provider_type ?? "all"}
                  options={filterOptions?.providers ?? []}
                  onValueChange={handleFilterChange("provider_type")}
                />
                <FilterSelect
                  label="风险"
                  value={filters.risk_level ?? "all"}
                  options={filterOptions?.risk_levels ?? []}
                  onValueChange={handleFilterChange("risk_level")}
                />
              </div>
              <div className="grid gap-3 md:grid-cols-3">
                <FilterSelect
                  label="员工类型"
                  value={filters.employee_type ?? "all"}
                  options={filterOptions?.employee_types ?? []}
                  onValueChange={handleFilterChange("employee_type")}
                />
                <FilterSelect
                  label="执行"
                  value={filters.execution_status ?? "all"}
                  options={filterOptions?.execution_statuses ?? []}
                  onValueChange={handleFilterChange("execution_status")}
                />
                <FilterSelect
                  label="最近任务"
                  value={filters.run_status ?? "all"}
                  options={filterOptions?.run_statuses ?? []}
                  onValueChange={handleFilterChange("run_status")}
                />
              </div>
            </div>
          </LiquidCard>

          <LiquidCard className="rounded-xl">
            {overview.isLoading ? (
              <div className="p-6 text-sm text-muted-foreground">加载中...</div>
            ) : null}
            {overview.isError ? (
              <div className="flex flex-col gap-3 p-6">
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
              </div>
            ) : null}
            {overview.data && items.length === 0 ? (
              <div className="p-6 text-sm text-muted-foreground">暂无数字员工</div>
            ) : null}
            {items.length > 0 ? <EmployeeOverviewTable items={items} /> : null}
          </LiquidCard>
        </div>
      </Main>
    </>
  );
}

function SummaryMetrics({ summary }: { summary: NonNullable<Awaited<ReturnType<typeof getDigitalEmployeeOverview>>>["summary"] }) {
  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-6">
      <MetricCard
        title="员工总数"
        value={formatNumber(summary.total_count)}
        icon={<Bot />}
        iconTone="primary"
      />
      <MetricCard
        title="可执行员工"
        value={formatNumber(summary.runnable_count)}
        icon={<Check />}
        iconTone="success"
        statusTone="success"
        meta="可领取"
      />
      <MetricCard
        title="执行中"
        value={formatNumber(summary.running_count)}
        icon={<Activity />}
        iconTone="info"
      />
      <MetricCard
        title="等待 Runtime"
        value={formatNumber(summary.waiting_runtime_count)}
        icon={<Clock />}
        iconTone="warning"
      />
      <MetricCard
        title="异常员工"
        value={formatNumber(summary.error_count)}
        icon={<AlertTriangle />}
        iconTone="danger"
        isError={summary.error_count > 0}
      />
      <MetricCard
        title="高风险"
        value={formatNumber(summary.high_risk_count)}
        icon={<Gauge />}
        iconTone="danger"
      />
    </div>
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

function EmployeeOverviewTable({ items }: { items: DigitalEmployeeOverviewItem[] }) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className="min-w-[220px]">员工</TableHead>
          <TableHead>团队</TableHead>
          <TableHead>类型 / 角色</TableHead>
          <TableHead>执行端点</TableHead>
          <TableHead>当前状态</TableHead>
          <TableHead>风险</TableHead>
          <TableHead>最近任务</TableHead>
          <TableHead>治理</TableHead>
          <TableHead>预算</TableHead>
          <TableHead className="text-right">操作</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((item) => (
          <EmployeeOverviewRow key={item.identity_summary.id} item={item} />
        ))}
      </TableBody>
    </Table>
  );
}

function EmployeeOverviewRow({ item }: { item: DigitalEmployeeOverviewItem }) {
  const identity = item.identity_summary;
  const execution = item.execution_summary;
  const latestRun = item.latest_run_summary;

  return (
    <TableRow>
      <TableCell className="min-w-[240px] whitespace-normal">
        <div className="flex min-w-0 items-start gap-3">
          <SemanticIconTile tone={statusTone(identity.status)} size="sm">
            <Bot />
          </SemanticIconTile>
          <div className="min-w-0">
            <div className="font-medium text-foreground">{identity.name}</div>
            {identity.description ? (
              <div className="mt-1 line-clamp-2 text-xs text-muted-foreground">{identity.description}</div>
            ) : null}
          </div>
        </div>
      </TableCell>
      <TableCell>{identity.team_name || "未分组"}</TableCell>
      <TableCell>
        <div className="flex flex-col gap-1">
          <span className="font-medium">{identity.employee_type_label || identity.employee_type}</span>
          <span className="text-xs text-muted-foreground">{identity.role}</span>
        </div>
      </TableCell>
      <TableCell>
        <div className="flex flex-col gap-1">
          <span className="inline-flex items-center gap-2 font-medium">
            <Server aria-hidden="true" className="size-4 text-muted-foreground" />
            {runtimeDisplay(item)}
          </span>
          <span className="text-xs text-muted-foreground">{execution.runtime_status || execution.provider_status}</span>
        </div>
      </TableCell>
      <TableCell>
        <div className="flex flex-col gap-1.5">
          <StatusBadge tone={statusTone(identity.status)}>{employeeStatusLabel(identity.status)}</StatusBadge>
          <span className="text-xs text-muted-foreground">{executionStatusLabel(execution.status)}</span>
        </div>
      </TableCell>
      <TableCell>
        <StatusBadge tone={riskTone(identity.risk_level)}>{riskLabel(identity.risk_level)}</StatusBadge>
      </TableCell>
      <TableCell className="min-w-[180px] whitespace-normal">
        {latestRun ? (
          <div className="flex flex-col gap-1">
            <span className="font-medium text-foreground">{latestRun.title}</span>
            <span className="flex flex-wrap items-center gap-2">
              <StatusBadge tone={statusTone(latestRun.status)}>{runStatusLabel(latestRun.status)}</StatusBadge>
              <span className="text-xs text-muted-foreground">{runTimeLabel(latestRun)}</span>
            </span>
          </div>
        ) : (
          <span className="text-sm text-muted-foreground">暂无任务</span>
        )}
      </TableCell>
      <TableCell>
        <div className="flex flex-col gap-1">
          <span className="inline-flex items-center gap-2 font-medium">
            <ShieldCheck aria-hidden="true" className="size-4 text-muted-foreground" />
            {governanceDisplay(item)}
          </span>
          <span className="text-xs text-muted-foreground">{governanceStatusLabel(item.governance_summary.status)}</span>
        </div>
      </TableCell>
      <TableCell>
        <div className="flex flex-col gap-1">
          <span className="font-medium">{budgetTokenDisplay(item)}</span>
          <span className="text-xs text-muted-foreground">{formatNumber(item.budget_summary.run_count_30d)} runs</span>
        </div>
      </TableCell>
      <TableCell className="text-right">
        <Button asChild size="sm" variant="outline">
          <Link params={{ employeeId: identity.id }} to="/employees/$employeeId">
            详情
          </Link>
        </Button>
      </TableCell>
    </TableRow>
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

function runTimeLabel(run: NonNullable<DigitalEmployeeOverviewItem["latest_run_summary"]>) {
  const timestamp = run.finished_at ?? run.updated_at ?? run.started_at;
  if (!timestamp) {
    return run.duration_sec ? `${formatNumber(run.duration_sec)} 秒` : "时间未记录";
  }

  const date = new Date(timestamp);
  if (Number.isNaN(date.getTime())) {
    return timestamp;
  }

  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(date);
}

function statusTone(status?: string): Tone {
  switch (status) {
    case "active":
    case "running":
    case "dispatching":
    case "provisioning":
      return "info";
    case "ready":
    case "completed":
    case "approved":
      return "success";
    case "draft":
    case "queued":
    case "cancelling":
    case "missing":
      return "warning";
    case "disabled":
    case "error":
    case "failed":
    case "cancelled":
    case "timed_out":
      return "danger";
    default:
      return "neutral";
  }
}

function riskTone(risk?: string): Tone {
  switch (risk) {
    case "high":
    case "critical":
      return "danger";
    case "medium":
      return "warning";
    case "low":
      return "success";
    default:
      return "neutral";
  }
}

function runtimeDisplay(item: DigitalEmployeeOverviewItem) {
  const execution = item.execution_summary;
  if (execution.status === "missing" || !execution.execution_instance_id) {
    return "未绑定 Runtime";
  }

  const node = execution.node_id || execution.runtime_name || execution.runtime_node_id || "Runtime";
  const provider = execution.provider_type || "Provider";
  return `${node} · ${provider}`;
}

function governanceDisplay(item: DigitalEmployeeOverviewItem) {
  const governance = item.governance_summary;
  return `skills ${formatNumber(governance.skills_count)} · MCP ${formatNumber(governance.mcp_servers_count)}`;
}

function budgetTokenDisplay(item: DigitalEmployeeOverviewItem) {
  return `${formatNumber(item.budget_summary.usage_tokens_30d)} tokens`;
}

function employeeStatusLabel(status: string) {
  const labels: Record<string, string> = {
    active: "生效",
    ready: "就绪",
    draft: "草稿",
    disabled: "已禁用",
    error: "异常",
  };

  return labels[status] ?? status;
}

function executionStatusLabel(status: string) {
  const labels: Record<string, string> = {
    missing: "未绑定执行实例",
    provisioning: "准备执行实例",
    ready: "执行就绪",
    active: "执行中",
    disabled: "执行停用",
    error: "执行异常",
  };

  return labels[status] ?? status;
}

function runStatusLabel(status: string) {
  const labels: Record<string, string> = {
    none: "无任务",
    queued: "排队中",
    dispatching: "分派中",
    running: "运行中",
    cancelling: "取消中",
    completed: "已完成",
    failed: "失败",
    cancelled: "已取消",
    timed_out: "超时",
  };

  return labels[status] ?? status;
}

function riskLabel(risk: string) {
  const labels: Record<string, string> = {
    low: "低风险",
    medium: "中风险",
    high: "高风险",
    critical: "极高风险",
  };

  return labels[risk] ?? risk;
}

function governanceStatusLabel(status: string) {
  const labels: Record<string, string> = {
    approved: "配置已批准",
    draft: "配置草稿",
    missing: "缺少配置",
    stale: "配置待更新",
    pending_approval: "等待批准",
    revoked: "已撤销",
  };

  return labels[status] ?? status;
}

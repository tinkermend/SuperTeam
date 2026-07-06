import { useEffect, useMemo, useState, type ChangeEvent, type ReactNode } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  AlertTriangle,
  Bot,
  Check,
  ChevronLeft,
  ChevronRight,
  ClipboardCheck,
  Cpu,
  LayoutTemplate,
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
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import {
  SoftCard,
  StatusPill,
  V3Button,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  type V3Tone,
} from "@/components/superteam";
import {
  getDigitalEmployeeOverview,
  type DigitalEmployeeOperationalStatus,
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

const DEFAULT_PAGE_SIZE = 12;
const PAGE_SIZE_OPTIONS = [12, 24, 48];

type FilterKey = Exclude<keyof DigitalEmployeeOverviewFilters, "limit" | "offset">;

const operationalStatusLabel: Record<DigitalEmployeeOperationalStatus, string> = {
  working: "工作中",
  idle: "空闲",
  queued: "排队",
  waiting_human: "待人工确认",
  error: "异常",
  unavailable: "不可用",
  needs_configuration: "待配置",
};

const operationalStatusTone: Record<DigitalEmployeeOperationalStatus, V3Tone> = {
  working: "info",
  idle: "ok",
  queued: "warn",
  waiting_human: "warn",
  error: "danger",
  unavailable: "mute",
  needs_configuration: "mute",
};

type OperationalStatusPresentation = {
  label: string;
  tone: V3Tone;
};

function operationalStatusPresentation(status?: string): OperationalStatusPresentation {
  if (isKnownOperationalStatus(status)) {
    return {
      label: operationalStatusLabel[status],
      tone: operationalStatusTone[status],
    };
  }

  return { label: "状态未知", tone: "mute" };
}

function isKnownOperationalStatus(status?: string): status is DigitalEmployeeOperationalStatus {
  return typeof status === "string" && Object.prototype.hasOwnProperty.call(operationalStatusLabel, status);
}

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
    limit: DEFAULT_PAGE_SIZE,
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

  const handlePageChange = (offset: number) => {
    setFilters((current) => ({
      ...current,
      offset: Math.max(0, offset),
    }));
  };

  const handlePageSizeChange = (value: string) => {
    const nextLimit = Number(value);
    setFilters((current) => ({
      ...current,
      limit: Number.isFinite(nextLimit) && nextLimit > 0 ? nextLimit : DEFAULT_PAGE_SIZE,
      offset: 0,
    }));
  };

  return (
    <>
      <ShellPageHeader
        icon={<Bot />}
        iconTone="brand"
        title="数字员工"
        subtitle="业务身份、执行实例和运行状态"
      />
      <Main>
        <div className="flex flex-col gap-5">
          <div className="flex flex-wrap items-center justify-start gap-2 sm:justify-end">
            <Button variant="outline" asChild>
              <Link to="/employees/templates">
                <LayoutTemplate data-icon="inline-start" />
                模板管理
              </Link>
            </Button>
            <Button asChild>
              <Link to="/employees/new">
                <Plus data-icon="inline-start" />
                创建数字员工
              </Link>
            </Button>
          </div>

          {overview.data ? <GalleryTrendStrip overview={overview.data} /> : null}

          {overview.data ? (
            <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_300px]">
              <div className="flex min-w-0 flex-col gap-4">
                <GalleryFilterBar
                  filters={filters}
                  filterOptions={filterOptions}
                  onFilterChange={handleFilterChange}
                  onSearchChange={handleSearchChange}
                />
                {items.length === 0 ? (
                  <SoftCard>
                    <V3EmptyState title="暂无数字员工" />
                  </SoftCard>
                ) : (
                  <div className="flex flex-col gap-4">
                    <div className="grid gap-3.5 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
                      {items.map((item) => (
                        <AvatarGalleryCard
                          key={item.identity_summary.id}
                          item={item}
                          selected={selectedItem?.identity_summary.id === item.identity_summary.id}
                          onSelect={() => setSelectedEmployeeId(item.identity_summary.id)}
                        />
                      ))}
                    </div>
                    <EmployeeCardPagination
                      isFetching={overview.isFetching}
                      pagination={overview.data.pagination}
                      visibleCount={items.length}
                      onOffsetChange={handlePageChange}
                      onPageSizeChange={handlePageSizeChange}
                    />
                  </div>
                )}
              </div>
              <div className="min-w-0">
                <GalleryRail overview={overview.data} selectedItem={selectedItem} />
              </div>
            </div>
          ) : null}
          {overview.isLoading ? (
            <SoftCard>
              <V3LoadingState label="加载数字员工..." />
            </SoftCard>
          ) : null}
          {overview.isError ? (
            <V3ErrorState title="加载失败" onRetry={() => void overview.refetch()} />
          ) : null}
        </div>
      </Main>
    </>
  );
}

/* ============================================================
 * 趋势统计条（GalleryTrendStrip）
 * 方向 C · 沉浸画廊：5 张统计卡 + 顶部品牌渐变 accent 条
 * 不使用 sparkline（API 无历史数据源），改用 operational_status_counts
 * 做轻量分布条。
 * ============================================================ */

function GalleryTrendStrip({ overview }: { overview: DigitalEmployeeOverview }) {
  const statusCounts = overview.summary.operational_status_counts ?? {};
  const totalForDistribution = Object.values(statusCounts).reduce((sum, n) => sum + (n ?? 0), 0);

  return (
    <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
      <TrendStatCard
        icon={<Check />}
        iconTone="ok"
        label="就绪"
        value={formatNumber(overview.summary.ready_count)}
        accentGradient="from-emerald-400 to-emerald-600"
      />
      <TrendStatCard
        icon={<LinkIcon />}
        iconTone="warn"
        label="待绑定"
        value={formatNumber(overview.summary.pending_runtime_binding_count)}
        accentGradient="from-amber-400 to-amber-600"
      />
      <TrendStatCard
        icon={<AlertTriangle />}
        iconTone="danger"
        label="异常"
        value={formatNumber(overview.summary.error_count)}
        accentGradient="from-rose-400 to-rose-600"
      />
      <TrendStatCard
        icon={<ClipboardCheck />}
        iconTone="artifact"
        label="配置待审批"
        value={formatNumber(overview.summary.pending_config_approval_count)}
        accentGradient="from-violet-400 to-violet-600"
      />
      <TrendStatCard
        icon={<XCircle />}
        iconTone="brand"
        label="运行失败"
        value={formatNumber(overview.summary.failed_recent_run_count)}
        accentGradient="from-blue-400 to-blue-600"
        distribution={
          totalForDistribution > 0 ? (
            <StatusDistributionBar counts={statusCounts} total={totalForDistribution} />
          ) : null
        }
      />
    </div>
  );
}

/** 静态映射：避免 Tailwind JIT 无法识别动态拼接的 class。 */
const toneIconText: Record<V3Tone, string> = {
  brand: "text-v3-brand",
  info: "text-v3-info",
  ok: "text-v3-ok",
  warn: "text-v3-warn",
  danger: "text-v3-danger",
  mute: "text-v3-mute",
  artifact: "text-v3-artifact",
};

const toneIconSoftBg: Record<V3Tone, string> = {
  brand: "bg-v3-brand-soft",
  info: "bg-v3-info-soft",
  ok: "bg-v3-ok-soft",
  warn: "bg-v3-warn-soft",
  danger: "bg-v3-danger-soft",
  mute: "bg-v3-mute-soft",
  artifact: "bg-v3-artifact-soft",
};

function TrendStatCard({
  icon,
  iconTone,
  label,
  value,
  accentGradient,
  distribution,
}: {
  icon: ReactNode;
  iconTone: V3Tone;
  label: string;
  value: string;
  accentGradient: string;
  distribution?: ReactNode;
}) {
  return (
    <div className="group relative overflow-hidden rounded-v3-card border border-v3-line bg-v3-card p-4 shadow-v3 transition-all duration-200 hover:-translate-y-0.5 hover:shadow-v3-pop">
      <span
        aria-hidden
        className={cn("absolute inset-x-0 top-0 h-[3px] bg-gradient-to-r", accentGradient)}
      />
      <div className="mb-2.5 flex items-center justify-between">
        <span
          className={cn(
            "grid size-8 place-items-center rounded-[9px] [&_svg]:size-4",
            toneIconText[iconTone],
            toneIconSoftBg[iconTone],
          )}
        >
          {icon}
        </span>
      </div>
      <p className="text-[22px] font-extrabold leading-none tracking-tight tabular-nums text-v3-ink">
        {value}
      </p>
      <p className="mt-1.5 text-[12px] font-semibold text-v3-ink-2">{label}</p>
      {distribution ? <div className="mt-2.5">{distribution}</div> : null}
    </div>
  );
}

function StatusDistributionBar({
  counts,
  total,
}: {
  counts: Partial<Record<DigitalEmployeeOperationalStatus, number>>;
  total: number;
}) {
  const segments: Array<{ status: DigitalEmployeeOperationalStatus; count: number; color: string }> = [
    { status: "idle", count: counts.idle ?? 0, color: "bg-v3-ok" },
    { status: "working", count: counts.working ?? 0, color: "bg-v3-info" },
    { status: "queued", count: counts.queued ?? 0, color: "bg-v3-warn" },
    { status: "waiting_human", count: counts.waiting_human ?? 0, color: "bg-v3-warn" },
    { status: "error", count: counts.error ?? 0, color: "bg-v3-danger" },
    { status: "unavailable", count: counts.unavailable ?? 0, color: "bg-v3-mute" },
    { status: "needs_configuration", count: counts.needs_configuration ?? 0, color: "bg-v3-mute" },
  ];

  const active = segments.filter((s) => s.count > 0);

  if (active.length === 0) {
    return null;
  }

  return (
    <div className="flex h-1.5 items-center gap-0.5 overflow-hidden rounded-full">
      {active.map((seg) => (
        <div
          key={seg.status}
          className={cn("h-full rounded-full", seg.color)}
          style={{ flexGrow: seg.count, flexBasis: 0 }}
          title={`${operationalStatusLabel[seg.status]} ${seg.count}`}
        />
      ))}
      <span className="ml-1.5 shrink-0 text-[10px] font-medium tabular-nums text-v3-ink-3">
        {total}
      </span>
    </div>
  );
}

/* ============================================================
 * 筛选栏（GalleryFilterBar）
 * 方向 C：更紧凑的单行筛选，次级筛选折叠
 * ============================================================ */

function GalleryFilterBar({
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
  const [showMore, setShowMore] = useState(false);

  return (
    <SoftCard className="rounded-v3-card">
      <div className="flex flex-col gap-3 p-3.5">
        <div className="grid gap-2.5 md:grid-cols-2 xl:grid-cols-[minmax(200px,1.4fr)_repeat(3,minmax(120px,1fr))]">
          <label className="flex flex-col gap-1 text-xs font-medium text-foreground">
            搜索
            <span className="relative">
              <SearchIcon
                aria-hidden="true"
                className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
              />
              <Input
                className="h-9 pl-9"
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
            label="Provider"
            value={filters.provider_type ?? "all"}
            options={filterOptions?.providers ?? []}
            onValueChange={onFilterChange("provider_type")}
          />
          <FilterSelect
            label="团队"
            value={filters.team_id ?? "all"}
            options={filterOptions?.teams ?? []}
            onValueChange={onFilterChange("team_id")}
          />
        </div>
        {showMore ? (
          <div className="grid gap-2.5 md:grid-cols-2 xl:grid-cols-4">
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
            <FilterSelect
              label="风险"
              value={filters.risk_level ?? "all"}
              options={filterOptions?.risk_levels ?? []}
              onValueChange={onFilterChange("risk_level")}
            />
          </div>
        ) : null}
        <button
          type="button"
          onClick={() => setShowMore((v) => !v)}
          className="self-start text-xs font-semibold text-v3-brand transition-colors hover:text-v3-brand-deep"
        >
          {showMore ? "收起筛选" : "更多筛选"}
        </button>
      </div>
    </SoftCard>
  );
}

/* ============================================================
 * 头像优先画廊卡（AvatarGalleryCard）
 * 方向 C：居中大头像 + 角色徽章 + 紧凑指标行
 * ============================================================ */

function AvatarGalleryCard({
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
  const operationalStatus = operationalStatusPresentation(item.operational_state?.status);

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
        "group relative flex min-h-[220px] cursor-pointer flex-col overflow-hidden rounded-v3-card border border-v3-line bg-v3-card p-4 text-left shadow-v3 transition-all duration-200 hover:-translate-y-0.5 hover:border-v3-brand/40 hover:shadow-v3-pop focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-v3-brand/60",
        selected && "border-v3-brand bg-v3-brand-soft/40 shadow-v3-pop ring-1 ring-v3-brand/20",
      )}
    >
      {selected ? <span className="absolute inset-y-0 left-0 w-1 bg-v3-brand" /> : null}

      {/* 头像区：居中 */}
      <div className="flex flex-col items-center gap-2 pb-3">
        <div className="relative">
          <EmployeeAvatar asset={avatarAsset} name={identity.name} size="lg" />
        </div>
        <div className="w-full text-center">
          <p className="truncate text-[13.5px] font-bold text-v3-ink">{identity.name}</p>
          <p className="mt-0.5 truncate text-[11px] text-v3-ink-3">
            {identity.employee_type_label || identity.role} · {identity.team_name || "未分组"}
          </p>
        </div>
        <StatusPill tone={operationalStatus.tone}>{operationalStatus.label}</StatusPill>
      </div>

      {/* 指标行 */}
      <div className="flex flex-col gap-1.5 border-t border-v3-line pt-2.5 text-[11.5px]">
        <div className="flex items-center justify-between gap-2">
          <span className="flex items-center gap-1 text-v3-ink-3">
            <Cpu className="size-3" aria-hidden />
            Provider
          </span>
          <span className="truncate text-right font-semibold text-v3-ink">
            {runtimeProviderLine(item)}
          </span>
        </div>
        <div className="flex items-center justify-between gap-2">
          <span className="text-v3-ink-3">工作台：</span>
          <span className="font-semibold text-v3-ink">{workbenchStatusLabel(item.workbench_status)}</span>
        </div>
        <div className="flex items-center justify-between gap-2">
          <span className="text-v3-ink-3">最近运行</span>
          <span className={cn("truncate text-right font-semibold", latestRunToneClass(item.latest_run_summary?.status))}>
            {latestRunCompact(item)}
          </span>
        </div>
        <p className="mt-1 text-[11px] text-v3-ink-3">{governanceLine(item)}</p>
      </div>

      {/* 预算条 */}
      <BudgetBar summary={item.budget_summary} />

      {/* 操作按钮 */}
      <div className="mt-auto grid grid-cols-2 gap-2 border-t border-v3-line pt-2.5">
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
    return <p className="mt-2 text-[11px] text-v3-ink-3">Token 预算：无预算上限</p>;
  }

  const percent = Math.min(summary.usage_percent_today ?? 0, 100);

  return (
    <div className="mt-2 flex flex-col gap-1">
      <div className="flex items-center justify-between gap-2 text-[11px] text-v3-ink-3">
        <span>Token 预算</span>
        <span className="tabular-nums">
          {formatNumber(summary.usage_tokens_today)} / {formatNumber(summary.daily_token_limit)}
        </span>
      </div>
      <div className="h-1 overflow-hidden rounded-full bg-v3-line">
        <div
          className={cn(
            "h-full rounded-full transition-all",
            summary.limit_exceeded ? "bg-v3-danger" : "bg-v3-brand",
          )}
          style={{ width: `${percent}%` }}
        />
      </div>
    </div>
  );
}

/* ============================================================
 * 分页
 * ============================================================ */

function EmployeeCardPagination({
  isFetching,
  onOffsetChange,
  onPageSizeChange,
  pagination,
  visibleCount,
}: {
  isFetching: boolean;
  onOffsetChange: (offset: number) => void;
  onPageSizeChange: (value: string) => void;
  pagination: DigitalEmployeeOverview["pagination"];
  visibleCount: number;
}) {
  const limit = pagination.limit || DEFAULT_PAGE_SIZE;
  const offset = pagination.offset || 0;
  const totalCount = pagination.total_count || 0;
  const start = totalCount === 0 || visibleCount === 0 ? 0 : offset + 1;
  const end = totalCount === 0 || visibleCount === 0 ? 0 : Math.min(offset + visibleCount, totalCount);
  const canPrevious = offset > 0;
  const canNext = offset + limit < totalCount;

  return (
    <div className="flex flex-col gap-3 rounded-v3-inner border border-v3-line bg-v3-card px-4 py-3 shadow-v3 sm:flex-row sm:items-center sm:justify-between">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2 text-sm text-muted-foreground">
        <span className="font-medium text-foreground">
          第 {formatNumber(start)}-{formatNumber(end)} 条，共 {formatNumber(totalCount)} 个
        </span>
        {isFetching ? <span>刷新中...</span> : null}
      </div>
      <div className="flex items-center justify-between gap-3 sm:justify-end">
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted-foreground">每页</span>
          <Select value={String(limit)} onValueChange={onPageSizeChange}>
            <SelectTrigger aria-label="每页数量" className="h-9 w-[84px] rounded-xl border-v3-line-strong bg-v3-card text-v3-ink shadow-none hover:bg-v3-card-soft">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {PAGE_SIZE_OPTIONS.map((pageSize) => (
                  <SelectItem key={pageSize} value={String(pageSize)}>
                    {pageSize}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>
        <div className="flex items-center gap-2">
          <Button
            aria-label="上一页"
            className="size-9 p-0"
            disabled={!canPrevious}
            type="button"
            variant="outline"
            onClick={() => onOffsetChange(Math.max(0, offset - limit))}
          >
            <ChevronLeft className="size-4" />
          </Button>
          <Button
            aria-label="下一页"
            className="size-9 p-0"
            disabled={!canNext}
            type="button"
            variant="outline"
            onClick={() => onOffsetChange(offset + limit)}
          >
            <ChevronRight className="size-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}

/* ============================================================
 * 右侧栏（GalleryRail）
 * 方向 C：待处理队列 + 选中员工（大头像 + 事件流）
 * ============================================================ */

function GalleryRail({
  overview,
  selectedItem,
}: {
  overview: DigitalEmployeeOverview;
  selectedItem?: DigitalEmployeeOverviewItem;
}) {
  return (
    <aside className="flex min-w-0 flex-col gap-4">
      <SoftCard className="p-4">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="font-semibold">待处理队列</h2>
        </div>
        <QueueRow
          action="绑定"
          label="待绑定 Runtime"
          tone="warn"
          to="/runtime"
          value={overview.queue_summary.pending_runtime_binding_count}
        />
        <QueueRow
          action="审批"
          label="配置过期"
          tone="artifact"
          to="/approvals"
          value={overview.queue_summary.stale_config_count}
        />
        <QueueRow
          action="查看"
          label="最近运行失败"
          tone="danger"
          to="/run-overview"
          value={overview.queue_summary.failed_recent_run_count}
        />
      </SoftCard>
      <SoftCard className="p-4">
        {selectedItem ? (
          <GallerySelectedPanel item={selectedItem} />
        ) : (
          <p className="text-sm text-muted-foreground">暂无选中员工</p>
        )}
      </SoftCard>
    </aside>
  );
}

function QueueRow({
  action,
  label,
  tone,
  to,
  value,
}: {
  action: string;
  label: string;
  tone: V3Tone;
  to: string;
  value: number;
}) {
  return (
    <div className="flex items-center justify-between gap-3 border-t py-2.5 first:border-t-0 first:pt-0">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <StatusPill tone={tone}>{formatNumber(value)}</StatusPill>
          <p className="truncate text-sm font-medium">{label}</p>
        </div>
        <p className="text-xs text-muted-foreground">{formatNumber(value)} 个数字员工</p>
      </div>
      {value > 0 ? (
        <Button asChild size="sm" variant="outline">
          <Link to={to}>{action}</Link>
        </Button>
      ) : (
        <Button disabled size="sm" type="button" variant="outline">
          {action}
        </Button>
      )}
    </div>
  );
}

function GallerySelectedPanel({ item }: { item: DigitalEmployeeOverviewItem }) {
  const identity = item.identity_summary;
  const avatarAsset = overviewAvatarAsset(item);
  const operationalStatus = operationalStatusPresentation(item.operational_state?.status);

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h2 className="font-semibold">选中员工</h2>
      </div>
      <div className="flex flex-col items-center gap-2">
        <div className="relative">
          <EmployeeAvatar asset={avatarAsset} name={identity.name} size="lg" />
        </div>
        <div className="text-center">
          <div className="flex items-center justify-center gap-2">
            <p className="truncate font-semibold">{identity.name}</p>
          </div>
          <p className="mt-1 text-xs text-muted-foreground">
            {identity.employee_type_label || identity.role} · {identity.team_name || "未分组"}
          </p>
          <div className="mt-2 flex justify-center">
            <StatusPill tone={operationalStatus.tone}>{operationalStatus.label}</StatusPill>
          </div>
        </div>
      </div>
      <div className="flex flex-col gap-1.5 text-xs">
        <div className="flex items-center justify-between rounded-lg bg-v3-card-soft px-3 py-2">
          <span className="text-v3-ink-3">工作台</span>
          <span className="font-semibold text-v3-ink">{workbenchStatusLabel(item.workbench_status)}</span>
        </div>
        <div className="flex items-center justify-between rounded-lg bg-v3-card-soft px-3 py-2">
          <span className="text-v3-ink-3">绑定</span>
          <span className="text-right font-semibold text-v3-ink">{runtimeProviderLine(item)}</span>
        </div>
      </div>
      <div className="flex flex-col gap-2">
        <p className="text-xs text-muted-foreground">最新事件</p>
        {item.recent_events.length === 0 ? (
          <p className="text-sm text-muted-foreground">暂无最近事件</p>
        ) : (
          <ol className="flex flex-col gap-2.5">
            {item.recent_events.map((event, index) => (
              <li className="flex items-start gap-3" key={`${event.label}-${event.occurred_at ?? index}`}>
                <span
                  className={cn(
                    "mt-1 size-2 rounded-full",
                    event.status === "failed" ? "bg-v3-danger" : "bg-v3-brand",
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
      <V3Button asChild className="w-full" variant="outline">
        <Link params={{ employeeId: identity.id }} to="/employees/$employeeId">
          查看详情
        </Link>
      </V3Button>
    </div>
  );
}

/* ============================================================
 * 筛选 Select
 * ============================================================ */

type FilterSelectProps = {
  label: string;
  value: string;
  options: OverviewFilterOption[];
  onValueChange: (value: string) => void;
};

function FilterSelect({ label, value, options, onValueChange }: FilterSelectProps) {
  const selectId = `employees-filter-${label}`;

  return (
    <div className="flex flex-col gap-1">
      <label className="text-xs font-medium text-foreground" htmlFor={selectId}>
        {label}
      </label>
      <Select value={value} onValueChange={onValueChange}>
        <SelectTrigger
          id={selectId}
          aria-label={label}
          className="h-9 w-full rounded-xl border-v3-line-strong bg-v3-card text-v3-ink shadow-none hover:bg-v3-card-soft"
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

/* ============================================================
 * 工具函数（保持与原实现一致，确保测试通过）
 * ============================================================ */

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

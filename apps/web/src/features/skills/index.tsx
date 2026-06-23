import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  Blocks,
  Bot,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  ClipboardList,
  FileText,
  Grid2X2,
  List,
  MoreHorizontal,
  ServerCog,
  ShieldCheck,
  Stethoscope,
  TriangleAlert,
  UploadCloud,
  UserRoundCheck,
  Users,
  type LucideIcon,
} from "lucide-react";
import {
  IconTile,
  SoftCard,
  StatusPill,
  V3Button,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3MetricCard,
  V3PageHeader,
  V3Segmented,
  V3Table,
  V3Td,
  V3Th,
  V3ToolbarSearch,
  V3Tr,
  WorkSurface,
  type V3Tone,
} from "@/components/superteam";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import { listSkillInstallations, listSkills, type Skill, type SkillInstallation } from "@/lib/api/skills";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { cn } from "@/lib/utils";
import { SkillInstallDialog } from "./install-dialog";

type SkillsViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

type ApiOpts = { baseUrl: string; fetcher?: typeof fetch };
type RiskFilter = "all" | "low" | "medium" | "high";
type ScopeFilter = "all" | "team" | "employee" | "unbound";
type DependencyFilter = "all" | "none" | "required";
type StatusFilter = "all" | SkillMarketStatus;
type ViewMode = "list" | "grid";

type SkillMarketStatus = "installed" | "available" | "approval" | "dependency";

type MetricDefinition = {
  icon: LucideIcon;
  label: string;
  tone: V3Tone;
  value: number;
  loud?: boolean;
};

type StatusDisplay = {
  label: string;
  tone: V3Tone;
  value: SkillMarketStatus;
};

const iconMap: Record<string, LucideIcon> = {
  blocks: Blocks,
  flask: FileText,
  "server-cog": ServerCog,
  "shield-check": ShieldCheck,
  stethoscope: Stethoscope,
};

const toneByColor: Record<string, V3Tone> = {
  blue: "info",
  cyan: "info",
  emerald: "ok",
  teal: "brand",
  violet: "artifact",
};

export function SkillsPage() {
  const apiBaseUrl = resolveControlPlaneUrl();
  return <SkillsView apiBaseUrl={apiBaseUrl} />;
}

export function SkillsView({ apiBaseUrl, fetcher }: SkillsViewProps) {
  const [query, setQuery] = useState("");
  const [riskFilter, setRiskFilter] = useState<RiskFilter>("all");
  const [scopeFilter, setScopeFilter] = useState<ScopeFilter>("all");
  const [dependencyFilter, setDependencyFilter] = useState<DependencyFilter>("all");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const [viewMode, setViewMode] = useState<ViewMode>("list");
  const [selectedSkillId, setSelectedSkillId] = useState<string>();
  const [installSkillId, setInstallSkillId] = useState<string>();
  const [pageSize, setPageSize] = useState(10);
  const [page, setPage] = useState(1);

  const apiOptions: ApiOpts = { baseUrl: apiBaseUrl, fetcher };
  const skills = useQuery({
    queryKey: ["skills", query],
    queryFn: () => listSkills(apiOptions, { q: query }),
  });

  const skillRows = skills.data ?? [];
  const skillsError = skills.error instanceof Error ? skills.error.message : undefined;
  const metrics = useMemo(() => buildMarketMetrics(skillRows), [skillRows]);
  const filteredRows = useMemo(
    () => filterSkills(skillRows, { dependencyFilter, riskFilter, scopeFilter, statusFilter }),
    [dependencyFilter, riskFilter, scopeFilter, skillRows, statusFilter],
  );
  const selectedSkill = filteredRows.find((skill) => skill.id === selectedSkillId) ?? filteredRows[0];
  const installSkillTarget = skillRows.find((skill) => skill.id === installSkillId);
  const skillInstallations = useQuery({
    enabled: Boolean(selectedSkill && skills.data),
    queryKey: ["skill", selectedSkill?.id, "installations"],
    queryFn: () => listSkillInstallations(apiOptions, selectedSkill!.id),
  });
  const pageCount = Math.max(1, Math.ceil(filteredRows.length / pageSize));
  const activePage = Math.min(page, pageCount);
  const pagedRows = filteredRows.slice((activePage - 1) * pageSize, activePage * pageSize);
  const isInitialLoading = skills.isPending && skillRows.length === 0;
  const isBlockingError = skills.isError && skillRows.length === 0;

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main className="min-w-0 overflow-x-hidden">
        <div className="flex min-w-0 flex-col gap-6">
          <V3PageHeader
            icon={<Blocks />}
            iconTone="artifact"
            title="技能市场"
            subtitle="发现、校验并安装技能到团队或数字员工"
            actions={
              <V3Button asChild className="h-11 self-start px-5">
              <Link to="/skills/upload">
                <UploadCloud data-icon="inline-start" />
                上传技能
              </Link>
              </V3Button>
            }
          />

          <section
            className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6"
            aria-label="技能市场指标"
          >
            {metrics.map((metric) => (
              <SkillMarketMetric key={metric.label} metric={metric} />
            ))}
          </section>

          <WorkSurface className="min-w-0">
            <SkillMarketToolbar
              dependencyFilter={dependencyFilter}
              onDependencyFilterChange={(value) => {
                setDependencyFilter(value);
                setPage(1);
              }}
              onQueryChange={(value) => {
                setQuery(value);
                setPage(1);
              }}
              onRiskFilterChange={(value) => {
                setRiskFilter(value);
                setPage(1);
              }}
              onScopeFilterChange={(value) => {
                setScopeFilter(value);
                setPage(1);
              }}
              onStatusFilterChange={(value) => {
                setStatusFilter(value);
                setPage(1);
              }}
              onViewModeChange={setViewMode}
              query={query}
              riskFilter={riskFilter}
              scopeFilter={scopeFilter}
              statusFilter={statusFilter}
              viewMode={viewMode}
            />

            {isInitialLoading ? (
              <V3LoadingState label="加载技能数据…" />
            ) : isBlockingError ? (
              <div className="p-4">
                <V3ErrorState
                  title="技能数据加载失败"
                  description={skillsError ?? "请检查 Control Plane 技能接口和数据库迁移状态。"}
                />
              </div>
            ) : (
              <>
                {skills.isError ? (
                  <div className="border-b border-v3-line p-4">
                    <V3ErrorState
                      title="技能数据加载失败"
                      description={skillsError ?? "请检查 Control Plane 技能接口和数据库迁移状态。"}
                    />
                  </div>
                ) : null}
                {viewMode === "list" ? (
                  <SkillMarketTable
                    onInstallSkill={setInstallSkillId}
                    onSelectSkill={setSelectedSkillId}
                    rows={pagedRows}
                    selectedSkillId={selectedSkill?.id}
                  />
                ) : (
                  <SkillMarketGrid
                    onInstallSkill={setInstallSkillId}
                    onSelectSkill={setSelectedSkillId}
                    rows={pagedRows}
                    selectedSkillId={selectedSkill?.id}
                  />
                )}

                <SkillMarketPagination
                  currentPage={activePage}
                  onPageChange={setPage}
                  onPageSizeChange={(value) => {
                    setPageSize(value);
                    setPage(1);
                  }}
                  pageCount={pageCount}
                  pageSize={pageSize}
                  total={filteredRows.length}
                />
              </>
            )}
          </WorkSurface>

          {selectedSkill ? (
            <SelectedSkillInstallations
              installations={skillInstallations.data ?? []}
              isError={skillInstallations.isError}
              isLoading={skillInstallations.isPending}
              errorMessage={skillInstallations.error instanceof Error ? skillInstallations.error.message : undefined}
              skill={selectedSkill}
            />
          ) : null}
        </div>
      </Main>
      <SkillInstallDialog
        apiBaseUrl={apiBaseUrl}
        fetcher={fetcher}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) {
            setInstallSkillId(undefined);
          }
        }}
        open={Boolean(installSkillTarget)}
        skill={installSkillTarget}
      />
    </>
  );
}

function SkillMarketMetric({ metric }: { metric: MetricDefinition }) {
  const Icon = metric.icon;
  return (
    <V3MetricCard
      icon={<Icon />}
      iconTone={metric.tone}
      label={metric.label}
      loud={metric.loud}
      value={metric.value}
    />
  );
}

function SelectedSkillInstallations({
  errorMessage,
  installations,
  isError,
  isLoading,
  skill,
}: {
  errorMessage?: string;
  installations: SkillInstallation[];
  isError: boolean;
  isLoading: boolean;
  skill: Skill;
}) {
  return (
    <WorkSurface
      aria-label={`${skill.name} 安装记录`}
      className="min-w-0 overflow-hidden"
      role="region"
    >
      <div className="flex min-w-0 flex-col gap-3 border-b border-v3-line p-4 md:flex-row md:items-start md:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <SkillIcon skill={skill} size="sm" />
          <div className="min-w-0">
            <h2 className="truncate text-base font-bold text-v3-ink">安装记录</h2>
            <p className="mt-1 min-w-0 text-[13px] leading-5 text-v3-ink-2">
              <span className="font-semibold text-v3-ink">{skill.name}</span>
              <span className="px-2 text-v3-ink-3">·</span>
              <span className="font-mono">{skill.version}</span>
            </p>
          </div>
        </div>
        <StatusPill tone={installations.length > 0 ? "ok" : "info"}>
          {installations.length} 个目标
        </StatusPill>
      </div>

      {isLoading ? (
        <V3LoadingState className="py-8" label="加载安装记录…" />
      ) : isError ? (
        <div className="p-4">
          <V3ErrorState
            title="安装记录加载失败"
            description={errorMessage ?? "请检查技能安装记录接口。"}
          />
        </div>
      ) : installations.length === 0 ? (
        <V3EmptyState
          className="py-10"
          title="暂无安装记录"
          description="成功安装到运行节点后会显示物理安装事实。"
        />
      ) : (
        <div className="divide-y divide-v3-line">
          {installations.map((installation, index) => (
            <SkillInstallationRow
              installation={installation}
              key={installation.id ?? `${installation.digital_employee_id ?? installation.node_id ?? "target"}-${index}`}
            />
          ))}
        </div>
      )}
    </WorkSurface>
  );
}

function SkillInstallationRow({ installation }: { installation: SkillInstallation }) {
  return (
    <div className="grid min-w-0 gap-3 px-4 py-3 text-[13px] md:grid-cols-[minmax(160px,1.1fr)_minmax(150px,0.9fr)_minmax(220px,1.4fr)] md:items-center">
      <div className="min-w-0">
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate font-bold text-v3-ink">{installationTargetLabel(installation)}</span>
          <StatusPill tone={providerTone(installation.provider_type)}>{installation.provider_type}</StatusPill>
        </div>
        {installation.digital_employee_id && installation.employee_name ? (
          <div className="mt-1 truncate font-mono text-[11px] text-v3-ink-3">
            {installation.digital_employee_id}
          </div>
        ) : null}
      </div>

      <div className="min-w-0 space-y-1">
        <div className="truncate font-mono text-xs text-v3-ink-2">{runtimeNodeLabel(installation)}</div>
        {installation.installed_at ? (
          <div className="truncate font-mono text-[11px] text-v3-ink-3">{installation.installed_at}</div>
        ) : null}
      </div>

      <div className="min-w-0 space-y-1">
        <div className="truncate rounded-lg bg-v3-mute-soft px-2 py-1 font-mono text-xs text-v3-ink">
          {installation.installed_path}
        </div>
        {installation.archive_checksum_sha256 ? (
          <div className="truncate font-mono text-[11px] text-v3-ink-3">
            {installation.archive_checksum_sha256}
          </div>
        ) : null}
      </div>
    </div>
  );
}

function SkillMarketToolbar({
  dependencyFilter,
  onDependencyFilterChange,
  onQueryChange,
  onRiskFilterChange,
  onScopeFilterChange,
  onStatusFilterChange,
  onViewModeChange,
  query,
  riskFilter,
  scopeFilter,
  statusFilter,
  viewMode,
}: {
  dependencyFilter: DependencyFilter;
  onDependencyFilterChange: (value: DependencyFilter) => void;
  onQueryChange: (value: string) => void;
  onRiskFilterChange: (value: RiskFilter) => void;
  onScopeFilterChange: (value: ScopeFilter) => void;
  onStatusFilterChange: (value: StatusFilter) => void;
  onViewModeChange: (value: ViewMode) => void;
  query: string;
  riskFilter: RiskFilter;
  scopeFilter: ScopeFilter;
  statusFilter: StatusFilter;
  viewMode: ViewMode;
}) {
  return (
    <div className="flex min-w-0 flex-col gap-3 border-b border-v3-line p-4 lg:flex-row lg:items-center lg:justify-between">
      <div className="flex min-w-0 flex-1 flex-col gap-3 lg:flex-row lg:items-center">
        <V3ToolbarSearch
          aria-label="搜索技能"
          onChange={(event) => onQueryChange(event.target.value)}
          placeholder="搜索技能、标签、运行依赖"
          type="search"
          value={query}
        />
        <div className="grid min-w-0 grid-cols-2 gap-3 sm:grid-cols-4 lg:flex lg:items-center">
          <FilterSelect
            label="风险等级"
            onValueChange={(value) => onRiskFilterChange(value as RiskFilter)}
            options={[
              { label: "全部风险", value: "all" },
              { label: "低风险", value: "low" },
              { label: "中风险", value: "medium" },
              { label: "高风险", value: "high" },
            ]}
            value={riskFilter}
          />
          <FilterSelect
            label="绑定范围"
            onValueChange={(value) => onScopeFilterChange(value as ScopeFilter)}
            options={[
              { label: "全部范围", value: "all" },
              { label: "团队绑定", value: "team" },
              { label: "员工绑定", value: "employee" },
              { label: "未绑定", value: "unbound" },
            ]}
            value={scopeFilter}
          />
          <FilterSelect
            label="运行依赖"
            onValueChange={(value) => onDependencyFilterChange(value as DependencyFilter)}
            options={[
              { label: "全部依赖", value: "all" },
              { label: "无依赖", value: "none" },
              { label: "有依赖", value: "required" },
            ]}
            value={dependencyFilter}
          />
          <FilterSelect
            label="状态"
            onValueChange={(value) => onStatusFilterChange(value as StatusFilter)}
            options={[
              { label: "全部状态", value: "all" },
              { label: "已安装", value: "installed" },
              { label: "可安装", value: "available" },
              { label: "需审批", value: "approval" },
              { label: "需补全依赖", value: "dependency" },
            ]}
            value={statusFilter}
          />
        </div>
      </div>
      <V3Segmented
        aria-label="技能视图"
        className="self-start"
        onChange={onViewModeChange}
        options={[
          {
            label: (
              <>
                <List aria-hidden className="size-4" />
                <span className="sr-only">列表视图</span>
              </>
            ),
            value: "list",
          },
          {
            label: (
              <>
                <Grid2X2 aria-hidden className="size-4" />
                <span className="sr-only">网格视图</span>
              </>
            ),
            value: "grid",
          },
        ]}
        value={viewMode}
      />
    </div>
  );
}

function FilterSelect({
  label,
  onValueChange,
  options,
  value,
}: {
  label: string;
  onValueChange: (value: string) => void;
  options: Array<{ label: string; value: string }>;
  value: string;
}) {
  return (
    <Select onValueChange={onValueChange} value={value}>
      <SelectTrigger
        aria-label={label}
        className="w-full min-w-0 rounded-xl border-v3-line-strong bg-v3-card text-v3-ink shadow-none hover:bg-v3-card-soft lg:w-[124px]"
      >
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option.value} value={option.value}>
            {option.label}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function SkillMarketTable({
  onInstallSkill,
  onSelectSkill,
  rows,
  selectedSkillId,
}: {
  onInstallSkill: (id: string) => void;
  onSelectSkill: (id: string) => void;
  rows: Skill[];
  selectedSkillId?: string;
}) {
  return (
    <V3Table>
      <thead>
        <tr>
          <V3Th className="min-w-[300px]" role="columnheader">技能</V3Th>
          <V3Th className="min-w-24" role="columnheader">风险</V3Th>
          <V3Th className="min-w-[180px]" role="columnheader">标签</V3Th>
          <V3Th className="min-w-24" role="columnheader">版本</V3Th>
          <V3Th className="min-w-32" role="columnheader">绑定目标</V3Th>
          <V3Th className="min-w-28" role="columnheader">状态</V3Th>
          <V3Th className="min-w-[220px] text-right" role="columnheader">操作</V3Th>
        </tr>
      </thead>
      <tbody>
        {rows.map((skill) => (
          <SkillMarketTableRow
            key={skill.id}
            onInstallSkill={onInstallSkill}
            onSelectSkill={onSelectSkill}
            selected={selectedSkillId === skill.id}
            skill={skill}
          />
        ))}
        {rows.length === 0 ? (
          <tr>
            <V3Td className="h-32 text-center text-v3-ink-3" colSpan={7}>
              暂无匹配技能，请调整搜索或筛选条件
            </V3Td>
          </tr>
        ) : null}
      </tbody>
    </V3Table>
  );
}

function SkillMarketTableRow({
  onInstallSkill,
  onSelectSkill,
  selected,
  skill,
}: {
  onInstallSkill: (id: string) => void;
  onSelectSkill: (id: string) => void;
  selected: boolean;
  skill: Skill;
}) {
  const risk = riskDisplay(skill.risk_level);
  const status = statusDisplay(skill);
  const rowTone =
    status.value === "approval" ? "danger" : status.value === "dependency" ? "warn" : undefined;

  return (
    <V3Tr
      className={cn(selected && "[&>td]:bg-v3-brand-soft/60")}
      data-state={selected ? "selected" : undefined}
      tone={rowTone}
    >
      <V3Td className="whitespace-normal">
        <button
          className="flex min-w-0 items-start gap-3 text-left"
          onClick={() => onSelectSkill(skill.id)}
          type="button"
        >
          <SkillIcon skill={skill} />
          <span className="min-w-0">
            <span className="block truncate font-bold text-v3-ink">{skill.name}</span>
            <span className="mt-0.5 block max-w-[310px] text-[13px] leading-5 text-v3-ink-2">
              {skill.description}
            </span>
          </span>
        </button>
      </V3Td>
      <V3Td>
        <StatusPill tone={risk.tone}>{risk.label}</StatusPill>
      </V3Td>
      <V3Td>
        <SkillTagStack tags={skill.tags} />
      </V3Td>
      <V3Td className="font-mono text-[13px] text-v3-ink-2">{skill.version}</V3Td>
      <V3Td>
        <BindingSummary skill={skill} />
      </V3Td>
      <V3Td>
        <StatusPill tone={status.tone}>{status.label}</StatusPill>
      </V3Td>
      <V3Td>
        <div className="flex justify-end gap-2">
          <V3Button
            aria-label={`查看详情 ${skill.name}`}
            onClick={() => onSelectSkill(skill.id)}
            size="sm"
            type="button"
            variant="outline"
          >
            查看详情
          </V3Button>
          <V3Button
            aria-label={`安装 ${skill.name}`}
            onClick={() => {
              onSelectSkill(skill.id);
              onInstallSkill(skill.id);
            }}
            size="sm"
            type="button"
          >
            安装
          </V3Button>
          <V3Button
            aria-label={`更多操作 ${skill.name}`}
            onClick={() => onSelectSkill(skill.id)}
            size="icon"
            type="button"
            variant="ghost"
          >
            <MoreHorizontal />
          </V3Button>
        </div>
      </V3Td>
    </V3Tr>
  );
}

function SkillMarketGrid({
  onInstallSkill,
  onSelectSkill,
  rows,
  selectedSkillId,
}: {
  onInstallSkill: (id: string) => void;
  onSelectSkill: (id: string) => void;
  rows: Skill[];
  selectedSkillId?: string;
}) {
  if (rows.length === 0) {
    return (
      <div className="p-8 text-center text-sm text-v3-ink-3">
        暂无匹配技能，请调整搜索或筛选条件
      </div>
    );
  }

  return (
    <div className="grid gap-4 p-4 md:grid-cols-2 xl:grid-cols-3">
      {rows.map((skill) => {
        const risk = riskDisplay(skill.risk_level);
        const status = statusDisplay(skill);
        const selected = selectedSkillId === skill.id;
        return (
          <SoftCard
            className={cn(
              "flex min-w-0 flex-col gap-4 border border-v3-line p-4",
              selected && "border-v3-brand ring-1 ring-v3-brand",
            )}
            interactive
            key={skill.id}
            onClick={() => onSelectSkill(skill.id)}
            role="button"
            tabIndex={0}
          >
            <div className="flex min-w-0 items-start gap-3">
              <SkillIcon skill={skill} />
              <div className="min-w-0 flex-1">
                <p className="truncate font-bold text-v3-ink">{skill.name}</p>
                <p className="mt-0.5 line-clamp-2 text-[13px] leading-5 text-v3-ink-2">
                  {skill.description}
                </p>
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              <StatusPill tone={risk.tone}>{risk.label}</StatusPill>
              <StatusPill tone={status.tone}>{status.label}</StatusPill>
            </div>
            <div className="flex items-center justify-between gap-3 border-t border-v3-line pt-3 text-[13px] text-v3-ink-2">
              <span className="inline-flex items-center gap-1.5">
                <Users className="size-4 text-v3-ink-3" />
                <span className="font-bold tabular-nums text-v3-ink">{skill.team_bindings.length}</span>
              </span>
              <span className="inline-flex items-center gap-1.5">
                <Bot className="size-4 text-v3-ink-3" />
                <span className="font-bold tabular-nums text-v3-ink">{skill.agent_bindings.length}</span>
              </span>
              <span className="font-mono text-xs text-v3-ink-3">{skill.version}</span>
            </div>
            <div className="flex justify-end gap-2">
              <V3Button
                aria-label={`查看详情 ${skill.name}`}
                onClick={(event) => {
                  event.stopPropagation();
                  onSelectSkill(skill.id);
                }}
                size="sm"
                type="button"
                variant="outline"
              >
                查看详情
              </V3Button>
              <V3Button
                aria-label={`安装 ${skill.name}`}
                onClick={(event) => {
                  event.stopPropagation();
                  onSelectSkill(skill.id);
                  onInstallSkill(skill.id);
                }}
                size="sm"
                type="button"
              >
                安装
              </V3Button>
            </div>
          </SoftCard>
        );
      })}
    </div>
  );
}

function SkillMarketPagination({
  currentPage,
  onPageChange,
  onPageSizeChange,
  pageCount,
  pageSize,
  total,
}: {
  currentPage: number;
  onPageChange: (page: number) => void;
  onPageSizeChange: (pageSize: number) => void;
  pageCount: number;
  pageSize: number;
  total: number;
}) {
  const pages = Array.from({ length: Math.min(pageCount, 5) }, (_, index) => index + 1);

  return (
    <div className="flex min-w-0 flex-col gap-3 border-t border-v3-line p-4 text-sm text-v3-ink-2 md:flex-row md:items-center md:justify-between">
      <span className="tabular-nums">共 {total} 条</span>
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex items-center gap-1">
          <V3Button
            aria-label="上一页"
            disabled={currentPage <= 1}
            onClick={() => onPageChange(Math.max(1, currentPage - 1))}
            size="icon"
            type="button"
            variant="ghost"
          >
            <ChevronLeft />
          </V3Button>
          {pages.map((pageNumber) => (
            <V3Button
              aria-current={currentPage === pageNumber ? "page" : undefined}
              className={cn(
                "size-8 px-0 tabular-nums",
                currentPage === pageNumber && "pointer-events-none",
              )}
              key={pageNumber}
              onClick={() => onPageChange(pageNumber)}
              size="sm"
              type="button"
              variant={currentPage === pageNumber ? "primary" : "ghost"}
            >
              {pageNumber}
            </V3Button>
          ))}
          {pageCount > 5 ? <span className="px-2">...</span> : null}
          <V3Button
            aria-label="下一页"
            disabled={currentPage >= pageCount}
            onClick={() => onPageChange(Math.min(pageCount, currentPage + 1))}
            size="icon"
            type="button"
            variant="ghost"
          >
            <ChevronRight />
          </V3Button>
        </div>
        <Select onValueChange={(value) => onPageSizeChange(Number(value))} value={`${pageSize}`}>
          <SelectTrigger
            aria-label="每页条数"
            className="w-[112px] rounded-xl border-v3-line-strong bg-v3-card text-v3-ink shadow-none hover:bg-v3-card-soft"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="10">10 条/页</SelectItem>
            <SelectItem value="20">20 条/页</SelectItem>
            <SelectItem value="50">50 条/页</SelectItem>
          </SelectContent>
        </Select>
      </div>
    </div>
  );
}

function SkillTagStack({ tags }: { tags: string[] }) {
  const visibleTags = tags.slice(0, 2);
  const extraCount = Math.max(0, tags.length - visibleTags.length);

  return (
    <div className="flex max-w-[180px] flex-wrap gap-1.5">
      {visibleTags.map((tag) => (
        <span
          className="rounded-lg bg-v3-mute-soft px-2 py-0.5 text-xs font-medium text-v3-ink-2"
          key={tag}
        >
          {tag}
        </span>
      ))}
      {extraCount > 0 ? (
        <span className="rounded-lg bg-v3-mute-soft px-2 py-0.5 text-xs font-medium text-v3-ink-2">
          +{extraCount}
        </span>
      ) : null}
    </div>
  );
}

function BindingSummary({ skill }: { skill: Skill }) {
  return (
    <div className="space-y-1 text-[13px] leading-5 text-v3-ink-2">
      <div className="flex items-center gap-2">
        <Users className="size-4 text-v3-ink-3" />
        <span>团队</span>
        <span className="font-bold tabular-nums text-v3-ink">{skill.team_bindings.length}</span>
      </div>
      <div className="flex items-center gap-2">
        <Bot className="size-4 text-v3-ink-3" />
        <span>数字员工</span>
        <span className="font-bold tabular-nums text-v3-ink">{skill.agent_bindings.length}</span>
      </div>
    </div>
  );
}

function SkillIcon({ size = "default", skill }: { size?: "sm" | "default"; skill: Skill }) {
  const Icon = iconMap[skill.icon_key] ?? Blocks;
  return (
    <IconTile aria-label={`${skill.name} 图标`} tone={toneByColor[skill.color_token] ?? "brand"} size={size}>
      <Icon />
    </IconTile>
  );
}

function installationTargetLabel(installation: SkillInstallation) {
  return installation.employee_name || installation.digital_employee_id || installation.node_id || installation.runtime_node_id || "未知目标";
}

function runtimeNodeLabel(installation: SkillInstallation) {
  return [installation.runtime_node_id, installation.node_id].filter(Boolean).join(" · ") || "未记录运行节点";
}

function providerTone(providerType: SkillInstallation["provider_type"]): V3Tone {
  if (providerType === "codex") return "brand";
  if (providerType === "claude-code") return "artifact";
  return "info";
}

function buildMarketMetrics(skills: Skill[]): MetricDefinition[] {
  const dependencyCount = skills.filter((skill) => runtimeDependencyCount(skill) > 0).length;
  const approvalCount = skills.filter(needsApproval).length;
  return [
    { icon: ClipboardList, label: "技能总数", tone: "brand", value: skills.length },
    {
      icon: CheckCircle2,
      label: "可安装",
      tone: "ok",
      value: skills.filter((skill) => statusDisplay(skill).value === "available").length,
    },
    {
      icon: TriangleAlert,
      label: "需补全依赖",
      tone: "warn",
      value: dependencyCount,
      loud: dependencyCount > 0,
    },
    {
      icon: UserRoundCheck,
      label: "需审批",
      tone: "danger",
      value: approvalCount,
    },
    { icon: Blocks, label: "团队绑定", tone: "info", value: countTeamBindings(skills) },
    { icon: Bot, label: "数字员工绑定", tone: "artifact", value: countAgentBindings(skills) },
  ];
}

function filterSkills(
  rows: Skill[],
  filters: {
    dependencyFilter: DependencyFilter;
    riskFilter: RiskFilter;
    scopeFilter: ScopeFilter;
    statusFilter: StatusFilter;
  },
) {
  return rows.filter((skill) => {
    const risk = normalizeRisk(skill.risk_level);
    const status = statusDisplay(skill).value;
    const dependencyCount = runtimeDependencyCount(skill);
    const hasTeamBindings = skill.team_bindings.length > 0;
    const hasAgentBindings = skill.agent_bindings.length > 0;

    if (filters.riskFilter !== "all" && risk !== filters.riskFilter) return false;
    if (filters.statusFilter !== "all" && status !== filters.statusFilter) return false;
    if (filters.dependencyFilter === "none" && dependencyCount > 0) return false;
    if (filters.dependencyFilter === "required" && dependencyCount === 0) return false;
    if (filters.scopeFilter === "team" && !hasTeamBindings) return false;
    if (filters.scopeFilter === "employee" && !hasAgentBindings) return false;
    if (filters.scopeFilter === "unbound" && (hasTeamBindings || hasAgentBindings)) return false;

    return true;
  });
}

function statusDisplay(skill: Skill): StatusDisplay {
  if (needsApproval(skill)) return { label: "需审批", tone: "danger", value: "approval" };
  if (runtimeDependencyCount(skill) > 0) return { label: "需补全依赖", tone: "warn", value: "dependency" };
  if (skill.team_bindings.length > 0 || skill.agent_bindings.length > 0) {
    return { label: "已安装", tone: "ok", value: "installed" };
  }
  return { label: "可安装", tone: "info", value: "available" };
}

function riskDisplay(riskLevel: string): { label: string; tone: V3Tone } {
  const risk = normalizeRisk(riskLevel);
  if (risk === "high") return { label: "高风险", tone: "danger" };
  if (risk === "medium") return { label: "中风险", tone: "warn" };
  return { label: "低风险", tone: "ok" };
}

function normalizeRisk(riskLevel: string): Exclude<RiskFilter, "all"> {
  const normalized = riskLevel.toLowerCase();
  if (normalized === "high" || normalized === "critical") return "high";
  if (normalized === "medium") return "medium";
  return "low";
}

function needsApproval(skill: Skill) {
  return normalizeRisk(skill.risk_level) === "high";
}

function runtimeDependencyCount(skill: Skill) {
  return (skill.runtime_dependencies?.tools?.length ?? 0) + (skill.runtime_dependencies?.env?.length ?? 0);
}

function countTeamBindings(skills: Skill[]) {
  return skills.reduce((total, skill) => total + skill.team_bindings.length, 0);
}

function countAgentBindings(skills: Skill[]) {
  return skills.reduce((total, skill) => total + skill.agent_bindings.length, 0);
}

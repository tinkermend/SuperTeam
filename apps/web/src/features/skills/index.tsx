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
  Search as SearchIcon,
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
  LiquidCard,
  SemanticIconTile,
  StatusBadge,
  type Tone,
} from "@/components/superteam";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
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
import { listSkills, type Skill } from "@/lib/api/skills";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { cn } from "@/lib/utils";

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
  tone: Tone;
  value: number;
};

type StatusDisplay = {
  label: string;
  tone: Tone;
  value: SkillMarketStatus;
};

const iconMap: Record<string, LucideIcon> = {
  blocks: Blocks,
  flask: FileText,
  "server-cog": ServerCog,
  "shield-check": ShieldCheck,
  stethoscope: Stethoscope,
};

const toneByColor: Record<string, Tone> = {
  blue: "info",
  cyan: "info",
  emerald: "success",
  teal: "primary",
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
  const [pageSize, setPageSize] = useState(10);
  const [page, setPage] = useState(1);

  const apiOptions: ApiOpts = { baseUrl: apiBaseUrl, fetcher };
  const skills = useQuery({
    queryKey: ["skills", query],
    queryFn: () => listSkills(apiOptions, { q: query }),
  });

  const skillRows = skills.data ?? [];
  const selectedSkill = skillRows.find((skill) => skill.id === selectedSkillId) ?? skillRows[0];
  const skillsError = skills.error instanceof Error ? skills.error.message : undefined;
  const metrics = useMemo(() => buildMarketMetrics(skillRows), [skillRows]);
  const filteredRows = useMemo(
    () => filterSkills(skillRows, { dependencyFilter, riskFilter, scopeFilter, statusFilter }),
    [dependencyFilter, riskFilter, scopeFilter, skillRows, statusFilter],
  );
  const pageCount = Math.max(1, Math.ceil(filteredRows.length / pageSize));
  const activePage = Math.min(page, pageCount);
  const pagedRows = filteredRows.slice((activePage - 1) * pageSize, activePage * pageSize);

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main className="min-w-0 overflow-x-hidden">
        <div className="flex min-w-0 flex-col gap-5">
          <header className="flex min-w-0 flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
            <div className="min-w-0">
              <h1 className="text-2xl font-bold tracking-normal">技能市场</h1>
              <p className="mt-1 text-sm text-muted-foreground">发现、校验并安装技能到团队或数字员工</p>
            </div>
            <Button asChild className="self-start">
              <Link to="/skills/upload">
                <UploadCloud data-icon="inline-start" />
                上传技能
              </Link>
            </Button>
          </header>

          <section className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6" aria-label="技能市场指标">
            {metrics.map((metric) => (
              <SkillMarketMetric key={metric.label} metric={metric} />
            ))}
          </section>

          {skills.isError ? (
            <Alert variant="destructive">
              <TriangleAlert />
              <AlertTitle>技能数据加载失败</AlertTitle>
              <AlertDescription>{skillsError ?? "请检查 Control Plane 技能接口和数据库迁移状态。"}</AlertDescription>
            </Alert>
          ) : null}

          <LiquidCard className="min-w-0 rounded-lg p-0">
            <CardContent className="p-0">
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

              {viewMode === "list" ? (
                <SkillMarketTable
                  onSelectSkill={setSelectedSkillId}
                  rows={pagedRows}
                  selectedSkillId={selectedSkill?.id}
                />
              ) : (
                <SkillMarketGrid
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
            </CardContent>
          </LiquidCard>
        </div>
      </Main>
    </>
  );
}

function SkillMarketMetric({ metric }: { metric: MetricDefinition }) {
  const Icon = metric.icon;
  return (
    <LiquidCard role="group" aria-label={`${metric.label} ${metric.value}`} className="rounded-lg py-0">
      <CardContent className="flex items-center gap-3 p-4">
        <SemanticIconTile tone={metric.tone} size="sm">
          <Icon />
        </SemanticIconTile>
        <div className="min-w-0">
          <p className="truncate text-sm text-muted-foreground">{metric.label}</p>
          <p className="text-2xl font-semibold leading-tight tracking-normal">{metric.value}</p>
        </div>
      </CardContent>
    </LiquidCard>
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
    <div className="flex min-w-0 flex-col gap-3 border-b p-4 lg:flex-row lg:items-center lg:justify-between">
      <div className="flex min-w-0 flex-1 flex-col gap-3 lg:flex-row lg:items-center">
        <label className="flex min-w-0 items-center gap-2 rounded-md border bg-background/80 px-3">
          <SearchIcon className="size-4 text-muted-foreground" />
          <Input
            aria-label="搜索技能"
            className="h-9 min-w-0 border-0 bg-transparent px-0 shadow-none focus-visible:ring-0"
            onChange={(event) => onQueryChange(event.target.value)}
            placeholder="搜索技能、标签、运行依赖"
            type="search"
            value={query}
          />
        </label>
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
      <div className="flex self-start rounded-md border bg-background/80 p-1">
        <Button
          aria-label="列表视图"
          aria-pressed={viewMode === "list"}
          className={cn("h-8 rounded-md px-2.5", viewMode === "list" && "bg-secondary text-secondary-foreground")}
          onClick={() => onViewModeChange("list")}
          size="sm"
          type="button"
          variant="ghost"
        >
          <List />
        </Button>
        <Button
          aria-label="网格视图"
          aria-pressed={viewMode === "grid"}
          className={cn("h-8 rounded-md px-2.5", viewMode === "grid" && "bg-secondary text-secondary-foreground")}
          onClick={() => onViewModeChange("grid")}
          size="sm"
          type="button"
          variant="ghost"
        >
          <Grid2X2 />
        </Button>
      </div>
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
      <SelectTrigger aria-label={label} className="w-full min-w-0 bg-background/70 lg:w-[124px]">
        <SelectValue />
      </SelectTrigger>
      <SelectContent>
        {options.map((option) => (
          <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
        ))}
      </SelectContent>
    </Select>
  );
}

function SkillMarketTable({
  onSelectSkill,
  rows,
  selectedSkillId,
}: {
  onSelectSkill: (id: string) => void;
  rows: Skill[];
  selectedSkillId?: string;
}) {
  return (
    <Table>
      <TableHeader className="bg-muted/35">
        <TableRow className="hover:bg-muted/35">
          <TableHead className="min-w-[300px] px-4" role="columnheader">技能</TableHead>
          <TableHead className="min-w-24" role="columnheader">风险</TableHead>
          <TableHead className="min-w-[180px]" role="columnheader">标签</TableHead>
          <TableHead className="min-w-24" role="columnheader">版本</TableHead>
          <TableHead className="min-w-32" role="columnheader">绑定目标</TableHead>
          <TableHead className="min-w-28" role="columnheader">状态</TableHead>
          <TableHead className="min-w-[220px] text-right" role="columnheader">操作</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((skill) => (
          <SkillMarketTableRow
            key={skill.id}
            onSelectSkill={onSelectSkill}
            selected={selectedSkillId === skill.id}
            skill={skill}
          />
        ))}
        {rows.length === 0 ? (
          <TableRow>
            <TableCell colSpan={7} className="h-32 text-center text-muted-foreground">
              暂无匹配技能，请调整搜索或筛选条件
            </TableCell>
          </TableRow>
        ) : null}
      </TableBody>
    </Table>
  );
}

function SkillMarketTableRow({
  onSelectSkill,
  selected,
  skill,
}: {
  onSelectSkill: (id: string) => void;
  selected: boolean;
  skill: Skill;
}) {
  const risk = riskDisplay(skill.risk_level);
  const status = statusDisplay(skill);

  return (
    <TableRow
      className={cn("h-[88px] border-b bg-background/35", selected && "bg-primary/5 outline outline-1 -outline-offset-1 outline-primary/70")}
      data-state={selected ? "selected" : undefined}
    >
      <TableCell className="px-4 whitespace-normal">
        <button
          className="flex min-w-0 items-start gap-3 text-left"
          onClick={() => onSelectSkill(skill.id)}
          type="button"
        >
          <SkillIcon skill={skill} />
          <span className="min-w-0">
            <span className="block truncate font-semibold">{skill.name}</span>
            <span className="mt-1 block max-w-[310px] text-sm leading-5 text-muted-foreground">{skill.description}</span>
          </span>
        </button>
      </TableCell>
      <TableCell>
        <StatusBadge tone={risk.tone}>{risk.label}</StatusBadge>
      </TableCell>
      <TableCell>
        <SkillTagStack tags={skill.tags} />
      </TableCell>
      <TableCell className="font-medium">{skill.version}</TableCell>
      <TableCell>
        <BindingSummary skill={skill} />
      </TableCell>
      <TableCell>
        <StatusBadge tone={status.tone}>{status.label}</StatusBadge>
      </TableCell>
      <TableCell>
        <div className="flex justify-end gap-2">
          <Button
            aria-label={`查看详情 ${skill.name}`}
            onClick={() => onSelectSkill(skill.id)}
            size="sm"
            type="button"
            variant="outline"
          >
            查看详情
          </Button>
          <Button
            aria-label={`安装 ${skill.name}`}
            onClick={() => onSelectSkill(skill.id)}
            size="sm"
            type="button"
          >
            安装
          </Button>
          <Button
            aria-label={`更多操作 ${skill.name}`}
            onClick={() => onSelectSkill(skill.id)}
            size="icon"
            type="button"
            variant="ghost"
          >
            <MoreHorizontal />
          </Button>
        </div>
      </TableCell>
    </TableRow>
  );
}

function SkillMarketGrid({
  onSelectSkill,
  rows,
  selectedSkillId,
}: {
  onSelectSkill: (id: string) => void;
  rows: Skill[];
  selectedSkillId?: string;
}) {
  if (rows.length === 0) {
    return <div className="p-8 text-center text-sm text-muted-foreground">暂无匹配技能，请调整搜索或筛选条件</div>;
  }

  return (
    <div className="grid gap-3 p-4 md:grid-cols-2 xl:grid-cols-3">
      {rows.map((skill) => {
        const risk = riskDisplay(skill.risk_level);
        const status = statusDisplay(skill);
        return (
          <button
            className={cn(
              "flex min-w-0 flex-col gap-4 rounded-lg border bg-background/60 p-4 text-left transition-colors hover:bg-accent/55",
              selectedSkillId === skill.id && "border-primary bg-primary/5",
            )}
            key={skill.id}
            onClick={() => onSelectSkill(skill.id)}
            type="button"
          >
            <span className="flex min-w-0 items-start gap-3">
              <SkillIcon skill={skill} />
              <span className="min-w-0 flex-1">
                <span className="block truncate font-semibold">{skill.name}</span>
                <span className="mt-1 line-clamp-2 block text-sm text-muted-foreground">{skill.description}</span>
              </span>
            </span>
            <span className="flex flex-wrap gap-2">
              <StatusBadge tone={risk.tone}>{risk.label}</StatusBadge>
              <StatusBadge tone={status.tone}>{status.label}</StatusBadge>
            </span>
            <span className="flex items-center justify-between gap-3 text-sm text-muted-foreground">
              <span>团队 {skill.team_bindings.length}</span>
              <span>数字员工 {skill.agent_bindings.length}</span>
              <span>{skill.version}</span>
            </span>
            <span className="flex justify-end gap-2">
              <Button aria-label={`查看详情 ${skill.name}`} size="sm" type="button" variant="outline">查看详情</Button>
              <Button aria-label={`安装 ${skill.name}`} size="sm" type="button">安装</Button>
            </span>
          </button>
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
    <div className="flex min-w-0 flex-col gap-3 border-t p-4 text-sm text-muted-foreground md:flex-row md:items-center md:justify-between">
      <span>共 {total} 条</span>
      <div className="flex flex-wrap items-center gap-3">
        <div className="flex items-center gap-1">
          <Button
            aria-label="上一页"
            disabled={currentPage <= 1}
            onClick={() => onPageChange(Math.max(1, currentPage - 1))}
            size="icon"
            type="button"
            variant="ghost"
          >
            <ChevronLeft />
          </Button>
          {pages.map((pageNumber) => (
            <Button
              aria-current={currentPage === pageNumber ? "page" : undefined}
              className={cn("size-8 rounded-md px-0", currentPage === pageNumber && "bg-secondary text-secondary-foreground")}
              key={pageNumber}
              onClick={() => onPageChange(pageNumber)}
              size="sm"
              type="button"
              variant="ghost"
            >
              {pageNumber}
            </Button>
          ))}
          {pageCount > 5 ? <span className="px-2">...</span> : null}
          <Button
            aria-label="下一页"
            disabled={currentPage >= pageCount}
            onClick={() => onPageChange(Math.min(pageCount, currentPage + 1))}
            size="icon"
            type="button"
            variant="ghost"
          >
            <ChevronRight />
          </Button>
        </div>
        <Select onValueChange={(value) => onPageSizeChange(Number(value))} value={`${pageSize}`}>
          <SelectTrigger aria-label="每页条数" className="w-[112px] bg-background/70">
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
        <Badge className="bg-background/70 text-muted-foreground" key={tag} variant="outline">{tag}</Badge>
      ))}
      {extraCount > 0 ? <Badge className="bg-background/70 text-muted-foreground" variant="outline">+{extraCount}</Badge> : null}
    </div>
  );
}

function BindingSummary({ skill }: { skill: Skill }) {
  return (
    <div className="space-y-1 text-sm leading-5">
      <div className="flex items-center gap-2">
        <Users className="size-4 text-muted-foreground" />
        <span>团队</span>
        <span className="font-medium">{skill.team_bindings.length}</span>
      </div>
      <div className="flex items-center gap-2">
        <Bot className="size-4 text-muted-foreground" />
        <span>数字员工</span>
        <span className="font-medium">{skill.agent_bindings.length}</span>
      </div>
    </div>
  );
}

function SkillIcon({ size = "default", skill }: { size?: "sm" | "default"; skill: Skill }) {
  const Icon = iconMap[skill.icon_key] ?? Blocks;
  return (
    <SemanticIconTile aria-label={`${skill.name} 图标`} tone={toneByColor[skill.color_token] ?? "primary"} size={size}>
      <Icon />
    </SemanticIconTile>
  );
}

function buildMarketMetrics(skills: Skill[]): MetricDefinition[] {
  return [
    { icon: ClipboardList, label: "技能总数", tone: "info", value: skills.length },
    { icon: CheckCircle2, label: "可安装", tone: "success", value: skills.filter((skill) => statusDisplay(skill).value === "available").length },
    { icon: TriangleAlert, label: "需补全依赖", tone: "warning", value: skills.filter((skill) => runtimeDependencyCount(skill) > 0).length },
    { icon: UserRoundCheck, label: "需审批", tone: "danger", value: skills.filter(needsApproval).length },
    { icon: Blocks, label: "团队绑定", tone: "info", value: countTeamBindings(skills) },
    { icon: Bot, label: "数字员工绑定", tone: "primary", value: countAgentBindings(skills) },
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
  if (runtimeDependencyCount(skill) > 0) return { label: "需补全依赖", tone: "warning", value: "dependency" };
  if (skill.team_bindings.length > 0 || skill.agent_bindings.length > 0) {
    return { label: "已安装", tone: "success", value: "installed" };
  }
  return { label: "可安装", tone: "info", value: "available" };
}

function riskDisplay(riskLevel: string): { label: string; tone: Tone } {
  const risk = normalizeRisk(riskLevel);
  if (risk === "high") return { label: "高风险", tone: "danger" };
  if (risk === "medium") return { label: "中风险", tone: "warning" };
  return { label: "低风险", tone: "success" };
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

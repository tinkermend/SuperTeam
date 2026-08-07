import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  Blocks,
  Bot,
  CheckCircle2,
  ClipboardList,
  FileText,
  ServerCog,
  ShieldCheck,
  Stethoscope,
  Trash2,
  UploadCloud,
  UserRoundCheck,
  Users,
  type LucideIcon
} from "lucide-react";
import {
  Button,
  CardGridSkeleton,
  Chip,
  EmptyNoData,
  EmptyNoMatch,
  EmptyState,
  EntityCard,
  ErrorState,
  IconTile,
  ListToolbar,
  MetricCard,
  MetricGrid,
  Pagination,
  StatusPill,
  ToolbarSearch,
  WorkSurface,
  shortId,
  type Tone
} from "@/components/superteam";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle
} from "@/components/ui/sheet";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import { deleteSkill, listSkills, type Skill } from "@/lib/api/skills";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { cn } from "@/lib/utils";
import { SkillInstallDialog } from "./install-dialog";
import { missingObjectLabel } from "@/lib/status-labels";

type SkillsViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

type ApiOpts = { baseUrl: string; fetcher?: typeof fetch };
type RiskFilter = "all" | "low" | "medium" | "high";
type ScopeFilter = "all" | "team" | "employee" | "unbound";
type DependencyFilter = "all" | "none" | "required";
type StatusFilter = "all" | SkillMarketStatus;

type SkillMarketStatus = "installed" | "available" | "approval";

type MetricDefinition = {
  icon: LucideIcon;
  label: string;
  tone: Tone;
  value: number;
  loud?: boolean;
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
  stethoscope: Stethoscope
};

const toneByColor: Record<string, Tone> = {
  blue: "info",
  cyan: "info",
  emerald: "ok",
  teal: "brand",
  violet: "artifact"
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
  const [selectedSkillId, setSelectedSkillId] = useState<string>();
  const [installSkillId, setInstallSkillId] = useState<string>();
  const [detailOpen, setDetailOpen] = useState(false);
  const [deleteSkillId, setDeleteSkillId] = useState<string>();
  const [deleteError, setDeleteError] = useState<string>();
  const [pageSize, setPageSize] = useState(10);
  const [page, setPage] = useState(1);

  const queryClient = useQueryClient();
  const apiOptions: ApiOpts = { baseUrl: apiBaseUrl, fetcher };
  const skills = useQuery({
    queryKey: ["skills", query],
    queryFn: () => listSkills(apiOptions, { q: query })
});
  const deleteMutation = useMutation({
    mutationFn: (skillId: string) => deleteSkill(apiOptions, skillId),
    onSuccess: (_, skillId) => {
      setDeleteSkillId(undefined);
      setDeleteError(undefined);
      setDetailOpen(false);
      setSelectedSkillId((current) => (current === skillId ? undefined : current));
      void queryClient.invalidateQueries({ queryKey: ["skills"] });
    },
    onError: (error) => {
      setDeleteError(error instanceof Error ? error.message : "删除技能失败，请稍后重试。");
    }
});

  const skillRows = skills.data ?? [];
  const skillsError = skills.error instanceof Error ? skills.error.message : undefined;
  const metrics = useMemo(() => buildMarketMetrics(skillRows), [skillRows]);
  const statusCounts = useMemo(() => countByStatus(skillRows), [skillRows]);
  const filteredRows = useMemo(
    () => filterSkills(skillRows, { dependencyFilter, riskFilter, scopeFilter, statusFilter }),
    [dependencyFilter, riskFilter, scopeFilter, skillRows, statusFilter],
  );
  const selectedSkill = filteredRows.find((skill) => skill.id === selectedSkillId) ?? filteredRows[0];
  const installSkillTarget = skillRows.find((skill) => skill.id === installSkillId);
  const deleteSkillTarget = skillRows.find((skill) => skill.id === deleteSkillId);
  const pageCount = Math.max(1, Math.ceil(filteredRows.length / pageSize));
  const activePage = Math.min(page, pageCount);
  const pagedRows = filteredRows.slice((activePage - 1) * pageSize, activePage * pageSize);
  const isInitialLoading = skills.isPending && skillRows.length === 0;
  const isBlockingError = skills.isError && skillRows.length === 0;

  return (
    <>
      <ShellPageHeader
        icon={<Blocks />}
        iconTone="artifact"
        title="技能市场"
        subtitle="发现、查看并治理技能档案与绑定范围"
      />
      <Main width="wide" className="min-w-0 overflow-x-hidden">
        <div className="flex min-w-0 flex-col gap-5">
          <div className="flex flex-wrap items-center justify-start gap-2 sm:justify-end">
            <Button asChild className="h-11 self-start px-5">
              <Link to="/skills/upload">
                <UploadCloud data-icon="inline-start" />
                上传技能
              </Link>
            </Button>
          </div>

          <MetricGrid aria-label="技能市场指标">
            {metrics.map((metric) => {
              const Icon = metric.icon;
              return (
                <MetricCard
                  key={metric.label}
                  label={metric.label}
                  value={String(metric.value)}
                  loud={metric.loud}
                  icon={<Icon />}
                  iconTone={metric.tone}
                />
              );
            })}
          </MetricGrid>

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
              query={query}
              riskFilter={riskFilter}
              scopeFilter={scopeFilter}
              statusCounts={statusCounts}
              statusFilter={statusFilter}
            />

            {isInitialLoading ? (
              <div className="p-4">
                <CardGridSkeleton count={8} className="sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4" />
              </div>
            ) : isBlockingError ? (
              <div className="p-4">
                <ErrorState
                  title="技能数据加载失败"
                  description={skillsError ?? "请检查 Control Plane 技能接口和数据库迁移状态。"}
                />
              </div>
            ) : (
              <>
                {skills.isError ? (
                  <div className="border-b border-line p-4">
                    <ErrorState
                      title="技能数据加载失败"
                      description={skillsError ?? "请检查 Control Plane 技能接口和数据库迁移状态。"}
                    />
                  </div>
                ) : null}
                <SkillMarketGrid
                  hasActiveFilters={Boolean(
                    query.trim() ||
                      riskFilter !== "all" ||
                      scopeFilter !== "all" ||
                      dependencyFilter !== "all" ||
                      statusFilter !== "all"
                  )}
                  onDeleteSkill={(id) => {
                    setDeleteError(undefined);
                    setDeleteSkillId(id);
                  }}
                  onInstallSkill={setInstallSkillId}
                  onOpenDetail={(id) => {
                    setSelectedSkillId(id);
                    setDetailOpen(true);
                  }}
                  onSelectSkill={setSelectedSkillId}
                  rows={pagedRows}
                  selectedSkillId={selectedSkill?.id}
                  totalUnfiltered={skillRows.length}
                />

                <div className="border-t border-line p-3">
                  <Pagination
                    total={filteredRows.length}
                    page={activePage}
                    pageSize={pageSize}
                    pageCount={pageCount}
                    onPageChange={setPage}
                    onPageSizeChange={(size) => {
                      setPageSize(size);
                      setPage(1);
                    }}
                  />
                </div>
              </>
            )}
          </WorkSurface>

          {selectedSkill ? <SelectedSkillBindings skill={selectedSkill} /> : null}
        </div>
      </Main>
      <SkillDetailSheet
        onDeleteSkill={(id) => {
          setDeleteError(undefined);
          setDeleteSkillId(id);
        }}
        onOpenChange={setDetailOpen}
        open={detailOpen && Boolean(selectedSkill)}
        skill={selectedSkill}
      />
      <ConfirmDialog
        open={Boolean(deleteSkillTarget)}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteSkillId(undefined);
            setDeleteError(undefined);
          }
        }}
        title={`删除技能 ${deleteSkillTarget?.name ?? ""}`}
        desc={
          <div className="space-y-2">
            <p>
              删除后技能将从市场下架，
              {deleteSkillTarget && deleteSkillTarget.team_bindings.length + deleteSkillTarget.agent_bindings.length > 0
                ? `当前 ${deleteSkillTarget.team_bindings.length} 个团队绑定与 ${deleteSkillTarget.agent_bindings.length} 个数字员工绑定会同时解除，`
                : ""}
              归档文件将被清除，此操作不可撤销。
            </p>
            {deleteError ? <p className="font-semibold text-danger">{deleteError}</p> : null}
          </div>
        }
        confirmText="删除"
        destructive
        isLoading={deleteMutation.isPending}
        handleConfirm={() => {
          if (deleteSkillTarget) {
            deleteMutation.mutate(deleteSkillTarget.id);
          }
        }}
      />
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

// SelectedSkillBindings 展示所选技能的逻辑绑定范围(团队/数字员工)。
// 物理物化在任务派发时由 runtime 懒收敛完成,事实进入派发 attestation,
// 平台不再维护独立的"安装记录"。
function SelectedSkillBindings({ skill }: { skill: Skill }) {
  const bindingCount = skill.team_bindings.length + skill.agent_bindings.length;
  return (
    <WorkSurface
      aria-label={`${skill.name} 加载范围`}
      className="min-w-0 overflow-hidden"
      role="region"
    >
      <div className="flex min-w-0 flex-col gap-3 border-b border-line p-4 md:flex-row md:items-start md:justify-between">
        <div className="flex min-w-0 items-start gap-3">
          <SkillIcon skill={skill} size="sm" />
          <div className="min-w-0">
            <h2 className="truncate text-base font-bold text-ink">加载范围</h2>
            <p className="mt-1 min-w-0 text-[13px] leading-5 text-ink-2">
              <span className="font-semibold text-ink">{skill.name}</span>
              <span className="px-2 text-ink-3">·</span>
              <span className="font-mono">{skill.version}</span>
            </p>
          </div>
        </div>
        <StatusPill tone={bindingCount > 0 ? "ok" : "info"}>{bindingCount} 个目标</StatusPill>
      </div>

      {bindingCount === 0 ? (
        <EmptyState
          className="py-10"
          title="尚未加载到任何目标"
          description="通过“加载”把技能绑定到团队或数字员工;技能文件在下次任务派发时同步到运行环境。"
        />
      ) : (
        <div className="grid gap-3 p-4 md:grid-cols-2">
          <div>
            <p className="mb-1.5 flex items-center gap-1.5 text-[12px] font-semibold text-ink-2">
              <Users className="size-3.5 text-ink-3" />团队（{skill.team_bindings.length}）
            </p>
            {skill.team_bindings.length === 0 ? (
              <p className="rounded-lg border border-dashed border-line-strong bg-card-inner px-3 py-2 text-[12px] text-ink-3">未绑定团队</p>
            ) : (
              <div className="space-y-1.5">
                {skill.team_bindings.map((team) => (
                  <div className="flex items-center gap-2 rounded-lg border border-line bg-card-inner px-3 py-1.5 text-[12px]" key={team.team_id}>
                    <span className="font-semibold text-ink">{team.team_name?.trim() || missingObjectLabel("team", team.team_id)}</span>
                    <span className="ml-auto font-mono text-[11px] text-ink-3" title={team.team_id}>{shortId(team.team_id)}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
          <div>
            <p className="mb-1.5 flex items-center gap-1.5 text-[12px] font-semibold text-ink-2">
              <Bot className="size-3.5 text-ink-3" />数字员工（{skill.agent_bindings.length}）
            </p>
            {skill.agent_bindings.length === 0 ? (
              <p className="rounded-lg border border-dashed border-line-strong bg-card-inner px-3 py-2 text-[12px] text-ink-3">未绑定数字员工</p>
            ) : (
              <div className="space-y-1.5">
                {skill.agent_bindings.map((agent) => (
                  <div className="flex items-center gap-2 rounded-lg border border-line bg-card-inner px-3 py-1.5 text-[12px]" key={agent.agent_id}>
                    <span className="font-semibold text-ink">{agent.agent_name?.trim() || missingObjectLabel("employee", agent.agent_id)}</span>
                    {agent.team_name?.trim() ? <span className="text-ink-3">· {agent.team_name.trim()}</span> : null}
                    <span className="ml-auto font-mono text-[11px] text-ink-3" title={agent.agent_id}>{shortId(agent.agent_id)}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
    </WorkSurface>
  );
}

/** 技能详情抽屉：点击「查看详情」从右侧滑出，展示完整信息，不跳转页面。 */
function SkillDetailSheet({
  onDeleteSkill,
  onOpenChange,
  open,
  skill
}: {
  onDeleteSkill: (id: string) => void;
  onOpenChange: (open: boolean) => void;
  open: boolean;
  skill?: Skill;
}) {
  if (!skill) return null;
  const risk = riskDisplay(skill.risk_level);
  const status = statusDisplay(skill);
  const depCount = runtimeDependencyCount(skill);
  const depTools = skill.runtime_dependencies?.tools ?? [];
  const depEnv = skill.runtime_dependencies?.env ?? [];
  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="w-[92vw] gap-0 p-0 sm:w-[640px] sm:max-w-[640px]" side="right">
        <SheetHeader className="flex-row items-center gap-3 border-b border-line p-4 pr-12">
          <SkillIcon skill={skill} size="sm" />
          <div className="min-w-0">
            <SheetTitle className="truncate text-base font-bold text-ink">{skill.name}</SheetTitle>
            <SheetDescription className="mt-0.5 truncate font-mono text-[11px] text-ink-3">
              {skill.version} · {skill.source}
            </SheetDescription>
          </div>
        </SheetHeader>
        <div className="flex-1 space-y-5 overflow-y-auto p-4">
          <section>
            <h3 className="mb-2.5 text-[11px] font-bold tracking-wide text-ink-3 uppercase">基础信息</h3>
            <div className="grid grid-cols-2 gap-x-4 gap-y-2.5">
              <div>
                <p className="text-[11px] font-semibold text-ink-3">风险等级</p>
                <p className="mt-0.5"><StatusPill tone={risk.tone}>{risk.label}</StatusPill></p>
              </div>
              <div>
                <p className="text-[11px] font-semibold text-ink-3">绑定状态</p>
                <p className="mt-0.5"><StatusPill tone={status.tone}>{status.label}</StatusPill></p>
              </div>
              <div>
                <p className="text-[11px] font-semibold text-ink-3">版本</p>
                <p className="mt-0.5 font-mono text-[13px] text-ink">{skill.version}</p>
              </div>
              <div>
                <p className="text-[11px] font-semibold text-ink-3">来源</p>
                <p className="mt-0.5 truncate font-mono text-[13px] text-ink">{skill.source}</p>
              </div>
              <div>
                <p className="text-[11px] font-semibold text-ink-3">团队绑定</p>
                <p className="mt-0.5 text-[13px] font-semibold text-ink">{skill.team_bindings.length} 个</p>
              </div>
              <div>
                <p className="text-[11px] font-semibold text-ink-3">数字员工绑定</p>
                <p className="mt-0.5 text-[13px] font-semibold text-ink">{skill.agent_bindings.length} 个</p>
              </div>
              <div>
                <p className="text-[11px] font-semibold text-ink-3">运行依赖</p>
                <p className="mt-0.5 text-[13px] font-semibold text-ink">{depCount} 项</p>
              </div>
              <div>
                <p className="text-[11px] font-semibold text-ink-3">创建人</p>
                <p className="mt-0.5 text-[13px] text-ink">{skill.created_by_name || "—"}</p>
              </div>
              <div>
                <p className="text-[11px] font-semibold text-ink-3">创建时间</p>
                <p className="mt-0.5 font-mono text-[12px] text-ink-2">{skill.created_at ?? "—"}</p>
              </div>
            </div>
          </section>

          <section>
            <h3 className="mb-2.5 text-[11px] font-bold tracking-wide text-ink-3 uppercase">运行依赖</h3>
            {depCount === 0 ? (
              <p className="text-[13px] text-ink-3">无运行依赖</p>
            ) : (
              <div className="flex flex-wrap gap-1.5">
                {depTools.map((tool) => (
                  <span className="rounded-md bg-info-soft px-2 py-1 font-mono text-[11px] font-semibold text-info-text" key={`tool-${tool}`}>tool: {tool}</span>
                ))}
                {depEnv.map((env) => (
                  <span className="rounded-md bg-artifact-soft px-2 py-1 font-mono text-[11px] font-semibold text-artifact-text" key={`env-${env}`}>env: {env}</span>
                ))}
              </div>
            )}
          </section>

          <section>
            <h3 className="mb-2.5 text-[11px] font-bold tracking-wide text-ink-3 uppercase">绑定范围</h3>
            <div className="space-y-3">
              <div>
                <p className="mb-1.5 flex items-center gap-1.5 text-[12px] font-semibold text-ink-2">
                  <Users className="size-3.5 text-ink-3" />团队（{skill.team_bindings.length}）
                </p>
                {skill.team_bindings.length === 0 ? (
                  <p className="rounded-lg border border-dashed border-line-strong bg-card-inner px-3 py-2 text-[12px] text-ink-3">未绑定团队</p>
                ) : (
                  <div className="space-y-1.5">
                    {skill.team_bindings.map((team) => (
                      <div className="flex items-center gap-2 rounded-lg border border-line bg-card-inner px-3 py-1.5 text-[12px]" key={team.team_id}>
                        <span className="font-semibold text-ink">{team.team_name?.trim() || missingObjectLabel("team", team.team_id)}</span>
                    <span className="ml-auto font-mono text-[11px] text-ink-3" title={team.team_id}>{shortId(team.team_id)}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
              <div>
                <p className="mb-1.5 flex items-center gap-1.5 text-[12px] font-semibold text-ink-2">
                  <Bot className="size-3.5 text-ink-3" />数字员工（{skill.agent_bindings.length}）
                </p>
                {skill.agent_bindings.length === 0 ? (
                  <p className="rounded-lg border border-dashed border-line-strong bg-card-inner px-3 py-2 text-[12px] text-ink-3">未绑定数字员工</p>
                ) : (
                  <div className="space-y-1.5">
                    {skill.agent_bindings.map((agent) => (
                      <div className="flex items-center gap-2 rounded-lg border border-line bg-card-inner px-3 py-1.5 text-[12px]" key={agent.agent_id}>
                        <span className="font-semibold text-ink">{agent.agent_name?.trim() || missingObjectLabel("employee", agent.agent_id)}</span>
                    {agent.team_name?.trim() ? <span className="text-ink-3">· {agent.team_name.trim()}</span> : null}
                    <span className="ml-auto font-mono text-[11px] text-ink-3" title={agent.agent_id}>{shortId(agent.agent_id)}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </section>

        </div>
        <div className="flex items-center justify-between gap-3 border-t border-line p-4">
          <p className="text-[12px] text-ink-3">删除会同时解除全部绑定并清除归档文件。</p>
          <Button
            aria-label={`删除技能 ${skill.name}`}
            onClick={() => onDeleteSkill(skill.id)}
            size="sm"
            type="button"
            variant="danger"
          >
            <Trash2 data-icon="inline-start" />
            删除技能
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  );
}

function SkillMarketToolbar({
  dependencyFilter,
  onDependencyFilterChange,
  onQueryChange,
  onRiskFilterChange,
  onScopeFilterChange,
  onStatusFilterChange,
  query,
  riskFilter,
  scopeFilter,
  statusCounts,
  statusFilter
}: {
  dependencyFilter: DependencyFilter;
  onDependencyFilterChange: (value: DependencyFilter) => void;
  onQueryChange: (value: string) => void;
  onRiskFilterChange: (value: RiskFilter) => void;
  onScopeFilterChange: (value: ScopeFilter) => void;
  onStatusFilterChange: (value: StatusFilter) => void;
  query: string;
  riskFilter: RiskFilter;
  scopeFilter: ScopeFilter;
  statusCounts: { all: number; installed: number; available: number; approval: number };
  statusFilter: StatusFilter;
}) {
  return (
    <ListToolbar
      className="border-b border-line px-3 py-3"
      search={
        <ToolbarSearch
          aria-label="搜索技能"
          onChange={(event) => onQueryChange(event.target.value)}
          placeholder="搜索技能、标签、运行依赖"
          type="search"
          value={query}
        />
      }
      filters={
        <>
          <Chip
            active={statusFilter === "all"}
            count={statusCounts.all}
            onClick={() => onStatusFilterChange("all")}
            type="button"
          >
            全部
          </Chip>
          <Chip
            active={statusFilter === "installed"}
            count={statusCounts.installed}
            onClick={() => onStatusFilterChange("installed")}
            type="button"
          >
            已绑定
          </Chip>
          <Chip
            active={statusFilter === "available"}
            count={statusCounts.available}
            onClick={() => onStatusFilterChange("available")}
            type="button"
          >
            可绑定
          </Chip>
          <Chip
            active={statusFilter === "approval"}
            count={statusCounts.approval}
            onClick={() => onStatusFilterChange("approval")}
            type="button"
          >
            需审批
          </Chip>
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
        </>
      }
    />
  );
}

function FilterSelect({
  label,
  onValueChange,
  options,
  value
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
        className="w-full min-w-0 rounded-[10px] border-transparent bg-card-soft text-ink shadow-none hover:bg-mute-soft lg:w-[124px]"
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

/** Soft-Flat 目录卡：EntityCard + 事实行 + 卡内动作 */
function SkillMarketGrid({
  hasActiveFilters,
  onDeleteSkill,
  onInstallSkill,
  onOpenDetail,
  onSelectSkill,
  rows,
  selectedSkillId,
  totalUnfiltered,
}: {
  hasActiveFilters: boolean;
  onDeleteSkill: (id: string) => void;
  onInstallSkill: (id: string) => void;
  onOpenDetail: (id: string) => void;
  onSelectSkill: (id: string) => void;
  rows: Skill[];
  selectedSkillId?: string;
  totalUnfiltered: number;
}) {
  if (rows.length === 0) {
    if (hasActiveFilters || totalUnfiltered > 0) {
      return (
        <EmptyNoMatch
          className="py-12"
          title="无匹配技能"
          description="请调整搜索词或筛选条件后再试。"
        />
      );
    }
    return (
      <EmptyNoData
        className="py-12"
        icon={<Blocks />}
        title="还没有技能"
        description="上传技能包后可在此发现、绑定并治理。"
      />
    );
  }

  return (
    <div className="grid gap-3 p-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      {rows.map((skill) => {
        const risk = riskDisplay(skill.risk_level);
        const status = statusDisplay(skill);
        const selected = selectedSkillId === skill.id;
        const depCount = runtimeDependencyCount(skill);
        return (
          <EntityCard
            key={skill.id}
            interactive
            selected={selected}
            data-skill-card={skill.id}
            role="button"
            tabIndex={0}
            aria-label={`选中 ${skill.name}`}
            aria-pressed={selected}
            onClick={() => onSelectSkill(skill.id)}
            onKeyDown={(event) => {
              if (event.key === "Enter" || event.key === " ") {
                event.preventDefault();
                onSelectSkill(skill.id);
              }
            }}
            leading={<SkillIcon skill={skill} />}
            title={skill.name}
            subtitle={`${skill.version} · ${skill.source}`}
            status={<StatusPill tone={status.tone}>{status.label}</StatusPill>}
            facts={[
              {
                key: "desc",
                label: "说明",
                value: skill.description || "暂无描述",
              },
              {
                key: "risk",
                label: "风险",
                value: <StatusPill tone={risk.tone}>{risk.label}</StatusPill>,
              },
              {
                key: "bind",
                label: "绑定",
                value: `团队 ${skill.team_bindings.length} · 员工 ${skill.agent_bindings.length}`,
              },
              ...(depCount > 0
                ? [{ key: "dep", label: "依赖", value: `${depCount} 项` }]
                : []),
            ]}
            actions={
              <div className="flex items-center gap-1">
                <Button
                  aria-label={`删除 ${skill.name}`}
                  className="text-ink-3 hover:text-danger"
                  onClick={(event) => {
                    event.stopPropagation();
                    onDeleteSkill(skill.id);
                  }}
                  size="icon"
                  type="button"
                  variant="ghost"
                >
                  <Trash2 />
                </Button>
              </div>
            }
            footer={
              <div className="flex flex-wrap items-center justify-between gap-2">
                <SkillTagStack tags={skill.tags} className="max-w-[min(100%,14rem)]" />
                <div className="flex gap-2">
                  <Button
                    aria-label={`查看详情 ${skill.name}`}
                    onClick={(event) => {
                      event.stopPropagation();
                      onOpenDetail(skill.id);
                    }}
                    size="sm"
                    type="button"
                    variant="outline"
                  >
                    查看详情
                  </Button>
                  <Button
                    aria-label={`加载 ${skill.name}`}
                    onClick={(event) => {
                      event.stopPropagation();
                      onSelectSkill(skill.id);
                      onInstallSkill(skill.id);
                    }}
                    size="sm"
                    type="button"
                  >
                    加载
                  </Button>
                </div>
              </div>
            }
          />
        );
      })}
    </div>
  );
}


function SkillTagStack({ className, tags }: { className?: string; tags: string[] }) {
  const visibleTags = tags.slice(0, 2);
  const extraCount = Math.max(0, tags.length - visibleTags.length);

  return (
    <div className={cn("flex max-w-[180px] flex-wrap gap-1.5", className)}>
      {visibleTags.map((tag) => (
        <span
          className="rounded-[6px] border border-line-strong bg-mute-soft/60 px-2 py-[1px] text-xs font-medium text-ink-2"
          key={tag}
        >
          {tag}
        </span>
      ))}
      {extraCount > 0 ? (
        <span className="rounded-[6px] border border-line-strong bg-mute-soft/60 px-2 py-[1px] text-xs font-medium text-ink-2">
          +{extraCount}
        </span>
      ) : null}
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

function buildMarketMetrics(skills: Skill[]): MetricDefinition[] {
  const dependencyCount = skills.filter((skill) => runtimeDependencyCount(skill) > 0).length;
  const approvalCount = skills.filter(needsApproval).length;
  return [
    { icon: ClipboardList, label: "技能总数", tone: "brand", value: skills.length },
    {
      icon: CheckCircle2,
      label: "可绑定",
      tone: "ok",
      value: skills.filter((skill) => statusDisplay(skill).value === "available").length
},
    {
      icon: ServerCog,
      label: "有运行依赖",
      tone: "info",
      value: dependencyCount,
      loud: dependencyCount > 0
},
    {
      icon: UserRoundCheck,
      label: "需审批",
      tone: "danger",
      value: approvalCount
},
    { icon: Blocks, label: "团队绑定", tone: "info", value: countTeamBindings(skills) },
    { icon: Bot, label: "数字员工绑定", tone: "artifact", value: countAgentBindings(skills) },
  ];
}

function countByStatus(skills: Skill[]): { all: number; installed: number; available: number; approval: number } {
  return {
    all: skills.length,
    installed: skills.filter((skill) => statusDisplay(skill).value === "installed").length,
    available: skills.filter((skill) => statusDisplay(skill).value === "available").length,
    approval: skills.filter((skill) => statusDisplay(skill).value === "approval").length
};
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
  if (skill.team_bindings.length > 0 || skill.agent_bindings.length > 0) {
    return { label: "已绑定", tone: "ok", value: "installed" };
  }
  return { label: "可绑定", tone: "info", value: "available" };
}

function riskDisplay(riskLevel: string): { label: string; tone: Tone } {
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

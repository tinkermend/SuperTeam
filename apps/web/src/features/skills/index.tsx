import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  Blocks,
  Bot,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  ClipboardList,
  FileText,
  ServerCog,
  ShieldCheck,
  Stethoscope,
  Trash2,
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
  V3Chip,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3ToolbarSearch,
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
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import { deleteSkill, listSkillInstallations, listSkills, type Skill, type SkillInstallation } from "@/lib/api/skills";
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

type SkillMarketStatus = "installed" | "available" | "approval";

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

/** 左侧 accent bar 实色（按 tone）。 */
const toneAccentBar: Record<V3Tone, string> = {
  brand: "bg-v3-brand",
  info: "bg-v3-info",
  ok: "bg-v3-ok",
  warn: "bg-v3-warn",
  danger: "bg-v3-danger",
  mute: "bg-v3-mute",
  artifact: "bg-v3-artifact",
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
    queryFn: () => listSkills(apiOptions, { q: query }),
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
    },
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
      <ShellPageHeader
        icon={<Blocks />}
        iconTone="artifact"
        title="技能市场"
        subtitle="发现、查看并治理技能档案与绑定范围"
      />
      <Main width="wide" className="min-w-0 overflow-x-hidden">
        <div className="flex min-w-0 flex-col gap-5">
          <div className="flex flex-wrap items-center justify-start gap-2 sm:justify-end">
            <V3Button asChild className="h-11 self-start px-5">
              <Link to="/skills/upload">
                <UploadCloud data-icon="inline-start" />
                上传技能
              </Link>
            </V3Button>
          </div>

          {/* 顶部 pill 状态卡：左侧 accent bar + 图标 + 大数字 + 标签，紧凑横排 */}
          <section
            className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-6"
            aria-label="技能市场指标"
          >
            {metrics.map((metric) => (
              <SkillMarketPillStat key={metric.label} metric={metric} />
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
              query={query}
              riskFilter={riskFilter}
              scopeFilter={scopeFilter}
              statusCounts={statusCounts}
              statusFilter={statusFilter}
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
                <SkillMarketGrid
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
                />

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
      <SkillDetailSheet
        errorMessage={skillInstallations.error instanceof Error ? skillInstallations.error.message : undefined}
        installations={skillInstallations.data ?? []}
        isError={skillInstallations.isError}
        isLoading={skillInstallations.isPending}
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
            {deleteError ? <p className="font-semibold text-v3-danger">{deleteError}</p> : null}
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

/** 紧凑 pill 状态卡：左侧 3px accent bar + 图标芯片 + 大数字 + 标签。 */
function SkillMarketPillStat({ metric }: { metric: MetricDefinition }) {
  const Icon = metric.icon;
  return (
    <SoftCard
      className={cn(
        "relative flex items-center gap-3 overflow-hidden p-3.5",
        metric.loud && "bg-gradient-to-b from-v3-warn-soft/50 to-v3-card",
      )}
    >
      <span aria-hidden className={cn("absolute inset-y-0 left-0 w-[3px]", toneAccentBar[metric.tone])} />
      <IconTile tone={metric.loud ? "warn" : metric.tone} size="sm">
        <Icon />
      </IconTile>
      <div className="min-w-0">
        <p
          className={cn(
            "text-[20px] font-extrabold leading-none tracking-tight tabular-nums",
            metric.loud ? "text-v3-warn" : "text-v3-ink",
          )}
        >
          {metric.value}
        </p>
        <p className="mt-1 text-[11.5px] font-semibold text-v3-ink-3">{metric.label}</p>
      </div>
    </SoftCard>
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
    <div className="grid min-w-0 gap-3 px-4 py-3 text-[13px] md:grid-cols-[minmax(160px,1.1fr)_minmax(150px,0.9fr)_minmax(220px,1.4fr)] md:items-start">
      <div className="min-w-0">
        <div className="flex min-w-0 items-center gap-2">
          <span className="truncate font-bold text-v3-ink" title={installationTargetLabel(installation)}>{installationTargetLabel(installation)}</span>
          <StatusPill tone={providerTone(installation.provider_type)}>{installation.provider_type}</StatusPill>
        </div>
        {installation.digital_employee_id && installation.employee_name ? (
          <div className="mt-1 break-all font-mono text-[11px] leading-relaxed text-v3-ink-3">
            {installation.digital_employee_id}
          </div>
        ) : null}
      </div>

      <div className="min-w-0 space-y-1">
        <div className="break-all font-mono text-xs leading-relaxed text-v3-ink-2">{runtimeNodeLabel(installation)}</div>
        {installation.installed_at ? (
          <div className="truncate font-mono text-[11px] text-v3-ink-3">{installation.installed_at}</div>
        ) : null}
      </div>

      <div className="min-w-0 space-y-1">
        <div className="break-all font-mono text-xs leading-relaxed text-v3-ink-2">
          {installation.installed_path}
        </div>
        {installation.archive_checksum_sha256 ? (
          <div className="break-all font-mono text-[11px] leading-relaxed text-v3-ink-3">
            {installation.archive_checksum_sha256}
          </div>
        ) : null}
      </div>
    </div>
  );
}

/** 技能详情抽屉：点击「查看详情」从右侧滑出，展示完整信息，不跳转页面。 */
function SkillDetailSheet({
  errorMessage,
  installations,
  isError,
  isLoading,
  onDeleteSkill,
  onOpenChange,
  open,
  skill,
}: {
  errorMessage?: string;
  installations: SkillInstallation[];
  isError: boolean;
  isLoading: boolean;
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
        <SheetHeader className="flex-row items-center gap-3 border-b border-v3-line p-4 pr-12">
          <SkillIcon skill={skill} size="sm" />
          <div className="min-w-0">
            <SheetTitle className="truncate text-base font-bold text-v3-ink">{skill.name}</SheetTitle>
            <SheetDescription className="mt-0.5 truncate font-mono text-[11px] text-v3-ink-3">
              {skill.version} · {skill.source}
            </SheetDescription>
          </div>
        </SheetHeader>
        <div className="flex-1 space-y-5 overflow-y-auto p-4">
          <section>
            <h3 className="mb-2.5 text-[11px] font-bold tracking-wide text-v3-ink-3 uppercase">基础信息</h3>
            <div className="grid grid-cols-2 gap-x-4 gap-y-2.5">
              <div>
                <p className="text-[11px] font-semibold text-v3-ink-3">风险等级</p>
                <p className="mt-0.5"><StatusPill tone={risk.tone}>{risk.label}</StatusPill></p>
              </div>
              <div>
                <p className="text-[11px] font-semibold text-v3-ink-3">绑定状态</p>
                <p className="mt-0.5"><StatusPill tone={status.tone}>{status.label}</StatusPill></p>
              </div>
              <div>
                <p className="text-[11px] font-semibold text-v3-ink-3">版本</p>
                <p className="mt-0.5 font-mono text-[13px] text-v3-ink">{skill.version}</p>
              </div>
              <div>
                <p className="text-[11px] font-semibold text-v3-ink-3">来源</p>
                <p className="mt-0.5 truncate font-mono text-[13px] text-v3-ink">{skill.source}</p>
              </div>
              <div>
                <p className="text-[11px] font-semibold text-v3-ink-3">团队绑定</p>
                <p className="mt-0.5 text-[13px] font-semibold text-v3-ink">{skill.team_bindings.length} 个</p>
              </div>
              <div>
                <p className="text-[11px] font-semibold text-v3-ink-3">数字员工绑定</p>
                <p className="mt-0.5 text-[13px] font-semibold text-v3-ink">{skill.agent_bindings.length} 个</p>
              </div>
              <div>
                <p className="text-[11px] font-semibold text-v3-ink-3">安装目标</p>
                <p className="mt-0.5 text-[13px] font-semibold text-v3-ink">{installations.length} 个</p>
              </div>
              <div>
                <p className="text-[11px] font-semibold text-v3-ink-3">运行依赖</p>
                <p className="mt-0.5 text-[13px] font-semibold text-v3-ink">{depCount} 项</p>
              </div>
              <div>
                <p className="text-[11px] font-semibold text-v3-ink-3">创建人</p>
                <p className="mt-0.5 text-[13px] text-v3-ink">{skill.created_by_name || "—"}</p>
              </div>
              <div>
                <p className="text-[11px] font-semibold text-v3-ink-3">创建时间</p>
                <p className="mt-0.5 font-mono text-[12px] text-v3-ink-2">{skill.created_at ?? "—"}</p>
              </div>
            </div>
          </section>

          <section>
            <h3 className="mb-2.5 text-[11px] font-bold tracking-wide text-v3-ink-3 uppercase">运行依赖</h3>
            {depCount === 0 ? (
              <p className="text-[13px] text-v3-ink-3">无运行依赖</p>
            ) : (
              <div className="flex flex-wrap gap-1.5">
                {depTools.map((tool) => (
                  <span className="rounded-md bg-v3-info-soft px-2 py-1 font-mono text-[11px] font-semibold text-v3-info-text" key={`tool-${tool}`}>tool: {tool}</span>
                ))}
                {depEnv.map((env) => (
                  <span className="rounded-md bg-v3-artifact-soft px-2 py-1 font-mono text-[11px] font-semibold text-v3-artifact-text" key={`env-${env}`}>env: {env}</span>
                ))}
              </div>
            )}
          </section>

          <section>
            <h3 className="mb-2.5 text-[11px] font-bold tracking-wide text-v3-ink-3 uppercase">绑定范围</h3>
            <div className="space-y-3">
              <div>
                <p className="mb-1.5 flex items-center gap-1.5 text-[12px] font-semibold text-v3-ink-2">
                  <Users className="size-3.5 text-v3-ink-3" />团队（{skill.team_bindings.length}）
                </p>
                {skill.team_bindings.length === 0 ? (
                  <p className="rounded-lg border border-dashed border-v3-line-strong bg-v3-card-inner px-3 py-2 text-[12px] text-v3-ink-3">未绑定团队</p>
                ) : (
                  <div className="space-y-1.5">
                    {skill.team_bindings.map((team) => (
                      <div className="flex items-center gap-2 rounded-lg border border-v3-line bg-v3-card-inner px-3 py-1.5 text-[12px]" key={team.team_id}>
                        <span className="font-semibold text-v3-ink">{team.team_name}</span>
                        <span className="ml-auto font-mono text-[11px] text-v3-ink-3">{team.team_id}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
              <div>
                <p className="mb-1.5 flex items-center gap-1.5 text-[12px] font-semibold text-v3-ink-2">
                  <Bot className="size-3.5 text-v3-ink-3" />数字员工（{skill.agent_bindings.length}）
                </p>
                {skill.agent_bindings.length === 0 ? (
                  <p className="rounded-lg border border-dashed border-v3-line-strong bg-v3-card-inner px-3 py-2 text-[12px] text-v3-ink-3">未绑定数字员工</p>
                ) : (
                  <div className="space-y-1.5">
                    {skill.agent_bindings.map((agent) => (
                      <div className="flex items-center gap-2 rounded-lg border border-v3-line bg-v3-card-inner px-3 py-1.5 text-[12px]" key={agent.agent_id}>
                        <span className="font-semibold text-v3-ink">{agent.agent_name}</span>
                        {agent.team_name ? <span className="text-v3-ink-3">· {agent.team_name}</span> : null}
                        <span className="ml-auto font-mono text-[11px] text-v3-ink-3">{agent.agent_id}</span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            </div>
          </section>

          <section>
            <h3 className="mb-2.5 text-[11px] font-bold tracking-wide text-v3-ink-3 uppercase">安装记录 · {installations.length} 个目标</h3>
            {isLoading ? (
              <V3LoadingState className="py-6" label="加载安装记录…" />
            ) : isError ? (
              <V3ErrorState description={errorMessage ?? "请检查技能安装记录接口。"} title="安装记录加载失败" />
            ) : installations.length === 0 ? (
              <V3EmptyState className="py-6" description="成功安装到运行节点后会显示物理安装事实。" title="暂无安装记录" />
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
          </section>
        </div>
        <div className="flex items-center justify-between gap-3 border-t border-v3-line p-4">
          <p className="text-[12px] text-v3-ink-3">删除会同时解除全部绑定并清除归档文件。</p>
          <V3Button
            aria-label={`删除技能 ${skill.name}`}
            onClick={() => onDeleteSkill(skill.id)}
            size="sm"
            type="button"
            variant="danger"
          >
            <Trash2 data-icon="inline-start" />
            删除技能
          </V3Button>
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
  statusFilter,
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
    <div className="flex min-w-0 flex-col gap-3 border-b border-v3-line p-3 lg:flex-row lg:items-center lg:gap-3">
      <V3ToolbarSearch
        aria-label="搜索技能"
        onChange={(event) => onQueryChange(event.target.value)}
        placeholder="搜索技能、标签、运行依赖"
        type="search"
        value={query}
      />
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <V3Chip
          active={statusFilter === "all"}
          count={statusCounts.all}
          onClick={() => onStatusFilterChange("all")}
          type="button"
        >
          全部
        </V3Chip>
        <V3Chip
          active={statusFilter === "installed"}
          count={statusCounts.installed}
          onClick={() => onStatusFilterChange("installed")}
          type="button"
        >
          已绑定
        </V3Chip>
        <V3Chip
          active={statusFilter === "available"}
          count={statusCounts.available}
          onClick={() => onStatusFilterChange("available")}
          type="button"
        >
          可绑定
        </V3Chip>
        <V3Chip
          active={statusFilter === "approval"}
          count={statusCounts.approval}
          onClick={() => onStatusFilterChange("approval")}
          type="button"
        >
          需审批
        </V3Chip>
      </div>
      <div className="grid min-w-0 grid-cols-2 gap-2 sm:grid-cols-3 lg:flex lg:items-center">
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
      <SelectTrigger
        aria-label={label}
        className="w-full min-w-0 rounded-[10px] border-transparent bg-v3-card-soft text-v3-ink shadow-none hover:bg-v3-mute-soft lg:w-[124px]"
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

/** 方向 B · 能力矩阵卡片：左侧状态 accent bar + 图标 + 名称 + 描述 + pill + 标签 + 绑定数 + 操作。 */
function SkillMarketGrid({
  onDeleteSkill,
  onInstallSkill,
  onOpenDetail,
  onSelectSkill,
  rows,
  selectedSkillId,
}: {
  onDeleteSkill: (id: string) => void;
  onInstallSkill: (id: string) => void;
  onOpenDetail: (id: string) => void;
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
    <div className="grid gap-3 p-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
      {rows.map((skill) => {
        const risk = riskDisplay(skill.risk_level);
        const status = statusDisplay(skill);
        const selected = selectedSkillId === skill.id;
        const depCount = runtimeDependencyCount(skill);
        return (
          <SoftCard
            aria-label={`选中 ${skill.name}`}
            className={cn(
              "relative flex min-w-0 flex-col gap-3 overflow-hidden p-4",
              selected && "border-v3-brand ring-1 ring-v3-brand",
            )}
            interactive
            key={skill.id}
            onClick={() => onSelectSkill(skill.id)}
            role="button"
            tabIndex={0}
          >
            <span aria-hidden className={cn("absolute inset-y-0 left-0 w-[3px]", toneAccentBar[status.tone])} />
            <div className="flex min-w-0 items-start gap-3">
              <SkillIcon skill={skill} />
              <div className="min-w-0 flex-1">
                <p className="truncate font-bold text-v3-ink">{skill.name}</p>
                <p className="mt-0.5 truncate font-mono text-[11px] text-v3-ink-3">
                  {skill.version} · {skill.source}
                </p>
              </div>
            </div>
            <p className="line-clamp-2 min-h-[38px] text-[13px] leading-5 text-v3-ink-2">
              {skill.description}
            </p>
            <div className="flex flex-wrap gap-1.5">
              <StatusPill tone={risk.tone}>{risk.label}</StatusPill>
              <StatusPill tone={status.tone}>{status.label}</StatusPill>
              {depCount > 0 ? (
                <StatusPill tone="artifact" showDot={false}>{depCount} 依赖</StatusPill>
              ) : null}
            </div>
            <SkillTagStack tags={skill.tags} className="max-w-none" />
            <div className="mt-auto flex items-center justify-between gap-3 border-t border-v3-line pt-3 text-[13px] text-v3-ink-2">
              <div className="flex items-center gap-3">
                <span className="inline-flex items-center gap-1">
                  <Users className="size-3.5 text-v3-ink-3" />
                  <span className="font-bold tabular-nums text-v3-ink">{skill.team_bindings.length}</span>
                </span>
                <span className="inline-flex items-center gap-1">
                  <Bot className="size-3.5 text-v3-ink-3" />
                  <span className="font-bold tabular-nums text-v3-ink">{skill.agent_bindings.length}</span>
                </span>
              </div>
              <div className="flex gap-2">
                <V3Button
                  aria-label={`删除 ${skill.name}`}
                  className="text-v3-ink-3 hover:text-v3-danger"
                  onClick={(event) => {
                    event.stopPropagation();
                    onDeleteSkill(skill.id);
                  }}
                  size="icon"
                  type="button"
                  variant="ghost"
                >
                  <Trash2 />
                </V3Button>
                <V3Button
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

function SkillTagStack({ className, tags }: { className?: string; tags: string[] }) {
  const visibleTags = tags.slice(0, 2);
  const extraCount = Math.max(0, tags.length - visibleTags.length);

  return (
    <div className={cn("flex max-w-[180px] flex-wrap gap-1.5", className)}>
      {visibleTags.map((tag) => (
        <span
          className="rounded-[6px] border border-v3-line-strong bg-v3-mute-soft/60 px-2 py-[1px] text-xs font-medium text-v3-ink-2"
          key={tag}
        >
          {tag}
        </span>
      ))}
      {extraCount > 0 ? (
        <span className="rounded-[6px] border border-v3-line-strong bg-v3-mute-soft/60 px-2 py-[1px] text-xs font-medium text-v3-ink-2">
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
      label: "可绑定",
      tone: "ok",
      value: skills.filter((skill) => statusDisplay(skill).value === "available").length,
    },
    {
      icon: ServerCog,
      label: "有运行依赖",
      tone: "info",
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

function countByStatus(skills: Skill[]): { all: number; installed: number; available: number; approval: number } {
  return {
    all: skills.length,
    installed: skills.filter((skill) => statusDisplay(skill).value === "installed").length,
    available: skills.filter((skill) => statusDisplay(skill).value === "available").length,
    approval: skills.filter((skill) => statusDisplay(skill).value === "approval").length,
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

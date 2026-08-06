import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Blocks,
  Bot,
  CalendarClock,
  FileArchive,
  Fingerprint,
  Network,
  PackageCheck,
  ServerCog,
  ShieldCheck,
  Trash2,
  UserRound,
  Users,
  FolderKanban,
  Wrench
} from "lucide-react";
import {
  IconTile,
  MasterDetailLayout,
  SoftCard,
  StatusPill,
  Button,
  EmptyState,
  ErrorState,
  LoadingState,
  MetricCard,
  WorkSurface,
  type Tone
} from "@/components/superteam";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue
} from "@/components/ui/select";
import { Main } from "@/components/layout/main";
import {
  ShellPageHeader,
  ShellPageHeaderBack
} from "@/components/layout/shell-page-header";
import { listMcpServerDefinitions } from "@/lib/api/capabilities";
import {
  getSkill,
  listSkillMcpDependencies,
  replaceSkillMcpDependencies,
  type Skill
} from "@/lib/api/skills";

type SkillDetailViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  skillId: string;
};

const riskTone: Record<string, Tone> = {
  critical: "danger",
  high: "danger",
  low: "ok",
  medium: "warn"
};

const riskLabel: Record<string, string> = {
  critical: "高风险",
  high: "高风险",
  low: "低风险",
  medium: "中风险"
};

export function SkillDetailView({ apiBaseUrl, fetcher, skillId }: SkillDetailViewProps) {
  const skill = useQuery({
    queryKey: ["skill", skillId],
    queryFn: () => getSkill({ baseUrl: apiBaseUrl, fetcher }, skillId)
});
  const errorMessage = skill.error instanceof Error ? skill.error.message : undefined;

  return (
    <>
      <ShellPageHeader
        back={<ShellPageHeaderBack ariaLabel="返回技能市场" to="/skills" />}
        title={skill.data?.name ?? "技能档案"}
        subtitle={
          skill.data ? (
            <span className="inline-flex flex-wrap items-center gap-2">
              <span>技能档案</span>
              <span className="text-ink-3">/</span>
              <span className="font-mono">{skill.data.slug}</span>
            </span>
          ) : (
            "加载技能详情"
          )
        }
      />
      <Main width="wide" className="min-w-0 overflow-x-hidden">
        {skill.data ? (
          <div className="mb-4 flex flex-wrap items-center justify-start gap-2 sm:justify-end">
            <Button disabled className="h-10 px-4" type="button">
              安装到...
            </Button>
            <StatusPill tone="mute">即将支持</StatusPill>
          </div>
        ) : null}
        {skill.isPending ? (
          <WorkSurface>
            <LoadingState label="加载技能档案…" />
          </WorkSurface>
        ) : skill.isError || !skill.data ? (
          <WorkSurface className="p-4">
            <ErrorState
              title="技能档案加载失败"
              description={errorMessage ?? "请检查技能详情接口和当前权限。"}
            />
          </WorkSurface>
        ) : (
          <SkillArchiveDetail apiBaseUrl={apiBaseUrl} fetcher={fetcher} skill={skill.data} skillId={skillId} />
        )}
      </Main>
    </>
  );
}

function SkillArchiveDetail({
  apiBaseUrl,
  fetcher,
  skill,
  skillId
}: {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  skill: Skill;
  skillId: string;
}) {
  const risk = normalizeRisk(skill.risk_level);
  const dependencyCount =
    (skill.runtime_dependencies?.tools?.length ?? 0) + (skill.runtime_dependencies?.env?.length ?? 0);

  return (
    <div className="flex min-w-0 flex-col gap-5">
      <SoftCard className="p-5">
        <div
          className="grid min-w-0 gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(280px,360px)] xl:grid-cols-[minmax(0,1fr)_minmax(300px,0.58fr)_minmax(320px,0.5fr)] xl:items-start"
          data-testid="skill-detail-hero-layout"
        >
          <div className="flex min-w-0 items-start gap-4" data-testid="skill-detail-identity">
            <IconTile tone={toneFromColor(skill.color_token)} size="lg">
              <Blocks />
            </IconTile>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <p className="text-xl font-extrabold text-ink">{skill.name}</p>
                <StatusPill tone={riskTone[risk]}>{riskLabel[risk]}</StatusPill>
                <StatusPill tone={dependencyCount > 0 ? "warn" : "ok"}>
                  {dependencyCount > 0 ? "有运行依赖" : "无运行依赖"}
                </StatusPill>
              </div>
              <p className="mt-2 max-w-4xl text-[13px] leading-6 text-ink-2">
                {skill.description}
              </p>
              <SkillTagStrip tags={skill.tags} />
            </div>
          </div>
          <div
            className="min-w-0 rounded-inner border border-line bg-card-inner p-4"
            data-testid="skill-detail-declaration"
          >
            <div className="mb-3 flex items-center gap-2">
              <ShieldCheck className="size-4 text-warn" />
              <span className="text-sm font-extrabold text-ink">创建者声明</span>
            </div>
            <div className="flex flex-wrap gap-2">
              <StatusPill tone={riskTone[risk]}>{riskLabel[risk]}</StatusPill>
              <StatusPill tone="mute">风险等级来自上传表单</StatusPill>
            </div>
            <div className="mt-4 rounded-[14px] bg-card px-3 py-2.5">
              <p className="text-xs font-bold text-ink-3">风险说明</p>
              <p className="mt-1 text-sm font-semibold text-ink">未记录单独风险说明</p>
            </div>
          </div>
          <div className="grid min-w-0 grid-cols-2 gap-2 text-[13px] sm:grid-cols-4 lg:grid-cols-2">
            <MiniField label="版本" value={skill.version} />
            <MiniField label="来源" value={sourceLabel(skill.source)} />
            <MiniField label="创建人" value={skill.created_by_name || skill.created_by} />
            <MiniField label="最近更新" value={formatDateTime(skill.updated_at)} />
          </div>
        </div>
      </SoftCard>

      <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-4" aria-label="技能档案指标">
        <MetricCard
          icon={<PackageCheck />}
          iconTone="brand"
          label="安装目标"
          meta="团队与数字员工绑定总数"
          value={skill.team_bindings.length + skill.agent_bindings.length}
        />
        <MetricCard icon={<Users />} iconTone="info" label="团队安装" value={skill.team_bindings.length} />
        <MetricCard icon={<Bot />} iconTone="artifact" label="数字员工安装" value={skill.agent_bindings.length} />
        <MetricCard
          icon={<FolderKanban />}
          iconTone="brand"
          label="项目绑定"
          value={(skill.project_bindings ?? []).length}
        />
        <MetricCard icon={<Wrench />} iconTone="warn" label="运行要求" value={dependencyCount} />
      </section>

      <MasterDetailLayout
        narrowDetail="stack"
        rail="lg"
        master={
          <div className="flex min-w-0 flex-col gap-5">
            <DetailSection
              action={
                <div className="flex items-center gap-2">
                  <Button disabled size="sm" type="button">
                    安装到...
                  </Button>
                  <StatusPill tone="mute">即将支持</StatusPill>
                </div>
              }
              icon={<Users />}
              title="安装范围"
            >
              <div className="grid gap-4 lg:grid-cols-2">
                <BindingList
                  empty="暂无团队安装"
                  icon={<Users />}
                  items={skill.team_bindings.map((binding) => ({
                    id: binding.team_id,
                    meta: "团队安装",
                    name: binding.team_name || binding.team_id,
                    tone: "info"
}))}
                  title="团队安装"
                />
                <BindingList
                  empty="暂无数字员工安装"
                  icon={<Bot />}
                  items={skill.agent_bindings.map((binding) => ({
                    id: binding.agent_id,
                    meta: [binding.team_name || binding.team_id, binding.status].filter(Boolean).join(" / ") || binding.status,
                    name: binding.agent_name || binding.agent_id,
                    tone: "artifact"
}))}
                  title="数字员工安装"
                />
                <BindingList
                  empty="暂无项目绑定"
                  icon={<FolderKanban />}
                  items={(skill.project_bindings ?? []).map((binding) => ({
                    id: binding.project_id,
                    meta: binding.project_id,
                    name:
                      (binding.project_name || "").trim() ||
                      (binding.project_id
                        ? `未命名项目 (${binding.project_id.slice(0, 8)})`
                        : "未命名项目"),
                    tone: "brand"
                  }))}
                  title="项目绑定"
                />
              </div>
            </DetailSection>

            <SkillMcpDependenciesSection apiBaseUrl={apiBaseUrl} fetcher={fetcher} skillId={skillId} />

            <DetailSection icon={<ServerCog />} title="运行要求">
              <div className="mb-4 rounded-inner bg-card-inner p-3 text-sm text-ink-2">
                当前仅展示创建者声明的运行要求，不做本地检测或依赖验证。
              </div>
              <DependencyGroup label="CLI 工具" items={skill.runtime_dependencies?.tools ?? []} />
              <DependencyGroup label="环境变量" items={skill.runtime_dependencies?.env ?? []} />
            </DetailSection>
          </div>
        }
        detail={
          <div className="flex min-w-0 flex-col gap-5">
            <DetailSection icon={<PackageCheck />} title="上传与存储信息">
              <div
                className="overflow-hidden rounded-inner border border-line bg-card-inner"
                data-testid="skill-upload-metadata"
              >
                <DataRow label="Slug" value={skill.slug} mono />
                <DataRow label="归档包" value={skill.archive_filename} />
                <DataRow label="对象引用" value={skill.archive_object_ref} mono />
                <DataRow label="文件大小" value={formatBytes(skill.archive_size_bytes)} />
                <DataRow label="文件数量" value={`${skill.archive_file_count} 个文件`} />
                <DataRow label="SHA256" value={skill.archive_checksum_sha256} mono wrap />
                <DataRow label="创建人" value={skill.created_by_name || skill.created_by} />
                <DataRow label="创建时间" value={formatDateTime(skill.created_at)} />
                <DataRow label="更新时间" value={formatDateTime(skill.updated_at)} />
              </div>
            </DetailSection>
          </div>
        }
      />
    </div>
  );
}

function SkillMcpDependenciesSection({
  apiBaseUrl,
  fetcher,
  skillId
}: {
  apiBaseUrl: string;
  fetcher?: typeof globalThis.fetch;
  skillId: string;
}) {
  const queryClient = useQueryClient();
  const options = { baseUrl: apiBaseUrl, fetcher };
  const dependencies = useQuery({
    queryKey: ["skill-mcp-dependencies", skillId],
    queryFn: () => listSkillMcpDependencies(options, skillId)
});
  const registry = useQuery({
    queryKey: ["mcp-server-definitions"],
    queryFn: () => listMcpServerDefinitions(options)
});
  const [selectedServerId, setSelectedServerId] = useState("");
  const replaceMutation = useMutation({
    mutationFn: (items: Array<{ mcp_server_id: string }>) =>
      replaceSkillMcpDependencies(options, skillId, { items }),
    onSuccess: () => {
      setSelectedServerId("");
      void queryClient.invalidateQueries({ queryKey: ["skill-mcp-dependencies", skillId] });
    }
});

  const current = dependencies.data ?? [];
  const candidates = (registry.data ?? []).filter(
    (definition) => !current.some((dep) => dep.mcp_server_id === definition.id),
  );

  return (
    <DetailSection
      icon={<Network />}
      title="依赖 MCP"
      action={
        <div className="flex items-center gap-2">
          <Select value={selectedServerId} onValueChange={setSelectedServerId}>
            <SelectTrigger aria-label="从注册表选择 MCP" className="w-52" size="sm">
              <SelectValue placeholder="从注册表选择..." />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {candidates.map((definition) => (
                  <SelectItem key={definition.id} value={definition.id}>
                    {definition.name}（{definition.server_key}）
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <Button
            size="sm"
            disabled={!selectedServerId || replaceMutation.isPending}
            onClick={() =>
              replaceMutation.mutate([
                ...current.map((dep) => ({ mcp_server_id: dep.mcp_server_id })),
                { mcp_server_id: selectedServerId },
              ])
            }
          >
            添加依赖
          </Button>
        </div>
      }
    >
      {current.length === 0 ? (
        <EmptyState
          title="未声明依赖"
          description="依赖分两种情况：所依赖的 MCP 已在注册表中、且执行者已配齐它要求的环境变量时，派发会自动把它一并投影（记为 dependency_closure，可在派发记录中追溯）；若环境变量未配齐，派发会被阻断等待人工处理——依赖永远不会替员工开通缺失的凭据。"
        />
      ) : (
        <>
          <ul className="flex flex-col gap-2">
            {current.map((dep) => (
              <li
                className="flex items-center justify-between gap-3 rounded-inner border border-line bg-card-inner px-3 py-2"
                key={dep.id}
              >
                <div className="min-w-0">
                  <p className="truncate text-sm font-bold text-ink">{dep.server_name}</p>
                  <p className="truncate font-mono text-xs text-ink-3">
                    {dep.server_key} · {dep.auth_strategy} · {dep.risk_level}
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  <StatusPill tone={dep.server_status === "active" ? "ok" : "warn"}>
                    {dep.server_status}
                  </StatusPill>
                  <Button
                    aria-label={`移除依赖 ${dep.server_key}`}
                    variant="ghost"
                    size="sm"
                    disabled={replaceMutation.isPending}
                    onClick={() =>
                      replaceMutation.mutate(
                        current
                          .filter((item) => item.id !== dep.id)
                          .map((item) => ({ mcp_server_id: item.mcp_server_id })),
                      )
                    }
                  >
                    <Trash2 />
                  </Button>
                </div>
              </li>
            ))}
          </ul>
          <p className="mt-3 text-xs text-ink-3">
            依赖分两种情况：所依赖的 MCP 已在注册表中、且执行者已配齐它要求的环境变量时，派发会自动把它一并投影（记为 dependency_closure，可在派发记录中追溯）；若环境变量未配齐，派发会被阻断等待人工处理——依赖永远不会替员工开通缺失的凭据。
          </p>
        </>
      )}
    </DetailSection>
  );
}

function SkillTagStrip({ tags }: { tags: string[] }) {
  if (tags.length === 0) return null;

  return (
    <div className="mt-3 flex flex-wrap gap-2" data-testid="skill-detail-tags">
      {tags.map((tag) => (
        <span className="rounded-lg bg-mute-soft px-2.5 py-1 text-xs font-semibold text-ink-2" key={tag}>
          {tag}
        </span>
      ))}
    </div>
  );
}

function DetailSection({
  action,
  children,
  icon,
  title
}: {
  action?: React.ReactNode;
  children: React.ReactNode;
  icon: React.ReactNode;
  title: string;
}) {
  return (
    <WorkSurface>
      <div className="flex flex-col gap-3 border-b border-line p-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex items-center gap-3">
          <IconTile tone="brand" size="sm">
            {icon}
          </IconTile>
          <h3 className="text-base font-extrabold text-ink">{title}</h3>
        </div>
        {action ? <div className="shrink-0">{action}</div> : null}
      </div>
      <div className="p-4">{children}</div>
    </WorkSurface>
  );
}

function BindingList({
  empty,
  icon,
  items,
  title
}: {
  empty: string;
  icon: React.ReactNode;
  items: Array<{ id: string; meta: string; name: string; tone: Tone }>;
  title: string;
}) {
  const countText = `${items.length} 个`;

  if (items.length === 0) {
    return (
      <div className="min-w-0 rounded-inner border border-line bg-card-inner">
        <BindingListHeader count={countText} icon={icon} title={title} />
        <EmptyState className="py-10" title={empty} description="安装能力暂未开放，当前仅展示已有绑定数据。" />
      </div>
    );
  }

  return (
    <div className="min-w-0 rounded-inner border border-line bg-card-inner">
      <BindingListHeader count={countText} icon={icon} title={title} />
      <div className="grid gap-3 p-3">
        {items.map((item) => (
          <div
            className="flex min-w-0 items-center justify-between gap-3 rounded-[14px] border border-line bg-card p-3"
            key={item.id}
          >
            <div className="flex min-w-0 items-center gap-3">
              <IconTile tone={item.tone} size="sm">
                {item.tone === "artifact" ? <Bot /> : <Users />}
              </IconTile>
              <div className="min-w-0">
                <p className="truncate text-sm font-bold text-ink">{item.name}</p>
                <p className="mt-0.5 text-xs text-ink-3">{item.meta}</p>
              </div>
            </div>
            <StatusPill tone={item.tone}>已安装</StatusPill>
          </div>
        ))}
      </div>
    </div>
  );
}

function BindingListHeader({
  count,
  icon,
  title
}: {
  count: string;
  icon: React.ReactNode;
  title: string;
}) {
  return (
    <div className="flex items-center justify-between gap-3 border-b border-line px-3 py-2.5">
      <div className="flex min-w-0 items-center gap-2">
        <span className="text-ink-3 [&_svg]:size-4">{icon}</span>
        <p className="truncate text-sm font-extrabold text-ink">{title}</p>
      </div>
      <span className="text-xs font-bold text-ink-3">{count}</span>
    </div>
  );
}

function DependencyGroup({ items, label }: { items: string[]; label: string }) {
  return (
    <div className="not-last:mb-4">
      <p className="mb-2 text-xs font-bold text-ink-3">{label}</p>
      {items.length > 0 ? (
        <div className="flex flex-wrap gap-2">
          {items.map((item) => (
            <span
              className="rounded-lg bg-warn-soft px-2.5 py-1 font-mono text-xs font-semibold text-warn"
              key={item}
            >
              {item}
            </span>
          ))}
        </div>
      ) : (
        <EmptyText>无声明</EmptyText>
      )}
    </div>
  );
}

function DataRow({
  label,
  mono,
  value,
  wrap
}: {
  label: string;
  mono?: boolean;
  value: string;
  wrap?: boolean;
}) {
  return (
    <div className="grid gap-1 border-b border-line px-3 py-2.5 last:border-b-0 sm:grid-cols-[108px_minmax(0,1fr)] sm:items-center sm:gap-3">
      <span className="inline-flex min-w-0 items-center gap-1.5 text-xs font-bold text-ink-3">
        <DataIcon label={label} />
        {label}
      </span>
      <span
        className={[
          "min-w-0 text-sm font-semibold text-ink",
          mono ? "font-mono text-xs" : "",
          wrap ? "break-all" : "truncate",
        ].join(" ")}
      >
        {value || "未记录"}
      </span>
    </div>
  );
}

function DataIcon({ label }: { label: string }) {
  if (label.includes("时间")) return <CalendarClock className="size-3.5" />;
  if (label.includes("SHA")) return <Fingerprint className="size-3.5" />;
  if (label.includes("创建人")) return <UserRound className="size-3.5" />;
  if (label.includes("风险")) return <ShieldCheck className="size-3.5" />;
  return <FileArchive className="size-3.5" />;
}

function MiniField({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-inner border border-line bg-card-inner px-3 py-2">
      <span className="block text-[11px] font-bold text-ink-3">{label}</span>
      <span className="mt-1 block truncate text-[13px] font-bold text-ink">{value || "未记录"}</span>
    </div>
  );
}

function EmptyText({ children }: { children: React.ReactNode }) {
  return <p className="rounded-inner bg-card-inner p-3 text-sm text-ink-3">{children}</p>;
}

function normalizeRisk(riskLevel: string) {
  const normalized = riskLevel.toLowerCase();
  if (normalized === "high" || normalized === "critical") return "high";
  if (normalized === "medium") return "medium";
  return "low";
}

function toneFromColor(colorToken: string): Tone {
  if (colorToken === "violet") return "artifact";
  if (colorToken === "emerald") return "ok";
  if (colorToken === "cyan" || colorToken === "blue") return "info";
  return "brand";
}

function sourceLabel(source: string) {
  if (source === "upload") return "上传";
  if (source === "system") return "系统内置";
  return source || "未记录";
}

function formatBytes(bytes: number) {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function formatDateTime(value?: string) {
  if (!value) return "未记录";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "2-digit",
    year: "numeric"
}).format(date);
}

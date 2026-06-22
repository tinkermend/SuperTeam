import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Blocks,
  Bot,
  FileArchive,
  Plus,
  Search as SearchIcon,
  ServerCog,
  ShieldCheck,
  Stethoscope,
  Trash2,
  TriangleAlert,
  UploadCloud,
  User,
  type LucideIcon,
} from "lucide-react";
import {
  LiquidCard,
  SemanticIconTile,
  type Tone,
} from "@/components/superteam";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import {
  bindEmployeeSkill,
  bindTeamSkill,
  deleteSkill,
  listSkills,
  unbindEmployeeSkill,
  unbindTeamSkill,
  type Skill,
} from "@/lib/api/skills";
import { listDigitalEmployees, type DigitalEmployee } from "@/lib/api/employees";
import { listTeams } from "@/lib/api/teams";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { cn } from "@/lib/utils";

type SkillsViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

type ApiOpts = { baseUrl: string; fetcher?: typeof fetch };

const iconMap: Record<string, LucideIcon> = {
  blocks: Blocks,
  flask: Blocks,
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
  const queryClient = useQueryClient();
  const [query, setQuery] = useState("");
  const [selectedSkillId, setSelectedSkillId] = useState<string>();
  const [deleteTarget, setDeleteTarget] = useState<Skill | null>(null);

  const apiOptions: ApiOpts = { baseUrl: apiBaseUrl, fetcher };
  const skills = useQuery({
    queryKey: ["skills", query],
    queryFn: () => listSkills(apiOptions, { q: query }),
  });

  const skillRows = skills.data ?? [];
  const selectedSkill = skillRows.find((skill) => skill.id === selectedSkillId) ?? skillRows[0];
  const skillsError = skills.error instanceof Error ? skills.error.message : undefined;

  const deleteMutation = useMutation({
    mutationFn: (skillId: string) => deleteSkill(apiOptions, skillId),
    onSuccess: async () => {
      setDeleteTarget(null);
      await queryClient.invalidateQueries({ queryKey: ["skills"] });
    },
  });

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main className="min-w-0 overflow-x-hidden">
        <div className="flex min-w-0 flex-col gap-5">
          <div className="flex min-w-0 flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
            <div className="flex items-start gap-3">
              <SemanticIconTile tone="primary" size="lg">
                <Blocks />
              </SemanticIconTile>
              <div className="min-w-0">
                <h1 className="text-2xl font-bold tracking-normal">技能管理</h1>
                <p className="text-sm text-muted-foreground">上传技能 zip 包，管理团队和数字员工的技能绑定。</p>
              </div>
            </div>
            <Button asChild>
              <a href="/skills/upload">
                <UploadCloud data-icon="inline-start" />
                上传技能
              </a>
            </Button>
          </div>

          <div className="grid gap-3 md:grid-cols-3">
            <SkillMetric icon={Blocks} label="技能总数" value={skillRows.length} tone="primary" />
            <SkillMetric icon={Bot} label="已绑定 Agent" value={countAgentBindings(skillRows)} tone="info" />
            <SkillMetric icon={FileArchive} label="归档文件数" value={countArchiveFiles(skillRows)} tone="artifact" />
          </div>

          {skills.isError ? (
            <Alert variant="destructive">
              <TriangleAlert />
              <AlertTitle>技能数据加载失败</AlertTitle>
              <AlertDescription>{skillsError ?? "请检查 Control Plane 技能接口和数据库迁移状态。"}</AlertDescription>
            </Alert>
          ) : null}

          <div className="grid min-h-[650px] gap-4 xl:grid-cols-[320px_minmax(0,1fr)]">
            <SkillListPanel
              onQueryChange={setQuery}
              onSelectSkill={setSelectedSkillId}
              query={query}
              selectedSkillId={selectedSkill?.id}
              skills={skillRows}
            />
            <SkillDetailPanel skill={selectedSkill} apiOptions={apiOptions} onDelete={setDeleteTarget} />
          </div>
        </div>
      </Main>

      <Dialog open={deleteTarget !== null} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <DialogContent className="max-w-md">
          <DialogHeader>
            <DialogTitle>删除技能</DialogTitle>
            <DialogDescription>
              确定要删除「{deleteTarget?.name}」吗？所有团队和数字员工的绑定关系将被移除，S3 归档对象也将被删除。此操作不可撤销。
            </DialogDescription>
          </DialogHeader>
          {deleteMutation.isError ? (
            <p className="text-sm text-destructive">{deleteMutation.error instanceof Error ? deleteMutation.error.message : "删除失败"}</p>
          ) : null}
          <DialogFooter>
            <Button onClick={() => setDeleteTarget(null)} type="button" variant="outline">取消</Button>
            <Button
              disabled={deleteMutation.isPending}
              onClick={() => deleteTarget && deleteMutation.mutate(deleteTarget.id)}
              type="button"
              variant="destructive"
            >
              <Trash2 data-icon="inline-start" />
              确认删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function SkillMetric({ icon: Icon, label, tone, value }: { icon: LucideIcon; label: string; tone: Tone; value: number }) {
  return (
    <LiquidCard role="group" aria-label={`${label} ${value}`} className="rounded-lg">
      <CardContent className="flex items-center gap-3 p-4">
        <SemanticIconTile tone={tone} size="sm">
          <Icon />
        </SemanticIconTile>
        <div>
          <p className="text-sm text-muted-foreground">{label}</p>
          <p className="text-2xl font-semibold tracking-normal">{value}</p>
        </div>
      </CardContent>
    </LiquidCard>
  );
}

function SkillListPanel({
  onQueryChange,
  onSelectSkill,
  query,
  selectedSkillId,
  skills,
}: {
  onQueryChange: (value: string) => void;
  onSelectSkill: (id: string) => void;
  query: string;
  selectedSkillId?: string;
  skills: Skill[];
}) {
  return (
    <LiquidCard className="min-w-0 rounded-lg">
      <CardHeader className="gap-3 border-b">
        <CardTitle className="text-base">技能列表</CardTitle>
        <div className="flex items-center gap-2 rounded-md border bg-background px-2">
          <SearchIcon className="size-4 text-muted-foreground" />
          <Input
            aria-label="搜索技能"
            className="border-0 px-0 shadow-none focus-visible:ring-0"
            onChange={(event) => onQueryChange(event.target.value)}
            placeholder="搜索技能名称"
            value={query}
          />
        </div>
      </CardHeader>
      <CardContent className="p-0">
        <ScrollArea className="h-[560px]">
          <div className="flex flex-col gap-1 p-3">
            {skills.map((skill) => (
              <button
                className={cn(
                  "flex min-w-0 items-start gap-2 rounded-md px-2 py-2 text-left text-sm hover:bg-muted",
                  selectedSkillId === skill.id && "bg-primary/10 text-primary",
                )}
                key={skill.id}
                onClick={() => onSelectSkill(skill.id)}
                type="button"
              >
                <SkillIcon skill={skill} size="sm" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium">{skill.name}</span>
                  <span className="block truncate text-xs text-muted-foreground">
                    {skill.archive_file_count} 个文件 · {skill.team_bindings.length} 个团队
                    {runtimeDependencyCount(skill) > 0 ? ` · ${runtimeDependencyCount(skill)} 个运行依赖` : ""}
                  </span>
                </span>
              </button>
            ))}
            {skills.length === 0 ? (
              <p className="p-4 text-center text-sm text-muted-foreground">暂无技能，请上传 zip 包</p>
            ) : null}
          </div>
        </ScrollArea>
      </CardContent>
    </LiquidCard>
  );
}

function SkillDetailPanel({
  skill,
  apiOptions,
  onDelete,
}: {
  skill?: Skill;
  apiOptions: ApiOpts;
  onDelete: (skill: Skill) => void;
}) {
  if (!skill) {
    return (
      <LiquidCard className="min-w-0 rounded-lg">
        <CardContent className="flex h-[560px] min-w-0 items-center justify-center p-6 text-center text-sm text-muted-foreground">
          请选择左侧技能查看详情
        </CardContent>
      </LiquidCard>
    );
  }

  return (
    <div className="flex min-w-0 flex-col gap-4">
      <LiquidCard className="rounded-lg">
        <CardHeader className="gap-3 border-b">
          <div className="flex min-w-0 items-start justify-between gap-3">
            <div className="flex min-w-0 items-start gap-3">
              <SkillIcon skill={skill} />
              <div className="min-w-0">
                <CardTitle className="truncate text-base">{skill.name}</CardTitle>
                <p className="mt-1 text-sm text-muted-foreground">{skill.description}</p>
              </div>
            </div>
            <Button aria-label={`删除技能 ${skill.name}`} onClick={() => onDelete(skill)} size="sm" type="button" variant="ghost">
              <Trash2 className="text-destructive" />
            </Button>
          </div>
        </CardHeader>
        <CardContent className="flex flex-col gap-4 p-4 text-sm">
          <div className="grid gap-3 md:grid-cols-2">
            <InfoRow label="版本" value={skill.version} />
            <InfoRow label="来源" value={skill.source} />
            <InfoRow label="风险等级" value={skill.risk_level} />
            <InfoRow label="文件数" value={`${skill.archive_file_count}`} />
            <InfoRow label="包大小" value={formatBytes(skill.archive_size_bytes)} />
            <InfoRow label="校验值" value={skill.archive_checksum_sha256.slice(0, 12) + "…"} />
          </div>
          <Separator />
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <User className="size-4" />
            <span>上传者：<span className="font-medium text-foreground">{skill.created_by_name || "未知"}</span></span>
          </div>
          <div className="flex flex-wrap gap-2">
            {skill.tags.map((tag) => (
              <Badge key={tag} variant="outline">{tag}</Badge>
            ))}
          </div>
          <RuntimeDependencyBadges skill={skill} />
        </CardContent>
      </LiquidCard>

      <TeamBindingSection skill={skill} apiOptions={apiOptions} />
      <EmployeeBindingSection skill={skill} apiOptions={apiOptions} />
    </div>
  );
}

function TeamBindingSection({ skill, apiOptions }: { skill: Skill; apiOptions: ApiOpts }) {
  const queryClient = useQueryClient();
  const [selectedTeamId, setSelectedTeamId] = useState<string>("");

  const teams = useQuery({
    queryKey: ["teams", "all"],
    queryFn: () => listTeams(apiOptions),
  });

  const boundTeamIds = useMemo(() => new Set(skill.team_bindings.map((b) => b.team_id)), [skill.team_bindings]);
  const availableTeams = (teams.data ?? []).filter((team) => !boundTeamIds.has(team.id));

  const bindMutation = useMutation({
    mutationFn: (teamId: string) => bindTeamSkill(apiOptions, teamId, skill.id),
    onSuccess: async () => {
      setSelectedTeamId("");
      await queryClient.invalidateQueries({ queryKey: ["skills"] });
    },
  });
  const unbindMutation = useMutation({
    mutationFn: (teamId: string) => unbindTeamSkill(apiOptions, teamId, skill.id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["skills"] });
    },
  });

  return (
    <LiquidCard className="rounded-lg">
      <CardHeader className="border-b">
        <CardTitle className="text-base">团队绑定</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3 p-4">
        <div className="flex items-center gap-2">
          <Select onValueChange={setSelectedTeamId} value={selectedTeamId}>
            <SelectTrigger className="min-w-0 flex-1">
              <SelectValue placeholder="选择要绑定的团队" />
            </SelectTrigger>
            <SelectContent>
              {availableTeams.map((team) => (
                <SelectItem key={team.id} value={team.id}>{team.name}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            disabled={!selectedTeamId || bindMutation.isPending}
            onClick={() => selectedTeamId && bindMutation.mutate(selectedTeamId)}
            size="sm"
            type="button"
          >
            <Plus data-icon="inline-start" />
            绑定
          </Button>
        </div>
        {bindMutation.isError ? (
          <p className="text-sm text-destructive">{bindMutation.error instanceof Error ? bindMutation.error.message : "绑定失败"}</p>
        ) : null}
        <div className="flex flex-col gap-2">
          {skill.team_bindings.map((binding) => (
            <div className="flex items-center justify-between gap-3 rounded-md border bg-background p-3" key={binding.team_id}>
              <span className="truncate text-sm font-medium">{binding.team_name}</span>
              <Button
                disabled={unbindMutation.isPending}
                onClick={() => unbindMutation.mutate(binding.team_id)}
                size="sm"
                type="button"
                variant="ghost"
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
          ))}
          {skill.team_bindings.length === 0 ? <p className="text-sm text-muted-foreground">暂无团队绑定</p> : null}
        </div>
      </CardContent>
    </LiquidCard>
  );
}

function EmployeeBindingSection({ skill, apiOptions }: { skill: Skill; apiOptions: ApiOpts }) {
  const queryClient = useQueryClient();
  const [selectedEmployeeId, setSelectedEmployeeId] = useState<string>("");

  const boundTeamIds = useMemo(() => skill.team_bindings.map((b) => b.team_id), [skill.team_bindings]);

  const employeesQuery = useQuery({
    enabled: boundTeamIds.length > 0,
    queryKey: ["skill-employees", boundTeamIds.join(",")],
    queryFn: async () => {
      const results = await Promise.all(
        boundTeamIds.map((teamId) => listDigitalEmployees(apiOptions, { team_id: teamId })),
      );
      const merged: DigitalEmployee[] = [];
      const seen = new Set<string>();
      for (const list of results) {
        for (const emp of list) {
          if (!seen.has(emp.id)) {
            seen.add(emp.id);
            merged.push(emp);
          }
        }
      }
      return merged;
    },
  });

  const personalBoundIds = useMemo(
    () => new Set(skill.agent_bindings.map((b) => b.agent_id)),
    [skill.agent_bindings],
  );

  const availableEmployees = (employeesQuery.data ?? []).filter(
    (emp) => !personalBoundIds.has(emp.id),
  );

  const bindMutation = useMutation({
    mutationFn: (employeeId: string) => bindEmployeeSkill(apiOptions, employeeId, skill.id),
    onSuccess: async () => {
      setSelectedEmployeeId("");
      await queryClient.invalidateQueries({ queryKey: ["skills"] });
    },
  });
  const unbindMutation = useMutation({
    mutationFn: (employeeId: string) => unbindEmployeeSkill(apiOptions, employeeId, skill.id),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["skills"] });
    },
  });

  const personalBindings = skill.agent_bindings;

  return (
    <LiquidCard className="rounded-lg">
      <CardHeader className="border-b">
        <CardTitle className="text-base">数字员工绑定</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3 p-4">
        {boundTeamIds.length === 0 ? (
          <p className="text-sm text-muted-foreground">请先绑定团队，员工将通过团队继承获得此技能。如需个人补充绑定，团队绑定后可在此操作。</p>
        ) : (
          <>
            <p className="text-xs text-muted-foreground">团队继承的员工自动拥有此技能。下方可对未继承的员工做个人补充绑定。</p>
            <div className="flex items-center gap-2">
              <Select onValueChange={setSelectedEmployeeId} value={selectedEmployeeId}>
                <SelectTrigger className="min-w-0 flex-1">
                  <SelectValue placeholder="选择要绑定的数字员工" />
                </SelectTrigger>
                <SelectContent>
                  {availableEmployees.map((emp) => (
                    <SelectItem key={emp.id} value={emp.id}>{emp.name}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
              <Button
                disabled={!selectedEmployeeId || bindMutation.isPending}
                onClick={() => selectedEmployeeId && bindMutation.mutate(selectedEmployeeId)}
                size="sm"
                type="button"
              >
                <Plus data-icon="inline-start" />
                绑定
              </Button>
            </div>
            {bindMutation.isError ? (
              <p className="text-sm text-destructive">
                {bindMutation.error instanceof Error ? bindMutation.error.message : "绑定失败，可能团队已继承此技能"}
              </p>
            ) : null}
          </>
        )}

        <div className="flex flex-col gap-2">
          <p className="text-xs font-medium text-muted-foreground">个人绑定</p>
          {personalBindings.map((binding) => (
            <div className="flex items-center justify-between gap-3 rounded-md border bg-background p-3" key={binding.agent_id}>
              <div className="flex min-w-0 items-center gap-2">
                <SemanticIconTile tone="info" size="sm">
                  <Bot />
                </SemanticIconTile>
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{binding.agent_name}</p>
                  <p className="truncate text-xs text-muted-foreground">{binding.team_name || "未归属团队"}</p>
                </div>
              </div>
              <Button
                disabled={unbindMutation.isPending}
                onClick={() => unbindMutation.mutate(binding.agent_id)}
                size="sm"
                type="button"
                variant="ghost"
              >
                <Trash2 className="size-4" />
              </Button>
            </div>
          ))}
          {personalBindings.length === 0 ? <p className="text-sm text-muted-foreground">暂无个人绑定</p> : null}
        </div>
      </CardContent>
    </LiquidCard>
  );
}

function RuntimeDependencyBadges({ skill }: { skill: Skill }) {
  const tools = skill.runtime_dependencies?.tools ?? [];
  const env = skill.runtime_dependencies?.env ?? [];
  if (tools.length === 0 && env.length === 0) {
    return <p className="text-xs text-muted-foreground">运行依赖：无</p>;
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-xs text-muted-foreground">运行依赖</span>
      {tools.map((tool) => (
        <Badge key={`tool-${tool}`} variant="secondary">CLI {tool}</Badge>
      ))}
      {env.map((name) => (
        <Badge key={`env-${name}`} variant="outline">ENV {name}</Badge>
      ))}
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

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="text-muted-foreground">{label}</span>
      <span className="truncate font-medium">{value}</span>
    </div>
  );
}

function runtimeDependencyCount(skill: Skill) {
  return (skill.runtime_dependencies?.tools?.length ?? 0) + (skill.runtime_dependencies?.env?.length ?? 0);
}

function countAgentBindings(skills: Skill[]) {
  return skills.reduce((total, skill) => total + skill.agent_bindings.length, 0);
}

function countArchiveFiles(skills: Skill[]) {
  return skills.reduce((total, skill) => total + skill.archive_file_count, 0);
}

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

import { Boxes, KeyRound, Network, Plus, ShieldCheck, Trash2 } from "lucide-react";
import type { ReactNode } from "react";
import { useMemo, useState } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { LiquidCard, SemanticIconTile, StatusBadge } from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  createEmployeeMcpBinding,
  deleteEmployeeMcpBinding,
  listEffectiveMcpServers,
  listUserCredentials,
  type McpServer,
  type UserCredential,
} from "@/lib/api/capabilities";
import {
  bindEmployeeSkill,
  listEmployeeSkills,
  listSkills,
  unbindEmployeeSkill,
  type EffectiveEmployeeSkill,
  type Skill,
} from "@/lib/api/skills";

type EmployeeCapabilitiesPanelProps = {
  apiOptions: ApiClientOptions;
  employeeId: string;
};

const noCredentialValue = "none";

export function EmployeeCapabilitiesPanel({ apiOptions, employeeId }: EmployeeCapabilitiesPanelProps) {
  const queryClient = useQueryClient();
  const [mcpName, setMcpName] = useState("");
  const [mcpUrl, setMcpUrl] = useState("");
  const [credentialId, setCredentialId] = useState(noCredentialValue);

  const marketplace = useQuery({
    queryKey: ["skills", ""],
    queryFn: () => listSkills(apiOptions),
    placeholderData: keepPreviousData,
  });
  const employeeSkills = useQuery({
    queryKey: ["employee-skills", employeeId],
    queryFn: () => listEmployeeSkills(apiOptions, employeeId),
    placeholderData: keepPreviousData,
  });
  const credentials = useQuery({
    queryKey: ["user-credentials", "mcp_token"],
    queryFn: () => listUserCredentials(apiOptions, "mcp_token"),
    placeholderData: keepPreviousData,
  });
  const effectiveMcp = useQuery({
    queryKey: ["effective-mcp-servers", employeeId],
    queryFn: () => listEffectiveMcpServers(apiOptions, employeeId),
    placeholderData: keepPreviousData,
  });

  const installedSkillIds = useMemo(
    () => new Set((employeeSkills.data ?? []).map((entry) => entry.skill.id)),
    [employeeSkills.data],
  );
  const availableSkills = useMemo(
    () => (marketplace.data ?? []).filter((skill) => !installedSkillIds.has(skill.id)),
    [installedSkillIds, marketplace.data],
  );

  const bindSkillMutation = useMutation({
    mutationFn: (skillId: string) => bindEmployeeSkill(apiOptions, employeeId, skillId),
    onSuccess: async (skill) => {
      queryClient.setQueryData<EffectiveEmployeeSkill[]>(
        ["employee-skills", employeeId],
        (currentSkills = []) => {
          if (currentSkills.some((entry) => entry.skill.id === skill.id)) return currentSkills;

          return [
            ...currentSkills,
            { skill, source_scope: "employee", inherited: false, read_only: false },
          ];
        },
      );
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["employee-skills", employeeId] }),
        queryClient.invalidateQueries({ queryKey: ["skills", ""] }),
      ]);
    },
  });

  const unbindSkillMutation = useMutation({
    mutationFn: (skillId: string) => unbindEmployeeSkill(apiOptions, employeeId, skillId),
    onSuccess: async (_result, skillId) => {
      queryClient.setQueryData<EffectiveEmployeeSkill[]>(
        ["employee-skills", employeeId],
        (currentSkills = []) =>
          currentSkills.filter((entry) => entry.skill.id !== skillId || entry.source_scope !== "employee"),
      );
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["employee-skills", employeeId] }),
        queryClient.invalidateQueries({ queryKey: ["skills", ""] }),
      ]);
    },
  });

  const createMcpMutation = useMutation({
    mutationFn: (input: { name: string; url: string; credential_id?: string }) =>
      createEmployeeMcpBinding(apiOptions, employeeId, input),
    onSuccess: async () => {
      setMcpName("");
      setMcpUrl("");
      setCredentialId(noCredentialValue);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["employee-mcp-bindings", employeeId] }),
        queryClient.invalidateQueries({ queryKey: ["effective-mcp-servers", employeeId] }),
      ]);
    },
  });

  const deleteMcpMutation = useMutation({
    mutationFn: (serverId: string) => deleteEmployeeMcpBinding(apiOptions, employeeId, serverId),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["employee-mcp-bindings", employeeId] }),
        queryClient.invalidateQueries({ queryKey: ["effective-mcp-servers", employeeId] }),
      ]);
    },
  });

  const canCreateMcp =
    mcpName.trim().length > 0 && mcpUrl.trim().length > 0 && !createMcpMutation.isPending;

  const handleCreateMcp = () => {
    const input = {
      name: mcpName.trim(),
      url: mcpUrl.trim(),
      ...(credentialId !== noCredentialValue ? { credential_id: credentialId } : {}),
    };
    createMcpMutation.mutate(input);
  };

  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <LiquidCard className="min-w-0 rounded-xl">
        <CardHeader className="gap-3 pb-3">
          <PanelTitle
            icon={<Boxes />}
            meta={`${employeeSkills.data?.length ?? 0} 个生效`}
            title="个人技能"
            tone="artifact"
          />
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <section className="flex flex-col gap-2">
            <div className="flex items-center justify-between gap-3">
              <h3 className="text-sm font-medium">已生效技能</h3>
              {employeeSkills.isFetching ? <StatusBadge tone="info">刷新中</StatusBadge> : null}
            </div>
            <div className="flex flex-col gap-2">
              {employeeSkills.isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
              {employeeSkills.isError ? <p className="text-sm text-destructive">技能加载失败</p> : null}
              {unbindSkillMutation.isError ? <p className="text-sm text-destructive">技能移除失败</p> : null}
              {!employeeSkills.isLoading && !employeeSkills.isError && (employeeSkills.data?.length ?? 0) === 0 ? (
                <p className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">暂无生效技能</p>
              ) : null}
              {(employeeSkills.data ?? []).map((entry) => (
                <EmployeeSkillRow
                  entry={entry}
                  key={`${entry.source_scope}-${entry.skill.id}`}
                  onRemove={() => unbindSkillMutation.mutate(entry.skill.id)}
                  pending={unbindSkillMutation.isPending}
                />
              ))}
            </div>
          </section>

          <section className="flex flex-col gap-2">
            <div className="flex items-center justify-between gap-3">
              <h3 className="text-sm font-medium">技能市场</h3>
              {marketplace.isFetching ? <StatusBadge tone="info">刷新中</StatusBadge> : null}
            </div>
            <div className="flex flex-col gap-2">
              {marketplace.isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
              {marketplace.isError ? <p className="text-sm text-destructive">技能市场加载失败</p> : null}
              {bindSkillMutation.isError ? <p className="text-sm text-destructive">技能安装失败</p> : null}
              {!marketplace.isLoading && !marketplace.isError && availableSkills.length === 0 ? (
                <p className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">暂无可安装技能</p>
              ) : null}
              {availableSkills.map((skill) => (
                <SkillInstallRow
                  key={skill.id}
                  onInstall={() => bindSkillMutation.mutate(skill.id)}
                  pending={bindSkillMutation.isPending}
                  skill={skill}
                />
              ))}
            </div>
          </section>
        </CardContent>
      </LiquidCard>

      <LiquidCard className="min-w-0 rounded-xl">
        <CardHeader className="gap-3 pb-3">
          <PanelTitle
            icon={<Network />}
            meta={`${effectiveMcp.data?.length ?? 0} 个生效`}
            title="个人 MCP"
            tone="info"
          />
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
            <div className="min-w-0 space-y-2">
              <Label htmlFor="employee-mcp-name">个人 MCP 名称</Label>
              <Input
                disabled={createMcpMutation.isPending}
                id="employee-mcp-name"
                onChange={(event) => setMcpName(event.target.value)}
                value={mcpName}
              />
            </div>
            <div className="min-w-0 space-y-2">
              <Label htmlFor="employee-mcp-url">个人 MCP URL</Label>
              <Input
                disabled={createMcpMutation.isPending}
                id="employee-mcp-url"
                onChange={(event) => setMcpUrl(event.target.value)}
                value={mcpUrl}
              />
            </div>
            <div className="min-w-0 space-y-2">
              <Label htmlFor="employee-mcp-credential">个人 MCP 凭据</Label>
              <Select
                disabled={createMcpMutation.isPending}
                onValueChange={setCredentialId}
                value={credentialId}
              >
                <SelectTrigger aria-label="个人 MCP 凭据" className="w-full" id="employee-mcp-credential">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    <SelectItem value={noCredentialValue}>不使用凭据</SelectItem>
                    {(credentials.data ?? []).map((credential) => (
                      <SelectItem key={credential.id} value={credential.id}>
                        {credentialLabel(credential)}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
            <div className="flex min-w-0 items-end">
              <Button className="w-full" disabled={!canCreateMcp} onClick={handleCreateMcp} type="button">
                <Plus data-icon="inline-start" />
                添加个人 MCP
              </Button>
            </div>
          </div>
          {credentials.isError ? <p className="text-sm text-destructive">凭据加载失败</p> : null}
          {createMcpMutation.isError ? <p className="text-sm text-destructive">个人 MCP 添加失败</p> : null}

          <div className="flex flex-col gap-2">
            <div className="flex items-center justify-between gap-3">
              <h3 className="text-sm font-medium">已生效 MCP</h3>
              {effectiveMcp.isFetching ? <StatusBadge tone="info">刷新中</StatusBadge> : null}
            </div>
            {effectiveMcp.isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
            {effectiveMcp.isError ? <p className="text-sm text-destructive">MCP 加载失败</p> : null}
            {deleteMcpMutation.isError ? <p className="text-sm text-destructive">个人 MCP 移除失败</p> : null}
            {!effectiveMcp.isLoading && !effectiveMcp.isError && (effectiveMcp.data?.length ?? 0) === 0 ? (
              <p className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">暂无生效 MCP</p>
            ) : null}
            {(effectiveMcp.data ?? []).map((server) => (
              <McpRow
                key={server.id}
                onRemove={() => deleteMcpMutation.mutate(server.id)}
                pending={deleteMcpMutation.isPending}
                server={server}
              />
            ))}
          </div>
        </CardContent>
      </LiquidCard>
    </div>
  );
}

function PanelTitle({
  icon,
  meta,
  title,
  tone,
}: {
  icon: ReactNode;
  meta: string;
  title: string;
  tone: "artifact" | "info";
}) {
  return (
    <div className="flex min-w-0 items-center justify-between gap-3">
      <div className="flex min-w-0 items-center gap-3">
        <SemanticIconTile tone={tone} size="sm">
          {icon}
        </SemanticIconTile>
        <CardTitle className="truncate text-base">{title}</CardTitle>
      </div>
      <StatusBadge tone={tone}>{meta}</StatusBadge>
    </div>
  );
}

function EmployeeSkillRow({
  entry,
  onRemove,
  pending,
}: {
  entry: EffectiveEmployeeSkill;
  onRemove: () => void;
  pending: boolean;
}) {
  const isReadOnly = entry.read_only || entry.inherited || entry.source_scope !== "employee";
  return (
    <div className="grid min-w-0 gap-3 rounded-md border bg-background/70 p-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
      <div className="flex min-w-0 items-start gap-3">
        <SemanticIconTile tone={entry.inherited ? "neutral" : "success"} size="sm">
          <ShieldCheck />
        </SemanticIconTile>
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <p className="truncate text-sm font-medium">{entry.skill.name}</p>
            {entry.inherited ? <StatusBadge tone="neutral">团队继承</StatusBadge> : null}
            <Badge variant="outline" className="shrink-0">
              {entry.skill.version}
            </Badge>
            <StatusBadge tone={skillRiskTone(entry.skill.risk_level)}>
              {skillRiskLabel(entry.skill.risk_level)}
            </StatusBadge>
          </div>
          <p className="truncate text-xs text-muted-foreground">{entry.skill.description}</p>
        </div>
      </div>
      <Button
        disabled={isReadOnly || pending}
        onClick={isReadOnly ? undefined : onRemove}
        size="sm"
        type="button"
        variant="ghost"
      >
        <Trash2 data-icon="inline-start" />
        {`移除 ${entry.skill.name}`}
      </Button>
    </div>
  );
}

function SkillInstallRow({
  onInstall,
  pending,
  skill,
}: {
  onInstall: () => void;
  pending: boolean;
  skill: Skill;
}) {
  return (
    <div className="grid min-w-0 gap-3 rounded-md border bg-background/70 p-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
      <div className="flex min-w-0 items-start gap-3">
        <SemanticIconTile tone="artifact" size="sm">
          <Boxes />
        </SemanticIconTile>
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <p className="truncate text-sm font-medium">{skill.name}</p>
            <Badge variant="outline" className="shrink-0">
              {skill.version}
            </Badge>
          </div>
          <p className="truncate text-xs text-muted-foreground">{skill.description}</p>
        </div>
      </div>
      <Button disabled={pending} onClick={onInstall} size="sm" type="button" variant="outline">
        <Plus data-icon="inline-start" />
        {`安装 ${skill.name}`}
      </Button>
    </div>
  );
}

function McpRow({
  onRemove,
  pending,
  server,
}: {
  onRemove: () => void;
  pending: boolean;
  server: McpServer;
}) {
  const isReadOnly = server.inherited || server.source_scope !== "employee";
  return (
    <div className="grid min-w-0 gap-3 rounded-md border bg-background/70 p-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
      <div className="flex min-w-0 items-start gap-3">
        <SemanticIconTile tone={server.inherited ? "neutral" : "info"} size="sm">
          <Network />
        </SemanticIconTile>
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <p className="truncate text-sm font-medium">{server.name}</p>
            {server.inherited ? <StatusBadge tone="neutral">团队继承</StatusBadge> : null}
            <StatusBadge tone={server.status === "active" ? "success" : "neutral"}>
              {serverStatusLabel(server.status)}
            </StatusBadge>
          </div>
          <p className="truncate text-xs text-muted-foreground">{server.url}</p>
          {server.credential_name ? (
            <p className="mt-1 flex min-w-0 items-center gap-1 text-xs text-muted-foreground">
              <KeyRound className="size-3 shrink-0" />
              <span className="truncate">{server.credential_name} ****{server.credential_last_four}</span>
            </p>
          ) : null}
        </div>
      </div>
      <Button
        aria-label={`移除 MCP ${server.name}`}
        disabled={isReadOnly || pending}
        onClick={isReadOnly ? undefined : onRemove}
        size="icon"
        type="button"
        variant="ghost"
      >
        <Trash2 />
      </Button>
    </div>
  );
}

function credentialLabel(credential: UserCredential) {
  return `${credential.name} ****${credential.last_four}`;
}

function skillRiskTone(riskLevel: string) {
  if (riskLevel === "high") return "danger";
  if (riskLevel === "medium") return "warning";
  return "neutral";
}

function skillRiskLabel(riskLevel: string) {
  const labels: Record<string, string> = {
    high: "高风险",
    low: "低风险",
    medium: "中风险",
  };

  return labels[riskLevel] ?? riskLevel;
}

function serverStatusLabel(status: string) {
  const labels: Record<string, string> = {
    active: "启用",
    disabled: "停用",
  };

  return labels[status] ?? status;
}

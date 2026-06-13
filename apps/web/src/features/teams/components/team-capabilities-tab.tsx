import { Boxes, KeyRound, Network, Plus, ShieldCheck, Trash2 } from "lucide-react";
import type { ReactNode } from "react";
import { useMemo, useState } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { LiquidCard, SemanticIconTile, StatusBadge } from "@/components/superteam";
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
import type { ApiClientOptions } from "@/lib/api/client";
import {
  createTeamMcpServer,
  deleteTeamMcpServer,
  listTeamMcpServers,
  listUserCredentials,
} from "@/lib/api/capabilities";
import type { UserCredential } from "@/lib/api/capabilities";
import { bindTeamSkill, listSkills, listTeamSkills, unbindTeamSkill } from "@/lib/api/skills";
import type { Skill } from "@/lib/api/skills";
import type { TeamConfigRevision } from "@/lib/api/teams";

type TeamCapabilitiesTabProps = {
  apiOptions: ApiClientOptions;
  canEdit: boolean;
  currentRevision?: TeamConfigRevision;
  teamId: string;
};

const noCredentialValue = "none";

export function TeamCapabilitiesTab({ apiOptions, canEdit, teamId }: TeamCapabilitiesTabProps) {
  const queryClient = useQueryClient();
  const [mcpName, setMcpName] = useState("");
  const [mcpUrl, setMcpUrl] = useState("");
  const [credentialId, setCredentialId] = useState(noCredentialValue);

  const marketplace = useQuery({
    queryKey: ["skills", ""],
    queryFn: () => listSkills(apiOptions),
    placeholderData: keepPreviousData,
  });
  const teamSkills = useQuery({
    queryKey: ["team-skills", teamId],
    queryFn: () => listTeamSkills(apiOptions, teamId),
    placeholderData: keepPreviousData,
  });
  const credentials = useQuery({
    queryKey: ["user-credentials", "mcp_token"],
    queryFn: () => listUserCredentials(apiOptions, "mcp_token"),
    placeholderData: keepPreviousData,
  });
  const mcpServers = useQuery({
    queryKey: ["team-mcp-servers", teamId],
    queryFn: () => listTeamMcpServers(apiOptions, teamId),
    placeholderData: keepPreviousData,
  });

  const installedSkillIds = useMemo(
    () => new Set((teamSkills.data ?? []).map((skill) => skill.id)),
    [teamSkills.data],
  );
  const availableSkills = useMemo(
    () => (marketplace.data ?? []).filter((skill) => !installedSkillIds.has(skill.id)),
    [installedSkillIds, marketplace.data],
  );

  const bindSkillMutation = useMutation({
    mutationFn: (skillId: string) => bindTeamSkill(apiOptions, teamId, skillId),
    onSuccess: async (installedSkill) => {
      queryClient.setQueryData<Skill[]>(["team-skills", teamId], (currentSkills = []) => {
        if (currentSkills.some((skill) => skill.id === installedSkill.id)) {
          return currentSkills;
        }

        return [...currentSkills, installedSkill];
      });
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["team-skills", teamId] }),
        queryClient.invalidateQueries({ queryKey: ["skills", ""] }),
      ]);
    },
  });
  const unbindSkillMutation = useMutation({
    mutationFn: (skillId: string) => unbindTeamSkill(apiOptions, teamId, skillId),
    onSuccess: async (_result, skillId) => {
      queryClient.setQueryData<Skill[]>(["team-skills", teamId], (currentSkills = []) =>
        currentSkills.filter((skill) => skill.id !== skillId),
      );
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["team-skills", teamId] }),
        queryClient.invalidateQueries({ queryKey: ["skills", ""] }),
      ]);
    },
  });
  const createMcpMutation = useMutation({
    mutationFn: () =>
      createTeamMcpServer(apiOptions, teamId, {
        name: mcpName.trim(),
        url: mcpUrl.trim(),
        ...(credentialId !== noCredentialValue ? { credential_id: credentialId } : {}),
      }),
    onSuccess: async () => {
      setMcpName("");
      setMcpUrl("");
      setCredentialId(noCredentialValue);
      await queryClient.invalidateQueries({ queryKey: ["team-mcp-servers", teamId] });
    },
  });
  const deleteMcpMutation = useMutation({
    mutationFn: (serverId: string) => deleteTeamMcpServer(apiOptions, teamId, serverId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["team-mcp-servers", teamId] });
    },
  });

  const canCreateMcp = canEdit && mcpName.trim().length > 0 && mcpUrl.trim().length > 0 && !createMcpMutation.isPending;

  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <LiquidCard className="min-w-0 rounded-xl">
        <CardHeader className="gap-3 pb-3">
          <PanelTitle
            icon={<Boxes />}
            meta={`${teamSkills.data?.length ?? 0} 个已安装`}
            title="公共技能"
            tone="artifact"
          />
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <section className="flex flex-col gap-2">
            <div className="flex items-center justify-between gap-3">
              <h3 className="text-sm font-medium">已安装</h3>
              {teamSkills.isFetching ? <StatusBadge tone="info">刷新中</StatusBadge> : null}
            </div>
            <div className="flex flex-col gap-2">
              {teamSkills.isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
              {teamSkills.isError ? <p className="text-sm text-destructive">公共技能加载失败</p> : null}
              {unbindSkillMutation.isError ? <p className="text-sm text-destructive">公共技能移除失败</p> : null}
              {!teamSkills.isLoading && !teamSkills.isError && (teamSkills.data?.length ?? 0) === 0 ? (
                <p className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">暂无公共技能</p>
              ) : null}
              {(teamSkills.data ?? []).map((skill) => (
                <SkillRow
                  actionLabel={`移除 ${skill.name}`}
                  canEdit={canEdit}
                  key={skill.id}
                  onAction={() => unbindSkillMutation.mutate(skill.id)}
                  pending={unbindSkillMutation.isPending}
                  skill={skill}
                  variant="installed"
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
                <SkillRow
                  actionLabel={`安装 ${skill.name}`}
                  canEdit={canEdit}
                  key={skill.id}
                  onAction={() => bindSkillMutation.mutate(skill.id)}
                  pending={bindSkillMutation.isPending}
                  skill={skill}
                  variant="available"
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
            meta={`${mcpServers.data?.length ?? 0} 个服务`}
            title="公共 MCP"
            tone="info"
          />
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
            <div className="min-w-0 space-y-2">
              <Label htmlFor="team-mcp-name">MCP 名称</Label>
              <Input
                disabled={!canEdit || createMcpMutation.isPending}
                id="team-mcp-name"
                onChange={(event) => setMcpName(event.target.value)}
                value={mcpName}
              />
            </div>
            <div className="min-w-0 space-y-2">
              <Label htmlFor="team-mcp-url">MCP URL</Label>
              <Input
                disabled={!canEdit || createMcpMutation.isPending}
                id="team-mcp-url"
                onChange={(event) => setMcpUrl(event.target.value)}
                value={mcpUrl}
              />
            </div>
            <div className="min-w-0 space-y-2">
              <Label htmlFor="team-mcp-credential">凭据</Label>
              <Select
                disabled={!canEdit || createMcpMutation.isPending}
                onValueChange={setCredentialId}
                value={credentialId}
              >
                <SelectTrigger aria-label="凭据" className="w-full" id="team-mcp-credential">
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
              <Button className="w-full" disabled={!canCreateMcp} onClick={() => createMcpMutation.mutate()} type="button">
                <Plus data-icon="inline-start" />
                添加公共 MCP
              </Button>
            </div>
          </div>
          {createMcpMutation.isError ? <p className="text-sm text-destructive">公共 MCP 添加失败</p> : null}

          <div className="flex flex-col gap-2">
            <div className="flex items-center justify-between gap-3">
              <h3 className="text-sm font-medium">已配置</h3>
              {mcpServers.isFetching ? <StatusBadge tone="info">刷新中</StatusBadge> : null}
            </div>
            {mcpServers.isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
            {mcpServers.isError ? <p className="text-sm text-destructive">公共 MCP 加载失败</p> : null}
            {deleteMcpMutation.isError ? <p className="text-sm text-destructive">公共 MCP 移除失败</p> : null}
            {!mcpServers.isLoading && !mcpServers.isError && (mcpServers.data?.length ?? 0) === 0 ? (
              <p className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">暂无公共 MCP</p>
            ) : null}
            {(mcpServers.data ?? []).map((server) => (
              <div
                className="grid min-w-0 gap-3 rounded-md border bg-background/70 p-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-center"
                key={server.id}
              >
                <div className="flex min-w-0 items-start gap-3">
                  <SemanticIconTile tone="info" size="sm">
                    <Network />
                  </SemanticIconTile>
                  <div className="min-w-0">
                    <div className="flex min-w-0 flex-wrap items-center gap-2">
                      <p className="truncate text-sm font-medium">{server.name}</p>
                      <StatusBadge tone={server.status === "active" ? "success" : "neutral"}>{serverStatusLabel(server.status)}</StatusBadge>
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
                  disabled={!canEdit || deleteMcpMutation.isPending}
                  onClick={() => deleteMcpMutation.mutate(server.id)}
                  size="icon"
                  type="button"
                  variant="ghost"
                >
                  <Trash2 />
                </Button>
              </div>
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

function SkillRow({
  actionLabel,
  canEdit,
  onAction,
  pending,
  skill,
  variant,
}: {
  actionLabel: string;
  canEdit: boolean;
  onAction: () => void;
  pending: boolean;
  skill: Skill;
  variant: "available" | "installed";
}) {
  return (
    <div className="grid min-w-0 gap-3 rounded-md border bg-background/70 p-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
      <div className="flex min-w-0 items-start gap-3">
        <SemanticIconTile tone={variant === "installed" ? "success" : "artifact"} size="sm">
          {variant === "installed" ? <ShieldCheck /> : <Boxes />}
        </SemanticIconTile>
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <p className="truncate text-sm font-medium">{skill.name}</p>
            <Badge variant="outline" className="shrink-0">
              {skill.version}
            </Badge>
            <StatusBadge tone={skillRiskTone(skill.risk_level)}>
              {skillRiskLabel(skill.risk_level)}
            </StatusBadge>
          </div>
          <p className="truncate text-xs text-muted-foreground">{skill.description}</p>
        </div>
      </div>
      <Button disabled={!canEdit || pending} onClick={onAction} size="sm" type="button" variant={variant === "installed" ? "ghost" : "outline"}>
        {variant === "installed" ? <Trash2 data-icon="inline-start" /> : <Plus data-icon="inline-start" />}
        {actionLabel}
      </Button>
    </div>
  );
}

function credentialLabel(credential: UserCredential) {
  return `${credential.name} ****${credential.last_four}`;
}

function skillRiskTone(riskLevel: string) {
  if (riskLevel === "high") {
    return "danger";
  }
  if (riskLevel === "medium") {
    return "warning";
  }
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

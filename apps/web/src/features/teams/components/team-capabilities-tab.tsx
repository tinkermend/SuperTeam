import { Boxes, KeyRound, Network, Plus, ShieldCheck, Trash2 } from "lucide-react";
import type { ReactNode } from "react";
import { useMemo, useState } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  IconTile,
  StatusPill,
  V3Button,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
  type V3Tone,
} from "@/components/superteam";
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
      <WorkSurface className="min-w-0">
        <PanelHeader
          icon={<Boxes />}
          meta={`${teamSkills.data?.length ?? 0} 个已安装`}
          title="公共技能"
          tone="artifact"
        />
        <div className="flex flex-col gap-5 p-4">
          <section className="flex flex-col gap-3">
            <SectionTitle
              isFetching={teamSkills.isFetching}
              title="已安装"
            />
            {unbindSkillMutation.isError ? (
              <V3ErrorState title="公共技能移除失败" />
            ) : null}
            <SkillTable
              actionLabel={(skill) => `移除 ${skill.name}`}
              canEdit={canEdit}
              emptyTitle="暂无公共技能"
              isError={teamSkills.isError}
              isLoading={teamSkills.isLoading}
              onAction={(skill) => unbindSkillMutation.mutate(skill.id)}
              pending={unbindSkillMutation.isPending}
              skills={teamSkills.data ?? []}
              variant="installed"
            />
          </section>

          <section className="flex flex-col gap-3">
            <SectionTitle
              isFetching={marketplace.isFetching}
              title="技能市场"
            />
            {bindSkillMutation.isError ? (
              <V3ErrorState title="技能安装失败" />
            ) : null}
            <SkillTable
              actionLabel={(skill) => `安装 ${skill.name}`}
              canEdit={canEdit}
              emptyTitle="暂无可安装技能"
              isError={marketplace.isError}
              isLoading={marketplace.isLoading}
              onAction={(skill) => bindSkillMutation.mutate(skill.id)}
              pending={bindSkillMutation.isPending}
              skills={availableSkills}
              variant="available"
            />
          </section>
        </div>
      </WorkSurface>

      <WorkSurface className="min-w-0">
        <PanelHeader
          icon={<Network />}
          meta={`${mcpServers.data?.length ?? 0} 个服务`}
          title="公共 MCP"
          tone="info"
        />
        <div className="flex flex-col gap-4 p-4">
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
              <V3Button className="w-full" disabled={!canCreateMcp} onClick={() => createMcpMutation.mutate()} type="button">
                <Plus data-icon="inline-start" />
                添加公共 MCP
              </V3Button>
            </div>
          </div>
          {createMcpMutation.isError ? (
            <V3ErrorState title="公共 MCP 添加失败" />
          ) : null}

          <section className="flex flex-col gap-3">
            <SectionTitle
              isFetching={mcpServers.isFetching}
              title="已配置"
            />
            {deleteMcpMutation.isError ? (
              <V3ErrorState title="公共 MCP 移除失败" />
            ) : null}
            {mcpServers.isLoading ? (
              <V3LoadingState label="公共 MCP 加载中" />
            ) : mcpServers.isError ? (
              <V3ErrorState title="公共 MCP 加载失败" />
            ) : (mcpServers.data?.length ?? 0) === 0 ? (
              <V3EmptyState title="暂无公共 MCP" />
            ) : (
              <V3Table>
                <thead>
                  <tr>
                    <V3Th>服务</V3Th>
                    <V3Th>凭据</V3Th>
                    <V3Th>状态</V3Th>
                    <V3Th className="text-right">操作</V3Th>
                  </tr>
                </thead>
                <tbody>
                  {(mcpServers.data ?? []).map((server) => (
                    <V3Tr key={server.id}>
                      <V3Td>
                        <div className="flex min-w-0 items-center gap-3">
                          <IconTile tone="info" size="sm">
                            <Network />
                          </IconTile>
                          <div className="min-w-0">
                            <p className="truncate text-sm font-medium text-v3-ink">{server.name}</p>
                            <p className="truncate text-xs text-v3-ink-2">{server.url}</p>
                          </div>
                        </div>
                      </V3Td>
                      <V3Td>
                        {server.credential_name ? (
                          <span className="flex min-w-0 items-center gap-1 text-xs text-v3-ink-2">
                            <KeyRound className="size-3 shrink-0" />
                            <span className="truncate">{server.credential_name} ****{server.credential_last_four}</span>
                          </span>
                        ) : (
                          <span className="text-xs text-v3-ink-3">无凭据</span>
                        )}
                      </V3Td>
                      <V3Td>
                        <StatusPill tone={server.status === "active" ? "ok" : "mute"}>
                          {serverStatusLabel(server.status)}
                        </StatusPill>
                      </V3Td>
                      <V3Td className="text-right">
                        <V3Button
                          aria-label={`移除 MCP ${server.name}`}
                          disabled={!canEdit || deleteMcpMutation.isPending}
                          onClick={() => deleteMcpMutation.mutate(server.id)}
                          size="icon"
                          type="button"
                          variant="ghost"
                        >
                          <Trash2 />
                        </V3Button>
                      </V3Td>
                    </V3Tr>
                  ))}
                </tbody>
              </V3Table>
            )}
          </section>
        </div>
      </WorkSurface>
    </div>
  );
}

function PanelHeader({
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
    <div className="flex min-w-0 items-center justify-between gap-3 border-b border-v3-line px-5 py-4">
      <div className="flex min-w-0 items-center gap-3">
        <IconTile tone={tone} size="sm">
          {icon}
        </IconTile>
        <h2 className="truncate text-base font-bold text-v3-ink">
          {title}
        </h2>
      </div>
      <StatusPill tone={tone}>{meta}</StatusPill>
    </div>
  );
}

function SectionTitle({
  isFetching,
  title,
}: {
  isFetching: boolean;
  title: string;
}) {
  return (
    <div className="flex items-center justify-between gap-3">
      <h3 className="text-sm font-bold text-v3-ink">{title}</h3>
      {isFetching ? <StatusPill tone="info">刷新中</StatusPill> : null}
    </div>
  );
}

function SkillTable({
  actionLabel,
  canEdit,
  emptyTitle,
  isError,
  isLoading,
  onAction,
  pending,
  skills,
  variant,
}: {
  actionLabel: (skill: Skill) => string;
  canEdit: boolean;
  emptyTitle: string;
  isError: boolean;
  isLoading: boolean;
  onAction: (skill: Skill) => void;
  pending: boolean;
  skills: Skill[];
  variant: "available" | "installed";
}) {
  if (isLoading) {
    return <V3LoadingState label="公共技能加载中" />;
  }

  if (isError) {
    return <V3ErrorState title={variant === "installed" ? "公共技能加载失败" : "技能市场加载失败"} />;
  }

  if (skills.length === 0) {
    return <V3EmptyState title={emptyTitle} />;
  }

  return (
    <V3Table>
      <thead>
        <tr>
          <V3Th>技能</V3Th>
          <V3Th>版本</V3Th>
          <V3Th>风险</V3Th>
          <V3Th className="text-right">操作</V3Th>
        </tr>
      </thead>
      <tbody>
        {skills.map((skill) => (
          <SkillRow
            actionLabel={actionLabel(skill)}
            canEdit={canEdit}
            key={skill.id}
            onAction={() => onAction(skill)}
            pending={pending}
            skill={skill}
            variant={variant}
          />
        ))}
      </tbody>
    </V3Table>
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
    <V3Tr>
      <V3Td>
        <div className="flex min-w-0 items-center gap-3">
          <IconTile tone={variant === "installed" ? "ok" : "artifact"} size="sm">
            {variant === "installed" ? <ShieldCheck /> : <Boxes />}
          </IconTile>
          <div className="min-w-0">
            <p className="truncate text-sm font-medium text-v3-ink">{skill.name}</p>
            <p className="truncate text-xs text-v3-ink-2">{skill.description}</p>
          </div>
        </div>
      </V3Td>
      <V3Td>
        <StatusPill tone="mute" showDot={false}>
          {skill.version}
        </StatusPill>
      </V3Td>
      <V3Td>
        <StatusPill tone={skillRiskTone(skill.risk_level)}>
          {skillRiskLabel(skill.risk_level)}
        </StatusPill>
      </V3Td>
      <V3Td className="text-right">
        <V3Button disabled={!canEdit || pending} onClick={onAction} size="sm" type="button" variant={variant === "installed" ? "ghost" : "outline"}>
          {variant === "installed" ? <Trash2 data-icon="inline-start" /> : <Plus data-icon="inline-start" />}
          {actionLabel}
        </V3Button>
      </V3Td>
    </V3Tr>
  );
}

function credentialLabel(credential: UserCredential) {
  return `${credential.name} ****${credential.last_four}`;
}

function skillRiskTone(riskLevel: string): V3Tone {
  if (riskLevel === "high") {
    return "danger";
  }
  if (riskLevel === "medium") {
    return "warn";
  }
  return "mute";
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

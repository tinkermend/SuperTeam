import { Boxes, KeyRound, Network, Plus, ShieldCheck, Trash2 } from "lucide-react";
import type { ReactNode } from "react";
import { useMemo, useState } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  IconTile,
  StatusPill,
  V3Button,
  V3ErrorState,
  V3LoadingState,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
  type V3Tone,
} from "@/components/superteam";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
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
  bindTeamMcpServer,
  deleteTeamMcpBinding,
  listMcpServerDefinitions,
  listTeamMcpBindings,
} from "@/lib/api/capabilities";
import type { McpBinding } from "@/lib/api/capabilities";
import { statusLabel } from "@/lib/status-labels";
import { bindTeamSkill, listSkills, listTeamSkills, unbindTeamSkill } from "@/lib/api/skills";
import type { Skill } from "@/lib/api/skills";

type TeamCapabilitiesTabProps = {
  apiOptions: ApiClientOptions;
  canEdit: boolean;
  teamId: string;
};

export function TeamCapabilitiesTab({ apiOptions, canEdit, teamId }: TeamCapabilitiesTabProps) {
  const queryClient = useQueryClient();
  const [skillDialogOpen, setSkillDialogOpen] = useState(false);
  const [mcpDialogOpen, setMcpDialogOpen] = useState(false);
  const [selectedServerId, setSelectedServerId] = useState("");
  const [credentialEnvVar, setCredentialEnvVar] = useState("");

  const marketplace = useQuery({
    enabled: skillDialogOpen,
    queryKey: ["skills", ""],
    queryFn: () => listSkills(apiOptions),
    placeholderData: keepPreviousData,
  });
  const teamSkills = useQuery({
    queryKey: ["team-skills", teamId],
    queryFn: () => listTeamSkills(apiOptions, teamId),
    placeholderData: keepPreviousData,
  });
  const mcpDefinitions = useQuery({
    enabled: mcpDialogOpen,
    queryKey: ["mcp-server-definitions"],
    queryFn: () => listMcpServerDefinitions(apiOptions),
    placeholderData: keepPreviousData,
  });
  const mcpBindings = useQuery({
    queryKey: ["team-mcp-bindings", teamId],
    queryFn: () => listTeamMcpBindings(apiOptions, teamId),
    placeholderData: keepPreviousData,
  });

  const selectedDefinition = useMemo(
    () => (mcpDefinitions.data ?? []).find((definition) => definition.id === selectedServerId),
    [mcpDefinitions.data, selectedServerId],
  );

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
      bindTeamMcpServer(apiOptions, teamId, {
        mcp_server_id: selectedServerId,
        ...(credentialEnvVar.trim().length > 0
          ? { credential_env_var: credentialEnvVar.trim() }
          : {}),
      }),
    onSuccess: async () => {
      setSelectedServerId("");
      setCredentialEnvVar("");
      setMcpDialogOpen(false);
      await queryClient.invalidateQueries({ queryKey: ["team-mcp-bindings", teamId] });
    },
  });
  const deleteMcpMutation = useMutation({
    mutationFn: (bindingId: string) => deleteTeamMcpBinding(apiOptions, teamId, bindingId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ["team-mcp-bindings", teamId] });
    },
  });

  const canCreateMcp =
    canEdit && selectedServerId.length > 0 && !createMcpMutation.isPending;
  const installedCount = teamSkills.data?.length ?? 0;
  const bindingCount = mcpBindings.data?.length ?? 0;

  return (
    <>
      <div className="grid gap-4 xl:grid-cols-2">
        <WorkSurface className="min-w-0">
          <PanelHeader
            action={
              canEdit ? (
                <V3Button
                  onClick={() => setSkillDialogOpen(true)}
                  size="sm"
                  type="button"
                  variant="ghost"
                >
                  <Plus data-icon="inline-start" className="size-3.5" />
                  安装技能
                </V3Button>
              ) : null
            }
            icon={<Boxes />}
            meta={`${installedCount} 个已安装`}
            title="公共技能"
            tone="artifact"
          />
          <div className="p-4">
            {unbindSkillMutation.isError ? <V3ErrorState title="公共技能移除失败" /> : null}
            <SkillTable
              actionLabel={(skill) => `移除 ${skill.name}`}
              canEdit={canEdit}
              emptyTitle="暂无已安装技能"
              isError={teamSkills.isError}
              isLoading={teamSkills.isLoading}
              onAction={(skill) => unbindSkillMutation.mutate(skill.id)}
              pending={unbindSkillMutation.isPending}
              skills={teamSkills.data ?? []}
              variant="installed"
            />
          </div>
        </WorkSurface>

        <WorkSurface className="min-w-0">
          <PanelHeader
            action={
              canEdit ? (
                <V3Button
                  onClick={() => setMcpDialogOpen(true)}
                  size="sm"
                  type="button"
                  variant="ghost"
                >
                  <Plus data-icon="inline-start" className="size-3.5" />
                  绑定 MCP
                </V3Button>
              ) : null
            }
            icon={<Network />}
            meta={`${bindingCount} 个绑定`}
            title="公共 MCP"
            tone="info"
          />
          <div className="p-4">
            {deleteMcpMutation.isError ? <V3ErrorState title="公共 MCP 移除失败" /> : null}
            {mcpBindings.isLoading ? (
              <V3LoadingState label="公共 MCP 加载中" />
            ) : mcpBindings.isError ? (
              <V3ErrorState title="公共 MCP 加载失败" />
            ) : bindingCount === 0 ? (
              <p className="py-2 text-[13px] text-v3-ink-2">暂无公共 MCP</p>
            ) : (
              <V3Table>
                <thead>
                  <tr>
                    <V3Th>服务</V3Th>
                    <V3Th>凭据环境变量</V3Th>
                    <V3Th>状态</V3Th>
                    <V3Th className="text-right">操作</V3Th>
                  </tr>
                </thead>
                <tbody>
                  {(mcpBindings.data ?? []).map((binding: McpBinding) => (
                    <V3Tr key={binding.id}>
                      <V3Td>
                        <div className="flex min-w-0 items-center gap-3">
                          <IconTile tone="info" size="sm">
                            <Network />
                          </IconTile>
                          <div className="min-w-0">
                            <p className="truncate text-sm font-medium text-v3-ink">
                              {binding.server_name ?? binding.server_key}
                            </p>
                            <p className="truncate font-mono text-xs text-v3-ink-2">
                              {binding.url ?? binding.server_key}
                            </p>
                          </div>
                        </div>
                      </V3Td>
                      <V3Td>
                        {binding.credential_env_var ? (
                          <span className="flex min-w-0 items-center gap-1 font-mono text-xs text-v3-ink-2">
                            <KeyRound className="size-3 shrink-0" />
                            <span className="truncate">{binding.credential_env_var}</span>
                          </span>
                        ) : (
                          <span className="text-xs text-v3-ink-3">无</span>
                        )}
                      </V3Td>
                      <V3Td>
                        <StatusPill tone={binding.status === "active" ? "ok" : "warn"}>
                          {statusLabel(binding.status)}
                        </StatusPill>
                      </V3Td>
                      <V3Td className="text-right">
                        <V3Button
                          aria-label={`移除 MCP ${binding.server_name ?? binding.server_key}`}
                          disabled={!canEdit || deleteMcpMutation.isPending}
                          onClick={() => deleteMcpMutation.mutate(binding.id)}
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
          </div>
        </WorkSurface>
      </div>

      <Dialog open={skillDialogOpen} onOpenChange={setSkillDialogOpen}>
        <DialogContent className="flex max-h-[85vh] flex-col gap-0 overflow-hidden p-0 sm:max-w-2xl">
          <DialogHeader className="border-b border-v3-line px-6 py-5 text-left">
            <DialogTitle>安装公共技能</DialogTitle>
            <DialogDescription>从技能市场选择并安装到本团队。</DialogDescription>
          </DialogHeader>
          <div className="min-h-0 flex-1 overflow-y-auto p-6">
            {bindSkillMutation.isError ? <V3ErrorState title="技能安装失败" /> : null}
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
          </div>
        </DialogContent>
      </Dialog>

      <Dialog
        open={mcpDialogOpen}
        onOpenChange={(open) => {
          setMcpDialogOpen(open);
          if (!open) {
            setSelectedServerId("");
            setCredentialEnvVar("");
          }
        }}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader className="text-left">
            <DialogTitle>绑定公共 MCP</DialogTitle>
            <DialogDescription>选择已注册的 MCP，可选填写凭据环境变量。</DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-4">
            <div className="min-w-0 space-y-2">
              <Label htmlFor="team-mcp-server">注册表 MCP</Label>
              <Select
                disabled={!canEdit || createMcpMutation.isPending}
                onValueChange={setSelectedServerId}
                value={selectedServerId}
              >
                <SelectTrigger aria-label="注册表 MCP" className="w-full" id="team-mcp-server">
                  <SelectValue placeholder="选择已注册的 MCP" />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {mcpDefinitions.isLoading ? (
                      <div className="px-2 py-4 text-center text-sm text-v3-ink-2">
                        加载中…
                      </div>
                    ) : mcpDefinitions.isError ? (
                      <div className="px-2 py-4 text-center text-sm text-v3-danger-text">
                        MCP 定义加载失败
                      </div>
                    ) : (mcpDefinitions.data ?? []).length === 0 ? (
                      <div className="px-2 py-4 text-center text-sm text-v3-ink-2">
                        暂无已注册 MCP
                      </div>
                    ) : (
                      (mcpDefinitions.data ?? []).map((definition) => (
                        <SelectItem key={definition.id} value={definition.id}>
                          {definition.name}（{definition.server_key}）
                        </SelectItem>
                      ))
                    )}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
            {selectedServerId ? (
              <div className="min-w-0 space-y-2">
                <Label htmlFor="team-mcp-credential-env">凭据环境变量（可选）</Label>
                <Input
                  disabled={!canEdit || createMcpMutation.isPending}
                  id="team-mcp-credential-env"
                  onChange={(event) => setCredentialEnvVar(event.target.value)}
                  placeholder="例如 GITHUB_TOKEN"
                  value={credentialEnvVar}
                />
              </div>
            ) : null}
            {selectedDefinition && selectedDefinition.required_env_vars.length > 0 ? (
              <p className="text-xs text-v3-ink-2">
                该 MCP 需要环境变量 {selectedDefinition.required_env_vars.join("、")}
                ，请在各数字员工环境变量中配置；团队级绑定为建议性。
              </p>
            ) : null}
            {createMcpMutation.isError ? <V3ErrorState title="公共 MCP 绑定失败" /> : null}
            <V3Button
              disabled={!canCreateMcp}
              onClick={() => createMcpMutation.mutate()}
              type="button"
            >
              <Plus data-icon="inline-start" />
              绑定公共 MCP
            </V3Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}

function PanelHeader({
  action,
  icon,
  meta,
  title,
  tone,
}: {
  action?: ReactNode;
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
        <h2 className="truncate text-base font-bold text-v3-ink">{title}</h2>
        <StatusPill tone={tone}>{meta}</StatusPill>
      </div>
      {action}
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
    return (
      <V3ErrorState
        title={variant === "installed" ? "公共技能加载失败" : "技能市场加载失败"}
      />
    );
  }

  if (skills.length === 0) {
    return <p className="py-2 text-[13px] text-v3-ink-2">{emptyTitle}</p>;
  }

  return (
    <V3Table>
      <thead>
        <tr>
          <V3Th>技能</V3Th>
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
            <p className="mt-0.5 truncate text-xs text-v3-ink-3">v{skill.version}</p>
          </div>
        </div>
      </V3Td>
      <V3Td>
        <StatusPill tone={skillRiskTone(skill.risk_level)}>
          {skillRiskLabel(skill.risk_level)}
        </StatusPill>
      </V3Td>
      <V3Td className="text-right">
        {variant === "installed" ? (
          <V3Button
            aria-label={actionLabel}
            disabled={!canEdit || pending}
            onClick={onAction}
            size="icon"
            type="button"
            variant="ghost"
          >
            <Trash2 className="size-4" />
            <span className="sr-only">{actionLabel}</span>
          </V3Button>
        ) : (
          <V3Button
            disabled={!canEdit || pending}
            onClick={onAction}
            size="sm"
            type="button"
            variant="outline"
          >
            <Plus data-icon="inline-start" />
            {actionLabel}
          </V3Button>
        )}
      </V3Td>
    </V3Tr>
  );
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

import { Boxes, KeyRound, Network, Plus, ShieldCheck, Trash2 } from "lucide-react";
import type { ReactNode } from "react";
import { useMemo, useState } from "react";
import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  IconTile,
  StatusPill,
  Button,
  ErrorState,
  LoadingState,
  DataTable,
  Td,
  Th,
  Tr,
  WorkSurface,
  type Tone
} from "@/components/superteam";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue
} from "@/components/ui/select";
import type { ApiClientOptions } from "@/lib/api/client";
import type { AllowedTeamAction } from "@/lib/api/teams";
import {
  bindTeamMcpServer,
  deleteTeamMcpBinding,
  getTeamCapabilityConflicts,
  listMcpServerDefinitions,
  listTeamMcpBindings
} from "@/lib/api/capabilities";
import { listDigitalEmployees } from "@/lib/api/employees";
import type { McpBinding, TeamCapabilityTakeover } from "@/lib/api/capabilities";
import { statusLabel } from "@/lib/status-labels";
import { bindTeamSkill, listSkills, listTeamSkills, unbindTeamSkill } from "@/lib/api/skills";
import type { Skill } from "@/lib/api/skills";

type TeamCapabilitiesTabProps = {
  allowedActions: AllowedTeamAction[];
  apiOptions: ApiClientOptions;
  teamId: string;
};

// 三个动作各管各的：技能安装/移除是 team.capability.bind / .unbind，MCP 绑定与解绑
// 都走 team.capability.manage（capability handler 的门禁）。此前统一用
// team.governance.edit，与服务端不一致——有权限的看到禁用按钮，没权限的点了拿 403。
export function TeamCapabilitiesTab({ allowedActions, apiOptions, teamId }: TeamCapabilitiesTabProps) {
  const canBindSkill = allowedActions.includes("team.capability.bind");
  const canUnbindSkill = allowedActions.includes("team.capability.unbind");
  const canManageMcp = allowedActions.includes("team.capability.manage");
  const queryClient = useQueryClient();
  const [skillDialogOpen, setSkillDialogOpen] = useState(false);
  const [mcpDialogOpen, setMcpDialogOpen] = useState(false);
  const [selectedServerId, setSelectedServerId] = useState("");
  const [credentialEnvVar, setCredentialEnvVar] = useState("");

  const marketplace = useQuery({
    enabled: skillDialogOpen,
    queryKey: ["skills", ""],
    queryFn: () => listSkills(apiOptions),
    placeholderData: keepPreviousData
});
  const teamSkills = useQuery({
    queryKey: ["team-skills", teamId],
    queryFn: () => listTeamSkills(apiOptions, teamId),
    placeholderData: keepPreviousData
});
  const mcpDefinitions = useQuery({
    enabled: mcpDialogOpen,
    queryKey: ["mcp-server-definitions"],
    queryFn: () => listMcpServerDefinitions(apiOptions),
    placeholderData: keepPreviousData
});
  const mcpBindings = useQuery({
    queryKey: ["team-mcp-bindings", teamId],
    queryFn: () => listTeamMcpBindings(apiOptions, teamId),
    placeholderData: keepPreviousData
});
  // 影响面：团队能力对全体成员生效，安装/移除前先让人看到会波及多少人。
  const teamEmployees = useQuery({
    queryKey: ["team-digital-employees", teamId],
    queryFn: () => listDigitalEmployees(apiOptions, { team_id: teamId }),
    placeholderData: keepPreviousData
});
  const memberCount = teamEmployees.data?.length ?? 0;
  // 接管预览：选中 MCP 后即查会收敛掉哪些成员的个人绑定（spec §5.2.1）。
  const takeoverPreview = useQuery({
    enabled: mcpDialogOpen && selectedServerId.length > 0,
    queryKey: ["team-capability-conflicts", teamId, selectedServerId],
    queryFn: () => getTeamCapabilityConflicts(apiOptions, teamId, selectedServerId)
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
    }
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
    }
});
  const createMcpMutation = useMutation({
    mutationFn: () =>
      bindTeamMcpServer(apiOptions, teamId, {
        mcp_server_id: selectedServerId,
        ...(credentialEnvVar.trim().length > 0
          ? { credential_env_var: credentialEnvVar.trim() }
          : {})
}),
    onSuccess: async () => {
      setSelectedServerId("");
      setCredentialEnvVar("");
      setMcpDialogOpen(false);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["team-mcp-bindings", teamId] }),
        queryClient.invalidateQueries({ queryKey: ["team-capability-readiness", teamId] }),
        queryClient.invalidateQueries({ queryKey: ["team-overview", teamId] }),
      ]);
    }
});
  const deleteMcpMutation = useMutation({
    mutationFn: (bindingId: string) => deleteTeamMcpBinding(apiOptions, teamId, bindingId),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["team-mcp-bindings", teamId] }),
        queryClient.invalidateQueries({ queryKey: ["team-capability-readiness", teamId] }),
        queryClient.invalidateQueries({ queryKey: ["team-overview", teamId] }),
      ]);
    }
});

  const canCreateMcp =
    canManageMcp && selectedServerId.length > 0 && !createMcpMutation.isPending;
  const installedCount = teamSkills.data?.length ?? 0;
  const bindingCount = mcpBindings.data?.length ?? 0;

  return (
    <>
      <div className="grid gap-4 xl:grid-cols-2">
        <WorkSurface className="min-w-0">
          <PanelHeader
            action={
              canBindSkill ? (
                <Button
                  onClick={() => setSkillDialogOpen(true)}
                  size="sm"
                  type="button"
                  variant="ghost"
                >
                  <Plus data-icon="inline-start" className="size-3.5" />
                  安装技能
                </Button>
              ) : null
            }
            icon={<Boxes />}
            meta={`${installedCount} 个已安装 · 对 ${memberCount} 名成员生效`}
            title="公共技能"
            tone="artifact"
          />
          <div className="p-4">
            {unbindSkillMutation.isError ? <ErrorState title="公共技能移除失败" /> : null}
            <SkillTable
              actionLabel={(skill) => `移除 ${skill.name}`}
              canAct={canUnbindSkill}
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
              canManageMcp ? (
                <Button
                  onClick={() => setMcpDialogOpen(true)}
                  size="sm"
                  type="button"
                  variant="ghost"
                >
                  <Plus data-icon="inline-start" className="size-3.5" />
                  绑定 MCP
                </Button>
              ) : null
            }
            icon={<Network />}
            meta={`${bindingCount} 个绑定 · 对 ${memberCount} 名成员生效`}
            title="公共 MCP"
            tone="info"
          />
          <div className="p-4">
            {deleteMcpMutation.isError ? <ErrorState title="公共 MCP 移除失败" /> : null}
            {mcpBindings.isLoading ? (
              <LoadingState label="公共 MCP 加载中" />
            ) : mcpBindings.isError ? (
              <ErrorState title="公共 MCP 加载失败" />
            ) : bindingCount === 0 ? (
              <p className="py-2 text-[13px] text-ink-2">暂无公共 MCP</p>
            ) : (
              <DataTable>
                <thead>
                  <tr>
                    <Th>服务</Th>
                    <Th>凭据环境变量</Th>
                    <Th>状态</Th>
                    <Th className="text-right">操作</Th>
                  </tr>
                </thead>
                <tbody>
                  {(mcpBindings.data ?? []).map((binding: McpBinding) => (
                    <Tr key={binding.id}>
                      <Td>
                        <div className="flex min-w-0 items-center gap-3">
                          <IconTile tone="info" size="sm">
                            <Network />
                          </IconTile>
                          <div className="min-w-0">
                            <p className="truncate text-sm font-medium text-ink">
                              {binding.server_name ?? binding.server_key}
                            </p>
                            <p className="truncate font-mono text-xs text-ink-2">
                              {binding.url ?? binding.server_key}
                            </p>
                          </div>
                        </div>
                      </Td>
                      <Td>
                        {binding.credential_env_var ? (
                          <span className="flex min-w-0 items-center gap-1 font-mono text-xs text-ink-2">
                            <KeyRound className="size-3 shrink-0" />
                            <span className="truncate">{binding.credential_env_var}</span>
                          </span>
                        ) : (
                          <span className="text-xs text-ink-3">无</span>
                        )}
                      </Td>
                      <Td>
                        <StatusPill tone={binding.status === "active" ? "ok" : "warn"}>
                          {statusLabel(binding.status)}
                        </StatusPill>
                      </Td>
                      <Td className="text-right">
                        <Button
                          aria-label={`移除 MCP ${binding.server_name ?? binding.server_key}`}
                          disabled={!canManageMcp || deleteMcpMutation.isPending}
                          onClick={() => deleteMcpMutation.mutate(binding.id)}
                          size="icon"
                          type="button"
                          variant="ghost"
                        >
                          <Trash2 />
                        </Button>
                      </Td>
                    </Tr>
                  ))}
                </tbody>
              </DataTable>
            )}
          </div>
        </WorkSurface>
      </div>

      <Dialog open={skillDialogOpen} onOpenChange={setSkillDialogOpen}>
        <DialogContent className="flex max-h-[85vh] flex-col gap-0 overflow-hidden p-0 sm:max-w-2xl">
          <DialogHeader className="border-b border-line px-6 py-5 text-left">
            <DialogTitle>安装公共技能</DialogTitle>
            <DialogDescription>从技能市场选择并安装到本团队。</DialogDescription>
          </DialogHeader>
          <div className="min-h-0 flex-1 overflow-y-auto p-6">
            {bindSkillMutation.isError ? <ErrorState title="技能安装失败" /> : null}
            <SkillTable
              actionLabel={(skill) => `安装 ${skill.name}`}
              canAct={canBindSkill}
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
                disabled={!canManageMcp || createMcpMutation.isPending}
                onValueChange={setSelectedServerId}
                value={selectedServerId}
              >
                <SelectTrigger aria-label="注册表 MCP" className="w-full" id="team-mcp-server">
                  <SelectValue placeholder="选择已注册的 MCP" />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {mcpDefinitions.isLoading ? (
                      <div className="px-2 py-4 text-center text-sm text-ink-2">
                        加载中…
                      </div>
                    ) : mcpDefinitions.isError ? (
                      <div className="px-2 py-4 text-center text-sm text-danger-text">
                        MCP 定义加载失败
                      </div>
                    ) : (mcpDefinitions.data ?? []).length === 0 ? (
                      <div className="px-2 py-4 text-center text-sm text-ink-2">
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
                  disabled={!canManageMcp || createMcpMutation.isPending}
                  id="team-mcp-credential-env"
                  onChange={(event) => setCredentialEnvVar(event.target.value)}
                  placeholder="例如 GITHUB_TOKEN"
                  value={credentialEnvVar}
                />
              </div>
            ) : null}
            {selectedDefinition && selectedDefinition.required_env_vars.length > 0 ? (
              <p className="text-xs text-ink-2">
                该 MCP 需要环境变量 {selectedDefinition.required_env_vars.join("、")}
                。变量名在这里声明，值按成员各自配置——绑定后到「成员就绪矩阵」补齐，缺值的成员用不了该能力。
              </p>
            ) : null}
            {selectedServerId ? (
              <TakeoverPreview
                isLoading={takeoverPreview.isLoading}
                takeovers={takeoverPreview.data?.takeovers ?? []}
                teamCredentialEnvVar={credentialEnvVar.trim()}
              />
            ) : null}
            {createMcpMutation.isError ? <ErrorState title="公共 MCP 绑定失败" /> : null}
            <Button
              disabled={!canCreateMcp}
              onClick={() => createMcpMutation.mutate()}
              type="button"
            >
              <Plus data-icon="inline-start" />
              绑定公共 MCP
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}

// 接管预览：团队绑定该 MCP 会物理收敛哪些成员的个人绑定。凭据变量名与团队不一致的
// 成员必须点名——接管后他们会立刻缺变量（spec §5.2.1）。
function TakeoverPreview({
  isLoading,
  takeovers,
  teamCredentialEnvVar
}: {
  isLoading: boolean;
  takeovers: TeamCapabilityTakeover[];
  teamCredentialEnvVar: string;
}) {
  if (isLoading) {
    return <p className="text-xs text-ink-3">正在检查会接管哪些成员的个人绑定…</p>;
  }
  if (takeovers.length === 0) {
    return null;
  }
  const mismatched = takeovers.filter(
    (item) => (item.prior_credential_env_var ?? "") !== teamCredentialEnvVar,
  );
  return (
    <div className="flex flex-col gap-1.5 rounded-[12px] border border-warn/40 bg-warn-soft px-3 py-2.5">
      <p className="text-[13px] font-medium text-warn-text">
        绑定后将接管 {takeovers.length} 名成员的同名个人绑定
      </p>
      <p className="text-xs text-ink-2">
        同一 MCP 在团队与个人两处只保留一份，团队生效；成员原有的个人绑定会被移除。
      </p>
      <ul className="flex flex-col gap-0.5">
        {takeovers.map((item) => (
          <li key={item.digital_employee_id} className="text-xs text-ink-2">
            {item.employee_name}
            {item.prior_credential_env_var ? (
              <span className="ml-1 font-mono text-[11px] text-ink-3">
                {item.prior_credential_env_var}
                {" → "}
                {teamCredentialEnvVar || "（团队未填凭据变量）"}
              </span>
            ) : null}
          </li>
        ))}
      </ul>
      {mismatched.length > 0 ? (
        <p className="text-xs text-danger">
          其中 {mismatched.length} 名成员原先用的是不同的凭据变量名，接管后会缺变量，请到「成员就绪矩阵」补齐。
        </p>
      ) : null}
    </div>
  );
}

function PanelHeader({
  action,
  icon,
  meta,
  title,
  tone
}: {
  action?: ReactNode;
  icon: ReactNode;
  meta: string;
  title: string;
  tone: "artifact" | "info";
}) {
  return (
    <div className="flex min-w-0 items-center justify-between gap-3 border-b border-line px-5 py-4">
      <div className="flex min-w-0 items-center gap-3">
        <IconTile tone={tone} size="sm">
          {icon}
        </IconTile>
        <h2 className="truncate text-base font-bold text-ink">{title}</h2>
        <StatusPill tone={tone}>{meta}</StatusPill>
      </div>
      {action}
    </div>
  );
}

function SkillTable({
  actionLabel,
  canAct,
  emptyTitle,
  isError,
  isLoading,
  onAction,
  pending,
  skills,
  variant
}: {
  actionLabel: (skill: Skill) => string;
  canAct: boolean;
  emptyTitle: string;
  isError: boolean;
  isLoading: boolean;
  onAction: (skill: Skill) => void;
  pending: boolean;
  skills: Skill[];
  variant: "available" | "installed";
}) {
  if (isLoading) {
    return <LoadingState label="公共技能加载中" />;
  }

  if (isError) {
    return (
      <ErrorState
        title={variant === "installed" ? "公共技能加载失败" : "技能市场加载失败"}
      />
    );
  }

  if (skills.length === 0) {
    return <p className="py-2 text-[13px] text-ink-2">{emptyTitle}</p>;
  }

  return (
    <DataTable>
      <thead>
        <tr>
          <Th>技能</Th>
          <Th>风险</Th>
          <Th className="text-right">操作</Th>
        </tr>
      </thead>
      <tbody>
        {skills.map((skill) => (
          <SkillRow
            actionLabel={actionLabel(skill)}
            canAct={canAct}
            key={skill.id}
            onAction={() => onAction(skill)}
            pending={pending}
            skill={skill}
            variant={variant}
          />
        ))}
      </tbody>
    </DataTable>
  );
}

function SkillRow({
  actionLabel,
  canAct,
  onAction,
  pending,
  skill,
  variant
}: {
  actionLabel: string;
  canAct: boolean;
  onAction: () => void;
  pending: boolean;
  skill: Skill;
  variant: "available" | "installed";
}) {
  return (
    <Tr>
      <Td>
        <div className="flex min-w-0 items-center gap-3">
          <IconTile tone={variant === "installed" ? "ok" : "artifact"} size="sm">
            {variant === "installed" ? <ShieldCheck /> : <Boxes />}
          </IconTile>
          <div className="min-w-0">
            <p className="truncate text-sm font-medium text-ink">{skill.name}</p>
            <p className="truncate text-xs text-ink-2">{skill.description}</p>
            <p className="mt-0.5 truncate text-xs text-ink-3">v{skill.version}</p>
          </div>
        </div>
      </Td>
      <Td>
        <StatusPill tone={skillRiskTone(skill.risk_level)}>
          {skillRiskLabel(skill.risk_level)}
        </StatusPill>
      </Td>
      <Td className="text-right">
        {variant === "installed" ? (
          <Button
            aria-label={actionLabel}
            disabled={!canAct || pending}
            onClick={onAction}
            size="icon"
            type="button"
            variant="ghost"
          >
            <Trash2 className="size-4" />
            <span className="sr-only">{actionLabel}</span>
          </Button>
        ) : (
          <Button
            disabled={!canAct || pending}
            onClick={onAction}
            size="sm"
            type="button"
            variant="outline"
          >
            <Plus data-icon="inline-start" />
            {actionLabel}
          </Button>
        )}
      </Td>
    </Tr>
  );
}

function skillRiskTone(riskLevel: string): Tone {
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
    medium: "中风险"
};

  return labels[riskLevel] ?? riskLevel;
}

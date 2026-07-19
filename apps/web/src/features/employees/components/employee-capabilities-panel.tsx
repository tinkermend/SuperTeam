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
import {
  IconTile,
  SoftCard,
  StatusPill,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
  type V3Tone,
} from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  bindEmployeeMcpServer,
  deleteEmployeeMcpBindingV2,
  listEffectiveMcpConfig,
  listEmployeeMcpBindingsV2,
  listEmployeeSkillMcpDependencyStatus,
  listMcpServerDefinitions,
  type EffectiveMcpServer,
  type EmployeeSkillMcpDependencyItem,
  type McpBinding,
  type McpServerDefinition,
} from "@/lib/api/capabilities";
import {
  bindEmployeeSkill,
  listEmployeeSkills,
  listSkills,
  unbindEmployeeSkill,
  type EffectiveEmployeeSkill,
  type Skill,
} from "@/lib/api/skills";
import {
  deleteEmployeeEnvironmentVariable,
  listEmployeeEnvironmentVariables,
  upsertEmployeeEnvironmentVariable,
  type DigitalEmployeeEnvironmentVariableSummary,
} from "@/lib/api/employees";
import { statusLabel } from "@/lib/status-labels";

type EmployeeCapabilitiesPanelProps = {
  apiOptions: ApiClientOptions;
  employeeId: string;
};

export function EmployeeCapabilitiesPanel({ apiOptions, employeeId }: EmployeeCapabilitiesPanelProps) {
  const queryClient = useQueryClient();
  const [selectedServerId, setSelectedServerId] = useState("");
  const [credentialEnvVar, setCredentialEnvVar] = useState("");
  const [replacementValues, setReplacementValues] = useState<Record<string, string>>({});

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
  const mcpDefinitions = useQuery({
    queryKey: ["mcp-server-definitions"],
    queryFn: () => listMcpServerDefinitions(apiOptions),
    placeholderData: keepPreviousData,
  });
  const employeeMcpBindings = useQuery({
    queryKey: ["employee-mcp-bindings-v2", employeeId],
    queryFn: () => listEmployeeMcpBindingsV2(apiOptions, employeeId),
    placeholderData: keepPreviousData,
  });
  // Registry-resolved effective MCP config (migration 037): shows inherited vs personal
  // source and surfaces blocked_missing_env so the operator knows which env vars to set.
  const effectiveMcpConfig = useQuery({
    queryKey: ["effective-mcp-config", employeeId],
    queryFn: () => listEffectiveMcpConfig(apiOptions, employeeId),
    placeholderData: keepPreviousData,
  });
  const envVars = useQuery({
    queryKey: ["employee-environment-variables", employeeId],
    queryFn: () => listEmployeeEnvironmentVariables(apiOptions, employeeId),
    placeholderData: keepPreviousData,
  });
  // Skill-level MCP dependency preflight (Task 6): flags skills whose declared MCP
  // dependencies are not yet bound or are blocked on missing env vars, so the operator
  // sees the exact skill row that will fail dispatch.
  const skillMcpDependencyStatus = useQuery({
    queryKey: ["skill-mcp-dependency-status", employeeId],
    queryFn: () => listEmployeeSkillMcpDependencyStatus(apiOptions, employeeId),
    placeholderData: keepPreviousData,
  });

  const unsatisfiedMcpDepsBySkillId = useMemo(() => {
    const map = new Map<string, EmployeeSkillMcpDependencyItem[]>();
    for (const status of skillMcpDependencyStatus.data ?? []) {
      const unsatisfied = status.dependencies.filter((dep) => dep.status !== "satisfied");
      if (unsatisfied.length > 0) {
        map.set(status.skill_id, unsatisfied);
      }
    }
    return map;
  }, [skillMcpDependencyStatus.data]);

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
        queryClient.invalidateQueries({ queryKey: ["skill-mcp-dependency-status", employeeId] }),
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
        queryClient.invalidateQueries({ queryKey: ["skill-mcp-dependency-status", employeeId] }),
      ]);
    },
  });

  const createMcpMutation = useMutation({
    mutationFn: (input: { mcp_server_id: string; credential_env_var?: string }) =>
      bindEmployeeMcpServer(apiOptions, employeeId, input),
    onSuccess: async () => {
      setSelectedServerId("");
      setCredentialEnvVar("");
      await Promise.all([
        // Pre-existing bug: these keys did not match the useQuery keys actually used
        // above (employee-mcp-bindings-v2 / effective-mcp-config), so this mutation never
        // invalidated those caches. Fixed alongside the dependency-status invalidation.
        queryClient.invalidateQueries({ queryKey: ["employee-mcp-bindings-v2", employeeId] }),
        queryClient.invalidateQueries({ queryKey: ["effective-mcp-config", employeeId] }),
        queryClient.invalidateQueries({ queryKey: ["skill-mcp-dependency-status", employeeId] }),
      ]);
    },
  });

  const deleteMcpMutation = useMutation({
    mutationFn: (serverId: string) => deleteEmployeeMcpBindingV2(apiOptions, employeeId, serverId),
    onSuccess: async () => {
      await Promise.all([
        // Pre-existing bug: see createMcpMutation above.
        queryClient.invalidateQueries({ queryKey: ["employee-mcp-bindings-v2", employeeId] }),
        queryClient.invalidateQueries({ queryKey: ["effective-mcp-config", employeeId] }),
        queryClient.invalidateQueries({ queryKey: ["skill-mcp-dependency-status", employeeId] }),
      ]);
    },
  });
  const upsertEnvMutation = useMutation({
    mutationFn: (input: { name: string; value: string; sensitive: boolean }) =>
      upsertEmployeeEnvironmentVariable(apiOptions, employeeId, input.name, {
        value: input.value,
        sensitive: input.sensitive,
      }),
    onSuccess: async (_summary, input) => {
      setReplacementValues((current) => ({ ...current, [input.name]: "" }));
      await queryClient.invalidateQueries({ queryKey: ["employee-environment-variables", employeeId] });
      await queryClient.invalidateQueries({ queryKey: ["employee-skills", employeeId] });
      await queryClient.invalidateQueries({ queryKey: ["skill-mcp-dependency-status", employeeId] });
    },
  });
  const deleteEnvMutation = useMutation({
    mutationFn: (name: string) => deleteEmployeeEnvironmentVariable(apiOptions, employeeId, name),
    onSuccess: async (_result, name) => {
      setReplacementValues((current) => {
        const next = { ...current };
        delete next[name];
        return next;
      });
      await queryClient.invalidateQueries({ queryKey: ["employee-environment-variables", employeeId] });
      await queryClient.invalidateQueries({ queryKey: ["employee-skills", employeeId] });
      await queryClient.invalidateQueries({ queryKey: ["skill-mcp-dependency-status", employeeId] });
    },
  });

  const selectedDefinition = useMemo(
    () => (mcpDefinitions.data ?? []).find((definition) => definition.id === selectedServerId),
    [mcpDefinitions.data, selectedServerId],
  );
  const configuredEnvNames = useMemo(
    () => new Set((envVars.data ?? []).map((envVar) => envVar.name)),
    [envVars.data],
  );
  // Preflight: an employee binding is blocked until every required env var is configured.
  const missingEnvVars = useMemo(() => {
    if (!selectedDefinition) return [];
    return selectedDefinition.required_env_vars.filter((name) => !configuredEnvNames.has(name));
  }, [selectedDefinition, configuredEnvNames]);

  const canCreateMcp =
    selectedServerId.length > 0 && missingEnvVars.length === 0 && !createMcpMutation.isPending;

  const handleCreateMcp = () => {
    createMcpMutation.mutate({
      mcp_server_id: selectedServerId,
      ...(credentialEnvVar.trim().length > 0
        ? { credential_env_var: credentialEnvVar.trim() }
        : {}),
    });
  };

  return (
    <div className="grid gap-4 xl:grid-cols-2">
      <SoftCard className="min-w-0">
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
              {employeeSkills.isFetching ? <StatusPill tone="info">刷新中</StatusPill> : null}
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
                  unsatisfiedMcpDeps={unsatisfiedMcpDepsBySkillId.get(entry.skill.id) ?? []}
                />
              ))}
            </div>
          </section>

          <EmployeeEnvironmentPanel
            envVars={envVars.data ?? []}
            isError={envVars.isError}
            isFetching={envVars.isFetching}
            isLoading={envVars.isLoading}
            onDelete={(name) => deleteEnvMutation.mutate(name)}
            onReplace={(name, sensitive) => {
              const value = replacementValues[name] ?? "";
              if (value) {
                upsertEnvMutation.mutate({ name, value, sensitive });
              }
            }}
            onReplacementValueChange={(name, value) =>
              setReplacementValues((current) => ({ ...current, [name]: value }))
            }
            pendingDelete={deleteEnvMutation.isPending}
            pendingReplace={upsertEnvMutation.isPending}
            replacementValues={replacementValues}
          />

          <section className="flex flex-col gap-2">
            <div className="flex items-center justify-between gap-3">
              <h3 className="text-sm font-medium">技能市场</h3>
              {marketplace.isFetching ? <StatusPill tone="info">刷新中</StatusPill> : null}
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
      </SoftCard>

      <SoftCard className="min-w-0">
        <CardHeader className="gap-3 pb-3">
          <PanelTitle
            icon={<Network />}
            meta={`${employeeMcpBindings.data?.length ?? 0} 个绑定`}
            title="个人 MCP"
            tone="info"
          />
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <div className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
            <div className="min-w-0 space-y-2">
              <Label htmlFor="employee-mcp-server">注册表 MCP</Label>
              <Select
                disabled={createMcpMutation.isPending}
                onValueChange={setSelectedServerId}
                value={selectedServerId}
              >
                <SelectTrigger aria-label="注册表 MCP" className="w-full" id="employee-mcp-server">
                  <SelectValue placeholder="选择已注册的 MCP" />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {(mcpDefinitions.data ?? []).map((definition: McpServerDefinition) => (
                      <SelectItem key={definition.id} value={definition.id}>
                        {definition.name}（{definition.server_key}）
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>
            <div className="min-w-0 space-y-2">
              <Label htmlFor="employee-mcp-credential-env">凭据环境变量（可选）</Label>
              <Input
                disabled={createMcpMutation.isPending}
                id="employee-mcp-credential-env"
                onChange={(event) => setCredentialEnvVar(event.target.value)}
                placeholder="例如 GITHUB_TOKEN"
                value={credentialEnvVar}
              />
            </div>
            <div className="flex min-w-0 items-end">
              <Button className="w-full" disabled={!canCreateMcp} onClick={handleCreateMcp} type="button">
                <Plus data-icon="inline-start" />
                绑定个人 MCP
              </Button>
            </div>
          </div>
          {selectedDefinition && selectedDefinition.required_env_vars.length > 0 ? (
            <div className="flex flex-wrap items-center gap-1 text-xs">
              <span className="text-muted-foreground">必需环境变量：</span>
              {selectedDefinition.required_env_vars.map((name) => {
                const missing = missingEnvVars.includes(name);
                return (
                  <span
                    key={name}
                    className={
                      missing
                        ? "rounded border border-destructive/40 bg-destructive/10 px-1.5 py-0.5 font-mono text-[11px] text-destructive"
                        : "rounded border bg-muted px-1.5 py-0.5 font-mono text-[11px]"
                    }
                  >
                    {name}
                  </span>
                );
              })}
            </div>
          ) : null}
          {missingEnvVars.length > 0 ? (
            <p className="text-xs text-destructive">
              缺少环境变量 {missingEnvVars.join("、")}，请先在下方环境变量区域配置后再绑定。
            </p>
          ) : null}
          {createMcpMutation.isError ? <p className="text-sm text-destructive">个人 MCP 绑定失败</p> : null}

          <EmployeeMcpBindingsSection
            bindings={employeeMcpBindings.data ?? []}
            isError={employeeMcpBindings.isError}
            isFetching={employeeMcpBindings.isFetching}
            isLoading={employeeMcpBindings.isLoading}
            onRemove={(bindingId) => deleteMcpMutation.mutate(bindingId)}
            removing={deleteMcpMutation.isPending}
            removeError={deleteMcpMutation.isError}
          />

          <EffectiveMcpRegistrySection
            config={effectiveMcpConfig.data ?? []}
            isError={effectiveMcpConfig.isError}
            isFetching={effectiveMcpConfig.isFetching}
            isLoading={effectiveMcpConfig.isLoading}
          />
        </CardContent>
      </SoftCard>
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
  tone: Extract<V3Tone, "artifact" | "info">;
}) {
  return (
    <div className="flex min-w-0 items-center justify-between gap-3">
      <div className="flex min-w-0 items-center gap-3">
        <IconTile tone={tone} size="sm">
          {icon}
        </IconTile>
        <CardTitle className="truncate text-base">{title}</CardTitle>
      </div>
      <StatusPill tone={tone}>{meta}</StatusPill>
    </div>
  );
}

function EmployeeSkillRow({
  entry,
  onRemove,
  pending,
  unsatisfiedMcpDeps,
}: {
  entry: EffectiveEmployeeSkill;
  onRemove: () => void;
  pending: boolean;
  unsatisfiedMcpDeps: EmployeeSkillMcpDependencyItem[];
}) {
  const isReadOnly = entry.read_only || entry.inherited || entry.source_scope !== "employee";
  return (
    <div className="grid min-w-0 gap-3 rounded-md border bg-background/70 p-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
      <div className="flex min-w-0 items-start gap-3">
        <IconTile tone={entry.inherited ? "mute" : "ok"} size="sm">
          <ShieldCheck />
        </IconTile>
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <p className="truncate text-sm font-medium">{entry.skill.name}</p>
            {entry.inherited ? <StatusPill tone="mute">团队继承</StatusPill> : null}
            <Badge variant="outline" className="shrink-0">
              {entry.skill.version}
            </Badge>
            <StatusPill tone={skillRiskTone(entry.skill.risk_level)}>
              {skillRiskLabel(entry.skill.risk_level)}
            </StatusPill>
            {skillLoadStatePills(entry).map((status) => (
              <StatusPill key={status.label} tone={status.tone}>{status.label}</StatusPill>
            ))}
            {unsatisfiedMcpDeps.map((dep) => (
              <StatusPill key={dep.mcp_server_id} tone="warn">
                {`缺 MCP ${dep.server_key}`}
              </StatusPill>
            ))}
          </div>
          <p className="truncate text-xs text-muted-foreground">{entry.skill.description}</p>
          {unsatisfiedMcpDeps.length > 0 ? (
            <p className="text-xs text-destructive">
              {unsatisfiedMcpDeps
                .map((dep) =>
                  dep.status === "missing_binding"
                    ? `依赖 MCP ${dep.server_key} 未绑定`
                    : `依赖 MCP ${dep.server_key} 缺环境变量 ${dep.missing_env_vars.join(", ")}`,
                )
                .join("；")}
              ，任务派发将被阻断。请在右侧"个人 MCP"绑定或补齐环境变量。
            </p>
          ) : null}
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

function EmployeeEnvironmentPanel({
  envVars,
  isError,
  isFetching,
  isLoading,
  onDelete,
  onReplace,
  onReplacementValueChange,
  pendingDelete,
  pendingReplace,
  replacementValues,
}: {
  envVars: DigitalEmployeeEnvironmentVariableSummary[];
  isError: boolean;
  isFetching: boolean;
  isLoading: boolean;
  onDelete: (name: string) => void;
  onReplace: (name: string, sensitive: boolean) => void;
  onReplacementValueChange: (name: string, value: string) => void;
  pendingDelete: boolean;
  pendingReplace: boolean;
  replacementValues: Record<string, string>;
}) {
  return (
    <section className="flex flex-col gap-2">
      <div className="flex items-center justify-between gap-3">
        <h3 className="flex items-center gap-2 text-sm font-medium">
          <KeyRound className="size-4 text-v3-ok" />
          环境变量
        </h3>
        {isFetching ? <StatusPill tone="info">刷新中</StatusPill> : null}
      </div>
      {isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
      {isError ? <p className="text-sm text-destructive">环境变量加载失败</p> : null}
      {!isLoading && !isError && envVars.length === 0 ? (
        <p className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">暂无员工环境变量</p>
      ) : null}
      {envVars.length > 0 ? (
        <WorkSurface>
          <V3Table>
            <thead>
              <tr>
                <V3Th>名称</V3Th>
                <V3Th>状态</V3Th>
                <V3Th>指纹</V3Th>
                <V3Th>更新时间</V3Th>
                <V3Th className="min-w-[260px]">操作</V3Th>
              </tr>
            </thead>
            <tbody>
              {envVars.map((envVar) => (
                <V3Tr key={envVar.name}>
                  <V3Td className="font-medium">{envVar.name}</V3Td>
                  <V3Td>
                    <StatusPill tone={envVar.configured ? "ok" : "warn"}>
                      {envVar.configured ? "已配置" : "缺失"}
                    </StatusPill>
                  </V3Td>
                  <V3Td className="font-mono text-xs text-v3-ink-2">{envVar.fingerprint || "-"}</V3Td>
                  <V3Td className="text-xs text-v3-ink-2">{formatDateTime(envVar.updated_at)}</V3Td>
                  <V3Td>
                    <span className="flex items-center gap-2">
                      <Input
                        aria-label={`替换 ${envVar.name}`}
                        className="h-8"
                        onChange={(event) => onReplacementValueChange(envVar.name, event.target.value)}
                        type="password"
                        value={replacementValues[envVar.name] ?? ""}
                      />
                      <Button
                        disabled={pendingReplace || !(replacementValues[envVar.name] ?? "")}
                        onClick={() => onReplace(envVar.name, envVar.sensitive)}
                        size="sm"
                        type="button"
                        variant="outline"
                      >
                        替换
                      </Button>
                      <Button
                        aria-label={`删除环境变量 ${envVar.name}`}
                        disabled={pendingDelete}
                        onClick={() => onDelete(envVar.name)}
                        size="icon"
                        type="button"
                        variant="ghost"
                      >
                        <Trash2 />
                      </Button>
                    </span>
                  </V3Td>
                </V3Tr>
              ))}
            </tbody>
          </V3Table>
        </WorkSurface>
      ) : null}
    </section>
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
        <IconTile tone="artifact" size="sm">
          <Boxes />
        </IconTile>
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

// EffectiveMcpRegistrySection renders the registry-resolved effective MCP config for an
// employee (team-inherited plus personal), highlighting bindings blocked by missing env
// vars so the operator can set the exact variable names.
function EffectiveMcpRegistrySection({
  config,
  isError,
  isFetching,
  isLoading,
}: {
  config: EffectiveMcpServer[];
  isError: boolean;
  isFetching: boolean;
  isLoading: boolean;
}) {
  return (
    <div className="flex flex-col gap-2 border-t pt-3">
      <div className="flex items-center justify-between gap-3">
        <h3 className="text-sm font-medium">生效 MCP 配置（注册表）</h3>
        {isFetching ? <StatusPill tone="info">刷新中</StatusPill> : null}
      </div>
      {isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
      {isError ? <p className="text-sm text-destructive">MCP 配置加载失败</p> : null}
      {!isLoading && !isError && config.length === 0 ? (
        <p className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">
          暂无注册表 MCP 绑定
        </p>
      ) : null}
      {config.map((server) => {
        const blocked = server.missing_env_vars && server.missing_env_vars.length > 0;
        const tone = blocked ? "warn" : "ok";
        return (
          <div key={server.server_id} className="flex flex-col gap-1 rounded-md border p-3">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="flex min-w-0 items-center gap-2">
                <span className="font-medium">{server.name}</span>
                <span className="font-mono text-xs text-muted-foreground">{server.server_key}</span>
              </div>
              <div className="flex items-center gap-2">
                <StatusPill tone="mute">
                  {server.source_scope === "team" ? "团队继承" : "个人"}
                </StatusPill>
                <StatusPill tone={tone as Extract<V3Tone, "ok" | "warn">}>
                  {blocked ? "缺少环境变量" : statusLabel(server.status)}
                </StatusPill>
              </div>
            </div>
            <p className="truncate font-mono text-xs text-muted-foreground" title={server.url}>
              {server.url}
            </p>
            {server.required_env_vars && server.required_env_vars.length > 0 ? (
              <div className="flex flex-wrap items-center gap-1">
                <span className="text-xs text-muted-foreground">必需环境变量：</span>
                {server.required_env_vars.map((name) => {
                  const missing = server.missing_env_vars?.includes(name);
                  return (
                    <span
                      key={name}
                      className={
                        missing
                          ? "rounded border border-destructive/40 bg-destructive/10 px-1.5 py-0.5 font-mono text-[11px] text-destructive"
                          : "rounded border bg-muted px-1.5 py-0.5 font-mono text-[11px]"
                      }
                    >
                      {name}
                    </span>
                  );
                })}
              </div>
            ) : null}
            {blocked ? (
              <p className="text-xs text-destructive">
                缺少环境变量 {server.missing_env_vars?.join(", ")}，请在下方环境变量区域配置后再启动。
              </p>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}

// EmployeeMcpBindingsSection lists the employee's personal MCP bindings (registry v2) with
// their preflight status and a remove action. Team-inherited bindings are not shown here —
// they surface in EffectiveMcpRegistrySection.
function EmployeeMcpBindingsSection({
  bindings,
  isError,
  isFetching,
  isLoading,
  onRemove,
  removing,
  removeError,
}: {
  bindings: McpBinding[];
  isError: boolean;
  isFetching: boolean;
  isLoading: boolean;
  onRemove: (bindingId: string) => void;
  removing: boolean;
  removeError: boolean;
}) {
  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center justify-between gap-3">
        <h3 className="text-sm font-medium">个人 MCP 绑定</h3>
        {isFetching ? <StatusPill tone="info">刷新中</StatusPill> : null}
      </div>
      {isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
      {isError ? <p className="text-sm text-destructive">MCP 绑定加载失败</p> : null}
      {removeError ? <p className="text-sm text-destructive">个人 MCP 移除失败</p> : null}
      {!isLoading && !isError && bindings.length === 0 ? (
        <p className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">暂无个人 MCP 绑定</p>
      ) : null}
      {bindings.map((binding) => {
        const blocked = (binding.missing_env_vars ?? []).length > 0;
        return (
          <div key={binding.id} className="grid min-w-0 gap-3 rounded-md border bg-background/70 p-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
            <div className="flex min-w-0 items-start gap-3">
              <IconTile tone="info" size="sm">
                <Network />
              </IconTile>
              <div className="min-w-0">
                <div className="flex min-w-0 flex-wrap items-center gap-2">
                  <p className="truncate text-sm font-medium">{binding.server_name ?? binding.server_key}</p>
                  <StatusPill tone={blocked ? "warn" : "ok"}>{statusLabel(binding.status)}</StatusPill>
                </div>
                <p className="truncate font-mono text-xs text-muted-foreground">{binding.url ?? binding.server_key}</p>
                {binding.credential_env_var ? (
                  <p className="mt-1 flex min-w-0 items-center gap-1 font-mono text-xs text-muted-foreground">
                    <KeyRound className="size-3 shrink-0" />
                    <span className="truncate">{binding.credential_env_var}</span>
                  </p>
                ) : null}
                {blocked ? (
                  <p className="mt-1 text-xs text-destructive">
                    缺少环境变量 {(binding.missing_env_vars ?? []).join("、")}
                  </p>
                ) : null}
              </div>
            </div>
            <Button
              aria-label={`移除 MCP ${binding.server_name ?? binding.server_key}`}
              disabled={removing}
              onClick={() => onRemove(binding.id)}
              size="icon"
              type="button"
              variant="ghost"
            >
              <Trash2 />
            </Button>
          </div>
        );
      })}
    </div>
  );
}

function skillRiskTone(riskLevel: string) {
  if (riskLevel === "high") return "danger";
  if (riskLevel === "medium") return "warn";
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

function skillLoadStatePills(entry: EffectiveEmployeeSkill): Array<{ label: string; tone: V3Tone }> {
  const status = entry.runtime_dependency_status;
  const missingTools = status?.missing_tools ?? [];
  const missingEnv = status?.missing_env ?? [];
  const dependencyCount =
    (entry.skill.runtime_dependencies?.tools?.length ?? 0) + (entry.skill.runtime_dependencies?.env?.length ?? 0);

  if (missingTools.length > 0 || status?.load_status === "missing_tools" || status?.load_status === "missing_runtime_tools") {
    return [{ label: `缺少 Runtime 工具${missingTools.length ? `：${missingTools.join(",")}` : ""}`, tone: "warn" }];
  }
  if (missingEnv.length > 0 || status?.load_status === "missing_env" || status?.load_status === "missing_employee_env") {
    return [{ label: `缺少员工环境变量${missingEnv.length ? `：${missingEnv.join(",")}` : ""}`, tone: "warn" }];
  }
  if (status?.load_status === "pending_runtime" || status?.load_status === "waiting_runtime_report") {
    return [{ label: "等待 Runtime 上报", tone: "info" }];
  }
  if (!status?.load_status && dependencyCount > 0) {
    return [{ label: "等待 Runtime 上报", tone: "info" }];
  }
  return [{ label: "可加载", tone: "ok" }];
}

function formatDateTime(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

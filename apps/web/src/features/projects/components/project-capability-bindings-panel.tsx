import { Boxes, KeyRound, Network, Plus, Save, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient
} from "@tanstack/react-query";
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
import {
  Button,
  Callout,
  EmptyState,
  ErrorState,
  IconTile,
  LoadingState,
  StatusPill,
  WorkSurface
} from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  listMcpServerDefinitions,
  listProjectMcpBindings,
  putProjectMcpBindings,
  type CreateMcpBindingInput,
  type McpBinding,
  type McpServerDefinition
} from "@/lib/api/capabilities";
import {
  listProjectSkillBindings,
  listSkills,
  putProjectSkillBindings,
  type ProjectSkillBinding,
  type Skill
} from "@/lib/api/skills";
import { riskLevelLabel } from "@/lib/status-labels";
import { cn } from "@/lib/utils";

type ProjectCapabilityBindingsPanelProps = {
  apiOptions: ApiClientOptions;
  disabled?: boolean;
  projectId: string;
};

type SkillDraftRow = {
  skill_id: string;
  name: string;
  slug: string;
  risk_level: string;
  mcp_deps: Array<{ server_key: string; server_name: string }>;
};

type McpDraftRow = {
  mcp_server_id: string;
  server_key: string;
  server_name: string;
  transport?: string;
  credential_env_var?: string;
};

function skillToRow(skill: Skill): SkillDraftRow {
  const deps = skill.runtime_dependencies?.mcp_servers ?? [];
  return {
    skill_id: skill.id,
    name: skill.name,
    slug: skill.slug,
    risk_level: skill.risk_level,
    mcp_deps: deps.map((d) => ({
      server_key: d.server_key || d.mcp_server_id,
      server_name: d.server_name || d.server_key || d.mcp_server_id
    }))
  };
}

function bindingToSkillRow(binding: ProjectSkillBinding): SkillDraftRow | null {
  if (!binding.skill) {
    return {
      skill_id: binding.skill_id,
      name: binding.skill_id.slice(0, 8),
      slug: binding.skill_id.slice(0, 8),
      risk_level: "medium",
      mcp_deps: []
    };
  }
  return skillToRow(binding.skill);
}

function bindingToMcpRow(binding: McpBinding): McpDraftRow {
  return {
    mcp_server_id: binding.mcp_server_id,
    server_key: binding.server_key ?? binding.mcp_server_id,
    server_name: binding.server_name ?? binding.server_key ?? binding.mcp_server_id,
    transport: binding.transport,
    credential_env_var: binding.credential_env_var
  };
}

function definitionToMcpRow(
  definition: McpServerDefinition,
  credentialEnvVar?: string
): McpDraftRow {
  const trimmed = credentialEnvVar?.trim();
  return {
    mcp_server_id: definition.id,
    server_key: definition.server_key,
    server_name: definition.name,
    transport: definition.transport,
    ...(trimmed ? { credential_env_var: trimmed } : {})
  };
}

function rowsEqualSkill(a: SkillDraftRow[], b: SkillDraftRow[]) {
  if (a.length !== b.length) return false;
  const ids = a.map((r) => r.skill_id).sort().join(",");
  const idsB = b.map((r) => r.skill_id).sort().join(",");
  return ids === idsB;
}

function rowsEqualMcp(a: McpDraftRow[], b: McpDraftRow[]) {
  if (a.length !== b.length) return false;
  const key = (rows: McpDraftRow[]) =>
    rows
      .map((r) => `${r.mcp_server_id}|${r.credential_env_var ?? ""}`)
      .sort()
      .join(";");
  return key(a) === key(b);
}

export function ProjectCapabilityBindingsPanel({
  apiOptions,
  disabled,
  projectId
}: ProjectCapabilityBindingsPanelProps) {
  return (
    <div className="flex flex-col gap-4" data-testid="project-capability-bindings-panel">
      <Callout
        tone="info"
        title="场地供给"
        description="这里绑定的能力对进入本项目的所有数字员工生效，且只在本项目的任务中投影。员工自带的通用能力不受影响。"
      />
      <ProjectSkillBindingsSection
        apiOptions={apiOptions}
        disabled={disabled}
        projectId={projectId}
      />
      <ProjectMcpBindingsSection
        apiOptions={apiOptions}
        disabled={disabled}
        projectId={projectId}
      />
    </div>
  );
}

function ProjectSkillBindingsSection({
  apiOptions,
  disabled,
  projectId
}: ProjectCapabilityBindingsPanelProps) {
  const queryClient = useQueryClient();
  const [selectedSkillId, setSelectedSkillId] = useState("");
  const [draftRows, setDraftRows] = useState<SkillDraftRow[] | null>(null);

  const bindingsQuery = useQuery({
    queryKey: ["project-skill-bindings", projectId],
    queryFn: () => listProjectSkillBindings(apiOptions, projectId),
    placeholderData: keepPreviousData
  });
  const marketplaceQuery = useQuery({
    queryKey: ["skills", "project-capability-market"],
    queryFn: () => listSkills(apiOptions),
    placeholderData: keepPreviousData
  });

  const serverRows = useMemo(() => {
    return (bindingsQuery.data ?? [])
      .map(bindingToSkillRow)
      .filter((row): row is SkillDraftRow => row != null);
  }, [bindingsQuery.data]);

  const rows = draftRows ?? serverRows;
  const isDirty = draftRows !== null && !rowsEqualSkill(draftRows, serverRows);

  const boundIds = new Set(rows.map((r) => r.skill_id));
  const availableSkills = (marketplaceQuery.data ?? []).filter((s) => !boundIds.has(s.id));

  const saveMutation = useMutation({
    mutationFn: (next: SkillDraftRow[]) =>
      putProjectSkillBindings(
        apiOptions,
        projectId,
        next.map((r) => ({ skill_id: r.skill_id }))
      ),
    onSuccess: async () => {
      setDraftRows(null);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project-skill-bindings", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["skills"] })
      ]);
    }
  });

  function addSkill() {
    const skill = (marketplaceQuery.data ?? []).find((s) => s.id === selectedSkillId);
    if (!skill || boundIds.has(skill.id)) return;
    setDraftRows([...rows, skillToRow(skill)]);
    setSelectedSkillId("");
  }

  function removeSkill(skillId: string) {
    setDraftRows(rows.filter((r) => r.skill_id !== skillId));
  }

  const isSaving = saveMutation.isPending;
  const canAdd = !disabled && !isSaving && selectedSkillId.length > 0;
  const canSave = !disabled && !isSaving && isDirty;

  return (
    <WorkSurface data-testid="project-skill-bindings-section">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line p-4">
        <div className="flex min-w-0 items-center gap-3">
          <IconTile tone="ok" size="sm">
            <Boxes />
          </IconTile>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="font-semibold text-ink">项目技能</h3>
              {isDirty ? <StatusPill tone="warn">未保存</StatusPill> : null}
              {bindingsQuery.isFetching && !bindingsQuery.isLoading ? (
                <StatusPill tone="info">刷新中</StatusPill>
              ) : null}
            </div>
            <p className="mt-0.5 text-xs text-ink-2">
              绑定后作为场地供给投影；仅限本项目任务生效。
            </p>
          </div>
        </div>
        <Button
          disabled={!canSave}
          type="button"
          onClick={() => saveMutation.mutate(rows)}
        >
          <Save data-icon="inline-start" />
          保存技能绑定
        </Button>
      </div>

      <div className="flex flex-col gap-4 p-4">
        {isDirty ? (
          <Callout
            tone="warn"
            title="保存将整体替换项目技能绑定"
            description="当前列表会声明式全量写入。未保存前刷新页面不会改动已有绑定。"
          />
        ) : null}
        {saveMutation.isError ? (
          <Callout
            tone="danger"
            title="技能绑定保存失败"
            description={saveMutation.error.message}
          />
        ) : null}

        <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
          <div className="min-w-0 flex-1 space-y-2">
            <Label htmlFor="project-skill-market">从技能市场添加</Label>
            <Select
              disabled={disabled || isSaving}
              onValueChange={setSelectedSkillId}
              value={selectedSkillId}
            >
              <SelectTrigger aria-label="从技能市场添加" className="w-full" id="project-skill-market">
                <SelectValue placeholder="选择技能" />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {availableSkills.map((skill) => (
                    <SelectItem key={skill.id} value={skill.id}>
                      {skill.name}（{skill.slug}）
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
          <Button
            className="w-full sm:w-fit"
            disabled={!canAdd}
            type="button"
            variant="outline"
            onClick={addSkill}
          >
            <Plus data-icon="inline-start" />
            添加到列表
          </Button>
        </div>
        {marketplaceQuery.isError ? <ErrorState title="技能市场加载失败" /> : null}

        {bindingsQuery.isLoading ? (
          <LoadingState label="项目技能绑定加载中" />
        ) : bindingsQuery.isError ? (
          <ErrorState title="项目技能绑定加载失败" />
        ) : rows.length === 0 ? (
          <EmptyState title="暂无项目技能绑定" description="从技能市场添加后保存，即可作为场地供给。" />
        ) : (
          <div className="flex flex-col gap-2">
            {rows.map((row) => (
              <div
                key={row.skill_id}
                className="rounded-md border border-line bg-card-inner p-3"
              >
                <div className="flex min-w-0 items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="flex min-w-0 flex-wrap items-center gap-2">
                      <p className="truncate text-sm font-medium text-ink">{row.name}</p>
                      <span className="font-mono text-xs text-ink-2">{row.slug}</span>
                      <StatusPill tone="mute">{riskLevelLabel(row.risk_level)}</StatusPill>
                      <StatusPill tone="info">
                        依赖 MCP {row.mcp_deps.length}
                      </StatusPill>
                    </div>
                    {row.mcp_deps.length > 0 ? (
                      <p className="mt-2 text-xs text-ink-2">
                        该技能依赖{" "}
                        {row.mcp_deps
                          .map((d) => d.server_key || d.server_name)
                          .join("、")}
                        ：任务派发时会一并投影（若执行者已配齐所需环境变量）；未配齐则该次派发会被拦下并提示补齐。
                      </p>
                    ) : null}
                  </div>
                  <Button
                    aria-label={`移除技能 ${row.name}`}
                    disabled={disabled || isSaving}
                    size="icon"
                    type="button"
                    variant="ghost"
                    onClick={() => removeSkill(row.skill_id)}
                  >
                    <Trash2 />
                  </Button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </WorkSurface>
  );
}

function ProjectMcpBindingsSection({
  apiOptions,
  disabled,
  projectId
}: ProjectCapabilityBindingsPanelProps) {
  const queryClient = useQueryClient();
  const [selectedServerId, setSelectedServerId] = useState("");
  const [credentialEnvVar, setCredentialEnvVar] = useState("");
  const [draftRows, setDraftRows] = useState<McpDraftRow[] | null>(null);

  const bindingsQuery = useQuery({
    queryKey: ["project-mcp-bindings", projectId],
    queryFn: () => listProjectMcpBindings(apiOptions, projectId),
    placeholderData: keepPreviousData
  });
  const definitionsQuery = useQuery({
    queryKey: ["mcp-server-definitions"],
    queryFn: () => listMcpServerDefinitions(apiOptions),
    placeholderData: keepPreviousData
  });

  const serverRows = useMemo(
    () => (bindingsQuery.data ?? []).map(bindingToMcpRow),
    [bindingsQuery.data]
  );
  const rows = draftRows ?? serverRows;
  const isDirty = draftRows !== null && !rowsEqualMcp(draftRows, serverRows);

  const boundIds = new Set(rows.map((r) => r.mcp_server_id));
  const availableDefinitions = (definitionsQuery.data ?? []).filter(
    (d) => !boundIds.has(d.id)
  );

  const saveMutation = useMutation({
    mutationFn: (next: McpDraftRow[]) => {
      const items: CreateMcpBindingInput[] = next.map((row) => {
        const env = row.credential_env_var?.trim();
        return {
          mcp_server_id: row.mcp_server_id,
          ...(env ? { credential_env_var: env } : {})
        };
      });
      return putProjectMcpBindings(apiOptions, projectId, items);
    },
    onSuccess: async () => {
      setDraftRows(null);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ["project-mcp-bindings", projectId] }),
        queryClient.invalidateQueries({ queryKey: ["mcp-server-definitions"] })
      ]);
    }
  });

  function addBinding() {
    const definition = (definitionsQuery.data ?? []).find((d) => d.id === selectedServerId);
    if (!definition || boundIds.has(definition.id)) return;
    setDraftRows([...rows, definitionToMcpRow(definition, credentialEnvVar)]);
    setSelectedServerId("");
    setCredentialEnvVar("");
  }

  function removeBinding(serverId: string) {
    setDraftRows(rows.filter((r) => r.mcp_server_id !== serverId));
  }

  const isSaving = saveMutation.isPending;
  const canAdd = !disabled && !isSaving && selectedServerId.length > 0;
  const canSave = !disabled && !isSaving && isDirty;

  return (
    <WorkSurface data-testid="project-mcp-bindings-section">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line p-4">
        <div className="flex min-w-0 items-center gap-3">
          <IconTile tone="info" size="sm">
            <Network />
          </IconTile>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="font-semibold text-ink">项目 MCP</h3>
              {isDirty ? <StatusPill tone="warn">未保存</StatusPill> : null}
              {bindingsQuery.isFetching && !bindingsQuery.isLoading ? (
                <StatusPill tone="info">刷新中</StatusPill>
              ) : null}
            </div>
            <p className="mt-0.5 text-xs text-ink-2">
              同 server_key 时，项目绑定覆盖员工绑定。
            </p>
          </div>
        </div>
        <Button disabled={!canSave} type="button" onClick={() => saveMutation.mutate(rows)}>
          <Save data-icon="inline-start" />
          保存 MCP 绑定
        </Button>
      </div>

      <div className="flex flex-col gap-4 p-4">
        {isDirty ? (
          <Callout
            tone="warn"
            title="保存将整体替换项目 MCP 绑定"
            description="当前列表会声明式全量写入。未保存前刷新页面不会改动已有绑定。"
          />
        ) : null}
        {saveMutation.isError ? (
          <Callout
            tone="danger"
            title="MCP 绑定保存失败"
            description={saveMutation.error.message}
          />
        ) : null}

        <div className="grid gap-3 lg:grid-cols-2">
          <div className="min-w-0 space-y-2">
            <Label htmlFor="project-mcp-server">注册表 MCP</Label>
            <Select
              disabled={disabled || isSaving}
              onValueChange={setSelectedServerId}
              value={selectedServerId}
            >
              <SelectTrigger aria-label="注册表 MCP" className="w-full" id="project-mcp-server">
                <SelectValue placeholder="选择已注册的 MCP" />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {availableDefinitions.map((definition) => (
                    <SelectItem key={definition.id} value={definition.id}>
                      {definition.name}（{definition.server_key}）
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
          {selectedServerId ? (
            <div className="min-w-0 space-y-2">
              <Label htmlFor="project-mcp-credential-env">凭据环境变量（可选）</Label>
              <Input
                disabled={disabled || isSaving}
                id="project-mcp-credential-env"
                placeholder="例如 GITHUB_TOKEN"
                value={credentialEnvVar}
                onChange={(event) => setCredentialEnvVar(event.target.value)}
              />
            </div>
          ) : null}
        </div>
        <Button
          className={cn("w-full sm:w-fit")}
          disabled={!canAdd}
          type="button"
          variant="outline"
          onClick={addBinding}
        >
          <Plus data-icon="inline-start" />
          添加到绑定列表
        </Button>
        {definitionsQuery.isError ? <ErrorState title="MCP 注册表加载失败" /> : null}

        {bindingsQuery.isLoading ? (
          <LoadingState label="MCP 绑定加载中" />
        ) : bindingsQuery.isError ? (
          <ErrorState title="MCP 绑定加载失败" />
        ) : rows.length === 0 ? (
          <EmptyState title="暂无 MCP 绑定" description="从注册表添加后保存，即可作为项目公共 MCP 供给。" />
        ) : (
          <div className="flex flex-col gap-2">
            {rows.map((row) => (
              <div
                key={row.mcp_server_id}
                className="flex min-w-0 items-center justify-between gap-3 rounded-md border border-line bg-card-inner p-3"
              >
                <div className="flex min-w-0 items-center gap-3">
                  <IconTile tone="info" size="sm">
                    <Network />
                  </IconTile>
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium text-ink">{row.server_name}</p>
                    <p className="truncate font-mono text-xs text-ink-2">
                      {row.server_key}
                      {row.transport ? ` · ${row.transport}` : ""}
                    </p>
                    {row.credential_env_var ? (
                      <p className="mt-1 flex items-center gap-1 font-mono text-xs text-ink-2">
                        <KeyRound className="size-3 shrink-0" />
                        {row.credential_env_var}
                      </p>
                    ) : (
                      <p className="mt-1 text-xs text-ink-3">无凭据环境变量</p>
                    )}
                  </div>
                </div>
                <Button
                  aria-label={`移除 MCP ${row.server_name}`}
                  disabled={disabled || isSaving}
                  size="icon"
                  type="button"
                  variant="ghost"
                  onClick={() => removeBinding(row.mcp_server_id)}
                >
                  <Trash2 />
                </Button>
              </div>
            ))}
          </div>
        )}
      </div>
    </WorkSurface>
  );
}


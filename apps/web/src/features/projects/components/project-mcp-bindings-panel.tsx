import { KeyRound, Network, Plus, Save, Trash2 } from "lucide-react";
import { useState } from "react";
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
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
} from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  listMcpServerDefinitions,
  listProjectMcpBindings,
  putProjectMcpBindings,
  type CreateMcpBindingInput,
  type McpBinding,
} from "@/lib/api/capabilities";

type ProjectMcpBindingsPanelProps = {
  apiOptions: ApiClientOptions;
  disabled?: boolean;
  projectId: string;
};

/** 面板内的待保存绑定行：来源可以是服务端已存在绑定，也可以是注册表新加项。 */
type BindingRow = {
  credential_env_var?: string;
  mcp_server_id: string;
  server_key: string;
  server_name: string;
  transport?: string;
};

function bindingToRow(binding: McpBinding): BindingRow {
  return {
    credential_env_var: binding.credential_env_var,
    mcp_server_id: binding.mcp_server_id,
    server_key: binding.server_key ?? binding.mcp_server_id,
    server_name: binding.server_name ?? binding.server_key ?? binding.mcp_server_id,
    transport: binding.transport,
  };
}

function rowsToPutItems(rows: BindingRow[]): CreateMcpBindingInput[] {
  return rows.map((row) => {
    const credentialEnvVar = row.credential_env_var?.trim();
    return {
      mcp_server_id: row.mcp_server_id,
      ...(credentialEnvVar ? { credential_env_var: credentialEnvVar } : {}),
    };
  });
}

export function ProjectMcpBindingsPanel({
  apiOptions,
  disabled,
  projectId,
}: ProjectMcpBindingsPanelProps) {
  const queryClient = useQueryClient();
  const [selectedServerId, setSelectedServerId] = useState("");
  const [credentialEnvVar, setCredentialEnvVar] = useState("");
  // null 表示未做本地编辑，直接展示服务端绑定；一旦增删即进入草稿态，保存后回落。
  const [draftRows, setDraftRows] = useState<BindingRow[] | null>(null);

  const bindingsQuery = useQuery({
    queryKey: ["project-mcp-bindings", projectId],
    queryFn: () => listProjectMcpBindings(apiOptions, projectId),
    placeholderData: keepPreviousData,
  });
  const definitionsQuery = useQuery({
    queryKey: ["mcp-server-definitions"],
    queryFn: () => listMcpServerDefinitions(apiOptions),
    placeholderData: keepPreviousData,
  });

  const rows = draftRows ?? (bindingsQuery.data ?? []).map(bindingToRow);
  const isDirty = draftRows !== null;

  const boundServerIds = new Set(rows.map((row) => row.mcp_server_id));
  const availableDefinitions = (definitionsQuery.data ?? []).filter(
    (definition) => !boundServerIds.has(definition.id),
  );

  const saveMutation = useMutation({
    mutationFn: (nextRows: BindingRow[]) =>
      putProjectMcpBindings(apiOptions, projectId, rowsToPutItems(nextRows)),
    onSuccess: async () => {
      setDraftRows(null);
      await queryClient.invalidateQueries({
        queryKey: ["project-mcp-bindings", projectId],
      });
    },
  });

  const isSaving = saveMutation.isPending;

  function addBinding() {
    const definition = (definitionsQuery.data ?? []).find(
      (candidate) => candidate.id === selectedServerId,
    );
    if (!definition || boundServerIds.has(definition.id)) return;
    const trimmedEnvVar = credentialEnvVar.trim();
    setDraftRows([
      ...rows,
      {
        ...(trimmedEnvVar ? { credential_env_var: trimmedEnvVar } : {}),
        mcp_server_id: definition.id,
        server_key: definition.server_key,
        server_name: definition.name,
        transport: definition.transport,
      },
    ]);
    setSelectedServerId("");
    setCredentialEnvVar("");
  }

  function removeBinding(serverId: string) {
    setDraftRows(rows.filter((row) => row.mcp_server_id !== serverId));
  }

  const canAdd = !disabled && !isSaving && selectedServerId.length > 0;
  const canSave = !disabled && !isSaving && isDirty;

  return (
    <WorkSurface data-testid="project-mcp-bindings-panel">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-v3-line p-4">
        <div className="flex min-w-0 items-center gap-3">
          <IconTile tone="info" size="sm">
            <Network />
          </IconTile>
          <div className="min-w-0">
            <h3 className="font-semibold text-v3-ink">MCP 绑定</h3>
            <p className="mt-0.5 text-xs text-v3-ink-2">
              同 server_key 时，项目绑定覆盖员工绑定。
            </p>
          </div>
        </div>
        <div className="flex items-center gap-2">
          {isDirty ? <StatusPill tone="warn">未保存</StatusPill> : null}
          <StatusPill tone="mute">{rows.length} 个绑定</StatusPill>
          <V3Button
            disabled={!canSave}
            type="button"
            onClick={() => saveMutation.mutate(rows)}
          >
            <Save data-icon="inline-start" />
            {isSaving ? "保存中" : "保存绑定"}
          </V3Button>
        </div>
      </div>

      <div className="flex flex-col gap-4 p-4">
        {isDirty ? (
          <Alert className="border-v3-info/30 bg-v3-info-soft text-v3-ink">
            <Network className="text-v3-info" />
            <AlertTitle>绑定变更尚未保存</AlertTitle>
            <AlertDescription>
              保存时会以当前列表全量替换项目 MCP 绑定；清空列表并保存即移除全部绑定。
            </AlertDescription>
          </Alert>
        ) : null}
        {saveMutation.isError ? (
          <Alert variant="destructive">
            <AlertTitle>MCP 绑定保存失败</AlertTitle>
            <AlertDescription>{saveMutation.error.message}</AlertDescription>
          </Alert>
        ) : null}

        <div className="flex flex-col gap-3">
          <div className="grid gap-3 lg:grid-cols-2">
            <div className="min-w-0 space-y-2">
              <Label htmlFor="project-mcp-server">注册表 MCP</Label>
              <Select
                disabled={disabled || isSaving}
                onValueChange={setSelectedServerId}
                value={selectedServerId}
              >
                <SelectTrigger
                  aria-label="注册表 MCP"
                  className="w-full"
                  id="project-mcp-server"
                >
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
                  onChange={(event) => setCredentialEnvVar(event.target.value)}
                  placeholder="例如 GITHUB_TOKEN"
                  value={credentialEnvVar}
                />
              </div>
            ) : null}
          </div>
          <V3Button
            className="w-full sm:w-fit"
            disabled={!canAdd}
            onClick={addBinding}
            type="button"
            variant="outline"
          >
            <Plus data-icon="inline-start" />
            添加到绑定列表
          </V3Button>
          {definitionsQuery.isError ? (
            <V3ErrorState title="MCP 注册表加载失败" />
          ) : null}
        </div>

        {bindingsQuery.isLoading ? (
          <V3LoadingState label="MCP 绑定加载中" />
        ) : bindingsQuery.isError ? (
          <V3ErrorState title="MCP 绑定加载失败" />
        ) : rows.length === 0 ? (
          <V3EmptyState title="暂无 MCP 绑定" />
        ) : (
          <V3Table>
            <thead>
              <tr>
                <V3Th>服务</V3Th>
                <V3Th>Transport</V3Th>
                <V3Th>凭据环境变量</V3Th>
                <V3Th className="text-right">操作</V3Th>
              </tr>
            </thead>
            <tbody>
              {rows.map((row) => (
                <V3Tr key={row.mcp_server_id}>
                  <V3Td>
                    <div className="flex min-w-0 items-center gap-3">
                      <IconTile tone="info" size="sm">
                        <Network />
                      </IconTile>
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium text-v3-ink">
                          {row.server_name}
                        </p>
                        <p className="truncate font-mono text-xs text-v3-ink-2">
                          {row.server_key}
                        </p>
                      </div>
                    </div>
                  </V3Td>
                  <V3Td className="font-mono text-xs text-v3-ink-2">
                    {row.transport ?? "—"}
                  </V3Td>
                  <V3Td>
                    {row.credential_env_var ? (
                      <span className="flex min-w-0 items-center gap-1 font-mono text-xs text-v3-ink-2">
                        <KeyRound className="size-3 shrink-0" />
                        <span className="truncate">{row.credential_env_var}</span>
                      </span>
                    ) : (
                      <span className="text-xs text-v3-ink-3">无</span>
                    )}
                  </V3Td>
                  <V3Td className="text-right">
                    <V3Button
                      aria-label={`移除 MCP ${row.server_name}`}
                      disabled={disabled || isSaving}
                      onClick={() => removeBinding(row.mcp_server_id)}
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
  );
}

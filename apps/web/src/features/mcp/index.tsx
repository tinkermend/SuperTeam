import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, KeyRound, Network, Plus, Trash2 } from "lucide-react";
import {
  MetricGrid,
  StatusPill,
  Button,
  EmptyState,
  ErrorState,
  LoadingState,
  MetricCard,
  DataTable,
  Td,
  Th,
  Tr,
  WorkSurface,
  type Tone,
  Callout
} from "@/components/superteam";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import { ApiRequestError } from "@/lib/api/client";
import {
  deleteMcpServerDefinition,
  listMcpServerDefinitions,
  listMcpServerDependentSkills,
  type McpServerDefinition
} from "@/lib/api/capabilities";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { statusLabel } from "@/lib/status-labels";
import { RegisterMcpDialog } from "./register-dialog";

type MetricTone = Tone;

export function McpManagementPage() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const queryClient = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<McpServerDefinition | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const definitions = useQuery({
    queryKey: ["mcp-server-definitions"],
    queryFn: () => listMcpServerDefinitions({ baseUrl: apiBaseUrl })
});

  const deleteMutation = useMutation({
    mutationFn: (serverId: string) =>
      deleteMcpServerDefinition({ baseUrl: apiBaseUrl }, serverId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["mcp-server-definitions"] });
      setDeleteError(null);
    },
    onError: (error: unknown) => {
      setDeleteError(
        error instanceof ApiRequestError && error.detail
          ? error.detail
          : error instanceof Error
            ? error.message
            : "删除 MCP 失败",
      );
    }
});

  const dependentSkills = useQuery({
    queryKey: ["mcp-dependent-skills", pendingDelete?.id],
    queryFn: () => listMcpServerDependentSkills({ baseUrl: apiBaseUrl }, pendingDelete!.id),
    enabled: pendingDelete !== null
});

  const rows = definitions.data ?? [];
  const metrics = useMemo(() => {
    const active = rows.filter((row) => row.status === "active").length;
    const requiresEnv = rows.filter((row) => row.required_env_vars.length > 0).length;
    return [
      {
        label: "MCP 总数",
        value: rows.length,
        iconTone: "info" as MetricTone,
        icon: <Network />
},
      {
        label: "活跃 MCP",
        value: active,
        iconTone: "ok" as MetricTone,
        icon: <CheckCircle2 />
},
      {
        label: "需要环境变量",
        value: requiresEnv,
        iconTone: "warn" as MetricTone,
        icon: <KeyRound />
},
    ];
  }, [rows]);

  const isInitialLoading = definitions.isPending && rows.length === 0;
  const isBlockingError = definitions.isError && rows.length === 0;

  return (
    <>
      <ShellPageHeader
        icon={<Network />}
        iconTone="brand"
        title="MCP 管理"
        subtitle="注册 HTTP/streamable HTTP 能力，绑定到团队或数字员工"
      />
      <Main width="wide" className="min-w-0 overflow-x-hidden">
        <div className="flex min-w-0 flex-col gap-6">
          <div className="flex flex-wrap items-center justify-start gap-2 sm:justify-end">
            <Button className="h-11 self-start px-5" onClick={() => setShowCreate(true)}>
              <Plus data-icon="inline-start" />
              注册 MCP
            </Button>
          </div>

          <MetricGrid aria-label="MCP 指标">
            {metrics.map((metric) => (
              <MetricCard
                key={metric.label}
                label={metric.label}
                value={metric.value}
                icon={metric.icon}
                iconTone={metric.iconTone}
              />
            ))}
          </MetricGrid>

          {deleteError ? (
            <Callout tone="danger" title="删除失败" description={deleteError} />
          ) : null}

          <WorkSurface className="min-w-0">
            {isInitialLoading ? (
              <LoadingState label="加载 MCP 定义…" />
            ) : isBlockingError ? (
              <ErrorState title="加载失败" description="无法加载 MCP 定义" />
            ) : rows.length === 0 ? (
              <EmptyState
                icon={<Network />}
                title="还没有注册 MCP"
                description="注册一个 HTTP/streamable HTTP MCP 能力后，可在团队或数字员工页面绑定。"
              />
            ) : (
              <DataTable>
                <thead>
                  <tr>
                    <Th>名称 / server_key</Th>
                    <Th>URL</Th>
                    <Th className="w-32">传输</Th>
                    <Th className="w-28">鉴权</Th>
                    <Th>必需环境变量</Th>
                    <Th className="w-24">状态</Th>
                    <Th className="w-14" aria-label="操作" />
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row: McpServerDefinition) => (
                    <McpDefinitionRow
                      key={row.id}
                      row={row}
                      onDelete={() => {
                        setDeleteError(null);
                        setPendingDelete(row);
                      }}
                    />
                  ))}
                </tbody>
              </DataTable>
            )}
          </WorkSurface>
        </div>
      </Main>

      <RegisterMcpDialog
        apiBaseUrl={apiBaseUrl}
        open={showCreate}
        onOpenChange={setShowCreate}
      />

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => {
          if (!open) setPendingDelete(null);
        }}
        title={`删除 MCP 定义 ${pendingDelete?.name ?? ""}`}
        desc={
          dependentSkills.data && dependentSkills.data.length > 0
            ? `该定义被 ${dependentSkills.data.length} 个技能依赖（${dependentSkills.data
                .map((skill) => skill.slug)
                .join("、")}），删除将被服务端拒绝。请先在技能详情页移除依赖。`
            : "删除后员工绑定与任务装载将不再包含该定义。此操作不可撤销。"
        }
        confirmText="删除"
        destructive
        disabled={dependentSkills.isLoading || (dependentSkills.data?.length ?? 0) > 0}
        isLoading={deleteMutation.isPending}
        handleConfirm={() => {
          if (pendingDelete) {
            deleteMutation.mutate(pendingDelete.id, {
              onSettled: () => setPendingDelete(null)
});
          }
        }}
      />
    </>
  );
}

function McpDefinitionRow({
  row,
  onDelete
}: {
  row: McpServerDefinition;
  onDelete: () => void;
}) {
  const tone: Tone = row.status === "active" ? "ok" : "mute";
  return (
    <Tr>
      <Td>
        <div className="flex min-w-0 flex-col">
          <span className="truncate font-medium">{row.name}</span>
          <span className="truncate font-mono text-xs text-muted-foreground">
            {row.server_key}
          </span>
        </div>
      </Td>
      <Td className="max-w-[24rem] truncate font-mono text-xs" title={row.url}>
        {row.url}
      </Td>
      <Td className="font-mono text-xs">{row.transport}</Td>
      <Td className="font-mono text-xs">{row.auth_strategy}</Td>
      <Td>
        {row.required_env_vars.length === 0 ? (
          <span className="text-xs text-muted-foreground">—</span>
        ) : (
          <div className="flex flex-wrap gap-1">
            {row.required_env_vars.map((name) => (
              <span
                key={name}
                className="inline-flex items-center gap-1 rounded border bg-muted px-1.5 py-0.5 font-mono text-[11px]"
              >
                <KeyRound className="size-3" />
                {name}
              </span>
            ))}
          </div>
        )}
      </Td>
      <Td>
        <StatusPill tone={tone}>{statusLabel(row.status)}</StatusPill>
      </Td>
      <Td>
        <Button
          variant="ghost"
          size="sm"
          aria-label={`删除 ${row.name}`}
          onClick={onDelete}
        >
          <Trash2 />
        </Button>
      </Td>
    </Tr>
  );
}

export default McpManagementPage;

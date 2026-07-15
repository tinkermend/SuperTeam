import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CheckCircle2, KeyRound, Network, Plus, Trash2 } from "lucide-react";
import {
  MetricGrid,
  StatusPill,
  V3Button,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3MetricCard,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
  type V3Tone,
} from "@/components/superteam";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import { ApiRequestError } from "@/lib/api/client";
import {
  deleteMcpServerDefinition,
  listMcpServerDefinitions,
  listMcpServerDependentSkills,
  type McpServerDefinition,
} from "@/lib/api/capabilities";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { RegisterMcpDialog } from "./register-dialog";

type MetricTone = V3Tone;

export function McpManagementPage() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const queryClient = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  const [pendingDelete, setPendingDelete] = useState<McpServerDefinition | null>(null);
  const [deleteError, setDeleteError] = useState<string | null>(null);

  const definitions = useQuery({
    queryKey: ["mcp-server-definitions"],
    queryFn: () => listMcpServerDefinitions({ baseUrl: apiBaseUrl }),
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
    },
  });

  const dependentSkills = useQuery({
    queryKey: ["mcp-dependent-skills", pendingDelete?.id],
    queryFn: () => listMcpServerDependentSkills({ baseUrl: apiBaseUrl }, pendingDelete!.id),
    enabled: pendingDelete !== null,
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
        icon: <Network />,
      },
      {
        label: "活跃 MCP",
        value: active,
        iconTone: "ok" as MetricTone,
        icon: <CheckCircle2 />,
      },
      {
        label: "需要环境变量",
        value: requiresEnv,
        iconTone: "warn" as MetricTone,
        icon: <KeyRound />,
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
            <V3Button className="h-11 self-start px-5" onClick={() => setShowCreate(true)}>
              <Plus data-icon="inline-start" />
              注册 MCP
            </V3Button>
          </div>

          <MetricGrid aria-label="MCP 指标">
            {metrics.map((metric) => (
              <V3MetricCard
                key={metric.label}
                label={metric.label}
                value={metric.value}
                icon={metric.icon}
                iconTone={metric.iconTone}
              />
            ))}
          </MetricGrid>

          {deleteError ? (
            <Alert variant="destructive">
              <AlertTitle>删除失败</AlertTitle>
              <AlertDescription>{deleteError}</AlertDescription>
            </Alert>
          ) : null}

          <WorkSurface className="min-w-0">
            {isInitialLoading ? (
              <V3LoadingState label="加载 MCP 定义…" />
            ) : isBlockingError ? (
              <V3ErrorState title="加载失败" description="无法加载 MCP 定义" />
            ) : rows.length === 0 ? (
              <V3EmptyState
                icon={<Network />}
                title="还没有注册 MCP"
                description="注册一个 HTTP/streamable HTTP MCP 能力后，可在团队或数字员工页面绑定。"
              />
            ) : (
              <V3Table>
                <thead>
                  <tr>
                    <V3Th>名称 / server_key</V3Th>
                    <V3Th>URL</V3Th>
                    <V3Th className="w-32">传输</V3Th>
                    <V3Th className="w-28">鉴权</V3Th>
                    <V3Th>必需环境变量</V3Th>
                    <V3Th className="w-24">状态</V3Th>
                    <V3Th className="w-14" aria-label="操作" />
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
              </V3Table>
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
              onSettled: () => setPendingDelete(null),
            });
          }
        }}
      />
    </>
  );
}

function McpDefinitionRow({
  row,
  onDelete,
}: {
  row: McpServerDefinition;
  onDelete: () => void;
}) {
  const tone: V3Tone = row.status === "active" ? "ok" : "mute";
  return (
    <V3Tr>
      <V3Td>
        <div className="flex min-w-0 flex-col">
          <span className="truncate font-medium">{row.name}</span>
          <span className="truncate font-mono text-xs text-muted-foreground">
            {row.server_key}
          </span>
        </div>
      </V3Td>
      <V3Td className="max-w-[24rem] truncate font-mono text-xs" title={row.url}>
        {row.url}
      </V3Td>
      <V3Td className="font-mono text-xs">{row.transport}</V3Td>
      <V3Td className="font-mono text-xs">{row.auth_strategy}</V3Td>
      <V3Td>
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
      </V3Td>
      <V3Td>
        <StatusPill tone={tone}>{row.status}</StatusPill>
      </V3Td>
      <V3Td>
        <V3Button
          variant="ghost"
          size="sm"
          aria-label={`删除 ${row.name}`}
          onClick={onDelete}
        >
          <Trash2 />
        </V3Button>
      </V3Td>
    </V3Tr>
  );
}

export default McpManagementPage;

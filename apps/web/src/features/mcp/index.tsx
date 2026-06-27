import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Network, Plus, Trash2, KeyRound } from "lucide-react";
import {
  StatusPill,
  V3Button,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3MetricCard,
  V3PageHeader,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
  type V3Tone,
} from "@/components/superteam";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import {
  createMcpServerDefinition,
  deleteMcpServerDefinition,
  listMcpServerDefinitions,
  type CreateMcpServerDefinitionInput,
  type McpServerDefinition,
} from "@/lib/api/capabilities";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

type MetricTone = V3Tone;

const EMPTY_FORM: CreateMcpServerDefinitionInput = {
  name: "",
  server_key: "",
  description: "",
  transport: "streamable_http",
  url: "",
  auth_strategy: "none",
  required_env_vars: [],
  risk_level: "medium",
};

export function McpManagementPage() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const queryClient = useQueryClient();
  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState<CreateMcpServerDefinitionInput>(EMPTY_FORM);
  const [requiredEnvInput, setRequiredEnvInput] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const definitions = useQuery({
    queryKey: ["mcp-server-definitions"],
    queryFn: () => listMcpServerDefinitions({ baseUrl: apiBaseUrl }),
  });

  const createMutation = useMutation({
    mutationFn: (input: CreateMcpServerDefinitionInput) =>
      createMcpServerDefinition({ baseUrl: apiBaseUrl }, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["mcp-server-definitions"] });
      setShowCreate(false);
      setForm(EMPTY_FORM);
      setRequiredEnvInput("");
      setFormError(null);
    },
    onError: (error: unknown) => {
      setFormError(error instanceof Error ? error.message : "创建 MCP 失败");
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (serverId: string) =>
      deleteMcpServerDefinition({ baseUrl: apiBaseUrl }, serverId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["mcp-server-definitions"] });
    },
  });

  const rows = definitions.data ?? [];
  const metrics = useMemo(() => {
    const active = rows.filter((row) => row.status === "active").length;
    const requiresEnv = rows.filter((row) => row.required_env_vars.length > 0).length;
    return [
      { label: "MCP 总数", value: rows.length, iconTone: "info" as MetricTone },
      { label: "活跃 MCP", value: active, iconTone: "ok" as MetricTone },
      { label: "需要环境变量", value: requiresEnv, iconTone: "warn" as MetricTone },
    ];
  }, [rows]);

  const addRequiredEnv = () => {
    const name = requiredEnvInput.trim();
    if (!name || form.required_env_vars?.includes(name)) {
      setRequiredEnvInput("");
      return;
    }
    setForm((prev) => ({
      ...prev,
      required_env_vars: [...(prev.required_env_vars ?? []), name],
    }));
    setRequiredEnvInput("");
  };

  const submitCreate = (event: React.FormEvent) => {
    event.preventDefault();
    createMutation.mutate(form);
  };

  const isInitialLoading = definitions.isPending && rows.length === 0;
  const isBlockingError = definitions.isError && rows.length === 0;

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main className="min-w-0 overflow-x-hidden">
        <div className="flex min-w-0 flex-col gap-6">
          <V3PageHeader
            icon={<Network />}
            iconTone="brand"
            title="MCP 管理"
            subtitle="注册 HTTP/streamable HTTP 能力，绑定到团队或数字员工"
            actions={
              <V3Button
                className="h-11 self-start px-5"
                onClick={() => setShowCreate((value) => !value)}
              >
                <Plus data-icon="inline-start" />
                注册 MCP
              </V3Button>
            }
          />

          <section
            className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3"
            aria-label="MCP 指标"
          >
            {metrics.map((metric) => (
              <V3MetricCard
                key={metric.label}
                label={metric.label}
                value={metric.value}
                iconTone={metric.iconTone}
              />
            ))}
          </section>

          {showCreate ? (
            <WorkSurface className="min-w-0">
              <form className="flex flex-col gap-4" onSubmit={submitCreate}>
                <h2 className="text-base font-semibold">注册新 MCP</h2>
                <div className="grid gap-4 md:grid-cols-2">
                  <label className="flex flex-col gap-1 text-sm">
                    <span className="text-muted-foreground">名称</span>
                    <input
                      className="h-10 rounded-md border bg-background px-3"
                      value={form.name}
                      onChange={(event) => setForm((prev) => ({ ...prev, name: event.target.value }))}
                      required
                    />
                  </label>
                  <label className="flex flex-col gap-1 text-sm">
                    <span className="text-muted-foreground">server_key（[A-Za-z0-9_-]）</span>
                    <input
                      className="h-10 rounded-md border bg-background px-3 font-mono"
                      value={form.server_key}
                      onChange={(event) =>
                        setForm((prev) => ({ ...prev, server_key: event.target.value }))
                      }
                      required
                      pattern="[A-Za-z0-9_-]+"
                    />
                  </label>
                  <label className="flex flex-col gap-1 text-sm">
                    <span className="text-muted-foreground">URL</span>
                    <input
                      className="h-10 rounded-md border bg-background px-3 font-mono"
                      value={form.url}
                      onChange={(event) => setForm((prev) => ({ ...prev, url: event.target.value }))}
                      required
                      type="url"
                    />
                  </label>
                  <label className="flex flex-col gap-1 text-sm">
                    <span className="text-muted-foreground">传输方式</span>
                    <select
                      className="h-10 rounded-md border bg-background px-3"
                      value={form.transport}
                      onChange={(event) =>
                        setForm((prev) => ({
                          ...prev,
                          transport: event.target.value as CreateMcpServerDefinitionInput["transport"],
                        }))
                      }
                    >
                      <option value="streamable_http">streamable_http</option>
                      <option value="http">http</option>
                    </select>
                  </label>
                  <label className="flex flex-col gap-1 text-sm">
                    <span className="text-muted-foreground">鉴权方式</span>
                    <select
                      className="h-10 rounded-md border bg-background px-3"
                      value={form.auth_strategy}
                      onChange={(event) =>
                        setForm((prev) => ({
                          ...prev,
                          auth_strategy:
                            event.target.value as CreateMcpServerDefinitionInput["auth_strategy"],
                        }))
                      }
                    >
                      <option value="none">none</option>
                      <option value="bearer_env">bearer_env</option>
                      <option value="headers_env">headers_env</option>
                    </select>
                  </label>
                  <label className="flex flex-col gap-1 text-sm">
                    <span className="text-muted-foreground">风险等级</span>
                    <select
                      className="h-10 rounded-md border bg-background px-3"
                      value={form.risk_level}
                      onChange={(event) =>
                        setForm((prev) => ({ ...prev, risk_level: event.target.value }))
                      }
                    >
                      <option value="low">low</option>
                      <option value="medium">medium</option>
                      <option value="high">high</option>
                    </select>
                  </label>
                </div>
                <div className="flex flex-col gap-2 text-sm">
                  <span className="text-muted-foreground">必需环境变量</span>
                  <div className="flex flex-wrap gap-2">
                    {(form.required_env_vars ?? []).map((name) => (
                      <span
                        key={name}
                        className="inline-flex items-center gap-1 rounded-md border bg-muted px-2 py-1 font-mono text-xs"
                      >
                        <KeyRound className="size-3" />
                        {name}
                        <button
                          type="button"
                          className="text-muted-foreground hover:text-foreground"
                          onClick={() =>
                            setForm((prev) => ({
                              ...prev,
                              required_env_vars: (prev.required_env_vars ?? []).filter(
                                (value) => value !== name,
                              ),
                            }))
                          }
                        >
                          ×
                        </button>
                      </span>
                    ))}
                  </div>
                  <div className="flex gap-2">
                    <input
                      className="h-10 flex-1 rounded-md border bg-background px-3 font-mono"
                      placeholder="例如 GITHUB_TOKEN"
                      aria-label="必需环境变量输入"
                      value={requiredEnvInput}
                      onChange={(event) => setRequiredEnvInput(event.target.value)}
                      onKeyDown={(event) => {
                        if (event.key === "Enter") {
                          event.preventDefault();
                          addRequiredEnv();
                        }
                      }}
                    />
                    <V3Button type="button" variant="outline" onClick={addRequiredEnv}>
                      添加
                    </V3Button>
                  </div>
                </div>
                {formError ? (
                  <p className="text-sm text-destructive">{formError}</p>
                ) : null}
                <div className="flex gap-2">
                  <V3Button type="submit" disabled={createMutation.isPending}>
                    {createMutation.isPending ? "创建中…" : "创建"}
                  </V3Button>
                  <V3Button
                    type="button"
                    variant="ghost"
                    onClick={() => {
                      setShowCreate(false);
                      setForm(EMPTY_FORM);
                      setRequiredEnvInput("");
                      setFormError(null);
                    }}
                  >
                    取消
                  </V3Button>
                </div>
              </form>
            </WorkSurface>
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
                    <V3Th>名称</V3Th>
                    <V3Th>server_key</V3Th>
                    <V3Th>URL</V3Th>
                    <V3Th>传输</V3Th>
                    <V3Th>鉴权</V3Th>
                    <V3Th>必需环境变量</V3Th>
                    <V3Th>状态</V3Th>
                    <V3Th aria-label="操作" />
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row: McpServerDefinition) => (
                    <McpDefinitionRow
                      key={row.id}
                      row={row}
                      onDelete={() => deleteMutation.mutate(row.id)}
                      deleting={deleteMutation.isPending}
                    />
                  ))}
                </tbody>
              </V3Table>
            )}
          </WorkSurface>
        </div>
      </Main>
    </>
  );
}

function McpDefinitionRow({
  row,
  onDelete,
  deleting,
}: {
  row: McpServerDefinition;
  onDelete: () => void;
  deleting: boolean;
}) {
  const tone: V3Tone = row.status === "active" ? "ok" : "mute";
  return (
    <V3Tr>
      <V3Td className="font-medium">{row.name}</V3Td>
      <V3Td className="font-mono text-xs">{row.server_key}</V3Td>
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
        <V3Button variant="ghost" size="sm" onClick={onDelete} disabled={deleting}>
          <Trash2 />
        </V3Button>
      </V3Td>
    </V3Tr>
  );
}

export default McpManagementPage;

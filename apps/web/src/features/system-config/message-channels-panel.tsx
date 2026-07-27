import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, Cable, Copy, KeyRound, Plus, Power, RefreshCw, ShieldCheck } from "lucide-react";
import { Link } from "@tanstack/react-router";
import {
  Button,
  Callout,
  DataTable,
  EmptyState,
  ErrorState,
  LoadingState,
  SoftCard,
  SoftTabs,
  SoftTabsContent,
  SoftTabsList,
  SoftTabsTrigger,
  StatusPill,
  Td,
  Th,
  Tr,
  WorkSurface,
  type Tone,
} from "@/components/superteam";
import { ConfirmDialog } from "@/components/confirm-dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ApiRequestError } from "@/lib/api/client";
import {
  getFeishuChannelHealth,
  issueServiceToken,
  listFeishuAppConfigs,
  listServiceTokens,
  revokeServiceToken,
  setFeishuAppConfigStatus,
  upsertFeishuAppConfig,
  type FeishuAppConfig,
  type FeishuAppConfigStatus,
  type FeishuChannelHealth,
  type FeishuChannelHealthStatus,
  type FeishuConnectivityReport,
  type IssuedServiceToken,
  type ServiceToken,
} from "@/lib/api/channel-admin";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";

const FEISHU_CONNECTOR_SERVICE = "feishu-connector";

function statusTone(status: FeishuAppConfigStatus): Tone {
  switch (status) {
    case "active":
      return "ok";
    case "unverified":
      return "warn";
    case "disabled":
      return "mute";
    default:
      return "mute";
  }
}

function statusLabel(status: FeishuAppConfigStatus): string {
  switch (status) {
    case "active":
      return "可用";
    case "unverified":
      return "未验证";
    case "disabled":
      return "已停用";
    default:
      return status;
  }
}

function tokenStatusLabel(status: ServiceToken["status"]): string {
  return status === "active" ? "有效" : "已吊销";
}

function formatTime(value?: string | null): string {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

function healthTone(status: FeishuChannelHealthStatus): Tone {
  switch (status) {
    case "healthy":
      return "ok";
    case "stale":
      return "warn";
    case "missing":
      return "danger";
    default:
      return "mute";
  }
}

function healthLabel(status: FeishuChannelHealthStatus): string {
  switch (status) {
    case "healthy":
      return "健康";
    case "stale":
      return "心跳超时";
    case "missing":
      return "未上报";
    default:
      return status;
  }
}

function healthSummary(health?: FeishuChannelHealth): string {
  if (!health) return "加载中…";
  if (health.status === "missing") {
    return "尚未收到 connector 心跳。确认进程已启动并完成 bootstrap。";
  }
  const age =
    health.age_seconds != null ? `${health.age_seconds}s 前` : formatTime(health.last_heartbeat_at);
  const ws = health.apps
    .map((app) => `${app.app_id || "app"}:${app.ws_status}`)
    .join(" · ");
  const poll = health.last_outbox_poll_at
    ? `轮询 ${formatTime(health.last_outbox_poll_at)}`
    : "轮询尚未上报";
  const version = health.version ? `v${health.version}` : "版本未知";
  return [version, `心跳 ${age}`, poll, ws || "无 app 快照"].filter(Boolean).join(" · ");
}


function errorMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiRequestError && error.detail) return error.detail;
  if (error instanceof Error && error.message) return error.message;
  return fallback;
}

export type ChannelSection = "access" | "tokens";

type MessageChannelsPanelProps = {
  section?: ChannelSection;
  onSectionChange?: (section: ChannelSection) => void;
};

export function MessageChannelsPanel({
  section: sectionProp,
  onSectionChange,
}: MessageChannelsPanelProps = {}) {
  const [internalSection, setInternalSection] = useState<ChannelSection>("access");
  const section = sectionProp ?? internalSection;
  const setSection = (next: ChannelSection) => {
    if (onSectionChange) onSectionChange(next);
    else setInternalSection(next);
  };

  const apiBaseUrl = resolveControlPlaneUrl();
  const apiOptions = useMemo(() => ({ baseUrl: apiBaseUrl }), [apiBaseUrl]);
  const queryClient = useQueryClient();
  const [actionError, setActionError] = useState<string | null>(null);
  const [editOpen, setEditOpen] = useState(false);
  const [appId, setAppId] = useState("");
  const [appSecret, setAppSecret] = useState("");
  const [lastReport, setLastReport] = useState<FeishuConnectivityReport | null>(null);
  const [issuedToken, setIssuedToken] = useState<IssuedServiceToken | null>(null);
  const [pendingRevoke, setPendingRevoke] = useState<ServiceToken | null>(null);
  const [pendingDisable, setPendingDisable] = useState<FeishuAppConfig | null>(null);
  const [copied, setCopied] = useState(false);

  const configsQuery = useQuery({
    queryKey: ["channel-admin", "feishu-configs"],
    queryFn: () => listFeishuAppConfigs(apiOptions),
  });
  const tokensQuery = useQuery({
    queryKey: ["channel-admin", "service-tokens"],
    queryFn: () => listServiceTokens(apiOptions),
  });
  const healthQuery = useQuery({
    queryKey: ["channel-admin", "feishu-channel-health"],
    queryFn: () => getFeishuChannelHealth(apiOptions),
    refetchInterval: 15_000,
  });

  const invalidate = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["channel-admin", "feishu-configs"] }),
      queryClient.invalidateQueries({ queryKey: ["channel-admin", "service-tokens"] }),
      queryClient.invalidateQueries({ queryKey: ["channel-admin", "feishu-channel-health"] }),
    ]);
  };

  const upsertMutation = useMutation({
    mutationFn: () =>
      upsertFeishuAppConfig(apiOptions, {
        app_id: appId.trim(),
        app_secret: appSecret,
      }),
    onSuccess: async (result) => {
      setLastReport(result.verify);
      setAppSecret("");
      setActionError(null);
      if (result.verify.ok) {
        setEditOpen(false);
      }
      await invalidate();
    },
    onError: (error) => setActionError(errorMessage(error, "保存飞书应用配置失败")),
  });

  const statusMutation = useMutation({
    mutationFn: (input: { id: string; status: FeishuAppConfigStatus }) =>
      setFeishuAppConfigStatus(apiOptions, input.id, input.status),
    onSuccess: async () => {
      setPendingDisable(null);
      setActionError(null);
      await invalidate();
    },
    onError: (error) => setActionError(errorMessage(error, "更新通道状态失败")),
  });

  const issueMutation = useMutation({
    mutationFn: () => issueServiceToken(apiOptions, FEISHU_CONNECTOR_SERVICE),
    onSuccess: async (token) => {
      setIssuedToken(token);
      setActionError(null);
      await invalidate();
    },
    onError: (error) => setActionError(errorMessage(error, "签发服务凭据失败")),
  });

  const revokeMutation = useMutation({
    mutationFn: (tokenId: string) => revokeServiceToken(apiOptions, tokenId),
    onSuccess: async () => {
      setPendingRevoke(null);
      setActionError(null);
      await invalidate();
    },
    onError: (error) => setActionError(errorMessage(error, "吊销服务凭据失败")),
  });


  const configs = configsQuery.data ?? [];
  const tokens = tokensQuery.data ?? [];
  const health = healthQuery.data;
  const connectorTokens = tokens.filter((token) => token.service_name === FEISHU_CONNECTOR_SERVICE);
  const isInitialLoading =
    (configsQuery.isPending && configs.length === 0) ||
    (tokensQuery.isPending && tokens.length === 0);
  const isBlockingError =
    (configsQuery.isError && configs.length === 0) || (tokensQuery.isError && tokens.length === 0);

  const healthForConfig = (config: FeishuAppConfig): { tone: Tone; label: string; detail: string } => {
    if (config.status === "disabled") {
      return { tone: "mute", label: "已停用", detail: "不再下发" };
    }
    if (config.status === "unverified") {
      return { tone: "warn", label: "未验证", detail: "待补权限/范围后重存" };
    }
    if (!health) {
      return { tone: "mute", label: "检测中", detail: "读取 connector 心跳…" };
    }
    const app = health.apps.find((item) => item.app_id === config.app_id || item.config_id === config.id);
    if (health.status === "healthy") {
      const ws = app?.ws_status ?? "unknown";
      const wsLabel =
        ws === "connected" ? "长连接正常" : ws === "reconnecting" ? "长连接重连中" : `长连接 ${ws}`;
      return { tone: "ok", label: "健康", detail: wsLabel };
    }
    if (health.status === "stale") {
      return {
        tone: "warn",
        label: "心跳超时",
        detail: health.age_seconds != null ? `已 ${health.age_seconds}s 未上报` : "connector 可能宕机",
      };
    }
    return { tone: "danger", label: "未上报", detail: "尚未收到 connector 心跳" };
  };

  const openCreate = () => {
    setAppId(configs[0]?.app_id ?? "");
    setAppSecret("");
    setLastReport(null);
    setActionError(null);
    setEditOpen(true);
  };

  const openRotate = (config: FeishuAppConfig) => {
    setAppId(config.app_id);
    setAppSecret("");
    setLastReport(null);
    setActionError(null);
    setEditOpen(true);
  };

  const copyIssued = async () => {
    if (!issuedToken?.token) return;
    try {
      await navigator.clipboard.writeText(issuedToken.token);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      setActionError("复制失败，请手动选中明文凭据复制");
    }
  };

  return (
    <div className="flex min-w-0 flex-col gap-4">
      {actionError ? <Callout tone="danger" title="操作失败" description={actionError} /> : null}

      <SoftCard
        className={`min-w-0 p-3 ${
          health?.status === "stale" || health?.status === "missing"
            ? "border border-warn/40 bg-warn/5"
            : ""
        }`}
      >
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <Activity className="size-4 shrink-0 text-ink-2" />
            <span className="text-sm font-semibold text-ink">Connector</span>
            {health ? (
              <StatusPill tone={healthTone(health.status)}>{healthLabel(health.status)}</StatusPill>
            ) : healthQuery.isError ? (
              <StatusPill tone="danger">读取失败</StatusPill>
            ) : (
              <StatusPill tone="mute">检测中</StatusPill>
            )}
            <span className="min-w-0 text-xs text-ink-2">{healthSummary(health)}</span>
          </div>
          <Button
            disabled={healthQuery.isFetching}
            onClick={() => void healthQuery.refetch()}
            size="sm"
            type="button"
            variant="outline"
          >
            <RefreshCw data-icon="inline-start" />
            刷新
          </Button>
        </div>
        {healthQuery.isError ? (
          <p className="mt-2 text-xs text-danger">无法读取通道健康，请确认管理员权限与控制平面状态。</p>
        ) : null}
      </SoftCard>

      <SoftTabs
        className="flex w-full min-w-0 flex-col gap-3"
        onValueChange={(value) => setSection(value as ChannelSection)}
        value={section}
      >
        <div className="w-full min-w-0 max-w-full overflow-x-auto overflow-y-hidden pb-0.5">
          <SoftTabsList aria-label="消息通道分区">
            <SoftTabsTrigger value="access">通道接入</SoftTabsTrigger>
            <SoftTabsTrigger value="tokens">服务凭据</SoftTabsTrigger>
          </SoftTabsList>
        </div>

        <SoftTabsContent value="access" className="min-w-0">
          <WorkSurface className="min-w-0">
            <div className="mb-0 flex flex-wrap items-start justify-between gap-3 border-b border-line px-4 py-3">
              <div className="min-w-0">
                <h2 className="text-sm font-semibold text-ink">飞书应用</h2>
                <p className="mt-0.5 text-xs text-ink-2">
                  租户级 App 凭据。保存时真实探测 token、通讯录反查与机器人能力；Secret 只写不回显。
                  投递失败台账见{" "}
                  <Link
                    className="font-medium text-brand underline-offset-2 hover:underline"
                    to="/logs/delivery"
                  >
                    日志管理 · 消息投递
                  </Link>
                  。
                </p>
              </div>
              <Button onClick={openCreate} size="sm" type="button">
                <Plus data-icon="inline-start" />
                {configs.length === 0 ? "接入飞书应用" : "轮换 / 新增应用"}
              </Button>
            </div>

            {isInitialLoading ? (
              <div className="p-4">
                <LoadingState label="加载消息通道…" />
              </div>
            ) : isBlockingError ? (
              <div className="p-4">
                <ErrorState title="加载失败" description="无法加载通道配置，请确认管理员权限" />
              </div>
            ) : configs.length === 0 ? (
              <div className="p-4">
                <EmptyState
                  icon={<Cable />}
                  title="尚未接入飞书应用"
                  description="填入开放平台的 App ID / App Secret，保存时会做连通自检。"
                  action={
                    <Button onClick={openCreate} size="sm" type="button">
                      接入飞书应用
                    </Button>
                  }
                />
              </div>
            ) : (
              <DataTable>
                <thead>
                  <tr>
                    <Th>通道</Th>
                    <Th>App ID</Th>
                    <Th className="w-28">状态</Th>
                    <Th className="w-36">连接</Th>
                    <Th className="w-44" aria-label="操作" />
                  </tr>
                </thead>
                <tbody>
                  {configs.map((config) => {
                    const row = healthForConfig(config);
                    return (
                      <Tr key={config.id}>
                        <Td>
                          <div className="font-medium text-ink">飞书</div>
                          <div className="text-xs text-ink-2">IM 投影通道</div>
                        </Td>
                        <Td className="font-mono text-sm">{config.app_id}</Td>
                        <Td>
                          <StatusPill tone={statusTone(config.status)}>
                            {statusLabel(config.status)}
                          </StatusPill>
                        </Td>
                        <Td>
                          <div className="min-w-0">
                            <StatusPill tone={row.tone}>{row.label}</StatusPill>
                            <div className="mt-1 text-xs text-ink-2">{row.detail}</div>
                          </div>
                        </Td>
                        <Td>
                          <div className="flex flex-wrap gap-2">
                            <Button
                              onClick={() => openRotate(config)}
                              size="sm"
                              type="button"
                              variant="outline"
                            >
                              <RefreshCw data-icon="inline-start" />
                              轮换 Secret
                            </Button>
                            {config.status === "disabled" ? (
                              <Button
                                disabled={statusMutation.isPending}
                                onClick={() =>
                                  statusMutation.mutate({ id: config.id, status: "active" })
                                }
                                size="sm"
                                type="button"
                                variant="outline"
                              >
                                <Power data-icon="inline-start" />
                                重新启用
                              </Button>
                            ) : (
                              <Button
                                disabled={statusMutation.isPending}
                                onClick={() => setPendingDisable(config)}
                                size="sm"
                                type="button"
                                variant="outline"
                              >
                                <Power data-icon="inline-start" />
                                停用
                              </Button>
                            )}
                          </div>
                        </Td>
                      </Tr>
                    );
                  })}
                </tbody>
              </DataTable>
            )}
          </WorkSurface>
        </SoftTabsContent>


        <SoftTabsContent value="tokens" className="min-w-0">
          <WorkSurface className="min-w-0">
            <div className="mb-0 flex flex-wrap items-start justify-between gap-3 border-b border-line px-4 py-3">
              <div className="min-w-0">
                <h2 className="text-sm font-semibold text-ink">Connector 服务凭据</h2>
                <p className="mt-0.5 text-xs text-ink-2">
                  供 feishu-connector 进程认证。明文只在签发瞬间展示一次；部署侧写入{" "}
                  <code className="rounded bg-card-soft px-1">FEISHU_CONNECTOR_TOKEN</code>
                  （dev 也可用{" "}
                  <code className="rounded bg-card-soft px-1">
                    .scratch/dev-services/feishu-connector.token
                  </code>
                  ）。
                </p>
              </div>
              <Button
                disabled={issueMutation.isPending}
                onClick={() => issueMutation.mutate()}
                size="sm"
                type="button"
              >
                <KeyRound data-icon="inline-start" />
                签发新凭据
              </Button>
            </div>

            {issuedToken ? (
              <SoftCard className="m-3 border border-warn/40 bg-warn/5 p-3">
                <div className="mb-2 flex items-center gap-2 text-sm font-medium text-ink">
                  <ShieldCheck className="size-4" />
                  新凭据已签发（只显示一次）
                </div>
                <p className="mb-2 text-xs text-ink-2">
                  请立即复制并更新 connector 部署环境，然后吊销旧凭据。关闭此页后无法再查看明文。
                </p>
                <div className="flex flex-wrap items-center gap-2">
                  <code className="max-w-full break-all rounded bg-card-soft px-2 py-1 text-xs">
                    {issuedToken.token}
                  </code>
                  <Button onClick={() => void copyIssued()} size="sm" type="button" variant="outline">
                    <Copy data-icon="inline-start" />
                    {copied ? "已复制" : "复制"}
                  </Button>
                  <Button onClick={() => setIssuedToken(null)} size="sm" type="button" variant="ghost">
                    我已保存，关闭
                  </Button>
                </div>
              </SoftCard>
            ) : null}

            {tokensQuery.isPending && connectorTokens.length === 0 ? (
              <div className="p-4">
                <LoadingState label="加载服务凭据…" />
              </div>
            ) : tokensQuery.isError && connectorTokens.length === 0 ? (
              <div className="p-4">
                <ErrorState title="加载失败" description="无法加载服务凭据" />
              </div>
            ) : connectorTokens.length === 0 ? (
              <div className="p-4">
                <EmptyState
                  icon={<KeyRound />}
                  title="还没有服务凭据"
                  description="签发后把明文写入 connector 环境变量，进程用 Bearer + X-Service-Name 认证。"
                />
              </div>
            ) : (
              <DataTable>
                <thead>
                  <tr>
                    <Th>服务名</Th>
                    <Th className="w-28">状态</Th>
                    <Th className="w-40">签发时间</Th>
                    <Th className="w-40">最近使用</Th>
                    <Th className="w-24" aria-label="操作" />
                  </tr>
                </thead>
                <tbody>
                  {connectorTokens.map((token) => (
                    <Tr key={token.id}>
                      <Td className="font-mono text-sm">{token.service_name}</Td>
                      <Td>
                        <StatusPill tone={token.status === "active" ? "ok" : "mute"}>
                          {tokenStatusLabel(token.status)}
                        </StatusPill>
                      </Td>
                      <Td className="tabular-nums text-sm text-ink-2">
                        {formatTime(token.created_at)}
                      </Td>
                      <Td className="tabular-nums text-sm text-ink-2">
                        {formatTime(token.last_used_at)}
                      </Td>
                      <Td>
                        {token.status === "active" ? (
                          <Button
                            onClick={() => setPendingRevoke(token)}
                            size="sm"
                            type="button"
                            variant="outline"
                          >
                            吊销
                          </Button>
                        ) : (
                          <span className="text-xs text-ink-2">{formatTime(token.revoked_at)}</span>
                        )}
                      </Td>
                    </Tr>
                  ))}
                </tbody>
              </DataTable>
            )}
          </WorkSurface>
        </SoftTabsContent>
      </SoftTabs>

      <Dialog
        open={editOpen}
        onOpenChange={(open) => {
          if (!open) {
            setEditOpen(false);
            setAppSecret("");
          }
        }}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>飞书应用凭据</DialogTitle>
            <DialogDescription>
              保存时会真实请求飞书：取 token、探通讯录反查、探机器人接口。失败仍可保存为「未验证」。
            </DialogDescription>
          </DialogHeader>
          <div className="flex flex-col gap-3">
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="feishu-app-id">App ID</Label>
              <Input
                id="feishu-app-id"
                autoComplete="off"
                onChange={(event) => setAppId(event.target.value)}
                placeholder="cli_xxx"
                value={appId}
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <Label htmlFor="feishu-app-secret">App Secret</Label>
              <Input
                id="feishu-app-secret"
                autoComplete="off"
                onChange={(event) => setAppSecret(event.target.value)}
                placeholder="只写不回显"
                type="password"
                value={appSecret}
              />
            </div>
            {lastReport ? <ConnectivityReportView report={lastReport} /> : null}
          </div>
          <DialogFooter>
            <Button
              onClick={() => {
                setEditOpen(false);
                setAppSecret("");
              }}
              type="button"
              variant="ghost"
            >
              取消
            </Button>
            <Button
              disabled={upsertMutation.isPending || !appId.trim() || !appSecret}
              onClick={() => upsertMutation.mutate()}
              type="button"
            >
              {upsertMutation.isPending ? "保存并自检…" : "保存并自检"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={pendingDisable !== null}
        onOpenChange={(open) => {
          if (!open) setPendingDisable(null);
        }}
        title="停用飞书通道"
        desc={
          pendingDisable
            ? `停用后 connector 启动将不再拉取 App ${pendingDisable.app_id}，飞书通知会暂停，直到重新启用并完成自检。`
            : ""
        }
        confirmText="停用"
        isLoading={statusMutation.isPending}
        handleConfirm={() => {
          if (pendingDisable) {
            statusMutation.mutate({ id: pendingDisable.id, status: "disabled" });
          }
        }}
      />

      <ConfirmDialog
        open={pendingRevoke !== null}
        onOpenChange={(open) => {
          if (!open) setPendingRevoke(null);
        }}
        title="吊销服务凭据"
        desc="吊销后使用该明文的 connector 进程将无法认证。请确认已切换到新凭据。"
        confirmText="吊销"
        isLoading={revokeMutation.isPending}
        handleConfirm={() => {
          if (pendingRevoke) {
            revokeMutation.mutate(pendingRevoke.id);
          }
        }}
      />
    </div>
  );
}


function ConnectivityReportView({ report }: { report: FeishuConnectivityReport }) {
  return (
    <SoftCard className={`p-4 ${report.ok ? "border border-ok/30 bg-ok/5" : "border border-warn/40 bg-warn/5"}`}>
      <div className="mb-2 font-medium text-ink">{report.summary}</div>
      <ul className="flex flex-col gap-2">
        {report.probes.map((probe) => (
          <li key={probe.key} className="rounded-md bg-card-soft px-3 py-2 text-sm">
            <div className="mb-1 flex items-center gap-2">
              <StatusPill tone={probe.ok ? "ok" : "danger"}>{probe.ok ? "通过" : "失败"}</StatusPill>
              <span className="font-medium text-ink">{probe.label}</span>
            </div>
            <p className="text-ink-2">{probe.hint}</p>
          </li>
        ))}
      </ul>
    </SoftCard>
  );
}

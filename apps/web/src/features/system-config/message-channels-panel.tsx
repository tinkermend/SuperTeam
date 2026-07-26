import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Cable, Copy, KeyRound, Plus, Power, RefreshCw, ShieldCheck } from "lucide-react";
import {
  Button,
  Callout,
  DataTable,
  EmptyState,
  ErrorState,
  LoadingState,
  SoftCard,
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
  issueServiceToken,
  listFeishuAppConfigs,
  listServiceTokens,
  revokeServiceToken,
  setFeishuAppConfigStatus,
  upsertFeishuAppConfig,
  type FeishuAppConfig,
  type FeishuAppConfigStatus,
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

function errorMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiRequestError && error.detail) return error.detail;
  if (error instanceof Error && error.message) return error.message;
  return fallback;
}

export function MessageChannelsPanel() {
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

  const invalidate = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["channel-admin", "feishu-configs"] }),
      queryClient.invalidateQueries({ queryKey: ["channel-admin", "service-tokens"] }),
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
  const connectorTokens = tokens.filter((token) => token.service_name === FEISHU_CONNECTOR_SERVICE);
  const isInitialLoading =
    (configsQuery.isPending && configs.length === 0) ||
    (tokensQuery.isPending && tokens.length === 0);
  const isBlockingError =
    (configsQuery.isError && configs.length === 0) || (tokensQuery.isError && tokens.length === 0);

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
    <div className="flex min-w-0 flex-col gap-6">
      {actionError ? <Callout tone="danger" title="操作失败" description={actionError} /> : null}

      <WorkSurface className="min-w-0">
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="text-base font-bold text-ink">飞书通道</h2>
            <p className="text-sm text-ink-2">
              租户级应用凭据。保存时会真实探测 token、通讯录反查与机器人能力；Secret 只写不回显。
            </p>
          </div>
          <Button onClick={openCreate} size="sm" type="button">
            <Plus data-icon="inline-start" />
            {configs.length === 0 ? "接入飞书应用" : "轮换 / 新增应用"}
          </Button>
        </div>

        {isInitialLoading ? (
          <LoadingState label="加载消息通道…" />
        ) : isBlockingError ? (
          <ErrorState title="加载失败" description="无法加载通道配置，请确认管理员权限" />
        ) : configs.length === 0 ? (
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
        ) : (
          <DataTable>
            <thead>
              <tr>
                <Th>通道</Th>
                <Th>App ID</Th>
                <Th className="w-28">状态</Th>
                <Th className="w-36">健康</Th>
                <Th className="w-44" aria-label="操作" />
              </tr>
            </thead>
            <tbody>
              {configs.map((config) => (
                <Tr key={config.id}>
                  <Td>
                    <div className="font-medium text-ink">飞书</div>
                    <div className="text-xs text-ink-2">IM 投影通道</div>
                  </Td>
                  <Td className="font-mono text-sm">{config.app_id}</Td>
                  <Td>
                    <StatusPill tone={statusTone(config.status)}>{statusLabel(config.status)}</StatusPill>
                  </Td>
                  <Td className="text-sm text-ink-2">
                    {config.status === "active"
                      ? "连通自检已通过"
                      : config.status === "unverified"
                        ? "待补权限/范围后重存"
                        : "已停用，不再下发"}
                  </Td>
                  <Td>
                    <div className="flex flex-wrap gap-2">
                      <Button onClick={() => openRotate(config)} size="sm" type="button" variant="outline">
                        <RefreshCw data-icon="inline-start" />
                        轮换 Secret
                      </Button>
                      {config.status === "disabled" ? (
                        <Button
                          disabled={statusMutation.isPending}
                          onClick={() => statusMutation.mutate({ id: config.id, status: "active" })}
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
              ))}
            </tbody>
          </DataTable>
        )}
      </WorkSurface>

      <WorkSurface className="min-w-0">
        <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="text-base font-bold text-ink">Connector 服务凭据</h2>
            <p className="text-sm text-ink-2">
              供 feishu-connector 进程认证。明文只在签发瞬间展示一次；部署侧写入{" "}
              <code className="rounded bg-card-soft px-1">FEISHU_CONNECTOR_TOKEN</code>
              （dev 也可用{" "}
              <code className="rounded bg-card-soft px-1">.scratch/dev-services/feishu-connector.token</code>
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
          <SoftCard className="mb-4 border border-warn/40 bg-warn/5 p-4">
            <div className="mb-2 flex items-center gap-2 font-medium text-ink">
              <ShieldCheck className="size-4" />
              新凭据已签发（只显示一次）
            </div>
            <p className="mb-3 text-sm text-ink-2">
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

        {connectorTokens.length === 0 ? (
          <EmptyState
            icon={<KeyRound />}
            title="还没有服务凭据"
            description="签发后把明文写入 connector 环境变量，进程用 Bearer + X-Service-Name 认证。"
          />
        ) : (
          <DataTable>
            <thead>
              <tr>
                <Th>服务名</Th>
                <Th className="w-28">状态</Th>
                <Th className="w-44">签发时间</Th>
                <Th className="w-44">最近使用</Th>
                <Th className="w-28" aria-label="操作" />
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
                  <Td className="tabular-nums text-sm text-ink-2">{formatTime(token.created_at)}</Td>
                  <Td className="tabular-nums text-sm text-ink-2">{formatTime(token.last_used_at)}</Td>
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

      <Dialog
        open={editOpen}
        onOpenChange={(open) => {
          if (!open) {
            setEditOpen(false);
            setAppSecret("");
          }
        }}
      >
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>飞书应用凭据</DialogTitle>
            <DialogDescription>
              保存时会真实请求飞书：取 token、探通讯录反查、探机器人接口。失败仍可保存为「未验证」。
            </DialogDescription>
          </DialogHeader>
          <form
            className="flex flex-col gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              if (!appId.trim() || !appSecret.trim()) {
                setActionError("App ID 与 App Secret 均必填");
                return;
              }
              upsertMutation.mutate();
            }}
          >
            <div className="flex flex-col gap-2">
              <Label htmlFor="feishu-app-id">App ID</Label>
              <Input
                id="feishu-app-id"
                onChange={(event) => setAppId(event.target.value)}
                placeholder="cli_…"
                value={appId}
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor="feishu-app-secret">App Secret</Label>
              <Input
                autoComplete="off"
                id="feishu-app-secret"
                onChange={(event) => setAppSecret(event.target.value)}
                placeholder="只写不回显；轮换时填新 Secret"
                type="password"
                value={appSecret}
              />
            </div>

            {lastReport ? <ConnectivityReportView report={lastReport} /> : null}

            <DialogFooter>
              <Button
                onClick={() => {
                  setEditOpen(false);
                  setAppSecret("");
                }}
                type="button"
                variant="outline"
              >
                取消
              </Button>
              <Button disabled={upsertMutation.isPending} type="submit">
                {upsertMutation.isPending ? "保存并检测中…" : "保存并连通自检"}
              </Button>
            </DialogFooter>
          </form>
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
        confirmText="确认停用"
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
        confirmText="确认吊销"
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

import { type ReactNode, useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSearch } from "@tanstack/react-router";
import {
  Activity,
  AlertTriangle,
  Ban,
  Check,
  Clock,
  Cpu,
  Download,
  Eye,
  EyeOff,
  FileClock,
  FileJson,
  RefreshCw,
  Server,
  ShieldCheck,
  Wifi
} from "lucide-react";
import {
  IconTile,
  MasterDetailLayout,
  SoftCard,
  StatusPill,
  Button,
  DetailSkeleton,
  EmptyState,
  ErrorState,
  MetricCard,
  DataTable,
  TableSkeleton,
  Td,
  Th,
  Tr,
  WorkSurface,
  type Tone,
  SoftTabs,
  SoftTabsList,
  SoftTabsTrigger,
  SoftTabsContent
} from "@/components/superteam";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle
} from "@/components/ui/alert-dialog";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from "@/components/ui/dialog";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle
} from "@/components/ui/sheet";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import {
  approveRuntimeEnrollment,
  getProviderNativeConfig,
  getRuntimeOverview,
  listProviderNativeConfigs,
  listRuntimeEnrollments,
  listRuntimeEvents,
  pullProviderNativeConfig,
  putProviderNativeConfig,
  rejectRuntimeEnrollment,
  type ProviderNativeConfigDetail,
  type ProviderNativeConfigListItem,
  type RuntimeEnrollment,
  type RuntimeEnrollmentStatus,
  type RuntimeEvent,
  type RuntimeEventSeverity,
  type RuntimeNodeResponse,
  type RuntimeProviderCapabilitySummary
} from "@/lib/api/runtime";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { statusLabel } from "@/lib/status-labels";
import { cn } from "@/lib/utils";
import { Input } from "@/components/ui/input";

type RuntimeEventFilters = {
  event_type?: string;
  limit: number;
  node_id?: string;
  offset: number;
  provider_type?: string;
  severity?: RuntimeEventSeverity;
};

const defaultEventFilters: RuntimeEventFilters = {
  limit: 50,
  offset: 0
};

const severityOptions: Array<{ label: string; value: RuntimeEventSeverity }> = [
  { label: "信息", value: "info" },
  { label: "成功", value: "success" },
  { label: "预警", value: "warning" },
  { label: "错误", value: "error" },
];

const severityLabel: Record<RuntimeEventSeverity, string> = {
  error: "错误",
  info: "信息",
  success: "成功",
  warning: "预警"
};

const severityTone: Record<RuntimeEventSeverity, Tone> = {
  error: "danger",
  info: "info",
  success: "ok",
  warning: "warn"
};

const enrollmentStatusLabel: Record<RuntimeEnrollmentStatus, string> = {
  approved: "已接入",
  pending: "待接入",
  rejected: "已拒绝",
  revoked: "已停用"
};

const enrollmentStatusTone: Record<RuntimeEnrollmentStatus, Tone> = {
  approved: "ok",
  pending: "warn",
  rejected: "danger",
  revoked: "mute"
};

const runtimeTabTriggerClass =
  "h-9 flex-none rounded-[10px] border-0 px-4 py-2 text-[13px] font-semibold text-ink-2 shadow-none transition-colors data-[state=active]:bg-brand-soft data-[state=active]:text-brand-deep data-[state=active]:shadow-none data-[state=inactive]:hover:bg-card-soft data-[state=inactive]:hover:text-ink";

export function RuntimeNodesPage() {
  const apiBaseUrl = resolveControlPlaneUrl();

  return <RuntimeNodesView apiBaseUrl={apiBaseUrl} />;
}

type RuntimeNodesViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

export function RuntimeNodesView({ apiBaseUrl, fetcher }: RuntimeNodesViewProps) {
  const queryClient = useQueryClient();
  const search = useSearch({ strict: false }) as { node?: string };
  const [activeTab, setActiveTab] = useState("overview");
  const [eventFilters, setEventFilters] = useState<RuntimeEventFilters>(() => ({
    ...defaultEventFilters,
    node_id: search.node || undefined
}));
  const [approveTarget, setApproveTarget] = useState<RuntimeEnrollment | null>(null);
  const [rejectTarget, setRejectTarget] = useState<RuntimeEnrollment | null>(null);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(search.node || null);
  const [rejectReason, setRejectReason] = useState("");

  const overview = useQuery({
    queryKey: ["runtime-overview"],
    queryFn: () => getRuntimeOverview({ baseUrl: apiBaseUrl, fetcher })
});

  const events = useQuery({
    queryKey: ["runtime-events", eventFilters],
    queryFn: () => listRuntimeEvents({ baseUrl: apiBaseUrl, fetcher, ...eventFilters }),
    enabled: activeTab === "events"
});

  const enrollments = useQuery({
    queryKey: ["runtime-enrollments"],
    queryFn: () => listRuntimeEnrollments({ baseUrl: apiBaseUrl, fetcher }),
    enabled: activeTab === "enrollments"
});

  useEffect(() => {
    setEventFilters((current) => {
      const nextNodeId = search.node || undefined;
      if (current.node_id === nextNodeId) {
        return current;
      }
      return {
        ...current,
        node_id: nextNodeId,
        offset: 0
      };
    });
  }, [search.node]);

  // Auto-select the query node or the first online node so the config panel is not empty on load.
  useEffect(() => {
    if (selectedNodeId) {
      return;
    }
    if (search.node) {
      setSelectedNodeId(search.node);
      return;
    }
    const nodes = overview.data?.nodes ?? [];
    if (nodes.length === 0) {
      return;
    }
    const preferred = nodes.find((node) => node.status === "online") ?? nodes[0];
    if (preferred) {
      setSelectedNodeId(preferred.node_id);
    }
  }, [overview.data?.nodes, search.node, selectedNodeId]);

  const invalidateRuntimeQueries = () => {
    void queryClient.invalidateQueries({ queryKey: ["runtime-overview"] });
    void queryClient.invalidateQueries({ queryKey: ["runtime-events"] });
    void queryClient.invalidateQueries({ queryKey: ["runtime-enrollments"] });
    void queryClient.invalidateQueries({ queryKey: ["runtime-nodes"] });
  };

  const approve = useMutation({
    mutationFn: (id: string) => approveRuntimeEnrollment({ baseUrl: apiBaseUrl, fetcher }, id),
    onSuccess: () => {
      setApproveTarget(null);
      invalidateRuntimeQueries();
    }
});

  const reject = useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) =>
      rejectRuntimeEnrollment({ baseUrl: apiBaseUrl, fetcher }, id, reason),
    onSuccess: () => {
      setRejectTarget(null);
      setRejectReason("");
      invalidateRuntimeQueries();
    }
});

  const openApproveDialog = (enrollment: RuntimeEnrollment) => {
    approve.reset();
    setApproveTarget(enrollment);
  };

  const openRejectDialog = (enrollment: RuntimeEnrollment) => {
    reject.reset();
    setRejectTarget(enrollment);
    setRejectReason("");
  };

  const filterOptions = useMemo(() => {
    const overviewData = overview.data;
    const eventItems = events.data?.items ?? [];
    const nodes = uniqueStrings([
      eventFilters.node_id,
      ...(overviewData?.nodes.map((node) => node.node_id) ?? []),
      ...eventItems.map((event) => event.node_id).filter(Boolean),
    ]);
    const providers = uniqueStrings([
      ...(overviewData?.provider_capabilities.map((capability) => capability.provider_type) ?? []),
      ...(overviewData?.nodes.flatMap((node) => node.supported_providers) ?? []),
      ...eventItems.map((event) => event.provider_type).filter(Boolean),
    ]);
    const eventTypes = uniqueStrings([
      ...(overviewData?.recent_events.map((event) => event.event_type) ?? []),
      ...eventItems.map((event) => event.event_type),
    ]);

    return { eventTypes, nodes, providers };
  }, [events.data?.items, overview.data]);

  const updateEventFilter = <Key extends keyof RuntimeEventFilters>(key: Key, value: RuntimeEventFilters[Key]) => {
    setEventFilters((current) => ({
      ...current,
      [key]: value || undefined,
      offset: 0
}));
  };

  const overviewData = overview.data;
  const recentEvents = overviewData?.recent_events ?? [];
  const eventItems = events.data?.items ?? [];
  const enrollmentItems = enrollments.data ?? overviewData?.pending_enrollments ?? [];
  const hasAppliedEventFilter = Boolean(
    eventFilters.event_type || eventFilters.severity || eventFilters.node_id || eventFilters.provider_type,
  );

  return (
    <>
      <ShellPageHeader
        icon={<Server />}
        iconTone="info"
        title="Runtime 节点"
        subtitle="运行节点接入、Provider 能力、事件审计和阻断信号的首屏视图。"
      />
      <Main width="wide">
        <div className="flex min-w-0 flex-col gap-5 text-ink">
          <div className="flex flex-wrap items-center justify-start gap-2 sm:justify-end">
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                void overview.refetch();
                void events.refetch();
              }}
            >
              <RefreshCw data-icon="inline-start" />
              刷新
            </Button>
          </div>
          {overview.isLoading && !overviewData ? (
            <WorkSurface className="p-4" data-testid="runtime-overview-skeleton">
              <DetailSkeleton />
            </WorkSurface>
          ) : null}

          {overview.isError ? (
            <WorkSurface className="p-4">
              <ErrorState
                title="Runtime 总览加载失败"
                description="请稍后重试，或检查 Control Plane Runtime API 是否可用。"
                onRetry={() => void overview.refetch()}
              />
            </WorkSurface>
          ) : null}

          {overviewData ? (
            <>
              <SummaryMetrics summary={overviewData.summary} />

              <SoftTabs value={activeTab} onValueChange={setActiveTab} className="gap-4">
                <div className="min-w-0 overflow-x-auto pb-1">
                  <SoftTabsList
                    aria-label="Runtime 管理视图"
                    className="h-auto w-max min-w-full max-w-none justify-start gap-1 overflow-visible rounded-[14px] bg-card p-1.5 text-ink shadow-card sm:min-w-0"
                  >
                    <SoftTabsTrigger className={runtimeTabTriggerClass} value="overview">
                      节点总览
                    </SoftTabsTrigger>
                    <SoftTabsTrigger className={runtimeTabTriggerClass} value="enrollments">
                      接入审批
                    </SoftTabsTrigger>
                    <SoftTabsTrigger className={runtimeTabTriggerClass} value="capabilities">
                      能力范围
                    </SoftTabsTrigger>
                    <SoftTabsTrigger className={runtimeTabTriggerClass} value="events">
                      事件审计
                    </SoftTabsTrigger>
                  </SoftTabsList>
                </div>

                <SoftTabsContent value="overview">
                  <MasterDetailLayout
                    narrowDetail="stack"
                    rail="md"
                    master={
                      <div className="flex min-w-0 flex-col gap-4">
                        <NodeInventoryPanel
                          nodes={overviewData.nodes}
                          selectedNodeId={selectedNodeId}
                          onSelectNode={setSelectedNodeId}
                        />
                        {overviewData.pending_enrollments.length > 0 ? (
                          <PendingEnrollmentPanel
                            enrollments={overviewData.pending_enrollments}
                            onApprove={openApproveDialog}
                            onReject={openRejectDialog}
                          />
                        ) : null}
                      </div>
                    }
                    detail={
                      <div className="flex min-w-0 flex-col gap-4">
                        {selectedNodeId ? (
                          <ProviderNativeConfigPanel
                            apiBaseUrl={apiBaseUrl}
                            fetcher={fetcher}
                            nodeId={selectedNodeId}
                            node={overviewData.nodes.find((n) => n.node_id === selectedNodeId)}
                          />
                        ) : (
                          <SoftCard className="p-4">
                            <EmptyState
                              title="选择左侧节点"
                              description="选中节点后可管理其 host 侧模型 / 供应商原生配置。"
                            />
                          </SoftCard>
                        )}
                        <RecentEventsPanel events={recentEvents} compact />
                      </div>
                    }
                  />
                </SoftTabsContent>

                <SoftTabsContent value="enrollments">
                  <PendingEnrollmentPanel
                    enrollments={enrollmentItems}
                    isError={enrollments.isError}
                    isLoading={enrollments.isLoading}
                    onApprove={openApproveDialog}
                    onReject={openRejectDialog}
                    showDescription
                  />
                </SoftTabsContent>

                <SoftTabsContent value="capabilities">
                  <ProviderCapabilityPanel capabilities={overviewData.provider_capabilities} />
                </SoftTabsContent>

                <SoftTabsContent value="events">
                  <EventAuditPanel
                    events={eventItems}
                    filters={eventFilters}
                    filterOptions={filterOptions}
                    hasAppliedFilter={hasAppliedEventFilter}
                    isError={events.isError}
                    isLoading={events.isLoading}
                    onFilterChange={updateEventFilter}
                  />
                </SoftTabsContent>
              </SoftTabs>
            </>
          ) : null}
        </div>

        <AlertDialog
          open={Boolean(approveTarget)}
          onOpenChange={(open) => {
            if (!open) {
              setApproveTarget(null);
              approve.reset();
            }
          }}
        >
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>确认 Runtime 接入</AlertDialogTitle>
              <AlertDialogDescription>
                批准后，{approveTarget?.node_id ?? "该节点"} 可以进入 Runtime 会话建立流程。此操作会写入审计记录。
              </AlertDialogDescription>
            </AlertDialogHeader>
            {approve.isError ? <MutationErrorLine error={approve.error} fallback="Runtime 接入批准失败" /> : null}
            <AlertDialogFooter>
              <AlertDialogCancel disabled={approve.isPending}>取消</AlertDialogCancel>
              <AlertDialogAction
                disabled={approve.isPending || !approveTarget}
                onClick={(event) => {
                  event.preventDefault();
                  if (approveTarget) {
                    approve.mutate(approveTarget.id);
                  }
                }}
              >
                确认接入
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>

        <Dialog
          open={Boolean(rejectTarget)}
          onOpenChange={(open) => {
            if (!open) {
              setRejectTarget(null);
              setRejectReason("");
              reject.reset();
            }
          }}
        >
          <DialogContent>
            <DialogHeader>
              <DialogTitle>拒绝 Runtime 接入</DialogTitle>
              <DialogDescription>
                拒绝原因会随审批结果持久化，供后续排查节点来源、权限或环境归属问题。
              </DialogDescription>
            </DialogHeader>
            <div className="flex flex-col gap-2">
              <Label htmlFor="runtime-reject-reason">拒绝原因</Label>
              <Textarea
                id="runtime-reject-reason"
                value={rejectReason}
                onChange={(event) => setRejectReason(event.target.value)}
                placeholder="例如：节点归属未完成线下确认"
              />
            </div>
            {reject.isError ? <MutationErrorLine error={reject.error} fallback="Runtime 接入拒绝失败" /> : null}
            <DialogFooter>
              <Button
                disabled={reject.isPending}
                type="button"
                variant="outline"
                onClick={() => {
                  setRejectTarget(null);
                  reject.reset();
                }}
              >
                取消
              </Button>
              <Button
                disabled={reject.isPending || rejectReason.trim().length === 0 || !rejectTarget}
                type="button"
                variant="danger"
                onClick={() => {
                  if (rejectTarget) {
                    reject.mutate({ id: rejectTarget.id, reason: rejectReason.trim() });
                  }
                }}
              >
                确认拒绝
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </Main>
    </>
  );
}

function SummaryMetrics({ summary }: { summary: RuntimeOverviewSummary }) {
  return (
    <div className="grid min-w-0 gap-4 [grid-template-columns:repeat(auto-fit,minmax(min(100%,13rem),1fr))]">
      <MetricCard
        icon={<Wifi />}
        iconTone="ok"
        label="节点在线"
        value={`${summary.online_nodes} / ${summary.total_nodes}`}
        meta="在线节点 / 已登记节点 · 心跳健康"
      />
      <MetricCard
        icon={<ShieldCheck />}
        iconTone={summary.pending_enrollments > 0 ? "warn" : "ok"}
        label="待接入"
        value={summary.pending_enrollments}
        meta="等待人类确认的 Runtime 接入"
        loud={summary.pending_enrollments > 0}
      />
      <MetricCard
        icon={<Activity />}
        iconTone="info"
        label="Provider 会话"
        value={summary.active_provider_sessions}
        meta="当前 Provider 会话占用"
      />
      <MetricCard
        icon={<AlertTriangle />}
        iconTone={summary.blocked_events > 0 ? "danger" : "mute"}
        label="阻断事件"
        value={summary.blocked_events}
        meta={summary.blocked_events > 0 ? "需要优先处理" : "无阻断事件"}
        loud={summary.blocked_events > 0}
      />
    </div>
  );
}

type RuntimeOverviewSummary = {
  active_provider_sessions: number;
  blocked_events: number;
  online_nodes: number;
  pending_enrollments: number;
  total_nodes: number;
};

function PendingEnrollmentPanel({
  enrollments,
  onApprove,
  onReject,
  isError,
  isLoading,
  showDescription
}: {
  enrollments: RuntimeEnrollment[];
  isError?: boolean;
  isLoading?: boolean;
  onApprove: (enrollment: RuntimeEnrollment) => void;
  onReject: (enrollment: RuntimeEnrollment) => void;
  showDescription?: boolean;
}) {
  return (
    <SoftCard className="min-w-0 overflow-hidden">
      <PanelHeader
        icon={<ShieldCheck />}
        title="接入审批"
        description={showDescription ? "确认节点来源和 Provider 能力后再批准接入。" : undefined}
      />
      <div className="divide-y divide-line">
        {isLoading && enrollments.length === 0 ? (
          <div className="p-4" data-testid="runtime-enrollments-skeleton">
            <TableSkeleton cols={3} rows={4} />
          </div>
        ) : null}
        {isError ? (
          <div className="p-4">
            <ErrorState title="Runtime 接入记录加载失败" />
          </div>
        ) : null}
        {!isLoading && enrollments.length === 0 ? <EmptyState title="暂无 Runtime 接入记录" /> : null}
        {enrollments.length > 0 ? (
          enrollments.map((enrollment) => (
            <EnrollmentRow key={enrollment.id} enrollment={enrollment} onApprove={onApprove} onReject={onReject} />
          ))
        ) : null}
      </div>
    </SoftCard>
  );
}

function EnrollmentRow({
  enrollment,
  onApprove,
  onReject
}: {
  enrollment: RuntimeEnrollment;
  onApprove: (enrollment: RuntimeEnrollment) => void;
  onReject: (enrollment: RuntimeEnrollment) => void;
}) {
  const extras = getEnrollmentExtras(enrollment);
  const isPending = enrollment.status === "pending";

  return (
    <div className="grid min-w-0 gap-3 p-4 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
      <div className="min-w-0">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="truncate font-semibold text-ink">{enrollment.node_id}</span>
          <StatusPill tone={enrollmentStatusTone[enrollment.status]}>
            {enrollmentStatusLabel[enrollment.status]}
          </StatusPill>
        </div>
        <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-ink-2">
          <span>创建：{formatTime(enrollment.created_at)}</span>
          <span>最近 hello：{formatTime(extras.lastHelloAt)}</span>
          <span>Slots：{extras.maxSlots ?? "-"}</span>
          <span>Provider：{extras.supportedProviders.length > 0 ? extras.supportedProviders.join(", ") : "-"}</span>
        </div>
        {enrollment.reject_reason ? (
          <p className="mt-2 text-sm text-ink-2">拒绝原因：{enrollment.reject_reason}</p>
        ) : null}
      </div>
      {isPending ? (
        <div className="flex shrink-0 flex-wrap gap-2">
          <Button type="button" size="sm" onClick={() => onApprove(enrollment)}>
            <Check data-icon="inline-start" />
            批准接入
          </Button>
          <Button type="button" size="sm" variant="outline" onClick={() => onReject(enrollment)}>
            <Ban data-icon="inline-start" />
            拒绝
          </Button>
        </div>
      ) : null}
    </div>
  );
}

function NodeInventoryPanel({
  nodes,
  selectedNodeId,
  onSelectNode
}: {
  nodes: RuntimeNodeResponse[];
  selectedNodeId?: string | null;
  onSelectNode?: (nodeId: string) => void;
}) {
  return (
    <WorkSurface className="min-w-0">
      <PanelHeader
        icon={<Server />}
        title="已登记节点"
        description="按心跳、槽位占用和 Provider 覆盖观察当前执行面。点击节点可管理原生模型配置。"
      />
      <DataTable>
        <thead>
          <tr>
            <Th className="min-w-[260px]">节点</Th>
            <Th>Provider</Th>
            <Th>心跳</Th>
            <Th className="min-w-[160px]">槽位占用</Th>
          </tr>
        </thead>
        <tbody>
          {nodes.length === 0 ? (
            <Tr>
              <Td colSpan={4}>
                <EmptyState title="暂无已登记 Runtime 节点" />
              </Td>
            </Tr>
          ) : (
            nodes.map((node) => (
              <NodeRow
                key={node.node_id}
                node={node}
                selected={selectedNodeId === node.node_id}
                onSelect={onSelectNode}
              />
            ))
          )}
        </tbody>
      </DataTable>
    </WorkSurface>
  );
}

function NodeRow({
  node,
  selected,
  onSelect
}: {
  node: RuntimeNodeResponse;
  selected?: boolean;
  onSelect?: (nodeId: string) => void;
}) {
  const loadPercent = node.max_slots > 0 ? Math.min(100, Math.round((node.current_load / node.max_slots) * 100)) : 0;

  return (
    <Tr
      tone={node.status === "online" ? undefined : "warn"}
      className={cn(onSelect ? "cursor-pointer" : undefined, selected ? "bg-brand-soft/40" : undefined)}
      onClick={onSelect ? () => onSelect(node.node_id) : undefined}
    >
      <Td className="min-w-[260px]">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="truncate font-semibold text-ink">{node.name || node.node_id}</span>
          <StatusPill tone={node.status === "online" ? "ok" : "mute"}>
            {node.status === "online" ? "在线" : "离线"}
          </StatusPill>
        </div>
        <p className="mt-1 truncate font-mono text-xs text-ink-2">节点 ID：{node.node_id}</p>
      </Td>
      <Td className="text-ink-2">
        Provider：{node.supported_providers.length > 0 ? node.supported_providers.join(", ") : "-"}
      </Td>
      <Td className="tabular-nums text-ink-2">{formatTime(node.last_heartbeat_at)}</Td>
      <Td className="min-w-[160px]">
        <div className="flex items-center justify-between gap-2 text-[13px]">
          <span className="text-ink-2">槽位</span>
          <span className="font-medium">
            {node.current_load} / {node.max_slots}
          </span>
        </div>
        <div className="mt-2 h-2 rounded-full bg-card-soft">
          <div
            className="h-2 rounded-full bg-info"
            style={{ width: `${loadPercent}%` }}
          />
        </div>
      </Td>
    </Tr>
  );
}

const MODEL_PROFILE_SEED_KEYS: Record<string, string[]> = {
  "claude-code": ["model", "fallbackModel", "env.ANTHROPIC_BASE_URL", "env.ANTHROPIC_MODEL"],
  codex: ["model", "model_provider"],
  opencode: ["model", "small_model"]
};

function ProviderNativeConfigPanel({
  apiBaseUrl,
  fetcher,
  nodeId,
  node
}: {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  nodeId: string;
  node?: RuntimeNodeResponse;
}) {
  const options = useMemo(() => ({ baseUrl: apiBaseUrl, fetcher }), [apiBaseUrl, fetcher]);
  const listQuery = useQuery({
    queryKey: ["provider-native-configs", nodeId],
    queryFn: () => listProviderNativeConfigs(options, nodeId),
    enabled: Boolean(nodeId)
  });
  const [editing, setEditing] = useState<{
    providerType: string;
    configKey: string;
    detail: ProviderNativeConfigDetail;
    draft: Record<string, string>;
    revealSensitive: boolean;
  } | null>(null);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [activeActionKey, setActiveActionKey] = useState<string | null>(null);

  useEffect(() => {
    setEditing(null);
    setErrorMessage(null);
    setActiveActionKey(null);
  }, [nodeId]);

  const openDetail = (detail: ProviderNativeConfigDetail) => {
    const draft = buildDraftFromDetail(detail);
    setEditing({
      providerType: detail.provider_type,
      configKey: detail.config_key,
      detail,
      draft,
      revealSensitive: false
    });
  };

  const pullMutation = useMutation({
    mutationFn: async ({ providerType, configKey }: { providerType: string; configKey: string }) => {
      setActiveActionKey(`${providerType}/${configKey}:pull`);
      return pullProviderNativeConfig(options, nodeId, providerType, configKey);
    },
    onSuccess: (detail) => {
      setErrorMessage(null);
      openDetail(detail);
      void listQuery.refetch();
    },
    onError: (error: Error) => {
      setErrorMessage(humanizeConfigError(error.message) || "从节点拉取失败");
    },
    onSettled: () => setActiveActionKey(null)
  });

  const snapshotMutation = useMutation({
    mutationFn: async ({ providerType, configKey }: { providerType: string; configKey: string }) => {
      setActiveActionKey(`${providerType}/${configKey}:snapshot`);
      return getProviderNativeConfig(options, nodeId, providerType, configKey);
    },
    onSuccess: (detail) => {
      setErrorMessage(null);
      openDetail(detail);
    },
    onError: (error: Error) => {
      setErrorMessage(humanizeConfigError(error.message) || "打开快照失败，请先从节点拉取");
    },
    onSettled: () => setActiveActionKey(null)
  });

  const saveMutation = useMutation({
    mutationFn: async () => {
      if (!editing) {
        throw new Error("无可保存配置");
      }
      if (!editing.detail.file_content_hash) {
        throw new Error("缺少文件指纹，请先从节点拉取");
      }
      if (!editing.detail.node_online) {
        throw new Error("节点离线，无法保存");
      }
      if (!editing.detail.manageable) {
        throw new Error("该配置面不可经文件管理");
      }
      const values: Record<string, unknown> = {};
      for (const [key, raw] of Object.entries(editing.draft)) {
        const trimmed = raw.trim();
        if (trimmed === "") {
          // Only send null for keys that already exist on the node, so we don't spam deletes.
          if (Object.prototype.hasOwnProperty.call(editing.detail.managed_values, key)) {
            values[key] = null;
          }
          continue;
        }
        try {
          values[key] = JSON.parse(trimmed) as unknown;
        } catch {
          values[key] = trimmed;
        }
      }
      if (Object.keys(values).length === 0) {
        throw new Error("没有需要下发的变更");
      }
      return putProviderNativeConfig(
        options,
        nodeId,
        editing.providerType,
        editing.configKey,
        values,
        editing.detail.file_content_hash
      );
    },
    onSuccess: (detail) => {
      setErrorMessage(null);
      openDetail(detail);
      void listQuery.refetch();
    },
    onError: (error: Error) => {
      setErrorMessage(humanizeConfigError(error.message) || "保存失败");
    }
  });

  const items = listQuery.data ?? [];
  const grouped = useMemo(() => {
    const map = new Map<string, ProviderNativeConfigListItem[]>();
    for (const item of items) {
      const list = map.get(item.provider_type) ?? [];
      list.push(item);
      map.set(item.provider_type, list);
    }
    // Prefer stable provider order.
    const order = ["claude-code", "codex", "opencode"];
    return [...map.entries()].sort((a, b) => {
      const ai = order.indexOf(a[0]);
      const bi = order.indexOf(b[0]);
      return (ai === -1 ? 99 : ai) - (bi === -1 ? 99 : bi);
    });
  }, [items]);

  const nodeOnline = node ? node.status === "online" : Boolean(items[0]?.node_online);
  const displayName = node?.name || nodeId;
  const busy = pullMutation.isPending || snapshotMutation.isPending || saveMutation.isPending;

  return (
    <SoftCard className="min-w-0 overflow-hidden" data-testid="provider-native-config-panel">
      <div className="flex flex-wrap items-start justify-between gap-3 border-b border-line px-4 py-3">
        <div className="min-w-0">
          <div className="flex min-w-0 flex-wrap items-center gap-2">
            <IconTile tone="info" size="sm">
              <Cpu />
            </IconTile>
            <div className="min-w-0">
              <h3 className="truncate text-[15px] font-semibold text-ink">Provider 原生配置</h3>
              <p className="mt-0.5 truncate text-xs text-ink-2">
                {displayName}
                <span className="mx-1 text-line">·</span>
                <span className="font-mono">{nodeId}</span>
              </p>
            </div>
            <StatusPill tone={nodeOnline ? "ok" : "mute"}>{nodeOnline ? "节点在线" : "节点离线"}</StatusPill>
          </div>
        </div>
      </div>

      <div className="border-b border-line bg-card-soft/60 px-4 py-2 text-xs leading-relaxed text-ink-2">
        仅管理模型 / 供应商 / 端点 / 认证定位；MCP 请到能力注册表或项目 MCP 绑定。带 MCP 的 OpenCode 任务可能不消费本页 host 配置。
      </div>

      <div className="flex flex-col gap-3 p-4">
        {listQuery.isLoading ? <DetailSkeleton /> : null}
        {listQuery.isError ? <ErrorState title="配置面列表加载失败" onRetry={() => void listQuery.refetch()} /> : null}
        {errorMessage ? (
          <div className="rounded-[10px] border border-danger/30 bg-danger/5 px-3 py-2 text-sm text-danger" data-testid="provider-native-config-error">
            {errorMessage}
          </div>
        ) : null}

        {grouped.map(([providerType, surfaces]) => (
          <div key={providerType} className="min-w-0 overflow-hidden rounded-[12px] border border-line">
            <div className="flex items-center justify-between gap-2 bg-card-soft px-3 py-2">
              <span className="text-sm font-semibold text-ink">{providerType}</span>
              <span className="text-xs text-ink-2">{surfaces.length} 个配置面</span>
            </div>
            <div className="divide-y divide-line">
              {surfaces.map((surface) => {
                const surfaceKey = `${surface.provider_type}/${surface.config_key}`;
                const hasSnapshot = Boolean(surface.file_content_hash || surface.source || surface.snapshot_at);
                const pulling = activeActionKey === `${surfaceKey}:pull`;
                const opening = activeActionKey === `${surfaceKey}:snapshot`;
                return (
                  <div
                    key={surfaceKey}
                    className="grid min-w-0 gap-2 px-3 py-2.5 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center"
                    data-testid={`provider-native-surface-${surface.provider_type}-${surface.config_key}`}
                  >
                    <div className="min-w-0">
                      <div className="flex min-w-0 flex-wrap items-center gap-1.5">
                        <span className="font-medium text-ink">{statusLabel(surface.config_key)}</span>
                        <StatusPill tone={surface.exists_on_node ? "ok" : "mute"}>
                          {surface.exists_on_node ? "文件存在" : "尚无文件"}
                        </StatusPill>
                        {!surface.manageable ? (
                          <StatusPill tone="warn">
                            不可管理
                            {surface.unmanageable_reason ? ` · ${statusLabel(surface.unmanageable_reason)}` : ""}
                          </StatusPill>
                        ) : null}
                        {surface.source ? (
                          <StatusPill tone="info">{statusLabel(surface.source)}</StatusPill>
                        ) : (
                          <StatusPill tone="mute">无快照</StatusPill>
                        )}
                      </div>
                      <p className="mt-1 truncate font-mono text-[11px] text-ink-2">
                        {surface.resolved_path || "路径待拉取"}
                        {surface.file_content_hash ? ` · ${shortHash(surface.file_content_hash)}` : ""}
                        {surface.snapshot_at ? ` · ${formatTime(surface.snapshot_at)}` : ""}
                      </p>
                    </div>
                    <div className="flex shrink-0 flex-wrap gap-1.5">
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled={busy || !hasSnapshot}
                        onClick={() =>
                          snapshotMutation.mutate({
                            providerType: surface.provider_type,
                            configKey: surface.config_key
                          })
                        }
                      >
                        <FileJson data-icon="inline-start" />
                        {opening ? "打开中…" : "打开快照"}
                      </Button>
                      <Button
                        type="button"
                        size="sm"
                        disabled={busy || !surface.node_online}
                        onClick={() =>
                          pullMutation.mutate({
                            providerType: surface.provider_type,
                            configKey: surface.config_key
                          })
                        }
                      >
                        <Download data-icon="inline-start" />
                        {pulling ? "拉取中…" : "从节点拉取"}
                      </Button>
                    </div>
                  </div>
                );
              })}
            </div>
          </div>
        ))}

        <Sheet
          open={Boolean(editing)}
          onOpenChange={(open) => {
            if (!open) {
              setEditing(null);
            }
          }}
        >
          <SheetContent
            side="right"
            className="flex w-full flex-col gap-0 p-0 sm:max-w-lg"
            data-testid="provider-native-config-editor"
          >
            {editing ? (
              <>
                <SheetHeader className="border-b border-line px-4 py-3 text-left">
                  <SheetTitle>
                    {editing.providerType} / {statusLabel(editing.configKey)}
                  </SheetTitle>
                  <SheetDescription className="font-mono text-[11px] text-ink-2">
                    {editing.detail.stale_hint ? "非实时快照 · " : "实时拉取 · "}
                    hash {shortHash(editing.detail.file_content_hash)}
                    {editing.detail.resolved_path ? ` · ${editing.detail.resolved_path}` : ""}
                  </SheetDescription>
                </SheetHeader>

                <div className="min-h-0 flex-1 space-y-3 overflow-y-auto px-4 py-3">
                  {!editing.detail.manageable ? (
                    <div className="rounded-[10px] border border-warn/30 bg-warn/10 px-3 py-2 text-sm text-ink">
                      不可编辑：{statusLabel(editing.detail.unmanageable_reason || "不可管理")}
                      。平台不会创建框架不读取的凭据文件。
                    </div>
                  ) : null}
                  {!editing.detail.node_online ? (
                    <div className="rounded-[10px] border border-line bg-card-soft px-3 py-2 text-sm text-ink-2">
                      节点离线：可浏览快照，保存已禁用。
                    </div>
                  ) : null}
                  {errorMessage ? (
                    <div className="rounded-[10px] border border-danger/30 bg-danger/5 px-3 py-2 text-sm text-danger">
                      {errorMessage}
                    </div>
                  ) : null}

                  <div className="flex flex-col gap-2.5">
                    {Object.keys(editing.draft).length === 0 ? (
                      <p className="text-sm text-ink-2">
                        当前无受管键。可先从节点拉取，或在有底本后编辑白名单字段。
                      </p>
                    ) : (
                      Object.entries(editing.draft).map(([key, value]) => {
                        const sensitive = isSensitiveField(key, editing.configKey);
                        return (
                          <div key={key} className="grid gap-1">
                            <Label className="font-mono text-[11px] text-ink-2">{key}</Label>
                            <Input
                              value={value}
                              disabled={!editing.detail.manageable}
                              type={sensitive && !editing.revealSensitive ? "password" : "text"}
                              placeholder={sensitive ? "••••••••" : undefined}
                              onChange={(event) =>
                                setEditing((prev) =>
                                  prev
                                    ? {
                                        ...prev,
                                        draft: { ...prev.draft, [key]: event.target.value }
                                      }
                                    : prev
                                )
                              }
                            />
                          </div>
                        );
                      })
                    )}
                  </div>

                  <details className="rounded-[10px] border border-line bg-card-soft p-2">
                    <summary className="cursor-pointer text-xs font-medium text-ink-2">
                      受管键 JSON 预览（敏感值已掩码）
                    </summary>
                    <pre className="mt-2 max-h-40 overflow-auto font-mono text-[11px] text-ink-2">
                      {JSON.stringify(
                        maskManagedValues(editing.detail.managed_values, editing.configKey),
                        null,
                        2
                      )}
                    </pre>
                  </details>
                </div>

                <SheetFooter className="flex-row flex-wrap justify-between gap-2 border-t border-line px-4 py-3 sm:space-x-0">
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() =>
                      setEditing((prev) => (prev ? { ...prev, revealSensitive: !prev.revealSensitive } : prev))
                    }
                  >
                    {editing.revealSensitive ? (
                      <EyeOff data-icon="inline-start" />
                    ) : (
                      <Eye data-icon="inline-start" />
                    )}
                    {editing.revealSensitive ? "隐藏敏感值" : "显示敏感值"}
                  </Button>
                  <div className="flex gap-2">
                    <Button type="button" size="sm" variant="outline" onClick={() => setEditing(null)}>
                      关闭
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      disabled={
                        saveMutation.isPending || !editing.detail.manageable || !editing.detail.node_online
                      }
                      onClick={() => saveMutation.mutate()}
                    >
                      {saveMutation.isPending ? "保存中…" : "保存下发"}
                    </Button>
                  </div>
                </SheetFooter>
              </>
            ) : null}
          </SheetContent>
        </Sheet>
      </div>
    </SoftCard>
  );
}

function buildDraftFromDetail(detail: ProviderNativeConfigDetail): Record<string, string> {
  const draft: Record<string, string> = {};
  const seeds =
    detail.config_key === "model_profile" ? (MODEL_PROFILE_SEED_KEYS[detail.provider_type] ?? []) : [];
  for (const key of seeds) {
    draft[key] = "";
  }
  for (const [key, value] of Object.entries(detail.managed_values ?? {})) {
    draft[key] =
      value === null || value === undefined ? "" : typeof value === "string" ? value : JSON.stringify(value);
  }
  return draft;
}

function humanizeConfigError(message: string): string {
  const text = message.trim();
  if (!text) {
    return text;
  }
  if (/deadline exceeded|timeout|timed out/i.test(text)) {
    return "节点命令通道超时。请确认 Runtime Agent 在线后重试「从节点拉取」。若仅需查看，可先「打开快照」。";
  }
  if (/conflict|hash mismatch/i.test(text)) {
    return "节点配置已变更（乐观锁冲突）。请重新从节点拉取后再编辑保存。";
  }
  if (/unmanageable|platform_keychain|oauth_session/i.test(text)) {
    return "该配置面不可经文件管理（系统钥匙串或 OAuth 会话受保护）。";
  }
  if (/not in allowlist|validation/i.test(text)) {
    return "包含白名单外的键，已拒绝写入。";
  }
  // Strip noisy client prefixes.
  return text
    .replace(/^pull provider native config request failed with status \d+:\s*/i, "")
    .replace(/^push provider native config request failed with status \d+:\s*/i, "")
    .replace(/^provider native config snapshot request failed with status \d+:\s*/i, "");
}

function shortHash(hash?: string): string {
  if (!hash) {
    return "-";
  }
  const bare = hash.replace(/^sha256:/, "");
  return bare.length > 12 ? `${bare.slice(0, 8)}…` : bare;
}

function isSensitiveField(key: string, configKey: string): boolean {
  if (configKey === "auth") {
    return true;
  }
  return (
    key.includes("TOKEN") ||
    key.includes("API_KEY") ||
    key.includes("bearer") ||
    key.includes("token") ||
    key.includes("secret")
  );
}

function maskManagedValues(
  values: Record<string, unknown> | undefined,
  configKey: string
): Record<string, unknown> {
  const out: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(values ?? {})) {
    out[key] = isSensitiveField(key, configKey) ? "••••••••" : value;
  }
  return out;
}

function ProviderCapabilityPanel({
  capabilities,
  compact
}: {
  capabilities: RuntimeProviderCapabilitySummary[];
  compact?: boolean;
}) {
  return (
    <SoftCard className="min-w-0 overflow-hidden">
      <PanelHeader icon={<Cpu />} title="能力范围" description="Provider 类型、节点覆盖和健康可用性快照。" />
      <div className={cn("grid gap-3 p-4", compact ? "grid-cols-1" : "lg:grid-cols-2")}>
        {capabilities.length === 0 ? (
          <EmptyState className="lg:col-span-2" title="暂无 Provider 能力上报" />
        ) : (
          capabilities.map((capability) => <CapabilityRow key={capability.provider_type} capability={capability} />)
        )}
      </div>
    </SoftCard>
  );
}

function CapabilityRow({ capability }: { capability: RuntimeProviderCapabilitySummary }) {
  return (
    <div className="min-w-0 rounded-inner bg-card-soft p-3">
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="truncate font-semibold text-ink">{capability.provider_type}</div>
          <p className="mt-1 text-xs text-ink-2">最近上报：{formatTime(capability.last_seen_at)}</p>
        </div>
        <StatusPill tone={capability.healthy_count > 0 ? "ok" : "warn"}>
          健康 {capability.healthy_count}
        </StatusPill>
      </div>
      <div className="my-3 border-t border-line" />
      <div className="grid grid-cols-3 gap-2 text-sm">
        <MetricLite label="节点" value={capability.node_count} />
        <MetricLite label="可用" value={capability.available_count} />
        <MetricLite label="健康" value={capability.healthy_count} />
      </div>
    </div>
  );
}

function RecentEventsPanel({ events, compact }: { events: RuntimeEvent[]; compact?: boolean }) {
  const limit = compact ? 3 : 5;
  return (
    <SoftCard className="min-w-0 overflow-hidden">
      <PanelHeader
        icon={<FileClock />}
        title="最近事件"
        description={compact ? undefined : "来自 Runtime command、节点心跳和 Provider 会话的最新回传。"}
      />
      <div className="divide-y divide-line">
        {events.length === 0 ? (
          <EmptyState title="暂无 Runtime 事件" />
        ) : (
          events.slice(0, limit).map((event) => <EventRow key={event.id} event={event} />)
        )}
      </div>
    </SoftCard>
  );
}

function EventAuditPanel({
  events,
  filterOptions,
  filters,
  hasAppliedFilter,
  isError,
  isLoading,
  onFilterChange
}: {
  events: RuntimeEvent[];
  filterOptions: {
    eventTypes: string[];
    nodes: string[];
    providers: string[];
  };
  filters: RuntimeEventFilters;
  hasAppliedFilter: boolean;
  isError: boolean;
  isLoading: boolean;
  onFilterChange: <Key extends keyof RuntimeEventFilters>(key: Key, value: RuntimeEventFilters[Key]) => void;
}) {
  return (
    <WorkSurface className="min-w-0">
      <div className="border-b border-line p-4">
        <div className="flex min-w-0 items-start gap-3">
          <IconTile tone="info" size="sm">
            <FileClock />
          </IconTile>
          <div className="min-w-0">
            <h2 className="text-sm font-semibold text-ink">事件审计</h2>
            <p className="mt-1 text-xs text-ink-2">按事件类型、严重级别、Runtime 节点和 Provider 过滤最近事件。</p>
          </div>
        </div>
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          <RuntimeSelectFilter
            id="runtime-event-type"
            label="事件类型"
            options={filterOptions.eventTypes.map((value) => ({ label: value, value }))}
            value={filters.event_type}
            onValueChange={(value) => onFilterChange("event_type", value)}
          />
          <RuntimeSelectFilter
            id="runtime-event-severity"
            label="严重级别"
            options={severityOptions}
            value={filters.severity}
            onValueChange={(value) => onFilterChange("severity", value as RuntimeEventSeverity | undefined)}
          />
          <RuntimeSelectFilter
            id="runtime-event-node"
            label="Runtime 节点"
            options={filterOptions.nodes.map((value) => ({ label: value, value }))}
            value={filters.node_id}
            onValueChange={(value) => onFilterChange("node_id", value)}
          />
          <RuntimeSelectFilter
            id="runtime-event-provider"
            label="Provider"
            options={filterOptions.providers.map((value) => ({ label: value, value }))}
            value={filters.provider_type}
            onValueChange={(value) => onFilterChange("provider_type", value)}
          />
        </div>
      </div>

      <DataTable>
        <thead>
          <tr>
            <Th className="min-w-[240px]">事件</Th>
            <Th>严重级别</Th>
            <Th>来源</Th>
            <Th>节点 / Provider</Th>
            <Th>时间</Th>
          </tr>
        </thead>
        <tbody>
          {isLoading && events.length === 0 ? (
            <Tr>
              <Td colSpan={5} className="p-4" data-testid="runtime-events-skeleton">
                <TableSkeleton cols={5} rows={5} />
              </Td>
            </Tr>
          ) : null}
          {isError ? (
            <Tr tone="danger">
              <Td colSpan={5}>
                <ErrorState title="Runtime 事件加载失败" />
              </Td>
            </Tr>
          ) : null}
          {!isLoading && events.length === 0 ? (
            <Tr>
              <Td colSpan={5}>
                <EmptyState title={hasAppliedFilter ? "筛选后无 Runtime 事件" : "暂无 Runtime 事件"} />
              </Td>
            </Tr>
          ) : null}
          {events.length > 0 ? events.map((event) => <EventAuditRow key={event.id} event={event} />) : null}
        </tbody>
      </DataTable>
    </WorkSurface>
  );
}

function RuntimeSelectFilter({
  id,
  label,
  onValueChange,
  options,
  value
}: {
  id: string;
  label: string;
  onValueChange: (value: string | undefined) => void;
  options: Array<{ label: string; value: string }>;
  value?: string;
}) {
  return (
    <div className="flex min-w-0 flex-col gap-2">
      <Label htmlFor={id} className="text-xs font-semibold text-ink-2">
        {label}
      </Label>
      <Select value={value ?? "all"} onValueChange={(nextValue) => onValueChange(nextValue === "all" ? undefined : nextValue)}>
        <SelectTrigger
          aria-label={label}
          id={id}
          className="w-full border-line-strong bg-card text-ink shadow-none"
        >
          <SelectValue placeholder="全部" />
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            <SelectItem value="all">全部</SelectItem>
            {options.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    </div>
  );
}

function EventRow({ event }: { event: RuntimeEvent }) {
  return (
    <div className="flex min-w-0 gap-3 p-4">
      <IconTile tone={severityTone[event.severity]} size="sm">
        {event.severity === "error" ? <AlertTriangle /> : <Clock />}
      </IconTile>
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="truncate font-semibold text-ink">{event.title}</span>
          <StatusPill tone={severityTone[event.severity]}>{severityLabel[event.severity]}</StatusPill>
        </div>
        <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-ink-2">
          <span>{event.event_type}</span>
          <span>{event.source}</span>
          <span>节点：{event.node_id ?? event.runtime_node_id ?? "-"}</span>
          <span>Provider：{event.provider_type ?? "-"}</span>
          <span>{formatTime(event.created_at)}</span>
        </div>
      </div>
    </div>
  );
}

function EventAuditRow({ event }: { event: RuntimeEvent }) {
  return (
    <Tr tone={event.severity === "error" ? "danger" : event.severity === "warning" ? "warn" : undefined}>
      <Td className="min-w-[240px]">
        <div className="flex min-w-0 items-start gap-3">
          <IconTile tone={severityTone[event.severity]} size="sm">
            {event.severity === "error" ? <AlertTriangle /> : <Clock />}
          </IconTile>
          <div className="min-w-0">
            <p className="truncate font-semibold text-ink">{event.title}</p>
            {event.description ? <p className="mt-1 line-clamp-2 text-xs text-ink-2">{event.description}</p> : null}
          </div>
        </div>
      </Td>
      <Td>
        <StatusPill tone={severityTone[event.severity]}>{severityLabel[event.severity]}</StatusPill>
      </Td>
      <Td className="text-ink-2">
        <div className="grid gap-1">
          <span>{event.event_type}</span>
          <span className="text-xs">{event.source}</span>
        </div>
      </Td>
      <Td className="text-ink-2">
        <div className="grid gap-1">
          <span>节点：{event.node_id ?? event.runtime_node_id ?? "-"}</span>
          <span>Provider：{event.provider_type ?? "-"}</span>
        </div>
      </Td>
      <Td className="tabular-nums text-ink-2">{formatTime(event.created_at)}</Td>
    </Tr>
  );
}

function PanelHeader({
  description,
  icon,
  title
}: {
  description?: string;
  icon: ReactNode;
  title: string;
}) {
  return (
    <div className="flex min-w-0 items-start gap-3 border-b border-line p-4">
      <IconTile tone="info" size="sm">
        {icon}
      </IconTile>
      <div className="min-w-0">
        <h2 className="text-sm font-semibold text-ink">{title}</h2>
        {description ? <p className="mt-1 text-xs text-ink-2">{description}</p> : null}
      </div>
    </div>
  );
}

function MetricLite({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-[12px] bg-card px-3 py-2">
      <div className="text-xs text-ink-2">{label}</div>
      <div className="mt-1 text-lg font-bold tracking-normal text-ink tabular-nums">{value}</div>
    </div>
  );
}

function MutationErrorLine({ error, fallback }: { error: unknown; fallback: string }) {
  return (
    <p className="rounded-inner bg-danger-soft px-3 py-2 text-sm text-danger">
      {readErrorMessage(error, fallback)}
    </p>
  );
}

function readErrorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

function uniqueStrings(values: Array<string | undefined>): string[] {
  return Array.from(new Set(values.filter((value): value is string => Boolean(value)))).sort((left, right) =>
    left.localeCompare(right),
  );
}

function getEnrollmentExtras(enrollment: RuntimeEnrollment) {
  const requestPayload = enrollment.request_payload ?? {};

  return {
    lastHelloAt: enrollment.last_hello_at,
    maxSlots: readNumber(requestPayload.max_slots),
    supportedProviders: readStringArray(requestPayload.supported_providers)
};
}

function readNumber(value: unknown): number | undefined {
  return typeof value === "number" && Number.isFinite(value) ? value : undefined;
}

function readStringArray(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((item): item is string => typeof item === "string") : [];
}

function formatTime(value?: string): string {
  if (!value) {
    return "-";
  }

  const date = new Date(value);

  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat("zh-CN", {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "2-digit"
}).format(date);
}

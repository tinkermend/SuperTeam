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
  FileClock,
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
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import {
  approveRuntimeEnrollment,
  getRuntimeOverview,
  listRuntimeEnrollments,
  listRuntimeEvents,
  rejectRuntimeEnrollment,
  type RuntimeEnrollment,
  type RuntimeEnrollmentStatus,
  type RuntimeEvent,
  type RuntimeEventSeverity,
  type RuntimeNodeResponse,
  type RuntimeProviderCapabilitySummary
} from "@/lib/api/runtime";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { cn } from "@/lib/utils";

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
                        <NodeInventoryPanel nodes={overviewData.nodes} />
                        <PendingEnrollmentPanel
                          enrollments={overviewData.pending_enrollments}
                          onApprove={openApproveDialog}
                          onReject={openRejectDialog}
                        />
                      </div>
                    }
                    detail={
                      <div className="flex min-w-0 flex-col gap-4">
                        <RecentEventsPanel events={recentEvents} />
                        <ProviderCapabilityPanel capabilities={overviewData.provider_capabilities} compact />
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

function NodeInventoryPanel({ nodes }: { nodes: RuntimeNodeResponse[] }) {
  return (
    <WorkSurface className="min-w-0">
      <PanelHeader
        icon={<Server />}
        title="已登记节点"
        description="按心跳、槽位占用和 Provider 覆盖观察当前执行面。"
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
            nodes.map((node) => <NodeRow key={node.node_id} node={node} />)
          )}
        </tbody>
      </DataTable>
    </WorkSurface>
  );
}

function NodeRow({ node }: { node: RuntimeNodeResponse }) {
  const loadPercent = node.max_slots > 0 ? Math.min(100, Math.round((node.current_load / node.max_slots) * 100)) : 0;

  return (
    <Tr tone={node.status === "online" ? undefined : "warn"}>
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

function RecentEventsPanel({ events }: { events: RuntimeEvent[] }) {
  return (
    <SoftCard className="min-w-0 overflow-hidden">
      <PanelHeader
        icon={<FileClock />}
        title="最近事件"
        description="来自 Runtime command、节点心跳和 Provider 会话的最新回传。"
      />
      <div className="divide-y divide-line">
        {events.length === 0 ? (
          <EmptyState title="暂无 Runtime 事件" />
        ) : (
          events.slice(0, 5).map((event) => <EventRow key={event.id} event={event} />)
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

import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
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
  Wifi,
} from "lucide-react";
import {
  LiquidCard,
  LiquidTabsList,
  LiquidTabsTrigger,
  MetricCard,
  SemanticIconTile,
  StatusBadge,
  type Tone,
} from "@/components/superteam";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Separator } from "@/components/ui/separator";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
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
  type RuntimeProviderCapabilitySummary,
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
  offset: 0,
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
  warning: "预警",
};

const severityTone: Record<RuntimeEventSeverity, Tone> = {
  error: "danger",
  info: "info",
  success: "success",
  warning: "warning",
};

const enrollmentStatusLabel: Record<RuntimeEnrollmentStatus, string> = {
  approved: "已接入",
  pending: "待接入",
  rejected: "已拒绝",
  revoked: "已停用",
};

const enrollmentStatusTone: Record<RuntimeEnrollmentStatus, Tone> = {
  approved: "success",
  pending: "warning",
  rejected: "danger",
  revoked: "neutral",
};

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
  const [eventFilters, setEventFilters] = useState<RuntimeEventFilters>(defaultEventFilters);
  const [approveTarget, setApproveTarget] = useState<RuntimeEnrollment | null>(null);
  const [rejectTarget, setRejectTarget] = useState<RuntimeEnrollment | null>(null);
  const [rejectReason, setRejectReason] = useState("");

  const overview = useQuery({
    queryKey: ["runtime-overview"],
    queryFn: () => getRuntimeOverview({ baseUrl: apiBaseUrl, fetcher }),
  });

  const events = useQuery({
    queryKey: ["runtime-events", eventFilters],
    queryFn: () => listRuntimeEvents({ baseUrl: apiBaseUrl, fetcher, ...eventFilters }),
  });

  const enrollments = useQuery({
    queryKey: ["runtime-enrollments"],
    queryFn: () => listRuntimeEnrollments({ baseUrl: apiBaseUrl, fetcher }),
  });

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
    },
  });

  const reject = useMutation({
    mutationFn: ({ id, reason }: { id: string; reason: string }) =>
      rejectRuntimeEnrollment({ baseUrl: apiBaseUrl, fetcher }, id, reason),
    onSuccess: () => {
      setRejectTarget(null);
      setRejectReason("");
      invalidateRuntimeQueries();
    },
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
      offset: 0,
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
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        <div className="flex min-w-0 flex-col gap-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex min-w-0 items-center gap-3">
              <SemanticIconTile tone="info">
                <Server />
              </SemanticIconTile>
              <div className="min-w-0">
                <h1 className="text-2xl font-bold tracking-normal">Runtime 节点</h1>
                <p className="text-sm text-muted-foreground">
                  运行节点接入、Provider 能力、事件审计和阻断信号的首屏视图。
                </p>
              </div>
            </div>
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

          {overview.isLoading ? (
            <LiquidCard>
              <CardContent className="py-8 text-sm text-muted-foreground">加载 Runtime 总览中</CardContent>
            </LiquidCard>
          ) : null}

          {overview.isError ? (
            <Alert variant="destructive">
              <AlertTriangle />
              <AlertTitle>Runtime 总览加载失败</AlertTitle>
              <AlertDescription className="mt-2 flex flex-wrap items-center gap-3">
                <span>请稍后重试，或检查 Control Plane Runtime API 是否可用。</span>
                <Button size="sm" type="button" variant="outline" onClick={() => void overview.refetch()}>
                  重试
                </Button>
              </AlertDescription>
            </Alert>
          ) : null}

          {overviewData ? (
            <>
              <SummaryMetrics summary={overviewData.summary} />

              <Tabs defaultValue="overview" className="gap-4">
                <LiquidCard>
                  <CardContent className="p-2">
                    <LiquidTabsList aria-label="Runtime 管理视图">
                      <LiquidTabsTrigger value="overview">节点总览</LiquidTabsTrigger>
                      <LiquidTabsTrigger value="enrollments">接入审批</LiquidTabsTrigger>
                      <LiquidTabsTrigger value="capabilities">能力范围</LiquidTabsTrigger>
                      <LiquidTabsTrigger value="events">事件审计</LiquidTabsTrigger>
                    </LiquidTabsList>
                  </CardContent>
                </LiquidCard>

                <TabsContent value="overview">
                  <div className="grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1.2fr)_minmax(20rem,0.8fr)]">
                    <div className="flex min-w-0 flex-col gap-4">
                      <NodeInventoryPanel nodes={overviewData.nodes} />
                      <PendingEnrollmentPanel
                        enrollments={overviewData.pending_enrollments}
                        onApprove={openApproveDialog}
                        onReject={openRejectDialog}
                      />
                    </div>
                    <div className="flex min-w-0 flex-col gap-4">
                      <RecentEventsPanel events={recentEvents} />
                      <ProviderCapabilityPanel capabilities={overviewData.provider_capabilities} compact />
                    </div>
                  </div>
                </TabsContent>

                <TabsContent value="enrollments">
                  <PendingEnrollmentPanel
                    enrollments={enrollmentItems}
                    isError={enrollments.isError}
                    isLoading={enrollments.isLoading}
                    onApprove={openApproveDialog}
                    onReject={openRejectDialog}
                    showDescription
                  />
                </TabsContent>

                <TabsContent value="capabilities">
                  <ProviderCapabilityPanel capabilities={overviewData.provider_capabilities} />
                </TabsContent>

                <TabsContent value="events">
                  <EventAuditPanel
                    events={eventItems}
                    filters={eventFilters}
                    filterOptions={filterOptions}
                    hasAppliedFilter={hasAppliedEventFilter}
                    isError={events.isError}
                    isLoading={events.isLoading}
                    onFilterChange={updateEventFilter}
                  />
                </TabsContent>
              </Tabs>
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
                variant="destructive"
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
        description="在线节点 / 已登记节点"
        icon={<Wifi />}
        iconTone="success"
        meta="心跳健康"
        statusTone="success"
        title="节点在线"
        value={`${summary.online_nodes} / ${summary.total_nodes}`}
      />
      <MetricCard
        description="等待人类确认的 Runtime 接入"
        icon={<ShieldCheck />}
        iconTone="decision"
        meta="审批队列"
        statusTone={summary.pending_enrollments > 0 ? "warning" : "success"}
        title="待接入"
        value={summary.pending_enrollments}
      />
      <MetricCard
        description="当前 Provider 会话占用"
        icon={<Activity />}
        iconTone="info"
        meta="执行中"
        statusTone="info"
        title="Provider 会话"
        value={summary.active_provider_sessions}
      />
      <MetricCard
        description="需要优先处理的阻断事件"
        icon={<AlertTriangle />}
        iconTone={summary.blocked_events > 0 ? "danger" : "neutral"}
        isError={summary.blocked_events > 0}
        meta={summary.blocked_events > 0 ? "需处理" : "无阻断"}
        statusTone={summary.blocked_events > 0 ? "danger" : "success"}
        title="阻断事件"
        value={summary.blocked_events}
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
  showDescription,
}: {
  enrollments: RuntimeEnrollment[];
  isError?: boolean;
  isLoading?: boolean;
  onApprove: (enrollment: RuntimeEnrollment) => void;
  onReject: (enrollment: RuntimeEnrollment) => void;
  showDescription?: boolean;
}) {
  return (
    <Card className="min-w-0 rounded-md">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <ShieldCheck />
          接入审批
        </CardTitle>
        {showDescription ? <CardDescription>确认节点来源和 Provider 能力后再批准接入。</CardDescription> : null}
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {isLoading ? <EmptyLine>加载 Runtime 接入记录中</EmptyLine> : null}
        {isError ? <p className="text-sm text-destructive">Runtime 接入记录加载失败</p> : null}
        {!isLoading && enrollments.length === 0 ? <EmptyLine>暂无 Runtime 接入记录</EmptyLine> : null}
        {enrollments.length > 0 ? (
          enrollments.map((enrollment) => (
            <EnrollmentRow key={enrollment.id} enrollment={enrollment} onApprove={onApprove} onReject={onReject} />
          ))
        ) : null}
      </CardContent>
    </Card>
  );
}

function EnrollmentRow({
  enrollment,
  onApprove,
  onReject,
}: {
  enrollment: RuntimeEnrollment;
  onApprove: (enrollment: RuntimeEnrollment) => void;
  onReject: (enrollment: RuntimeEnrollment) => void;
}) {
  const extras = getEnrollmentExtras(enrollment);
  const isPending = enrollment.status === "pending";

  return (
    <div className="flex min-w-0 flex-col gap-3 rounded-md border bg-card/70 p-3 md:flex-row md:items-center md:justify-between">
      <div className="min-w-0">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="truncate font-medium">{enrollment.node_id}</span>
          <StatusBadge tone={enrollmentStatusTone[enrollment.status]}>{enrollmentStatusLabel[enrollment.status]}</StatusBadge>
        </div>
        <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
          <span>创建：{formatTime(enrollment.created_at)}</span>
          <span>最近 hello：{formatTime(extras.lastHelloAt)}</span>
          <span>Slots：{extras.maxSlots ?? "-"}</span>
          <span>Provider：{extras.supportedProviders.length > 0 ? extras.supportedProviders.join(", ") : "-"}</span>
        </div>
        {enrollment.reject_reason ? (
          <p className="mt-2 text-sm text-muted-foreground">拒绝原因：{enrollment.reject_reason}</p>
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
    <Card className="min-w-0 rounded-md">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Server />
          已登记节点
        </CardTitle>
        <CardDescription>按心跳、槽位占用和 Provider 覆盖观察当前执行面。</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {nodes.length === 0 ? (
          <EmptyLine>暂无已登记 Runtime 节点</EmptyLine>
        ) : (
          nodes.map((node) => <NodeRow key={node.node_id} node={node} />)
        )}
      </CardContent>
    </Card>
  );
}

function NodeRow({ node }: { node: RuntimeNodeResponse }) {
  const loadPercent = node.max_slots > 0 ? Math.min(100, Math.round((node.current_load / node.max_slots) * 100)) : 0;

  return (
    <div className="grid min-w-0 gap-3 rounded-md border bg-card/70 p-3 lg:grid-cols-[minmax(0,1fr)_12rem]">
      <div className="min-w-0">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="truncate font-medium">{node.name || node.node_id}</span>
          <StatusBadge tone={node.status === "online" ? "success" : "neutral"}>
            {node.status === "online" ? "在线" : "离线"}
          </StatusBadge>
        </div>
        <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
          <span>节点 ID：{node.node_id}</span>
          <span>Provider：{node.supported_providers.length > 0 ? node.supported_providers.join(", ") : "-"}</span>
          <span>心跳：{formatTime(node.last_heartbeat_at)}</span>
        </div>
      </div>
      <div className="min-w-0">
        <div className="flex items-center justify-between gap-2 text-sm">
          <span className="text-muted-foreground">槽位占用</span>
          <span className="font-medium">
            {node.current_load} / {node.max_slots}
          </span>
        </div>
        <div className="mt-2 h-2 rounded-full bg-muted">
          <div
            className="h-2 rounded-full bg-[color:var(--superteam-info)]"
            style={{ width: `${loadPercent}%` }}
          />
        </div>
      </div>
    </div>
  );
}

function ProviderCapabilityPanel({
  capabilities,
  compact,
}: {
  capabilities: RuntimeProviderCapabilitySummary[];
  compact?: boolean;
}) {
  return (
    <Card className="min-w-0 rounded-md">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Cpu />
          能力范围
        </CardTitle>
        <CardDescription>Provider 类型、节点覆盖和健康可用性快照。</CardDescription>
      </CardHeader>
      <CardContent className={cn("grid gap-3", compact ? "grid-cols-1" : "lg:grid-cols-2")}>
        {capabilities.length === 0 ? (
          <EmptyLine>暂无 Provider 能力上报</EmptyLine>
        ) : (
          capabilities.map((capability) => <CapabilityRow key={capability.provider_type} capability={capability} />)
        )}
      </CardContent>
    </Card>
  );
}

function CapabilityRow({ capability }: { capability: RuntimeProviderCapabilitySummary }) {
  return (
    <div className="min-w-0 rounded-md border bg-card/70 p-3">
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="truncate font-medium">{capability.provider_type}</div>
          <p className="mt-1 text-xs text-muted-foreground">最近上报：{formatTime(capability.last_seen_at)}</p>
        </div>
        <StatusBadge tone={capability.healthy_count > 0 ? "success" : "warning"}>
          健康 {capability.healthy_count}
        </StatusBadge>
      </div>
      <Separator className="my-3" />
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
    <Card className="min-w-0 rounded-md">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <FileClock />
          最近事件
        </CardTitle>
        <CardDescription>来自 Runtime command、节点心跳和 Provider 会话的最新回传。</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {events.length === 0 ? (
          <EmptyLine>暂无 Runtime 事件</EmptyLine>
        ) : (
          events.slice(0, 5).map((event) => <EventRow key={event.id} event={event} />)
        )}
      </CardContent>
    </Card>
  );
}

function EventAuditPanel({
  events,
  filterOptions,
  filters,
  hasAppliedFilter,
  isError,
  isLoading,
  onFilterChange,
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
    <Card className="min-w-0 rounded-md">
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <FileClock />
          事件审计
        </CardTitle>
        <CardDescription>按事件类型、严重级别、Runtime 节点和 Provider 过滤最近事件。</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
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

        {isLoading ? <EmptyLine>加载 Runtime 事件中</EmptyLine> : null}
        {isError ? <p className="text-sm text-destructive">Runtime 事件加载失败</p> : null}
        {!isLoading && events.length === 0 ? (
          <EmptyLine>{hasAppliedFilter ? "筛选后无 Runtime 事件" : "暂无 Runtime 事件"}</EmptyLine>
        ) : null}
        {events.length > 0 ? (
          <div className="flex flex-col gap-3">
            {events.map((event) => (
              <EventRow key={event.id} event={event} showDetails />
            ))}
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

function RuntimeSelectFilter({
  id,
  label,
  onValueChange,
  options,
  value,
}: {
  id: string;
  label: string;
  onValueChange: (value: string | undefined) => void;
  options: Array<{ label: string; value: string }>;
  value?: string;
}) {
  return (
    <div className="flex min-w-0 flex-col gap-2">
      <Label htmlFor={id}>{label}</Label>
      <Select value={value ?? "all"} onValueChange={(nextValue) => onValueChange(nextValue === "all" ? undefined : nextValue)}>
        <SelectTrigger id={id} className="w-full">
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

function EventRow({ event, showDetails }: { event: RuntimeEvent; showDetails?: boolean }) {
  return (
    <div className="flex min-w-0 gap-3 rounded-md border bg-card/70 p-3">
      <SemanticIconTile tone={severityTone[event.severity]} size="sm">
        {event.severity === "error" ? <AlertTriangle /> : <Clock />}
      </SemanticIconTile>
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="truncate font-medium">{event.title}</span>
          <StatusBadge tone={severityTone[event.severity]}>{severityLabel[event.severity]}</StatusBadge>
        </div>
        <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
          <span>{event.event_type}</span>
          <span>{event.source}</span>
          <span>节点：{event.node_id ?? event.runtime_node_id ?? "-"}</span>
          <span>Provider：{event.provider_type ?? "-"}</span>
          <span>{formatTime(event.created_at)}</span>
        </div>
        {showDetails && event.description ? <p className="mt-2 text-sm text-muted-foreground">{event.description}</p> : null}
      </div>
    </div>
  );
}

function MetricLite({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-md bg-muted/40 px-3 py-2">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="mt-1 text-lg font-semibold tracking-normal">{value}</div>
    </div>
  );
}

function EmptyLine({ children }: { children: string }) {
  return <p className="rounded-md border border-dashed bg-muted/20 px-3 py-4 text-sm text-muted-foreground">{children}</p>;
}

function MutationErrorLine({ error, fallback }: { error: unknown; fallback: string }) {
  return (
    <p className="rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
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
    supportedProviders: readStringArray(requestPayload.supported_providers),
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
    month: "2-digit",
  }).format(date);
}

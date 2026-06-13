import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { ArrowLeft, Play, Square } from "lucide-react";
import type { ReactNode } from "react";
import { useState } from "react";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { ApiRequestError } from "@/lib/api/client";
import type { DigitalEmployeeRun, DigitalEmployeeRunEvent, DigitalEmployeeRunStatus } from "@/lib/api/employees";
import {
  createDigitalEmployeeRun,
  getDigitalEmployee,
  getDigitalEmployeeExecutionInstance,
  listDigitalEmployeeRunEvents,
  listDigitalEmployeeRuns,
  stopDigitalEmployeeRun,
} from "@/lib/api/employees";
import { getRuntimeOverview } from "@/lib/api/runtime";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { cn } from "@/lib/utils";
import { EmployeeAvatar } from "./avatar";
import { employeeAvatarAsset } from "./avatar-library";

const activeRunStatuses = new Set<DigitalEmployeeRunStatus>(["queued", "dispatching", "running", "cancelling"]);
const failedRunStatuses = new Set<DigitalEmployeeRunStatus>(["failed", "cancelled", "timed_out"]);

export function EmployeeDetailPage({ employeeId }: { employeeId: string }) {
  const apiBaseUrl = resolveControlPlaneUrl();

  return <EmployeeDetailView apiBaseUrl={apiBaseUrl} employeeId={employeeId} />;
}

type EmployeeDetailViewProps = {
  apiBaseUrl: string;
  employeeId: string;
  fetcher?: typeof fetch;
};

export function EmployeeDetailView({ apiBaseUrl, employeeId, fetcher }: EmployeeDetailViewProps) {
  const apiOptions = { baseUrl: apiBaseUrl, fetcher };
  const queryClient = useQueryClient();
  const [objective, setObjective] = useState("");
  const [prompt, setPrompt] = useState("");
  const [selectedRunId, setSelectedRunId] = useState<string | null>(null);
  const runsQueryKey = ["digital-employee-runs", employeeId, { limit: 10 }] as const;
  const employee = useQuery({
    queryKey: ["digital-employee", employeeId],
    queryFn: () => getDigitalEmployee(apiOptions, employeeId),
  });
  const instance = useQuery({
    queryKey: ["digital-employee-execution-instance", employeeId],
    queryFn: () => getDigitalEmployeeExecutionInstance(apiOptions, employeeId),
    retry: false,
  });
  const runs = useQuery({
    queryKey: runsQueryKey,
    queryFn: () => listDigitalEmployeeRuns(apiOptions, employeeId, { limit: 10 }),
    refetchInterval: (query) => {
      const data = query.state.data as DigitalEmployeeRun[] | undefined;
      return data?.some((run) => isActiveRun(run.status)) ? 2500 : false;
    },
  });
  const runtimeOverview = useQuery({
    queryKey: ["runtime-overview"],
    queryFn: () => getRuntimeOverview(apiOptions),
    refetchInterval: 5000,
  });
  const runList = runs.data ?? [];
  const latestRun = runList[0];
  const selectedRunFromList = selectedRunId ? runList.find((run) => run.id === selectedRunId) : undefined;
  const selectedRunMissing = Boolean(selectedRunId && runs.isSuccess && !selectedRunFromList);
  const selectedRun = selectedRunId ? selectedRunFromList : latestRun;
  const events = useQuery({
    enabled: Boolean(selectedRun?.id),
    queryKey: ["digital-employee-run-events", employeeId, selectedRun?.id, { limit: 50 }],
    queryFn: () => listDigitalEmployeeRunEvents(apiOptions, employeeId, selectedRun?.id ?? "", { limit: 50 }),
    refetchInterval: selectedRun && isActiveRun(selectedRun.status) ? 2500 : false,
  });
  const putRunInCache = (run: DigitalEmployeeRun, prepend: boolean) => {
    queryClient.setQueryData<DigitalEmployeeRun[]>(runsQueryKey, (current = []) => {
      const withoutRun = current.filter((item) => item.id !== run.id);
      if (prepend) {
        return [run, ...withoutRun].slice(0, 10);
      }
      const replaced = current.some((item) => item.id === run.id);
      const next = current.map((item) => (item.id === run.id ? run : item));
      return (replaced ? next : [run, ...next]).slice(0, 10);
    });
  };
  const refreshRunFacts = async () => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ["digital-employee-runs", employeeId] }),
      queryClient.invalidateQueries({ queryKey: ["digital-employee-run-events", employeeId] }),
    ]);
  };
  const stopRun = useMutation({
    mutationFn: (run: DigitalEmployeeRun) =>
      stopDigitalEmployeeRun(apiOptions, employeeId, run.id, { reason: "用户从 Web 停止" }),
    onSuccess: async (run) => {
      setSelectedRunId(run.id);
      putRunInCache(run, false);
      await refreshRunFacts();
    },
  });
  const createRun = useMutation({
    mutationFn: () =>
      createDigitalEmployeeRun(apiOptions, employeeId, {
        objective: objective.trim(),
        prompt: prompt.trim(),
      }),
    onSuccess: async (run) => {
      setSelectedRunId(run.id);
      putRunInCache(run, true);
      setObjective("");
      setPrompt("");
      await refreshRunFacts();
    },
  });
  const instanceNotFound = instance.error instanceof ApiRequestError && instance.error.status === 404;
  const hasActiveRun = runList.some((run) => isActiveRun(run.status));
  const employeeCanRun = employee.data?.status === "ready" || employee.data?.status === "active";
  const executionInstanceCanRun =
    instance.isSuccess && (instance.data.status === "ready" || instance.data.status === "active");
  const executionRuntimeNodeId = instance.data?.runtime_node_id;
  const selectedRunNodeId = selectedRun?.node_id;
  const runtimeNode = runtimeOverview.data?.nodes.find(
    (node) =>
      (executionRuntimeNodeId && node.runtime_node_id === executionRuntimeNodeId) ||
      (selectedRunNodeId && node.node_id === selectedRunNodeId),
  );
  const runtimeCommandChannelDisconnected =
    runtimeOverview.isSuccess && runtimeNode?.command_channel_connected === false;
  const canStartTask =
    employeeCanRun && executionInstanceCanRun && runs.isSuccess && !hasActiveRun && !runtimeCommandChannelDisconnected;
  const trimmedObjective = objective.trim();
  const avatarAsset = employee.data ? employeeAvatarAsset(employee.data) : null;

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main className="min-w-0 overflow-x-hidden">
        <div className="mb-4 flex min-w-0 flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex min-w-0 items-center gap-3">
            <EmployeeAvatar asset={avatarAsset} name={employee.data?.name ?? "数字员工详情"} size="md" />
            <div className="min-w-0">
              <h1 className="truncate text-2xl font-bold tracking-tight">{employee.data?.name ?? "数字员工详情"}</h1>
              <p className="text-sm text-muted-foreground">执行实例、运行事件、结果和人工停止。</p>
            </div>
          </div>
          <Button asChild className="self-start sm:self-auto" type="button" variant="outline">
            <Link to="/employees">
              <ArrowLeft className="size-4" />
              返回列表
            </Link>
          </Button>
        </div>

        {employee.isLoading ? <p className="text-sm text-muted-foreground">加载中</p> : null}
        {employee.isError ? <p className="text-sm text-destructive">数字员工加载失败</p> : null}

        {employee.data ? (
          <div className="space-y-4">
            <Card>
              <CardHeader>
                <CardTitle>概览</CardTitle>
              </CardHeader>
              <CardContent>
                <div className="grid gap-3 md:grid-cols-4">
                  <SummaryItem label="角色" value={employee.data.role} />
                  <SummaryItem label="状态" value={<Badge variant={employee.data.status === "active" ? "default" : "secondary"}>{employee.data.status}</Badge>} />
                  <SummaryItem label="风险" value={employee.data.risk_level ?? "medium"} />
                  <SummaryItem
                    label="执行实例"
                    value={
                      instance.data
                        ? `${instance.data.provider_type} · ${instance.data.status}`
                        : instanceNotFound
                          ? "未绑定 Runtime"
                          : "加载中"
                    }
                  />
                </div>
                <p className="mt-3 text-sm text-muted-foreground">{employee.data.description || "暂无描述"}</p>
                {instance.data ? (
                  <p className="mt-2 truncate text-xs text-muted-foreground">Runtime：{instance.data.runtime_node_id}</p>
                ) : null}
                {instance.isError && !instanceNotFound ? (
                  <p className="mt-2 text-xs text-destructive">执行实例加载失败</p>
                ) : null}
              </CardContent>
            </Card>

            <section className="grid gap-4 lg:grid-cols-[minmax(0,1.2fr)_minmax(320px,0.8fr)]">
              <div className="rounded-md border p-4">
                <div className="mb-3 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
                  <div>
                    <h2 className="text-base font-semibold">当前运行</h2>
                    <p className="text-sm text-muted-foreground">最新一次数字员工执行事实。</p>
                  </div>
                  {selectedRun ? <RunStatusBadge status={selectedRun.status} /> : null}
                </div>
                {runs.isLoading ? <p className="text-sm text-muted-foreground">运行加载中</p> : null}
                {runs.isError ? <p className="text-sm text-destructive">运行加载失败</p> : null}
                {selectedRunMissing ? (
                  <div className="rounded-md border border-dashed p-3">
                    <p className="text-sm text-muted-foreground">选择的运行已不在当前列表</p>
                    <Button className="mt-2" onClick={() => setSelectedRunId(null)} size="sm" type="button" variant="outline">
                      查看最新运行
                    </Button>
                  </div>
                ) : selectedRun ? (
                  <div className="space-y-3">
                    <div className="grid gap-2 text-sm md:grid-cols-2">
                      <SummaryItem label="命令" value={selectedRun.command_id} />
                      <SummaryItem label="Provider" value={selectedRun.provider_type} />
                      <SummaryItem label="节点" value={selectedRun.node_id || selectedRun.runtime_node_id} />
                      <SummaryItem label="更新时间" value={selectedRun.updated_at ?? selectedRun.created_at ?? "-"} />
                    </div>
                    {isFailedRun(selectedRun.status) ? <FailureBlock run={selectedRun} /> : null}
                    {selectedRun.status === "completed" ? <ResultBlock run={selectedRun} /> : null}
                    {isActiveRun(selectedRun.status) ? (
                      <Button
                        disabled={selectedRun.status === "cancelling" || stopRun.isPending}
                        onClick={() => stopRun.mutate(selectedRun)}
                        type="button"
                        variant="destructive"
                      >
                        <Square className="size-4" />
                        停止
                      </Button>
                    ) : null}
                    {stopRun.isError ? <p className="text-sm text-destructive">停止失败</p> : null}
                    {runList.length > 1 ? (
                      <div className="space-y-2">
                        <p className="text-sm font-medium">运行历史</p>
                        <div className="flex flex-wrap gap-2">
                          {runList.map((run) => (
                            <Button
                              aria-pressed={run.id === selectedRun?.id}
                              key={run.id}
                              onClick={() => setSelectedRunId(run.id)}
                              size="sm"
                              type="button"
                              variant={run.id === selectedRun?.id ? "secondary" : "outline"}
                            >
                              {runHistoryLabel(run)}
                            </Button>
                          ))}
                        </div>
                      </div>
                    ) : null}
                  </div>
                ) : !runs.isLoading ? (
                  <p className="text-sm text-muted-foreground">暂无运行记录</p>
                ) : null}
              </div>

              <div className="rounded-md border p-4">
                <h2 className="text-base font-semibold">开始任务</h2>
                <form
                  className="mt-3 space-y-3"
                  onSubmit={(event) => {
                    event.preventDefault();
                    if (canStartTask && trimmedObjective) {
                      createRun.mutate();
                    }
                  }}
                >
                  <div className="space-y-2">
                    <Label htmlFor="run-objective">任务目标</Label>
                    <Textarea
                      disabled={!canStartTask || createRun.isPending}
                      id="run-objective"
                      onChange={(event) => setObjective(event.target.value)}
                      rows={2}
                      value={objective}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="run-prompt">任务提示</Label>
                    <Textarea
                      disabled={!canStartTask || createRun.isPending}
                      id="run-prompt"
                      onChange={(event) => setPrompt(event.target.value)}
                      rows={4}
                      value={prompt}
                    />
                  </div>
                  <Button disabled={!canStartTask || !trimmedObjective || createRun.isPending} type="submit">
                    <Play className="size-4" />
                    开始任务
                  </Button>
                  {hasActiveRun ? <p className="text-xs text-muted-foreground">当前已有活跃运行</p> : null}
                  {!executionInstanceCanRun && instance.isSuccess ? (
                    <p className="text-xs text-muted-foreground">执行实例当前不可执行</p>
                  ) : null}
                  {runtimeCommandChannelDisconnected ? (
                    <p className="text-xs text-muted-foreground">Runtime 命令通道未连接，暂不能开始任务</p>
                  ) : null}
                  {instanceNotFound ? <p className="text-xs text-muted-foreground">未绑定 Runtime，暂不能开始任务</p> : null}
                  {runs.isError ? <p className="text-xs text-destructive">运行列表加载失败，暂不能开始新任务</p> : null}
                  {createRun.isError ? <p className="text-sm text-destructive">开始任务失败</p> : null}
                </form>
              </div>
            </section>

            <section className="rounded-md border p-4">
              <div className="mb-3 flex items-center justify-between gap-3">
                <div>
                  <h2 className="text-base font-semibold">事件流</h2>
                  <p className="text-sm text-muted-foreground">Runtime 和 Provider 回写事件。</p>
                </div>
                {events.data ? <Badge variant="secondary">{events.data.length}</Badge> : null}
              </div>
              {selectedRunMissing ? <p className="text-sm text-muted-foreground">选择的运行已不在当前列表</p> : null}
              {!selectedRunMissing && events.isLoading ? <p className="text-sm text-muted-foreground">事件加载中</p> : null}
              {!selectedRunMissing && events.isError ? <p className="text-sm text-destructive">事件加载失败</p> : null}
              {!selectedRunMissing && events.data?.length ? (
                <div className="space-y-2">
                  {events.data.map((event) => (
                    <RunEventRow event={event} key={`${event.sequence_number}-${event.event_type}`} />
                  ))}
                </div>
              ) : !selectedRunMissing && !events.isLoading ? (
                <p className="text-sm text-muted-foreground">暂无事件</p>
              ) : null}
            </section>
          </div>
        ) : null}
      </Main>
    </>
  );
}

function SummaryItem({ label, value }: { label: string; value: ReactNode }) {
  const isTextValue = typeof value === "string" || typeof value === "number";

  return (
    <div className="min-w-0 rounded-md border bg-muted/20 px-3 py-2">
      <p className="text-xs text-muted-foreground">{label}</p>
      <div className={cn("mt-1 text-sm font-medium", isTextValue ? "truncate" : "flex items-center")}>{value}</div>
    </div>
  );
}

function RunStatusBadge({ status }: { status: DigitalEmployeeRunStatus }) {
  const label = runStatusLabel(status);
  const variant = isFailedRun(status) ? "destructive" : status === "completed" ? "default" : "secondary";

  return <Badge variant={variant}>{label}</Badge>;
}

function FailureBlock({ run }: { run: DigitalEmployeeRun }) {
  return (
    <div className="rounded-md border border-destructive/30 bg-destructive/5 p-3">
      <p className="text-sm font-medium text-destructive">失败原因</p>
      <p className="mt-1 text-sm">{failureReason(run)}</p>
    </div>
  );
}

function ResultBlock({ run }: { run: DigitalEmployeeRun }) {
  return (
    <div>
      <p className="text-sm font-medium">结果</p>
      <pre className="mt-2 max-h-72 overflow-auto rounded-md border bg-muted/30 p-3 text-xs">{compactJson(run.result)}</pre>
    </div>
  );
}

function RunEventRow({ event }: { event: DigitalEmployeeRunEvent }) {
  return (
    <div className="grid gap-2 rounded-md border px-3 py-2 md:grid-cols-[120px_160px_minmax(0,1fr)]">
      <p className="text-sm font-medium">#{event.sequence_number}</p>
      <p className="truncate text-sm">{event.event_type}</p>
      <pre className="min-w-0 overflow-auto whitespace-pre-wrap break-words text-xs text-muted-foreground">
        {compactJson(event.payload)}
      </pre>
    </div>
  );
}

function isActiveRun(status: DigitalEmployeeRunStatus) {
  return activeRunStatuses.has(status);
}

function isFailedRun(status: DigitalEmployeeRunStatus) {
  return failedRunStatuses.has(status);
}

function runStatusLabel(status: DigitalEmployeeRunStatus) {
  switch (status) {
    case "queued":
      return "排队中";
    case "dispatching":
      return "调度中";
    case "running":
      return "执行中";
    case "cancelling":
      return "取消中";
    case "completed":
      return "已完成";
    case "failed":
      return "失败";
    case "cancelled":
      return "已取消";
    case "timed_out":
      return "已超时";
  }
}

function runHistoryLabel(run: DigitalEmployeeRun) {
  return `${runStatusLabel(run.status)} · ${runTimeLabel(run.updated_at ?? run.created_at)} · ${shortRunId(run.id)}`;
}

function runTimeLabel(value?: string) {
  if (!value) {
    return "未知时间";
  }
  const match = value.match(/^\d{4}-(\d{2})-(\d{2})T(\d{2}:\d{2})/);
  if (match) {
    return `${match[1]}-${match[2]} ${match[3]}`;
  }
  return value;
}

function shortRunId(id: string) {
  return id.slice(0, 8);
}

function failureReason(run: DigitalEmployeeRun) {
  return run.error_message || compactJson(run.diagnostic) || compactJson(run.result) || "未提供失败原因";
}

function compactJson(value: unknown) {
  if (!value || (typeof value === "object" && Object.keys(value).length === 0)) {
    return "";
  }

  return JSON.stringify(value, null, 2);
}

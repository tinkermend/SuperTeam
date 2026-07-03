import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Square } from "lucide-react";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { StatusPill, V3Button, type V3Tone } from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  listDigitalEmployeeRunEvents,
  stopDigitalEmployeeRun,
  type DigitalEmployeeRun,
  type DigitalEmployeeRunEvent,
  type DigitalEmployeeRunListItem,
  type DigitalEmployeeRunStatus,
} from "@/lib/api/employees";

const activeRunStatuses = new Set<DigitalEmployeeRunStatus>(["queued", "dispatching", "running", "cancelling"]);
const failedRunStatuses = new Set<DigitalEmployeeRunStatus>(["failed", "cancelled", "timed_out"]);

type RunDetailDrawerProps = {
  apiOptions: ApiClientOptions;
  employeeId: string;
  run: DigitalEmployeeRunListItem | undefined;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onStopped: (run: DigitalEmployeeRun) => void;
};

export function RunDetailDrawer({ apiOptions, employeeId, run, open, onOpenChange, onStopped }: RunDetailDrawerProps) {
  const queryClient = useQueryClient();
  const events = useQuery({
    enabled: Boolean(run?.id) && open,
    queryKey: ["digital-employee-run-events", employeeId, run?.id, { limit: 50 }],
    queryFn: () => listDigitalEmployeeRunEvents(apiOptions, employeeId, run?.id ?? "", { limit: 50 }),
    refetchInterval: run && isActiveRun(run.status) ? 2500 : false,
  });
  const stopRun = useMutation({
    mutationFn: (target: DigitalEmployeeRunListItem) =>
      stopDigitalEmployeeRun(apiOptions, employeeId, target.id, { reason: "用户从 Web 停止" }),
    onSuccess: async (updatedRun) => {
      onStopped(updatedRun);
      await queryClient.invalidateQueries({ queryKey: ["digital-employee-run-events", employeeId, updatedRun.id] });
    },
  });

  // After a successful stop, prefer the mutation result so the pill and Stop button reflect the
  // new status immediately — BUT only while the `run` prop hasn't caught up. Once the parent's
  // list refetch returns and passes a terminal-status prop (e.g. "cancelled"), trust the prop
  // over the (now stale) mutation state. This avoids the drawer getting stuck on "取消中" after
  // the run has actually transitioned to "已取消". Gating on `isActiveRun(run.status)` is what
  // distinguishes the two phases: stop returns while prop is still active → use stopRun.data;
  // refetch lands with a terminal prop → use prop.
  const displayedRun =
    stopRun.data && run && isActiveRun(run.status) && stopRun.data.id === run.id ? stopRun.data : run;

  if (!run) {
    return null;
  }

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="w-full gap-0 overflow-y-auto sm:max-w-xl" side="right">
        <SheetHeader>
          <SheetTitle className="flex items-center gap-2">
            {displayedRun.task_title}
            <RunStatusPill status={displayedRun.status} />
          </SheetTitle>
        </SheetHeader>
        <div className="flex flex-col gap-4 px-4 pb-6">
          <div className="grid gap-2 text-sm md:grid-cols-2">
            <SummaryItem label="命令" value={displayedRun.command_id} />
            <SummaryItem label="Provider" value={displayedRun.provider_type} />
            <SummaryItem label="节点" value={displayedRun.node_id || displayedRun.runtime_node_id} />
            <SummaryItem label="更新时间" value={displayedRun.updated_at ?? displayedRun.created_at ?? "-"} />
          </div>
          {isFailedRun(displayedRun.status) ? <FailureBlock run={displayedRun} /> : null}
          {displayedRun.status === "completed" ? <ResultBlock run={displayedRun} /> : null}
          {isActiveRun(displayedRun.status) ? (
            <V3Button
              disabled={displayedRun.status === "cancelling" || stopRun.isPending}
              onClick={() => stopRun.mutate(displayedRun)}
              type="button"
              variant="danger"
            >
              <Square className="size-4" />
              停止
            </V3Button>
          ) : null}
          {stopRun.isError ? <p className="text-sm text-destructive">停止失败</p> : null}
          <div>
            <div className="mb-2 flex items-center justify-between">
              <p className="text-sm font-semibold">事件流</p>
              {events.data ? <span className="text-xs text-v3-ink-3">{events.data.length} 条</span> : null}
            </div>
            {events.isLoading ? <p className="text-sm text-v3-ink-2">事件加载中</p> : null}
            {events.isError ? <p className="text-sm text-destructive">事件加载失败</p> : null}
            {events.data?.length ? (
              <div className="space-y-2">
                {events.data.map((event) => (
                  <RunEventRow event={event} key={`${event.sequence_number}-${event.event_type}`} />
                ))}
              </div>
            ) : !events.isLoading ? (
              <p className="text-sm text-v3-ink-2">暂无事件</p>
            ) : null}
          </div>
        </div>
      </SheetContent>
    </Sheet>
  );
}

function SummaryItem({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-md border border-v3-line bg-v3-card-soft px-3 py-2">
      <p className="text-xs text-v3-ink-3">{label}</p>
      <p className="mt-1 truncate text-sm font-medium text-v3-ink">{value}</p>
    </div>
  );
}

function RunStatusPill({ status }: { status: DigitalEmployeeRunStatus }) {
  const tone: V3Tone = isFailedRun(status) ? "danger" : status === "completed" ? "ok" : "mute";
  return <StatusPill tone={tone}>{runStatusLabel(status)}</StatusPill>;
}

function FailureBlock({ run }: { run: DigitalEmployeeRunListItem }) {
  return (
    <div className="rounded-md border border-destructive/30 bg-destructive/5 p-3">
      <p className="text-sm font-medium text-destructive">失败原因</p>
      <p className="mt-1 text-sm">{failureReason(run)}</p>
    </div>
  );
}

function ResultBlock({ run }: { run: DigitalEmployeeRunListItem }) {
  return (
    <div>
      <p className="text-sm font-medium">结果</p>
      <pre className="mt-2 max-h-72 overflow-auto rounded-md border border-v3-line bg-v3-card-soft p-3 text-xs">
        {compactJson(run.result)}
      </pre>
    </div>
  );
}

function RunEventRow({ event }: { event: DigitalEmployeeRunEvent }) {
  return (
    <div className="grid gap-2 rounded-md border border-v3-line px-3 py-2 md:grid-cols-[120px_160px_minmax(0,1fr)]">
      <p className="text-sm font-medium">#{event.sequence_number}</p>
      <p className="truncate text-sm">{event.event_type}</p>
      <pre className="min-w-0 overflow-auto whitespace-pre-wrap break-words text-xs text-v3-ink-2">
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

function failureReason(run: DigitalEmployeeRunListItem) {
  return run.error_message || compactJson(run.diagnostic) || compactJson(run.result) || "未提供失败原因";
}

function compactJson(value: unknown) {
  if (!value || (typeof value === "object" && Object.keys(value).length === 0)) {
    return "";
  }
  return JSON.stringify(value, null, 2);
}

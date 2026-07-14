import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Square } from "lucide-react";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { StatusPill, V3Button, type V3Tone } from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  listDigitalEmployeeRunEvents,
  stopDigitalEmployeeRun,
  type DigitalEmployeeRun,
  type DigitalEmployeeRunListItem,
  type DigitalEmployeeRunStatus,
} from "@/lib/api/employees";
import { formatDateTime } from "@/lib/format-time";
import { runStatusLabel } from "@/lib/status-labels";
import { providerDisplayName } from "../provider-label";
import { RunEventTimeline } from "./run-event-timeline";

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
  if (!run) {
    return null;
  }

  const displayedRun: DigitalEmployeeRunListItem =
    stopRun.data && isActiveRun(run.status) && stopRun.data.id === run.id ? { ...run, ...stopRun.data } : run;

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
            <SummaryItem label="命令" mono value={displayedRun.command_id} />
            <SummaryItem label="Provider" value={providerDisplayName(displayedRun.provider_type)} />
            <SummaryItem label="节点" mono value={displayedRun.node_id || displayedRun.runtime_node_id} />
            <SummaryItem label="更新时间" value={formatRunTimestamp(displayedRun)} />
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
              <RunEventTimeline events={events.data} limitReached={events.data.length >= 50} />
            ) : !events.isLoading ? (
              <p className="text-sm text-v3-ink-2">暂无事件</p>
            ) : null}
          </div>
        </div>
      </SheetContent>
    </Sheet>
  );
}

function SummaryItem({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="min-w-0 rounded-md border border-v3-line bg-v3-card-soft px-3 py-2">
      <p className="text-xs text-v3-ink-3">{label}</p>
      <p className={mono ? "mt-1 truncate font-mono text-xs text-v3-ink" : "mt-1 truncate text-sm font-medium text-v3-ink"}>
        {value}
      </p>
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
  const [rawOpen, setRawOpen] = useState(false);
  const conclusion = extractResultText(run.result);
  const rawJson = compactJson(run.result);
  return (
    <div>
      <p className="text-sm font-medium">结果</p>
      {conclusion ? (
        <p className="mt-2 whitespace-pre-wrap break-words rounded-md border border-v3-line bg-v3-card-soft p-3 text-sm leading-6 text-v3-ink">
          {conclusion}
        </p>
      ) : null}
      {rawJson && conclusion ? (
        <details className="mt-2" onToggle={(event) => setRawOpen(event.currentTarget.open)}>
          <summary className="cursor-pointer text-xs text-v3-ink-3">原始结果 JSON</summary>
          {rawOpen ? (
            <pre className="mt-2 max-h-72 overflow-auto rounded-md border border-v3-line bg-v3-card-soft p-3 text-xs">
              {rawJson}
            </pre>
          ) : null}
        </details>
      ) : null}
      {rawJson && !conclusion ? (
        <pre className="mt-2 max-h-72 overflow-auto rounded-md border border-v3-line bg-v3-card-soft p-3 text-xs">
          {rawJson}
        </pre>
      ) : null}
      {!rawJson && !conclusion ? <p className="mt-2 text-sm text-v3-ink-2">无结果数据</p> : null}
    </div>
  );
}

function isActiveRun(status: DigitalEmployeeRunStatus) {
  return activeRunStatuses.has(status);
}

function isFailedRun(status: DigitalEmployeeRunStatus) {
  return failedRunStatuses.has(status);
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

function formatRunTimestamp(run: DigitalEmployeeRunListItem) {
  const value = run.updated_at ?? run.created_at;
  return value ? formatDateTime(value) : "-";
}

const RESULT_TEXT_KEYS = ["summary", "conclusion", "text", "message"] as const;

function extractResultText(result: unknown): string | undefined {
  if (!result || typeof result !== "object") return undefined;
  const record = result as Record<string, unknown>;
  for (const key of RESULT_TEXT_KEYS) {
    const value = record[key];
    if (typeof value === "string" && value.trim()) return value;
  }
  return undefined;
}

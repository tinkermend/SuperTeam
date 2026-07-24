import { useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { RotateCcw, Square } from "lucide-react";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { MarkdownProse, StatusPill, Button, type Tone } from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  acknowledgeDigitalEmployeeRunFailure,
  listDigitalEmployeeRunEvents,
  retryDigitalEmployeeRunFailure,
  stopDigitalEmployeeRun,
  type DigitalEmployeeRun,
  type DigitalEmployeeRunListItem,
  type DigitalEmployeeRunStatus
} from "@/lib/api/employees";
import { formatDateTime } from "@/lib/format-time";
import { runStatusLabel, statusLabel } from "@/lib/status-labels";
import { cn } from "@/lib/utils";
import { providerDisplayName } from "../provider-label";
import { RunEventTimeline } from "./run-event-timeline";

const activeRunStatuses = new Set<DigitalEmployeeRunStatus>(["queued", "dispatching", "running", "cancelling"]);
const failedRunStatuses = new Set<DigitalEmployeeRunStatus>(["failed", "cancelled", "timed_out"]);
const recoverableRunStatuses = new Set<DigitalEmployeeRunStatus>(["failed", "timed_out"]);

function runProjectDisplayName(name: string | undefined): string {
  return name?.trim() || "项目详情";
}

function RunProjectValue({
  projectId,
  projectName,
  projectDeleted
}: {
  projectId?: string;
  projectName?: string;
  projectDeleted?: boolean;
}) {
  if (!projectId) {
    return "无关联项目";
  }
  const name = runProjectDisplayName(projectName);
  if (projectDeleted) {
    return (
      <span className="font-medium text-ink">
        {name}
        <span className="ml-1 font-normal text-ink-3">（{statusLabel("deleted")}）</span>
      </span>
    );
  }
  return (
    <Link
      className="font-medium text-brand underline-offset-2 hover:underline"
      params={{ projectId }}
      to="/projects/$projectId"
    >
      {name}
    </Link>
  );
}

type RunDetailDrawerProps = {
  apiOptions: ApiClientOptions;
  employeeId: string;
  run: DigitalEmployeeRunListItem | undefined;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onStopped: (run: DigitalEmployeeRun) => void;
  onRecovered?: (run: DigitalEmployeeRun) => void;
};

const EVENT_DISPLAY_LIMIT = 50;

export function RunDetailDrawer({
  apiOptions,
  employeeId,
  run,
  open,
  onOpenChange,
  onStopped,
  onRecovered
}: RunDetailDrawerProps) {
  const queryClient = useQueryClient();
  // 多取一条(51)只用于判断是否真的被截断:恰好 50 条时不该显示「仅显示前 50 条」提示。
  const events = useQuery({
    enabled: Boolean(run?.id) && open,
    queryKey: ["digital-employee-run-events", employeeId, run?.id, { limit: EVENT_DISPLAY_LIMIT + 1 }],
    queryFn: () =>
      listDigitalEmployeeRunEvents(apiOptions, employeeId, run?.id ?? "", { limit: EVENT_DISPLAY_LIMIT + 1 }),
    refetchInterval: run && isActiveRun(run.status) ? 2500 : false
});
  const stopRun = useMutation({
    mutationFn: (target: DigitalEmployeeRunListItem) =>
      stopDigitalEmployeeRun(apiOptions, employeeId, target.id, { reason: "用户从 Web 停止" }),
    onSuccess: async (updatedRun) => {
      onStopped(updatedRun);
      await queryClient.invalidateQueries({ queryKey: ["digital-employee-run-events", employeeId, updatedRun.id] });
    }
});
  const acknowledgeFailure = useMutation({
    mutationFn: (target: DigitalEmployeeRunListItem) =>
      acknowledgeDigitalEmployeeRunFailure(apiOptions, employeeId, target.id),
    onSuccess: async (updatedRun) => {
      onRecovered?.(updatedRun);
      onStopped(updatedRun);
      await queryClient.invalidateQueries({ queryKey: ["digital-employee-runs", employeeId] });
      await queryClient.invalidateQueries({ queryKey: ["digital-employee", employeeId] });
      await queryClient.invalidateQueries({ queryKey: ["inbox"] });
    }
});
  const retryFailure = useMutation({
    mutationFn: (target: DigitalEmployeeRunListItem) =>
      retryDigitalEmployeeRunFailure(apiOptions, employeeId, target.id),
    onSuccess: async (createdRun) => {
      onRecovered?.(createdRun);
      await queryClient.invalidateQueries({ queryKey: ["digital-employee-runs", employeeId] });
      await queryClient.invalidateQueries({ queryKey: ["digital-employee", employeeId] });
      await queryClient.invalidateQueries({ queryKey: ["inbox"] });
      onOpenChange(false);
    }
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
    acknowledgeFailure.data && acknowledgeFailure.data.id === run.id
      ? { ...run, ...acknowledgeFailure.data }
      : stopRun.data && isActiveRun(run.status) && stopRun.data.id === run.id
        ? { ...run, ...stopRun.data }
        : run;
  const displayedEvents = events.data?.slice(0, EVENT_DISPLAY_LIMIT);
  const eventsTruncated = (events.data?.length ?? 0) > EVENT_DISPLAY_LIMIT;
  // 项目任务执行由协调线程的 failure recovery 处置;抽屉只处理 standalone run。
  // list 的 project_id 来自 project_tasks.digital_employee_run_id 关联。
  const isProjectLinkedRun = Boolean(displayedRun.project_id);
  const canRecoverFailure =
    isRecoverableRun(displayedRun.status) &&
    !displayedRun.failure_acknowledged_at &&
    !isProjectLinkedRun;
  const recoveryPending = acknowledgeFailure.isPending || retryFailure.isPending;

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent
        className="w-[min(880px,calc(100vw-2rem))] max-w-[calc(100vw-2rem)] gap-0 overflow-y-auto sm:max-w-[880px]"
        side="right"
      >
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
            <SummaryItem
              className="md:col-span-2"
              label="所属项目"
              value={
                <RunProjectValue
                  projectDeleted={displayedRun.project_deleted}
                  projectId={displayedRun.project_id}
                  projectName={displayedRun.project_name}
                />
              }
            />
          </div>
          {isFailedRun(displayedRun.status) ? <FailureBlock run={displayedRun} /> : null}
          {/* key=run.id:切换到另一条运行时重挂载,重置原始 JSON 折叠的展开态 */}
          {displayedRun.status === "completed" ? <ResultBlock key={displayedRun.id} run={displayedRun} /> : null}
          {isActiveRun(displayedRun.status) ? (
            <Button
              disabled={displayedRun.status === "cancelling" || stopRun.isPending}
              onClick={() => stopRun.mutate(displayedRun)}
              type="button"
              variant="danger"
            >
              <Square className="size-4" />
              停止
            </Button>
          ) : null}
          {canRecoverFailure ? (
            <div className="flex flex-wrap gap-2">
              <Button
                disabled={recoveryPending}
                onClick={() => retryFailure.mutate(displayedRun)}
                type="button"
                variant="primary"
              >
                <RotateCcw className="size-4" />
                重试
              </Button>
              <Button
                disabled={recoveryPending}
                onClick={() => acknowledgeFailure.mutate(displayedRun)}
                type="button"
                variant="outline"
              >
                确认关闭
              </Button>
            </div>
          ) : null}
          {isProjectLinkedRun && isRecoverableRun(displayedRun.status) ? (
            <p className="text-sm text-ink-2">
              {displayedRun.project_deleted ? (
                <>
                  此运行属于已删除项目「{runProjectDisplayName(displayedRun.project_name)}」，失败恢复请在收件箱处理。
                </>
              ) : (
                <>
                  此运行属于项目任务，失败恢复请在
                  {displayedRun.project_id ? (
                    <>
                      {" "}
                      <Link
                        className="font-medium text-brand underline-offset-2 hover:underline"
                        params={{ projectId: displayedRun.project_id }}
                        to="/projects/$projectId"
                      >
                        {runProjectDisplayName(displayedRun.project_name)}
                      </Link>
                      {" "}
                      或收件箱处理
                    </>
                  ) : (
                    " 项目详情或收件箱处理"
                  )}
                  。
                </>
              )}
            </p>
          ) : null}
          {displayedRun.failure_acknowledged_at ? (
            <p className="text-sm text-ink-2">失败已确认关闭</p>
          ) : null}
          {stopRun.isError ? <p className="text-sm text-destructive">停止失败</p> : null}
          {acknowledgeFailure.isError ? (
            <p className="text-sm text-destructive">{formatRecoveryError(acknowledgeFailure.error, "确认关闭失败")}</p>
          ) : null}
          {retryFailure.isError ? (
            <p className="text-sm text-destructive">{formatRecoveryError(retryFailure.error, "重试失败")}</p>
          ) : null}
          <div>
            <div className="mb-2 flex items-center justify-between">
              <p className="text-sm font-semibold">事件流</p>
              {displayedEvents ? <span className="text-xs text-ink-3">{displayedEvents.length} 条</span> : null}
            </div>
            {events.isLoading ? <p className="text-sm text-ink-2">事件加载中</p> : null}
            {events.isError ? <p className="text-sm text-destructive">事件加载失败</p> : null}
            {displayedEvents?.length ? (
              <RunEventTimeline events={displayedEvents} key={displayedRun.id} limitReached={eventsTruncated} />
            ) : !events.isLoading ? (
              <p className="text-sm text-ink-2">暂无事件</p>
            ) : null}
          </div>
        </div>
      </SheetContent>
    </Sheet>
  );
}

function SummaryItem({
  label,
  value,
  mono,
  className
}: {
  label: string;
  value: ReactNode;
  mono?: boolean;
  className?: string;
}) {
  return (
    <div className={cn("min-w-0 rounded-md border border-line bg-card-soft px-3 py-2", className)}>
      <p className="text-xs text-ink-3">{label}</p>
      <div
        className={
          mono
            ? "mt-1 truncate font-mono text-xs text-ink"
            : "mt-1 truncate text-sm font-medium text-ink"
        }
      >
        {value}
      </div>
    </div>
  );
}

function RunStatusPill({ status }: { status: DigitalEmployeeRunStatus }) {
  const tone: Tone = isFailedRun(status) ? "danger" : status === "completed" ? "ok" : "mute";
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
        <MarkdownProse className="mt-2 break-words rounded-md border border-line bg-card-soft p-3">
          {conclusion}
        </MarkdownProse>
      ) : null}
      {rawJson && conclusion ? (
        <details className="mt-2" onToggle={(event) => setRawOpen(event.currentTarget.open)}>
          <summary className="cursor-pointer text-xs text-ink-3">原始结果 JSON</summary>
          {rawOpen ? (
            <pre className="mt-2 max-h-72 overflow-auto rounded-md border border-line bg-card-soft p-3 text-xs">
              {rawJson}
            </pre>
          ) : null}
        </details>
      ) : null}
      {rawJson && !conclusion ? (
        <pre className="mt-2 max-h-72 overflow-auto rounded-md border border-line bg-card-soft p-3 text-xs">
          {rawJson}
        </pre>
      ) : null}
      {!rawJson && !conclusion ? <p className="mt-2 text-sm text-ink-2">无结果数据</p> : null}
    </div>
  );
}

function isActiveRun(status: DigitalEmployeeRunStatus) {
  return activeRunStatuses.has(status);
}

function isFailedRun(status: DigitalEmployeeRunStatus) {
  return failedRunStatuses.has(status);
}

function isRecoverableRun(status: DigitalEmployeeRunStatus) {
  return recoverableRunStatuses.has(status);
}

function formatRecoveryError(error: unknown, fallback: string) {
  const message = error instanceof Error ? error.message : String(error ?? "");
  if (message.includes("project-linked runs use project recovery decisions") || message.includes("项目任务失败请在项目详情")) {
    return "此运行属于项目任务，请在项目详情或收件箱处理失败恢复";
  }
  return fallback;
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

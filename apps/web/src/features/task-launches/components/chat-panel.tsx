import { useEffect, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { ArrowRightLeft, MessageCircle, SendHorizontal, UserRound } from "lucide-react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { V3EmptyState, V3ErrorState, V3LoadingState } from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  createDigitalEmployeeRun,
  getDigitalEmployeeRun,
  listDigitalEmployees,
  type DigitalEmployeeRun,
  type DigitalEmployeeRunStatus,
} from "@/lib/api/employees";
import { LaunchChip } from "./task-launch-form";

const ACTIVE_RUN_STATUSES = new Set<DigitalEmployeeRunStatus>([
  "queued",
  "dispatching",
  "running",
  "cancelling",
]);

const TERMINAL_RUN_STATUSES = new Set<DigitalEmployeeRunStatus>([
  "completed",
  "failed",
  "cancelled",
  "timed_out",
]);

export type ChatEntry = {
  runId: string;
  question: string;
  status: DigitalEmployeeRunStatus | "sending";
  answer?: string;
  error?: string;
};

export type ChatPanelProps = {
  apiOptions: ApiClientOptions;
  onConvertToTask: (draft: string) => void;
};

function isActiveEntryStatus(status: DigitalEmployeeRunStatus | "sending"): boolean {
  return status === "sending" || ACTIVE_RUN_STATUSES.has(status as DigitalEmployeeRunStatus);
}

/** 从运行结果里挑一个可读的回答文本；本函数导出以便测试直接覆盖场景。 */
export function extractAnswerText(run: DigitalEmployeeRun): string {
  const result = run.result as Record<string, unknown> | null | undefined;
  for (const key of ["output", "summary", "message", "text"]) {
    const value = result?.[key];
    if (typeof value === "string" && value.trim()) {
      return value;
    }
  }
  return result ? JSON.stringify(result, null, 2) : "(无结果内容)";
}

/** 把一条已完成的对话条目改写为可编辑的任务草稿。 */
export function buildTaskDraft(entry: ChatEntry, employeeName: string): string {
  const excerpt = (entry.answer ?? "").slice(0, 3000);
  return `【目标】(请改写为你要的结果)\n\n${excerpt}\n\n【背景】源自与 @${employeeName} 的单次对话：${entry.question}`;
}

export function ChatPanel({ apiOptions, onConvertToTask }: ChatPanelProps) {
  const [employeeId, setEmployeeId] = useState("");
  const [question, setQuestion] = useState("");
  const [thread, setThread] = useState<ChatEntry[]>([]);
  const [sendError, setSendError] = useState("");

  const employeesQuery = useQuery({
    queryFn: () => listDigitalEmployees(apiOptions),
    queryKey: ["chat-employees"],
  });
  const employees = employeesQuery.data ?? [];
  const selectedEmployee = employees.find((employee) => employee.id === employeeId);

  useEffect(() => {
    if (!employees.length) {
      if (employeeId) {
        setEmployeeId("");
      }
      return;
    }
    if (!employees.some((employee) => employee.id === employeeId)) {
      setEmployeeId(employees[0].id);
    }
  }, [employees, employeeId]);

  const lastCompleted = [...thread].reverse().find((entry) => entry.status === "completed");
  const activeEntry = thread.find((entry) => isActiveEntryStatus(entry.status));
  const runQueryEnabled = Boolean(activeEntry && employeeId && activeEntry.status !== "sending");

  const runQuery = useQuery({
    enabled: runQueryEnabled,
    queryFn: () => getDigitalEmployeeRun(apiOptions, employeeId, activeEntry!.runId),
    queryKey: ["chat-run", employeeId, activeEntry?.runId],
    refetchInterval: runQueryEnabled ? 2500 : false,
  });

  useEffect(() => {
    const run = runQuery.data;
    if (!run) {
      return;
    }
    setThread((prev) =>
      prev.map((entry) => {
        if (entry.runId !== run.id) {
          return entry;
        }
        if (TERMINAL_RUN_STATUSES.has(run.status)) {
          return {
            ...entry,
            answer: run.status === "completed" ? extractAnswerText(run) : undefined,
            error: run.status === "completed" ? undefined : run.error_message ?? "对话执行失败，请重试",
            status: run.status,
          };
        }
        return { ...entry, status: run.status };
      }),
    );
  }, [runQuery.data]);

  const sendMutation = useMutation({
    mutationFn: (input: { objective: string; resumeOf?: string }) =>
      createDigitalEmployeeRun(apiOptions, employeeId, {
        objective: input.objective,
        run_kind: "chat",
        ...(input.resumeOf ? { resume_of_run_id: input.resumeOf } : {}),
      }),
    onError: (mutationError: unknown) => {
      setSendError(mutationError instanceof Error ? mutationError.message : "发送失败，请重试");
    },
    onSuccess: (run, variables) => {
      setSendError("");
      setThread((prev) => [
        ...prev,
        { question: variables.objective, runId: run.id, status: run.status },
      ]);
    },
  });

  function handleEmployeeChange(nextEmployeeId: string) {
    setEmployeeId(nextEmployeeId);
    setThread([]);
    setSendError("");
  }

  function handleSend() {
    const trimmed = question.trim();
    if (!trimmed || !employeeId || activeEntry) {
      return;
    }
    sendMutation.mutate({ objective: trimmed, resumeOf: lastCompleted?.runId });
    setQuestion("");
  }

  function handleRetry(entry: ChatEntry) {
    if (activeEntry) {
      return;
    }
    sendMutation.mutate({ objective: entry.question });
  }

  function handleConvert(entry: ChatEntry) {
    onConvertToTask(buildTaskDraft(entry, selectedEmployee?.name ?? ""));
  }

  const canSend =
    Boolean(question.trim()) && Boolean(employeeId) && !activeEntry && !sendMutation.isPending;

  return (
    <div className="tl-chat">
      <div className="tl-chat-header">
        <LaunchChip icon={<UserRound aria-hidden />} label="对话员工" required>
          <Select value={employeeId} onValueChange={handleEmployeeChange}>
            <SelectTrigger aria-label="对话员工" className="tl-chip-select">
              <SelectValue placeholder="选择数字员工" />
            </SelectTrigger>
            <SelectContent>
              {employees.map((employee) => (
                <SelectItem key={employee.id} value={employee.id}>
                  {employee.name} · {employee.role}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </LaunchChip>
      </div>

      <div className="tl-chat-thread v3-glass-inner" data-testid="chat-thread">
        {employeesQuery.isLoading ? <V3LoadingState label="加载数字员工…" /> : null}
        {employeesQuery.isError ? (
          <V3ErrorState
            description="无法加载数字员工列表"
            onRetry={() => employeesQuery.refetch()}
          />
        ) : null}
        {employeesQuery.isSuccess && employees.length === 0 ? (
          <V3EmptyState
            icon={<UserRound aria-hidden />}
            title="暂无可对话的数字员工"
            description="请先创建一名数字员工再发起对话"
          />
        ) : null}
        {employeesQuery.isSuccess && employees.length > 0 && thread.length === 0 ? (
          <V3EmptyState
            icon={<MessageCircle aria-hidden />}
            title="向数字员工提问开始对话"
            description="对话结果不会进入项目流转，可随时转为正式任务"
          />
        ) : null}
        {thread.map((entry) => (
          <div className="tl-chat-entry" key={entry.runId}>
            <p className="tl-chat-question">{entry.question}</p>
            {entry.status === "completed" ? (
              <div className="tl-chat-answer">
                <p>{entry.answer}</p>
                <button
                  className="tl-ghost"
                  onClick={() => handleConvert(entry)}
                  type="button"
                >
                  <ArrowRightLeft aria-hidden className="size-3.5" />
                  转为任务
                </button>
              </div>
            ) : entry.status === "failed" ||
              entry.status === "cancelled" ||
              entry.status === "timed_out" ? (
              <V3ErrorState
                description={entry.error}
                onRetry={() => handleRetry(entry)}
                title="对话失败"
              />
            ) : (
              <V3LoadingState label="数字员工思考中…" />
            )}
          </div>
        ))}
      </div>

      {sendError ? <div className="tl-err">⚠ {sendError}</div> : null}

      <div className="tl-chat-composer v3-glass-inner">
        <textarea
          aria-label="对话问题"
          className="tl-chat-textarea"
          onChange={(event) => setQuestion(event.target.value)}
          placeholder="向数字员工提问，回答不会写入项目流转"
          value={question}
        />
        <button className="tl-btn-send" disabled={!canSend} onClick={handleSend} type="button">
          发送
          <SendHorizontal aria-hidden className="size-4" />
        </button>
      </div>
    </div>
  );
}

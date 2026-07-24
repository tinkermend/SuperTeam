import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  ArrowRightLeft,
  FolderOpen,
  MessageCircle,
  MessageSquarePlus,
  SendHorizontal,
  UserRound
} from "lucide-react";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from "@/components/ui/select";
import { EmptyState, ErrorState, LoadingState } from "@/components/superteam";
import { ApiRequestError, type ApiClientOptions } from "@/lib/api/client";
import {
  createDigitalEmployeeRun,
  getDigitalEmployeeRun,
  listDigitalEmployeeRuns,
  listDigitalEmployees,
  type DigitalEmployeeRun,
  type DigitalEmployeeRunListItem,
  type DigitalEmployeeRunStatus
} from "@/lib/api/employees";
import { listProjectMembers, type Project } from "@/lib/api/projects";
import { LaunchChip, ProjectPicker } from "./task-launch-form";

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
  /** Set when this entry was produced by an automatic no-resume retry after the
   * server rejected `resume_of_run_id` (lost/invalid session) — surfaces a
   * non-blocking "上下文未延续" hint instead of silently dropping the context. */
  contextNotContinued?: boolean;
};

export type ConvertToTaskPayload = {
  draft: string;
  chatRunId: string;
  digitalEmployeeId: string;
  /** The project the source chat run was anchored to; pre-selects the task
   * form's project when converting so the anchor and the eventual demand's
   * project default to the same place (user can still change it). */
  anchorProjectId: string;
};

export type ChatPanelProps = {
  apiOptions: ApiClientOptions;
  onConvertToTask: (payload: ConvertToTaskPayload) => void;
  onProjectChange: (projectId: string) => void;
  projectId: string;
  projects: Project[];
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

/** 把服务端持久化的 chat run 还原成一条对话条目（会话恢复用）。问题文本即
 * run 的 task_title（objective 全文入库）；已完成但结果被平台清理/过期的
 * run 保留条目、答案降级提示（spec §6 决议 2）。导出以便测试直接覆盖场景。 */
export function entryFromRunListItem(item: DigitalEmployeeRunListItem): ChatEntry {
  const entry: ChatEntry = {
    question: item.task_title,
    runId: item.id,
    status: item.status
};
  if (item.status === "completed") {
    const hasResult = item.result && Object.keys(item.result).length > 0;
    entry.answer = hasResult ? extractAnswerText(item) : "（内容已过期或无结果）";
  } else if (TERMINAL_RUN_STATUSES.has(item.status)) {
    entry.error = item.error_message ?? "对话执行失败，请重试";
  }
  return entry;
}

export function ChatPanel({
  apiOptions,
  onConvertToTask,
  onProjectChange,
  projectId,
  projects
}: ChatPanelProps) {
  const [employeeId, setEmployeeId] = useState("");
  const [question, setQuestion] = useState("");
  const [thread, setThread] = useState<ChatEntry[]>([]);
  const [sendError, setSendError] = useState("");
  // 会话恢复的水合标记：记录 thread 当前对应的 (employee, project) 锚点。
  // 锚点变化(挂载/换员工/换项目)后由服务端最新链重建;点"新对话"只清视图、
  // 不动水合标记,下一条消息不带 resume 即开新链(spec §4.4)。
  const [hydratedAnchor, setHydratedAnchor] = useState("");
  const [restoreFailed, setRestoreFailed] = useState(false);
  const anchorKey = employeeId && projectId ? `${employeeId}::${projectId}` : "";

  const employeesQuery = useQuery({
    queryFn: () => listDigitalEmployees(apiOptions),
    queryKey: ["chat-employees"]
});
  // 参与门禁：chat 只能选锚点项目的 active digital_employee 成员，
  // 与后端 createChatRun 的成员资格校验同口径。未选项目时不出候选。
  const membersQuery = useQuery({
    enabled: Boolean(projectId),
    queryFn: () => listProjectMembers(apiOptions, projectId),
    queryKey: ["chat-project-members", projectId]
});
  const memberEmployeeIds = useMemo(() => {
    const ids = new Set<string>();
    for (const member of membersQuery.data ?? []) {
      if (member.principal_type === "digital_employee" && member.status === "active") {
        ids.add(member.principal_id);
      }
    }
    return ids;
  }, [membersQuery.data]);
  const employees = useMemo(
    () =>
      projectId
        ? (employeesQuery.data ?? []).filter((employee) => memberEmployeeIds.has(employee.id))
        : [],
    [employeesQuery.data, memberEmployeeIds, projectId],
  );
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

  // 会话恢复：按 (employee, project) 锚点取最新 chat run 的 chat_thread_id,
  // 再拉全链重建"当前会话"。
  const restoreQuery = useQuery({
    enabled: Boolean(anchorKey),
    refetchOnMount: "always",
    queryKey: ["chat-restore", employeeId, projectId],
    queryFn: async (): Promise<ChatEntry[]> => {
      const latest = await listDigitalEmployeeRuns(apiOptions, employeeId, {
        limit: 1,
        project_id: projectId,
        run_kind: "chat"
});
      const threadId = latest.items[0]?.chat_thread_id;
      if (!threadId) {
        return [];
      }
      const runs = await listDigitalEmployeeRuns(apiOptions, employeeId, {
        chat_thread_id: threadId,
        limit: 100
});
      // 服务端按 created_at 倒序分页,视图按时间正序展示。
      return [...runs.items].reverse().map(entryFromRunListItem);
    }
});

  const restoring = Boolean(anchorKey) && hydratedAnchor !== anchorKey && !restoreQuery.isError;

  // 水合只认本次挂载后取回的新鲜数据(isFetchedAfterMount)：跨路由缓存的旧线程
  // 可能缺少离开页面前最后发出的消息,直接吃缓存会静默丢轮。
  useEffect(() => {
    if (
      !anchorKey ||
      hydratedAnchor === anchorKey ||
      !restoreQuery.isSuccess ||
      !restoreQuery.isFetchedAfterMount ||
      restoreQuery.isFetching
    ) {
      return;
    }
    setThread(restoreQuery.data);
    setHydratedAnchor(anchorKey);
    setRestoreFailed(false);
  }, [
    anchorKey,
    hydratedAnchor,
    restoreQuery.isSuccess,
    restoreQuery.isFetchedAfterMount,
    restoreQuery.isFetching,
    restoreQuery.data,
  ]);

  // 恢复失败不阻塞对话：按空线程水合并给出非阻断提示,用户可直接开始新对话
  // (代价是本条链暂时不可见,下次进入页面会重试恢复)。
  useEffect(() => {
    if (!anchorKey || hydratedAnchor === anchorKey || !restoreQuery.isError) {
      return;
    }
    setThread([]);
    setHydratedAnchor(anchorKey);
    setRestoreFailed(true);
  }, [anchorKey, hydratedAnchor, restoreQuery.isError]);

  // Synchronous in-flight guard: React (re)renders — and therefore refreshes the
  // `sendMutation.isPending` closures captured by button handlers — only after the
  // current discrete event finishes. A genuine fast double-click can fire both
  // click handlers before that happens, so `isPending` alone cannot be trusted to
  // block the second call. This ref updates immediately, with no render involved.
  const sendInFlightRef = useRef(false);

  const lastCompleted = [...thread].reverse().find((entry) => entry.status === "completed");
  const activeEntry = thread.find((entry) => isActiveEntryStatus(entry.status));
  const runQueryEnabled = Boolean(activeEntry && employeeId && activeEntry.status !== "sending");

  const runQuery = useQuery({
    enabled: runQueryEnabled,
    queryFn: () => getDigitalEmployeeRun(apiOptions, employeeId, activeEntry!.runId),
    queryKey: ["chat-run", employeeId, activeEntry?.runId],
    refetchInterval: runQueryEnabled ? 2500 : false
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
            status: run.status
};
        }
        return { ...entry, status: run.status };
      }),
    );
  }, [runQuery.data]);

  const sendMutation = useMutation({
    mutationFn: (input: { objective: string; resumeOf?: string; degraded?: boolean }) =>
      createDigitalEmployeeRun(apiOptions, employeeId, {
        objective: input.objective,
        run_kind: "chat",
        project_id: projectId,
        ...(input.resumeOf ? { resume_of_run_id: input.resumeOf } : {})
}),
    onSuccess: (run, variables) => {
      setSendError("");
      setThread((prev) => [
        ...prev,
        {
          question: variables.objective,
          runId: run.id,
          status: run.status,
          ...(variables.degraded ? { contextNotContinued: true } : {})
},
      ]);
    }
});

  // When a send carrying resume_of_run_id fails with a 400 (server rejected the
  // resumed session — invalid or lost), automatically resend exactly once without
  // resume_of_run_id rather than leaving the thread wedged on a repeating 400. The
  // guard against a resend loop is structural: the retry omits `resumeOf`, so its
  // own failure path below never re-enters this branch.
  async function sendWithDegradeFallback(objective: string, resumeOf?: string) {
    try {
      return await sendMutation.mutateAsync({ objective, resumeOf });
    } catch (error) {
      if (resumeOf && error instanceof ApiRequestError && error.status === 400) {
        try {
          return await sendMutation.mutateAsync({ objective, degraded: true });
        } catch (retryError) {
          setSendError(retryError instanceof Error ? retryError.message : "发送失败，请重试");
          throw retryError;
        }
      }
      setSendError(error instanceof Error ? error.message : "发送失败，请重试");
      throw error;
    }
  }

  // 换员工/换项目 = 切换会话锚点：清掉当前视图,由恢复查询重建新锚点自己的
  // 最新链(切回来时旧会话仍在)。
  function handleEmployeeChange(nextEmployeeId: string) {
    setEmployeeId(nextEmployeeId);
    setThread([]);
    setSendError("");
    setRestoreFailed(false);
  }

  function handleProjectChange(nextProjectId: string) {
    onProjectChange(nextProjectId);
    setThread([]);
    setSendError("");
    setRestoreFailed(false);
  }

  // 新对话是断链的唯一主动入口：只清本地视图,下一条消息不带 resume_of_run_id,
  // 服务端即冠新 thread 根,当前锚点的"最新链"随之切换。
  function handleNewConversation() {
    if (activeEntry || sendInFlightRef.current) {
      return;
    }
    setThread([]);
    setSendError("");
  }

  function handleSend() {
    const trimmed = question.trim();
    if (
      !trimmed ||
      !employeeId ||
      !projectId ||
      restoring ||
      activeEntry ||
      sendInFlightRef.current
    ) {
      return;
    }
    sendInFlightRef.current = true;
    sendWithDegradeFallback(trimmed, lastCompleted?.runId)
      .then(() => setQuestion(""))
      .catch(() => {})
      .finally(() => {
        sendInFlightRef.current = false;
      });
  }

  // 重试沿用当前会话的续链语义(带上最近完成轮的 resume 目标)；此前不带
  // resume 会静默开新链,恢复视图时旧上下文"消失"。
  function handleRetry(entry: ChatEntry) {
    if (activeEntry || sendInFlightRef.current) {
      return;
    }
    sendInFlightRef.current = true;
    sendWithDegradeFallback(entry.question, lastCompleted?.runId)
      .catch(() => {})
      .finally(() => {
        sendInFlightRef.current = false;
      });
  }

  function handleConvert(entry: ChatEntry) {
    onConvertToTask({
      anchorProjectId: projectId,
      chatRunId: entry.runId,
      digitalEmployeeId: employeeId,
      draft: buildTaskDraft(entry, selectedEmployee?.name ?? "")
});
  }

  const canSend =
    Boolean(question.trim()) &&
    Boolean(employeeId) &&
    Boolean(projectId) &&
    !restoring &&
    !activeEntry &&
    !sendMutation.isPending;

  return (
    <div className="tl-chat">
      <div className="tl-chat-header">
        <LaunchChip icon={<FolderOpen aria-hidden />} label="项目" required>
          <ProjectPicker onChange={handleProjectChange} projects={projects} value={projectId} />
        </LaunchChip>
        <LaunchChip icon={<UserRound aria-hidden />} label="对话员工" required>
          <Select disabled={!projectId} value={employeeId} onValueChange={handleEmployeeChange}>
            <SelectTrigger aria-label="对话员工" className="tl-chip-select">
              <SelectValue placeholder={projectId ? "选择数字员工" : "请先选择项目"} />
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
        <button
          className="tl-ghost tl-chat-new"
          disabled={thread.length === 0 || Boolean(activeEntry) || sendMutation.isPending}
          onClick={handleNewConversation}
          type="button"
        >
          <MessageSquarePlus aria-hidden className="size-3.5" />
          新对话
        </button>
      </div>

      <div className="tl-chat-thread glass-inner" data-testid="chat-thread">
        {employeesQuery.isLoading || (Boolean(projectId) && membersQuery.isLoading) ? (
          <LoadingState label="加载数字员工…" />
        ) : null}
        {employeesQuery.isError || membersQuery.isError ? (
          <ErrorState
            description="无法加载数字员工列表"
            onRetry={() => {
              void employeesQuery.refetch();
              void membersQuery.refetch();
            }}
          />
        ) : null}
        {!projectId && employeesQuery.isSuccess ? (
          <EmptyState
            icon={<FolderOpen aria-hidden />}
            title="请先选择项目"
            description="对话按项目锚定，仅项目内的数字员工成员可参与对话"
          />
        ) : null}
        {Boolean(projectId) && employeesQuery.isSuccess && membersQuery.isSuccess && employees.length === 0 ? (
          <EmptyState
            icon={<UserRound aria-hidden />}
            title="该项目暂无可对话的数字员工成员"
            description="请先在项目配置中把数字员工加入项目成员"
          />
        ) : null}
        {employeesQuery.isSuccess && employees.length > 0 && restoring ? (
          <LoadingState label="恢复历史对话…" />
        ) : null}
        {employeesQuery.isSuccess && employees.length > 0 && !restoring && thread.length === 0 ? (
          <EmptyState
            icon={<MessageCircle aria-hidden />}
            title="向数字员工提问开始对话"
            description="对话结果不会进入项目流转，可随时转为正式任务"
          />
        ) : null}
        {thread.map((entry) => (
          <div className="tl-chat-entry" key={entry.runId}>
            <p className="tl-chat-question">{entry.question}</p>
            {entry.contextNotContinued ? (
              <p className="tl-chat-notice">上下文未延续，已作为新对话重新发送</p>
            ) : null}
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
              <ErrorState
                description={entry.error}
                onRetry={sendMutation.isPending ? undefined : () => handleRetry(entry)}
                title="对话失败"
              />
            ) : (
              <LoadingState label="数字员工思考中…" />
            )}
          </div>
        ))}
      </div>

      {restoreFailed ? <div className="tl-err">⚠ 历史对话恢复失败，可直接开始新对话</div> : null}
      {sendError ? <div className="tl-err">⚠ {sendError}</div> : null}

      <div className="tl-chat-composer glass-inner">
        <textarea
          aria-label="对话问题"
          className="tl-chat-textarea"
          onChange={(event) => setQuestion(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && event.shiftKey && !event.nativeEvent.isComposing) {
              event.preventDefault();
              handleSend();
            }
          }}
          placeholder="向数字员工提问，回答不会写入项目流转"
          value={question}
        />
        <button
          className="tl-btn-send"
          disabled={!canSend}
          onClick={handleSend}
          title="发送（Shift+Enter）"
          type="button"
        >
          发送
          <SendHorizontal aria-hidden className="size-4" />
        </button>
      </div>
    </div>
  );
}

import { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  ArrowRightLeft,
  FolderOpen,
  MessageCircle,
  MessageSquarePlus,
  SendHorizontal,
  UserRound
} from "lucide-react";
import { EmptyState, ErrorState, LoadingState, SoftDialog, SoftDialogBody, SoftDialogContent, SoftDialogDescription, SoftDialogFooter, SoftDialogHeader, SoftDialogTitle, Button } from "@/components/superteam";
import { EmployeeAvatar } from "@/features/employees/avatar";
import { employeeAvatarAsset } from "@/features/employees/avatar-library";
import "./task-hub-chat.css";
import { ApiRequestError, type ApiClientOptions } from "@/lib/api/client";
import { getCurrentUser } from "@/lib/api/auth";
import {
  createDigitalEmployeeRun,
  getDigitalEmployeeRun,
  listDigitalEmployeeChatThreads,
  listDigitalEmployeeRuns,
  listDigitalEmployees,
  type DigitalEmployeeChatThread,
  type DigitalEmployeeRun,
  type DigitalEmployeeRunListItem,
  type DigitalEmployeeRunStatus
} from "@/lib/api/employees";
import { listProjectMembers, getProject, refreshProjectWorkspaceGitStatus, type Project } from "@/lib/api/projects";
import { listProjectSkillBindings } from "@/lib/api/skills";
import { ProjectWorkspaceGitPanel } from "@/features/projects/components/project-workspace-git-panel";
import {
  NoProjectsEmptyState,
  ProjectPicker,
  type ProjectChangeHandler,
} from "./task-launch-form";

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
  creatorDisplayName?: string;
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
  onProjectChange: ProjectChangeHandler;
  projectId: string;
  projects: Project[];
  /** 父层缓存的已选项目（搜索越界）。 */
  resolvedProject?: Project | null;
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
    status: item.status,
    creatorDisplayName: item.creator_display_name,
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
  projects,
  resolvedProject = null,
}: ChatPanelProps) {
  const [employeeId, setEmployeeId] = useState("");
  const [question, setQuestion] = useState("");
  const [thread, setThread] = useState<ChatEntry[]>([]);
  const [sendError, setSendError] = useState("");
  /** Current SuperTeam chat thread root id; null means composing a brand-new root. */
  const [selectedThreadId, setSelectedThreadId] = useState<string | null>(null);
  const [threadFilter, setThreadFilter] = useState<"all" | "mine">("all");
  /** True only after the user clicks「新会话」; blocks auto-pin of latest thread. */
  const [explicitNewSession, setExplicitNewSession] = useState(false);
  /** Per-turn skill envelope (autonomy P1). Empty = full project Chat surface. */
  const [selectedSkillIds, setSelectedSkillIds] = useState<string[]>([]);
  const [pendingConfirm, setPendingConfirm] = useState<{
    objective: string;
    resumeOf?: string;
  } | null>(null);
  const [restoreFailed, setRestoreFailed] = useState(false);

  const employeesQuery = useQuery({
    queryFn: () => listDigitalEmployees(apiOptions),
    queryKey: ["chat-employees"]
});
  const currentUserQuery = useQuery({
    queryFn: () => getCurrentUser(apiOptions),
    queryKey: ["auth", "current-user", "chat-panel"],
  });
  const currentUserId = currentUserQuery.data?.user?.id;
  // 参与门禁：chat 只能选锚点项目的 active digital_employee 成员，
  // 与后端 createChatRun 的成员资格校验同口径。未选项目时不出候选。
  const membersQuery = useQuery({
    enabled: Boolean(projectId),
    queryFn: () => listProjectMembers(apiOptions, projectId),
    queryKey: ["chat-project-members", projectId]
});
  const skillBindingsQuery = useQuery({
    enabled: Boolean(projectId),
    queryFn: () => listProjectSkillBindings(apiOptions, projectId),
    queryKey: ["chat-project-skill-bindings", projectId],
  });
  const projectDetailQuery = useQuery({
    enabled: Boolean(projectId),
    queryFn: () => getProject(apiOptions, projectId),
    queryKey: ["chat-project-detail", projectId],
    // Ceiling drives SoftDialog; avoid long-lived stale policy after config edits.
    staleTime: 0,
  });
  const refreshGitMutation = useMutation({
    mutationFn: () => refreshProjectWorkspaceGitStatus(apiOptions, projectId),
    onSuccess: () => {
      void projectDetailQuery.refetch();
    },
  });
  const chatSkillOptions = useMemo(() => {
    const rows = skillBindingsQuery.data ?? [];
    return rows.map((row) => ({
      id: row.skill_id,
      label: row.skill?.name?.trim() || row.skill?.slug?.trim() || row.skill_id.slice(0, 8),
    }));
  }, [skillBindingsQuery.data]);

  useEffect(() => {
    // Drop selections that left the project surface when the project changes.
    const allow = new Set(chatSkillOptions.map((item) => item.id));
    setSelectedSkillIds((prev) => prev.filter((id) => allow.has(id)));
  }, [chatSkillOptions]);
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

  const threadsQuery = useQuery({
    enabled: Boolean(employeeId && projectId),
    queryFn: () => listDigitalEmployeeChatThreads(apiOptions, employeeId, projectId),
    queryKey: ["chat-threads", employeeId, projectId],
    refetchInterval: 5000,
  });

  const chatThreads = useMemo(() => {
    const items = threadsQuery.data?.items ?? [];
    if (threadFilter !== "mine" || !currentUserId) {
      return items;
    }
    return items.filter((item) => item.initiator_user_id === currentUserId);
  }, [currentUserId, threadFilter, threadsQuery.data?.items]);

  // 默认钉最近活跃会话；仅在用户显式「新会话」时不自动选。
  // 用 layout effect，避免首帧先按「无选中」水合空线程。
  useLayoutEffect(() => {
    if (!employeeId || !projectId || explicitNewSession || selectedThreadId) {
      return;
    }
    if (threadsQuery.isLoading || threadsQuery.isError) {
      return;
    }
    const newest = threadsQuery.data?.items?.[0];
    if (!newest) {
      return;
    }
    setSelectedThreadId(newest.chat_thread_id);
  }, [
    employeeId,
    explicitNewSession,
    projectId,
    selectedThreadId,
    threadsQuery.data?.items,
    threadsQuery.isError,
    threadsQuery.isLoading,
  ]);

  // 会话恢复：选中 thread 后拉全链；新会话（selectedThreadId=null）保持空视图。
  const restoreQuery = useQuery({
    enabled: Boolean(employeeId && projectId && selectedThreadId),
    refetchOnMount: "always",
    queryKey: ["chat-restore", employeeId, projectId, selectedThreadId],
    queryFn: async (): Promise<ChatEntry[]> => {
      const runs = await listDigitalEmployeeRuns(apiOptions, employeeId, {
        chat_thread_id: selectedThreadId!,
        limit: 100,
      });
      return [...runs.items].reverse().map(entryFromRunListItem);
    },
  });

  const restoring =
    Boolean(employeeId && projectId && selectedThreadId) &&
    !restoreQuery.isError &&
    !restoreQuery.isSuccess;

  // 服务端恢复结果优先用于展示：避免 effect→setState 在测试/严格模式下丢水合。
  // 本地 thread 在发送/轮询后成为真相；恢复成功且本地仍空时回退到 query 数据。
  const restoreDataRef = useRef(restoreQuery.data);
  restoreDataRef.current = restoreQuery.data;

  useLayoutEffect(() => {
    if (!selectedThreadId) {
      return;
    }
    if (restoreQuery.isError) {
      setRestoreFailed(true);
      return;
    }
    if (!restoreQuery.isSuccess || !restoreQuery.data) {
      return;
    }
    setRestoreFailed(false);
    setThread((prev) => (prev.length === 0 ? restoreQuery.data! : prev));
  }, [
    restoreQuery.data,
    restoreQuery.isError,
    restoreQuery.isSuccess,
    selectedThreadId,
  ]);

  useEffect(() => {
    if (selectedThreadId || !explicitNewSession) {
      return;
    }
    setThread([]);
    setRestoreFailed(false);
  }, [explicitNewSession, selectedThreadId]);

  // Synchronous in-flight guard: React (re)renders — and therefore refreshes the
  // `sendMutation.isPending` closures captured by button handlers — only after the
  // current discrete event finishes. A genuine fast double-click can fire both
  // click handlers before that happens, so `isPending` alone cannot be trusted to
  // block the second call. This ref updates immediately, with no render involved.
  const sendInFlightRef = useRef(false);

  const displayThread =
    thread.length > 0
      ? thread
      : selectedThreadId && restoreQuery.data
        ? restoreQuery.data
        : [];

  const lastCompleted = [...displayThread].reverse().find((entry) => entry.status === "completed");
  const activeEntry = displayThread.find((entry) => isActiveEntryStatus(entry.status));
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
    setThread((prev) => {
      const base = prev.length > 0 ? prev : (restoreDataRef.current ?? []);
      return base.map((entry) => {
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
      });
    });
  }, [runQuery.data]);

  const sendMutation = useMutation({
    mutationFn: (input: {
      objective: string;
      resumeOf?: string;
      chatThreadId?: string;
      degraded?: boolean;
      interactiveConfirmed?: boolean;
    }) =>
      createDigitalEmployeeRun(apiOptions, employeeId, {
        objective: input.objective,
        run_kind: "chat",
        project_id: projectId,
        ...(selectedSkillIds.length > 0 ? { skill_ids: selectedSkillIds } : {}),
        ...(input.interactiveConfirmed ? { interactive_confirmed: true } : {}),
        ...(input.resumeOf ? { resume_of_run_id: input.resumeOf } : {}),
        // TTL 过期续写：无 resume 时仍带 chat_thread_id，禁止拆新 thread。
        ...(!input.resumeOf && input.chatThreadId
          ? { chat_thread_id: input.chatThreadId }
          : {}),
      }),
    onSuccess: (run, variables) => {
      setSendError("");
      if (run.chat_thread_id && run.chat_thread_id !== selectedThreadId) {
        setSelectedThreadId(run.chat_thread_id);
      }
      void threadsQuery.refetch();
      setThread((prev) => {
        const base = prev.length > 0 ? prev : (restoreDataRef.current ?? []);
        return [
          ...base,
          {
            question: variables.objective,
            runId: run.id,
            status: run.status,
            creatorDisplayName: run.creator_display_name,
            ...(variables.degraded ? { contextNotContinued: true } : {}),
          },
        ];
      });
    },
  });

  function autonomyCeilingFromPolicy(policy: Record<string, unknown> | undefined | null): string {
    const raw = policy?.autonomy_ceiling;
    return raw === "pause_at_gate" || raw === "full_auto" ? raw : "";
  }

  function isInteractiveConfirmRequiredError(error: unknown): boolean {
    if (!(error instanceof ApiRequestError) || error.status !== 400) {
      return false;
    }
    return /interactive light confirm required/i.test(error.message);
  }

  // When a send carrying resume_of_run_id fails with a 400 (server rejected the
  // resumed session — invalid or lost), automatically resend exactly once without
  // resume_of_run_id but WITH the same chat_thread_id rather than splitting a new
  // SuperTeam thread. The guard against a resend loop is structural: the retry
  // omits `resumeOf`, so its own failure path below never re-enters this branch.
  async function sendWithDegradeFallback(
    objective: string,
    resumeOf?: string,
    interactiveConfirmed?: boolean,
  ) {
    const chatThreadId = selectedThreadId ?? undefined;
    try {
      return await sendMutation.mutateAsync({
        objective,
        resumeOf,
        chatThreadId,
        interactiveConfirmed,
      });
    } catch (error) {
      if (
        !interactiveConfirmed &&
        isInteractiveConfirmRequiredError(error)
      ) {
        setSendError("");
        setPendingConfirm({ objective, resumeOf });
        return undefined;
      }
      if (resumeOf && error instanceof ApiRequestError && error.status === 400) {
        try {
          return await sendMutation.mutateAsync({
            objective,
            chatThreadId,
            degraded: true,
            interactiveConfirmed,
          });
        } catch (retryError) {
          if (
            !interactiveConfirmed &&
            isInteractiveConfirmRequiredError(retryError)
          ) {
            setSendError("");
            setPendingConfirm({ objective, resumeOf: undefined });
            return undefined;
          }
          setSendError(retryError instanceof Error ? retryError.message : "发送失败，请重试");
          throw retryError;
        }
      }
      setSendError(error instanceof Error ? error.message : "发送失败，请重试");
      throw error;
    }
  }

  const projectAutonomyCeiling = useMemo(() => {
    const policy =
      projectDetailQuery.data?.coordination_policy ??
      resolvedProject?.coordination_policy ??
      {};
    return autonomyCeilingFromPolicy(policy);
  }, [projectDetailQuery.data, resolvedProject]);

  const needsInteractiveConfirm =
    selectedSkillIds.length > 0 || projectAutonomyCeiling === "pause_at_gate";

  // 换员工/换项目 = 切换会话锚点：清掉当前视图,由恢复查询重建新锚点自己的
  // 最新链(切回来时旧会话仍在)。
  function handleEmployeeChange(nextEmployeeId: string) {
    setEmployeeId(nextEmployeeId);
    setSelectedThreadId(null);
    setExplicitNewSession(false);
    setThread([]);
    setSendError("");
    setRestoreFailed(false);
  }

  function handleProjectChange(project: Project) {
    onProjectChange(project);
    setSelectedThreadId(null);
    setExplicitNewSession(false);
    setThread([]);
    setSendError("");
    setRestoreFailed(false);
    setSelectedSkillIds([]);
  }

  function handleSelectThread(threadId: string) {
    if (threadId === selectedThreadId || activeEntry || sendInFlightRef.current) {
      return;
    }
    setExplicitNewSession(false);
    setSelectedThreadId(threadId);
    setThread([]);
    setSendError("");
    setRestoreFailed(false);
  }

  function toggleSkill(skillId: string) {
    setSelectedSkillIds((prev) =>
      prev.includes(skillId) ? prev.filter((id) => id !== skillId) : [...prev, skillId],
    );
  }

  // 新对话是断链的唯一主动入口：清空选中 thread，下一条消息开新根。
  function handleNewConversation() {
    if (activeEntry || sendInFlightRef.current) {
      return;
    }
    setExplicitNewSession(true);
    setSelectedThreadId(null);
    setThread([]);
    setSendError("");
  }

  function beginSend(objective: string, resumeOf?: string) {
    if (needsInteractiveConfirm) {
      setPendingConfirm({ objective, resumeOf });
      return;
    }
    sendInFlightRef.current = true;
    sendWithDegradeFallback(objective, resumeOf)
      .then((run) => {
        if (run) {
          setQuestion("");
        }
      })
      .catch(() => {})
      .finally(() => {
        sendInFlightRef.current = false;
      });
  }

  function confirmPendingSend() {
    if (!pendingConfirm || sendInFlightRef.current) {
      return;
    }
    const { objective, resumeOf } = pendingConfirm;
    setPendingConfirm(null);
    sendInFlightRef.current = true;
    sendWithDegradeFallback(objective, resumeOf, true)
      .then((run) => {
        if (run) {
          setQuestion("");
        }
      })
      .catch(() => {})
      .finally(() => {
        sendInFlightRef.current = false;
      });
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
    beginSend(trimmed, lastCompleted?.runId);
  }

  // 重试沿用当前会话的续链语义(带上最近完成轮的 resume 目标)；此前不带
  // resume 会静默开新链,恢复视图时旧上下文"消失"。
  function handleRetry(entry: ChatEntry) {
    if (activeEntry || sendInFlightRef.current) {
      return;
    }
    beginSend(entry.question, lastCompleted?.runId);
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
    !sendMutation.isPending &&
    // Wait for project policy so pause_at_gate SoftDialog is not skipped on first paint.
    (!projectId || !projectDetailQuery.isPending);

  return (
    <div className="hub-chat">
      <div className="hub-top">
        <span className="hub-top-label">项目</span>
        {projects.length === 0 ? (
          <NoProjectsEmptyState />
        ) : (
          <ProjectPicker
            apiOptions={apiOptions}
            onChange={handleProjectChange}
            projects={projects}
            resolvedProject={resolvedProject}
            value={projectId}
          />
        )}
        <span className="hub-top-note">对话按项目锚定，产出默认旁路</span>
      </div>

      <div className="hub-body">
        <aside className="hub-rail hub-rail-left" aria-label="数字员工与会话">
          <section className="hub-emp-block">
            <p className="hub-rail-label">数字员工</p>
            {!projectId ? (
              <p className="hub-rail-empty">请先选择项目</p>
            ) : employeesQuery.isLoading || membersQuery.isLoading ? (
              <p className="hub-rail-empty">加载数字员工…</p>
            ) : employees.length === 0 ? (
              <p className="hub-rail-empty">该项目暂无可对话的数字员工成员</p>
            ) : (
              <ul className="hub-emp-list" aria-label="数字员工列表">
                {employees.map((employee) => (
                  <li key={employee.id}>
                    <button
                      aria-pressed={employee.id === employeeId}
                      className="hub-emp"
                      onClick={() => handleEmployeeChange(employee.id)}
                      type="button"
                    >
                      <EmployeeAvatar
                        asset={employeeAvatarAsset(employee)}
                        name={employee.name}
                        size="md"
                      />
                      <span className="hub-emp-main">
                        <span className="hub-emp-name">{employee.name}</span>
                        <span className="hub-emp-role">{employee.role}</span>
                        {employee.description?.trim() ? (
                          <span className="hub-emp-desc">{employee.description}</span>
                        ) : null}
                      </span>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </section>
          <section className="hub-session-block" aria-label="会话列表" data-testid="chat-session-list">
            <div className="hub-session-head">
              <p className="hub-rail-label">会话</p>
              <button
                className="hub-new-session"
                disabled={
                  (!selectedThreadId && displayThread.length === 0) ||
                  Boolean(activeEntry) ||
                  sendMutation.isPending
                }
                onClick={handleNewConversation}
                type="button"
              >
                <MessageSquarePlus aria-hidden className="size-3.5" />
                新会话
              </button>
            </div>
            <div className="hub-session-filters" role="group" aria-label="会话过滤">
              <button
                type="button"
                className={threadFilter === "all" ? "hub-filter is-active" : "hub-filter"}
                onClick={() => setThreadFilter("all")}
              >
                全部
              </button>
              <button
                type="button"
                className={threadFilter === "mine" ? "hub-filter is-active" : "hub-filter"}
                onClick={() => setThreadFilter("mine")}
              >
                我发起的
              </button>
            </div>
            {!projectId || !employeeId ? (
              <p className="hub-rail-empty">选择项目与员工后显示会话</p>
            ) : threadsQuery.isLoading ? (
              <LoadingState label="加载会话…" />
            ) : chatThreads.length === 0 ? (
              <p className="hub-rail-empty">暂无会话，发送第一条消息开始</p>
            ) : (
              <ul className="hub-session-list">
                {chatThreads.map((item: DigitalEmployeeChatThread) => {
                  const active = item.chat_thread_id === selectedThreadId;
                  const speakerLine =
                    item.last_speaker_display_name &&
                    item.last_speaker_display_name !== item.initiator_display_name
                      ? `${item.initiator_display_name} · ${item.last_speaker_display_name}`
                      : item.initiator_display_name;
                  return (
                    <li key={item.chat_thread_id}>
                      <button
                        type="button"
                        className={active ? "hub-session is-active" : "hub-session"}
                        onClick={() => handleSelectThread(item.chat_thread_id)}
                      >
                        <span className="hub-session-title">{item.title}</span>
                        <span className="hub-session-meta">
                          <span>{speakerLine}</span>
                          {item.has_active_run ? (
                            <span className="hub-session-pill">
                              {item.active_runner_display_name
                                ? `${item.active_runner_display_name}进行中`
                                : "进行中"}
                            </span>
                          ) : null}
                          <span className="hub-session-time">
                            {formatRelativeTime(item.last_active_at)}
                          </span>
                        </span>
                        {item.last_prompt?.trim() ? (
                          <span className="hub-session-sum">{item.last_prompt}</span>
                        ) : null}
                      </button>
                    </li>
                  );
                })}
              </ul>
            )}
          </section>
        </aside>
        <section aria-label="对话线程" className="hub-main">
          <div className="hub-identity">
            {selectedEmployee ? (
              <>
                <EmployeeAvatar
                  asset={employeeAvatarAsset(selectedEmployee)}
                  name={selectedEmployee.name}
                  size="xl"
                />
                <div className="hub-identity-main">
                  <div className="hub-identity-name-row">
                    <span className="hub-identity-name">{selectedEmployee.name}</span>
                    <span className="hub-identity-role">{selectedEmployee.role}</span>
                  </div>
                  <span className="hub-identity-desc">
                    {selectedEmployee.description?.trim() ||
                      "本项目可对话，回答默认不写入主轨"}
                  </span>
                </div>
              </>
            ) : (
              <span className="hub-identity-empty">
                {projectId ? "从左侧选择数字员工开始对话" : "选择项目后显示可对话的数字员工"}
              </span>
            )}
          </div>

          <div className="hub-thread" data-testid="chat-thread">
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
            {employeesQuery.isSuccess && employees.length > 0 && !restoring && displayThread.length === 0 ? (
              <EmptyState
                icon={<MessageCircle aria-hidden />}
                title="向数字员工提问开始对话"
                description="对话结果不会进入项目流转，可随时转为正式任务"
              />
            ) : null}
            {displayThread.map((entry) => (
              <div className="hub-entry" key={entry.runId}>
                <div className="hub-turn-human">
                  {entry.creatorDisplayName ? (
                    <p className="hub-speaker">{entry.creatorDisplayName}</p>
                  ) : null}
                  <p className="hub-q">{entry.question}</p>
                  {entry.contextNotContinued ? (
                    <p className="hub-notice">上下文未延续，已在同一会话继续</p>
                  ) : null}
                </div>
                <div className="hub-turn-agent">
                  <EmployeeAvatar
                    asset={selectedEmployee ? employeeAvatarAsset(selectedEmployee) : null}
                    name={selectedEmployee?.name ?? "数字员工"}
                    size="sm"
                  />
                  <div className="hub-turn-agent-body">
                    {selectedEmployee ? (
                      <p className="hub-speaker">{selectedEmployee.name}</p>
                    ) : null}
                    {entry.status === "completed" ? (
                      <div className="hub-a">
                        <p>{entry.answer}</p>
                        <button
                          className="hub-convert"
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
                      <div className="hub-turn-error">
                        <p className="hub-turn-error-title">对话失败</p>
                        {entry.error ? (
                          <p className="hub-turn-error-desc">{entry.error}</p>
                        ) : null}
                        <button
                          className="hub-retry"
                          disabled={sendMutation.isPending}
                          onClick={() => handleRetry(entry)}
                          type="button"
                        >
                          重试
                        </button>
                      </div>
                    ) : (
                      <div className="hub-agent-pending">
                        <LoadingState label="数字员工思考中…" />
                      </div>
                    )}
                  </div>
                </div>
              </div>
            ))}
          </div>

          {restoreFailed ? (
            <p className="hub-send-error">历史对话恢复失败，可直接开始新对话</p>
          ) : null}
          {sendError ? <p className="hub-send-error">{sendError}</p> : null}

          <div className="hub-composer">
            {projectId && chatSkillOptions.length > 0 ? (
              <div className="hub-skill-row" data-testid="chat-skill-chips">
                <p className="hub-skill-label">本轮技能（不选则用项目默认面）</p>
                {chatSkillOptions.map((skill) => {
                  const active = selectedSkillIds.includes(skill.id);
                  return (
                    <button
                      aria-pressed={active}
                      className={active ? "hub-skill is-active" : "hub-skill"}
                      key={skill.id}
                      onClick={() => toggleSkill(skill.id)}
                      type="button"
                    >
                      {skill.label}
                    </button>
                  );
                })}
              </div>
            ) : null}
            <div className="hub-composer-box">
              <textarea
                aria-label="对话问题"
                className="hub-textarea"
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
            </div>
            <div className="hub-composer-foot">
              <p className="hub-composer-hint">Shift+Enter 发送</p>
              <button
                className="hub-send"
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
        </section>

        <aside aria-label="项目现场" className="hub-rail hub-rail-right" data-testid="chat-context-rail">
          <p className="hub-rail-label">项目现场</p>
          <div className="hub-rail-section">
            {projectId ? (
              <ProjectWorkspaceGitPanel
                onRefresh={() => refreshGitMutation.mutate()}
                pending={refreshGitMutation.isPending || Boolean(projectDetailQuery.data?.workspace_git?.refresh_pending)}
                status={projectDetailQuery.data?.workspace_git}
              />
            ) : (
              <p className="hub-rail-line">选择项目后显示 git 状态</p>
            )}
          </div>
          <div className="hub-rail-section">
            <p className="hub-rail-label">技能面</p>
            {!projectId ? (
              <p className="hub-rail-line">选择项目后显示</p>
            ) : skillBindingsQuery.isLoading ? (
              <p className="hub-rail-line">加载中…</p>
            ) : chatSkillOptions.length === 0 ? (
              <p className="hub-rail-line">项目尚未绑定技能（Chat 默认面为空）</p>
            ) : (
              <p className="hub-rail-line">
                {chatSkillOptions
                  .map((skill) =>
                    selectedSkillIds.length === 0 || selectedSkillIds.includes(skill.id)
                      ? `${skill.label} · 将投影`
                      : `${skill.label} · 未选`,
                  )
                  .join("；")}
              </p>
            )}
          </div>
          <p className="hub-rail-foot">
            Chat 不进审批/验收；动手会改工作区，正式进入项目需「转为任务」。
          </p>
        </aside>
      </div>

      <SoftDialog
        open={Boolean(pendingConfirm)}
        onOpenChange={(open) => {
          if (!open) {
            setPendingConfirm(null);
          }
        }}
      >
        <SoftDialogContent size="sm">
          <SoftDialogHeader>
            <SoftDialogTitle>确认开跑</SoftDialogTitle>
            <SoftDialogDescription>
              Chat 不进审批/验收，动手会改工作区。本确认仅本轮有效，不进收件箱。
            </SoftDialogDescription>
          </SoftDialogHeader>
          <SoftDialogBody className="space-y-2 text-sm text-ink-2">
            {selectedSkillIds.length > 0 ? (
              <p>本轮显式选用 {selectedSkillIds.length} 个技能。</p>
            ) : null}
            {projectAutonomyCeiling === "pause_at_gate" ? (
              <p>项目自治上限为「遇闸暂停」，交互侧需点头后再开跑。</p>
            ) : null}
            {selectedSkillIds.length === 0 && projectAutonomyCeiling !== "pause_at_gate" ? (
              <p>本轮需确认后再开跑。</p>
            ) : null}
          </SoftDialogBody>
          <SoftDialogFooter>
            <Button variant="outline" onClick={() => setPendingConfirm(null)}>
              取消
            </Button>
            <Button onClick={confirmPendingSend}>确认开跑</Button>
          </SoftDialogFooter>
        </SoftDialogContent>
      </SoftDialog>
    </div>
  );
}

function formatRelativeTime(value: string): string {
  const then = Date.parse(value);
  if (Number.isNaN(then)) {
    return "";
  }
  const deltaSec = Math.round((Date.now() - then) / 1000);
  if (deltaSec < 60) {
    return "刚刚";
  }
  if (deltaSec < 3600) {
    return `${Math.floor(deltaSec / 60)} 分钟前`;
  }
  if (deltaSec < 86400) {
    return `${Math.floor(deltaSec / 3600)} 小时前`;
  }
  if (deltaSec < 86400 * 7) {
    return `${Math.floor(deltaSec / 86400)} 天前`;
  }
  return new Date(then).toLocaleDateString("zh-CN");
}

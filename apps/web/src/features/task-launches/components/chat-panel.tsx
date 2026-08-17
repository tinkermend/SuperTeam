import {
  useEffect,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowRightLeft,
  Check,
  ChevronDown,
  Copy,
  FolderOpen,
  MessageCircle,
  MessageSquarePlus,
  SendHorizontal,
  Square,
  UserRound
} from "lucide-react";
import { EmptyState, ErrorState, LoadingState, SoftDialog, SoftDialogBody, SoftDialogContent, SoftDialogDescription, SoftDialogFooter, SoftDialogHeader, SoftDialogTitle, Button, MarkdownProse } from "@/components/superteam";
import { EmployeeAvatar } from "@/features/employees/avatar";
import { employeeAvatarAsset } from "@/features/employees/avatar-library";
import { UserIdentityAvatar } from "@/components/superteam/user-identity";
import "./task-hub-chat.css";
import { ApiRequestError, type ApiClientOptions } from "@/lib/api/client";
import { getCurrentUser } from "@/lib/api/auth";
import {
  createDigitalEmployeeRun,
  getDigitalEmployeeRun,
  listDigitalEmployeeChatThreads,
  listDigitalEmployeeRuns,
  listDigitalEmployees,
  stopDigitalEmployeeRun,
  type DigitalEmployeeChatThread,
  type DigitalEmployeeRun,
  type DigitalEmployeeRunListItem,
  type DigitalEmployeeRunStatus
} from "@/lib/api/employees";
import { listProjectMembers, getProject, refreshProjectWorkspaceGitStatus, type Project } from "@/lib/api/projects";
import { listProjectSkillBindings } from "@/lib/api/skills";
import { failureFamilyLabel } from "@/lib/status-labels";
import { ProjectWorkspaceGitPanel } from "@/features/projects/components/project-workspace-git-panel";
import { type ProjectChangeHandler } from "./task-launch-form";
import { HubTopbar } from "./hub-topbar";
import { useStickToBottom } from "./use-stick-to-bottom";
import { useChatActivityStream } from "./use-chat-activity-stream";
import { ChatLiveProcess } from "./chat-live-process";
import { CHAT_HISTORY_WINDOW_SIZE, sliceChatHistoryWindow } from "./chat-history-window";

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
  errorFamily?: string;
  createdAt?: string;
  completedAt?: string;
  durationSec?: number;
  creatorDisplayName?: string;
  /** Set when this entry was produced by an automatic no-resume retry after the
   * server rejected `resume_of_run_id` (lost/invalid session) — surfaces a
   * non-blocking "上下文未延续" hint instead of silently dropping the context. */
  contextNotContinued?: boolean;
};

const COMPOSER_MAX_HEIGHT_PX = 168;

export function durationSecFromRun(run: {
  completed_at?: string;
  created_at?: string;
  duration_sec?: number;
  finished_at?: string;
  started_at?: string;
}): number | undefined {
  if (typeof run.duration_sec === "number" && Number.isFinite(run.duration_sec)) {
    return Math.max(0, Math.round(run.duration_sec));
  }
  const start = run.started_at ?? run.created_at;
  const end = run.completed_at ?? run.finished_at;
  if (!start || !end) {
    return undefined;
  }
  const ms = Date.parse(end) - Date.parse(start);
  if (!Number.isFinite(ms) || ms < 0) {
    return undefined;
  }
  return Math.round(ms / 1000);
}

export function formatChatDuration(seconds: number): string {
  const total = Math.max(0, Math.round(seconds));
  const minutes = Math.floor(total / 60);
  const remain = total % 60;
  if (minutes === 0) {
    return `${remain}秒`;
  }
  return `${minutes}分${remain}秒`;
}

export function formatChatTurnTime(iso?: string): string | undefined {
  if (!iso) {
    return undefined;
  }
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return undefined;
  }
  return date.toLocaleString("zh-CN", {
    hour: "2-digit",
    hour12: false,
    minute: "2-digit",
    month: "numeric",
    day: "numeric",
    timeZone: "Asia/Shanghai",
  });
}

export function chatTurnErrorTitle(
  status: ChatEntry["status"],
  errorFamily?: string,
): string {
  if (status === "cancelled") {
    return "对话已停止";
  }
  if (status === "timed_out") {
    return failureFamilyLabel("timeout");
  }
  if (errorFamily) {
    return failureFamilyLabel(errorFamily);
  }
  return "对话失败";
}

function HumanChatAvatar({
  name,
  self,
}: {
  name?: string;
  self?: {
    avatar?: { provider: "dicebear"; seed: string; style: "adventurer"; svg?: string; options?: Record<string, unknown> };
    avatar_asset_id?: string | null;
    display_name?: string | null;
    id: string;
    status: string;
    username?: string;
  };
}) {
  const selfName = self?.display_name?.trim() || self?.username?.trim();
  const isSelf = Boolean(self && (!name || name === selfName));
  if (isSelf && self) {
    return (
      <UserIdentityAvatar
        className="hub-human-avatar"
        selfHeal
        user={self}
      />
    );
  }
  const initial = (name ?? "?").trim().slice(0, 1).toUpperCase() || "?";
  return (
    <span aria-label={name ?? "用户"} className="hub-human-avatar-fallback">
      {initial}
    </span>
  );
}

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
  /** 父层 listProjects 失败时不要当成「没有项目」。 */
  projectsError?: boolean;
  /** 父层 listProjects 进行中；空数组在加载期不是「没有项目」。 */
  projectsLoading?: boolean;
  /** 父层缓存的已选项目（搜索越界）。 */
  resolvedProject?: Project | null;
  /** 工具条前置槽：任务中枢把「对话 / 任务」页签放进项目切换同一条工具条，
   * 顶部只留一截控件；独立渲染 ChatPanel 时不传即可。 */
  topbarLead?: ReactNode;
};

function isActiveEntryStatus(status: DigitalEmployeeRunStatus | "sending"): boolean {
  return status === "sending" || ACTIVE_RUN_STATUSES.has(status as DigitalEmployeeRunStatus);
}

export function chatRestoreQueryKey(
  employeeId: string,
  projectId: string,
  threadId: string,
): readonly ["chat-restore", string, string, string] {
  return ["chat-restore", employeeId, projectId, threadId];
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

/** JSON.stringify 兜底的整段结果，不当 markdown 渲染。 */
export function isJsonDumpAnswer(text: string): boolean {
  const trimmed = text.trim();
  if (!(trimmed.startsWith("{") || trimmed.startsWith("["))) {
    return false;
  }
  try {
    JSON.parse(trimmed);
    return true;
  } catch {
    return false;
  }
}

function isTerminalEntryStatus(status: ChatEntry["status"]): boolean {
  return TERMINAL_RUN_STATUSES.has(status as DigitalEmployeeRunStatus);
}

function pickRicherEntry(server: ChatEntry, local: ChatEntry): ChatEntry {
  const localAhead =
    isActiveEntryStatus(server.status) && !isActiveEntryStatus(local.status);
  const localHasAnswer = Boolean(local.answer) && !server.answer;
  const localHasError = Boolean(local.error) && !server.error;
  const contextNotContinued = Boolean(server.contextNotContinued || local.contextNotContinued);
  const merged = localAhead || localHasAnswer || localHasError
    ? { ...server, ...local }
    : { ...local, ...server };
  if (contextNotContinued) {
    merged.contextNotContinued = true;
  } else {
    delete merged.contextNotContinued;
  }
  return merged;
}

/** 按 runId 合并：服务端顺序为时间轴，仅把本地独有轮次（发送中）接到末尾。 */
export function mergeChatThread(server: ChatEntry[], local: ChatEntry[]): ChatEntry[] {
  const byId = new Map<string, ChatEntry>();
  const order: string[] = [];
  const take = (entry: ChatEntry, incoming: "local" | "server") => {
    const previous = byId.get(entry.runId);
    if (!previous) {
      order.push(entry.runId);
      byId.set(entry.runId, entry);
      return;
    }
    byId.set(
      entry.runId,
      incoming === "local" ? pickRicherEntry(previous, entry) : pickRicherEntry(entry, previous),
    );
  };
  for (const entry of server) {
    take(entry, "server");
  }
  for (const entry of local) {
    take(entry, "local");
  }
  return order.map((runId) => byId.get(runId)!);
}

function patchEntryFromLiveRun(run: DigitalEmployeeRun, previous?: ChatEntry): ChatEntry {
  const entry: ChatEntry = {
    question: previous?.question ?? "",
    runId: run.id,
    status: run.status,
    creatorDisplayName: run.creator_display_name ?? previous?.creatorDisplayName,
    contextNotContinued: previous?.contextNotContinued,
    createdAt: run.created_at ?? previous?.createdAt,
    completedAt: run.completed_at ?? run.finished_at ?? previous?.completedAt,
    durationSec: durationSecFromRun(run) ?? previous?.durationSec,
    errorFamily: run.error_family ?? previous?.errorFamily,
  };
  if (run.status === "completed") {
    entry.answer = extractAnswerText(run);
  } else if (isTerminalEntryStatus(run.status)) {
    entry.error = run.error_message ?? "对话执行失败，请重试";
  } else if (previous?.answer) {
    entry.answer = previous.answer;
  }
  return entry;
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
    createdAt: item.created_at,
    completedAt: item.completed_at ?? item.finished_at,
    durationSec: durationSecFromRun(item),
    errorFamily: item.error_family,
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
  projectsError = false,
  projectsLoading = false,
  resolvedProject = null,
  topbarLead,
}: ChatPanelProps) {
  const [employeeId, setEmployeeId] = useState("");
  const [question, setQuestion] = useState("");
  const [sendError, setSendError] = useState("");
  const [copiedRunId, setCopiedRunId] = useState("");
  const composerRef = useRef<HTMLTextAreaElement>(null);
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
    pendingId: string;
  } | null>(null);
  const [extraRevealed, setExtraRevealed] = useState(0);
  /** 发送中/新会话尚未挂上 thread 时仍要立刻画出这一轮，避免空窗直到刷新。 */
  const [pendingEntries, setPendingEntries] = useState<ChatEntry[]>([]);

  const queryClient = useQueryClient();
  const threadRef = useRef<HTMLDivElement | null>(null);
  const threadInnerRef = useRef<HTMLDivElement | null>(null);
  const revealSnapshotRef = useRef<{ scrollHeight: number; scrollTop: number } | null>(null);
  const activityLiveRef = useRef(false);

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
    refetchInterval: () => (activityLiveRef.current ? 30_000 : 5000),
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
      const threadId = selectedThreadId!;
      const key = chatRestoreQueryKey(employeeId, projectId, threadId);
      const cachedBefore = queryClient.getQueryData<ChatEntry[]>(key) ?? [];
      const runs = await listDigitalEmployeeRuns(apiOptions, employeeId, {
        chat_thread_id: threadId,
        limit: 100,
        project_id: projectId,
        run_kind: "chat",
      });
      const server = [...runs.items].reverse().map(entryFromRunListItem);
      const cachedAfter = queryClient.getQueryData<ChatEntry[]>(key) ?? cachedBefore;
      return mergeChatThread(server, cachedAfter);
    },
  });

  const restoreFailed = Boolean(selectedThreadId) && restoreQuery.isError;
  const restoring =
    Boolean(employeeId && projectId && selectedThreadId) &&
    restoreQuery.isPending &&
    !restoreQuery.data;

  const sendInFlightRef = useRef(false);

  const displayThread = mergeChatThread(restoreQuery.data ?? [], pendingEntries);
  const { hidden: hiddenEarlierCount, visible: visibleThread } = sliceChatHistoryWindow(
    displayThread,
    extraRevealed,
  );

  useEffect(() => {
    setExtraRevealed(0);
  }, [employeeId, projectId, selectedThreadId]);

  useLayoutEffect(() => {
    const snapshot = revealSnapshotRef.current;
    const node = threadRef.current;
    if (!snapshot || !node) {
      return;
    }
    revealSnapshotRef.current = null;
    const insertedHeight = node.scrollHeight - snapshot.scrollHeight;
    if (insertedHeight > 0) {
      node.scrollTop = snapshot.scrollTop + insertedHeight;
    }
  }, [extraRevealed, hiddenEarlierCount]);

  const lastCompleted = [...displayThread].reverse().find((entry) => entry.status === "completed");
  const activeEntry = displayThread.find((entry) => isActiveEntryStatus(entry.status));
  const runQueryEnabled = Boolean(activeEntry && employeeId && activeEntry.status !== "sending");

  const activityLive = useChatActivityStream({
    apiBaseUrl: apiOptions.baseUrl,
    employeeId,
    enabled: Boolean(employeeId),
    runId: activeEntry?.runId,
  });
  activityLiveRef.current = activityLive;

  const runQuery = useQuery({
    enabled: runQueryEnabled,
    queryFn: () => getDigitalEmployeeRun(apiOptions, employeeId, activeEntry!.runId),
    queryKey: ["chat-run", employeeId, activeEntry?.runId],
    refetchInterval: runQueryEnabled && !activityLive ? 10_000 : false,
  });

  useEffect(() => {
    const run = runQuery.data;
    if (!run || !employeeId || !projectId) {
      return;
    }
    const threadId = run.chat_thread_id ?? selectedThreadId;
    if (!threadId) {
      return;
    }
    queryClient.setQueryData<ChatEntry[]>(
      chatRestoreQueryKey(employeeId, projectId, threadId),
      (prev = []) => {
        const existing = prev.find((entry) => entry.runId === run.id);
        if (!existing) {
          return prev;
        }
        return prev.map((entry) => (entry.runId === run.id ? patchEntryFromLiveRun(run, existing) : entry));
      },
    );
    setPendingEntries((prev) =>
      prev.map((entry) => (entry.runId === run.id ? patchEntryFromLiveRun(run, entry) : entry)),
    );
  }, [employeeId, projectId, queryClient, runQuery.data, selectedThreadId]);

  const sendMutation = useMutation({
    mutationFn: (input: {
      objective: string;
      resumeOf?: string;
      chatThreadId?: string;
      degraded?: boolean;
      interactiveConfirmed?: boolean;
      pendingId: string;
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
      const threadId = run.chat_thread_id || selectedThreadId || run.id;
      const nextEntry = patchEntryFromLiveRun(run, {
        question: variables.objective,
        runId: run.id,
        status: run.status,
        contextNotContinued: variables.degraded || undefined,
      });
      queryClient.setQueryData<ChatEntry[]>(
        chatRestoreQueryKey(employeeId, projectId, threadId),
        (prev = []) => mergeChatThread(prev, [nextEntry]),
      );
      setPendingEntries((prev) =>
        prev.map((entry) => (entry.runId === variables.pendingId ? nextEntry : entry)),
      );
      if (threadId !== selectedThreadId) {
        setSelectedThreadId(threadId);
      }
      setExplicitNewSession(false);
      void threadsQuery.refetch();
    },
  });

  const stopMutation = useMutation({
    mutationFn: () =>
      stopDigitalEmployeeRun(apiOptions, employeeId, activeEntry!.runId, {
        reason: "用户停止对话",
      }),
    onSuccess: (run) => {
      const threadId = run.chat_thread_id ?? selectedThreadId;
      if (!threadId) {
        return;
      }
      queryClient.setQueryData<ChatEntry[]>(
        chatRestoreQueryKey(employeeId, projectId, threadId),
        (prev = []) => {
          const existing = prev.find((entry) => entry.runId === run.id);
          if (!existing) {
            return prev;
          }
          return prev.map((entry) =>
            entry.runId === run.id ? patchEntryFromLiveRun(run, existing) : entry,
          );
        },
      );
    },
    onError: (error) => {
      setSendError(error instanceof Error ? error.message : "停止失败，请重试");
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
    resumeOf: string | undefined,
    interactiveConfirmed: boolean | undefined,
    pendingId: string,
  ) {
    const chatThreadId = selectedThreadId ?? undefined;
    try {
      return await sendMutation.mutateAsync({
        objective,
        resumeOf,
        chatThreadId,
        interactiveConfirmed,
        pendingId,
      });
    } catch (error) {
      if (
        !interactiveConfirmed &&
        isInteractiveConfirmRequiredError(error)
      ) {
        setSendError("");
        setPendingConfirm({ objective, resumeOf, pendingId });
        return undefined;
      }
      if (resumeOf && error instanceof ApiRequestError && error.status === 400) {
        try {
          return await sendMutation.mutateAsync({
            objective,
            chatThreadId,
            degraded: true,
            interactiveConfirmed,
            pendingId,
          });
        } catch (retryError) {
          if (
            !interactiveConfirmed &&
            isInteractiveConfirmRequiredError(retryError)
          ) {
            setSendError("");
            setPendingConfirm({ objective, resumeOf: undefined, pendingId });
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
    setSendError("");
    setPendingEntries([]);
  }

  function handleProjectChange(project: Project) {
    onProjectChange(project);
    setSelectedThreadId(null);
    setExplicitNewSession(false);
    setSendError("");
    setSelectedSkillIds([]);
    setPendingEntries([]);
  }

  function handleSelectThread(threadId: string) {
    if (threadId === selectedThreadId || sendInFlightRef.current) {
      return;
    }
    setExplicitNewSession(false);
    setSelectedThreadId(threadId);
    setSendError("");
    setPendingEntries([]);
  }

  function toggleSkill(skillId: string) {
    setSelectedSkillIds((prev) =>
      prev.includes(skillId) ? prev.filter((id) => id !== skillId) : [...prev, skillId],
    );
  }

  // 新对话是断链的唯一主动入口：清空选中 thread，下一条消息开新根。
  function handleNewConversation() {
    if (sendInFlightRef.current) {
      return;
    }
    setExplicitNewSession(true);
    setSelectedThreadId(null);
    setSendError("");
    setPendingEntries([]);
  }

  function speakerName() {
    const user = currentUserQuery.data?.user;
    return user?.display_name?.trim() || user?.username?.trim() || undefined;
  }

  function attachPending(objective: string): string {
    const pendingId = `pending:${crypto.randomUUID()}`;
    setPendingEntries((prev) => [
      ...prev,
      {
        creatorDisplayName: speakerName(),
        question: objective,
        runId: pendingId,
        status: "sending",
      },
    ]);
    return pendingId;
  }

  function beginSend(objective: string, resumeOf?: string) {
    if (needsInteractiveConfirm) {
      setPendingConfirm({ objective, pendingId: "", resumeOf });
      return;
    }
    const pendingId = attachPending(objective);
    sendInFlightRef.current = true;
    sendWithDegradeFallback(objective, resumeOf, undefined, pendingId)
      .then((run) => {
        if (run) {
          setQuestion("");
        }
      })
      .catch(() => {
        setPendingEntries((prev) => prev.filter((entry) => entry.runId !== pendingId));
      })
      .finally(() => {
        sendInFlightRef.current = false;
      });
  }

  function dismissConfirm() {
    const pendingId = pendingConfirm?.pendingId;
    setPendingConfirm(null);
    if (pendingId) {
      setPendingEntries((prev) => prev.filter((entry) => entry.runId !== pendingId));
    }
  }

  function confirmPendingSend() {
    if (!pendingConfirm || sendInFlightRef.current) {
      return;
    }
    const { objective, resumeOf } = pendingConfirm;
    const pendingId = pendingConfirm.pendingId || attachPending(objective);
    setPendingConfirm(null);
    sendInFlightRef.current = true;
    sendWithDegradeFallback(objective, resumeOf, true, pendingId)
      .then((run) => {
        if (run) {
          setQuestion("");
        }
      })
      .catch(() => {
        setPendingEntries((prev) => prev.filter((entry) => entry.runId !== pendingId));
      })
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

  function handleRevealEarlier() {
    const node = threadRef.current;
    if (node) {
      revealSnapshotRef.current = {
        scrollHeight: node.scrollHeight,
        scrollTop: node.scrollTop,
      };
    }
    setExtraRevealed((current) => current + CHAT_HISTORY_WINDOW_SIZE);
  }

  function handleConvert(entry: ChatEntry) {
    onConvertToTask({
      anchorProjectId: projectId,
      chatRunId: entry.runId,
      digitalEmployeeId: employeeId,
      draft: buildTaskDraft(entry, selectedEmployee?.name ?? "")
});
  }

  function handleCopyAnswer(entry: ChatEntry) {
    const text = entry.answer?.trim();
    if (!text || !navigator.clipboard?.writeText) {
      return;
    }
    void navigator.clipboard.writeText(text).then(() => {
      setCopiedRunId(entry.runId);
      window.setTimeout(() => {
        setCopiedRunId((current) => (current === entry.runId ? "" : current));
      }, 1600);
    });
  }

  useLayoutEffect(() => {
    const node = composerRef.current;
    if (!node) {
      return;
    }
    node.style.height = "auto";
    node.style.height = `${Math.min(node.scrollHeight, COMPOSER_MAX_HEIGHT_PX)}px`;
  }, [question]);

  const canSend =
    Boolean(question.trim()) &&
    Boolean(employeeId) &&
    Boolean(projectId) &&
    !restoring &&
    !activeEntry &&
    !sendMutation.isPending &&
    // Wait for project policy so pause_at_gate SoftDialog is not skipped on first paint.
    (!projectId || !projectDetailQuery.isPending);

  const lastStick = displayThread.at(-1);
  const stickPinToken = `${selectedThreadId ?? ""}:${lastStick?.runId ?? ""}`;
  const stickContentKey = `${displayThread.length}:${lastStick?.runId ?? ""}:${lastStick?.status ?? ""}:${lastStick?.answer?.length ?? 0}`;
  const { awayFromBottom, jumpToBottom } = useStickToBottom(
    threadRef,
    stickContentKey,
    stickPinToken,
    threadInnerRef,
  );

  return (
    <div className="hub-chat">
      <HubTopbar
        apiOptions={apiOptions}
        lead={topbarLead}
        onProjectChange={handleProjectChange}
        projectId={projectId}
        projects={projects}
        projectsError={projectsError}
        projectsLoading={projectsLoading}
        resolvedProject={resolvedProject}
      />

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

          <div className="hub-thread" data-testid="chat-thread" ref={threadRef}>
            <div className="hub-thread-inner" ref={threadInnerRef}>
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
            {hiddenEarlierCount > 0 ? (
              <button
                className="hub-history-chip"
                onClick={handleRevealEarlier}
                type="button"
              >
                显示之前的 {Math.min(hiddenEarlierCount, CHAT_HISTORY_WINDOW_SIZE)} 条消息
              </button>
            ) : null}
            {visibleThread.map((entry) => (
              <div className="hub-entry" key={entry.runId}>
                <div className="hub-turn-human">
                  <div className="hub-turn-human-body">
                    {entry.creatorDisplayName ? (
                      <p className="hub-speaker">{entry.creatorDisplayName}</p>
                    ) : null}
                    <p className="hub-q">{entry.question}</p>
                    {entry.contextNotContinued ? (
                      <p className="hub-notice">上下文未延续，已在同一会话继续</p>
                    ) : null}
                  </div>
                  <HumanChatAvatar
                    name={entry.creatorDisplayName}
                    self={currentUserQuery.data?.user}
                  />
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
                        <ChatLiveProcess
                          apiOptions={apiOptions}
                          employeeId={employeeId}
                          pollIntervalMs={false}
                          runId={entry.runId}
                          variant="settled"
                        />
                        {entry.answer && isJsonDumpAnswer(entry.answer) ? (
                          <pre className="hub-a-json">{entry.answer}</pre>
                        ) : (
                          <MarkdownProse className="hub-md">{entry.answer ?? ""}</MarkdownProse>
                        )}
                        <div className="hub-a-foot">
                          <p className="hub-a-meta">
                            {[
                              formatChatTurnTime(entry.completedAt ?? entry.createdAt),
                              entry.durationSec != null ? formatChatDuration(entry.durationSec) : undefined,
                            ]
                              .filter(Boolean)
                              .join(" · ")}
                          </p>
                          <button
                            className="hub-a-action"
                            onClick={() => handleCopyAnswer(entry)}
                            type="button"
                          >
                            {copiedRunId === entry.runId ? (
                              <Check aria-hidden className="size-3.5" />
                            ) : (
                              <Copy aria-hidden className="size-3.5" />
                            )}
                            {copiedRunId === entry.runId ? "已复制" : "复制"}
                          </button>
                          <button
                            className="hub-convert"
                            onClick={() => handleConvert(entry)}
                            type="button"
                          >
                            <ArrowRightLeft aria-hidden className="size-3.5" />
                            转为任务
                          </button>
                        </div>
                      </div>
                    ) : entry.status === "failed" ||
                      entry.status === "cancelled" ||
                      entry.status === "timed_out" ? (
                      <div className="hub-turn-error">
                        <ChatLiveProcess
                          apiOptions={apiOptions}
                          employeeId={employeeId}
                          pollIntervalMs={false}
                          runId={entry.runId}
                          variant="settled"
                        />
                        <p className="hub-turn-error-title">
                          {chatTurnErrorTitle(entry.status, entry.errorFamily)}
                        </p>
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
                        <ChatLiveProcess
                          apiOptions={apiOptions}
                          employeeId={employeeId}
                          pollIntervalMs={activityLive ? false : 10_000}
                          runId={entry.runId}
                          variant="live"
                        />
                    )}
                  </div>
                </div>
              </div>
            ))}
            </div>
            {awayFromBottom ? (
              <button className="hub-jump" onClick={jumpToBottom} type="button">
                <ChevronDown aria-hidden className="size-3.5" />
                {activeEntry ? "有新消息" : "回到底部"}
              </button>
            ) : null}
            {restoreFailed ? (
              <p className="hub-send-error">历史对话恢复失败，可直接开始新对话</p>
            ) : null}
            {sendError ? <p className="hub-send-error">{sendError}</p> : null}
          </div>

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
                  if (event.key !== "Enter") {
                    return;
                  }
                  if (event.nativeEvent.isComposing || event.keyCode === 229) {
                    return;
                  }
                  if (event.shiftKey) {
                    return;
                  }
                  event.preventDefault();
                  handleSend();
                }}
                placeholder="向数字员工提问，回答不会写入项目流转"
                ref={composerRef}
                rows={2}
                value={question}
              />
            </div>
            <div className="hub-composer-foot">
              <p className="hub-composer-hint">
                {activeEntry ? "运行中可查看历史并切换会话" : "Enter 发送，Shift+Enter 换行"}
              </p>
              {activeEntry ? (
                <button
                  className="hub-stop"
                  disabled={stopMutation.isPending || activeEntry.status === "cancelling"}
                  onClick={() => stopMutation.mutate()}
                  type="button"
                >
                  {activeEntry.status === "cancelling" ? "正在停止…" : "停止"}
                  <Square aria-hidden className="size-3.5 fill-current" />
                </button>
              ) : (
                <button
                  className="hub-send"
                  disabled={!canSend}
                  onClick={handleSend}
                  title="发送（Enter）"
                  type="button"
                >
                  发送
                  <SendHorizontal aria-hidden className="size-4" />
                </button>
              )}
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
            dismissConfirm();
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
            <Button variant="outline" onClick={dismissConfirm}>
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

import { useEffect, useMemo, useRef, useState } from "react";
import type { RuntimeOverviewActivityItem, RuntimeOverviewEmployee } from "./runtime-overview-model";
import type { ProjectRunBandOption } from "./runtime-overview-project-lens";

// 大屏模式镜头编排：员工焦点镜头 + 项目链路镜头循环轮转，SSE/轮询发现新失败时异常插队。
// 纯函数(队列构建/失败检测)与 hook(计时/插队/交互暂停)分离,前者可单测。

export type DisplayShot =
  | { kind: "employee"; employeeId: string; alert?: boolean }
  | { kind: "project"; projectId: string };

// 员工镜头准入与排序沿用焦点轮播口径:非空闲员工按紧迫度(异常>待人工>工作中>排队)。
const SHOT_STATUS_RANK: Partial<Record<RuntimeOverviewEmployee["status"], number>> = {
  error: 0,
  waiting_human: 1,
  working: 2,
  queued: 3,
};

export const DISPLAY_EMPLOYEE_DWELL_MS = 10_000;
export const DISPLAY_PROJECT_DWELL_MS = 15_000;
export const DISPLAY_ALERT_DWELL_MS = 20_000;
export const DISPLAY_RESUME_AFTER_MS = 45_000;

// 镜头队列 = 员工段(紧迫度序) + 项目段(run-summary 服务端序,仅有活跃任务的项目)。
// 队列循环播放即形成"员工焦点 → 项目链路"的段落交替。
export function buildDisplayShotQueue(
  employees: RuntimeOverviewEmployee[],
  projects: ProjectRunBandOption[],
): DisplayShot[] {
  const employeeShots: DisplayShot[] = employees
    .filter((employee) => SHOT_STATUS_RANK[employee.status] !== undefined)
    .sort((a, b) => {
      const rankDiff = (SHOT_STATUS_RANK[a.status] ?? 9) - (SHOT_STATUS_RANK[b.status] ?? 9);
      if (rankDiff !== 0) return rankDiff;
      if (a.statusSince !== b.statusSince) {
        if (!a.statusSince) return 1;
        if (!b.statusSince) return -1;
        return a.statusSince.localeCompare(b.statusSince);
      }
      return a.employeeId.localeCompare(b.employeeId);
    })
    .map((employee) => ({ kind: "employee", employeeId: employee.employeeId }));
  const projectShots: DisplayShot[] = projects
    .filter((project) => project.hasActive)
    .map((project) => ({ kind: "project", projectId: project.projectId }));
  return [...employeeShots, ...projectShots];
}

export function shotKey(shot: DisplayShot): string {
  return shot.kind === "employee" ? `employee:${shot.employeeId}` : `project:${shot.projectId}`;
}

function activityKey(item: RuntimeOverviewActivityItem): string {
  return `${item.employeeId}|${item.label}|${item.occurredAt ?? ""}`;
}

// 失败插队检测:与上一批活动流 diff,只报"新出现的 failed 条目"。
// previous 为 undefined(首批数据)时不报——避免大屏刚打开就被历史失败霸屏。
export function detectNewFailures(
  previous: RuntimeOverviewActivityItem[] | undefined,
  next: RuntimeOverviewActivityItem[],
): RuntimeOverviewActivityItem[] {
  if (!previous) return [];
  const seen = new Set(previous.map(activityKey));
  return next.filter((item) => item.status === "failed" && !seen.has(activityKey(item)));
}

export function shotDwellMs(shot: DisplayShot | undefined): number {
  if (!shot) return DISPLAY_EMPLOYEE_DWELL_MS;
  if (shot.kind === "employee" && shot.alert) return DISPLAY_ALERT_DWELL_MS;
  if (shot.kind === "project") return DISPLAY_PROJECT_DWELL_MS;
  return DISPLAY_EMPLOYEE_DWELL_MS;
}

type UseDisplayCameraInput = {
  enabled: boolean;
  employees: RuntimeOverviewEmployee[];
  projects: ProjectRunBandOption[];
  activity?: RuntimeOverviewActivityItem[];
};

export type DisplayCamera = {
  shot?: DisplayShot;
  queueLength: number;
  isPaused: boolean;
  notifyInteraction: () => void;
  resume: () => void;
};

export function useDisplayCamera({ enabled, employees, projects, activity }: UseDisplayCameraInput): DisplayCamera {
  const queue = useMemo(() => buildDisplayShotQueue(employees, projects), [employees, projects]);
  const queueRef = useRef(queue);
  queueRef.current = queue;
  const queueKey = queue.map(shotKey).join("|");

  const [shot, setShot] = useState<DisplayShot>();
  const [isPaused, setIsPaused] = useState(false);
  const isPausedRef = useRef(isPaused);
  isPausedRef.current = isPaused;
  const resumeTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);

  // 队列变化校正:当前镜头出队则回到队首;停用/空队列清空镜头。
  useEffect(() => {
    if (!enabled) {
      setShot(undefined);
      return;
    }
    setShot((current) => {
      const activeQueue = queueRef.current;
      if (activeQueue.length === 0) return undefined;
      if (current && activeQueue.some((item) => shotKey(item) === shotKey(current))) return current;
      return activeQueue[0];
    });
  }, [enabled, queueKey]);

  // 驻留计时:按镜头类型取驻留时长,到点推进下一镜头(插队镜头播完自然回到队列)。
  const shotIdentity = shot ? `${shotKey(shot)}${shot.kind === "employee" && shot.alert ? ":alert" : ""}` : "";
  useEffect(() => {
    if (!enabled || isPaused || !shot || queueRef.current.length <= 1) return;
    const timer = setTimeout(() => {
      setShot((current) => {
        const activeQueue = queueRef.current;
        if (activeQueue.length === 0) return undefined;
        const index = current ? activeQueue.findIndex((item) => shotKey(item) === shotKey(current)) : -1;
        return activeQueue[(index + 1) % activeQueue.length];
      });
    }, shotDwellMs(shot));
    return () => clearTimeout(timer);
  }, [enabled, isPaused, shotIdentity, queueKey]);

  // 异常插队:活动流出现新 failed 条目时,立即切到该员工的告警镜头(停留加倍);
  // 用户交互暂停期间不打断。
  const activitySnapshotRef = useRef<RuntimeOverviewActivityItem[] | undefined>(undefined);
  useEffect(() => {
    if (!enabled || !activity) return;
    const failures = detectNewFailures(activitySnapshotRef.current, activity);
    activitySnapshotRef.current = activity;
    if (failures.length === 0 || isPausedRef.current) return;
    setShot({ kind: "employee", employeeId: failures[0].employeeId, alert: true });
  }, [enabled, activity]);

  const notifyInteraction = () => {
    setIsPaused(true);
    if (resumeTimerRef.current) clearTimeout(resumeTimerRef.current);
    resumeTimerRef.current = setTimeout(() => setIsPaused(false), DISPLAY_RESUME_AFTER_MS);
  };

  const resume = () => {
    if (resumeTimerRef.current) clearTimeout(resumeTimerRef.current);
    setIsPaused(false);
  };

  useEffect(() => {
    return () => {
      if (resumeTimerRef.current) clearTimeout(resumeTimerRef.current);
    };
  }, []);

  return { shot: enabled ? shot : undefined, queueLength: queue.length, isPaused, notifyInteraction, resume };
}

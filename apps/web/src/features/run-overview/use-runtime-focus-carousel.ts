import { useEffect, useMemo, useRef, useState } from "react";
import type { RuntimeOverviewEmployee } from "./runtime-overview-model";

// 轮播队列 = 非空闲员工，按紧迫度排序；空闲/待配置/不可用不进队列。
const CAROUSEL_STATUS_RANK: Partial<Record<RuntimeOverviewEmployee["status"], number>> = {
  error: 0,
  waiting_human: 1,
  working: 2,
  queued: 3,
};

export const CAROUSEL_DEFAULT_DWELL_MS = 10_000;
export const CAROUSEL_DEFAULT_RESUME_MS = 45_000;

type UseRuntimeFocusCarouselInput = {
  employees: RuntimeOverviewEmployee[];
  // 初始即视为有人在交互（如带 ?employee= 深链打开），先暂停再自动恢复。
  initialInteracted?: boolean;
  // 强制暂停（如项目透镜态）：为 true 期间不轮播也不自动恢复，翻回 false 立即恢复。
  forcePaused?: boolean;
  dwellMs?: number;
  resumeAfterMs?: number;
};

export type RuntimeFocusCarousel = {
  focusEmployeeId?: string;
  queue: RuntimeOverviewEmployee[];
  queueIndex: number;
  isPaused: boolean;
  // 用户交互（点头像/切楼层/深链进入）：暂停轮播，超时自动恢复。
  notifyInteraction: () => void;
  resume: () => void;
};

export function useRuntimeFocusCarousel({
  employees,
  initialInteracted = false,
  forcePaused = false,
  dwellMs = CAROUSEL_DEFAULT_DWELL_MS,
  resumeAfterMs = CAROUSEL_DEFAULT_RESUME_MS,
}: UseRuntimeFocusCarouselInput): RuntimeFocusCarousel {
  const [focusEmployeeId, setFocusEmployeeId] = useState<string>();
  const [isPaused, setIsPaused] = useState(initialInteracted);
  const effectivePaused = isPaused || forcePaused;
  const resumeTimerRef = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const isPausedRef = useRef(effectivePaused);
  isPausedRef.current = effectivePaused;

  const queue = useMemo(() => {
    return employees
      .filter((employee) => CAROUSEL_STATUS_RANK[employee.status] !== undefined)
      .sort((a, b) => {
        const rankDiff = (CAROUSEL_STATUS_RANK[a.status] ?? 9) - (CAROUSEL_STATUS_RANK[b.status] ?? 9);
        if (rankDiff !== 0) return rankDiff;
        // 同紧迫度按滞留时长加权：进入状态越早（等/跑/滞留越久）越靠前；无时长信息排最后。
        if (a.statusSince !== b.statusSince) {
          if (!a.statusSince) return 1;
          if (!b.statusSince) return -1;
          return a.statusSince.localeCompare(b.statusSince);
        }
        if (a.floorId !== b.floorId) return a.floorId.localeCompare(b.floorId);
        return a.employeeId.localeCompare(b.employeeId);
      });
  }, [employees]);
  const queueRef = useRef(queue);
  queueRef.current = queue;
  const queueKey = queue.map((employee) => employee.employeeId).join("|");

  // 队列成员变化时校正焦点：焦点已出队则回到队首；队列为空则清空焦点。
  useEffect(() => {
    setFocusEmployeeId((current) => {
      const activeQueue = queueRef.current;
      if (activeQueue.length === 0) return undefined;
      if (current && activeQueue.some((employee) => employee.employeeId === current)) return current;
      return activeQueue[0]?.employeeId;
    });
  }, [queueKey]);

  // 驻留计时：每 dwellMs 前进一位；焦点变化（含插队）即重新计时。
  useEffect(() => {
    if (effectivePaused || queue.length <= 1) return;
    const timer = setTimeout(() => {
      setFocusEmployeeId((current) => {
        const activeQueue = queueRef.current;
        if (activeQueue.length === 0) return undefined;
        const index = activeQueue.findIndex((employee) => employee.employeeId === current);
        return activeQueue[(index + 1) % activeQueue.length]?.employeeId;
      });
    }, dwellMs);
    return () => clearTimeout(timer);
  }, [effectivePaused, queueKey, dwellMs, focusEmployeeId]);

  // 状态变化插队：与上次快照 diff，出现进入队列态的状态变化时立即抢占焦点（暂停时不打断用户）。
  const statusSnapshotRef = useRef(new Map<string, RuntimeOverviewEmployee["status"]>());
  useEffect(() => {
    const previous = statusSnapshotRef.current;
    const next = new Map(employees.map((employee) => [employee.employeeId, employee.status] as const));
    const changed = employees.filter((employee) => {
      const before = previous.get(employee.employeeId);
      return (
        before !== undefined &&
        before !== employee.status &&
        CAROUSEL_STATUS_RANK[employee.status] !== undefined
      );
    });
    statusSnapshotRef.current = next;
    if (previous.size === 0 || changed.length === 0 || isPausedRef.current) return;
    const preempted = changed.sort(
      (a, b) => (CAROUSEL_STATUS_RANK[a.status] ?? 9) - (CAROUSEL_STATUS_RANK[b.status] ?? 9),
    )[0];
    setFocusEmployeeId(preempted.employeeId);
  }, [employees]);

  const notifyInteraction = () => {
    setIsPaused(true);
    if (resumeTimerRef.current) clearTimeout(resumeTimerRef.current);
    resumeTimerRef.current = setTimeout(() => setIsPaused(false), resumeAfterMs);
  };

  const resume = () => {
    if (resumeTimerRef.current) clearTimeout(resumeTimerRef.current);
    setIsPaused(false);
  };

  useEffect(() => {
    if (initialInteracted) {
      resumeTimerRef.current = setTimeout(() => setIsPaused(false), resumeAfterMs);
    }
    return () => {
      if (resumeTimerRef.current) clearTimeout(resumeTimerRef.current);
    };
  }, []);

  const queueIndex = queue.findIndex((employee) => employee.employeeId === focusEmployeeId);

  return { focusEmployeeId, queue, queueIndex, isPaused: effectivePaused, notifyInteraction, resume };
}

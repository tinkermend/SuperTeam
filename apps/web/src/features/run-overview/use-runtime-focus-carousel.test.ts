import { describe, expect, it } from "vitest";
import { renderHook } from "vitest-browser-react";
import type { RuntimeOverviewEmployee } from "./runtime-overview-model";
import { useRuntimeFocusCarousel } from "./use-runtime-focus-carousel";

function employee(
  id: string,
  status: RuntimeOverviewEmployee["status"],
  floorId: RuntimeOverviewEmployee["floorId"] = "floor-1",
): RuntimeOverviewEmployee {
  return {
    employeeId: id,
    teamId: "team-1",
    floorId,
    name: id,
    roleLabel: "数字员工",
    status,
    statusReasons: [],
    recentEvents: [],
    projects: [],
    projectCount: 0,
    artifacts: [],
  };
}

describe("useRuntimeFocusCarousel", () => {
  it("builds the queue from non-idle employees ordered by urgency and focuses the head", async () => {
    const { result } = await renderHook(() =>
      useRuntimeFocusCarousel({
        employees: [
          employee("idle-1", "idle"),
          employee("work-1", "working"),
          employee("wait-1", "waiting_human"),
          employee("err-1", "error"),
          employee("conf-1", "needs_configuration"),
          employee("queue-1", "queued"),
        ],
        dwellMs: 60_000,
      }),
    );

    expect(result.current.queue.map((item) => item.employeeId)).toEqual(["err-1", "wait-1", "work-1", "queue-1"]);
    expect(result.current.focusEmployeeId).toBe("err-1");
    expect(result.current.queueIndex).toBe(0);
    expect(result.current.isPaused).toBe(false);
  });

  it("weights longer-stuck employees ahead within the same urgency tier", async () => {
    const fresh = { ...employee("wait-fresh", "waiting_human"), statusSince: "2026-07-16T08:00:00Z" };
    const stale = { ...employee("wait-stale", "waiting_human"), statusSince: "2026-07-15T20:00:00Z" };
    const unknown = employee("wait-unknown", "waiting_human");
    const err = { ...employee("err-fresh", "error"), statusSince: "2026-07-16T09:00:00Z" };

    const { result } = await renderHook(() =>
      useRuntimeFocusCarousel({ employees: [fresh, unknown, stale, err], dwellMs: 60_000 }),
    );

    // 紧迫度仍是第一优先级（异常最前）；同级内等得越久越靠前，无时长信息垫底。
    expect(result.current.queue.map((item) => item.employeeId)).toEqual([
      "err-fresh",
      "wait-stale",
      "wait-fresh",
      "wait-unknown",
    ]);
  });

  it("keeps focus empty when no employee is active", async () => {
    const { result } = await renderHook(() =>
      useRuntimeFocusCarousel({ employees: [employee("idle-1", "idle")], dwellMs: 60_000 }),
    );

    expect(result.current.queue).toEqual([]);
    expect(result.current.focusEmployeeId).toBeUndefined();
  });

  it("advances focus through the queue after each dwell interval", async () => {
    const { result } = await renderHook(() =>
      useRuntimeFocusCarousel({
        employees: [employee("err-1", "error"), employee("work-1", "working")],
        dwellMs: 60,
      }),
    );

    expect(result.current.focusEmployeeId).toBe("err-1");
    await expect.poll(() => result.current.focusEmployeeId, { timeout: 2_000 }).toBe("work-1");
    await expect.poll(() => result.current.focusEmployeeId, { timeout: 2_000 }).toBe("err-1");
  });

  it("preempts focus when an employee status changes into an active state", async () => {
    const initial = [employee("work-1", "working"), employee("idle-1", "idle")];
    const { result, rerender } = await renderHook(
      (props?: { employees: RuntimeOverviewEmployee[] }) =>
        useRuntimeFocusCarousel({ employees: props?.employees ?? [], dwellMs: 60_000 }),
      { initialProps: { employees: initial } },
    );

    expect(result.current.focusEmployeeId).toBe("work-1");
    await rerender({ employees: [employee("work-1", "working"), employee("idle-1", "error")] });
    expect(result.current.focusEmployeeId).toBe("idle-1");
  });

  it("pauses on interaction without preempting and resumes automatically", async () => {
    const initial = [employee("err-1", "error"), employee("work-1", "working")];
    const { result, rerender } = await renderHook(
      (props?: { employees: RuntimeOverviewEmployee[] }) =>
        useRuntimeFocusCarousel({ employees: props?.employees ?? [], dwellMs: 80, resumeAfterMs: 240 }),
      { initialProps: { employees: initial } },
    );

    result.current.notifyInteraction();
    await expect.poll(() => result.current.isPaused).toBe(true);
    const pausedFocus = result.current.focusEmployeeId;

    // 暂停期间：不轮转，也不被状态变化插队。
    await rerender({ employees: [employee("err-1", "error"), employee("work-1", "waiting_human")] });
    await new Promise((resolve) => setTimeout(resolve, 160));
    expect(result.current.focusEmployeeId).toBe(pausedFocus);

    // 超时自动恢复轮播。
    await expect.poll(() => result.current.isPaused, { timeout: 2_000 }).toBe(false);
    await expect.poll(() => result.current.focusEmployeeId !== pausedFocus, { timeout: 2_000 }).toBe(true);
  });

  it("starts paused when opened with an explicit employee deep link", async () => {
    const { result } = await renderHook(() =>
      useRuntimeFocusCarousel({
        employees: [employee("err-1", "error")],
        initialInteracted: true,
        dwellMs: 60_000,
        resumeAfterMs: 120,
      }),
    );

    expect(result.current.isPaused).toBe(true);
    await expect.poll(() => result.current.isPaused, { timeout: 2_000 }).toBe(false);
  });

  it("falls back to the queue head when the focused employee leaves the queue", async () => {
    const { result, rerender } = await renderHook(
      (props?: { employees: RuntimeOverviewEmployee[] }) =>
        useRuntimeFocusCarousel({ employees: props?.employees ?? [], dwellMs: 60_000 }),
      { initialProps: { employees: [employee("err-1", "error"), employee("work-1", "working")] } },
    );

    expect(result.current.focusEmployeeId).toBe("err-1");
    await rerender({ employees: [employee("err-1", "idle"), employee("work-1", "working")] });
    expect(result.current.focusEmployeeId).toBe("work-1");
  });
});

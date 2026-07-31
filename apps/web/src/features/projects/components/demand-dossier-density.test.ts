import { describe, expect, it } from "vitest";

import {
  DOSSIER_DENSITY_STORAGE_KEY,
  inferDossierDensity,
  readStoredDossierDensity,
  resolveDossierDensity,
  writeStoredDossierDensity,
} from "./demand-dossier-density";

describe("一单卷宗密度推导", () => {
  it("有待决 → 驱动态", () => {
    expect(
      inferDossierDensity({
        active_task_count: 0,
        demand_terminal: true,
        has_open_decisions: true,
      }),
    ).toBe("drive");
  });

  it("有非终态任务 → 驱动态", () => {
    expect(
      inferDossierDensity({
        active_task_count: 2,
        demand_terminal: false,
        has_open_decisions: false,
      }),
    ).toBe("drive");
  });

  it("终态且无待办无活跃任务 → 巡检态", () => {
    expect(
      inferDossierDensity({
        active_task_count: 0,
        demand_terminal: true,
        has_open_decisions: false,
      }),
    ).toBe("inspect");
  });

  // 非终态但也没有活跃任务（例如规划中）仍算驱动：人还得盯着。
  it("非终态且无活跃任务仍是驱动态", () => {
    expect(
      inferDossierDensity({
        active_task_count: 0,
        demand_terminal: false,
        has_open_decisions: false,
      }),
    ).toBe("drive");
  });

  it("signals 缺失时保守取驱动态", () => {
    expect(inferDossierDensity(undefined)).toBe("drive");
  });
});

describe("一单卷宗密度偏好", () => {
  function memoryStorage(initial?: string) {
    const store = new Map<string, string>();
    if (initial) {
      store.set(DOSSIER_DENSITY_STORAGE_KEY, initial);
    }
    return {
      getItem: (key: string) => store.get(key) ?? null,
      setItem: (key: string, value: string) => void store.set(key, value),
      snapshot: () => store.get(DOSSIER_DENSITY_STORAGE_KEY),
    };
  }

  it("用户偏好盖过推导值", () => {
    const signals = {
      active_task_count: 3,
      demand_terminal: false,
      has_open_decisions: true,
    };
    expect(resolveDossierDensity(signals, "inspect")).toBe("inspect");
    expect(resolveDossierDensity(signals, undefined)).toBe("drive");
  });

  it("读写偏好走同一个键", () => {
    const storage = memoryStorage();
    writeStoredDossierDensity(storage, "inspect");
    expect(storage.snapshot()).toBe("inspect");
    expect(readStoredDossierDensity(storage)).toBe("inspect");
  });

  it("忽略非法存量值", () => {
    expect(readStoredDossierDensity(memoryStorage("compact"))).toBeUndefined();
  });

  // 隐私模式下 localStorage 会抛，密度只是呈现偏好，不该把整页拖崩。
  it("存储抛错时降级为无偏好", () => {
    const throwing = {
      getItem: () => {
        throw new Error("blocked");
      },
      setItem: () => {
        throw new Error("blocked");
      },
    };
    expect(readStoredDossierDensity(throwing)).toBeUndefined();
    expect(() => writeStoredDossierDensity(throwing, "drive")).not.toThrow();
  });

  it("无 storage（SSR）时安全返回", () => {
    expect(readStoredDossierDensity(undefined)).toBeUndefined();
    expect(() => writeStoredDossierDensity(undefined, "drive")).not.toThrow();
  });
});

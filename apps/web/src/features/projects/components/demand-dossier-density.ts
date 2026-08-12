import type { ProjectDemandDossier } from "@/lib/api/projects";

/**
 * 一单卷宗的呈现密度（历史时间线用）。
 *
 * 服务端只给 signals（有无待决 / 活跃任务数 / 需求是否终态），密度结论在前端算。
 * 不再提供「驱动/巡检」人工切换——旧偏好存储 API 保留兼容，resolve 时传入
 * undefined 即纯自动推导。
 *
 * - drive：还需要人跟进 → 时间线展开
 * - inspect：已收口 → 时间线折叠为最近若干条
 */
export type DossierDensity = "drive" | "inspect";

export const DOSSIER_DENSITY_STORAGE_KEY = "superteam.demand-dossier.density";

/** inspect 态时间线默认只显示这几条，其余折叠。 */
export const DOSSIER_INSPECT_TIMELINE_PREVIEW = 3;

export function inferDossierDensity(
  signals: ProjectDemandDossier["signals"] | undefined,
): DossierDensity {
  if (!signals) return "drive";
  if (signals.has_open_decisions) return "drive";
  if (signals.active_task_count > 0) return "drive";
  if (!signals.demand_terminal) return "drive";
  return "inspect";
}

export function isDossierDensity(value: unknown): value is DossierDensity {
  return value === "drive" || value === "inspect";
}

export function readStoredDossierDensity(
  storage: Storage | undefined,
): DossierDensity | undefined {
  if (!storage) return undefined;
  try {
    const raw = storage.getItem(DOSSIER_DENSITY_STORAGE_KEY);
    return isDossierDensity(raw) ? raw : undefined;
  } catch {
    // 隐私模式 / 禁用存储时读取会抛，密度退回推导值即可，不该让整页崩。
    return undefined;
  }
}

export function writeStoredDossierDensity(
  storage: Storage | undefined,
  density: DossierDensity,
): void {
  if (!storage) return;
  try {
    storage.setItem(DOSSIER_DENSITY_STORAGE_KEY, density);
  } catch {
    // 写失败同样吞掉：密度只是呈现偏好。
  }
}

export function resolveDossierDensity(
  signals: ProjectDemandDossier["signals"] | undefined,
  stored: DossierDensity | undefined,
): DossierDensity {
  return stored ?? inferDossierDensity(signals);
}

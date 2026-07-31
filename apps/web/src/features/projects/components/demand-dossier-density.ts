import type { ProjectDemandDossier } from "@/lib/api/projects";

/**
 * 一单卷宗的呈现密度（spec 2026-07-29 R2 §4.6）。
 *
 * 服务端只给 signals（有无待决 / 活跃任务数 / 需求是否终态），密度结论在前端算：
 * 密度是**用户偏好**而不是服务端事实，硬判在服务端必然要回头再补一层"用户覆盖"。
 * 用户手动切过之后，偏好记 localStorage 并盖过推导值。
 *
 * - drive（驱动）：还需要人跟进 → 时间线展开、右轨常显、待你处理置顶
 * - inspect（巡检）：已收口 → 时间线折叠为最近若干条，突出结论与交付判定
 */
export type DossierDensity = "drive" | "inspect";

export const DOSSIER_DENSITY_STORAGE_KEY = "superteam.demand-dossier.density";

/** inspect 态时间线默认只显示这几条，其余折叠。 */
export const DOSSIER_INSPECT_TIMELINE_PREVIEW = 3;

export function inferDossierDensity(
  signals: ProjectDemandDossier["signals"] | undefined,
): DossierDensity {
  if (!signals) {
    return "drive";
  }
  if (signals.has_open_decisions) {
    return "drive";
  }
  if (signals.active_task_count > 0) {
    return "drive";
  }
  return signals.demand_terminal ? "inspect" : "drive";
}

export function isDossierDensity(value: unknown): value is DossierDensity {
  return value === "drive" || value === "inspect";
}

/**
 * 读用户偏好。刻意用全局键而不是 per-demand：人对"我想看多细"的偏好是稳定的，
 * 逐单记会让同一个人在每条需求上重新调一遍。
 */
export function readStoredDossierDensity(
  storage: Pick<Storage, "getItem"> | undefined,
): DossierDensity | undefined {
  if (!storage) {
    return undefined;
  }
  try {
    const raw = storage.getItem(DOSSIER_DENSITY_STORAGE_KEY);
    return isDossierDensity(raw) ? raw : undefined;
  } catch {
    // 隐私模式 / 禁用存储时读取会抛，密度退回推导值即可，不该让整页崩。
    return undefined;
  }
}

export function writeStoredDossierDensity(
  storage: Pick<Storage, "setItem"> | undefined,
  density: DossierDensity,
): void {
  if (!storage) {
    return;
  }
  try {
    storage.setItem(DOSSIER_DENSITY_STORAGE_KEY, density);
  } catch {
    // 同上：写不进去只是偏好不持久，不影响本次呈现。
  }
}

export function resolveDossierDensity(
  signals: ProjectDemandDossier["signals"] | undefined,
  stored: DossierDensity | undefined,
): DossierDensity {
  return stored ?? inferDossierDensity(signals);
}

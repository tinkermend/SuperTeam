/**
 * 项目生命周期阶段色：单一事实源。
 *
 * 阶段轴（draft → configuring → running → acceptance → paused → archived）
 * 不是紧迫度轴；颜色走 `--phase-*` 分类 token，不占用 ok/warn/danger 语义预算。
 * 文案仍走 `lib/status-labels.ts` 的 `projectStatusLabel`。
 *
 * @see docs/superpowers/specs/2026-08-11-projects-home-status-color-and-drilldown-remediation.md §5
 */

export type ProjectLifecyclePhase =
  | "draft"
  | "configuring"
  | "running"
  | "acceptance"
  | "paused"
  | "archived";

const PHASE_DOT_CLASS: Record<ProjectLifecyclePhase, string> = {
  running: "bg-phase-ready",
  acceptance: "bg-phase-acceptance",
  configuring: "bg-phase-configuring",
  draft: "bg-phase-draft",
  paused: "bg-phase-paused",
  archived: "bg-phase-archived",
};

const PHASE_COLOR_VAR: Record<ProjectLifecyclePhase, string> = {
  running: "var(--phase-ready)",
  acceptance: "var(--phase-acceptance)",
  configuring: "var(--phase-configuring)",
  draft: "var(--phase-draft)",
  paused: "var(--phase-paused)",
  archived: "var(--phase-archived)",
};

function asPhase(status: string): ProjectLifecyclePhase | undefined {
  if (
    status === "draft" ||
    status === "configuring" ||
    status === "running" ||
    status === "acceptance" ||
    status === "paused" ||
    status === "archived"
  ) {
    return status;
  }
  return undefined;
}

/** Tailwind 圆点 class：`bg-phase-*`。未知状态回退 draft 灰点。 */
export function projectPhaseDotClass(status: string): string {
  return PHASE_DOT_CLASS[asPhase(status) ?? "draft"];
}

/** CSS 变量：环图 conic-gradient 等内联 style 用。未知状态回退 draft。 */
export function projectPhaseColorVar(status: string): string {
  return PHASE_COLOR_VAR[asPhase(status) ?? "draft"];
}

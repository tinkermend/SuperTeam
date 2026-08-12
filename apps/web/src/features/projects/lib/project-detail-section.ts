/** 项目详情顶栏区段（任务表默认；卷宗左轨始终可见）。 */
export type ProjectDetailSection =
  | "tasks"
  | "flow"
  | "approval"
  | "history"
  | "assets";

/** 兼容旧 ?tab= 深链。`demands` 已退役，落到任务表并保留 ?demand=。 */
export function normalizeProjectDetailSection(
  tab: string | undefined,
): ProjectDetailSection {
  switch (tab) {
    case "flow":
      return "flow";
    case "approval":
      return "approval";
    case "history":
    // 执行轨迹深链：落在历史页签，由 ProjectOperationalDetail 展开高级事实区定位。
    case "trace":
      return "history";
    case "assets":
    case "artifacts":
    case "budget":
    case "acceptance":
    case "closure":
      return "assets";
    case "demands":
    case "tasks":
    case "overview":
    case "config":
    case "workbench":
    default:
      return "tasks";
  }
}

export function assetsInitialTabFromQuery(
  tab: string | undefined,
): "artifacts" | "budget" | "acceptance" {
  if (tab === "budget") return "budget";
  if (tab === "acceptance" || tab === "closure") return "acceptance";
  return "artifacts";
}

export function isProjectDetailSectionQuery(value: string | undefined): boolean {
  return (
    value === "workbench" ||
    value === "overview" ||
    value === "tasks" ||
    value === "artifacts" ||
    value === "approval" ||
    value === "budget" ||
    value === "acceptance" ||
    value === "closure" ||
    value === "config" ||
    value === "assets" ||
    value === "trace" ||
    value === "demands" ||
    value === "flow" ||
    value === "history"
  );
}

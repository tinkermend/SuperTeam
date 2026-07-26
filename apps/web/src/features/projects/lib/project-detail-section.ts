/** 项目详情顶栏区段（工作台默认，无「概览」对等 Tab）。 */
export type ProjectDetailSection =
  | "workbench"
  | "tasks"
  | "approval"
  | "assets";

/** 兼容旧 ?tab= 深链。 */
export function normalizeProjectDetailSection(
  tab: string | undefined,
): ProjectDetailSection {
  switch (tab) {
    case "tasks":
      return "tasks";
    case "approval":
      return "approval";
    case "assets":
    case "artifacts":
    case "budget":
    case "acceptance":
    case "closure":
      return "assets";
    case "overview":
    case "config":
    case "workbench":
    // 执行轨迹深链：落在工作台，由 ProjectOperationalDetail 展开高级事实区定位。
    case "trace":
    default:
      return "workbench";
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
    value === "trace"
  );
}

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
    case "artifacts":
    case "budget":
    case "acceptance":
      return "assets";
    case "overview":
    case "config":
    case "workbench":
    default:
      return "workbench";
  }
}

export function assetsInitialTabFromQuery(
  tab: string | undefined,
): "artifacts" | "budget" | "acceptance" {
  if (tab === "budget") return "budget";
  if (tab === "acceptance") return "acceptance";
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
    value === "config" ||
    value === "assets"
  );
}

export const RESERVED_CAPABILITY_KEYS = [
  "external_capabilities",
  "environment_variable_refs",
  "skills",
  "mcp_servers",
];

export function stringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value.filter((item): item is string => typeof item === "string");
}

export function sameStringArray(a: string[], b: string[]) {
  return a.length === b.length && a.every((value, index) => value === b[index]);
}

export function sameStringSet(a: string[], b: string[]) {
  if (a.length !== b.length) return false;
  const other = new Set(b);
  return a.every((value) => other.has(value));
}

export function budgetPolicyValue(value: Record<string, unknown>) {
  const rawValue = value.daily_token_limit;
  if (typeof rawValue === "number" && Number.isInteger(rawValue) && rawValue > 0) {
    return String(rawValue);
  }
  if (typeof rawValue === "string") {
    const trimmed = rawValue.trim();
    if (trimmed) {
      return trimmed;
    }
  }
  return "";
}

export function budgetPolicyFromDailyTokenLimit(dailyTokenLimit: string) {
  const trimmed = dailyTokenLimit.trim();
  if (!trimmed) return {};

  const parsed = Number(trimmed);
  if (!Number.isInteger(parsed) || parsed <= 0) {
    return undefined;
  }

  return { daily_token_limit: parsed };
}

export function submitPermissionErrorMessage(error: unknown) {
  const message = error instanceof Error ? error.message : "";
  const detail = error instanceof Error && "detail" in error ? String((error as { detail?: string }).detail ?? "") : "";
  if (message.includes("active work")) {
    return "员工当前有进行中的工作，无法提交权限变更。";
  }
  if (message.includes("not configured")) {
    return "权限审批链路未配置，请联系管理员。";
  }
  if (message.includes("team")) {
    return "员工需先归属团队才能提交权限变更。";
  }
  const extra = detail && !message.includes(detail) ? `（${detail}）` : "";
  return `提交失败：${message || "未知错误"}${extra}`;
}

export type EmployeeConfigTab = "identity" | "capabilities" | "execution" | "permission";

export const EMPLOYEE_CONFIG_TABS: EmployeeConfigTab[] = [
  "identity",
  "capabilities",
  "execution",
  "permission",
];

export function parseEmployeeConfigTab(value: unknown): EmployeeConfigTab {
  if (
    value === "identity" ||
    value === "capabilities" ||
    value === "execution" ||
    value === "permission"
  ) {
    return value;
  }
  return "identity";
}

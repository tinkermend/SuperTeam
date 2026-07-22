import type { ApiClientOptions } from "./client";
import {
  deleteJson,
  getJson,
  patchJson,
  postJson,
  postJsonWithoutBody,
} from "./client";

export type AutomationCoordinationMode = "plan" | "loop" | "chat";
export type AutomationScheduleKind = "cron" | "interval";
export type AutomationFireStatus =
  | "pending"
  | "succeeded"
  | "failed"
  | "skipped_overlap"
  | "skipped_disabled";
export type AutomationDisabledReason =
  | "actor_removed_from_project"
  | "actor_deactivated"
  | "consecutive_fire_failures"
  | "user_disabled";

export type AutomationRule = {
  id: string;
  tenant_id: string;
  team_id: string;
  project_id: string;
  project_name?: string;
  name: string;
  enabled: boolean;
  coordination_mode: AutomationCoordinationMode;
  demand_title_template?: string | null;
  demand_body_template?: string | null;
  scenario_template_key?: string | null;
  digital_employee_id?: string | null;
  digital_employee_name?: string | null;
  chat_objective_template?: string | null;
  schedule_kind: AutomationScheduleKind;
  cron_expr?: string | null;
  interval_seconds?: number | null;
  timezone: string;
  overlap_policy: string;
  actor_user_id: string;
  actor_display_name?: string;
  disabled_reason?: AutomationDisabledReason | string | null;
  consecutive_failure_count: number;
  created_at: string;
  updated_at: string;
  latest_fire?: AutomationFire | null;
};

export type AutomationFire = {
  id: string;
  tenant_id: string;
  rule_id: string;
  scheduled_fire_at: string;
  idempotency_key: string;
  status: AutomationFireStatus | string;
  demand_id?: string | null;
  run_id?: string | null;
  error_code?: string | null;
  error_message?: string | null;
  created_at: string;
};

export type AutomationRuleListResponse = {
  items: AutomationRule[];
};

export type AutomationFireListResponse = {
  items: AutomationFire[];
};

export type CreateAutomationRuleInput = {
  name: string;
  project_id: string;
  coordination_mode: AutomationCoordinationMode;
  demand_title_template?: string;
  demand_body_template?: string;
  scenario_template_key?: string;
  digital_employee_id?: string;
  chat_objective_template?: string;
  schedule_kind: AutomationScheduleKind;
  cron_expr?: string;
  interval_seconds?: number;
  timezone?: string;
  enabled?: boolean;
};

export type UpdateAutomationRuleInput = {
  name?: string;
  demand_title_template?: string | null;
  demand_body_template?: string | null;
  scenario_template_key?: string | null;
  digital_employee_id?: string | null;
  chat_objective_template?: string | null;
  schedule_kind?: AutomationScheduleKind;
  cron_expr?: string | null;
  interval_seconds?: number | null;
  timezone?: string;
};

function encodeRuleId(ruleId: string): string {
  return encodeURIComponent(ruleId);
}

export function listAutomationRules(
  options: ApiClientOptions,
  params?: { project_id?: string; enabled?: boolean },
): Promise<AutomationRuleListResponse> {
  const search = new URLSearchParams();
  if (params?.project_id) search.set("project_id", params.project_id);
  if (params?.enabled !== undefined) search.set("enabled", String(params.enabled));
  const qs = search.toString();
  return getJson(
    options,
    `/api/v1/automations${qs ? `?${qs}` : ""}`,
    "automation rules",
  );
}

export function getAutomationRule(
  options: ApiClientOptions,
  ruleId: string,
): Promise<AutomationRule> {
  const encodedRuleId = encodeRuleId(ruleId);
  return getJson(
    options,
    `/api/v1/automations/${encodedRuleId}`,
    "automation rule",
  );
}

export function createAutomationRule(
  options: ApiClientOptions,
  input: CreateAutomationRuleInput,
): Promise<AutomationRule> {
  return postJson(options, "/api/v1/automations", input, "automation rule");
}

export function updateAutomationRule(
  options: ApiClientOptions,
  ruleId: string,
  input: UpdateAutomationRuleInput,
): Promise<AutomationRule> {
  const encodedRuleId = encodeRuleId(ruleId);
  return patchJson(
    options,
    `/api/v1/automations/${encodedRuleId}`,
    input,
    "automation rule",
  );
}

export function deleteAutomationRule(
  options: ApiClientOptions,
  ruleId: string,
): Promise<void> {
  const encodedRuleId = encodeRuleId(ruleId);
  return deleteJson(
    options,
    `/api/v1/automations/${encodedRuleId}`,
    "automation rule",
  );
}

export function enableAutomationRule(
  options: ApiClientOptions,
  ruleId: string,
): Promise<AutomationRule> {
  const encodedRuleId = encodeRuleId(ruleId);
  return postJsonWithoutBody(
    options,
    `/api/v1/automations/${encodedRuleId}/enable`,
    "automation rule",
  );
}

export function disableAutomationRule(
  options: ApiClientOptions,
  ruleId: string,
): Promise<AutomationRule> {
  const encodedRuleId = encodeRuleId(ruleId);
  return postJsonWithoutBody(
    options,
    `/api/v1/automations/${encodedRuleId}/disable`,
    "automation rule",
  );
}

export function triggerAutomationRule(
  options: ApiClientOptions,
  ruleId: string,
): Promise<AutomationFire> {
  const encodedRuleId = encodeRuleId(ruleId);
  return postJsonWithoutBody(
    options,
    `/api/v1/automations/${encodedRuleId}/trigger`,
    "automation fire",
  );
}

export function listAutomationFires(
  options: ApiClientOptions,
  ruleId: string,
): Promise<AutomationFireListResponse> {
  const encodedRuleId = encodeRuleId(ruleId);
  return getJson(
    options,
    `/api/v1/automations/${encodedRuleId}/fires`,
    "automation fires",
  );
}

export function formatAutomationScheduleSummary(rule: {
  schedule_kind: AutomationScheduleKind;
  cron_expr?: string | null;
  interval_seconds?: number | null;
  timezone?: string;
}): string {
  if (rule.schedule_kind === "interval") {
    const seconds = rule.interval_seconds ?? 0;
    if (seconds >= 86400 && seconds % 86400 === 0) {
      return `每 ${seconds / 86400} 天`;
    }
    if (seconds >= 3600 && seconds % 3600 === 0) {
      return `每 ${seconds / 3600} 小时`;
    }
    if (seconds >= 60 && seconds % 60 === 0) {
      return `每 ${seconds / 60} 分钟`;
    }
    return `每 ${seconds} 秒`;
  }
  const cron = (rule.cron_expr ?? "").trim();
  if (cron === "0 9 * * *") return "每天 09:00";
  if (cron === "0 9 * * 1") return "每周一 09:00";
  if (cron === "0 9 * * 1-5") return "工作日 09:00";
  return cron || "未设置";
}

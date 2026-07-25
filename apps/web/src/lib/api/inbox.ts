import type { ApiClientOptions } from "./client";
import { getJson, postJson } from "./client";

export type InboxViewMode = "mine" | "team";

export type InboxStatus = "open" | "resolved" | "cancelled";

export type InboxItemType =
  | "approval"
  | "project_decision"
  | "team_pending_delete"
  | "digital_employee_run_recovery";

export type InboxSourceType =
  | "approval_request"
  | "project_decision_request"
  | "team_pending_delete"
  | "digital_employee_run";

export type InboxItemAction = {
  key: string;
  label: string;
  tone: string;
  requires_comment: boolean;
  metadata?: Record<string, unknown>;
};

export type InboxAction = InboxItemAction;

/** HumanTask 闭环进度（OpenAPI HumanTaskProgress）。 */
export type HumanTaskProgress = {
  step: number;
  total: number;
  label: string;
};

/** HumanTask 证据条目（OpenAPI HumanTaskEvidenceItem）。 */
export type HumanTaskEvidenceItem = {
  id?: string;
  statement?: string;
  title?: string;
  status?: string;
  verification_method?: string;
  verdict?: string;
  summary?: string;
  conclusion?: string;
  [key: string]: unknown;
};

/**
 * 人类待办读权威（OpenAPI HumanTask）。
 * InboxItem 嵌入本类型；inbox_items 仅为存储实现，非第二真相。
 */
export type HumanTask = {
  kind?: string;
  layer?: "task" | "demand" | "project" | string;
  why?: string;
  evidence?: HumanTaskEvidenceItem[];
  progress?: HumanTaskProgress;
  /** 唯一权威落点 URL，服务端唯一来源。 */
  primary_surface?: string;
};

export type InboxItem = HumanTask & {
  id: string;
  tenant_id: string;
  team_id?: string;
  target_user_id: string;
  item_type: InboxItemType;
  source_type: InboxSourceType;
  source_id: string;
  source_project_id?: string;
  source_task_id?: string;
  source_approval_request_id?: string;
  source_project_name?: string;
  source_task_name?: string;
  title: string;
  summary?: string;
  status: InboxStatus;
  risk_level?: string;
  priority?: string;
  actions: InboxItemAction[];
  /** 原始快照；why/evidence/progress/primary_surface 已提升为具名字段。 */
  context: Record<string, unknown>;
  deep_link: Record<string, unknown>;
  last_activity_at: string;
  created_at: string;
  updated_at: string;
  resolved_at?: string;
};

export type InboxListFilters = {
  view?: InboxViewMode;
  status?: InboxStatus;
  item_type?: InboxItemType;
  risk_level?: string;
  project_id?: string;
  target_user_id?: string;
  limit?: number;
  offset?: number;
};

export type InboxListPagination = {
  limit: number;
  offset: number;
  has_more: boolean;
};

export type InboxListSummary = {
  open_count: number;
  high_risk_count: number;
  blocked_count: number;
};

export type InboxListResponse = {
  items: InboxItem[];
  pagination: InboxListPagination;
  summary: InboxListSummary;
};

export type InboxBadge = {
  mine_open_count: number;
  team_open_count: number;
  high_risk_count: number;
};

export type ExecuteInboxActionInput = {
  action: string;
  comment?: string;
  payload?: Record<string, unknown>;
};

export type InboxSourceActionResult = {
  source_type: string;
  source_id: string;
  status: string;
};

export type ExecuteInboxActionResponse = {
  item: InboxItem;
  source_result: InboxSourceActionResult;
};

function inboxItemsPath(filters: InboxListFilters): string {
  const params = new URLSearchParams();

  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined) {
      params.set(key, String(value));
    }
  }

  const query = params.toString();
  return `/api/v1/inbox/items${query ? `?${query}` : ""}`;
}

export function listInboxItems(
  options: ApiClientOptions,
  filters: InboxListFilters = {},
): Promise<InboxListResponse> {
  return getJson<InboxListResponse>(options, inboxItemsPath(filters), "inbox items");
}

export function getInboxBadge(options: ApiClientOptions): Promise<InboxBadge> {
  return getJson<InboxBadge>(options, "/api/v1/inbox/badge", "inbox badge");
}

export function executeInboxAction(
  options: ApiClientOptions,
  itemId: string,
  input: ExecuteInboxActionInput,
): Promise<ExecuteInboxActionResponse> {
  return postJson<ExecuteInboxActionResponse>(
    options,
    `/api/v1/inbox/items/${itemId}/actions`,
    {
      action: input.action,
      comment: input.comment ?? "",
      payload: input.payload ?? {},
    },
    "inbox action",
  );
}

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

export type InboxItem = {
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
  // 规范化 HumanTask 分类(§4.1/§4.2):kind 如 dispatch_release/acceptance_sign/…,
  // layer 为 task/demand/project。服务端附加读模型元数据,用于分组与命名。
  kind?: string;
  layer?: string;
  /** §4.1 why：一句话说明为什么需要你（服务端中文）。 */
  why?: string;
  /** §4.1 evidence：判据/结论/交付物摘录。 */
  evidence?: Array<Record<string, unknown>>;
  /** §4.1/§6.1 闭环进度条数据。 */
  progress?: {
    step: number;
    total: number;
    label: string;
  };
  risk_level?: string;
  priority?: string;
  actions: InboxItemAction[];
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

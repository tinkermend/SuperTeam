import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type InboxViewMode = "mine" | "team";

export type InboxStatus = "open" | "resolved" | "cancelled";

export type InboxItemType = "approval" | "project_decision";

export type InboxSourceType = "approval_request" | "project_decision_request";

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
  title: string;
  summary?: string;
  status: InboxStatus;
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

function buildInboxItemsUrl(baseUrl: string, filters: InboxListFilters): string {
  const params = new URLSearchParams();

  for (const [key, value] of Object.entries(filters)) {
    if (value !== undefined) {
      params.set(key, String(value));
    }
  }

  const query = params.toString();
  return buildApiUrl(baseUrl, `/api/v1/inbox/items${query ? `?${query}` : ""}`);
}

export async function listInboxItems(
  options: ApiClientOptions,
  filters: InboxListFilters = {},
): Promise<InboxListResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildInboxItemsUrl(options.baseUrl, filters), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<InboxListResponse>(response, "inbox items");
}

export async function getInboxBadge(options: ApiClientOptions): Promise<InboxBadge> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, "/api/v1/inbox/badge"), {
    credentials: "include",
    headers: {
      accept: "application/json",
    },
    method: "GET",
  });

  return parseJson<InboxBadge>(response, "inbox badge");
}

export async function executeInboxAction(
  options: ApiClientOptions,
  itemId: string,
  input: ExecuteInboxActionInput,
): Promise<ExecuteInboxActionResponse> {
  const fetcher = options.fetcher ?? fetch;
  const response = await fetcher(buildApiUrl(options.baseUrl, `/api/v1/inbox/items/${itemId}/actions`), {
    body: JSON.stringify({
      action: input.action,
      comment: input.comment ?? "",
      payload: input.payload ?? {},
    }),
    credentials: "include",
    headers: {
      accept: "application/json",
      "content-type": "application/json",
    },
    method: "POST",
  });

  return parseJson<ExecuteInboxActionResponse>(response, "inbox action");
}

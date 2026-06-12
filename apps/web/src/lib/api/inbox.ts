import type { ApiClientOptions } from "./client";
import { buildApiUrl, parseJson } from "./client";

export type InboxViewMode = "mine" | "team";

export type InboxStatus = "open" | "resolved" | "cancelled";

export type InboxAction = {
  key: string;
  label: string;
  style: string;
  requires_comment: boolean;
  comment_placeholder?: string;
  confirm_label?: string;
  payload_schema?: Record<string, unknown>;
};

export type InboxItem = {
  id: string;
  tenant_id: string;
  item_type: string;
  source_type: string;
  source_id: string;
  source_project_id?: string;
  source_project_task_id?: string;
  source_approval_request_id?: string;
  target_user_id: string;
  title: string;
  summary: string;
  status: InboxStatus;
  risk_level: string;
  actions: InboxAction[];
  context: Record<string, unknown>;
  last_activity_at: string;
  created_at: string;
  updated_at: string;
};

export type InboxListFilters = {
  view?: InboxViewMode;
  status?: InboxStatus;
  item_type?: string;
  risk_level?: string;
  source_project_id?: string;
  limit?: number;
  offset?: number;
};

export type InboxListResponse = {
  items: InboxItem[];
  total_count: number;
  has_more: boolean;
};

export type InboxBadge = {
  mine_open_count: number;
};

export type ExecuteInboxActionInput = {
  action: string;
  comment?: string;
  payload?: Record<string, unknown>;
};

export type ExecuteInboxActionResponse = {
  item: InboxItem;
  source_result: Record<string, unknown>;
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

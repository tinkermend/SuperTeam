import { describe, expect, it, vi } from "vitest";
import type { InboxBadge, InboxItem, InboxListResponse } from "./inbox";
import { executeInboxAction, getInboxBadge, listInboxItems } from "./inbox";

const inboxItem: InboxItem = {
  id: "item-1",
  tenant_id: "tenant-1",
  item_type: "approval",
  source_type: "approval_request",
  source_id: "source-1",
  source_project_id: "project-1",
  source_approval_request_id: "approval-1",
  target_user_id: "user-1",
  title: "审批上线窗口",
  summary: "需要确认发布窗口。",
  status: "open",
  risk_level: "high",
  actions: [
    {
      key: "approve",
      label: "批准",
      tone: "positive",
      requires_comment: false,
      metadata: {
        source: "approval",
      },
    },
  ],
  context: {
    project_title: "客户交付闭环",
  },
  deep_link: {
    route: "/projects/project-1",
    anchor: "approval-1",
  },
  team_id: "team-1",
  source_task_id: "task-1",
  priority: "high",
  last_activity_at: "2026-06-12T02:00:00Z",
  created_at: "2026-06-12T01:00:00Z",
  updated_at: "2026-06-12T02:00:00Z",
};

describe("listInboxItems", () => {
  it("calls the inbox items endpoint with filters and parses JSON", async () => {
    const responseBody: InboxListResponse = {
      items: [inboxItem],
      pagination: {
        limit: 50,
        offset: 0,
        has_more: false,
      },
      summary: {
        open_count: 1,
        high_risk_count: 1,
        blocked_count: 0,
      },
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(responseBody), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(
      listInboxItems(
        {
          baseUrl: "http://api.test",
          fetcher,
        },
        {
          view: "mine",
          status: "open",
          project_id: "project-1",
          limit: 50,
        },
      ),
    ).resolves.toEqual(responseBody);

    expect(fetcher).toHaveBeenCalledWith("http://api.test/api/v1/inbox/items?view=mine&status=open&project_id=project-1&limit=50", {
      credentials: "include",
      headers: {
        accept: "application/json",
      },
      method: "GET",
    });
  });

  it("keeps zero-valued pagination filters in the query string", async () => {
    const responseBody: InboxListResponse = {
      items: [],
      pagination: {
        limit: 0,
        offset: 0,
        has_more: false,
      },
      summary: {
        open_count: 0,
        high_risk_count: 0,
        blocked_count: 0,
      },
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(responseBody), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(
      listInboxItems(
        {
          baseUrl: "http://api.test",
          fetcher,
        },
        {
          offset: 0,
        },
      ),
    ).resolves.toEqual(responseBody);

    expect(fetcher).toHaveBeenCalledWith("http://api.test/api/v1/inbox/items?offset=0", {
      credentials: "include",
      headers: {
        accept: "application/json",
      },
      method: "GET",
    });
  });
});

describe("getInboxBadge", () => {
  it("calls the inbox badge endpoint and parses JSON", async () => {
    const responseBody: InboxBadge = {
      mine_open_count: 7,
      team_open_count: 12,
      high_risk_count: 3,
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(responseBody), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(
      getInboxBadge({
        baseUrl: "http://api.test",
        fetcher,
      }),
    ).resolves.toEqual(responseBody);

    expect(fetcher).toHaveBeenCalledWith("http://api.test/api/v1/inbox/badge", {
      credentials: "include",
      headers: {
        accept: "application/json",
      },
      method: "GET",
    });
  });
});

describe("executeInboxAction", () => {
  it("posts the action payload and parses the source result", async () => {
    const responseBody = {
      item: {
        ...inboxItem,
        status: "resolved" as const,
      },
      source_result: {
        source_type: "approval_request",
        source_id: "approval-1",
        status: "approved",
      },
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(responseBody), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await expect(
      executeInboxAction(
        {
          baseUrl: "http://api.test",
          fetcher,
        },
        "item-1",
        {
          action: "approve",
          comment: "同意执行",
          payload: {
            approved: true,
          },
        },
      ),
    ).resolves.toEqual(responseBody);

    expect(fetcher).toHaveBeenCalledWith("http://api.test/api/v1/inbox/items/item-1/actions", {
      body: JSON.stringify({
        action: "approve",
        comment: "同意执行",
        payload: {
          approved: true,
        },
      }),
      credentials: "include",
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "POST",
    });
  });

  it("defaults missing action comment and payload", async () => {
    const responseBody = {
      item: inboxItem,
      source_result: {
        source_type: "approval_request",
        source_id: "approval-1",
        status: "resolved",
      },
    };
    const fetcher = vi.fn(async () =>
      new Response(JSON.stringify(responseBody), {
        status: 200,
        headers: {
          "content-type": "application/json",
        },
      }),
    );

    await executeInboxAction(
      {
        baseUrl: "http://api.test",
        fetcher,
      },
      "item-1",
      {
        action: "resolve",
      },
    );

    expect(fetcher).toHaveBeenCalledWith("http://api.test/api/v1/inbox/items/item-1/actions", {
      body: JSON.stringify({
        action: "resolve",
        comment: "",
        payload: {},
      }),
      credentials: "include",
      headers: {
        accept: "application/json",
        "content-type": "application/json",
      },
      method: "POST",
    });
  });
});

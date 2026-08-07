import { describe, expect, it } from "vitest";
import { formatContext } from "./components/inbox-item-list";
import type { InboxItem } from "@/lib/api/inbox";

function baseItem(overrides: Partial<InboxItem> = {}): InboxItem {
  return {
    id: "item-1",
    tenant_id: "tenant-1",
    target_user_id: "user-1",
    item_type: "project_decision",
    source_type: "project_decision_request",
    source_id: "25a6b54b-1111-2222-3333-444444444444",
    title: "确认计划",
    status: "open",
    actions: [],
    context: {},
    deep_link: {},
    created_at: "2026-08-07T00:00:00Z",
    updated_at: "2026-08-07T00:00:00Z",
    last_activity_at: "2026-08-07T00:00:00Z",
    ...overrides,
  };
}

describe("formatContext D3", () => {
  it("does not put full project UUID in the primary meta string when name is missing", () => {
    const fullId = "25a6b54b-1111-2222-3333-444444444444";
    const label = formatContext(
      baseItem({
        source_project_id: fullId,
        source_project_name: undefined,
        context: {},
      }),
    );
    expect(label).toBe("未命名项目 (25a6b54b…)");
    expect(label).not.toContain(fullId);
  });

  it("prefers project name when present", () => {
    expect(
      formatContext(
        baseItem({
          source_project_id: "25a6b54b-1111-2222-3333-444444444444",
          source_project_name: "上线项目",
        }),
      ),
    ).toBe("上线项目");
  });
});

import { describe, expect, it } from "vitest";
import {
  resolvedActionDisabledMessage,
  resolvedTimelineDescription,
  resolvedTimelineTitle
} from "@/features/inbox/components/inbox-shell";
import type { InboxItem } from "@/lib/api/inbox";

function baseItem(overrides: Partial<InboxItem> = {}): InboxItem {
  return {
    id: "item-1",
    tenant_id: "tenant-1",
    target_user_id: "user-1",
    item_type: "project_decision",
    source_type: "project_decision_request",
    source_id: "decision-1",
    title: "确认计划",
    status: "resolved",
    actions: [],
    context: {},
    deep_link: {},
    created_at: "2026-08-03T00:00:00Z",
    updated_at: "2026-08-03T01:00:00Z",
    last_activity_at: "2026-08-03T01:00:00Z",
    resolved_at: "2026-08-03T01:00:00Z",
    ...overrides
  };
}

describe("inbox terminal resolution snapshot copy", () => {
  it("titles with decision verb when resolution is present", () => {
    const item = baseItem({
      context: {
        resolution: {
          decision_label: "批准",
          resolved_by_name: "开发管理员",
          channel_label: "飞书"
        }
      }
    });
    expect(resolvedTimelineTitle(item)).toBe("已批准");
    expect(resolvedTimelineDescription(item)).toBe("开发管理员 经 飞书 已批准。");
    expect(resolvedActionDisabledMessage(item)).toBe(
      "已由 开发管理员 经 飞书 已批准，无需再操作。"
    );
  });

  it("includes comment in timeline description", () => {
    const item = baseItem({
      context: {
        resolution: {
          decision_label: "驳回",
          resolved_by_name: "张三",
          channel_label: "Console 收件箱",
          comment: "证据不足"
        }
      }
    });
    expect(resolvedTimelineTitle(item)).toBe("已驳回");
    expect(resolvedTimelineDescription(item)).toContain("张三 经 Console 收件箱 已驳回。");
    expect(resolvedTimelineDescription(item)).toContain("备注：证据不足");
  });

  it("falls back gracefully without resolution snapshot", () => {
    const item = baseItem({ context: {} });
    expect(resolvedTimelineTitle(item)).toBe("已处理");
    expect(resolvedTimelineDescription(item)).toBe("该事项已完成处理，无需再操作。");
    expect(resolvedActionDisabledMessage(item)).toBe("该事项已处理完毕，无需再操作。");
  });

  it("handles cancelled status", () => {
    const item = baseItem({ status: "cancelled", context: {} });
    expect(resolvedTimelineTitle(item)).toBe("已取消");
    expect(resolvedTimelineDescription(item)).toBe("该事项已取消，无需再处理。");
  });
});

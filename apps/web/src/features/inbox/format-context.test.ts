import { describe, expect, it } from "vitest";
import {
  formatContext,
  formatRealCurrentNode,
  inboxListDescriptionParagraphs,
  primaryDemandLabel,
  readDemandRefs,
} from "./components/inbox-item-list";
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

  it("does not put full demand UUID in meta when demand_title is missing", () => {
    const fullId = "a1b2c3d4-1111-2222-3333-444444444444";
    const label = formatContext(
      baseItem({
        context: { demand_id: fullId },
      }),
    );
    expect(label).toBe("未命名需求 (a1b2c3d4…)");
    expect(label).not.toContain(fullId);
  });

  it("prefers demand_title when present", () => {
    expect(
      formatContext(
        baseItem({
          context: {
            demand_id: "a1b2c3d4-1111-2222-3333-444444444444",
            demand_title: "Runtime 接入验收",
          },
        }),
      ),
    ).toBe("Runtime 接入验收");
  });

  it("falls back when demands[].title is empty or equals id", () => {
    const fullId = "b2c3d4e5-1111-2222-3333-444444444444";
    const refs = readDemandRefs(
      baseItem({
        context: {
          demands: [{ id: fullId, title: fullId }],
        },
      }),
    );
    expect(refs).toHaveLength(1);
    expect(refs[0].title).toBe("未命名需求 (b2c3d4e5…)");
    expect(primaryDemandLabel(
      baseItem({ context: { demands: [{ id: fullId, title: fullId }] } }),
    )).not.toContain(fullId);
  });
});

describe("formatRealCurrentNode", () => {
  it("returns undefined when no real node keys exist", () => {
    expect(
      formatRealCurrentNode(
        baseItem({ kind: "plan_review", context: { kind: "plan_review" } }),
      ),
    ).toBeUndefined();
  });

  it("returns real node when present", () => {
    expect(
      formatRealCurrentNode(
        baseItem({ context: { current_node: "计划确认节点" } }),
      ),
    ).toBe("计划确认节点");
  });
});

describe("inboxListDescriptionParagraphs (§3.4 why/summary 去重)", () => {
  it("renders a single paragraph when why equals summary after trim", () => {
    const text =
      "系统补建人工决策卡：任务已停在待人工确认，但缺少可处理的决策（原因：需要恢复 Runtime）";
    expect(
      inboxListDescriptionParagraphs(
        baseItem({ summary: text, why: `  ${text}  ` }),
      ),
    ).toEqual([text]);
  });

  it("renders both paragraphs when why differs from summary", () => {
    expect(
      inboxListDescriptionParagraphs(
        baseItem({
          summary: "计划版本需要你确认后才能开始执行",
          why: "需求要求在 README 中添加项目简介",
        }),
      ),
    ).toEqual([
      "计划版本需要你确认后才能开始执行",
      "需求要求在 README 中添加项目简介",
    ]);
  });

  it("renders only why when summary is empty", () => {
    expect(
      inboxListDescriptionParagraphs(
        baseItem({ summary: undefined, why: "仅 why 有值" }),
      ),
    ).toEqual(["仅 why 有值"]);
  });

  it("returns empty when both are blank", () => {
    expect(
      inboxListDescriptionParagraphs(baseItem({ summary: "  ", why: "" })),
    ).toEqual([]);
  });
});

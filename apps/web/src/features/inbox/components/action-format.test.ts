import { describe, expect, it } from "vitest";
import type { InboxAction } from "@/lib/api/inbox";
import { formatInboxActionLabel } from "./action-format";

function makeAction(overrides: Partial<InboxAction> = {}): InboxAction {
  return {
    key: "rejected",
    label: "驳回",
    requires_comment: false,
    tone: "destructive",
    ...overrides,
  };
}

describe("formatInboxActionLabel", () => {
  it("translates legacy English labels to Chinese regardless of key", () => {
    expect(formatInboxActionLabel(makeAction({ key: "approved", label: "Approve" }))).toBe("同意");
    expect(formatInboxActionLabel(makeAction({ key: "rejected", label: "Reject" }))).toBe("驳回");
    expect(
      formatInboxActionLabel(
        makeAction({ key: "needs_more_evidence", label: "Request evidence" }),
      ),
    ).toBe("要求补证");
  });

  it("passes through a server-provided Chinese label as-is, even for a generic key", () => {
    // planning_gap 决策的第三个动作 DecisionActions 服务端给 {Key:"rejected", Label:"关闭"}——
    // 不能因为 key 是通用的 "rejected" 就强行覆盖成默认的"驳回"文案。
    expect(formatInboxActionLabel(makeAction({ key: "rejected", label: "关闭" }))).toBe("关闭");
    expect(
      formatInboxActionLabel(
        makeAction({ key: "restaffed", label: "已补员，重新规划", tone: "positive" }),
      ),
    ).toBe("已补员，重新规划");
    expect(
      formatInboxActionLabel(
        makeAction({ key: "exempted", label: "豁免约束并重规划", tone: "positive" }),
      ),
    ).toBe("豁免约束并重规划");
  });

  it("falls back to key-derived Chinese label only when the server label is empty", () => {
    expect(formatInboxActionLabel(makeAction({ key: "approved", label: "" }))).toBe("同意");
    expect(formatInboxActionLabel(makeAction({ key: "rejected", label: "" }))).toBe("驳回");
    expect(formatInboxActionLabel(makeAction({ key: "needs_more_evidence", label: "" }))).toBe(
      "要求补证",
    );
    expect(formatInboxActionLabel(makeAction({ key: "some_custom_key", label: "" }))).toBe("");
  });
});

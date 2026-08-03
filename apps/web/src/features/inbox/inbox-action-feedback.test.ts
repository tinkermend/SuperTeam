import { describe, expect, it } from "vitest";
import { inboxActionSuccessFeedback } from "./inbox-action-feedback";

describe("inboxActionSuccessFeedback", () => {
  it("tells project decision actors that Feishu cards will catch up", () => {
    expect(inboxActionSuccessFeedback("project_decision")).toEqual({
      message: "决策已提交",
      description: "飞书通知将同步更新为已处理"
    });
  });

  it("uses a generic success line for other inbox item types", () => {
    expect(inboxActionSuccessFeedback("approval")).toEqual({
      message: "操作已提交"
    });
    expect(inboxActionSuccessFeedback(undefined)).toEqual({
      message: "操作已提交"
    });
  });
});

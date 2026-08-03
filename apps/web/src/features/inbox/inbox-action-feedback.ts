/**
 * 收件箱动作成功后的 toast 文案。
 * project_decision 会投影到飞书审批卡：提示同步，降低「一边批了另一边还在等」的错觉。
 */
export function inboxActionSuccessFeedback(itemType: string | undefined): {
  message: string;
  description?: string;
} {
  if (itemType === "project_decision") {
    return {
      message: "决策已提交",
      description: "飞书通知将同步更新为已处理",
    };
  }
  return { message: "操作已提交" };
}

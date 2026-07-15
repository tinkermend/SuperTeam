import type { InboxAction } from "@/lib/api/inbox";

/**
 * 兼容层：早期/外部来源的 inbox action 可能只给英文 label（Approve/Reject/
 * Request evidence），这里补一层中文映射。服务端已提供的中文 label（如
 * DecisionActions 给 planning_gap 决策的“已补员，重新规划”“豁免并重规划”“关闭”）
 * 必须原样透传——不能仅凭 key 是通用的 approved/rejected/needs_more_evidence
 * 就强行覆盖成默认中文文案，否则同一个 key 在不同决策类型下的自定义 label 会被吞掉
 * （例如 planning_gap 的 rejected=关闭 曾被这里覆盖成驳回）。
 */
export function formatInboxActionLabel(action: InboxAction) {
  const label = action.label.trim();
  const normalizedLabel = label.toLowerCase();

  if (normalizedLabel === "approve") return "同意";
  if (normalizedLabel === "reject") return "驳回";
  if (normalizedLabel === "request evidence") return "要求补证";

  if (label.length > 0) {
    return label;
  }

  const normalizedKey = action.key.trim().toLowerCase();
  if (normalizedKey === "approved" || normalizedKey === "approve") return "同意";
  if (normalizedKey === "rejected" || normalizedKey === "reject") return "驳回";
  if (normalizedKey === "needs_more_evidence" || normalizedKey === "request_evidence") {
    return "要求补证";
  }

  return label;
}

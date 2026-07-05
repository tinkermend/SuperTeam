import type { InboxAction } from "@/lib/api/inbox";

export function formatInboxActionLabel(action: InboxAction) {
  const normalizedKey = action.key.trim().toLowerCase();
  const normalizedLabel = action.label.trim().toLowerCase();

  if (normalizedKey === "approved" || normalizedKey === "approve" || normalizedLabel === "approve") {
    return "同意";
  }
  if (normalizedKey === "rejected" || normalizedKey === "reject" || normalizedLabel === "reject") {
    return "驳回";
  }
  if (
    normalizedKey === "needs_more_evidence" ||
    normalizedKey === "request_evidence" ||
    normalizedLabel === "request evidence"
  ) {
    return "要求补证";
  }

  return action.label;
}

import { createFileRoute } from "@tanstack/react-router";
import { ApprovalsCenterPage } from "@/features/approvals";

export const Route = createFileRoute("/_authenticated/approvals/")({
  component: ApprovalsCenterPage,
});

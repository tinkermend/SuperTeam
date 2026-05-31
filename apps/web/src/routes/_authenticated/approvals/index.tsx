import { createFileRoute } from "@tanstack/react-router";
import { ShieldCheck } from "lucide-react";
import { UnimplementedPage } from "@/features/shared/unimplemented-page";

export const Route = createFileRoute("/_authenticated/approvals/")({
  component: () => (
    <UnimplementedPage
      icon={ShieldCheck}
      title="审批中心"
      description="承载高风险动作、需求歧义和上线发布等人类决策。"
    />
  ),
});

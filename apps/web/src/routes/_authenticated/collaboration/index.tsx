import { createFileRoute } from "@tanstack/react-router";
import { MessagesSquare } from "lucide-react";
import { UnimplementedPage } from "@/features/shared/unimplemented-page";

export const Route = createFileRoute("/_authenticated/collaboration/")({
  component: () => (
    <UnimplementedPage
      tone="info"
      icon={MessagesSquare}
      title="协作集成"
      description="接入钉钉、飞书等企业通讯软件，承载消息交互、审批触达和结果通知。"
    />
  ),
});

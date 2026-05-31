import { createFileRoute } from "@tanstack/react-router";
import { Server } from "lucide-react";
import { UnimplementedPage } from "@/features/shared/unimplemented-page";

export const Route = createFileRoute("/_authenticated/runtime/")({
  component: () => (
    <UnimplementedPage
      icon={Server}
      title="Runtime 节点"
      description="查看 Runtime Agent 节点、Provider 支持能力、负载和心跳状态。"
    />
  ),
});

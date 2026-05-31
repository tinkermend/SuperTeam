import { createFileRoute } from "@tanstack/react-router";
import { Puzzle } from "lucide-react";
import { UnimplementedPage } from "@/features/shared/unimplemented-page";

export const Route = createFileRoute("/_authenticated/capabilities/")({
  component: () => (
    <UnimplementedPage
      icon={Puzzle}
      title="外部能力"
      description="注册和审计 Dify、Deephub、HTTP 服务和企业内部系统能力。"
    />
  ),
});

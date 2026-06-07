import { createFileRoute } from "@tanstack/react-router";
import { FolderKanban } from "lucide-react";
import { UnimplementedPage } from "@/features/shared/unimplemented-page";

export const Route = createFileRoute("/_authenticated/projects/")({
  component: () => (
    <UnimplementedPage
      tone="info"
      icon={FolderKanban}
      title="项目管理"
      description="组织项目目标、阶段、任务集合、交付物和验收状态。"
    />
  ),
});

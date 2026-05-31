import { createFileRoute } from "@tanstack/react-router";
import { GitBranch } from "lucide-react";
import { UnimplementedPage } from "@/features/shared/unimplemented-page";

export const Route = createFileRoute("/_authenticated/workflows/")({
  component: () => (
    <UnimplementedPage
      icon={GitBranch}
      title="流程编排"
      description="编排需求分析、人类确认、Runtime 执行、工件回传和验收流程。"
    />
  ),
});

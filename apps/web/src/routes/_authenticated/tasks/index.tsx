import { createFileRoute } from "@tanstack/react-router";
import { ClipboardList } from "lucide-react";
import { UnimplementedPage } from "@/features/shared/unimplemented-page";

export const Route = createFileRoute("/_authenticated/tasks/")({
  component: () => (
    <UnimplementedPage
      tone="primary"
      icon={ClipboardList}
      title="任务中心"
      description="围绕任务输入、输出、上下文、风险和验收状态组织执行链路。"
    />
  ),
});

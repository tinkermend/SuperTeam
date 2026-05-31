import { createFileRoute } from "@tanstack/react-router";
import { Bot } from "lucide-react";
import { UnimplementedPage } from "@/features/shared/unimplemented-page";

export const Route = createFileRoute("/_authenticated/employees/")({
  component: () => (
    <UnimplementedPage
      icon={Bot}
      title="数字员工"
      description="管理数字员工定义、技能绑定、权限边界和上下文策略。"
    />
  ),
});

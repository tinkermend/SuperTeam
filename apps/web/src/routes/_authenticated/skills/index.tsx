import { createFileRoute } from "@tanstack/react-router";
import { Blocks } from "lucide-react";
import { UnimplementedPage } from "@/features/shared/unimplemented-page";

export const Route = createFileRoute("/_authenticated/skills/")({
  component: () => (
    <UnimplementedPage
      icon={Blocks}
      title="技能管理"
      description="管理数字员工可绑定的技能、输入输出约束和风险边界。"
    />
  ),
});

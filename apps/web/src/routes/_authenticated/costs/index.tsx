import { createFileRoute } from "@tanstack/react-router";
import { CircleDollarSign } from "lucide-react";
import { UnimplementedPage } from "@/features/shared/unimplemented-page";

export const Route = createFileRoute("/_authenticated/costs/")({
  component: () => (
    <UnimplementedPage
      icon={CircleDollarSign}
      title="成本管理"
      description="按数字员工查看 token 消耗成本，并预留每日、每月 token 成本统计视图。"
    />
  ),
});

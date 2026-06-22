import { createFileRoute } from "@tanstack/react-router";
import { CalendarClock } from "lucide-react";
import { UnimplementedPage } from "@/features/shared/unimplemented-page";

export const Route = createFileRoute("/_authenticated/automations/")({
  component: () => (
    <UnimplementedPage
      tone="info"
      icon={CalendarClock}
      title="自动化任务"
      description="配置由定时规则驱动的平台任务入口，后续接入 Control Plane 的调度、审批、审计和执行记录。"
    />
  ),
});

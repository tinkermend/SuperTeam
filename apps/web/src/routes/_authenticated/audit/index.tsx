import { createFileRoute } from "@tanstack/react-router";
import { FileClock } from "lucide-react";
import { UnimplementedPage } from "@/features/shared/unimplemented-page";

export const Route = createFileRoute("/_authenticated/audit/")({
  component: () => (
    <UnimplementedPage
      tone="neutral" icon={FileClock} title="审计日志" description="查询平台关键操作、登录事件、风险和执行结果。" />
  ),
});

import { createFileRoute } from "@tanstack/react-router";
import { Gauge } from "lucide-react";
import { UnimplementedPage } from "@/features/shared/unimplemented-page";

export const Route = createFileRoute("/_authenticated/run-overview/")({
  component: () => (
    <UnimplementedPage
      tone="info"
      icon={Gauge}
      title="运行总览"
      description="查看平台运行状态、执行链路和关键运行信号的统一入口。"
    />
  ),
});

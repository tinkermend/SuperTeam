import { createFileRoute } from "@tanstack/react-router";
import { RunOverviewPage } from "@/features/run-overview";

type RunOverviewSearch = {
  employee?: string;
  project?: string;
  // 大屏投屏模式:去侧栏 + KPI 带 + 镜头轮巡;投屏设备以带 ?mode=display 的 URL 直开。
  mode?: "display";
};

export const Route = createFileRoute("/_authenticated/run-overview/")({
  component: RunOverviewPage,
  validateSearch: (search: Record<string, unknown>): RunOverviewSearch => {
    const result: RunOverviewSearch = {};
    if (typeof search.employee === "string" && search.employee) result.employee = search.employee;
    if (typeof search.project === "string" && search.project) result.project = search.project;
    if (search.mode === "display") result.mode = "display";
    return result;
  }
});

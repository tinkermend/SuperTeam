import { createFileRoute } from "@tanstack/react-router";
import { RunOverviewPage } from "@/features/run-overview";

type RunOverviewSearch = {
  employee?: string;
  project?: string;
};

export const Route = createFileRoute("/_authenticated/run-overview/")({
  component: RunOverviewPage,
  validateSearch: (search: Record<string, unknown>): RunOverviewSearch => {
    const result: RunOverviewSearch = {};
    if (typeof search.employee === "string" && search.employee) result.employee = search.employee;
    if (typeof search.project === "string" && search.project) result.project = search.project;
    return result;
  }
});

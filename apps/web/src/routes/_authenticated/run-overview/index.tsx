import { createFileRoute } from "@tanstack/react-router";
import { RunOverviewPage } from "@/features/run-overview";

export const Route = createFileRoute("/_authenticated/run-overview/")({
  component: RunOverviewPage,
});

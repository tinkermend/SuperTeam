import { createFileRoute } from "@tanstack/react-router";
import { WorkflowPage } from "@/features/workflows";

export const Route = createFileRoute("/_authenticated/workflows/")({
  component: WorkflowPage,
});

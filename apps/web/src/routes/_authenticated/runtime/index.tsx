import { createFileRoute } from "@tanstack/react-router";
import { RuntimeNodesPage } from "@/features/runtime";

export const Route = createFileRoute("/_authenticated/runtime/")({
  component: RuntimeNodesPage,
});

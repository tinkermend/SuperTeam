import { createFileRoute } from "@tanstack/react-router";
import { ScenarioTemplatesPage } from "@/features/scenario-templates";

export const Route = createFileRoute("/_authenticated/scenario-templates/")({
  component: ScenarioTemplatesPage
});

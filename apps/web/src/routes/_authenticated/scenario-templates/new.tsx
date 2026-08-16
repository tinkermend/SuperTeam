import { createFileRoute } from "@tanstack/react-router";
import { ScenarioTemplateComposerPage } from "@/features/scenario-templates/composer-page";

export const Route = createFileRoute("/_authenticated/scenario-templates/new")({
  component: NewScenarioTemplateRoute,
});

function NewScenarioTemplateRoute() {
  return <ScenarioTemplateComposerPage mode="create" />;
}

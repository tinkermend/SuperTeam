import { createFileRoute } from "@tanstack/react-router";
import { ScenarioTemplateComposerPage } from "@/features/scenario-templates/composer-page";

export const Route = createFileRoute("/_authenticated/scenario-templates/$templateKey/edit")({
  component: EditScenarioTemplateRoute,
});

function EditScenarioTemplateRoute() {
  const { templateKey } = Route.useParams();
  return <ScenarioTemplateComposerPage mode="edit" templateKey={templateKey} />;
}

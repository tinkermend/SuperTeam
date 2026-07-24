import { createFileRoute } from "@tanstack/react-router";
import { TemplateDetailPage } from "@/features/employees/templates";

export const Route = createFileRoute("/_authenticated/employees/templates/$templateType")({
  component: TemplateDetailRoute
});

function TemplateDetailRoute() {
  const { templateType } = Route.useParams();
  return <TemplateDetailPage templateType={templateType} />;
}

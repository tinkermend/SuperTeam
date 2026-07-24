import { createFileRoute } from "@tanstack/react-router";
import { TemplateListPage } from "@/features/employees/templates";

export const Route = createFileRoute("/_authenticated/employees/templates/")({
  component: TemplateListPage
});

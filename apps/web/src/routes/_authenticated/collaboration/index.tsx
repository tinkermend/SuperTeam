import { createFileRoute } from "@tanstack/react-router";
import { ExternalIntegrationsPage } from "@/features/external-integrations";

export const Route = createFileRoute("/_authenticated/collaboration/")({
  component: ExternalIntegrationsPage,
});

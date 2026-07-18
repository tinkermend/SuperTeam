import { createFileRoute } from "@tanstack/react-router";
import { SystemConfigPage } from "@/features/system-config";

export const Route = createFileRoute("/_authenticated/system-config/")({
  component: SystemConfigPage,
});

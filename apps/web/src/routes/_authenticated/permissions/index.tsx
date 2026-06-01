import { createFileRoute } from "@tanstack/react-router";
import { PermissionsCenter } from "@/features/permissions";

export const Route = createFileRoute("/_authenticated/permissions/")({
  component: PermissionsCenter,
});

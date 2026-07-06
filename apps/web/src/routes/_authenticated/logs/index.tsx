import { createFileRoute, redirect } from "@tanstack/react-router";

export const Route = createFileRoute("/_authenticated/logs/")({
  beforeLoad: () => {
    throw redirect({ to: "/logs/login" });
  },
});

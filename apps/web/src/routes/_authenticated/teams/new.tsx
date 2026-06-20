import { createFileRoute } from "@tanstack/react-router";
import { CreateTeamPage } from "@/features/teams";

export const Route = createFileRoute("/_authenticated/teams/new")({
  component: CreateTeamPage,
});

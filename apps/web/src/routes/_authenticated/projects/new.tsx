import { createFileRoute } from "@tanstack/react-router";
import { CreateProjectPage } from "@/features/projects";

export const Route = createFileRoute("/_authenticated/projects/new")({
  component: CreateProjectPage
});

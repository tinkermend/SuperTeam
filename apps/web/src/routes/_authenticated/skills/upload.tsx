import { createFileRoute } from "@tanstack/react-router";
import { SkillUploadPage } from "@/features/skills/upload";

export const Route = createFileRoute("/_authenticated/skills/upload")({
  component: SkillUploadPage
});

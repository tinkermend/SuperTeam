import { createFileRoute } from "@tanstack/react-router";
import { InboxPage } from "@/features/inbox";

export const Route = createFileRoute("/_authenticated/inbox/")({
  component: InboxPage,
});

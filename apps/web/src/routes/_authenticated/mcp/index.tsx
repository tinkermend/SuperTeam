import { createFileRoute } from "@tanstack/react-router";
import { McpManagementPage } from "@/features/mcp";

export const Route = createFileRoute("/_authenticated/mcp/")({
  component: McpManagementPage
});

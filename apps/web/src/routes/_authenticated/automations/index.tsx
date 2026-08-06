import { createFileRoute } from "@tanstack/react-router";
import { AutomationsPage } from "@/features/automations";

export type AutomationsSearch = {
  rule?: string;
};

export const Route = createFileRoute("/_authenticated/automations/")({
  component: AutomationsPage,
  validateSearch: (search: Record<string, unknown>): AutomationsSearch => {
    const result: AutomationsSearch = {};
    if (typeof search.rule === "string" && search.rule.trim()) {
      result.rule = search.rule.trim();
    }
    return result;
  },
});

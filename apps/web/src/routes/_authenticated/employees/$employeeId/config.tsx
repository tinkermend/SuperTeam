import { createFileRoute } from "@tanstack/react-router";
import { EmployeeConfigPage } from "@/features/employees/config";
import { parseEmployeeConfigTab, type EmployeeConfigTab } from "@/features/employees/config-utils";

export type EmployeeConfigSearch = {
  tab?: EmployeeConfigTab;
};

export const Route = createFileRoute("/_authenticated/employees/$employeeId/config")({
  component: RouteComponent,
  validateSearch: (search: Record<string, unknown>): EmployeeConfigSearch => {
    return { tab: parseEmployeeConfigTab(search.tab) };
  },
});

function RouteComponent() {
  const { employeeId } = Route.useParams();
  const { tab } = Route.useSearch();
  return <EmployeeConfigPage employeeId={employeeId} tab={tab} />;
}

import { createFileRoute } from "@tanstack/react-router";
import { EmployeeConfigPage } from "@/features/employees/config";

export const Route = createFileRoute("/_authenticated/employees/$employeeId/config")({
  component: RouteComponent,
});

function RouteComponent() {
  const { employeeId } = Route.useParams();
  return <EmployeeConfigPage employeeId={employeeId} />;
}

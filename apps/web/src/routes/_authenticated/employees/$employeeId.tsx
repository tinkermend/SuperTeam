import { createFileRoute } from "@tanstack/react-router";
import { EmployeeDetailPage } from "@/features/employees/detail";

export const Route = createFileRoute("/_authenticated/employees/$employeeId")({
  component: EmployeeDetailRoute,
});

function EmployeeDetailRoute() {
  const { employeeId } = Route.useParams();

  return <EmployeeDetailPage employeeId={employeeId} />;
}

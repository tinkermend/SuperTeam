import { Outlet, createFileRoute, useRouterState } from "@tanstack/react-router";
import { EmployeeDetailPage } from "@/features/employees/detail";

export const Route = createFileRoute("/_authenticated/employees/$employeeId")({
  component: EmployeeDetailRoute,
});

function EmployeeDetailRoute() {
  const { employeeId } = Route.useParams();

  return <EmployeeRouteContent employeeId={employeeId} />;
}

export function EmployeeRouteContent({ employeeId }: { employeeId: string }) {
  const pathname = useRouterState({ select: (state) => state.location.pathname });

  if (pathname.endsWith("/config")) {
    return <Outlet />;
  }

  return <EmployeeDetailPage employeeId={employeeId} />;
}

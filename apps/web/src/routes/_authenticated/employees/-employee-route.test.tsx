import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { EmployeeRouteContent } from "./$employeeId";

vi.mock("@/features/employees/detail", () => ({
  EmployeeDetailPage: ({ employeeId }: { employeeId: string }) => (
    <div>员工详情 {employeeId}</div>
  ),
}));

vi.mock("@tanstack/react-router", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-router")>(
    "@tanstack/react-router",
  );

  return {
    ...actual,
    Outlet: () => <div>员工配置子路由</div>,
    useRouterState: ({
      select,
    }: {
      select: (state: { location: { pathname: string } }) => string;
    }) => select({ location: { pathname: "/employees/employee-1/config" } }),
  };
});

describe("EmployeeRouteContent", () => {
  it("renders child route content for employee config path", async () => {
    const screen = await render(<EmployeeRouteContent employeeId="employee-1" />);

    await expect.element(screen.getByText("员工配置子路由")).toBeInTheDocument();
    await expect.element(screen.getByText("员工详情 employee-1")).not.toBeInTheDocument();
  });
});

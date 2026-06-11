import { describe, expect, it, vi } from "vitest";
import { render } from "vitest-browser-react";
import { ProjectRouteContent } from "./$projectId";

vi.mock("@/features/projects", () => ({
  ProjectDetailPage: ({ projectId }: { projectId: string }) => (
    <div>项目详情 {projectId}</div>
  ),
}));

vi.mock("@tanstack/react-router", async () => {
  const actual = await vi.importActual<typeof import("@tanstack/react-router")>(
    "@tanstack/react-router",
  );

  return {
    ...actual,
    Outlet: () => <div>配置子路由</div>,
    useRouterState: ({
      select,
    }: {
      select: (state: { location: { pathname: string } }) => string;
    }) => select({ location: { pathname: "/projects/project-1/config" } }),
  };
});

describe("ProjectRouteContent", () => {
  it("renders child route content for project config path", async () => {
    const screen = await render(<ProjectRouteContent projectId="project-1" />);

    await expect.element(screen.getByText("配置子路由")).toBeInTheDocument();
    await expect.element(screen.getByText("项目详情 project-1")).not.toBeInTheDocument();
  });
});

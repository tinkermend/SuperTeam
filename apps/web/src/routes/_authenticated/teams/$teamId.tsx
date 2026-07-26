import { Outlet, createFileRoute, useRouterState } from "@tanstack/react-router";
import { TeamDetailPage } from "@/features/teams";

export const Route = createFileRoute("/_authenticated/teams/$teamId")({
  component: TeamDetailRoute
});

// $teamId 既是详情路由，也是 /config 的父路由：命中子路由时必须让位给 Outlet，
// 否则子路由渲染不出来、页面停在详情（与 employees/$employeeId 同一处理）。
function TeamDetailRoute() {
  const { teamId } = Route.useParams();
  const pathname = useRouterState({ select: (state) => state.location.pathname });

  if (pathname.endsWith("/config")) {
    return <Outlet />;
  }

  return <TeamDetailPage teamId={teamId} />;
}

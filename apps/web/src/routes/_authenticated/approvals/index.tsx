import { createFileRoute, redirect } from "@tanstack/react-router";

// 审批中心已退役:项目决策事项回归收件箱,权限审批并入权限中心「权限审批」页签。
export const Route = createFileRoute("/_authenticated/approvals/")({
  beforeLoad: () => {
    throw redirect({
      to: "/permissions",
      search: { tab: "permission-approvals" },
      replace: true,
    });
  },
});

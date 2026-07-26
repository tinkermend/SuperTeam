import { createFileRoute, redirect } from "@tanstack/react-router";

// 流程编排菜单已退役（IA Phase 2 P2c）：跨项目实例监控迁任务中枢「流程实例」页签。
// 路由保留为重定向壳，兼容历史链接。
export const Route = createFileRoute("/_authenticated/workflows/")({
  beforeLoad: () => {
    throw redirect({ replace: true, search: { view: "instances" }, to: "/" });
  }
});

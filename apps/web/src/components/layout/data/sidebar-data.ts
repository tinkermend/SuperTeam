import {
  Bot,
  ClipboardList,
  FileClock,
  GitBranch,
  LayoutDashboard,
  Puzzle,
  Server,
  ShieldCheck,
  Users,
} from "lucide-react";
import { type SidebarData } from "../types";

export const sidebarData: SidebarData = {
  navGroups: [
    {
      title: "控制台",
      items: [
        {
          title: "工作台",
          url: "/",
          icon: LayoutDashboard,
        },
        {
          title: "任务中心",
          url: "/tasks",
          icon: ClipboardList,
        },
        {
          title: "数字员工",
          url: "/employees",
          icon: Bot,
        },
        {
          title: "流程编排",
          url: "/workflows",
          icon: GitBranch,
        },
        {
          title: "外部能力",
          url: "/capabilities",
          icon: Puzzle,
        },
        {
          title: "审批中心",
          url: "/approvals",
          icon: ShieldCheck,
        },
        {
          title: "Runtime 节点",
          url: "/runtime",
          icon: Server,
        },
        {
          title: "用户管理",
          url: "/users",
          icon: Users,
        },
        {
          title: "审计日志",
          url: "/audit",
          icon: FileClock,
        },
      ],
    },
  ],
};

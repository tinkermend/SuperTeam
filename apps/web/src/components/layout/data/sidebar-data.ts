import {
  Blocks,
  Bot,
  CircleDollarSign,
  ClipboardList,
  FileClock,
  FolderKanban,
  GitBranch,
  KeyRound,
  LayoutDashboard,
  MessagesSquare,
  Puzzle,
  Server,
  ShieldCheck,
  Users,
  UsersRound,
} from "lucide-react";
import { type SidebarData } from "../types";

export const sidebarData: SidebarData = {
  navGroups: [
    {
      title: "工作区",
      items: [
        {
          title: "工作台",
          url: "/",
          icon: LayoutDashboard,
          iconTone: "primary",
        },
        {
          title: "任务中心",
          url: "/tasks",
          icon: ClipboardList,
          iconTone: "task",
        },
        {
          title: "数字员工",
          url: "/employees",
          icon: Bot,
          iconTone: "employee",
        },
        {
          title: "技能管理",
          url: "/skills",
          icon: Blocks,
          iconTone: "capability",
        },
        {
          title: "项目管理",
          url: "/projects",
          icon: FolderKanban,
          iconTone: "workflow",
        },
        {
          title: "团队管理",
          url: "/teams",
          icon: UsersRound,
          iconTone: "permission",
        },
      ],
    },
    {
      title: "核心导航",
      items: [
        {
          title: "流程编排",
          url: "/workflows",
          icon: GitBranch,
          iconTone: "workflow",
        },
        {
          title: "外部能力",
          url: "/capabilities",
          icon: Puzzle,
          iconTone: "capability",
        },
        {
          title: "协作集成",
          url: "/collaboration",
          icon: MessagesSquare,
          iconTone: "approval",
        },
        {
          title: "审批中心",
          url: "/approvals",
          icon: ShieldCheck,
          iconTone: "approval",
        },
        {
          title: "Runtime 节点",
          url: "/runtime",
          icon: Server,
          iconTone: "runtime",
        },
      ],
    },
    {
      title: "平台管理",
      items: [
        {
          title: "权限中心",
          url: "/permissions",
          icon: KeyRound,
          iconTone: "permission",
        },
        {
          title: "成本管理",
          url: "/costs",
          icon: CircleDollarSign,
          iconTone: "audit",
        },
        {
          title: "用户管理",
          url: "/users",
          icon: Users,
          iconTone: "neutral",
        },
        {
          title: "审计日志",
          url: "/audit",
          icon: FileClock,
          iconTone: "audit",
        },
      ],
    },
  ],
};

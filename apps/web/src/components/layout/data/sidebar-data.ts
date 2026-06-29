import {
  Blocks,
  Bot,
  CalendarClock,
  CircleDollarSign,
  FileClock,
  FolderKanban,
  GitBranch,
  Inbox,
  KeyRound,
  MessagesSquare,
  Network,
  Puzzle,
  SendHorizontal,
  Server,
  ShieldCheck,
  Users,
  UsersRound,
} from "lucide-react";
import { type SidebarData } from "../types";

type BuildSidebarDataOptions = {
  inboxBadge?: string;
};

export function buildSidebarData({
  inboxBadge,
}: BuildSidebarDataOptions = {}): SidebarData {
  return {
    navGroups: [
      {
        title: "工作区",
        items: [
          {
            title: "任务中枢",
            url: "/",
            icon: SendHorizontal,
            iconTone: "neutral",
          },
          {
            title: "收件箱",
            url: "/inbox",
            icon: Inbox,
            iconTone: "neutral",
            ...(inboxBadge ? { badge: inboxBadge } : {}),
          },
          {
            title: "项目管理",
            url: "/projects",
            icon: FolderKanban,
            iconTone: "neutral",
          },
          {
            title: "数字员工",
            url: "/employees",
            icon: Bot,
            iconTone: "neutral",
          },
          {
            title: "技能管理",
            url: "/skills",
            icon: Blocks,
            iconTone: "neutral",
          },
          {
            title: "团队管理",
            url: "/teams",
            icon: UsersRound,
            iconTone: "neutral",
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
            iconTone: "neutral",
          },
          {
            title: "自动化任务",
            url: "/automations",
            icon: CalendarClock,
            iconTone: "neutral",
          },
          {
            title: "外部能力",
            url: "/capabilities",
            icon: Puzzle,
            iconTone: "neutral",
          },
          {
            title: "MCP 管理",
            url: "/mcp",
            icon: Network,
            iconTone: "neutral",
          },
          {
            title: "协作集成",
            url: "/collaboration",
            icon: MessagesSquare,
            iconTone: "neutral",
          },
          {
            title: "审批中心",
            url: "/approvals",
            icon: ShieldCheck,
            iconTone: "neutral",
          },
          {
            title: "Runtime 节点",
            url: "/runtime",
            icon: Server,
            iconTone: "neutral",
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
            iconTone: "neutral",
          },
          {
            title: "成本管理",
            url: "/costs",
            icon: CircleDollarSign,
            iconTone: "neutral",
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
            iconTone: "neutral",
          },
        ],
      },
    ],
  };
}

export const sidebarData: SidebarData = buildSidebarData();

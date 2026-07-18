import {
  Blocks,
  Bot,
  CalendarClock,
  CircleDollarSign,
  FileClock,
  FolderKanban,
  GitBranch,
  Gauge,
  Inbox,
  KeyRound,
  LayoutTemplate,
  MessagesSquare,
  Network,
  Puzzle,
  ScrollText,
  SendHorizontal,
  Server,
  Settings2,
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
        title: "工作台",
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
            title: "运行总览",
            url: "/run-overview",
            icon: Gauge,
            iconTone: "neutral",
          },
        ],
      },
      {
        title: "协作对象",
        items: [
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
        title: "流程能力",
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
            title: "场景模板",
            url: "/scenario-templates",
            icon: LayoutTemplate,
            iconTone: "neutral",
          },
          {
            title: "协作集成",
            url: "/collaboration",
            icon: MessagesSquare,
            iconTone: "neutral",
          },
        ],
      },
      {
        title: "治理平台",
        items: [
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
            title: "审计中心",
            url: "/audit",
            icon: FileClock,
            iconTone: "neutral",
          },
          {
            title: "日志管理",
            url: "/logs",
            icon: ScrollText,
            iconTone: "neutral",
          },
          {
            title: "系统配置",
            url: "/system-config",
            icon: Settings2,
            iconTone: "neutral",
          },
        ],
      },
    ],
  };
}

export const sidebarData: SidebarData = buildSidebarData();

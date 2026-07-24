import { type LinkProps } from "@tanstack/react-router";

type NavIconTone =
  | "primary"
  | "task"
  | "employee"
  | "workflow"
  | "capability"
  | "approval"
  | "runtime"
  | "permission"
  | "audit"
  | "neutral";

type BaseNavItem = {
  title: string;
  badge?: string;
  icon?: React.ElementType;
  iconTone?: NavIconTone;
  /** 悬停/聚焦时预热目标页数据（如收件箱列表）。 */
  onPrefetch?: () => void;
};

type NavLink = BaseNavItem & {
  url: LinkProps["to"] | (string & {});
  items?: never;
};

type NavCollapsible = BaseNavItem & {
  items: (BaseNavItem & { url: LinkProps["to"] | (string & {}) })[];
  url?: never;
};

type NavItem = NavCollapsible | NavLink;

type NavGroup = {
  title: string;
  items: NavItem[];
};

type SidebarData = {
  navGroups: NavGroup[];
};

export type {
  SidebarData,
  NavGroup,
  NavItem,
  NavCollapsible,
  NavLink,
  NavIconTone,
};

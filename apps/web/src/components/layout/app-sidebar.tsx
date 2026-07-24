import { useCallback, useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useLayout } from "@/context/layout-provider";
import {
  Sidebar,
  SidebarContent,
  SidebarHeader,
  SidebarRail
} from "@/components/ui/sidebar";
import {
  DEFAULT_INBOX_LIST_FILTERS,
  DEFAULT_INBOX_VIEW,
  inboxItemsQueryKey
} from "@/features/inbox/inbox-query";
import { inboxBadgeRefetchInterval } from "@/features/inbox/inbox-stream-status";
import { useInboxStreamStatus } from "@/features/inbox/use-inbox-stream-status";
import { getInboxBadge, listInboxItems } from "@/lib/api/inbox";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { AppTitle } from "./app-title";
import { buildSidebarData } from "./data/sidebar-data";
import { NavGroup } from "./nav-group";
import type { NavItem } from "./types";

export function AppSidebar() {
  const { collapsible, variant } = useLayout();
  const apiBaseUrl = resolveControlPlaneUrl();
  const queryClient = useQueryClient();
  const streamStatus = useInboxStreamStatus();
  // 401 必须抛出：QueryCache 会导向 /login；勿吞成全零——否则失效会话被伪装成「无待办」。
  const inboxBadgeQuery = useQuery({
    queryKey: ["inbox-badge"],
    queryFn: () => getInboxBadge({ baseUrl: apiBaseUrl }),
    staleTime: 60 * 1000,
    // 主刷新由全局 SSE（含 onopen 重连追平）；断流时与列表同频 5s 快拉。
    refetchInterval: inboxBadgeRefetchInterval(streamStatus.connection),
    refetchOnWindowFocus: true
});
  const inboxBadge =
    inboxBadgeQuery.data && inboxBadgeQuery.data.mine_open_count > 0
      ? String(inboxBadgeQuery.data.mine_open_count)
      : undefined;
  const prefetchInboxList = useCallback(() => {
    void queryClient.prefetchQuery({
      queryKey: inboxItemsQueryKey(DEFAULT_INBOX_VIEW, DEFAULT_INBOX_LIST_FILTERS),
      queryFn: () =>
        listInboxItems(
          { baseUrl: apiBaseUrl },
          { ...DEFAULT_INBOX_LIST_FILTERS, view: DEFAULT_INBOX_VIEW },
        ),
    });
  }, [apiBaseUrl, queryClient]);
  const sidebarData = useMemo(() => {
    const data = buildSidebarData({ inboxBadge });
    return {
      ...data,
      navGroups: data.navGroups.map((group) => ({
        ...group,
        items: group.items.map((item) => attachInboxPrefetch(item, prefetchInboxList)),
      })),
    };
  }, [inboxBadge, prefetchInboxList]);

  return (
    <Sidebar collapsible={collapsible} variant={variant}>
      <SidebarHeader>
        <AppTitle />
      </SidebarHeader>
      <SidebarContent>
        {sidebarData.navGroups.map((props) => (
          <NavGroup key={props.title} {...props} />
        ))}
      </SidebarContent>
      <SidebarRail />
    </Sidebar>
  );
}

function attachInboxPrefetch(item: NavItem, onPrefetch: () => void): NavItem {
  if (!("url" in item) || item.url !== "/inbox") {
    return item;
  }
  return { ...item, onPrefetch };
}

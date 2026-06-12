import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useLayout } from "@/context/layout-provider";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarRail,
} from "@/components/ui/sidebar";
import { getInboxBadge } from "@/lib/api/inbox";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { AppTitle } from "./app-title";
import { buildSidebarData } from "./data/sidebar-data";
import { NavGroup } from "./nav-group";
import { NavUser } from "./nav-user";

export function AppSidebar() {
  const { collapsible, variant } = useLayout();
  const apiBaseUrl = resolveControlPlaneUrl();
  const inboxBadgeQuery = useQuery({
    queryKey: ["inbox-badge"],
    queryFn: async () => {
      try {
        return await getInboxBadge({ baseUrl: apiBaseUrl });
      } catch {
        return { mine_open_count: 0, team_open_count: 0, high_risk_count: 0 };
      }
    },
    staleTime: 30 * 1000,
  });
  const inboxBadge =
    inboxBadgeQuery.data && inboxBadgeQuery.data.mine_open_count > 0
      ? String(inboxBadgeQuery.data.mine_open_count)
      : undefined;
  const sidebarData = useMemo(() => buildSidebarData({ inboxBadge }), [inboxBadge]);

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
      <SidebarFooter>
        <NavUser />
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  );
}

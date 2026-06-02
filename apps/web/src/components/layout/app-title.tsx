import { Link } from "@tanstack/react-router";
import { Command } from "lucide-react";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";

export function AppTitle() {
  const { setOpenMobile } = useSidebar();

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <SidebarMenuButton size="lg" asChild>
          <Link to="/" onClick={() => setOpenMobile(false)}>
            <div className="superteam-primary-action flex aspect-square size-9 items-center justify-center rounded-xl text-white">
              <Command />
            </div>
            <div className="grid flex-1 text-start text-sm leading-tight">
              <span className="truncate text-base font-semibold text-slate-950 dark:text-slate-50">
                SuperTeam
              </span>
              <span className="truncate text-xs text-muted-foreground">
                数字员工控制平面
              </span>
            </div>
          </Link>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}

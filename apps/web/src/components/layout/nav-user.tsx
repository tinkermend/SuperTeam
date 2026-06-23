import { Link } from "@tanstack/react-router";
import { BadgeCheck, ChevronsUpDown, LogOut } from "lucide-react";
import { useAuth } from "@/features/auth/use-auth";
import useDialogState from "@/hooks/use-dialog-state";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";
import { SignOutDialog } from "@/components/sign-out-dialog";

export function NavUser() {
  const { user } = useAuth();
  const { isMobile } = useSidebar();
  const [open, setOpen] = useDialogState();
  const displayName = user?.display_name || user?.username || "未登录";
  const displayEmail = user?.email || user?.username || (user?.status === "active" ? "active" : "disabled");
  const fallback = displayName.slice(0, 2).toUpperCase();

  return (
    <>
      <SidebarMenu>
        <SidebarMenuItem>
          <DropdownMenu>
            <DropdownMenuTrigger asChild>
              <SidebarMenuButton
                size="lg"
                className="border border-v3-line bg-v3-card-soft text-v3-ink shadow-v3 data-[state=open]:bg-v3-brand-soft data-[state=open]:text-v3-brand-deep"
              >
                <Avatar className="size-8 rounded-lg">
                  <AvatarFallback className="rounded-lg bg-v3-brand text-white">
                    {fallback}
                  </AvatarFallback>
                </Avatar>
                <div className="grid flex-1 text-start text-sm leading-tight">
                  <span className="truncate font-semibold text-v3-ink">
                    {displayName}
                  </span>
                  <span className="truncate text-xs text-v3-ink-2">
                    {displayEmail}
                  </span>
                </div>
                <ChevronsUpDown className="ms-auto size-4 text-v3-ink-3" />
              </SidebarMenuButton>
            </DropdownMenuTrigger>
            <DropdownMenuContent
              className="w-(--radix-dropdown-menu-trigger-width) min-w-56 rounded-v3-inner border-v3-line bg-v3-card text-v3-ink shadow-v3-pop"
              side={isMobile ? "bottom" : "right"}
              align="end"
              sideOffset={4}
            >
              <DropdownMenuLabel className="p-0 font-normal">
                <div className="flex items-center gap-2 px-1 py-1.5 text-start text-sm">
                  <Avatar className="size-8 rounded-lg">
                    <AvatarFallback className="rounded-lg bg-v3-brand text-white">
                      {fallback}
                    </AvatarFallback>
                  </Avatar>
                  <div className="grid flex-1 text-start text-sm leading-tight">
                    <span className="truncate font-semibold">
                      {displayName}
                    </span>
                    <span className="truncate text-xs">{displayEmail}</span>
                  </div>
                </div>
              </DropdownMenuLabel>
              <DropdownMenuSeparator />
              <DropdownMenuItem asChild>
                <Link to="/settings/account">
                  <BadgeCheck />
                  账户
                </Link>
              </DropdownMenuItem>
              <DropdownMenuSeparator />
              <DropdownMenuItem
                variant="destructive"
                onClick={() => setOpen(true)}
              >
                <LogOut />
                退出登录
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
        </SidebarMenuItem>
      </SidebarMenu>

      <SignOutDialog open={!!open} onOpenChange={setOpen} />
    </>
  );
}

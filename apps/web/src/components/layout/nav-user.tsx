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
  DropdownMenuTrigger
} from "@/components/ui/dropdown-menu";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar
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
          <DropdownMenu modal={false}>
            <DropdownMenuTrigger asChild>
              <SidebarMenuButton
                size="lg"
                className="border border-line bg-card-soft text-ink shadow-card data-[state=open]:bg-brand-soft data-[state=open]:text-brand-deep"
              >
                <Avatar className="size-8 rounded-lg">
                  <AvatarFallback className="rounded-lg bg-brand text-white">
                    {fallback}
                  </AvatarFallback>
                </Avatar>
                <div className="grid flex-1 text-start text-sm leading-tight">
                  <span className="truncate font-semibold text-ink">
                    {displayName}
                  </span>
                  <span className="truncate text-xs text-ink-2">
                    {displayEmail}
                  </span>
                </div>
                <ChevronsUpDown className="ms-auto size-4 text-ink-3" />
              </SidebarMenuButton>
            </DropdownMenuTrigger>
            <DropdownMenuContent
              className="w-(--radix-dropdown-menu-trigger-width) min-w-56 rounded-inner border-line bg-card text-ink shadow-pop"
              side={isMobile ? "bottom" : "right"}
              align="end"
              sideOffset={4}
            >
              <DropdownMenuLabel className="p-0 font-normal">
                <div className="flex items-center gap-2 px-1 py-1.5 text-start text-sm">
                  <Avatar className="size-8 rounded-lg">
                    <AvatarFallback className="rounded-lg bg-brand text-white">
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
                onSelect={() => setTimeout(() => setOpen(true), 0)}
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

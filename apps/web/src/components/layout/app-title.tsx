import { Link } from "@tanstack/react-router";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";

const BRAND_MARK_SRC = "/images/brand/jushu-platform-mark.png";

export function AppTitle() {
  const { setOpenMobile } = useSidebar();

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <SidebarMenuButton
          size="lg"
          asChild
          className="h-[4.5rem] gap-3 px-2.5 hover:bg-transparent group-data-[collapsible=icon]:h-10 group-data-[collapsible=icon]:px-0"
        >
          <Link
            to="/"
            aria-label="炬枢平台 - 新炬网络"
            onClick={() => setOpenMobile(false)}
          >
            <span className="flex size-14 shrink-0 items-center justify-center overflow-hidden group-data-[collapsible=icon]:size-10">
              <img
                src={BRAND_MARK_SRC}
                alt=""
                aria-hidden="true"
                className="h-14 w-14 object-contain opacity-100 drop-shadow-none dark:brightness-[1.12] dark:saturate-[1.08] group-data-[collapsible=icon]:h-10 group-data-[collapsible=icon]:w-10"
              />
            </span>
            <span className="flex min-w-0 flex-col leading-none group-data-[collapsible=icon]:hidden">
              <span className="truncate text-[1.25rem] font-semibold leading-[1.1] text-v3-brand-deep">
                炬枢平台
              </span>
              <span className="mt-1.5 text-[0.82rem] font-medium leading-none text-v3-ink-2">
                新炬网络
              </span>
            </span>
          </Link>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}

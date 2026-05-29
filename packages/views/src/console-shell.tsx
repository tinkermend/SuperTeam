import type { ComponentType, ReactNode, SVGProps } from "react";

import { IconBadge, StatusPill } from "@superteam/ui";

type ShellIcon = ComponentType<SVGProps<SVGSVGElement>>;

function cn(...classNames: Array<string | false | null | undefined>) {
  return classNames.filter(Boolean).join(" ");
}

export type ConsoleNavItem = {
  active?: boolean;
  href: string;
  icon?: ShellIcon;
  label: string;
};

export type ConsoleShellProps = {
  activeWorkspace: string;
  children: ReactNode;
  navItems: ConsoleNavItem[];
  pageActions?: ReactNode;
  pageDescription?: ReactNode;
  pageTitle: string;
  productName: string;
  tenantName: string;
  user: {
    name: string;
    role: string;
  };
};

function UserInitial({ name }: { name: string }) {
  return (
    <span className="inline-flex size-9 shrink-0 items-center justify-center rounded-full bg-muted text-sm font-semibold text-foreground">
      {name.slice(0, 1)}
    </span>
  );
}

export function ConsoleShell({
  activeWorkspace,
  children,
  navItems,
  pageActions,
  pageDescription,
  pageTitle,
  productName,
  tenantName,
  user,
}: ConsoleShellProps) {
  return (
    <div className="min-h-screen w-full max-w-full overflow-x-hidden bg-background text-foreground">
      <div className="grid min-h-screen w-full max-w-full grid-cols-1 lg:grid-cols-[264px_minmax(0,1fr)]">
        <aside className="hidden min-h-screen flex-col border-r bg-sidebar px-4 pb-14 pt-5 text-sidebar-foreground lg:flex">
          <div className="flex items-center gap-3 px-2">
            <IconBadge label={`${productName} 标识`} tone="accent">
              <span aria-hidden="true" className="text-sm font-semibold">
                ST
              </span>
            </IconBadge>
            <div className="min-w-0">
              <p className="truncate text-sm font-semibold">{productName}</p>
              <p className="truncate text-xs text-muted-foreground">数字员工控制平面</p>
            </div>
          </div>

          <nav aria-label="主导航" className="mt-8 flex flex-1 flex-col gap-1">
            {navItems.map((item) => {
              const Icon = item.icon;

              return (
                <a
                  aria-current={item.active ? "page" : undefined}
                  className={cn(
                    "flex h-10 items-center gap-3 rounded-md px-3 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
                    item.active
                      ? "bg-sidebar-accent text-sidebar-accent-foreground"
                      : "text-muted-foreground hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
                  )}
                  href={item.href}
                  key={item.label}
                >
                  {Icon ? <Icon aria-hidden="true" className="shrink-0" /> : null}
                  <span className="truncate">{item.label}</span>
                </a>
              );
            })}
          </nav>

          <div className="flex flex-col gap-3">
            <section className="rounded-md border bg-card p-3 text-card-foreground">
              <p className="text-xs text-muted-foreground">当前租户</p>
              <p className="mt-1 truncate text-sm font-medium">{tenantName}</p>
              <div className="mt-3">
                <StatusPill tone="success">控制台在线</StatusPill>
              </div>
            </section>
            <section className="flex items-center gap-3 rounded-md border bg-card p-3 text-card-foreground">
              <UserInitial name={user.name} />
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">{user.name}</p>
                <p className="truncate text-xs text-muted-foreground">{user.role}</p>
              </div>
            </section>
          </div>
        </aside>

        <div className="min-w-0 max-w-full overflow-x-hidden">
          <header className="sticky top-0 z-10 max-w-full overflow-x-hidden border-b bg-background/90 backdrop-blur">
            <div className="flex min-h-16 items-center gap-4 px-4 lg:px-6">
              <div className="min-w-0">
                <p className="text-xs text-muted-foreground">{activeWorkspace}</p>
                <h1 className="truncate text-lg font-semibold">{pageTitle}</h1>
              </div>
              <div className="ml-auto flex min-w-0 items-center gap-3">
                <label className="relative hidden min-w-64 sm:block">
                  <span className="sr-only">全局搜索</span>
                  <input
                    aria-label="全局搜索"
                    className="h-9 w-full rounded-md border bg-background px-3 text-sm outline-none transition-colors placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring"
                    placeholder="搜索任务、工件、员工、流程..."
                    type="search"
                  />
                </label>
                {pageActions}
                <UserInitial name={user.name} />
              </div>
            </div>
            <div className="w-full max-w-full overflow-hidden border-t lg:hidden">
              <nav aria-label="移动主导航" className="flex w-full min-w-0 gap-2 overflow-x-auto px-4 py-2">
                {navItems.map((item) => (
                  <a
                    aria-current={item.active ? "page" : undefined}
                    className={cn(
                      "shrink-0 rounded-md px-3 py-1.5 text-sm font-medium",
                      item.active ? "bg-accent text-accent-foreground" : "text-muted-foreground",
                    )}
                    href={item.href}
                    key={item.label}
                  >
                    {item.label}
                  </a>
                ))}
              </nav>
            </div>
          </header>

          <main className="mx-auto flex w-full min-w-0 max-w-7xl flex-col gap-5 overflow-x-hidden px-4 py-5 lg:px-6">
            {pageDescription ? (
              <p className="max-w-3xl break-all text-sm text-muted-foreground sm:break-normal">{pageDescription}</p>
            ) : null}
            {children}
          </main>
        </div>
      </div>
    </div>
  );
}

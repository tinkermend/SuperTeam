import { createFileRoute, Link, Outlet, useRouterState } from "@tanstack/react-router";
import { ScrollText } from "lucide-react";
import { cn } from "@/lib/utils";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";

export const Route = createFileRoute("/_authenticated/logs")({
  component: LogsLayout
});

const tabItems = [
  { label: "登录日志", to: "/logs/login", value: "login" },
  { label: "操作日志", to: "/logs/operation", value: "operation" },
  { label: "平台事件", to: "/logs/runtime", value: "runtime" },
  { label: "消息投递", to: "/logs/delivery", value: "delivery" },
] as const;

function LogsLayout() {
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const activeValue = tabItems.find((t) => pathname.startsWith(t.to))?.value ?? "login";

  return (
    <>
      <ShellPageHeader
        icon={<ScrollText />}
        iconTone="mute"
        title="日志管理"
        subtitle="登录审计、操作追溯、平台事件与消息投递"
      />
      <Main width="canvas" className="min-w-0 overflow-x-hidden">
        <div className="flex w-full flex-col gap-4 text-ink">
          <nav className="h-auto max-w-full flex-wrap justify-start gap-1 overflow-x-auto rounded-[14px] bg-card p-1.5 shadow-card flex">
            {tabItems.map((tab) => (
              <Link
                key={tab.value}
                to={tab.to}
                className={cn(
                  "h-9 flex-none rounded-[10px] border-0 px-4 py-2 text-[13px] font-semibold shadow-none transition-colors hover:bg-card-soft hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/60",
                  activeValue === tab.value
                    ? "bg-brand-soft text-brand-deep"
                    : "text-ink-2"
                )}
              >
                {tab.label}
              </Link>
            ))}
          </nav>
          <Outlet />
        </div>
      </Main>
    </>
  );
}

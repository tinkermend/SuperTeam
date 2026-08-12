import { createFileRoute, Link, Outlet, useRouterState } from "@tanstack/react-router";
import { ScrollText } from "lucide-react";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import { PageTab, PageTabList, PageTabs } from "@/components/superteam";

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
        subtitle="最近的登录、控制台变更、节点事件与消息投递"
      />
      <Main width="wide" className="min-w-0 overflow-x-hidden">
        <div className="flex w-full flex-col gap-4 text-ink">
          <PageTabs aria-label="日志类型">
            <PageTabList>
              {tabItems.map((tab) => (
                <PageTab active={activeValue === tab.value} asChild key={tab.value}>
                  <Link to={tab.to}>{tab.label}</Link>
                </PageTab>
              ))}
            </PageTabList>
          </PageTabs>
          <Outlet />
        </div>
      </Main>
    </>
  );
}

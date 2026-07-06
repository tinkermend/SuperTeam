import { createFileRoute, Outlet, useNavigate, useRouterState } from "@tanstack/react-router";
import { ScrollText } from "lucide-react";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

export const Route = createFileRoute("/_authenticated/logs")({
  component: LogsLayout,
});

const tabItems = [
  { label: "登录日志", to: "/logs/login", value: "login" },
  { label: "操作日志", to: "/logs/operation", value: "operation" },
  { label: "平台事件", to: "/logs/runtime", value: "runtime" },
] as const;

function LogsLayout() {
  const navigate = useNavigate();
  const pathname = useRouterState({ select: (s) => s.location.pathname });
  const activeTab = tabItems.find((t) => pathname.startsWith(t.to))?.value ?? "login";

  return (
    <>
      <ShellPageHeader
        icon={<ScrollText />}
        iconTone="mute"
        title="日志管理"
        subtitle="登录审计、操作追溯与平台事件"
      />
      <Main className="min-w-0 overflow-x-hidden">
        <div className="mx-auto flex w-full max-w-6xl flex-col gap-4 text-v3-ink">
          <Tabs
            value={activeTab}
            onValueChange={(v) => {
              const tab = tabItems.find((t) => t.value === v);
              if (tab) navigate({ to: tab.to });
            }}
            className="gap-4"
          >
            <TabsList className="h-auto max-w-full flex-wrap justify-start gap-1 overflow-x-auto rounded-[14px] bg-v3-card p-1.5 text-v3-ink-2 shadow-v3">
              {tabItems.map((tab) => (
                <TabsTrigger
                  key={tab.value}
                  value={tab.value}
                  className="h-9 flex-none rounded-[10px] border-0 px-4 py-2 text-[13px] font-semibold text-v3-ink-2 shadow-none transition-colors hover:bg-v3-card-soft hover:text-v3-ink focus-visible:ring-v3-brand/60 focus-visible:ring-offset-v3-bg data-[state=active]:bg-v3-brand-soft data-[state=active]:text-v3-brand-deep data-[state=active]:shadow-none"
                >
                  {tab.label}
                </TabsTrigger>
              ))}
            </TabsList>
          </Tabs>
          <Outlet />
        </div>
      </Main>
    </>
  );
}

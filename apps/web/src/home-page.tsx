"use client";

import { ConsoleHealthView } from "@superteam/views";
import {
  Bell,
  Bot,
  BriefcaseBusiness,
  CalendarCheck,
  CheckCircle2,
  ClipboardCheck,
  Database,
  Factory,
  FileClock,
  Gauge,
  GitFork,
  Home,
  LayoutGrid,
  LockKeyhole,
  Network,
  Search,
  Settings,
  ShieldCheck,
  Sparkles,
  UserRound,
  Workflow,
} from "lucide-react";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Separator } from "@/components/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";

type HomePageProps = {
  summary: string;
};

const navigationItems = [
  { label: "首页", icon: Home, active: true },
  { label: "工作台", icon: LayoutGrid },
  { label: "任务中心", icon: CalendarCheck },
  { label: "数字员工", icon: Bot },
  { label: "流程编排", icon: Workflow },
  { label: "工作库", icon: BriefcaseBusiness },
  { label: "MCP 网关", icon: Network },
  { label: "审批中心", icon: ShieldCheck },
  { label: "审计日志", icon: FileClock },
  { label: "设置中心", icon: Settings },
];

const quickStats = [
  { label: "任务输入", value: "待接入", icon: ClipboardCheck },
  { label: "审批策略", value: "待配置", icon: LockKeyhole },
  { label: "Runtime 节点", value: "待注册", icon: Gauge },
  { label: "工件回传", value: "待开发", icon: Database },
];

export function HomePage({ summary }: HomePageProps) {
  return (
    <div className="dark min-h-screen bg-background text-foreground">
      <div className="grid min-h-screen grid-cols-[260px_minmax(0,1fr)]">
        <aside className="flex min-h-screen flex-col border-r border-sidebar-border bg-sidebar px-4 py-5 text-sidebar-foreground">
          <div className="flex items-center gap-3 px-2">
            <div className="flex size-10 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground">
              <Sparkles aria-hidden="true" />
            </div>
            <div className="min-w-0">
              <p className="truncate text-sm font-semibold">Super Team</p>
              <p className="truncate text-xs text-muted-foreground">数字员工平台</p>
            </div>
          </div>

          <nav aria-label="主导航" className="mt-8 flex flex-1 flex-col gap-1">
            {navigationItems.map((item) => (
              <Button
                key={item.label}
                aria-current={item.active ? "page" : undefined}
                className="h-11 justify-start gap-3 px-3"
                variant={item.active ? "secondary" : "ghost"}
              >
                <item.icon data-icon="inline-start" aria-hidden="true" />
                {item.label}
              </Button>
            ))}
          </nav>

          <div className="flex flex-col gap-3">
            <div className="px-2">
              <p className="text-xs text-muted-foreground">当前租户</p>
              <p className="mt-1 truncate text-sm font-medium">示例科技有限公司</p>
            </div>
            <Card size="sm" className="bg-card/60">
              <CardContent className="flex items-center gap-3">
                <Avatar>
                  <AvatarFallback>张</AvatarFallback>
                </Avatar>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-sm font-medium">张伟</p>
                  <p className="truncate text-xs text-muted-foreground">平台管理员</p>
                </div>
                <Badge variant="secondary">在线</Badge>
              </CardContent>
            </Card>
          </div>
        </aside>

        <main className="min-w-0 bg-background">
          <header className="sticky top-0 flex h-18 items-center gap-4 border-b bg-background/90 px-6 backdrop-blur">
            <h1 className="text-lg font-semibold">首页</h1>
            <div className="ml-auto flex items-center gap-3">
              <div className="relative w-[min(36vw,360px)]">
                <Search className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
                <Input
                  aria-label="全局搜索"
                  className="pl-9"
                  placeholder="搜索任务、工件、员工、流程..."
                  type="search"
                />
              </div>
              <Separator orientation="vertical" className="h-8" />
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button aria-label="通知" size="icon" variant="ghost">
                    <Bell aria-hidden="true" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>通知</TooltipContent>
              </Tooltip>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button aria-label="能力面板" size="icon" variant="ghost">
                    <GitFork aria-hidden="true" />
                  </Button>
                </TooltipTrigger>
                <TooltipContent>能力面板</TooltipContent>
              </Tooltip>
              <Avatar>
                <AvatarFallback>张</AvatarFallback>
              </Avatar>
            </div>
          </header>

          <div className="mx-auto flex max-w-7xl flex-col gap-5 px-6 py-6">
            <Card>
              <CardHeader>
                <CardTitle>
                  <h2>晚上好，张伟</h2>
                </CardTitle>
                <CardDescription>当前外部系统骨架已就绪，业务页面将按主链路逐步接入。</CardDescription>
                <CardAction>
                  <Badge variant="outline">全部项目</Badge>
                </CardAction>
              </CardHeader>
              <CardContent className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
                {quickStats.map((stat) => (
                  <Card key={stat.label} size="sm" className="bg-muted/30">
                    <CardContent className="flex items-center gap-3">
                      <div className="flex size-9 items-center justify-center rounded-lg bg-secondary text-secondary-foreground">
                        <stat.icon aria-hidden="true" />
                      </div>
                      <div>
                        <p className="text-sm text-muted-foreground">{stat.label}</p>
                        <p className="mt-1 text-lg font-semibold">{stat.value}</p>
                      </div>
                    </CardContent>
                  </Card>
                ))}
              </CardContent>
            </Card>

            <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
              <Card className="min-h-[420px]">
                <CardHeader>
                  <CardTitle>
                    <h2>页面正在开发中</h2>
                  </CardTitle>
                  <CardDescription>
                    这里会承载需求分析、人类确认、Runtime Agent 执行、工件回传和验收主链路。
                  </CardDescription>
                </CardHeader>
                <CardContent className="flex min-h-[300px] flex-col items-center justify-center gap-4 rounded-lg border border-dashed bg-muted/20 text-center">
                  <div className="flex size-14 items-center justify-center rounded-xl bg-secondary text-secondary-foreground">
                    <Factory aria-hidden="true" />
                  </div>
                  <div className="max-w-md">
                    <p className="text-base font-medium">标准空页面</p>
                    <p className="mt-2 text-sm text-muted-foreground">
                      左侧导航和顶部操作区已按外部系统骨架搭建，后续可在此接入具体业务模块。
                    </p>
                  </div>
                  <Button variant="outline">
                    <CheckCircle2 data-icon="inline-start" aria-hidden="true" />
                    查看建设清单
                  </Button>
                </CardContent>
              </Card>

              <div className="flex flex-col gap-5">
                <ConsoleHealthView summary={summary} />
                <Card>
                  <CardHeader>
                    <CardTitle>
                      <h2>近期活动</h2>
                    </CardTitle>
                    <CardDescription>等待真实审计事件接入。</CardDescription>
                  </CardHeader>
                  <CardContent className="flex flex-col gap-3">
                    {["需求澄清确认", "Runtime 节点注册", "工件归档策略"].map((label) => (
                      <div key={label} className="flex items-center gap-3 rounded-lg bg-muted/30 p-3">
                        <UserRound aria-hidden="true" className="text-muted-foreground" />
                        <div className="min-w-0 flex-1">
                          <p className="truncate text-sm font-medium">{label}</p>
                          <p className="text-xs text-muted-foreground">待开发</p>
                        </div>
                      </div>
                    ))}
                  </CardContent>
                </Card>
              </div>
            </div>
          </div>
        </main>
      </div>
    </div>
  );
}

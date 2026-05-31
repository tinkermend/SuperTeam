import type { LucideIcon } from "lucide-react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";

type UnimplementedPageProps = {
  description: string;
  icon: LucideIcon;
  title: string;
};

export function UnimplementedPage({ description, icon: Icon, title }: UnimplementedPageProps) {
  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        <div className="mb-4 flex items-center gap-3">
          <div className="flex size-10 items-center justify-center rounded-md border bg-muted">
            <Icon />
          </div>
          <div>
            <h1 className="text-2xl font-bold tracking-tight">{title}</h1>
            <p className="text-sm text-muted-foreground">{description}</p>
          </div>
        </div>
        <Card>
          <CardHeader>
            <CardTitle>功能建设中</CardTitle>
            <CardDescription>当前页面只保留导航入口，不使用 mock 数据冒充真实业务能力。</CardDescription>
          </CardHeader>
          <CardContent className="text-sm text-muted-foreground">
            后续实现会从 Control Plane API 获取真实数据，并按任务、审批、审计和工件边界逐步接入。
          </CardContent>
        </Card>
      </Main>
    </>
  );
}

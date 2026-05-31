import { useQuery } from "@tanstack/react-query";
import { Activity, ShieldCheck, Users } from "lucide-react";
import { useAuth } from "@/features/auth/use-auth";
import { getHealth, listLoginLogs } from "@/lib/api";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";

const apiBaseUrl = resolveControlPlaneUrl();

export function Dashboard() {
  const { user } = useAuth();
  const healthQuery = useQuery({
    queryKey: ["control-plane-health"],
    queryFn: () => getHealth({ baseUrl: apiBaseUrl }),
  });
  const loginLogsQuery = useQuery({
    queryKey: ["login-logs", 5],
    queryFn: () => listLoginLogs({ baseUrl: apiBaseUrl, limit: 5, offset: 0 }),
  });

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        <div className="mb-4">
          <h1 className="text-2xl font-bold tracking-tight">工作台</h1>
          <p className="text-sm text-muted-foreground">欢迎回来，{user?.username ?? "用户"}。</p>
        </div>
        <div className="grid gap-4 md:grid-cols-3">
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <Activity /> Control Plane
              </CardTitle>
              <CardDescription>后端健康状态</CardDescription>
            </CardHeader>
            <CardContent>
              <Badge variant={healthQuery.data?.status === "ok" ? "default" : "secondary"}>
                {healthQuery.isLoading ? "检查中" : healthQuery.data?.status ?? "不可用"}
              </Badge>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <Users /> 当前用户
              </CardTitle>
              <CardDescription>来自 /api/auth/me</CardDescription>
            </CardHeader>
            <CardContent className="text-sm">{user?.username ?? "未登录"}</CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2 text-base">
                <ShieldCheck /> 登录日志
              </CardTitle>
              <CardDescription>来自 /api/auth/login-logs</CardDescription>
            </CardHeader>
            <CardContent className="text-sm">
              {loginLogsQuery.isLoading ? "加载中" : `${loginLogsQuery.data?.items.length ?? 0} 条最近记录`}
            </CardContent>
          </Card>
        </div>
      </Main>
    </>
  );
}

import { useQuery } from "@tanstack/react-query";
import { Activity, ShieldCheck, Users } from "lucide-react";
import { useAuth } from "@/features/auth/use-auth";
import { getHealth, listLoginLogs } from "@/lib/api";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { LiquidPill, MetricCard } from "@/components/superteam";
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

  const healthStatus = healthQuery.isLoading
    ? "检查中"
    : healthQuery.data?.status ?? "不可用";
  const loginLogCount = loginLogsQuery.data?.items.length ?? 0;

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        <div className="mb-6 flex flex-col gap-2">
          <LiquidPill className="w-fit px-3 py-1 text-primary">
            SuperTeam Console
          </LiquidPill>
          <h1 className="text-4xl font-bold tracking-normal text-foreground">
            工作台
          </h1>
          <p className="text-sm text-muted-foreground">
            欢迎回来，{user?.username ?? "用户"}。
          </p>
        </div>
        <div className="grid gap-4 md:grid-cols-3">
          <MetricCard
            title="Control Plane"
            description="后端健康状态"
            icon={<Activity />}
            iconTone="success"
            value={healthStatus}
            meta={healthQuery.data?.status === "ok" ? "健康" : "状态"}
            statusTone={healthQuery.data?.status === "ok" ? "success" : "neutral"}
          />
          <MetricCard
            title="当前用户"
            description="来自 /api/auth/me"
            icon={<Users />}
            iconTone="info"
            value={user?.username ?? "未登录"}
            meta={user?.username ? "已登录" : "未认证"}
            statusTone={user?.username ? "decision" : "neutral"}
          />
          <MetricCard
            title="登录日志"
            description="来自 /api/auth/login-logs"
            icon={<ShieldCheck />}
            iconTone="artifact"
            value={
              loginLogsQuery.isLoading
                ? "加载中"
                : loginLogsQuery.isError
                  ? "失败"
                  : String(loginLogCount)
            }
            meta={
              loginLogsQuery.isError
                ? "登录日志加载失败"
                : loginLogsQuery.isLoading
                  ? "正在读取"
                  : "条最近记录"
            }
            statusTone={loginLogsQuery.isError ? "danger" : "artifact"}
            isError={loginLogsQuery.isError}
          />
        </div>
      </Main>
    </>
  );
}

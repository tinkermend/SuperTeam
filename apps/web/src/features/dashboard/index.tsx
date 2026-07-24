import { useQuery } from "@tanstack/react-query";
import { Activity, Clock3, LayoutDashboard, LogIn, ShieldCheck, Users } from "lucide-react";
import { useAuth } from "@/features/auth/use-auth";
import type { LoginLogRecord } from "@/lib/api";
import { getHealth, listLoginLogs } from "@/lib/api";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import {
  IconTile,
  SignatureCard,
  StatusPill,
  EmptyState,
  ErrorState,
  LoadingState,
  MetricCard,
  DataTable,
  Td,
  Th,
  Tr,
  WorkSurface
} from "@/components/superteam";

const apiBaseUrl = resolveControlPlaneUrl();

export function Dashboard() {
  const { user } = useAuth();
  const healthQuery = useQuery({
    queryKey: ["control-plane-health"],
    queryFn: () => getHealth({ baseUrl: apiBaseUrl })
});
  const loginLogsQuery = useQuery({
    queryKey: ["login-logs", 5],
    queryFn: () => listLoginLogs({ baseUrl: apiBaseUrl, limit: 5, offset: 0 })
});

  const healthStatus = healthQuery.isLoading
    ? "检查中"
    : healthQuery.isError
      ? "不可用"
    : healthQuery.data?.status ?? "不可用";
  const loginLogs = loginLogsQuery.data?.items ?? [];
  const loginLogCount = loginLogs.length;
  const failedLoginCount = loginLogs.filter((record) => record.result === "failed").length;
  const displayName = user?.display_name || user?.username || "用户";

  return (
    <>
      <ShellPageHeader
        title="工作台"
        subtitle={`欢迎回来，${displayName}。`}
        icon={<LayoutDashboard />}
        iconTone="brand"
        actions={
          <StatusPill tone={healthQuery.data?.status === "ok" ? "ok" : "mute"}>
            Control Plane {healthStatus}
          </StatusPill>
        }
      />
      <Main width="wide" className="space-y-6 text-ink">
        <div className="grid gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(18rem,0.85fr)]">
          <SignatureCard className="min-h-[14rem]">
            <div className="flex h-full flex-col justify-between gap-8">
              <div className="flex items-center justify-between gap-3">
                <span className="text-sm font-bold text-white/80">炬枢控制台</span>
                <span className="rounded-lg bg-white/16 px-2.5 py-1 text-xs font-bold text-white">
                  v3 Soft-Flat
                </span>
              </div>
              <div>
                <p className="text-4xl font-extrabold tracking-tight md:text-5xl">
                  AI 执行控制台
                </p>
                <p className="mt-3 max-w-2xl text-sm leading-6 text-white/78">
                  聚合控制平面健康、账号活动和当前操作者状态，保持所有入口从真实服务读取。
                </p>
              </div>
            </div>
          </SignatureCard>

          <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-1">
            <MetricCard
              label="Control Plane"
              icon={<Activity />}
              iconTone={healthQuery.data?.status === "ok" ? "ok" : "mute"}
              value={healthStatus}
              meta={healthQuery.data?.status === "ok" ? "后端健康" : "状态"}
              loud={healthQuery.isError}
            />
            <MetricCard
              label="当前用户"
              icon={<Users />}
              iconTone="info"
              value={user?.username ?? "未登录"}
              meta={user?.username ? "来自 /api/auth/me" : "未认证"}
            />
          </div>
        </div>

        <div className="grid gap-4 lg:grid-cols-3">
          <MetricCard
            label="最近登录"
            icon={<ShieldCheck />}
            iconTone={loginLogsQuery.isError ? "danger" : "artifact"}
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
                  ? "正在读取 /api/auth/login-logs"
                  : "条最近记录"
            }
            loud={loginLogsQuery.isError}
          />
          <MetricCard
            label="失败登录"
            icon={<LogIn />}
            iconTone={failedLoginCount > 0 ? "danger" : "ok"}
            value={loginLogsQuery.isLoading ? "加载中" : String(failedLoginCount)}
            meta="最近 5 条登录日志内"
            loud={failedLoginCount > 0}
          />
          <MetricCard
            label="当前身份"
            icon={<Clock3 />}
            iconTone="brand"
            value={user?.status === "active" ? "active" : user?.status ?? "unknown"}
            meta={user?.email || user?.username || "无账号信息"}
          />
        </div>

        <WorkSurface>
          <div className="flex flex-col gap-3 border-b border-line px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h2 className="text-lg font-extrabold tracking-tight text-ink">
                最近登录日志
              </h2>
              <p className="mt-1 text-sm text-ink-2">
                来自 /api/auth/login-logs，展示最近 5 条账号活动。
              </p>
            </div>
            <IconTile tone="artifact" size="sm">
              <ShieldCheck />
            </IconTile>
          </div>
          {loginLogsQuery.isLoading ? (
            <LoadingState label="加载登录日志…" />
          ) : loginLogsQuery.isError ? (
            <div className="p-5">
              <ErrorState title="登录日志加载失败" description="请检查 Auth API 连接。" />
            </div>
          ) : loginLogs.length === 0 ? (
            <EmptyState title="暂无登录日志" description="当前账号还没有可展示的登录记录。" />
          ) : (
            <DataTable>
              <thead>
                <tr>
                  <Th>事件</Th>
                  <Th>账号</Th>
                  <Th>来源</Th>
                  <Th>时间</Th>
                </tr>
              </thead>
              <tbody>
                {loginLogs.map((record) => (
                  <LoginLogRow key={record.id} record={record} />
                ))}
              </tbody>
            </DataTable>
          )}
        </WorkSurface>
      </Main>
    </>
  );
}

function LoginLogRow({ record }: { record: LoginLogRecord }) {
  const failed = record.result === "failed";
  return (
    <Tr tone={failed ? "danger" : undefined}>
      <Td>
        <StatusPill tone={failed ? "danger" : "ok"}>
          {formatLoginEvent(record.event_type)}
        </StatusPill>
      </Td>
      <Td>
        <span className="font-semibold text-ink">{record.username}</span>
      </Td>
      <Td className="max-w-[20rem] text-ink-2">
        <span className="block truncate">{record.user_agent ?? "未知设备"}</span>
        <span className="font-mono text-xs text-ink-3">{record.client_ip ?? "未知 IP"}</span>
      </Td>
      <Td className="whitespace-nowrap text-ink-2 tabular-nums">
        <time dateTime={record.created_at}>{formatDateTime(record.created_at)}</time>
      </Td>
    </Tr>
  );
}

function formatLoginEvent(eventType: LoginLogRecord["event_type"]) {
  if (eventType === "login_succeeded") {
    return "登录成功";
  }
  if (eventType === "logout_succeeded") {
    return "退出登录";
  }
  return "登录失败";
}

function formatDateTime(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "2-digit",
    year: "numeric"
}).format(new Date(value));
}

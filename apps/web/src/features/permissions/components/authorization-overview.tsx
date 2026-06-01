import { useQuery } from "@tanstack/react-query";
import { Gauge, ShieldAlert, ShieldCheck, Sigma } from "lucide-react";
import type { ApiClientOptions, AuthzDecisionRecord } from "@/lib/api";
import { getAuthzOverview } from "@/lib/api";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

type AuthorizationOverviewProps = {
  apiOptions: ApiClientOptions;
};

export function AuthorizationOverview({ apiOptions }: AuthorizationOverviewProps) {
  const overviewQuery = useQuery({
    queryKey: ["authz-overview", apiOptions.baseUrl],
    queryFn: () => getAuthzOverview(apiOptions),
  });

  if (overviewQuery.isLoading) {
    return (
      <div className="grid gap-4 md:grid-cols-4">
        {Array.from({ length: 4 }).map((_, index) => (
          <Skeleton key={index} className="h-28" />
        ))}
      </div>
    );
  }

  if (overviewQuery.isError) {
    return (
      <Alert variant="destructive">
        <ShieldAlert />
        <AlertTitle>授权概览加载失败</AlertTitle>
        <AlertDescription>请稍后刷新或检查 Control Plane 连接。</AlertDescription>
      </Alert>
    );
  }

  const overview = overviewQuery.data;

  if (!overview) {
    return <p className="text-sm text-muted-foreground">暂无授权概览。</p>;
  }

  const metricCards = [
    {
      title: "授权引擎",
      value: overview.engine.engine,
      description: overview.engine.status,
      icon: ShieldCheck,
    },
    {
      title: "总决策",
      value: formatNumber(overview.totals.total),
      description: "全部授权决策",
      icon: Sigma,
    },
    {
      title: "拒绝次数",
      value: formatNumber(overview.totals.denied),
      description: `${formatNumber(overview.totals.allowed)} 次允许`,
      icon: ShieldAlert,
    },
    {
      title: "拒绝率",
      value: formatRate(overview.totals.denied_rate),
      description: "Denied / Total",
      icon: Gauge,
    },
  ];

  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        {metricCards.map((metric) => {
          const Icon = metric.icon;

          return (
            <Card key={metric.title}>
              <CardHeader>
                <CardTitle className="flex items-center gap-2 text-base">
                  <Icon />
                  {metric.title}
                </CardTitle>
                <CardDescription>{metric.description}</CardDescription>
              </CardHeader>
              <CardContent>
                <div className="text-2xl font-semibold">{metric.value}</div>
              </CardContent>
            </Card>
          );
        })}
      </div>
      <Card>
        <CardHeader>
          <CardTitle>最近授权事件</CardTitle>
          <CardDescription>授权引擎返回的最近决策记录。</CardDescription>
        </CardHeader>
        <CardContent>
          {overview.recent_events.length === 0 ? (
            <p className="text-sm text-muted-foreground">暂无最近授权事件。</p>
          ) : (
            <RecentEventsTable events={overview.recent_events} />
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function RecentEventsTable({ events }: { events: AuthzDecisionRecord[] }) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>时间</TableHead>
          <TableHead>结果</TableHead>
          <TableHead>动作</TableHead>
          <TableHead>资源</TableHead>
          <TableHead>原因</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {events.map((event) => (
          <TableRow key={event.id}>
            <TableCell>{formatTime(event.created_at)}</TableCell>
            <TableCell>
              <DecisionBadge result={event.result} />
            </TableCell>
            <TableCell>{event.action}</TableCell>
            <TableCell>{formatResource(event.resource_type, event.resource_id)}</TableCell>
            <TableCell className="max-w-64 truncate">{event.reason ?? "-"}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function DecisionBadge({ result }: { result: AuthzDecisionRecord["result"] }) {
  return <Badge variant={result === "succeeded" ? "default" : "destructive"}>{result === "succeeded" ? "允许" : "拒绝"}</Badge>;
}

function formatNumber(value: number) {
  return new Intl.NumberFormat("zh-CN").format(value);
}

function formatRate(value: number) {
  return `${(value * 100).toFixed(1)}%`;
}

function formatTime(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "2-digit",
  }).format(new Date(value));
}

function formatResource(type?: string | null, id?: string | null) {
  if (!type && !id) {
    return "-";
  }

  return [type, id].filter(Boolean).join(":");
}

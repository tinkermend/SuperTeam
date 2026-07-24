import { useQuery } from "@tanstack/react-query";
import { Gauge, ShieldAlert, ShieldCheck, Sigma } from "lucide-react";
import type { ApiClientOptions, AuthzDecisionRecord } from "@/lib/api";
import { getAuthzOverview } from "@/lib/api";
import {
  IconTile,
  StatusPill,
  EmptyState,
  ErrorState,
  LoadingState,
  MetricCard,
  DataTable,
  Td,
  Th,
  Tr,
  WorkSurface,
  type Tone
} from "@/components/superteam";
import { statusLabel } from "@/lib/status-labels";

type AuthorizationOverviewProps = {
  apiOptions: ApiClientOptions;
};

export function AuthorizationOverview({ apiOptions }: AuthorizationOverviewProps) {
  const overviewQuery = useQuery({
    queryKey: ["authz-overview", apiOptions.baseUrl],
    queryFn: () => getAuthzOverview(apiOptions)
});

  if (overviewQuery.isLoading) {
    return (
      <div className="grid gap-4 md:grid-cols-4">
        {Array.from({ length: 4 }).map((_, index) => (
          <WorkSurface key={index}>
            <LoadingState className="py-9" label="加载授权概览…" />
          </WorkSurface>
        ))}
      </div>
    );
  }

  if (overviewQuery.isError) {
    return (
      <ErrorState
        title="授权概览加载失败"
        description="请稍后刷新或检查 Control Plane 连接。"
      />
    );
  }

  const overview = overviewQuery.data;

  if (!overview) {
    return (
      <WorkSurface>
        <EmptyState icon={<ShieldAlert />} title="暂无授权概览" description="授权引擎尚未返回概览数据。" />
      </WorkSurface>
    );
  }

  const metricCards: Array<{
    description: string;
    details?: Array<{ label: string; value: string }>;
    icon: typeof ShieldCheck;
    iconTone: Tone;
    loud?: boolean;
    title: string;
    value: string;
  }> = [
    {
      title: "授权引擎",
      value: overview.engine.engine,
      description: engineStatusDescription(overview.engine),
      icon: ShieldCheck,
      iconTone: overview.engine.status === "ok" ? "ok" : "warn",
      details: engineDetails(overview.engine)
},
    {
      title: "总决策",
      value: formatNumber(overview.totals.total),
      description: "全部授权决策",
      icon: Sigma,
      iconTone: "info"
},
    {
      title: "拒绝次数",
      value: formatNumber(overview.totals.denied),
      description: `${formatNumber(overview.totals.allowed)} 次允许`,
      icon: ShieldAlert,
      iconTone: overview.totals.denied > 0 ? "danger" : "ok",
      loud: overview.totals.denied > 0
},
    {
      title: "拒绝率",
      value: formatRate(overview.totals.denied_rate),
      description: "Denied / Total",
      icon: Gauge,
      iconTone: overview.totals.denied_rate > 0 ? "warn" : "ok",
      loud: overview.totals.denied_rate > 0
},
  ];

  return (
    <div className="flex flex-col gap-4">
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        {metricCards.map((metric) => {
          const Icon = metric.icon;

          return (
            <MetricCard
              key={metric.title}
              icon={<Icon />}
              iconTone={metric.iconTone}
              label={metric.title}
              value={metric.value}
              loud={metric.loud}
              meta={
                metric.details ? (
                  <>
                    {metric.description}
                    {metric.details.map((detail) => (
                      <span key={detail.label} className="block max-w-full truncate font-mono">
                        {detail.label}: {detail.value}
                      </span>
                    ))}
                  </>
                ) : (
                  metric.description
                )
              }
            />
          );
        })}
      </div>
      <WorkSurface>
        <div className="flex items-start gap-3 border-b border-line px-5 py-4">
          <IconTile tone="info" size="sm">
            <ShieldCheck />
          </IconTile>
          <div>
            <h2 className="text-base font-bold text-ink">最近授权事件</h2>
            <p className="mt-1 text-sm text-ink-2">授权引擎返回的最近决策记录。</p>
          </div>
        </div>
        {overview.recent_events.length === 0 ? (
          <EmptyState title="暂无最近授权事件" description="新的授权决策会显示在这里。" />
        ) : (
          <RecentEventsTable events={overview.recent_events} />
        )}
      </WorkSurface>
    </div>
  );
}

function RecentEventsTable({ events }: { events: AuthzDecisionRecord[] }) {
  return (
    <DataTable>
      <thead>
        <Tr>
          <Th>时间</Th>
          <Th>结果</Th>
          <Th>动作</Th>
          <Th>资源</Th>
          <Th>原因</Th>
        </Tr>
      </thead>
      <tbody>
        {events.map((event) => (
          <Tr key={event.id} tone={event.result === "succeeded" ? undefined : "danger"}>
            <Td className="whitespace-nowrap tabular-nums">{formatTime(event.created_at)}</Td>
            <Td>
              <DecisionBadge result={event.result} />
            </Td>
            <Td>{event.action}</Td>
            <Td>{formatResource(event.resource_type, event.resource_id)}</Td>
            <Td className="max-w-64 truncate">{event.reason ?? "-"}</Td>
          </Tr>
        ))}
      </tbody>
    </DataTable>
  );
}

function DecisionBadge({ result }: { result: AuthzDecisionRecord["result"] }) {
  return <StatusPill tone={result === "succeeded" ? "ok" : "danger"}>{result === "succeeded" ? "允许" : "拒绝"}</StatusPill>;
}

function formatNumber(value: number) {
  return new Intl.NumberFormat("zh-CN").format(value);
}

function formatRate(value: number) {
  return `${(value * 100).toFixed(1)}%`;
}

function engineStatusDescription(engine: { status: string; recent_diff_count: number }) {
  return `${statusLabel(engine.status)} · 近 24h diff ${formatNumber(engine.recent_diff_count)}`;
}

function engineDetails(engine: {
  engine_version?: string | null;
  openfga_store_id?: string | null;
  openfga_model_id?: string | null;
}) {
  return [
    { label: "版本", value: engine.engine_version ?? "-" },
    { label: "Store", value: engine.openfga_store_id ?? "-" },
    { label: "Model", value: engine.openfga_model_id ?? "-" },
  ];
}

function formatTime(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "2-digit"
}).format(new Date(value));
}

function formatResource(type?: string | null, id?: string | null) {
  if (!type && !id) {
    return "-";
  }

  return [type, id].filter(Boolean).join(":");
}

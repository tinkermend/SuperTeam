import { useState } from "react";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { createFileRoute, Link } from "@tanstack/react-router";
import {
  Bot,
  CircleDollarSign,
  Hash,
  Play,
  TrendingUp
} from "lucide-react";
import {
  SoftCard,
  StatusPill,
  EmptyState,
  MetricCard,
  Segmented,
  StateSurface,
  DataTable,
  Td,
  Th,
  Tr,
  WorkSurface
} from "@/components/superteam";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import type { CostPeriod, CostSummary } from "@/lib/api/costs";
import { getCostSummary } from "@/lib/api/costs";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { providerDisplayName } from "@/features/employees/provider-label";

export const Route = createFileRoute("/_authenticated/costs/")({
  component: CostsRoute
});

const PERIOD_OPTIONS: Array<{ label: string; value: CostPeriod }> = [
  { label: "7 天", value: "7d" },
  { label: "30 天", value: "30d" },
  { label: "90 天", value: "90d" },
];

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(2)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return String(n);
}

function tokenPercent(tokens: number, total: number): string {
  if (total === 0) return "0%";
  return `${((tokens / total) * 100).toFixed(1)}%`;
}

function CostsRoute() {
  const [period, setPeriod] = useState<CostPeriod>("30d");
  const apiBaseUrl = resolveControlPlaneUrl();

  const summaryQuery = useQuery({
    queryKey: ["costs-summary", period],
    queryFn: () => getCostSummary({ baseUrl: apiBaseUrl }, period),
    placeholderData: keepPreviousData,
    staleTime: 60 * 1000
});

  const summary: CostSummary | undefined = summaryQuery.data;

  const totalTokens = summary?.total_tokens ?? 0;
  const totalRuns = summary?.total_runs ?? 0;
  const employees = summary?.by_employee ?? [];
  const byProvider = summary?.by_provider ?? {};
  const providerKeys = Object.keys(byProvider).filter((k) => byProvider[k] > 0);

  const periodDays = period === "7d" ? 7 : period === "90d" ? 90 : 30;
  const dailyAvg = totalTokens > 0 ? Math.round(totalTokens / periodDays) : 0;
  const activeEmployees = employees.filter((e) => e.total_tokens > 0).length;

  return (
    <>
      <ShellPageHeader
        icon={<CircleDollarSign />}
        iconTone="warn"
        title="成本管理"
        subtitle="数字员工 Token 用量统计"
      />
      <Main width="wide" className="min-w-0 overflow-x-hidden">
        <div className="mx-auto flex w-full max-w-6xl flex-col gap-6">
      <div className="flex flex-wrap items-center justify-start gap-2 sm:justify-end">
        <Segmented<CostPeriod>
          options={PERIOD_OPTIONS}
          value={period}
          onChange={setPeriod}
        />
      </div>

      {/* 顶部指标卡 */}
      <section className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <MetricCard
          icon={<Hash />}
          iconTone="brand"
          label="总 Token 用量"
          value={formatTokens(totalTokens)}
          meta={`近 ${periodDays} 天`}
        />
        <MetricCard
          icon={<Play />}
          iconTone="info"
          label="总执行次数"
          value={totalRuns.toLocaleString()}
          meta={`近 ${periodDays} 天`}
        />
        <MetricCard
          icon={<Bot />}
          iconTone="ok"
          label="活跃员工数"
          value={activeEmployees}
          meta="有 Token 消耗"
        />
        <MetricCard
          icon={<TrendingUp />}
          iconTone="artifact"
          label="日均 Token"
          value={formatTokens(dailyAvg)}
          meta="平均每日消耗"
        />
      </section>

      {/* Provider 分布进度条 */}
      {providerKeys.length > 0 && (
        <SoftCard className="p-5">
          <p className="mb-4 text-[13px] font-semibold text-ink">
            Provider 分布
          </p>
          <div className="flex flex-col gap-3">
            {providerKeys
              .sort((a, b) => byProvider[b] - byProvider[a])
              .map((key) => {
                const tokens = byProvider[key];
                const pct = tokenPercent(tokens, totalTokens);
                const widthPct =
                  totalTokens > 0 ? (tokens / totalTokens) * 100 : 0;
                return (
                  <div key={key} className="flex flex-col gap-1">
                    <div className="flex items-center justify-between text-[13px]">
                      <span className="font-medium text-ink">
                        {providerDisplayName(key)}
                      </span>
                      <span className="tabular-nums text-ink-2">
                        {formatTokens(tokens)}{" "}
                        <span className="text-ink-3">({pct})</span>
                      </span>
                    </div>
                    <div className="h-1.5 w-full overflow-hidden rounded-full bg-card-soft">
                      <div
                        className="h-full rounded-full bg-brand transition-all"
                        style={{ width: `${widthPct}%` }}
                      />
                    </div>
                  </div>
                );
              })}
          </div>
        </SoftCard>
      )}

      {/* 员工 Token 明细表 */}
      <WorkSurface>
        <div className="flex items-center justify-between border-b border-line px-4 py-3">
          <p className="text-[13px] font-semibold text-ink">
            数字员工 Token 明细
          </p>
          <span className="text-xs text-ink-3">
            近 {periodDays} 天 · 按用量降序
          </span>
        </div>
        <StateSurface
          isLoading={summaryQuery.isPending}
          isError={summaryQuery.isError}
          error={summaryQuery.error}
          empty={employees.length === 0}
          emptyState={
            <EmptyState
              icon={<CircleDollarSign />}
              title="暂无执行数据"
              description="本周期内没有已完成的数字员工执行记录"
            />
          }
        >
          <DataTable>
            <thead>
              <tr>
                <Th>员工名称</Th>
                <Th>Provider</Th>
                <Th align="right">执行次数</Th>
                <Th align="right">Token 用量</Th>
                <Th align="right">占比</Th>
              </tr>
            </thead>
            <tbody>
              {employees.map((row) => (
                <Tr key={`${row.employee_id}-${row.provider_type}`}>
                  <Td>
                    <Link
                      to="/employees/$employeeId"
                      params={{ employeeId: row.employee_id }}
                      className="text-brand hover:underline"
                    >
                      {row.employee_name || row.employee_id.slice(0, 8)}
                    </Link>
                  </Td>
                  <Td>
                    <StatusPill tone="info">
                      {providerDisplayName(row.provider_type)}
                    </StatusPill>
                  </Td>
                  <Td align="right" className="tabular-nums">
                    {row.run_count.toLocaleString()}
                  </Td>
                  <Td align="right" className="tabular-nums font-medium">
                    {formatTokens(row.total_tokens)}
                  </Td>
                  <Td align="right" className="tabular-nums text-ink-2">
                    {tokenPercent(row.total_tokens, totalTokens)}
                  </Td>
                </Tr>
              ))}
            </tbody>
          </DataTable>
        </StateSurface>
      </WorkSurface>
        </div>
      </Main>
    </>
  );
}

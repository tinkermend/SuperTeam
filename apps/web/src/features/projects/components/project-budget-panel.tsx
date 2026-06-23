import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { BadgeDollarSign } from "lucide-react";
import {
  IconTile,
  SoftCard,
  StatusPill,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
} from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import type {
  ProjectBudgetLedgerEntry,
  ProjectBudgetSummary,
} from "@/lib/api/projects";
import {
  getProjectBudgetSummary,
  listProjectBudgetLedger,
} from "@/lib/api/projects";

type ProjectBudgetPanelProps = {
  budgetLedger?: ProjectBudgetLedgerEntry[];
  budgetSummary?: ProjectBudgetSummary;
};

type CostsProjectViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  projectId: string;
};

type ProjectBudgetLedgerResult = {
  projectId: string;
  ledger: ProjectBudgetLedgerEntry[];
};

type ProjectBudgetSummaryResult = {
  projectId: string;
  summary: ProjectBudgetSummary;
};

export function CostsProjectView({
  apiBaseUrl,
  fetcher,
  projectId,
}: CostsProjectViewProps) {
  const apiOptions: ApiClientOptions = { baseUrl: apiBaseUrl, fetcher };
  const ledgerQuery = useQuery({
    enabled: Boolean(projectId),
    queryKey: ["costs-project-budget-ledger", projectId],
    queryFn: async (): Promise<ProjectBudgetLedgerResult> => {
      const ledger = await listProjectBudgetLedger(apiOptions, projectId, {
        limit: 50,
      });
      return { projectId, ledger };
    },
    placeholderData: keepPreviousData,
  });
  const summaryQuery = useQuery({
    enabled: Boolean(projectId),
    queryKey: ["costs-project-budget-summary", projectId],
    queryFn: async (): Promise<ProjectBudgetSummaryResult> => {
      const summary = await getProjectBudgetSummary(apiOptions, projectId);
      return { projectId, summary };
    },
    placeholderData: keepPreviousData,
  });
  const ledgerData = ledgerQuery.data;
  const summaryData = summaryQuery.data;
  const currentLedger =
    ledgerData?.projectId === projectId ? ledgerData.ledger : undefined;
  const currentSummary =
    summaryData?.projectId === projectId ? summaryData.summary : undefined;
  const isInitialLoading =
    (ledgerQuery.isLoading || summaryQuery.isLoading) &&
    !currentLedger &&
    !currentSummary;
  const error = ledgerQuery.error ?? summaryQuery.error;

  if (isInitialLoading) {
    return (
      <SoftCard>
        <V3LoadingState label="正在加载项目成本数据..." />
      </SoftCard>
    );
  }

  if (error) {
    return (
      <V3ErrorState
        title="项目成本加载失败"
        description="请稍后重试，或确认当前账号仍有项目访问权限。"
      />
    );
  }

  return (
    <div className="space-y-3">
      {ledgerQuery.isFetching || summaryQuery.isFetching ? (
        <div className="flex justify-end">
          <StatusPill tone="info">刷新中</StatusPill>
        </div>
      ) : null}
      <ProjectBudgetPanel
        budgetLedger={currentLedger ?? []}
        budgetSummary={currentSummary}
      />
    </div>
  );
}

export function ProjectBudgetPanel({
  budgetLedger = [],
  budgetSummary,
}: ProjectBudgetPanelProps) {
  const summary = budgetSummary ?? {
    actual_cost: "0",
    actual_tokens: 0,
    estimated_cost: "0",
    estimated_tokens: 0,
    ledger_count: budgetLedger.length,
  };

  return (
    <div className="grid gap-4">
      <SoftCard className="overflow-hidden">
        <div className="flex items-center justify-between gap-3 border-b border-v3-line p-4">
          <div className="flex min-w-0 items-center gap-3">
            <IconTile tone="warn" size="sm">
              <BadgeDollarSign />
            </IconTile>
            <div className="min-w-0">
              <h3 className="font-semibold text-v3-ink">预算流水</h3>
              <p className="truncate text-xs text-v3-ink-2">
                Token、成本估算与实际消耗
              </p>
            </div>
          </div>
          <StatusPill tone="warn">{summary.ledger_count} 条</StatusPill>
        </div>

        <div className="grid gap-3 p-4 sm:grid-cols-2 xl:grid-cols-5">
          <MetricBlock label="预估 Token" value={formatNumber(summary.estimated_tokens)} />
          <MetricBlock label="实际 Token" value={formatNumber(summary.actual_tokens)} />
          <MetricBlock label="预估成本" value={formatCost(summary.estimated_cost)} />
          <MetricBlock label="实际成本" value={formatCost(summary.actual_cost)} />
          <MetricBlock label="流水数" value={formatNumber(summary.ledger_count)} />
        </div>
      </SoftCard>

      <WorkSurface>
        <V3Table>
          <thead>
            <tr>
              <V3Th>类型</V3Th>
              <V3Th>来源</V3Th>
              <V3Th>Token</V3Th>
              <V3Th>成本</V3Th>
              <V3Th className="min-w-[180px]">原因</V3Th>
            </tr>
          </thead>
          <tbody>
            {budgetLedger.length === 0 ? (
              <V3Tr>
                <V3Td colSpan={5}>
                  <V3EmptyState title="暂无预算流水" />
                </V3Td>
              </V3Tr>
            ) : (
              budgetLedger.map((entry) => (
                <V3Tr key={entry.id}>
                  <V3Td>
                    <StatusPill tone="mute">{entry.cost_type}</StatusPill>
                  </V3Td>
                  <V3Td className="text-v3-ink-2">{entry.source}</V3Td>
                  <V3Td>
                    <span className="font-mono text-xs text-v3-ink tabular-nums">
                      {formatOptionalNumber(entry.estimated_tokens)} /{" "}
                      {formatOptionalNumber(entry.actual_tokens)}
                    </span>
                  </V3Td>
                  <V3Td>
                    <span className="font-mono text-xs text-v3-ink tabular-nums">
                      {formatCost(entry.estimated_cost)} / {formatCost(entry.actual_cost)}
                    </span>
                  </V3Td>
                  <V3Td className="max-w-[260px] whitespace-normal">
                    <span className="line-clamp-2 text-sm text-v3-ink-2">
                      {entry.reason || "未记录原因"}
                    </span>
                  </V3Td>
                </V3Tr>
              ))
            )}
          </tbody>
        </V3Table>
      </WorkSurface>
    </div>
  );
}

function MetricBlock({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0 rounded-v3-inner bg-v3-card-soft p-3">
      <p className="text-xs text-v3-ink-2">{label}</p>
      <p className="mt-2 truncate font-mono text-sm font-semibold text-v3-ink tabular-nums">
        {value}
      </p>
    </div>
  );
}

function formatNumber(value: number) {
  return new Intl.NumberFormat("zh-CN").format(value);
}

function formatOptionalNumber(value?: number) {
  return typeof value === "number" ? formatNumber(value) : "-";
}

function formatCost(value: string) {
  return value ? `¥${value}` : "¥0";
}

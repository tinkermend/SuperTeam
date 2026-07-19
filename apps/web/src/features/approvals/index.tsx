import { useMemo, useState } from "react";
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Link, useSearch } from "@tanstack/react-router";
import { AlertTriangle, ShieldCheck } from "lucide-react";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import {
  StatusPill,
  V3Button,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3MetricCard,
  WorkSurface,
} from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  executeInboxAction,
  listInboxItems,
  type ExecuteInboxActionInput,
  type InboxAction,
  type InboxItem,
  type InboxListFilters,
} from "@/lib/api/inbox";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { InboxActionDialog } from "@/features/inbox/components/inbox-action-dialog";
import {
  formatContext,
  resolveInboxHref,
  riskLabel,
  riskTone,
} from "@/features/inbox/components/inbox-item-list";
import { statusLabel } from "@/lib/status-labels";

type ApprovalsCenterViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

type SelectedAction = {
  action: InboxAction;
  item: InboxItem;
};

export function ApprovalsCenterPage({ fetcher }: { fetcher?: typeof fetch } = {}) {
  return <ApprovalsCenterView apiBaseUrl={resolveControlPlaneUrl()} fetcher={fetcher} />;
}

export function ApprovalsCenterView({ apiBaseUrl, fetcher }: ApprovalsCenterViewProps) {
  const search = useSearch({ strict: false }) as {
    project?: string;
    risk?: string;
    status?: string;
  };
  const queryClient = useQueryClient();
  const apiOptions = useMemo<ApiClientOptions>(
    () => ({ baseUrl: apiBaseUrl, fetcher }),
    [apiBaseUrl, fetcher],
  );
  const [selectedAction, setSelectedAction] = useState<SelectedAction | null>(null);
  const filters = useMemo<InboxListFilters>(() => {
    const next: InboxListFilters = {
      item_type: "project_decision",
      limit: 50,
      offset: 0,
      status: search.status === "resolved" || search.status === "cancelled" ? search.status : "open",
      view: "mine",
    };
    if (search.risk) {
      next.risk_level = search.risk;
    }
    if (search.project) {
      next.project_id = search.project;
    }
    return next;
  }, [search.project, search.risk, search.status]);

  const approvalsQuery = useQuery({
    queryKey: ["approvals-center", filters],
    queryFn: () => listInboxItems(apiOptions, filters),
    placeholderData: keepPreviousData,
    // 同 inbox:外部渠道 resolve 无推送,靠轮询反映状态变化。
    refetchInterval: 5000,
  });

  const actionMutation = useMutation({
    mutationFn: ({
      input,
      itemId,
    }: {
      input: ExecuteInboxActionInput;
      itemId: string;
    }) => executeInboxAction(apiOptions, itemId, input),
    onSuccess: () => {
      setSelectedAction(null);
      void queryClient.invalidateQueries({ queryKey: ["approvals-center"] });
      void queryClient.invalidateQueries({ queryKey: ["inbox-items"] });
      void queryClient.invalidateQueries({ queryKey: ["inbox-badge"] });
    },
  });

  const data = approvalsQuery.data;
  const items = data?.items ?? [];

  return (
    <>
      <ShellPageHeader
        icon={<ShieldCheck />}
        iconTone="ok"
        title="审批中心"
        subtitle="聚合项目决策和审批事项，按状态、风险和项目来源筛选处理。"
      />
      <Main width="wide" className="space-y-5 text-v3-ink">
        {data ? (
          <section className="grid gap-4 sm:grid-cols-3">
            <V3MetricCard
              icon={<ShieldCheck />}
              iconTone="info"
              label="开放审批"
              value={data.summary.open_count}
              meta="来自 inbox API"
            />
            <V3MetricCard
              icon={<AlertTriangle />}
              iconTone="danger"
              label="高风险"
              value={data.summary.high_risk_count}
              meta="需优先确认"
            />
            <V3MetricCard
              icon={<AlertTriangle />}
              iconTone="warn"
              label="阻断"
              value={data.summary.blocked_count}
              meta="等待人类判断"
            />
          </section>
        ) : null}

        <WorkSurface className="min-w-0">
          <div className="flex flex-col gap-3 border-b border-v3-line p-4 lg:flex-row lg:items-center lg:justify-between">
            <div className="min-w-0">
              <h2 className="text-sm font-semibold text-v3-ink">审批队列</h2>
              <p className="mt-1 text-xs text-v3-ink-2">
                默认展示项目决策事项；详情和处理动作复用收件箱能力。
              </p>
            </div>
            <div className="flex flex-wrap gap-2 text-xs">
              <FilterChip label="状态" value={statusLabel(filters.status ?? "open")} />
              {filters.risk_level ? (
                <FilterChip label="风险" value={riskLabel[filters.risk_level] ?? filters.risk_level} />
              ) : null}
              {filters.project_id ? (
                <FilterChip
                  label="项目"
                  value={
                    items.find((entry) => entry.source_project_id === filters.project_id)
                      ?.source_project_name ?? filters.project_id
                  }
                />
              ) : null}
            </div>
          </div>

          {approvalsQuery.isLoading && !data ? <V3LoadingState label="加载审批中心" /> : null}
          {approvalsQuery.isError && !data ? (
            <V3ErrorState title="审批中心加载失败" description={approvalsQuery.error.message} />
          ) : null}
          {data && items.length === 0 ? <V3EmptyState title="当前没有审批事项" /> : null}
          {items.length > 0 ? (
            <div className="divide-y divide-v3-line">
              {items.map((item) => (
                <ApprovalRow
                  item={item}
                  key={item.id}
                  onAction={(action) => {
                    actionMutation.reset();
                    setSelectedAction({ action, item });
                  }}
                />
              ))}
            </div>
          ) : null}
        </WorkSurface>
      </Main>

      <InboxActionDialog
        action={selectedAction?.action ?? null}
        item={selectedAction?.item ?? null}
        onOpenChange={(open) => {
          if (!open && !actionMutation.isPending) {
            setSelectedAction(null);
          }
        }}
        onSubmit={(input) => {
          if (!selectedAction) {
            return Promise.resolve();
          }
          return actionMutation.mutateAsync({
            input,
            itemId: selectedAction.item.id,
          });
        }}
        open={Boolean(selectedAction)}
        pending={actionMutation.isPending}
      />
    </>
  );
}

function ApprovalRow({
  item,
  onAction,
}: {
  item: InboxItem;
  onAction: (action: InboxAction) => void;
}) {
  const actions = Array.isArray(item.actions) ? item.actions : [];

  return (
    <div className="grid gap-3 p-4 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
      <div className="min-w-0">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <h3 className="text-sm font-semibold text-v3-ink">{item.title}</h3>
          {item.risk_level ? (
            <StatusPill tone={riskTone[item.risk_level] ?? "mute"}>
              {riskLabel[item.risk_level] ?? item.risk_level}
            </StatusPill>
          ) : null}
          <StatusPill tone={item.status === "open" ? "warn" : "mute"}>
            {statusLabel(item.status)}
          </StatusPill>
        </div>
        <p className="mt-1 line-clamp-2 text-sm text-v3-ink-2">
          {item.summary ?? "等待处理"}
        </p>
        <p className="mt-1 truncate text-xs text-v3-ink-3">
          {formatContext(item) ?? item.source_project_id ?? item.source_id}
        </p>
      </div>
      <div className="flex flex-wrap gap-2">
        {actions.slice(0, 3).map((action) => (
          <V3Button
            key={action.key}
            size="sm"
            type="button"
            variant={action.tone === "destructive" ? "danger" : action.tone === "positive" ? "primary" : "outline"}
            onClick={() => onAction(action)}
          >
            {formatApprovalActionLabel(action)} {item.title}
          </V3Button>
        ))}
        <V3Button asChild size="sm" variant="outline">
          <Link to={resolveInboxHref(item)}>查看项目审批</Link>
        </V3Button>
      </div>
    </div>
  );
}

function FilterChip({ label, value }: { label: string; value: string }) {
  return (
    <span className="inline-flex h-8 items-center gap-1 rounded-v3-inner border border-v3-line bg-v3-card px-3 font-semibold text-v3-ink-2">
      {label}: <span className="font-mono text-v3-ink">{value}</span>
    </span>
  );
}

function formatApprovalActionLabel(action: InboxAction) {
  if (action.key === "approved") return "同意";
  if (action.key === "rejected") return "驳回";
  if (action.key === "needs_more_evidence") return "要求补证";
  return action.label || action.key;
}

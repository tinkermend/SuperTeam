import { useMemo, useRef, useState } from "react";
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient
} from "@tanstack/react-query";
import { Link, useSearch } from "@tanstack/react-router";
import { AlertTriangle, ShieldCheck } from "lucide-react";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import {
  StatusPill,
  Button,
  EmptyState,
  ErrorState,
  LoadingState,
  MetricCard,
  WorkSurface
} from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  executeInboxAction,
  listInboxItems,
  type ExecuteInboxActionInput,
  type InboxAction,
  type InboxItem,
  type InboxListFilters
} from "@/lib/api/inbox";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { InboxActionDialog } from "@/features/inbox/components/inbox-action-dialog";
import {
  formatContext,
  resolveInboxHref,
  riskLabel,
  riskTone
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
  // 与收件箱同一套按事项并行模式:在飞按事项 id 记录,不同事项互不阻塞;
  // 弹窗已切走或关闭后失败的后台提交升级到页面横幅,不静默丢失。
  const [pendingItemIds, setPendingItemIds] = useState<ReadonlySet<string>>(() => new Set());
  const [backgroundActionError, setBackgroundActionError] = useState<Error | null>(null);
  const selectedActionRef = useRef<SelectedAction | null>(null);
  selectedActionRef.current = selectedAction;
  const filters = useMemo<InboxListFilters>(() => {
    const next: InboxListFilters = {
      item_type: "project_decision",
      limit: 50,
      offset: 0,
      status: search.status === "resolved" || search.status === "cancelled" ? search.status : "open",
      view: "mine"
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
    refetchInterval: 5000
});

  const actionMutation = useMutation({
    mutationFn: ({
      input,
      itemId
}: {
      input: ExecuteInboxActionInput;
      itemId: string;
    }) => executeInboxAction(apiOptions, itemId, input),
    onMutate: ({ itemId }) => {
      setPendingItemIds((current) => {
        const next = new Set(current);
        next.add(itemId);
        return next;
      });
    },
    onSuccess: (_data, { itemId }) => {
      // 只关掉本次提交对应的弹窗;用户已切到别的事项时不打断。
      setSelectedAction((current) => (current && current.item.id === itemId ? null : current));
      void queryClient.invalidateQueries({ queryKey: ["approvals-center"] });
      void queryClient.invalidateQueries({ queryKey: ["inbox-items"] });
      void queryClient.invalidateQueries({ queryKey: ["inbox-badge"] });
    },
    onError: (error, { itemId }) => {
      // 弹窗仍停在该事项时错误由弹窗内联展示;否则升级到页面横幅。
      const current = selectedActionRef.current;
      if (!current || current.item.id !== itemId) {
        setBackgroundActionError(error);
      }
    },
    onSettled: (_data, _error, { itemId }) => {
      setPendingItemIds((current) => {
        const next = new Set(current);
        next.delete(itemId);
        return next;
      });
    }
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
      <Main width="wide" className="space-y-5 text-ink">
        {backgroundActionError ? (
          <div
            className="rounded-inner bg-danger-soft p-4 text-sm text-danger"
            role="alert"
          >
            <p className="font-bold">操作未完成</p>
            <p className="mt-1 text-ink-2">{backgroundActionError.message}</p>
          </div>
        ) : null}
        {data ? (
          <section className="grid gap-4 sm:grid-cols-3">
            <MetricCard
              icon={<ShieldCheck />}
              iconTone="info"
              label="开放审批"
              value={data.summary.open_count}
              meta="来自 inbox API"
            />
            <MetricCard
              icon={<AlertTriangle />}
              iconTone="danger"
              label="高风险"
              value={data.summary.high_risk_count}
              meta="需优先确认"
            />
            <MetricCard
              icon={<AlertTriangle />}
              iconTone="warn"
              label="阻断"
              value={data.summary.blocked_count}
              meta="等待人类判断"
            />
          </section>
        ) : null}

        <WorkSurface className="min-w-0">
          <div className="flex flex-col gap-3 border-b border-line p-4 lg:flex-row lg:items-center lg:justify-between">
            <div className="min-w-0">
              <h2 className="text-sm font-semibold text-ink">审批队列</h2>
              <p className="mt-1 text-xs text-ink-2">
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

          {approvalsQuery.isLoading && !data ? <LoadingState label="加载审批中心" /> : null}
          {approvalsQuery.isError && !data ? (
            <ErrorState title="审批中心加载失败" description={approvalsQuery.error.message} />
          ) : null}
          {data && items.length === 0 ? <EmptyState title="当前没有审批事项" /> : null}
          {items.length > 0 ? (
            <div className="divide-y divide-line">
              {items.map((item) => (
                <ApprovalRow
                  item={item}
                  key={item.id}
                  onAction={(action) => {
                    setBackgroundActionError(null);
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
          // 提交中也允许关闭:提交在后台继续,结果由 onSuccess/onError 按事项归属处理。
          if (!open) {
            setSelectedAction(null);
          }
        }}
        onSubmit={(input) => {
          const current = selectedActionRef.current;
          if (!current || pendingItemIds.has(current.item.id)) {
            return Promise.resolve();
          }
          return actionMutation.mutateAsync({
            input,
            itemId: current.item.id
});
        }}
        open={Boolean(selectedAction)}
        pending={selectedAction ? pendingItemIds.has(selectedAction.item.id) : false}
      />
    </>
  );
}

function ApprovalRow({
  item,
  onAction
}: {
  item: InboxItem;
  onAction: (action: InboxAction) => void;
}) {
  const actions = Array.isArray(item.actions) ? item.actions : [];

  return (
    <div className="grid gap-3 p-4 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-center">
      <div className="min-w-0">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <h3 className="text-sm font-semibold text-ink">{item.title}</h3>
          {item.risk_level ? (
            <StatusPill tone={riskTone[item.risk_level] ?? "mute"}>
              {riskLabel[item.risk_level] ?? item.risk_level}
            </StatusPill>
          ) : null}
          <StatusPill tone={item.status === "open" ? "warn" : "mute"}>
            {statusLabel(item.status)}
          </StatusPill>
        </div>
        <p className="mt-1 line-clamp-2 text-sm text-ink-2">
          {item.summary ?? "等待处理"}
        </p>
        <p className="mt-1 truncate text-xs text-ink-3">
          {formatContext(item) ?? item.source_project_id ?? item.source_id}
        </p>
      </div>
      <div className="flex flex-wrap gap-2">
        {actions.slice(0, 3).map((action) => (
          <Button
            key={action.key}
            size="sm"
            type="button"
            variant={action.tone === "destructive" ? "danger" : action.tone === "positive" ? "primary" : "outline"}
            onClick={() => onAction(action)}
          >
            {formatApprovalActionLabel(action)} {item.title}
          </Button>
        ))}
        <Button asChild size="sm" variant="outline">
          <Link to={resolveInboxHref(item)}>查看项目审批</Link>
        </Button>
      </div>
    </div>
  );
}

function FilterChip({ label, value }: { label: string; value: string }) {
  return (
    <span className="inline-flex h-8 items-center gap-1 rounded-inner border border-line bg-card px-3 font-semibold text-ink-2">
      {label}: <span className="font-mono text-ink">{value}</span>
    </span>
  );
}

function formatApprovalActionLabel(action: InboxAction) {
  if (action.key === "approved") return "同意";
  if (action.key === "rejected") return "驳回";
  if (action.key === "needs_more_evidence") return "要求补证";
  return action.label || action.key;
}

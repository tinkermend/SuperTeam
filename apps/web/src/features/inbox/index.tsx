import { useEffect, useMemo, useRef, useState } from "react";
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient
} from "@tanstack/react-query";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  executeInboxAction,
  listInboxItems,
  type ExecuteInboxActionInput,
  type InboxAction,
  type InboxItem,
  type InboxListFilters,
  type InboxSortMode,
  type InboxViewMode
} from "@/lib/api/inbox";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { notifySuccess } from "@/components/superteam";
import {
  DEFAULT_INBOX_LIST_FILTERS,
  inboxItemsQueryKey
} from "./inbox-query";
import { inboxActionSuccessFeedback } from "./inbox-action-feedback";
import { inboxListRefetchInterval } from "./inbox-stream-status";
import { InboxActionDialog } from "./components/inbox-action-dialog";
import { flatInboxRenderOrder } from "./components/inbox-item-list";
import {
  InboxShell,
  type InboxFilterChangeValue,
  type InboxFilterKey
} from "./components/inbox-shell";
import { useInboxStreamStatus } from "./use-inbox-stream-status";

type InboxPageProps = {
  fetcher?: typeof fetch;
  /** 路由 search 中的 sort（URL 为事实源）；测试可不传。 */
  initialSort?: InboxSortMode;
  /** 排序变更写回 URL；测试可不传。 */
  onSortChange?: (sort: InboxSortMode) => void;
};

type InboxViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  initialSort?: InboxSortMode;
  onSortChange?: (sort: InboxSortMode) => void;
};

function sortFromRoute(initialSort?: InboxSortMode): InboxSortMode | undefined {
  return initialSort === "oldest" ? "oldest" : undefined;
}

type SelectedAction = {
  action: InboxAction;
  item: InboxItem;
};

// 默认只看 open:服务端无 status 过滤时会返回 resolved/cancelled 项,
// "待处理事项"列表会把几天前已处理的旧项继续标成待处理(外部渠道 resolve
// 后该项也因此永不消失)。用户可用状态筛选切回"所有"。
const DEFAULT_INBOX_FILTERS = DEFAULT_INBOX_LIST_FILTERS;

const INBOX_STATUSES = ["open", "resolved", "cancelled"] satisfies Array<
  NonNullable<InboxListFilters["status"]>
>;
const INBOX_ITEM_TYPES = [
  "approval",
  "project_decision",
  "team_pending_delete",
  "channel_alert",
  "automation_alert",
  "casting_invalidated",
] satisfies Array<NonNullable<InboxListFilters["item_type"]>>;

export function InboxPage({
  fetcher,
  initialSort,
  onSortChange,
}: InboxPageProps = {}) {
  return (
    <InboxView
      apiBaseUrl={resolveControlPlaneUrl()}
      fetcher={fetcher}
      initialSort={initialSort}
      onSortChange={onSortChange}
    />
  );
}

export function InboxView({
  apiBaseUrl,
  fetcher,
  initialSort,
  onSortChange,
}: InboxViewProps) {
  const queryClient = useQueryClient();
  const streamStatus = useInboxStreamStatus();
  const apiOptions = useMemo<ApiClientOptions>(
    () => ({ baseUrl: apiBaseUrl, fetcher }),
    [apiBaseUrl, fetcher],
  );
  const [view, setView] = useState<InboxViewMode>("mine");
  const [filters, setFilters] = useState<InboxListFilters>(() => ({
    ...DEFAULT_INBOX_LIST_FILTERS,
    ...(sortFromRoute(initialSort) ? { sort: sortFromRoute(initialSort) } : {}),
  }));
  const [selectedAction, setSelectedAction] = useState<SelectedAction | null>(null);
  const [selectedItemId, setSelectedItemId] = useState<string | null>(null);
  // 弹窗关闭（取消或提交完成）后递增，触发列表把焦点还给选中行。
  const [refocusToken, setRefocusToken] = useState(0);
  // 提交按事项并行:记录在飞事项 id,弹窗仅在"当前事项在飞"时置提交中,不同事项互不阻塞。
  const [pendingItemIds, setPendingItemIds] = useState<ReadonlySet<string>>(() => new Set());
  // 弹窗已切走或关闭后失败的后台提交,升级到页面横幅提示,不静默丢失。
  const [backgroundActionError, setBackgroundActionError] = useState<Error | null>(null);
  const selectedActionRef = useRef<SelectedAction | null>(null);
  selectedActionRef.current = selectedAction;
  // 列表快照供 onSuccess 计算「同位置下一条」（invalidate 前的渲染序）。
  const itemsSnapshotRef = useRef<InboxItem[]>([]);
  // 与 onSuccess 里读 filters 时的闭包脱节无关：sort 用 ref 镜像当前值。
  const sortRef = useRef<InboxSortMode>(filters.sort === "oldest" ? "oldest" : "risk");
  sortRef.current = filters.sort === "oldest" ? "oldest" : "risk";

  // U5 / §4.4.2：URL search 是 sort 事实源。in-SPA 改 search 不会 remount，须同步 filters。
  useEffect(() => {
    const fromUrl = initialSort === "oldest" ? "oldest" : "risk";
    setFilters((current) => {
      const currentSort = current.sort === "oldest" ? "oldest" : "risk";
      if (currentSort === fromUrl) return current;
      if (fromUrl === "oldest") {
        return { ...current, sort: "oldest", offset: 0 };
      }
      const { sort: _drop, ...rest } = current;
      return { ...rest, offset: 0 };
    });
  }, [initialSort]);

  const handleFilterChange = <Key extends InboxFilterKey>(
    key: Key,
    value: InboxFilterChangeValue<Key>,
  ) => {
    setFilters((current) => updateInboxFilter(current, key, value));
    if (key === "sort") {
      onSortChange?.(value === "oldest" ? "oldest" : "risk");
    }
  };

  const inboxQuery = useQuery({
    queryKey: inboxItemsQueryKey(view, filters),
    queryFn: () => listInboxItems(apiOptions, { ...filters, view }),
    placeholderData: keepPreviousData,
    // 主刷新由全局 SSE（含 onopen 重连追平）驱动；连上时 15s 兜底，断流 5s 快拉。
    // 覆盖 main.tsx 里 DEV 关闭的 refetchOnWindowFocus——守候人切回 tab 必须追平。
    refetchInterval: inboxListRefetchInterval(streamStatus.connection),
    refetchOnWindowFocus: true,
  });
  itemsSnapshotRef.current = inboxQuery.data?.items ?? [];

  const actionMutation = useMutation({
    mutationFn: ({
      itemId,
      input
    }: {
      itemId: string;
      itemType?: string;
      input: ExecuteInboxActionInput;
    }) => executeInboxAction(apiOptions, itemId, input),
    onMutate: ({ itemId }) => {
      setPendingItemIds((current) => {
        const next = new Set(current);
        next.add(itemId);
        return next;
      });
    },
    onSuccess: (_data, { itemId, itemType }) => {
      // 只关掉本次提交对应的弹窗;用户已切到别的事项时不打断。
      setSelectedAction((current) => (current && current.item.id === itemId ? null : current));
      // §4.3.2 处理后自动前进：仅在本成功回调生效。用提交前快照取渲染序位置，
      // 选中「同位置的下一条」；已是最后一条则选中前一条；仅剩自身则清空。
      // 若用户在后台提交期间已切到其他事项，不得抢选中（与弹窗关闭守卫同口径）。
      // 他人处理导致消失仍由 shell 的 effect 清空，两条语义不得互污。
      const flat = flatInboxRenderOrder(itemsSnapshotRef.current, {
        sort: sortRef.current,
      });
      const idx = flat.findIndex((item) => item.id === itemId);
      if (idx >= 0) {
        const next =
          idx < flat.length - 1 ? flat[idx + 1] : idx > 0 ? flat[idx - 1] : null;
        const nextId = next && next.id !== itemId ? next.id : null;
        setSelectedItemId((current) => {
          if (current !== null && current !== itemId) {
            return current;
          }
          return nextId;
        });
      }
      void queryClient.invalidateQueries({ queryKey: ["inbox-items"] });
      void queryClient.invalidateQueries({ queryKey: ["inbox-badge"] });
      const feedback = inboxActionSuccessFeedback(itemType);
      notifySuccess(feedback.message, {
        description: feedback.description
      });
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

  return (
    <>
      <InboxShell
        apiBaseUrl={apiBaseUrl}
        data={inboxQuery.data}
        dataUpdatedAt={inboxQuery.dataUpdatedAt}
        error={inboxQuery.error}
        fetcher={fetcher}
        isFetching={inboxQuery.isFetching}
        isLoading={inboxQuery.isLoading}
        mutationError={backgroundActionError}
        onAction={(item, action) => {
          setBackgroundActionError(null);
          setSelectedItemId(item.id);
          setSelectedAction({ action, item });
        }}
        onFilterChange={handleFilterChange}
        onRefresh={() => {
          void inboxQuery.refetch();
        }}
        onRetry={() => {
          void inboxQuery.refetch();
        }}
        onResetFilters={() => {
          setFilters({ ...DEFAULT_INBOX_FILTERS });
          // 排序只进 URL：重置须同步清掉 sort，避免仍停在 oldest 非分诊视图（U5）。
          onSortChange?.("risk");
        }}
        onSelectItem={setSelectedItemId}
        refocusToken={refocusToken}
        onViewChange={(nextView) => {
          setView(nextView);
          // 目标用户筛选仅团队视图有意义；切回我的待办时清掉。
          if (nextView === "mine") {
            setFilters((current) => {
              if (!current.target_user_id) return current;
              const { target_user_id: _cleared, ...rest } = current;
              return { ...rest, offset: 0 };
            });
          }
        }}
        selectedItemId={selectedItemId}
        filters={filters}
        streamConnection={streamStatus.connection}
        view={view}
      />
      <InboxActionDialog
        action={selectedAction?.action ?? null}
        item={selectedAction?.item ?? null}
        onOpenChange={(open) => {
          // 提交中也允许关闭:提交在后台继续,结果由 onSuccess/onError 按事项归属处理。
          if (!open) {
            setSelectedAction(null);
            // 焦点归还：Radix 无 trigger 可归还（弹窗由行 Enter / 行内 CTA 程序化
            // 打开），不还则焦点落 body，键盘队列断掉。取消与提交后都要还。
            setRefocusToken((n) => n + 1);
          }
        }}
        onSubmit={(input) => {
          const current = selectedActionRef.current;
          if (!current || pendingItemIds.has(current.item.id)) {
            return Promise.resolve();
          }

          return actionMutation.mutateAsync({
            input,
            itemId: current.item.id,
            itemType: current.item.item_type
          });
        }}
        open={Boolean(selectedAction)}
        pending={selectedAction ? pendingItemIds.has(selectedAction.item.id) : false}
      />
    </>
  );
}

function updateInboxFilter(
  filters: InboxListFilters,
  key: InboxFilterKey,
  value: string,
): InboxListFilters {
  const next: InboxListFilters = { ...filters, offset: 0 };
  const normalized = value.trim();

  if (normalized === "" || normalized === "all") {
    clearInboxFilter(next, key);
    return next;
  }

  setInboxFilter(next, key, normalized);
  return next;
}

function clearInboxFilter(filters: InboxListFilters, key: InboxFilterKey) {
  switch (key) {
    case "item_type":
      delete filters.item_type;
      break;
    case "project_id":
      delete filters.project_id;
      break;
    case "risk_level":
      delete filters.risk_level;
      break;
    case "sort":
      // 排序回落默认 risk，不删除字段以免 queryKey 抖动无意义。
      filters.sort = "risk";
      break;
    case "status":
      delete filters.status;
      break;
    case "target_user_id":
      delete filters.target_user_id;
      break;
  }
}

function setInboxFilter(filters: InboxListFilters, key: InboxFilterKey, value: string) {
  switch (key) {
    case "item_type":
      if (isInboxItemType(value)) {
        filters.item_type = value;
      }
      break;
    case "project_id":
      // 项目选择器输出合法 UUID；保留字符串原样，服务端再校验。
      filters.project_id = value;
      break;
    case "risk_level":
      filters.risk_level = value;
      break;
    case "sort":
      filters.sort = value === "oldest" ? "oldest" : "risk";
      break;
    case "status":
      if (isInboxStatus(value)) {
        filters.status = value;
      }
      break;
    case "target_user_id":
      filters.target_user_id = value;
      break;
  }
}

function isInboxStatus(value: string): value is NonNullable<InboxListFilters["status"]> {
  return includesString(INBOX_STATUSES, value);
}

function isInboxItemType(value: string): value is NonNullable<InboxListFilters["item_type"]> {
  return includesString(INBOX_ITEM_TYPES, value);
}

function includesString<Value extends string>(
  values: readonly Value[],
  value: string,
): value is Value {
  return values.some((candidate) => candidate === value);
}

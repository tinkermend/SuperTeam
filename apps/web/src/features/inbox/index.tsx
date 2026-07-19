import { useEffect, useMemo, useRef, useState } from "react";
import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import type { ApiClientOptions } from "@/lib/api/client";
import {
  executeInboxAction,
  listInboxItems,
  type ExecuteInboxActionInput,
  type InboxAction,
  type InboxItem,
  type InboxListFilters,
  type InboxViewMode,
} from "@/lib/api/inbox";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { InboxActionDialog } from "./components/inbox-action-dialog";
import {
  InboxShell,
  type InboxFilterChangeValue,
  type InboxFilterKey,
  type InboxUuidFilterDrafts,
  type InboxUuidFilterKey,
} from "./components/inbox-shell";

type InboxPageProps = {
  fetcher?: typeof fetch;
};

type InboxViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  // 测试注入用；生产默认用带凭据的原生 EventSource。
  eventSourceFactory?: (url: string) => EventSource;
};

type SelectedAction = {
  action: InboxAction;
  item: InboxItem;
};

const DEFAULT_INBOX_FILTERS = {
  limit: 50,
  offset: 0,
  // 默认只看 open:服务端无 status 过滤时会返回 resolved/cancelled 项,
  // "待处理事项"列表会把几天前已处理的旧项继续标成待处理(外部渠道 resolve
  // 后该项也因此永不消失)。用户可用状态筛选切回"所有"。
  status: "open",
} satisfies InboxListFilters;

const EMPTY_UUID_FILTER_DRAFTS = {
  project_id: "",
  target_user_id: "",
} satisfies InboxUuidFilterDrafts;

const UUID_FILTER_ERROR = "请输入有效 UUID";
const NIL_UUID = "00000000-0000-0000-0000-000000000000";
const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;
const INBOX_STATUSES = ["open", "resolved", "cancelled"] satisfies Array<
  NonNullable<InboxListFilters["status"]>
>;
const INBOX_ITEM_TYPES = ["approval", "project_decision"] satisfies Array<
  NonNullable<InboxListFilters["item_type"]>
>;

export function InboxPage({ fetcher }: InboxPageProps = {}) {
  return <InboxView apiBaseUrl={resolveControlPlaneUrl()} fetcher={fetcher} />;
}

export function InboxView({ apiBaseUrl, fetcher, eventSourceFactory }: InboxViewProps) {
  const queryClient = useQueryClient();
  const actionInFlightRef = useRef(false);
  const apiOptions = useMemo<ApiClientOptions>(
    () => ({ baseUrl: apiBaseUrl, fetcher }),
    [apiBaseUrl, fetcher],
  );
  const [view, setView] = useState<InboxViewMode>("mine");
  const [filters, setFilters] = useState<InboxListFilters>(() => ({
    ...DEFAULT_INBOX_FILTERS,
  }));
  const [uuidFilterDrafts, setUuidFilterDrafts] = useState<InboxUuidFilterDrafts>(() => ({
    ...EMPTY_UUID_FILTER_DRAFTS,
  }));
  const [selectedAction, setSelectedAction] = useState<SelectedAction | null>(null);
  const uuidFilterErrors = useMemo(
    () => ({
      project_id: getUuidFilterError(uuidFilterDrafts.project_id),
      target_user_id: getUuidFilterError(uuidFilterDrafts.target_user_id),
    }),
    [uuidFilterDrafts],
  );

  const handleFilterChange = <Key extends InboxFilterKey>(
    key: Key,
    value: InboxFilterChangeValue<Key>,
  ) => {
    if (isUuidFilterKey(key)) {
      setUuidFilterDrafts((current) => ({ ...current, [key]: value }));
    }

    setFilters((current) => updateInboxFilter(current, key, value));
  };

  const inboxQuery = useQuery({
    queryKey: ["inbox-items", view, filters],
    queryFn: () => listInboxItems(apiOptions, { ...filters, view }),
    placeholderData: keepPreviousData,
    // 外部渠道(飞书/他人)的变更主要靠下方 SSE 脏通知推送刷新;
    // 这里只留低频轮询兜底流断开的窗口。
    refetchInterval: 60_000,
  });

  // SSE 脏通知:服务端探测到可见范围内收件箱变更时推 inbox-changed,收到即重拉列表与角标。
  // 流断开由 EventSource 自动重连,60s 轮询兜底。
  useEffect(() => {
    // 组件测试注入 fetcher 时默认不开真实流,避免连不上的重连噪音;显式给 factory 则照常开。
    if (fetcher && !eventSourceFactory) return;
    const factory =
      eventSourceFactory ?? ((url: string) => new EventSource(url, { withCredentials: true }));
    let source: EventSource | undefined;
    try {
      source = factory(`${apiBaseUrl}/api/v1/inbox/stream`);
    } catch {
      return;
    }
    const onChanged = () => {
      void queryClient.invalidateQueries({ queryKey: ["inbox-items"] });
      void queryClient.invalidateQueries({ queryKey: ["inbox-badge"] });
    };
    source.addEventListener("inbox-changed", onChanged);
    return () => {
      source?.removeEventListener("inbox-changed", onChanged);
      source?.close();
    };
  }, [apiBaseUrl, eventSourceFactory, fetcher, queryClient]);

  const actionMutation = useMutation({
    mutationFn: ({
      itemId,
      input,
    }: {
      itemId: string;
      input: ExecuteInboxActionInput;
    }) => executeInboxAction(apiOptions, itemId, input),
    onSuccess: () => {
      actionInFlightRef.current = false;
      setSelectedAction(null);
      void queryClient.invalidateQueries({ queryKey: ["inbox-items"] });
      void queryClient.invalidateQueries({ queryKey: ["inbox-badge"] });
    },
    onError: () => {
      actionInFlightRef.current = false;
    },
  });

  return (
    <>
      <InboxShell
        data={inboxQuery.data}
        error={inboxQuery.error}
        isLoading={inboxQuery.isLoading}
        mutationError={selectedAction ? null : actionMutation.error}
        onAction={(item, action) => {
          actionMutation.reset();
          setSelectedAction({ action, item });
        }}
        onFilterChange={handleFilterChange}
        onRetry={() => {
          void inboxQuery.refetch();
        }}
        onResetFilters={() => {
          setUuidFilterDrafts({ ...EMPTY_UUID_FILTER_DRAFTS });
          setFilters({ ...DEFAULT_INBOX_FILTERS });
        }}
        onViewChange={setView}
        filters={filters}
        uuidFilterDrafts={uuidFilterDrafts}
        uuidFilterErrors={uuidFilterErrors}
        view={view}
      />
      <InboxActionDialog
        action={selectedAction?.action ?? null}
        item={selectedAction?.item ?? null}
        onOpenChange={(open) => {
          if (!open && !actionMutation.isPending) {
            setSelectedAction(null);
          }
        }}
        onSubmit={(input) => {
          if (!selectedAction || actionInFlightRef.current) {
            return Promise.resolve();
          }

          actionInFlightRef.current = true;
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

  if (isUuidFilterKey(key) && !isValidNonNilUuid(normalized)) {
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
      if (isValidNonNilUuid(value)) {
        filters.project_id = value;
      }
      break;
    case "risk_level":
      filters.risk_level = value;
      break;
    case "status":
      if (isInboxStatus(value)) {
        filters.status = value;
      }
      break;
    case "target_user_id":
      if (isValidNonNilUuid(value)) {
        filters.target_user_id = value;
      }
      break;
  }
}

function getUuidFilterError(value: string) {
  const normalized = value.trim();
  return normalized !== "" && !isValidNonNilUuid(normalized) ? UUID_FILTER_ERROR : undefined;
}

function isUuidFilterKey(key: InboxFilterKey): key is InboxUuidFilterKey {
  return key === "project_id" || key === "target_user_id";
}

function isValidNonNilUuid(value: string) {
  return value.toLowerCase() !== NIL_UUID && UUID_PATTERN.test(value);
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

import { useMemo, useRef, useState } from "react";
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
};

type SelectedAction = {
  action: InboxAction;
  item: InboxItem;
};

const DEFAULT_INBOX_FILTERS = {
  limit: 50,
  offset: 0,
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

export function InboxView({ apiBaseUrl, fetcher }: InboxViewProps) {
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
  });

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
        isFetching={inboxQuery.isFetching}
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

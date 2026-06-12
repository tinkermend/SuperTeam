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
import { InboxShell, type InboxFilterKey } from "./components/inbox-shell";

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
  const [selectedAction, setSelectedAction] = useState<SelectedAction | null>(null);

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
        onFilterChange={(key, value) => {
          setFilters((current) => updateInboxFilter(current, key, value));
        }}
        onRetry={() => {
          void inboxQuery.refetch();
        }}
        onResetFilters={() => {
          setFilters({ ...DEFAULT_INBOX_FILTERS });
        }}
        onViewChange={setView}
        filters={filters}
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
      filters.item_type = value as InboxListFilters["item_type"];
      break;
    case "project_id":
      filters.project_id = value;
      break;
    case "risk_level":
      filters.risk_level = value;
      break;
    case "status":
      filters.status = value as InboxListFilters["status"];
      break;
    case "target_user_id":
      filters.target_user_id = value;
      break;
  }
}

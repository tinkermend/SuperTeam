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
import { InboxShell } from "./components/inbox-shell";

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
  const [filters] = useState<InboxListFilters>({
    limit: 50,
    offset: 0,
    status: "open",
  });
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
        onRetry={() => {
          void inboxQuery.refetch();
        }}
        onViewChange={setView}
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

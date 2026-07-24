import type { InboxListFilters, InboxViewMode } from "@/lib/api/inbox";

/**
 * 收件箱页默认筛选：与 InboxView 初始 state 保持一致，供侧栏 prefetch 对齐 queryKey。
 */
export const DEFAULT_INBOX_LIST_FILTERS = {
  limit: 50,
  offset: 0,
  status: "open",
} satisfies InboxListFilters;

export const DEFAULT_INBOX_VIEW = "mine" satisfies InboxViewMode;

export function inboxItemsQueryKey(
  view: InboxViewMode,
  filters: InboxListFilters,
) {
  return ["inbox-items", view, filters] as const;
}

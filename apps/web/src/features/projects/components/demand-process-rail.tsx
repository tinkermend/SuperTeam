import { Link } from "@tanstack/react-router";
import { SoftCard, StatusPill, ToolbarSearch } from "@/components/superteam";
import { cn } from "@/lib/utils";
import type { ProjectDemand } from "@/lib/api/projects";
import { demandStatusLabel } from "@/lib/status-labels";
import { formatRelativeTime } from "@/lib/format-time";
import { demandStatusTone } from "./demand-dossier-header";
import { findChainOf, foldDemandChains } from "./demand-chains";

type DemandProcessRailProps = {
  currentTab: string;
  demands: ProjectDemand[];
  /** 卷宗壳左栏：不再套一张独立 SoftCard。 */
  embedded?: boolean;
  hasMore?: boolean;
  loadMorePending?: boolean;
  onLoadMore?: () => void;
  pendingByDemand: ReadonlyMap<string, number>;
  projectId: string;
  searchQuery: string;
  onSearchQueryChange: (value: string) => void;
  selectedDemandId?: string;
};

/**
 * 项目详情左轨：需求流程（一次对话）。角标只用 sibling_pending 的待决数，
 * 不显示子任务计数（任务表才是条数事实）。
 */
export function DemandProcessRail({
  currentTab,
  demands,
  embedded = false,
  hasMore = false,
  loadMorePending = false,
  onLoadMore,
  pendingByDemand,
  projectId,
  searchQuery,
  onSearchQueryChange,
  selectedDemandId,
}: DemandProcessRailProps) {
  const filtered = searchQuery.trim()
    ? demands.filter((demand) => demand.title.includes(searchQuery.trim()))
    : demands;
  const chains = foldDemandChains(filtered);
  const selectedChain = findChainOf(chains, selectedDemandId);

  const body = (
    <>
      <div className="border-b border-line px-3 py-2.5">
        <h3 className="text-[12px] font-bold tracking-wide text-ink-3">需求流程</h3>
        <p className="mt-0.5 text-[11px] leading-4 text-ink-3">
          最近更新优先 · 执行中先于已完成
        </p>
        <div className="mt-2">
          <ToolbarSearch
            aria-label="搜索已加载的需求流程"
            onChange={(event) => onSearchQueryChange(event.target.value)}
            placeholder="搜索已加载的需求…"
            value={searchQuery}
          />
        </div>
        {searchQuery.trim() ? (
          <p className="mt-1.5 text-[11px] text-ink-3">仅筛选已加载的需求，不是全库检索。</p>
        ) : null}
      </div>
      <nav aria-label="需求流程" className="min-h-0 flex-1 divide-y divide-line overflow-y-auto">
        {chains.length === 0 ? (
          <p className="px-4 py-6 text-[12.5px] text-ink-3">没有匹配的需求流程。</p>
        ) : (
          chains.map((chain) => {
            const demand = chain.latest;
            const isSelected = chain === selectedChain;
            const continuationCount = chain.members.length - 1;
            const pendingCount = chain.members.reduce(
              (total, member) => total + (pendingByDemand.get(member.id) ?? 0),
              0,
            );
            const activityAt = demand.updated_at ?? demand.created_at;
            return (
              <Link
                aria-current={isSelected ? "true" : undefined}
                className={cn(
                  "block px-3 py-2 transition-colors hover:bg-card",
                  isSelected && "bg-brand-soft shadow-[inset_2px_0_0_var(--brand)]",
                )}
                data-testid={`demand-list-item-${demand.id}`}
                key={demand.id}
                params={{ projectId }}
                search={(prev) => ({
                  ...prev,
                  demand: demand.id,
                  tab: currentTab,
                })}
                to="/projects/$projectId"
              >
                <div className="flex items-start justify-between gap-2">
                  <p
                    className={cn(
                      "min-w-0 flex-1 truncate text-[13px] font-semibold",
                      isSelected ? "text-brand-deep" : "text-ink",
                    )}
                  >
                    {demand.title}
                  </p>
                  {pendingCount > 0 ? (
                    <span
                      aria-label={`待你处理 ${pendingCount} 项`}
                      className="shrink-0 rounded-full bg-warn-soft px-1.5 py-0.5 text-[10.5px] font-semibold tabular-nums text-warn-text"
                      data-testid={`demand-list-pending-${demand.id}`}
                    >
                      {pendingCount}
                    </span>
                  ) : null}
                </div>
                <div className="mt-1 flex flex-wrap items-center gap-1.5">
                  <StatusPill tone={demandStatusTone(demand.status)}>
                    {demandStatusLabel(demand.status)}
                  </StatusPill>
                  {continuationCount > 0 ? (
                    <span
                      className="rounded-full bg-card-soft px-1.5 py-0.5 text-[10.5px] text-ink-3"
                      data-testid={`demand-list-continuations-${demand.id}`}
                    >
                      接续 {continuationCount} 次
                    </span>
                  ) : null}
                  {activityAt ? (
                    <time
                      className="text-[11px] tabular-nums text-ink-3"
                      dateTime={activityAt}
                      title={activityAt}
                    >
                      {formatRelativeTime(activityAt)}
                    </time>
                  ) : null}
                </div>
              </Link>
            );
          })
        )}
      </nav>
      {hasMore && !searchQuery.trim() ? (
        <div className="border-t border-line p-2">
          <button
            className="h-8 w-full rounded-[10px] border border-dashed border-line-strong text-[12px] font-semibold text-ink-2 hover:bg-card hover:text-ink"
            data-testid="demand-rail-load-more"
            disabled={loadMorePending}
            onClick={onLoadMore}
            type="button"
          >
            {loadMorePending ? "加载中…" : "加载更多"}
          </button>
        </div>
      ) : null}
    </>
  );

  if (embedded) {
    return (
      <div
        className="flex max-h-56 min-h-0 flex-col overflow-hidden border-b border-line bg-card-soft @3xl/dossier:h-0 @3xl/dossier:max-h-none @3xl/dossier:min-h-full @3xl/dossier:border-b-0 @3xl/dossier:border-r"
        data-testid="demand-process-rail"
      >
        {body}
      </div>
    );
  }

  return (
    <SoftCard
      className="flex max-h-[720px] min-h-0 flex-col overflow-hidden @3xl/content:sticky @3xl/content:top-4"
      data-testid="demand-process-rail"
    >
      {body}
    </SoftCard>
  );
}

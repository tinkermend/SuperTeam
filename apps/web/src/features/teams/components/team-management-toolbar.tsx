import { RotateCcw, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { GovernanceSummaryStatus, TeamStatus } from "@/lib/api/teams";

export type TeamListFilters = {
  governance_status?: GovernanceSummaryStatus;
  q: string;
  status?: TeamStatus;
};

type TeamManagementToolbarProps = {
  filters: TeamListFilters;
  onChange: (filters: TeamListFilters) => void;
  onReset: () => void;
};

export function TeamManagementToolbar({
  filters,
  onChange,
  onReset,
}: TeamManagementToolbarProps) {
  return (
    <div className="mb-4 grid gap-3 rounded-md border bg-card/95 p-3 shadow-sm md:grid-cols-[minmax(260px,1fr)_180px_180px_auto]">
      <label className="relative">
        <span className="sr-only">搜索团队名称、slug、负责人</span>
        <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          className="pl-9"
          onChange={(event) => onChange({ ...filters, q: event.target.value })}
          placeholder="搜索团队名称、slug、负责人"
          value={filters.q}
        />
      </label>
      <select
        aria-label="团队状态"
        className="h-9 rounded-md border border-input bg-background px-3 text-sm"
        onChange={(event) =>
          onChange({
            ...filters,
            status:
              event.target.value === "all"
                ? undefined
                : (event.target.value as TeamStatus),
          })
        }
        value={filters.status ?? "all"}
      >
        <option value="all">全部状态</option>
        <option value="active">活跃</option>
        <option value="disabled">已禁用</option>
        <option value="archived">已归档</option>
      </select>
      <select
        aria-label="治理状态"
        className="h-9 rounded-md border border-input bg-background px-3 text-sm"
        onChange={(event) =>
          onChange({
            ...filters,
            governance_status:
              event.target.value === "all"
                ? undefined
                : (event.target.value as GovernanceSummaryStatus),
          })
        }
        value={filters.governance_status ?? "all"}
      >
        <option value="all">全部治理</option>
        <option value="not_configured">未配置</option>
        <option value="draft_pending">草案待批准</option>
        <option value="active">已生效</option>
        <option value="needs_update">需更新</option>
      </select>
      <Button onClick={onReset} type="button" variant="outline">
        <RotateCcw data-icon="inline-start" />
        重置
      </Button>
    </div>
  );
}

import { RotateCcw } from "lucide-react";
import {
  Button,
  ListToolbar,
  ToolbarSearch,
} from "@/components/superteam";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { GovernanceSummaryStatus, TeamStatus } from "@/lib/api/teams";
import { governanceStatusLabel, teamStatusLabel } from "@/lib/status-labels";

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
    <ListToolbar
      className="rounded-card border border-line bg-card px-3 py-2.5 shadow-sm sm:px-4"
      search={
        <ToolbarSearch
          aria-label="搜索团队名称、slug、负责人"
          onChange={(event) => onChange({ ...filters, q: event.target.value })}
          placeholder="搜索团队名称、slug、负责人…"
          value={filters.q}
        />
      }
      filters={
        <>
          <Select
            onValueChange={(value) =>
              onChange({
                ...filters,
                status: value === "all" ? undefined : (value as TeamStatus),
              })
            }
            value={filters.status ?? "all"}
          >
            <SelectTrigger
              aria-label="团队状态"
              className="h-9 w-[132px] border-line bg-card-soft shadow-none"
            >
              <SelectValue placeholder="团队状态" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部状态</SelectItem>
              <SelectItem value="active">{teamStatusLabel("active")}</SelectItem>
              <SelectItem value="disabled">{teamStatusLabel("disabled")}</SelectItem>
              <SelectItem value="archived">{teamStatusLabel("archived")}</SelectItem>
            </SelectContent>
          </Select>

          <Select
            onValueChange={(value) =>
              onChange({
                ...filters,
                governance_status:
                  value === "all" ? undefined : (value as GovernanceSummaryStatus),
              })
            }
            value={filters.governance_status ?? "all"}
          >
            <SelectTrigger
              aria-label="治理状态"
              className="h-9 w-[132px] border-line bg-card-soft shadow-none"
            >
              <SelectValue placeholder="治理状态" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">全部治理</SelectItem>
              <SelectItem value="not_configured">
                {governanceStatusLabel("not_configured")}
              </SelectItem>
              <SelectItem value="draft_pending">
                {governanceStatusLabel("draft_pending")}
              </SelectItem>
              <SelectItem value="active">{governanceStatusLabel("active")}</SelectItem>
              <SelectItem value="needs_update">
                {governanceStatusLabel("needs_update")}
              </SelectItem>
            </SelectContent>
          </Select>
        </>
      }
      actions={
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={onReset}
          aria-label="重置筛选"
        >
          <RotateCcw className="size-4" />
          <span className="max-sm:sr-only">重置</span>
        </Button>
      }
    />
  );
}

import { RotateCcw, Search } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
    <div className="mb-6 flex flex-col gap-3 rounded-lg border bg-card/60 p-2 shadow-sm backdrop-blur-sm sm:flex-row sm:items-center">
      <div className="relative flex-1">
        <span className="sr-only">搜索团队名称、slug、负责人</span>
        <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
        <Input
          className="h-10 border-none bg-background/50 pl-9 shadow-none focus-visible:ring-1"
          onChange={(event) => onChange({ ...filters, q: event.target.value })}
          placeholder="搜索团队名称、slug、负责人..."
          value={filters.q}
        />
      </div>
      
      <div className="flex items-center gap-2">
        <div className="h-6 w-px bg-border max-sm:hidden" />
        
        <Select
          onValueChange={(value) =>
            onChange({
              ...filters,
              status: value === "all" ? undefined : (value as TeamStatus),
            })
          }
          value={filters.status ?? "all"}
        >
          <SelectTrigger aria-label="团队状态" className="h-10 w-[140px] border-none bg-background/50 shadow-none focus:ring-1">
            <SelectValue placeholder="团队状态" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部状态</SelectItem>
            <SelectItem value="active">活跃</SelectItem>
            <SelectItem value="disabled">已禁用</SelectItem>
            <SelectItem value="archived">已归档</SelectItem>
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
          <SelectTrigger aria-label="治理状态" className="h-10 w-[140px] border-none bg-background/50 shadow-none focus:ring-1">
            <SelectValue placeholder="治理状态" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">全部治理</SelectItem>
            <SelectItem value="not_configured">未配置</SelectItem>
            <SelectItem value="draft_pending">草案待批准</SelectItem>
            <SelectItem value="active">已生效</SelectItem>
            <SelectItem value="needs_update">需更新</SelectItem>
          </SelectContent>
        </Select>

        <div className="h-6 w-px bg-border" />

        <Button 
          className="h-10 px-3" 
          onClick={onReset} 
          size="sm" 
          variant="ghost"
        >
          <RotateCcw className="size-4" />
          <span className="sr-only sm:not-sr-only sm:ml-2">重置</span>
        </Button>
      </div>
    </div>
  );
}

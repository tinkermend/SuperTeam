import { MoreHorizontal } from "lucide-react";
import {
  TeamIconTile,
  type TeamDisplayMetadata,
} from "@/components/superteam/team-icon-tile";
import {
  UserIdentity,
  type UserIdentityData,
} from "@/components/superteam/user-identity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import type {
  GovernanceSummaryStatus,
  TeamListItem,
  TeamStatus,
} from "@/lib/api/teams";

const TEAM_LIST_COLUMN_COUNT = 10;

type TeamListTableProps = {
  teams: TeamListItem[];
  canGoNext: boolean;
  highlightedTeamId?: string;
  isError?: boolean;
  isLoading?: boolean;
  onPageChange: (pageIndex: number) => void;
  onPageSizeChange: (pageSize: number) => void;
  pageIndex: number;
  pageSize: number;
};

export function TeamListTable({
  canGoNext,
  highlightedTeamId,
  isError,
  isLoading,
  onPageChange,
  onPageSizeChange,
  pageIndex,
  pageSize,
  teams,
}: TeamListTableProps) {
  return (
    <div className="rounded-md border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="min-w-52">团队</TableHead>
            <TableHead>负责人</TableHead>
            <TableHead>成员</TableHead>
            <TableHead>数字员工</TableHead>
            <TableHead>能力</TableHead>
            <TableHead>治理状态</TableHead>
            <TableHead>当前版本</TableHead>
            <TableHead>待批准</TableHead>
            <TableHead>更新时间</TableHead>
            <TableHead className="w-12">
              <span className="sr-only">操作</span>
            </TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {isLoading ? (
            <TeamListStateRow message="加载中" />
          ) : isError ? (
            <TeamListStateRow message="团队列表加载失败" tone="destructive" />
          ) : teams.length === 0 ? (
            <TeamListStateRow message="暂无团队" />
          ) : (
            teams.map((team) => (
              <TableRow
                className={
                  team.id === highlightedTeamId
                    ? "bg-[var(--superteam-menu-accent-soft)]"
                    : undefined
                }
                key={team.id}
              >
                <TableCell>
                  <div className="flex min-w-0 items-center gap-3">
                    <TeamIconTile metadata={getTeamMetadata(team)} />
                    <div className="min-w-0">
                      <a
                        className="truncate font-medium hover:underline"
                        href={`/teams/${team.id}`}
                      >
                        {team.name}
                      </a>
                      <div className="mt-1 flex items-center gap-2 text-xs text-muted-foreground">
                        <span className="truncate">{team.slug}</span>
                        <TeamStatusBadge status={team.status} />
                      </div>
                    </div>
                  </div>
                </TableCell>
                <TableCell>
                  <TeamOwnerIdentity team={team} />
                </TableCell>
                <TableCell>{team.member_count}</TableCell>
                <TableCell>{team.digital_employee_count}</TableCell>
                <TableCell>{team.capability_count}</TableCell>
                <TableCell>
                  <GovernanceStatusBadge status={team.governance_status} />
                </TableCell>
                <TableCell>
                  {team.current_revision
                    ? `v${team.current_revision}`
                    : "未配置"}
                </TableCell>
                <TableCell>{team.pending_draft_count}</TableCell>
                <TableCell>
                  {team.updated_at
                    ? new Date(team.updated_at).toLocaleString("zh-CN")
                    : "-"}
                </TableCell>
                <TableCell>
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        aria-label={`${team.name} 行操作`}
                        size="icon"
                        type="button"
                        variant="ghost"
                      >
                        <MoreHorizontal />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuGroup>
                        <DropdownMenuItem asChild>
                          <a href={`/teams/${team.id}`}>查看详情</a>
                        </DropdownMenuItem>
                      </DropdownMenuGroup>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
      <div className="flex items-center justify-between border-t px-3 py-3 text-sm text-muted-foreground">
        <span>第 {pageIndex + 1} 页</span>
        <div className="flex items-center gap-2">
          <Button
            disabled={pageIndex === 0 || isLoading}
            onClick={() => onPageChange(pageIndex - 1)}
            size="sm"
            type="button"
            variant="outline"
          >
            上一页
          </Button>
          <select
            aria-label="每页数量"
            className="h-9 rounded-md border bg-background px-2 text-sm"
            onChange={(event) => onPageSizeChange(Number(event.target.value))}
            value={pageSize}
          >
            {[10, 20, 50].map((size) => (
              <option key={size} value={size}>
                {size} 条/页
              </option>
            ))}
          </select>
          <Button
            disabled={!canGoNext || isLoading}
            onClick={() => onPageChange(pageIndex + 1)}
            size="sm"
            type="button"
            variant="outline"
          >
            下一页
          </Button>
        </div>
      </div>
    </div>
  );
}

function TeamListStateRow({
  message,
  tone = "muted",
}: {
  message: string;
  tone?: "destructive" | "muted";
}) {
  return (
    <TableRow>
      <TableCell
        className={
          tone === "destructive"
            ? "h-24 text-center text-sm text-destructive"
            : "h-24 text-center text-sm text-muted-foreground"
        }
        colSpan={TEAM_LIST_COLUMN_COUNT}
      >
        {message}
      </TableCell>
    </TableRow>
  );
}

function TeamOwnerIdentity({ team }: { team: TeamListItem }) {
  const owner = getTeamOwnerIdentity(team);

  if (!owner) {
    return <span className="text-sm text-muted-foreground">未设置</span>;
  }

  return <UserIdentity showSecondary size="sm" user={owner} />;
}

function getTeamOwnerIdentity(team: TeamListItem): UserIdentityData | undefined {
  if (team.human_owner) {
    return {
      avatar: team.human_owner.avatar,
      display_name: team.human_owner.display_name,
      email: team.human_owner.email,
      id: team.human_owner.user_id,
      status: team.human_owner.status,
      username: team.human_owner.username,
    };
  }

  if (team.human_owner_user_id) {
    return {
      id: team.human_owner_user_id,
      status: "active",
    };
  }

  return undefined;
}

function getTeamMetadata(team: TeamListItem): TeamDisplayMetadata {
  return (team.metadata ?? {}) as TeamDisplayMetadata;
}

export function TeamStatusBadge({ status }: { status: TeamStatus }) {
  const label: Record<TeamStatus, string> = {
    active: "活跃",
    archived: "已归档",
    disabled: "已禁用",
  };

  return (
    <Badge variant={status === "active" ? "default" : "secondary"}>
      {label[status]}
    </Badge>
  );
}

function GovernanceStatusBadge({
  status,
}: {
  status: GovernanceSummaryStatus;
}) {
  const label: Record<GovernanceSummaryStatus, string> = {
    active: "已生效",
    draft_pending: "草案待批准",
    needs_update: "需更新",
    not_configured: "未配置",
  };
  const variant: Record<
    GovernanceSummaryStatus,
    "default" | "outline" | "secondary"
  > = {
    active: "default",
    draft_pending: "outline",
    needs_update: "outline",
    not_configured: "secondary",
  };

  return <Badge variant={variant[status]}>{label[status]}</Badge>;
}

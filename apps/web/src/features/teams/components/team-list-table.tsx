import { Badge } from "@/components/ui/badge";
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

type TeamListTableProps = {
  teams: TeamListItem[];
  highlightedTeamId?: string;
  isError?: boolean;
  isLoading?: boolean;
};

export function TeamListTable({
  highlightedTeamId,
  isError,
  isLoading,
  teams,
}: TeamListTableProps) {
  if (isLoading) {
    return <p className="text-sm text-muted-foreground">加载中</p>;
  }

  if (isError) {
    return <p className="text-sm text-destructive">团队列表加载失败</p>;
  }

  if (teams.length === 0) {
    return <p className="text-sm text-muted-foreground">暂无团队</p>;
  }

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
          </TableRow>
        </TableHeader>
        <TableBody>
          {teams.map((team) => (
            <TableRow
              className={
                team.id === highlightedTeamId
                  ? "bg-[var(--superteam-menu-accent-soft)]"
                  : undefined
              }
              key={team.id}
            >
              <TableCell>
                <a
                  className="font-medium hover:underline"
                  href={`/teams/${team.id}`}
                >
                  {team.name}
                </a>
                <div className="mt-1 flex items-center gap-2 text-xs text-muted-foreground">
                  <span>{team.slug}</span>
                  <TeamStatusBadge status={team.status} />
                </div>
              </TableCell>
              <TableCell>
                <div className="font-medium">{teamOwnerLabel(team)}</div>
                {team.human_owner?.email ? (
                  <div className="text-xs text-muted-foreground">
                    {team.human_owner.email}
                  </div>
                ) : null}
              </TableCell>
              <TableCell>{team.member_count}</TableCell>
              <TableCell>{team.digital_employee_count}</TableCell>
              <TableCell>{team.capability_count}</TableCell>
              <TableCell>
                <GovernanceStatusBadge status={team.governance_status} />
              </TableCell>
              <TableCell>
                {team.current_revision ? `v${team.current_revision}` : "未配置"}
              </TableCell>
              <TableCell>{team.pending_draft_count}</TableCell>
              <TableCell>
                {team.updated_at
                  ? new Date(team.updated_at).toLocaleString("zh-CN")
                  : "-"}
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}

function teamOwnerLabel(team: TeamListItem) {
  if (team.human_owner) {
    return (
      team.human_owner.display_name ||
      team.human_owner.username ||
      team.human_owner.email ||
      team.human_owner.user_id
    );
  }
  return team.human_owner_user_id ?? "未设置";
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

  return (
    <Badge variant={status === "active" ? "default" : "secondary"}>
      {label[status]}
    </Badge>
  );
}

import { useQuery } from "@tanstack/react-query";
import { ShieldAlert, Users } from "lucide-react";
import type { ApiClientOptions, AuthzMemberRecord } from "@/lib/api";
import { listAuthzMembers } from "@/lib/api";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";

type MemberRolesProps = {
  apiOptions: ApiClientOptions;
};

export function MemberRoles({ apiOptions }: MemberRolesProps) {
  const membersQuery = useQuery({
    queryKey: ["authz-members", apiOptions.baseUrl, 50, 0],
    queryFn: () => listAuthzMembers({ ...apiOptions, limit: 50, offset: 0 }),
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Users />
          成员角色
        </CardTitle>
        <CardDescription>当前只读展示成员、租户/团队角色和控制台访问能力。</CardDescription>
      </CardHeader>
      <CardContent>
        {membersQuery.isLoading ? (
          <Skeleton className="h-40" />
        ) : membersQuery.isError ? (
          <Alert variant="destructive">
            <ShieldAlert />
            <AlertTitle>成员角色加载失败</AlertTitle>
            <AlertDescription>请稍后刷新或检查 Control Plane 连接。</AlertDescription>
          </Alert>
        ) : (membersQuery.data?.items.length ?? 0) === 0 ? (
          <p className="text-sm text-muted-foreground">暂无成员角色记录。</p>
        ) : (
          <MemberRolesTable members={membersQuery.data?.items ?? []} />
        )}
      </CardContent>
    </Card>
  );
}

function MemberRolesTable({ members }: { members: AuthzMemberRecord[] }) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>成员</TableHead>
          <TableHead>账号状态</TableHead>
          <TableHead>控制台</TableHead>
          <TableHead>角色</TableHead>
          <TableHead>最近拒绝原因</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {members.map((member) => (
          <TableRow key={member.user_id}>
            <TableCell>
              <div className="flex flex-col gap-1">
                <span className="font-medium">{member.display_name || member.username}</span>
                <span className="text-xs text-muted-foreground">{member.email ?? member.user_id}</span>
              </div>
            </TableCell>
            <TableCell>
              <Badge variant={member.account_status === "active" ? "default" : "secondary"}>{member.account_status}</Badge>
            </TableCell>
            <TableCell>
              <Badge variant={member.console_access ? "default" : "secondary"}>{member.console_access ? "允许" : "无访问"}</Badge>
            </TableCell>
            <TableCell>{formatMemberships(member)}</TableCell>
            <TableCell className="max-w-72 truncate">{member.recent_denied_reason ?? "-"}</TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

function formatMemberships(member: AuthzMemberRecord) {
  if (member.memberships.length === 0) {
    return "无角色";
  }

  return member.memberships
    .map((membership) => {
      const scope = membership.team_id ? `team:${membership.team_id}` : `tenant:${membership.tenant_id}`;
      return `${membership.role} / ${scope} / ${membership.status}`;
    })
    .join("; ");
}

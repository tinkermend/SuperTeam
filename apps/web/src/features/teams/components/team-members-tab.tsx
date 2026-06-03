import { useMemo, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Check, ShieldAlert, Trash2, UserPlus, X } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import {
  addTeamMember,
  approveTeamMemberRoleRequest,
  createTeamMemberRoleRequest,
  listTeamMemberRoleRequests,
  listTeamMembers,
  rejectTeamMemberRoleRequest,
  removeTeamMember,
  type AllowedTeamAction,
  type TeamMember,
  type TeamMemberRole,
  type TeamMemberRoleRequest,
} from "@/lib/api/teams";

type TeamMembersTabProps = {
  allowedActions: AllowedTeamAction[];
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  teamId: string;
};

const roleLabels: Record<TeamMemberRole, string> = {
  owner: "负责人",
  admin: "管理员",
  approver: "审批人",
  member: "普通成员",
  viewer: "只读观察者",
};

const directRoles = [
  { label: "普通成员", value: "member" },
  { label: "只读观察者", value: "viewer" },
] as const;

const privilegedRoles = [
  { label: "负责人", value: "owner" },
  { label: "管理员", value: "admin" },
  { label: "审批人", value: "approver" },
] as const;

export function TeamMembersTab({ allowedActions, apiBaseUrl, fetcher, teamId }: TeamMembersTabProps) {
  const apiOptions = useMemo(() => ({ baseUrl: apiBaseUrl, fetcher }), [apiBaseUrl, fetcher]);
  const canAddMember = allowedActions.includes("team.member.add");
  const canRequestPrivilegedRole = allowedActions.includes("team.member.request_privileged_role");
  const members = useQuery({
    queryKey: ["team-members", teamId],
    queryFn: () => listTeamMembers(apiOptions, teamId),
  });
  const roleRequests = useQuery({
    queryKey: ["team-member-role-requests", teamId, "pending"],
    queryFn: () => listTeamMemberRoleRequests(apiOptions, teamId, "pending"),
  });
  const refetchRoster = () => {
    void members.refetch();
    void roleRequests.refetch();
  };
  const addMutation = useMutation({
    mutationFn: (input: { role: "member" | "viewer"; user_id: string }) => addTeamMember(apiOptions, teamId, input),
    onSuccess: refetchRoster,
  });
  const removeMutation = useMutation({
    mutationFn: (memberId: string) => removeTeamMember(apiOptions, teamId, memberId),
    onSuccess: refetchRoster,
  });
  const createRequestMutation = useMutation({
    mutationFn: (input: { reason: string; requested_role: "owner" | "admin" | "approver"; target_user_id: string }) =>
      createTeamMemberRoleRequest(apiOptions, teamId, input),
    onSuccess: refetchRoster,
  });
  const approveMutation = useMutation({
    mutationFn: (requestId: string) =>
      approveTeamMemberRoleRequest(apiOptions, teamId, requestId, { decision_reason: "控制台审批通过" }),
    onSuccess: refetchRoster,
  });
  const rejectMutation = useMutation({
    mutationFn: (requestId: string) =>
      rejectTeamMemberRoleRequest(apiOptions, teamId, requestId, { decision_reason: "控制台拒绝" }),
    onSuccess: refetchRoster,
  });
  const roster = members.data ?? [];
  const requests = roleRequests.data ?? [];
  const counters = countMembers(roster);

  return (
    <div className="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
      <div className="flex min-w-0 flex-col gap-4">
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
          <Metric label="人类成员" value={roster.length} />
          <Metric label="负责人" value={counters.owner} />
          <Metric label="管理员" value={counters.admin} />
          <Metric label="审批人" value={counters.approver} />
          <Metric label="直接生效角色" value={counters.direct} />
        </div>

        <Alert>
          <ShieldAlert />
          <AlertTitle>最终负责人保护</AlertTitle>
          <AlertDescription>
            删除或禁用最后一位负责人会被控制平面拒绝；请先完成负责人交接，再移除原负责人。
          </AlertDescription>
        </Alert>

        <Card className="rounded-md">
          <CardHeader>
            <CardTitle>成员名册</CardTitle>
            <CardDescription>按团队角色排序，面向人类成员的权限边界。</CardDescription>
          </CardHeader>
          <CardContent>
            {members.isLoading ? <p className="text-sm text-muted-foreground">加载成员中</p> : null}
            {members.isError ? <p className="text-sm text-destructive">成员加载失败</p> : null}
            {!members.isLoading && roster.length === 0 ? (
              <p className="text-sm text-muted-foreground">当前团队还没有人类成员。</p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>成员</TableHead>
                    <TableHead>角色</TableHead>
                    <TableHead>账号状态</TableHead>
                    <TableHead className="text-right">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {roster.map((member) => (
                    <MemberRow
                      key={member.membership_id}
                      member={member}
                      onRemove={() => removeMutation.mutate(member.membership_id)}
                    />
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
      </div>

      <aside className="flex min-w-0 flex-col gap-4">
        <DirectAddPanel
          canAdd={canAddMember}
          isPending={addMutation.isPending}
          onSubmit={(input) => addMutation.mutate(input)}
        />
        <PrivilegedRequestPanel
          canRequest={canRequestPrivilegedRole}
          isPending={createRequestMutation.isPending}
          onSubmit={(input) => createRequestMutation.mutate(input)}
        />
        <PendingRequestsPanel
          approvePending={approveMutation.isPending}
          onApprove={(requestId) => approveMutation.mutate(requestId)}
          onReject={(requestId) => rejectMutation.mutate(requestId)}
          rejectPending={rejectMutation.isPending}
          requests={requests}
        />
      </aside>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-md border p-3">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className="mt-2 text-2xl font-semibold">{value}</p>
    </div>
  );
}

function MemberRow({ member, onRemove }: { member: TeamMember; onRemove: () => void }) {
  const displayName = member.display_name || member.username;

  return (
    <TableRow>
      <TableCell>
        <div className="flex min-w-0 flex-col">
          <span className="font-medium">{displayName}</span>
          <span className="text-xs text-muted-foreground">{member.email || member.user_id}</span>
        </div>
      </TableCell>
      <TableCell>
        <Badge variant={member.role === "owner" ? "default" : "secondary"}>{roleLabels[member.role]}</Badge>
      </TableCell>
      <TableCell>{member.account_status || member.membership_status}</TableCell>
      <TableCell className="text-right">
        <Button onClick={onRemove} size="sm" variant="ghost">
          <Trash2 data-icon="inline-start" />
          移除
        </Button>
      </TableCell>
    </TableRow>
  );
}

function DirectAddPanel({
  canAdd,
  isPending,
  onSubmit,
}: {
  canAdd: boolean;
  isPending: boolean;
  onSubmit: (input: { role: "member" | "viewer"; user_id: string }) => void;
}) {
  const [userId, setUserId] = useState("");
  const [role, setRole] = useState<"member" | "viewer">("member");

  return (
    <Card className="rounded-md">
      <CardHeader>
        <CardTitle>直接添加</CardTitle>
        <CardDescription>普通成员和只读观察者会立即生效。</CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            if (userId.trim()) {
              onSubmit({ role, user_id: userId.trim() });
            }
          }}
        >
          <div className="flex flex-col gap-2">
            <Label htmlFor="team-member-user-id">用户 ID</Label>
            <Input
              disabled={!canAdd || isPending}
              id="team-member-user-id"
              onChange={(event) => setUserId(event.target.value)}
              placeholder="auth user uuid"
              value={userId}
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="team-member-role">直接生效角色</Label>
            <select
              className="h-9 rounded-md border bg-background px-3 text-sm shadow-xs"
              disabled={!canAdd || isPending}
              id="team-member-role"
              onChange={(event) => setRole(event.target.value as "member" | "viewer")}
              value={role}
            >
              {directRoles.map((directRole) => (
                <option key={directRole.value} value={directRole.value}>
                  {directRole.label}
                </option>
              ))}
            </select>
          </div>
          <Button disabled={!canAdd || isPending || !userId.trim()} type="submit">
            <UserPlus data-icon="inline-start" />
            添加成员
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

function PrivilegedRequestPanel({
  canRequest,
  isPending,
  onSubmit,
}: {
  canRequest: boolean;
  isPending: boolean;
  onSubmit: (input: { reason: string; requested_role: "owner" | "admin" | "approver"; target_user_id: string }) => void;
}) {
  const [targetUserId, setTargetUserId] = useState("");
  const [requestedRole, setRequestedRole] = useState<"owner" | "admin" | "approver">("admin");
  const [reason, setReason] = useState("");

  return (
    <Card className="rounded-md">
      <CardHeader>
        <CardTitle>高权限申请</CardTitle>
        <CardDescription>负责人、管理员和审批人需要审批后生效。</CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            if (targetUserId.trim()) {
              onSubmit({
                reason: reason.trim(),
                requested_role: requestedRole,
                target_user_id: targetUserId.trim(),
              });
            }
          }}
        >
          <div className="flex flex-col gap-2">
            <Label htmlFor="team-member-request-user-id">目标用户 ID</Label>
            <Input
              disabled={!canRequest || isPending}
              id="team-member-request-user-id"
              onChange={(event) => setTargetUserId(event.target.value)}
              placeholder="auth user uuid"
              value={targetUserId}
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="team-member-request-role">申请角色</Label>
            <select
              className="h-9 rounded-md border bg-background px-3 text-sm shadow-xs"
              disabled={!canRequest || isPending}
              id="team-member-request-role"
              onChange={(event) => setRequestedRole(event.target.value as "owner" | "admin" | "approver")}
              value={requestedRole}
            >
              {privilegedRoles.map((privilegedRole) => (
                <option key={privilegedRole.value} value={privilegedRole.value}>
                  {privilegedRole.label}
                </option>
              ))}
            </select>
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="team-member-request-reason">申请原因</Label>
            <Textarea
              disabled={!canRequest || isPending}
              id="team-member-request-reason"
              onChange={(event) => setReason(event.target.value)}
              value={reason}
            />
          </div>
          <Button disabled={!canRequest || isPending || !targetUserId.trim()} type="submit" variant="outline">
            <ShieldAlert data-icon="inline-start" />
            提交申请
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

function PendingRequestsPanel({
  approvePending,
  onApprove,
  onReject,
  rejectPending,
  requests,
}: {
  approvePending: boolean;
  onApprove: (requestId: string) => void;
  onReject: (requestId: string) => void;
  rejectPending: boolean;
  requests: TeamMemberRoleRequest[];
}) {
  return (
    <Card className="rounded-md">
      <CardHeader>
        <CardTitle>待审批高权限</CardTitle>
        <CardDescription>审批后由控制平面写入团队成员角色。</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {requests.length === 0 ? <p className="text-sm text-muted-foreground">暂无待审批申请。</p> : null}
        {requests.map((request) => (
          <div key={request.id} className="flex flex-col gap-3 rounded-md border p-3">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <p className="text-sm font-medium">{request.target_user_id}</p>
                <p className="text-xs text-muted-foreground">申请成为 {roleLabels[request.requested_role]}</p>
              </div>
              <Badge variant="outline">待审批</Badge>
            </div>
            <p className="text-sm text-muted-foreground">{request.reason || "未填写申请原因"}</p>
            <Separator />
            <div className="flex justify-end gap-2">
              <Button disabled={rejectPending} onClick={() => onReject(request.id)} size="sm" variant="outline">
                <X data-icon="inline-start" />
                拒绝
              </Button>
              <Button disabled={approvePending} onClick={() => onApprove(request.id)} size="sm">
                <Check data-icon="inline-start" />
                审批
              </Button>
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

function countMembers(members: TeamMember[]) {
  return members.reduce(
    (result, member) => {
      if (member.role === "owner") {
        result.owner += 1;
      }
      if (member.role === "admin") {
        result.admin += 1;
      }
      if (member.role === "approver") {
        result.approver += 1;
      }
      if (member.role === "member" || member.role === "viewer") {
        result.direct += 1;
      }
      return result;
    },
    { admin: 0, approver: 0, direct: 0, owner: 0 },
  );
}

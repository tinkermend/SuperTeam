import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Check, ShieldAlert, Trash2, UserPlus, X, Bot, Users, Puzzle, TriangleAlert, ShieldCheck } from "lucide-react";
import { MetricCard } from "@/components/superteam/liquid-components";
import {
  TeamRoleBadge,
  TeamRoleSelect,
  teamRoleLabel,
  type DirectTeamRole,
  type PrivilegedTeamRole,
} from "@/components/superteam/team-role";
import { UserIdentity } from "@/components/superteam/user-identity";
import { UserSearchSelect } from "@/components/superteam/user-search-select";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Separator } from "@/components/ui/separator";
import { Textarea } from "@/components/ui/textarea";
import type { UserSummary } from "@/lib/api";
import type { TeamOverview, AllowedTeamAction, TeamMember, TeamMemberRoleRequest } from "@/lib/api/teams";
import {
  addTeamMember,
  approveTeamMemberRoleRequest,
  createTeamMemberRoleRequest,
  listTeamMemberRoleRequests,
  listTeamMembers,
  rejectTeamMemberRoleRequest,
  removeTeamMember,
} from "@/lib/api/teams";
import type { DigitalEmployee } from "@/lib/api/employees";
import { listDigitalEmployees } from "@/lib/api/employees";

type TeamOverviewTabProps = {
  allowedActions: AllowedTeamAction[];
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  overview: TeamOverview;
  teamId: string;
};

// 统一成员视图
type UnifiedMember = {
  id: string;
  type: "human" | "digital";
  name: string;
  description: string;
  role: string;
  status: string;
  originalData: TeamMember | DigitalEmployee;
};

const roleOrder: Record<string, number> = {
  owner: 1,
  admin: 2,
  approver: 3,
  member: 4,
  viewer: 5,
};

export function TeamOverviewTab({ allowedActions, apiBaseUrl, fetcher, overview, teamId }: TeamOverviewTabProps) {
  const { member_count, digital_employee_count, capability_count, pending_item_count } = overview;
  
  const apiOptions = useMemo(() => ({ baseUrl: apiBaseUrl, fetcher }), [apiBaseUrl, fetcher]);
  const canAddMember = allowedActions.includes("team.member.add");
  const canRequestPrivilegedRole = allowedActions.includes("team.member.request_privileged_role");
  
  const [directAddResetToken, setDirectAddResetToken] = useState(0);
  const [privilegedRequestResetToken, setPrivilegedRequestResetToken] = useState(0);

  // Queries
  const membersQuery = useQuery({
    queryKey: ["team-members", teamId],
    queryFn: () => listTeamMembers(apiOptions, teamId),
  });
  
  const digitalEmployeesQuery = useQuery({
    queryKey: ["team-digital-employees", teamId],
    queryFn: () => listDigitalEmployees(apiOptions, { team_id: teamId }),
  });

  const roleRequestsQuery = useQuery({
    queryKey: ["team-member-role-requests", teamId, "pending"],
    queryFn: () => listTeamMemberRoleRequests(apiOptions, teamId, "pending"),
  });

  const refetchRoster = () => {
    void membersQuery.refetch();
    void roleRequestsQuery.refetch();
  };

  // Mutations
  const addMutation = useMutation({
    mutationFn: (input: { role: "member" | "viewer"; user_id: string }) => addTeamMember(apiOptions, teamId, input),
    onSuccess: () => {
      refetchRoster();
      setDirectAddResetToken((token) => token + 1);
    },
  });

  const removeMutation = useMutation({
    mutationFn: (memberId: string) => removeTeamMember(apiOptions, teamId, memberId),
    onSuccess: refetchRoster,
  });

  const createRequestMutation = useMutation({
    mutationFn: (input: { reason: string; requested_role: "owner" | "admin" | "approver"; target_user_id: string }) =>
      createTeamMemberRoleRequest(apiOptions, teamId, input),
    onSuccess: () => {
      refetchRoster();
      setPrivilegedRequestResetToken((token) => token + 1);
    },
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

  // Combine and sort
  const humanRoster = membersQuery.data ?? [];
  const digitalRoster = digitalEmployeesQuery.data ?? [];
  const requests = roleRequestsQuery.data ?? [];
  const existingUserIds = humanRoster.map((member) => member.user_id);

  const unifiedList: UnifiedMember[] = [
    ...humanRoster.map(
      (m): UnifiedMember => ({
        id: m.membership_id,
        type: "human",
        name: m.display_name || m.username,
        description: m.email || "人类成员",
        role: m.role,
        status: m.account_status || m.membership_status,
        originalData: m,
      })
    ),
    ...digitalRoster.map(
      (d): UnifiedMember => ({
        id: d.id,
        type: "digital",
        name: d.name,
        description: d.description || "执行代理",
        role: d.role,
        status: d.status,
        originalData: d,
      })
    ),
  ].sort((a, b) => {
    const roleA = roleOrder[a.role] || 99;
    const roleB = roleOrder[b.role] || 99;
    if (roleA !== roleB) return roleA - roleB;
    return a.name.localeCompare(b.name);
  });

  const isLoadingList = membersQuery.isLoading || digitalEmployeesQuery.isLoading;

  return (
    <div className="flex flex-col gap-6">
      {/* 核心指标 */}
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <MetricCard title="人类成员" value={member_count} icon={<Users />} meta="当前团队成员" />
        <MetricCard title="数字员工" value={digital_employee_count} icon={<Bot />} meta="AI 代理执行引擎" />
        <MetricCard title="绑定能力" value={capability_count} icon={<Puzzle />} meta="MCP 与外部工具" />
        <MetricCard
          title="待审批项"
          value={pending_item_count}
          icon={<TriangleAlert />}
          meta="需人类介入决策"
          statusTone={pending_item_count > 0 ? "warning" : "success"}
        />
      </div>

      {/* 主体左右布局 */}
      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_360px]">
        {/* 左侧：统一混合列表 */}
        <div className="flex min-w-0 flex-col gap-4">
          <Card className="rounded-2xl">
            <CardHeader className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between border-b pb-4">
              <div>
                <CardTitle>团队成员与代理</CardTitle>
                <CardDescription className="mt-1">统一查看人类成员与数字员工。</CardDescription>
              </div>
              <Button asChild size="sm" variant="outline">
                <Link to="/employees/new">
                  <Bot data-icon="inline-start" className="mr-1" />
                  新建数字员工
                </Link>
              </Button>
            </CardHeader>
            <CardContent className="p-0">
              {isLoadingList ? (
                <div className="p-6 text-center text-sm text-muted-foreground">加载数据中...</div>
              ) : unifiedList.length === 0 ? (
                <div className="p-6 text-center text-sm text-muted-foreground">团队暂无成员和代理。</div>
              ) : (
                <ul className="flex flex-col">
                  {unifiedList.map((item, index) => (
                    <li
                      key={item.id}
                      className={`flex flex-col sm:flex-row sm:items-center justify-between gap-4 p-4 hover:bg-muted/30 transition-colors ${
                        index !== unifiedList.length - 1 ? "border-b" : ""
                      }`}
                    >
                      {/* 左侧：头像与基本信息 */}
                      <div className="flex items-center gap-3 overflow-hidden">
                        {item.type === "human" ? (
                          <UserIdentity
                            showSecondary
                            user={{
                              id: (item.originalData as TeamMember).user_id,
                              username: (item.originalData as TeamMember).username,
                              display_name: (item.originalData as TeamMember).display_name,
                              email: (item.originalData as TeamMember).email,
                              avatar: (item.originalData as TeamMember).avatar,
                            }}
                          />
                        ) : (
                          <div className="flex items-center gap-3 min-w-0">
                            <div className="flex size-10 shrink-0 items-center justify-center rounded-full border bg-muted">
                              <Bot className="size-5" />
                            </div>
                            <div className="min-w-0">
                              <p className="truncate font-medium leading-none">{item.name}</p>
                              <p className="truncate text-sm text-muted-foreground mt-1.5">{item.description}</p>
                            </div>
                          </div>
                        )}
                      </div>

                      {/* 右侧：状态标签与操作 */}
                      <div className="flex items-center gap-3 shrink-0">
                        <Badge variant="outline" className={item.type === "human" ? "text-blue-600" : "text-emerald-600"}>
                          {item.type === "human" ? "人类" : "数字员工"}
                        </Badge>
                        <TeamRoleBadge role={item.role as PrivilegedTeamRole | DirectTeamRole} />
                        {item.type === "human" && (
                          <Button 
                            onClick={() => removeMutation.mutate(item.id)} 
                            size="sm" 
                            variant="ghost" 
                            className="text-muted-foreground hover:text-destructive"
                          >
                            <Trash2 className="size-4" />
                            <span className="sr-only">移除</span>
                          </Button>
                        )}
                        {item.type === "digital" && (
                          <Button asChild size="sm" variant="ghost" className="text-muted-foreground">
                            <Link to="/employees/$employeeId" params={{ employeeId: item.id }}>
                              管理
                            </Link>
                          </Button>
                        )}
                      </div>
                    </li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>
        </div>

        {/* 右侧：管控面板 */}
        <aside className="flex min-w-0 flex-col gap-4">
          <DirectAddPanel
            apiBaseUrl={apiBaseUrl}
            canAdd={canAddMember}
            existingUserIds={existingUserIds}
            fetcher={fetcher}
            isPending={addMutation.isPending}
            onSubmit={(input) => addMutation.mutate(input)}
            resetToken={directAddResetToken}
          />
          <PrivilegedRequestPanel
            apiBaseUrl={apiBaseUrl}
            canRequest={canRequestPrivilegedRole}
            existingUserIds={existingUserIds}
            fetcher={fetcher}
            isPending={createRequestMutation.isPending}
            onSubmit={(input) => createRequestMutation.mutate(input)}
            resetToken={privilegedRequestResetToken}
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
    </div>
  );
}

// === Panels ===

function DirectAddPanel({
  apiBaseUrl,
  canAdd,
  existingUserIds,
  fetcher,
  isPending,
  onSubmit,
  resetToken,
}: {
  apiBaseUrl: string;
  canAdd: boolean;
  existingUserIds: string[];
  fetcher?: typeof fetch;
  isPending: boolean;
  onSubmit: (input: { role: DirectTeamRole; user_id: string }) => void;
  resetToken: number;
}) {
  const [selectedUser, setSelectedUser] = useState<UserSummary | undefined>();
  const [role, setRole] = useState<DirectTeamRole>("member");

  useEffect(() => {
    setSelectedUser(undefined);
    setRole("member");
  }, [resetToken]);

  return (
    <Card className="rounded-2xl">
      <CardHeader>
        <CardTitle className="text-base">直接添加人类成员</CardTitle>
        <CardDescription>普通成员和只读观察者会立即生效。</CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            if (selectedUser) {
              onSubmit({ role, user_id: selectedUser.id });
            }
          }}
        >
          <div className="flex flex-col gap-2">
            <Label>用户</Label>
            <UserSearchSelect
              apiBaseUrl={apiBaseUrl}
              disabled={!canAdd || isPending}
              excludedUserIds={existingUserIds}
              fetcher={fetcher}
              inputLabel="搜索用户"
              onSelect={setSelectedUser}
              value={selectedUser}
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label>直接生效角色</Label>
            <TeamRoleSelect
              ariaLabel="直接生效角色"
              disabled={!canAdd || isPending}
              mode="direct"
              onChange={setRole}
              value={role}
            />
          </div>
          <Button disabled={!canAdd || isPending || !selectedUser} type="submit">
            <UserPlus data-icon="inline-start" className="mr-2" />
            添加成员
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

function PrivilegedRequestPanel({
  apiBaseUrl,
  canRequest,
  existingUserIds,
  fetcher,
  isPending,
  onSubmit,
  resetToken,
}: {
  apiBaseUrl: string;
  canRequest: boolean;
  existingUserIds: string[];
  fetcher?: typeof fetch;
  isPending: boolean;
  onSubmit: (input: { reason: string; requested_role: PrivilegedTeamRole; target_user_id: string }) => void;
  resetToken: number;
}) {
  const [selectedUser, setSelectedUser] = useState<UserSummary | undefined>();
  const [requestedRole, setRequestedRole] = useState<PrivilegedTeamRole>("admin");
  const [reason, setReason] = useState("");

  useEffect(() => {
    setSelectedUser(undefined);
    setRequestedRole("admin");
    setReason("");
  }, [resetToken]);

  return (
    <Card className="rounded-2xl">
      <CardHeader>
        <CardTitle className="text-base">高权限申请</CardTitle>
        <CardDescription>负责人、管理员和审批人需要审批后生效。</CardDescription>
      </CardHeader>
      <CardContent>
        <form
          className="flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            if (selectedUser) {
              onSubmit({
                reason: reason.trim(),
                requested_role: requestedRole,
                target_user_id: selectedUser.id,
              });
            }
          }}
        >
          <div className="flex flex-col gap-2">
            <Label>目标用户</Label>
            <UserSearchSelect
              apiBaseUrl={apiBaseUrl}
              disabled={!canRequest || isPending}
              excludedUserIds={existingUserIds}
              fetcher={fetcher}
              inputLabel="搜索用户"
              onSelect={setSelectedUser}
              value={selectedUser}
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label>申请角色</Label>
            <TeamRoleSelect
              ariaLabel="申请角色"
              disabled={!canRequest || isPending}
              mode="privileged"
              onChange={setRequestedRole}
              value={requestedRole}
            />
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
          <Button disabled={!canRequest || isPending || !selectedUser} type="submit" variant="outline">
            <ShieldAlert data-icon="inline-start" className="mr-2" />
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
  if (requests.length === 0) return null;

  return (
    <Card className="rounded-2xl border-superteam-warning/50 bg-superteam-warning/5 shadow-sm">
      <CardHeader>
        <CardTitle className="text-base flex items-center gap-2 text-superteam-warning">
          <ShieldCheck className="size-4" />
          待审批高权限
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-3">
        {requests.map((request) => (
          <div key={request.id} className="flex flex-col gap-3 rounded-xl border bg-background p-3 shadow-sm">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <p className="text-sm font-medium">{request.target_user_id}</p>
                <p className="text-xs text-muted-foreground mt-0.5">申请成为 {teamRoleLabel(request.requested_role)}</p>
              </div>
            </div>
            <p className="text-sm text-muted-foreground">{request.reason || "未填写申请原因"}</p>
            <Separator />
            <div className="flex justify-end gap-2">
              <Button disabled={rejectPending} onClick={() => onReject(request.id)} size="sm" variant="ghost" className="text-muted-foreground">
                拒绝
              </Button>
              <Button disabled={approvePending} onClick={() => onApprove(request.id)} size="sm">
                审批通过
              </Button>
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  );
}

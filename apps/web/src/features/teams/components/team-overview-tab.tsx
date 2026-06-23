import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Trash2, UserPlus, Bot, Users, Puzzle, TriangleAlert } from "lucide-react";
import {
  SoftCard,
  StatusPill,
  V3Button,
  V3EmptyState,
  V3LoadingState,
  V3MetricCard,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
} from "@/components/superteam";
import {
  TeamRoleBadge,
  TeamRoleSelect,
  type DirectTeamRole,
} from "@/components/superteam/team-role";
import { UserIdentity } from "@/components/superteam/user-identity";
import { UserSearchSelect } from "@/components/superteam/user-search-select";
import { Label } from "@/components/ui/label";
import type { UserSummary } from "@/lib/api";
import type { AllowedTeamAction, TeamMember, TeamOverview } from "@/lib/api/teams";
import {
  addTeamMember,
  listTeamMembers,
  removeTeamMember,
} from "@/lib/api/teams";
import type { DigitalEmployee } from "@/lib/api/employees";
import { listDigitalEmployees } from "@/lib/api/employees";
import { EmployeeAvatar } from "@/features/employees/avatar";
import { employeeAvatarAsset } from "@/features/employees/avatar-library";

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
  
  const [directAddResetToken, setDirectAddResetToken] = useState(0);

  // Queries
  const membersQuery = useQuery({
    queryKey: ["team-members", teamId],
    queryFn: () => listTeamMembers(apiOptions, teamId),
  });
  
  const digitalEmployeesQuery = useQuery({
    queryKey: ["team-digital-employees", teamId],
    queryFn: () => listDigitalEmployees(apiOptions, { team_id: teamId }),
  });

  const refetchRoster = () => {
    void membersQuery.refetch();
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

  // Combine and sort
  const humanRoster = membersQuery.data ?? [];
  const digitalRoster = digitalEmployeesQuery.data ?? [];
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
        <V3MetricCard label="人类成员" value={member_count} icon={<Users />} meta="当前团队成员" />
        <V3MetricCard label="数字员工" value={digital_employee_count} icon={<Bot />} iconTone="info" meta="AI 代理执行引擎" />
        <V3MetricCard label="绑定能力" value={capability_count} icon={<Puzzle />} iconTone="artifact" meta="MCP 与外部工具" />
        <V3MetricCard
          label="待审批项"
          value={pending_item_count}
          icon={<TriangleAlert />}
          meta="需人类介入决策"
          iconTone={pending_item_count > 0 ? "warn" : "ok"}
          loud={pending_item_count > 0}
        />
      </div>

      {/* 主体左右布局 */}
      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_360px]">
        {/* 左侧：统一混合列表 */}
        <div className="flex min-w-0 flex-col gap-4">
          <WorkSurface>
            <div className="flex flex-col gap-3 border-b border-v3-line px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
              <div>
                <h2 className="text-base font-bold text-v3-ink">团队成员与代理</h2>
                <p className="mt-1 text-[13px] text-v3-ink-2">统一查看人类成员与数字员工。</p>
              </div>
              <V3Button asChild size="sm" variant="outline">
                <Link to="/employees/new">
                  <Bot data-icon="inline-start" className="mr-1" />
                  新建数字员工
                </Link>
              </V3Button>
            </div>
            <div>
              {isLoadingList ? (
                <V3LoadingState label="加载团队成员与代理" />
              ) : unifiedList.length === 0 ? (
                <V3EmptyState title="团队暂无成员和代理" />
              ) : (
                <V3Table>
                  <thead>
                    <tr>
                      <V3Th>成员</V3Th>
                      <V3Th>类型</V3Th>
                      <V3Th>角色</V3Th>
                      <V3Th className="text-right">操作</V3Th>
                    </tr>
                  </thead>
                  <tbody>
                    {unifiedList.map((item) => (
                      <V3Tr key={item.id}>
                        <V3Td>
                          <div className="flex min-w-0 items-center gap-3">
                            {item.type === "human" ? (
                              <UserIdentity
                                showSecondary
                                user={{
                                  id: (item.originalData as TeamMember).user_id,
                                  username: (item.originalData as TeamMember).username,
                                  display_name: (item.originalData as TeamMember).display_name,
                                  email: (item.originalData as TeamMember).email,
                                  avatar: (item.originalData as TeamMember).avatar,
                                  status: (item.originalData as TeamMember).account_status || "active",
                                }}
                              />
                            ) : (
                              <div className="flex min-w-0 items-center gap-3">
                                <EmployeeAvatar
                                  asset={employeeAvatarAsset(item.originalData as DigitalEmployee)}
                                  name={item.name}
                                  size="md"
                                />
                                <div className="min-w-0">
                                  <p className="truncate font-medium leading-none text-v3-ink">{item.name}</p>
                                  <p className="mt-1.5 truncate text-sm text-v3-ink-2">{item.description}</p>
                                </div>
                              </div>
                            )}
                          </div>
                        </V3Td>
                        <V3Td>
                          <StatusPill tone={item.type === "human" ? "info" : "ok"}>
                            {item.type === "human" ? "人类" : "数字员工"}
                          </StatusPill>
                        </V3Td>
                        <V3Td>
                          <TeamRoleBadge role={item.role as DirectTeamRole} />
                        </V3Td>
                        <V3Td className="text-right">
                          {item.type === "human" ? (
                            <V3Button
                              aria-label={`移除 ${item.name}`}
                              onClick={() => removeMutation.mutate(item.id)}
                              size="icon"
                              type="button"
                              variant="ghost"
                            >
                              <Trash2 className="size-4" />
                              <span className="sr-only">移除</span>
                            </V3Button>
                          ) : (
                            <V3Button asChild size="sm" variant="ghost">
                              <Link to="/employees/$employeeId" params={{ employeeId: item.id }}>
                                管理
                              </Link>
                            </V3Button>
                          )}
                        </V3Td>
                      </V3Tr>
                    ))}
                  </tbody>
                </V3Table>
              )}
            </div>
          </WorkSurface>
        </div>

        {/* 右侧：添加成员面板 */}
        {canAddMember && (
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
          </aside>
        )}
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
    <SoftCard className="p-5">
      <div className="mb-4">
        <h2 className="text-base font-bold text-v3-ink">直接添加人类成员</h2>
        <p className="mt-1 text-[13px] text-v3-ink-2">普通成员和只读观察者会立即生效。</p>
      </div>
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
              inputLabel="搜索直接添加用户"
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
          <V3Button disabled={!canAdd || isPending || !selectedUser} type="submit">
            <UserPlus data-icon="inline-start" className="mr-2" />
            添加成员
          </V3Button>
        </form>
    </SoftCard>
  );
}

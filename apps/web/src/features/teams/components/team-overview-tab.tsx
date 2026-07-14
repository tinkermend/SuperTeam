import type { ReactNode } from "react";
import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Trash2, UserPlus, Bot, Users, Puzzle, TriangleAlert } from "lucide-react";
import {
  MasterDetailLayout,
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
import { employeeStatusLabel } from "@/lib/status-labels";

type TeamOverviewTabProps = {
  allowedActions: AllowedTeamAction[];
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  overview: TeamOverview;
  teamId: string;
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

  const humanRoster = membersQuery.data ?? [];
  const digitalRoster = digitalEmployeesQuery.data ?? [];
  const existingUserIds = humanRoster.map((member) => member.user_id);

  return (
    <div className="flex flex-col gap-6">
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

      <DigitalEmployeesSection
        employees={digitalRoster}
        isLoading={digitalEmployeesQuery.isLoading}
      />

      <HumanMembersSection
        addPanel={
          <DirectAddPanel
            apiBaseUrl={apiBaseUrl}
            canAdd={canAddMember}
            existingUserIds={existingUserIds}
            fetcher={fetcher}
            isPending={addMutation.isPending}
            onSubmit={(input) => addMutation.mutate(input)}
            resetToken={directAddResetToken}
          />
        }
        canAddMember={canAddMember}
        isLoading={membersQuery.isLoading}
        members={humanRoster}
        onRemove={(membershipId) => removeMutation.mutate(membershipId)}
        removing={removeMutation.isPending}
      />
    </div>
  );
}

// === Panels ===

function DigitalEmployeesSection({
  employees,
  isLoading,
}: {
  employees: DigitalEmployee[];
  isLoading: boolean;
}) {
  return (
    <WorkSurface>
      <div className="flex flex-col gap-3 border-b border-v3-line px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 className="text-base font-bold text-v3-ink">数字员工</h2>
          <p className="mt-1 text-[13px] text-v3-ink-2">团队当前绑定的数字员工。</p>
        </div>
        <V3Button asChild size="sm" variant="outline">
          <Link to="/employees/new">
            <Bot data-icon="inline-start" className="mr-1" />
            新建数字员工
          </Link>
        </V3Button>
      </div>
      <div>
        {isLoading ? (
          <V3LoadingState label="加载数字员工" />
        ) : employees.length === 0 ? (
          <V3EmptyState
            title="团队暂无数字员工"
            action={
              <V3Button asChild size="sm" variant="outline">
                <Link to="/employees/new">
                  <Bot data-icon="inline-start" className="mr-1" />
                  新建第一个数字员工
                </Link>
              </V3Button>
            }
          />
        ) : (
          <V3Table>
            <thead>
              <tr>
                <V3Th>数字员工</V3Th>
                <V3Th>职能</V3Th>
                <V3Th>状态</V3Th>
                <V3Th className="text-right">操作</V3Th>
              </tr>
            </thead>
            <tbody>
              {employees.map((employee) => (
                <V3Tr key={employee.id}>
                  <V3Td>
                    <div className="flex min-w-0 items-center gap-3">
                      <EmployeeAvatar
                        asset={employeeAvatarAsset(employee)}
                        name={employee.name}
                        size="md"
                      />
                      <div className="min-w-0">
                        <p className="truncate font-medium leading-none text-v3-ink">{employee.name}</p>
                        <p className="mt-1.5 truncate text-sm text-v3-ink-2">{employee.description || "执行代理"}</p>
                      </div>
                    </div>
                  </V3Td>
                  <V3Td>
                    <StatusPill tone="info">{employee.role || "未设置"}</StatusPill>
                  </V3Td>
                  <V3Td>
                    <StatusPill tone={employee.status === "active" ? "ok" : "warn"}>
                      {employeeStatusLabel(employee.status)}
                    </StatusPill>
                  </V3Td>
                  <V3Td className="text-right">
                    <V3Button asChild size="sm" variant="outline">
                      <Link to="/employees/$employeeId" params={{ employeeId: employee.id }}>
                        详情
                      </Link>
                    </V3Button>
                  </V3Td>
                </V3Tr>
              ))}
            </tbody>
          </V3Table>
        )}
      </div>
    </WorkSurface>
  );
}

function HumanMembersSection({
  addPanel,
  canAddMember,
  isLoading,
  members,
  onRemove,
  removing,
}: {
  addPanel: ReactNode;
  canAddMember: boolean;
  isLoading: boolean;
  members: TeamMember[];
  onRemove: (membershipId: string) => void;
  removing: boolean;
}) {
  return (
    <MasterDetailLayout
      narrowDetail="stack"
      rail="md"
      master={
        <WorkSurface className="min-w-0">
          <div className="border-b border-v3-line px-5 py-4">
            <h2 className="text-base font-bold text-v3-ink">人类管理成员</h2>
            <p className="mt-1 text-[13px] text-v3-ink-2">团队的管理、审批与观察人员。</p>
          </div>
          <div>
            {isLoading ? (
              <V3LoadingState label="加载人类成员" />
            ) : members.length === 0 ? (
              <V3EmptyState title="暂无人类成员" />
            ) : (
              <V3Table>
                <thead>
                  <tr>
                    <V3Th>成员</V3Th>
                    <V3Th>角色</V3Th>
                    <V3Th className="text-right">操作</V3Th>
                  </tr>
                </thead>
                <tbody>
                  {members.map((member) => (
                    <V3Tr key={member.membership_id}>
                      <V3Td>
                        <UserIdentity
                          showSecondary
                          user={{
                            id: member.user_id,
                            username: member.username,
                            display_name: member.display_name,
                            email: member.email,
                            avatar: member.avatar,
                            status: member.account_status || "active",
                          }}
                        />
                      </V3Td>
                      <V3Td>
                        <TeamRoleBadge role={member.role as DirectTeamRole} />
                      </V3Td>
                      <V3Td className="text-right">
                        {member.role === "owner" ? (
                          <span className="text-xs text-v3-ink-3">—</span>
                        ) : (
                          <V3Button
                            aria-label={`移除 ${member.display_name || member.username}`}
                            disabled={removing}
                            onClick={() => onRemove(member.membership_id)}
                            size="icon"
                            type="button"
                            variant="ghost"
                          >
                            <Trash2 className="size-4" />
                            <span className="sr-only">移除</span>
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
      }
      detail={
        canAddMember ? (
          <aside className="flex min-w-0 flex-col gap-4">{addPanel}</aside>
        ) : undefined
      }
    />
  );
}

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
    <WorkSurface className="p-5">
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
    </WorkSurface>
  );
}

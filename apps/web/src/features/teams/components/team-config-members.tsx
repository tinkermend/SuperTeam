import { useEffect, useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { ShieldPlus, Trash2, UserPlus } from "lucide-react";
import {
  Button,
  DataTable,
  ErrorState,
  LoadingState,
  StatusPill,
  Td,
  Th,
  Tr,
  WorkSurface
} from "@/components/superteam";
import {
  TeamRoleBadge,
  TeamRoleSelect,
  type DirectTeamRole,
  type PrivilegedTeamRole
} from "@/components/superteam/team-role";
import { UserIdentity } from "@/components/superteam/user-identity";
import { UserSearchSelect } from "@/components/superteam/user-search-select";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle
} from "@/components/ui/alert-dialog";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type { UserSummary } from "@/lib/api";
import { listAuthzMembers } from "@/lib/api";
import type { ApiClientOptions } from "@/lib/api/client";
import { ApiRequestError } from "@/lib/api/client";
import { requestTeamPrivilegedRole } from "@/lib/api/permission-approvals";
import type { AllowedTeamAction, TeamMember } from "@/lib/api/teams";
import {
  addTeamMember,
  changeTeamMemberRole,
  listTeamMembers,
  removeTeamMember
} from "@/lib/api/teams";

function errorText(error: unknown, fallback: string) {
  if (error instanceof ApiRequestError && error.detail) {
    return error.detail;
  }
  if (error instanceof Error && error.message) {
    return error.message;
  }
  return fallback;
}

type TeamConfigMembersProps = {
  allowedActions: AllowedTeamAction[];
  apiOptions: ApiClientOptions;
  teamId: string;
};

export function TeamConfigMembers({ allowedActions, apiOptions, teamId }: TeamConfigMembersProps) {
  const canAdd = allowedActions.includes("team.member.add");
  const canRemove = allowedActions.includes("team.member.remove");
  const canRequestPrivileged = allowedActions.includes("team.member.request_privileged_role");
  const [addResetToken, setAddResetToken] = useState(0);
  const [privilegedTarget, setPrivilegedTarget] = useState<TeamMember | undefined>();

  const membersQuery = useQuery({
    queryKey: ["team-members", teamId],
    queryFn: () => listTeamMembers(apiOptions, teamId)
  });
  const tenantMembersQuery = useQuery({
    enabled: canAdd,
    queryFn: () => listAuthzMembers({ ...apiOptions, limit: 200, offset: 0 }),
    queryKey: ["authz-members", "team-add-candidates", apiOptions.baseUrl]
  });

  const members = membersQuery.data ?? [];
  const existingUserIds = members.map((member) => member.user_id);
  const tenantMemberUserIds = (tenantMembersQuery.data?.items ?? [])
    .filter((member) => member.console_access && member.account_status === "active")
    .map((member) => member.user_id);
  const ownerCount = members.filter((member) => member.role === "owner").length;

  const addMutation = useMutation({
    mutationFn: (input: { role: DirectTeamRole; user_id: string }) =>
      addTeamMember(apiOptions, teamId, input),
    onSuccess: () => {
      void membersQuery.refetch();
      setAddResetToken((token) => token + 1);
    }
  });
  const removeMutation = useMutation({
    mutationFn: (membershipId: string) => removeTeamMember(apiOptions, teamId, membershipId),
    onSuccess: () => void membersQuery.refetch()
  });
  // 角色变更后 membership_id 会变（停旧行 + upsert 新行），必须重新拉取，
  // 不能就地改本地缓存里的行。
  const changeRoleMutation = useMutation({
    mutationFn: (input: { membershipId: string; role: DirectTeamRole }) =>
      changeTeamMemberRole(apiOptions, teamId, input.membershipId, input.role),
    onSuccess: () => void membersQuery.refetch()
  });

  return (
    <div className="flex flex-col gap-4">
      <WorkSurface>
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-line px-5 py-4">
          <div className="min-w-0">
            <h2 className="text-base font-bold text-ink">人类成员</h2>
            <p className="mt-1 text-[13px] text-ink-2">
              普通角色（普通成员 / 只读观察者）改动即时生效；负责人、管理员、审批人属于特权角色，需经权限中心审批。
            </p>
          </div>
          <StatusPill tone="mute">{members.length} 人</StatusPill>
        </div>
        <div className="p-4">
          {membersQuery.isLoading ? <LoadingState label="加载人类成员" /> : null}
          {membersQuery.isError ? <ErrorState title="人类成员加载失败" /> : null}
          {changeRoleMutation.isError ? (
            <p className="pb-2 text-[13px] text-danger">
              {errorText(changeRoleMutation.error, "角色变更失败，请重试")}
            </p>
          ) : null}
          {removeMutation.isError ? (
            <p className="pb-2 text-[13px] text-danger">
              {errorText(removeMutation.error, "移除成员失败，请重试")}
            </p>
          ) : null}
          {!membersQuery.isLoading && !membersQuery.isError && members.length === 0 ? (
            <p className="py-2 text-[13px] text-ink-2">暂无人类成员</p>
          ) : null}
          {members.length > 0 ? (
            <DataTable aria-label="团队人类成员">
              <thead>
                <tr>
                  <Th>成员</Th>
                  <Th>当前角色</Th>
                  <Th>普通角色调整</Th>
                  <Th className="text-right">操作</Th>
                </tr>
              </thead>
              <tbody>
                {members.map((member) => {
                  const isPrivileged = member.role !== "member" && member.role !== "viewer";
                  // 最后一名负责人既不能移除也不能降级，否则团队会失去负责人集合。
                  const isFinalOwner = member.role === "owner" && ownerCount <= 1;
                  return (
                    <Tr key={member.membership_id}>
                      <Td>
                        <UserIdentity
                          showSecondary
                          size="sm"
                          user={{
                            id: member.user_id,
                            username: member.username,
                            display_name: member.display_name,
                            email: member.email,
                            avatar: member.avatar,
                            status: member.account_status || "active"
                          }}
                        />
                      </Td>
                      <Td>
                        <TeamRoleBadge role={member.role} />
                      </Td>
                      <Td>
                        {isPrivileged ? (
                          <span className="text-xs text-ink-3">
                            {isFinalOwner ? "最后一名负责人，不可调整" : "特权角色，改动需走审批"}
                          </span>
                        ) : (
                          <TeamRoleSelect
                            ariaLabel={`调整 ${member.display_name || member.username} 的角色`}
                            disabled={changeRoleMutation.isPending}
                            mode="direct"
                            onChange={(role) =>
                              changeRoleMutation.mutate({
                                membershipId: member.membership_id,
                                role
                              })
                            }
                            value={member.role as DirectTeamRole}
                          />
                        )}
                      </Td>
                      <Td className="text-right">
                        <div className="flex items-center justify-end gap-1">
                          {canRequestPrivileged ? (
                            <Button
                              aria-label={`为 ${member.display_name || member.username} 申请特权角色`}
                              onClick={() => setPrivilegedTarget(member)}
                              size="sm"
                              type="button"
                              variant="ghost"
                            >
                              <ShieldPlus data-icon="inline-start" className="size-3.5" />
                              申请特权角色
                            </Button>
                          ) : null}
                          {canRemove && !isFinalOwner ? (
                            <Button
                              aria-label={`移除 ${member.display_name || member.username}`}
                              disabled={removeMutation.isPending}
                              onClick={() => removeMutation.mutate(member.membership_id)}
                              size="icon"
                              type="button"
                              variant="ghost"
                            >
                              <Trash2 className="size-4" />
                            </Button>
                          ) : null}
                        </div>
                      </Td>
                    </Tr>
                  );
                })}
              </tbody>
            </DataTable>
          ) : null}
        </div>
      </WorkSurface>

      {canAdd ? (
        <WorkSurface>
          <div className="border-b border-line px-5 py-4">
            <h2 className="text-base font-bold text-ink">添加成员</h2>
            <p className="mt-1 text-[13px] text-ink-2">
              仅列出已有控制台访问（租户成员）的用户；无租户成员请先到用户管理授予。
            </p>
          </div>
          <div className="p-5">
            {addMutation.isError ? (
              <p className="pb-2 text-[13px] text-danger">
                {errorText(addMutation.error, "添加成员失败，请重试")}
              </p>
            ) : null}
            <DirectAddForm
              allowedUserIds={tenantMemberUserIds}
              apiBaseUrl={apiOptions.baseUrl}
              existingUserIds={existingUserIds}
              fetcher={apiOptions.fetcher}
              isPending={addMutation.isPending || tenantMembersQuery.isLoading}
              onSubmit={(input) => addMutation.mutate(input)}
              resetToken={addResetToken}
            />
          </div>
        </WorkSurface>
      ) : null}

      <PrivilegedRoleRequestDialog
        apiOptions={apiOptions}
        member={privilegedTarget}
        onOpenChange={(open) => {
          if (!open) {
            setPrivilegedTarget(undefined);
          }
        }}
        teamId={teamId}
      />
    </div>
  );
}

// 特权角色不能直接授予：这里只提交申请，由权限中心路由给审批人，批准后才落库。
function PrivilegedRoleRequestDialog({
  apiOptions,
  member,
  onOpenChange,
  teamId
}: {
  apiOptions: ApiClientOptions;
  member?: TeamMember;
  onOpenChange: (open: boolean) => void;
  teamId: string;
}) {
  const [role, setRole] = useState<PrivilegedTeamRole>("approver");
  const [reason, setReason] = useState("");

  const requestMutation = useMutation({
    mutationFn: () =>
      requestTeamPrivilegedRole(apiOptions, teamId, {
        target_user_id: member?.user_id ?? "",
        requested_role: role,
        reason: reason.trim()
      }),
    onSuccess: () => onOpenChange(false)
  });

  useEffect(() => {
    if (member) {
      setRole("approver");
      setReason("");
      requestMutation.reset();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [member?.membership_id]);

  return (
    <AlertDialog open={Boolean(member)} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>申请特权角色</AlertDialogTitle>
          <AlertDialogDescription>
            为「{member?.display_name || member?.username}」申请团队特权角色。提交后进入权限中心，由审批人决定；批准前当前角色不变。
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <Label>申请角色</Label>
            <TeamRoleSelect
              ariaLabel="申请角色"
              disabled={requestMutation.isPending}
              mode="privileged"
              onChange={setRole}
              value={role}
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="privileged-role-reason">申请理由</Label>
            <Textarea
              disabled={requestMutation.isPending}
              id="privileged-role-reason"
              onChange={(event) => setReason(event.target.value)}
              placeholder="说明为什么需要该权限，便于审批人判断"
              rows={3}
              value={reason}
            />
          </div>
          {requestMutation.isError ? (
            <p className="text-[13px] text-danger">
              {errorText(requestMutation.error, "提交申请失败，请重试")}
            </p>
          ) : null}
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={requestMutation.isPending}>取消</AlertDialogCancel>
          <AlertDialogAction
            disabled={requestMutation.isPending || !reason.trim()}
            onClick={(event) => {
              event.preventDefault();
              requestMutation.mutate();
            }}
          >
            提交申请
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function DirectAddForm({
  allowedUserIds,
  apiBaseUrl,
  existingUserIds,
  fetcher,
  isPending,
  onSubmit,
  resetToken
}: {
  allowedUserIds: string[];
  apiBaseUrl: string;
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
    <form
      className="flex flex-col gap-4"
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
          allowedUserIds={allowedUserIds}
          apiBaseUrl={apiBaseUrl}
          disabled={isPending}
          excludedUserIds={existingUserIds}
          fetcher={fetcher}
          inputLabel="搜索直接添加用户"
          onSelect={setSelectedUser}
          placeholder="搜索已有租户成员"
          value={selectedUser}
        />
      </div>
      <div className="flex flex-col gap-2">
        <Label>直接生效角色</Label>
        <TeamRoleSelect
          ariaLabel="直接生效角色"
          disabled={isPending}
          mode="direct"
          onChange={setRole}
          value={role}
        />
      </div>
      <Button className="self-start" disabled={isPending || !selectedUser} type="submit">
        <UserPlus data-icon="inline-start" />
        添加成员
      </Button>
    </form>
  );
}

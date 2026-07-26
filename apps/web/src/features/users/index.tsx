import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  Ban,
  CheckCircle2,
  KeyRound,
  LockKeyhole,
  RotateCcw,
  ShieldCheck,
  SlidersHorizontal,
  UserPlus,
  UsersRound
} from "lucide-react";
import {
  MasterDetailLayout,
  SoftCard,
  StatusPill,
  Button,
  EmptyState,
  ErrorState,
  LoadingState,
  MetricCard,
  Segmented,
  DataTable,
  Td,
  Th,
  ToolbarSearch,
  Tr,
  WorkSurface,
  type Tone
} from "@/components/superteam";
import {
  UserIdentity,
  UserIdentityAvatar,
  getUserIdentityLabel,
  type UserIdentityData
} from "@/components/superteam/user-identity";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  listAuthzMembers,
  createUser,
  updateUserContact,
  deleteUserTenantMembership,
  feishuOAuthStartUrl,
  listFeishuIdentities,
  listTeams,
  listUserProjectTeamScopes,
  listUsers,
  replaceUserProjectTeamScopes,
  resetUserPassword,
  syncFeishuContacts,
  updateUserStatus,
  upsertUserTenantMembership,
  type ApiClientOptions,
  type AuthzMemberRecord,
  type CreateUserRequest,
  type FeishuIdentity,
  type TenantRole,
  type UserSummary
} from "@/lib/api";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { tenantRoleLabel } from "@/lib/status-labels";
import {
  CreateUserDrawer,
  type CreateUserDraft
} from "./components/create-user-drawer";
import { SelectableTeamList } from "./components/selectable-team-list";

const apiBaseUrl = resolveControlPlaneUrl();
/** 与 control-plane `platform.DefaultTenantID` 对齐；摘要在无成员数据时回退展示。 */
const TENANT_SUMMARY_FALLBACK_ID = "00000000-0000-0000-0000-000000000001";

type UserStatusFilter = "all" | "active" | "disabled";
type ConsoleAccessFilter = "all" | "granted" | "ghost";
type UserTableDensity = "comfortable" | "compact";

type UserManagementFilters = {
  consoleAccess: ConsoleAccessFilter;
  q: string;
  status: UserStatusFilter;
};

type UsersViewProps = {
  fetcher?: typeof fetch;
  /** Deep-link：从其他页跳入时预选该用户（如项目负责人 popover）。 */
  initialUserId?: string;
};

const defaultUserFilters: UserManagementFilters = {
  consoleAccess: "all",
  q: "",
  status: "all"
};

export function Users({ initialUserId }: { initialUserId?: string } = {}) {
  return <UsersView initialUserId={initialUserId} />;
}

export function UsersView({ fetcher, initialUserId }: UsersViewProps = {}) {
  const queryClient = useQueryClient();
  const [filters, setFilters] = useState<UserManagementFilters>(defaultUserFilters);
  const [selectedUserId, setSelectedUserId] = useState<string | undefined>(
    initialUserId,
  );
  const [pendingCreatedUserId, setPendingCreatedUserId] = useState<string>();
  const [resetPasswordOpen, setResetPasswordOpen] = useState(false);
  const [contactDialogOpen, setContactDialogOpen] = useState(false);
  const [contactEmail, setContactEmail] = useState("");
  const [contactMobile, setContactMobile] = useState("");
  const [resetPasswordValue, setResetPasswordValue] = useState("");
  const [createUserOpen, setCreateUserOpen] = useState(false);
  const [tableDensity, setTableDensity] = useState<UserTableDensity>("comfortable");
  const apiOptions = useMemo(
    () => ({
      baseUrl: apiBaseUrl,
      fetcher
}),
    [fetcher],
  );

  const usersQuery = useQuery({
    queryFn: () =>
      listUsers({
        ...apiOptions,
        limit: 50,
        offset: 0,
        q: filters.q,
        status: filters.status === "all" ? undefined : filters.status
}),
    queryKey: ["users", "management", filters]
});
  const authzMembersQuery = useQuery({
    queryFn: () =>
      listAuthzMembers({
        ...apiOptions,
        limit: 100,
        offset: 0
}),
    queryKey: ["users", "authz-members"]
});

  const feishuIdentitiesQuery = useQuery({
    queryFn: () => listFeishuIdentities(apiOptions),
    queryKey: ["users", "feishu-identities"]
});
  const contactSyncMutation = useMutation({
    mutationFn: () => syncFeishuContacts(apiOptions),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["users", "feishu-identities"] });
    }
});

  const listedUsers = usersQuery.data?.items ?? [];
  const authzMembersByUserId = useMemo(() => {
    return new Map((authzMembersQuery.data?.items ?? []).map((member) => [member.user_id, member]));
  }, [authzMembersQuery.data?.items]);
  const feishuIdentitiesByUserId = useMemo(() => {
    return new Map((feishuIdentitiesQuery.data ?? []).map((identity) => [identity.auth_user_id, identity]));
  }, [feishuIdentitiesQuery.data]);
  const contactSyncSummary = contactSyncMutation.data
    ? contactSyncMutation.data.reduce(
        (acc, report) => ({
          bound: acc.bound + report.bound,
          alreadyBound: acc.alreadyBound + report.already_bound,
          unmatched: acc.unmatched + report.unmatched
}),
        { bound: 0, alreadyBound: 0, unmatched: 0 },
      )
    : undefined;
  const users = useMemo(() => {
    if (filters.consoleAccess === "all") {
      return listedUsers;
    }
    return listedUsers.filter((user) => {
      const hasAccess = Boolean(authzMembersByUserId.get(user.id)?.console_access);
      return filters.consoleAccess === "granted" ? hasAccess : !hasAccess;
    });
  }, [authzMembersByUserId, filters.consoleAccess, listedUsers]);
  const selectedUser = users.find((user) => user.id === selectedUserId) ?? users[0];
  const selectedMember = selectedUser ? authzMembersByUserId.get(selectedUser.id) : undefined;
  const selectedIdentity = selectedUser ? mergeUserIdentity(selectedUser, selectedMember) : undefined;
  const stats = getUserStats(listedUsers, authzMembersQuery.data?.items ?? []);
  const tenantSummaryId =
    authzMembersQuery.data?.items
      .flatMap((member) => member.memberships)
      .find((membership) => !membership.team_id)?.tenant_id ?? TENANT_SUMMARY_FALLBACK_ID;

  useEffect(() => {
    if (initialUserId) {
      setSelectedUserId(initialUserId);
    }
  }, [initialUserId]);

  useEffect(() => {
    if (pendingCreatedUserId) {
      if (users.some((user) => user.id === pendingCreatedUserId)) {
        setSelectedUserId(pendingCreatedUserId);
        setPendingCreatedUserId(undefined);
      }
      return;
    }

    if (users.length === 0) {
      setSelectedUserId(undefined);
      return;
    }

    if (!selectedUserId || !users.some((user) => user.id === selectedUserId)) {
      setSelectedUserId(users[0].id);
    }
  }, [pendingCreatedUserId, selectedUserId, users]);

  const invalidateUserWorkspace = () => queryClient.invalidateQueries({ queryKey: ["users"] });
  const statusMutation = useMutation({
    mutationFn: (input: { status: UserSummary["status"]; userId: string }) =>
      updateUserStatus(apiOptions, input.userId, input.status),
    onSuccess: () => {
      void invalidateUserWorkspace();
    }
});
  const resetPasswordMutation = useMutation({
    mutationFn: (input: { password: string; userId: string }) =>
      resetUserPassword(apiOptions, input.userId, input.password),
    onSuccess: () => {
      setResetPasswordOpen(false);
      setResetPasswordValue("");
      void invalidateUserWorkspace();
    }
});
  const createUserMutation = useMutation({
    mutationFn: (input: CreateUserDraft) => {
      const payload: CreateUserRequest = {
        username: input.username,
        display_name: input.display_name,
        password: input.password,
        avatar: input.avatar,
        tenant_role: input.tenant_role,
        selectable_team_ids: input.selectable_team_ids
};
      if (input.email.trim()) {
        payload.email = input.email.trim();
      }
      if (input.mobile.trim()) {
        payload.mobile = input.mobile.trim();
      }
      return createUser(apiOptions, payload);
    },
    onSuccess: async (response) => {
      setCreateUserOpen(false);
      setFilters(defaultUserFilters);
      setPendingCreatedUserId(response.user.id);
      setSelectedUserId(response.user.id);
      await invalidateUserWorkspace();
    }
});
  const updateContactMutation = useMutation({
    mutationFn: (input: { userId: string; email: string; mobile: string }) =>
      updateUserContact(apiOptions, input.userId, { email: input.email, mobile: input.mobile }),
    onSuccess: async () => {
      setContactDialogOpen(false);
      await invalidateUserWorkspace();
    }
});
  const upsertTenantMembershipMutation = useMutation({
    mutationFn: (input: { role: TenantRole; userId: string }) =>
      upsertUserTenantMembership(apiOptions, input.userId, input.role),
    onSuccess: () => {
      void invalidateUserWorkspace();
    }
  });
  const deleteTenantMembershipMutation = useMutation({
    mutationFn: (userId: string) => deleteUserTenantMembership(apiOptions, userId),
    onSuccess: () => {
      void invalidateUserWorkspace();
    }
  });
  const replaceScopesMutation = useMutation({
    mutationFn: (input: { teamIds: string[]; userId: string }) =>
      replaceUserProjectTeamScopes(apiOptions, input.userId, { team_ids: input.teamIds }),
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({
        queryKey: ["users", "project-team-scopes", variables.userId]
      });
    }
  });

  const handleCreateUserOpenChange = (open: boolean) => {
    createUserMutation.reset();
    setCreateUserOpen(open);
  };

  return (
    <>
      <ShellPageHeader
        icon={<UsersRound />}
        iconTone="mute"
        title={
          <span className="inline-flex flex-wrap items-center gap-2">
            用户管理
            <StatusPill className="align-middle" tone="info" showDot={false}>用户治理台</StatusPill>
          </span>
        }
        subtitle="管理平台人类用户、租户角色（控制台访问）、账号状态；团队角色请到团队管理页调整。"
      />
      <Main width="wide" className="min-w-0 overflow-x-hidden">
        <div className="mb-4 flex flex-wrap items-center justify-start gap-2 sm:justify-end">
          {contactSyncSummary ? (
            <span className="text-sm text-ink-2 tabular-nums" data-testid="feishu-contact-sync-summary">
              飞书同步:新绑 {contactSyncSummary.bound} · 已绑 {contactSyncSummary.alreadyBound} · 未匹配 {contactSyncSummary.unmatched}
            </span>
          ) : null}
          {contactSyncMutation.isError ? (
            <span className="text-sm text-danger-text" data-testid="feishu-contact-sync-error">
              飞书同步失败,请检查应用配置
            </span>
          ) : null}
          <Button
            disabled={contactSyncMutation.isPending}
            onClick={() => contactSyncMutation.mutate()}
            type="button"
            variant="outline"
          >
            {contactSyncMutation.isPending ? "同步中…" : "同步飞书绑定"}
          </Button>
          <Button
            onClick={() => {
              // OAuth 授权是外部整页跳转,允许原生 location 导航。
              window.location.href = feishuOAuthStartUrl(apiOptions, "/users");
            }}
            type="button"
            variant="outline"
          >
            绑定我的飞书
          </Button>
          <Button onClick={() => handleCreateUserOpenChange(true)} type="button">
            <UserPlus data-icon="inline-start" />
            新建用户
          </Button>
        </div>

        <SoftCard className="mb-4 p-4" data-testid="tenant-iam-summary">
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0">
              <p className="text-sm font-bold text-ink">当前租户（单租户）</p>
              <p className="mt-1 truncate text-xs text-ink-3 font-mono">{tenantSummaryId}</p>
            </div>
            <p className="text-sm text-ink-2">
              租户成员 {stats.tenantRoles} · 控制台访问 {stats.consoleAccess} · 无控制台访问（幽灵账号）{" "}
              {stats.ghostAccounts}
            </p>
          </div>
        </SoftCard>

        <div className="mb-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
          <UserMetric icon={<CheckCircle2 />} label="活跃用户" tone="ok" value={stats.active} />
          <UserMetric icon={<Ban />} label="禁用用户" tone="danger" value={stats.disabled} />
          <UserMetric
            icon={<ShieldCheck />}
            label="控制台访问"
            onSelect={() => setFilters((current) => ({ ...current, consoleAccess: "granted" }))}
            tone="brand"
            value={stats.consoleAccess}
          />
          <UserMetric icon={<KeyRound />} label="租户成员" tone="artifact" value={stats.tenantRoles} />
          <UserMetric
            icon={<UsersRound />}
            label="无控制台访问"
            onSelect={() =>
              setFilters((current) => ({
                ...current,
                consoleAccess: "ghost",
                status: "active"
              }))
            }
            tone="warn"
            value={stats.ghostAccounts}
          />
        </div>

        <MasterDetailLayout
          data-layout="table-governance"
          data-testid="users-management-layout"
          narrowDetail="stack"
          rail="md"
          master={
            <UserGovernanceTable
              authzMembersByUserId={authzMembersByUserId}
              feishuIdentitiesByUserId={feishuIdentitiesByUserId}
              density={tableDensity}
              filters={filters}
              isError={usersQuery.isError}
              isLoading={usersQuery.isLoading}
              isStatusPending={statusMutation.isPending}
              onDensityChange={setTableDensity}
              onFiltersChange={setFilters}
              onResetPassword={(userId) => {
                setSelectedUserId(userId);
                setResetPasswordOpen(true);
              }}
              onSelectUser={setSelectedUserId}
              onToggleStatus={(user) =>
                statusMutation.mutate({
                  status: user.status === "active" ? "disabled" : "active",
                  userId: user.id
})
              }
              selectedUserId={selectedUser?.id}
              users={users}
            />
          }
          detail={
            <UserGovernancePreview
              apiOptions={apiOptions}
              isMembershipPending={
                upsertTenantMembershipMutation.isPending || deleteTenantMembershipMutation.isPending
              }
              isScopesPending={replaceScopesMutation.isPending}
              isStatusPending={statusMutation.isPending}
              member={selectedMember}
              membershipError={
                upsertTenantMembershipMutation.error instanceof Error
                  ? upsertTenantMembershipMutation.error.message
                  : deleteTenantMembershipMutation.error instanceof Error
                    ? deleteTenantMembershipMutation.error.message
                    : undefined
              }
              onDeleteTenantMembership={() => {
                if (!selectedUser) {
                  return;
                }
                deleteTenantMembershipMutation.mutate(selectedUser.id);
              }}
              onReplaceScopes={(teamIds) => {
                if (!selectedUser) {
                  return;
                }
                replaceScopesMutation.mutate({ teamIds, userId: selectedUser.id });
              }}
              onEditContact={() => {
                setContactEmail(selectedUser?.email ?? "");
                setContactMobile(selectedUser?.mobile ?? "");
                updateContactMutation.reset();
                setContactDialogOpen(true);
              }}
              onResetPassword={() => setResetPasswordOpen(true)}
              onToggleStatus={() => {
                if (!selectedUser) {
                  return;
                }
                statusMutation.mutate({
                  status: selectedUser.status === "active" ? "disabled" : "active",
                  userId: selectedUser.id
                });
              }}
              onUpsertTenantMembership={(role) => {
                if (!selectedUser) {
                  return;
                }
                upsertTenantMembershipMutation.mutate({ role, userId: selectedUser.id });
              }}
              scopesError={
                replaceScopesMutation.error instanceof Error
                  ? replaceScopesMutation.error.message
                  : undefined
              }
              user={selectedIdentity}
              userId={selectedUser?.id}
            />
          }
        />

        {selectedUser ? (
          <>
          <ContactDialog
            email={contactEmail}
            error={updateContactMutation.error}
            isOpen={contactDialogOpen}
            isPending={updateContactMutation.isPending}
            mobile={contactMobile}
            onOpenChange={setContactDialogOpen}
            onSubmit={() => {
              if (!selectedUser) {
                return;
              }
              updateContactMutation.mutate({ email: contactEmail, mobile: contactMobile, userId: selectedUser.id });
            }}
            setEmail={setContactEmail}
            setMobile={setContactMobile}
            username={selectedUser?.username ?? ""}
          />
          <ResetPasswordDialog
            error={resetPasswordMutation.error}
            isOpen={resetPasswordOpen}
            isPending={resetPasswordMutation.isPending}
            onOpenChange={setResetPasswordOpen}
            onSubmit={() =>
              resetPasswordMutation.mutate({
                password: resetPasswordValue,
                userId: selectedUser.id
})
            }
            password={resetPasswordValue}
            setPassword={setResetPasswordValue}
            username={selectedUser.username}
          />
          </>
        ) : null}
        <CreateUserDrawer
          apiBaseUrl={apiBaseUrl}
          fetcher={fetcher}
          isSubmitting={createUserMutation.isPending}
          onOpenChange={handleCreateUserOpenChange}
          onSubmit={(draft) => createUserMutation.mutate(draft)}
          open={createUserOpen}
          submitError={createUserMutation.error instanceof Error ? createUserMutation.error.message : undefined}
        />
      </Main>
    </>
  );
}

function UserGovernanceTable({
  authzMembersByUserId,
  feishuIdentitiesByUserId,
  density,
  filters,
  isError,
  isLoading,
  isStatusPending,
  onDensityChange,
  onFiltersChange,
  onResetPassword,
  onSelectUser,
  onToggleStatus,
  selectedUserId,
  users
}: {
  authzMembersByUserId: Map<string, AuthzMemberRecord>;
  feishuIdentitiesByUserId: Map<string, FeishuIdentity>;
  density: UserTableDensity;
  filters: UserManagementFilters;
  isError: boolean;
  isLoading: boolean;
  isStatusPending: boolean;
  onDensityChange: (density: UserTableDensity) => void;
  onFiltersChange: (filters: UserManagementFilters) => void;
  onResetPassword: (userId: string) => void;
  onSelectUser: (userId: string) => void;
  onToggleStatus: (user: UserSummary) => void;
  selectedUserId?: string;
  users: UserSummary[];
}) {
  return (
    <WorkSurface className="min-w-0" data-testid="users-governance-table">
      <div className="flex flex-col gap-3 border-b border-line p-4">
        <div className="flex min-w-0 flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div className="min-w-0">
            <h2 className="text-base font-bold text-ink">用户治理表</h2>
            <p className="text-sm text-ink-2">逐行治理人类用户的账号状态、控制台访问与成员身份。</p>
          </div>
          <Segmented
            aria-label="表格密度"
            onChange={onDensityChange}
            options={[
              { label: "舒适", value: "comfortable" },
              { label: "紧凑", value: "compact" },
            ]}
            value={density}
          />
        </div>
        <div className="flex min-w-0 flex-col gap-2 xl:flex-row xl:items-center">
          <ToolbarSearch
            aria-label="搜索用户"
            onChange={(event) =>
              onFiltersChange({
                ...filters,
                q: event.target.value
})
            }
            placeholder="搜索用户名、姓名或邮箱"
            type="search"
            value={filters.q}
          />
          <div className="flex flex-wrap gap-2">
            {[
              ["all", "全部"],
              ["active", "活跃"],
              ["disabled", "禁用"],
            ].map(([status, label]) => (
              <Button
                aria-pressed={filters.status === status}
                key={status}
                onClick={() =>
                  onFiltersChange({
                    ...filters,
                    status: status as UserStatusFilter
                  })
                }
                size="sm"
                type="button"
                variant={filters.status === status ? "primary" : "outline"}
              >
                {label}
              </Button>
            ))}
            {[
              ["all", "控制台：全部"],
              ["granted", "有访问"],
              ["ghost", "无访问"],
            ].map(([consoleAccess, label]) => (
              <Button
                aria-pressed={filters.consoleAccess === consoleAccess}
                key={consoleAccess}
                onClick={() =>
                  onFiltersChange({
                    ...filters,
                    consoleAccess: consoleAccess as ConsoleAccessFilter
                  })
                }
                size="sm"
                type="button"
                variant={filters.consoleAccess === consoleAccess ? "primary" : "outline"}
              >
                {label}
              </Button>
            ))}
          </div>
        </div>
      </div>

      {isLoading ? <LoadingState className="min-h-[360px]" label="加载用户中" /> : null}
      {isError ? (
        <ErrorState className="m-4" title="用户列表加载失败" description="请刷新页面或检查 Control Plane 连接。" />
      ) : null}
      {!isLoading && !isError && users.length === 0 ? (
        <EmptyState className="min-h-[360px]" title="暂无匹配用户。" />
      ) : null}
      {!isLoading && !isError && users.length > 0 ? (
        <>
          <DataTable className={density === "compact" ? "[&_td]:py-2 [&_th]:py-2" : undefined}>
            <thead>
              <Tr>
                <Th>用户</Th>
                <Th>状态</Th>
                <Th>控制台访问</Th>
                <Th>飞书</Th>
                <Th>成员身份</Th>
                <Th>操作</Th>
              </Tr>
            </thead>
            <tbody>
              {users.map((user) => {
                const member = authzMembersByUserId.get(user.id);
                const selected = user.id === selectedUserId;
                const identity = mergeUserIdentity(user, member);
                return (
                  <Tr
                    className={selected ? "[&>td]:bg-brand-soft/55" : undefined}
                    key={user.id}
                    onClick={() => onSelectUser(user.id)}
                    tone={user.status === "disabled" ? "danger" : undefined}
                  >
                    <Td className="min-w-[220px]">
                      <UserIdentity className="min-w-0" showSecondary user={identity} />
                    </Td>
                    <Td>
                      <StatusPill tone={userStatusTone(user.status)}>{formatUserStatus(user.status)}</StatusPill>
                    </Td>
                    <Td>
                      <StatusPill tone={member?.console_access ? "ok" : "mute"}>
                        {member?.console_access ? "允许" : "未确认"}
                      </StatusPill>
                    </Td>
                    <Td>
                      {(() => {
                        const identity = feishuIdentitiesByUserId.get(user.id);
                        return (
                          <StatusPill
                            title={identity ? identity.open_id : undefined}
                            tone={identity ? "ok" : "mute"}
                          >
                            {identity ? (identity.bound_via === "oauth" ? "已绑定 · 授权" : "已绑定 · 同步") : "未绑定"}
                          </StatusPill>
                        );
                      })()}
                    </Td>
                    <Td className="min-w-[150px]">{formatMembershipSummary(member)}</Td>
                    <Td>
                      <div className="flex min-w-max gap-2">
                        <Button
                          onClick={(event) => {
                            event.stopPropagation();
                            onSelectUser(user.id);
                          }}
                          size="sm"
                          type="button"
                          variant="outline"
                        >
                          详情
                        </Button>
                        <Button
                          disabled={isStatusPending}
                          onClick={(event) => {
                            event.stopPropagation();
                            onToggleStatus(user);
                          }}
                          size="sm"
                          type="button"
                          variant={user.status === "active" ? "danger" : "outline"}
                        >
                          {user.status === "active" ? "禁用账号" : "启用账号"}
                        </Button>
                        <Button
                          onClick={(event) => {
                            event.stopPropagation();
                            onResetPassword(user.id);
                          }}
                          size="sm"
                          type="button"
                          variant="outline"
                        >
                          重置密码
                        </Button>
                      </div>
                    </Td>
                  </Tr>
                );
              })}
            </tbody>
          </DataTable>
          <div className="flex flex-col gap-2 border-t border-line px-4 py-3 text-sm text-ink-2 sm:flex-row sm:items-center sm:justify-between">
            <span className="tabular-nums">共 {users.length} 个用户</span>
            <span className="inline-flex items-center gap-2">
              <SlidersHorizontal className="size-4" />
              状态与关键字筛选保持在当前工作台内
            </span>
          </div>
        </>
      ) : null}
    </WorkSurface>
  );
}

function UserGovernancePreview({
  apiOptions,
  isMembershipPending,
  isScopesPending,
  isStatusPending,
  member,
  membershipError,
  onDeleteTenantMembership,
  onEditContact,
  onReplaceScopes,
  onResetPassword,
  onToggleStatus,
  onUpsertTenantMembership,
  scopesError,
  user,
  userId
}: {
  apiOptions: ApiClientOptions;
  isMembershipPending: boolean;
  isScopesPending: boolean;
  isStatusPending: boolean;
  member?: AuthzMemberRecord;
  membershipError?: string;
  onDeleteTenantMembership: () => void;
  onEditContact: () => void;
  onReplaceScopes: (teamIds: string[]) => void;
  onResetPassword: () => void;
  onToggleStatus: () => void;
  onUpsertTenantMembership: (role: TenantRole) => void;
  scopesError?: string;
  user?: UserIdentityData;
  userId?: string;
}) {
  const tenantMembership = member?.memberships.find((item) => !item.team_id && item.status === "active");
  const [draftRole, setDraftRole] = useState<TenantRole>(
    (tenantMembership?.role as TenantRole | undefined) ?? "member",
  );
  const [draftTeamIds, setDraftTeamIds] = useState<string[]>([]);

  const scopesQuery = useQuery({
    enabled: Boolean(userId),
    queryFn: () => listUserProjectTeamScopes(apiOptions, userId as string),
    queryKey: ["users", "project-team-scopes", userId]
  });
  const teamsQuery = useQuery({
    enabled: Boolean(userId),
    queryFn: () => listTeams(apiOptions),
    queryKey: ["users", "teams", "scope-editor"]
  });

  useEffect(() => {
    setDraftRole((tenantMembership?.role as TenantRole | undefined) ?? "member");
  }, [tenantMembership?.role, user?.id]);

  useEffect(() => {
    const activeIds = (scopesQuery.data?.items ?? [])
      .filter((scope) => scope.status === "active" && !scope.revoked_at)
      .map((scope) => scope.team_id);
    setDraftTeamIds(activeIds);
  }, [scopesQuery.data?.items, userId]);

  if (!user) {
    return (
      <aside className="flex min-w-0 flex-col gap-4">
        <SoftCard className="min-h-[420px]">
          <EmptyState className="min-h-[420px]" title="请选择一个用户查看详情" />
        </SoftCard>
      </aside>
    );
  }

  const label = getUserIdentityLabel(user);
  const memberships = member?.memberships ?? [];
  const hasConsoleAccess = Boolean(member?.console_access);
  const activeTeams = (teamsQuery.data ?? []).filter((team) => team.status === "active");
  const savedScopeIds = (scopesQuery.data?.items ?? [])
    .filter((scope) => scope.status === "active" && !scope.revoked_at)
    .map((scope) => scope.team_id)
    .sort()
    .join(",");
  const draftScopeIds = [...draftTeamIds].sort().join(",");
  const scopesDirty = savedScopeIds !== draftScopeIds;

  return (
    <aside className="flex min-w-0 flex-col gap-4">
      <SoftCard className="p-5">
        <div className="mb-4 flex min-w-0 items-start gap-3">
          <UserIdentityAvatar className="size-14" user={user} />
          <div className="min-w-0">
            <h2 className="truncate text-xl font-bold tracking-normal text-ink">{label.primary}</h2>
            <div className="mt-2 flex flex-wrap gap-2">
              <StatusPill tone={userStatusTone(user.status)}>{formatUserStatus(user.status)}</StatusPill>
              <StatusPill tone={hasConsoleAccess ? "ok" : "warn"}>
                控制台访问：{hasConsoleAccess ? "允许" : "无"}
              </StatusPill>
            </div>
          </div>
        </div>
        <div className="grid gap-3 text-sm">
          <InfoRow label="邮箱/标识" value={label.secondary} />
          <InfoRow label="用户名" value={user.username ?? "-"} />
          <InfoRow label="用户 ID" value={shortId(user.id)} />
          <InfoRow
            label="租户角色"
            value={tenantMembership ? tenantRoleLabel(tenantMembership.role) : "未授予"}
          />
          <InfoRow label="成员身份" value={formatMembershipSummary(member)} />
        </div>
        <div className="mt-4 flex flex-wrap gap-2">
          <Button
            disabled={isStatusPending}
            onClick={onToggleStatus}
            size="sm"
            type="button"
            variant={user.status === "active" ? "danger" : "outline"}
          >
            {user.status === "active" ? <Ban data-icon="inline-start" /> : <RotateCcw data-icon="inline-start" />}
            {user.status === "active" ? "禁用账号" : "启用账号"}
          </Button>
          <Button onClick={onResetPassword} size="sm" type="button" variant="outline">
            <LockKeyhole data-icon="inline-start" />
            重置密码
          </Button>
          <Button onClick={onEditContact} size="sm" type="button" variant="outline">
            联系方式
          </Button>
          <Button asChild size="sm" variant="outline">
            <Link to="/teams">
              <UsersRound data-icon="inline-start" />
              去团队管理分配
            </Link>
          </Button>
        </div>
      </SoftCard>

      <SoftCard className="p-5">
        <div className="mb-4">
          <h3 className="text-base font-bold text-ink">租户成员 / 控制台访问</h3>
          <p className="text-sm text-ink-2">
            租户级成员（非团队角色）决定能否进入控制台。撤销后保留账号，但无法通过 `/me`。
          </p>
        </div>
        <div className="flex flex-col gap-3">
          <div className="flex flex-col gap-2">
            <Label htmlFor="tenant-role-select">租户角色</Label>
            <select
              className="h-10 rounded-md border border-line bg-card-soft px-3 text-sm"
              disabled={isMembershipPending}
              id="tenant-role-select"
              onChange={(event) => setDraftRole(event.target.value as TenantRole)}
              value={draftRole}
            >
              <option value="member">{tenantRoleLabel("member")}</option>
              <option value="viewer">{tenantRoleLabel("viewer")}</option>
              <option value="admin">{tenantRoleLabel("admin")}</option>
              <option value="owner">{tenantRoleLabel("owner")}</option>
            </select>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              disabled={isMembershipPending}
              onClick={() => onUpsertTenantMembership(draftRole)}
              size="sm"
              type="button"
            >
              {tenantMembership ? "更新租户角色" : "授予控制台访问"}
            </Button>
            <Button
              disabled={isMembershipPending || !tenantMembership}
              onClick={onDeleteTenantMembership}
              size="sm"
              type="button"
              variant="outline"
            >
              撤销租户成员
            </Button>
          </div>
          {membershipError ? <p className="text-sm text-danger-text">{membershipError}</p> : null}
        </div>
      </SoftCard>

      <SoftCard className="p-5">
        <div className="mb-4">
          <h3 className="text-base font-bold text-ink">创建项目时可选择的团队</h3>
          <p className="text-sm text-ink-2">
            当前用户创建或协作项目时可选择的团队范围。不等于团队成员身份，也不授予控制台访问。
          </p>
        </div>
        {scopesQuery.isLoading || teamsQuery.isLoading ? (
          <LoadingState label="加载可选团队" />
        ) : scopesQuery.isError || teamsQuery.isError ? (
          <ErrorState title="可选团队加载失败" description="请稍后重试或检查 Control Plane 连接。" />
        ) : activeTeams.length === 0 ? (
          <EmptyState className="py-8" title="暂无可用团队。" />
        ) : (
          <div className="flex flex-col gap-3">
            <SelectableTeamList
              disabled={isScopesPending}
              onChange={setDraftTeamIds}
              selectedTeamIds={draftTeamIds}
              teams={activeTeams}
            />
            <div className="flex flex-wrap items-center gap-2">
              <Button
                disabled={isScopesPending || !scopesDirty}
                onClick={() => onReplaceScopes(draftTeamIds)}
                size="sm"
                type="button"
              >
                保存可选团队
              </Button>
              {scopesDirty ? (
                <span className="text-xs text-ink-3">有未保存变更</span>
              ) : (
                <span className="text-xs text-ink-3">已与服务器一致</span>
              )}
            </div>
            {scopesError ? <p className="text-sm text-danger-text">{scopesError}</p> : null}
          </div>
        )}
      </SoftCard>

      <SoftCard className="p-5">
        <div className="mb-4">
          <h3 className="text-base font-bold text-ink">成员身份一览</h3>
          <p className="text-sm text-ink-2">只读展示租户与团队成员；团队角色仍在团队管理页调整。</p>
        </div>
        {memberships.length === 0 ? (
          <EmptyState className="py-8" title="暂无成员身份。" />
        ) : (
          <div className="flex flex-col gap-2">
            {memberships.map((membership) => (
              <div
                className="flex items-center justify-between gap-3 rounded-inner border border-line bg-card-soft px-3 py-2 text-sm"
                key={`${membership.tenant_id}-${membership.team_id ?? "tenant"}-${membership.role}`}
              >
                <span className="truncate text-ink">{formatMembershipScope(membership)}</span>
                <StatusPill tone={membership.status === "active" ? "ok" : "mute"}>
                  {membership.status === "active" ? "有效" : membership.status}
                </StatusPill>
              </div>
            ))}
          </div>
        )}
      </SoftCard>

    </aside>
  );
}

function ResetPasswordDialog({
  error,
  isOpen,
  isPending,
  onOpenChange,
  onSubmit,
  password,
  setPassword,
  username
}: {
  error: unknown;
  isOpen: boolean;
  isPending: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: () => void;
  password: string;
  setPassword: (password: string) => void;
  username: string;
}) {
  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>重置密码</DialogTitle>
          <DialogDescription>为 {username} 设置新的临时密码。敏感操作会由 Control Plane 记录操作日志。</DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            onSubmit();
          }}
        >
          <div className="flex flex-col gap-2">
            <Label htmlFor="reset-password">新密码</Label>
            <Input
              id="reset-password"
              minLength={4}
              onChange={(event) => setPassword(event.target.value)}
              placeholder="输入临时密码"
              required
              type="password"
              value={password}
            />
          </div>
          {error instanceof Error ? <p className="text-sm text-destructive">{error.message}</p> : null}
          <DialogFooter>
            <Button onClick={() => onOpenChange(false)} type="button" variant="outline">
              取消
            </Button>
            <Button disabled={isPending || password.length < 4} type="submit">
              确认重置
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function ContactDialog({
  email,
  error,
  isOpen,
  isPending,
  mobile,
  onOpenChange,
  onSubmit,
  setEmail,
  setMobile,
  username
}: {
  email: string;
  error: unknown;
  isOpen: boolean;
  isPending: boolean;
  mobile: string;
  onOpenChange: (open: boolean) => void;
  onSubmit: () => void;
  setEmail: (value: string) => void;
  setMobile: (value: string) => void;
  username: string;
}) {
  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>联系方式</DialogTitle>
          <DialogDescription>
            为 {username} 维护邮箱与手机号——它们是「同步飞书绑定」按通讯录反查的撞库键,需与飞书档案一致。留空提交即清除。
          </DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            onSubmit();
          }}
        >
          <div className="flex flex-col gap-2">
            <Label htmlFor="contact-email">邮箱</Label>
            <Input
              id="contact-email"
              onChange={(event) => setEmail(event.target.value)}
              placeholder="与飞书档案一致"
              type="email"
              value={email}
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="contact-mobile">手机号</Label>
            <Input
              id="contact-mobile"
              onChange={(event) => setMobile(event.target.value)}
              placeholder="含区号,如 +8613800138000"
              value={mobile}
            />
          </div>
          {error instanceof Error ? <p className="text-sm text-destructive">{error.message}</p> : null}
          <DialogFooter>
            <Button onClick={() => onOpenChange(false)} type="button" variant="outline">
              取消
            </Button>
            <Button disabled={isPending} type="submit">
              保存
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function UserMetric({
  icon,
  label,
  onSelect,
  tone,
  value
}: {
  icon: ReactNode;
  label: string;
  onSelect?: () => void;
  tone: Tone;
  value: number;
}) {
  return (
    <MetricCard
      action={
        onSelect ? (
          <Button onClick={onSelect} size="sm" type="button" variant="ghost">
            筛选
          </Button>
        ) : undefined
      }
      icon={icon}
      iconTone={tone}
      label={label}
      loud={tone === "warn" || tone === "danger"}
      value={value}
    />
  );
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex min-w-0 items-center justify-between gap-3 rounded-inner border border-line bg-card-soft px-3 py-2">
      <span className="shrink-0 text-ink-2">{label}</span>
      <span className="min-w-0 truncate text-right font-medium text-ink">{value}</span>
    </div>
  );
}

function mergeUserIdentity(user: UserSummary, member?: AuthzMemberRecord): UserIdentityData {
  return {
    avatar: user.avatar,
    avatar_asset_id: user.avatar_asset_id,
    display_name: member?.display_name ?? user.display_name ?? undefined,
    email: member?.email ?? user.email ?? undefined,
    id: user.id,
    status: member?.account_status ?? user.status,
    username: member?.username ?? user.username
};
}

function getUserStats(users: UserSummary[], members: AuthzMemberRecord[]) {
  const active = users.filter((user) => user.status === "active").length;
  const disabled = users.filter((user) => user.status === "disabled").length;
  const membersByID = new Map(members.map((member) => [member.user_id, member]));
  const consoleAccess = members.filter((member) => member.console_access).length;
  const tenantRoles = members.reduce(
    (count, member) => count + member.memberships.filter((membership) => !membership.team_id).length,
    0,
  );
  const ghostAccounts = users.filter((user) => {
    if (user.status !== "active") {
      return false;
    }
    const member = membersByID.get(user.id);
    return !member?.console_access;
  }).length;

  return {
    active,
    consoleAccess,
    disabled,
    ghostAccounts,
    tenantRoles
};
}

function formatUserStatus(status: string) {
  if (status === "active") {
    return "活跃";
  }
  if (status === "disabled") {
    return "禁用";
  }

  return status;
}

function userStatusTone(status: string): Tone {
  if (status === "active") {
    return "ok";
  }
  if (status === "disabled") {
    return "danger";
  }

  return "mute";
}

function formatMembershipScope(membership: AuthzMemberRecord["memberships"][number]) {
  if (membership.team_id) {
    return `团队 ${formatRoleLabel(membership.role)}`;
  }

  return `租户 ${formatRoleLabel(membership.role)}`;
}

function formatMembershipSummary(member?: AuthzMemberRecord) {
  const memberships = member?.memberships ?? [];

  if (memberships.length === 0) {
    return "暂无成员身份";
  }

  const teamCount = new Set(memberships.map((membership) => membership.team_id).filter(Boolean)).size;
  const roleCount = new Set(memberships.map((membership) => membership.role)).size;

  if (teamCount === 0) {
    return `租户角色 / ${roleCount} 个角色`;
  }

  return `${teamCount} 个团队 / ${roleCount} 个角色`;
}

function formatRoleLabel(role: string) {
  if (role === "owner") {
    return "Owner";
  }
  if (role === "admin") {
    return "Admin";
  }

  return role;
}

function shortId(value: string) {
  if (value.length <= 10) {
    return value;
  }

  return `${value.slice(0, 8)}...${value.slice(-4)}`;
}

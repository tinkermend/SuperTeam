import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import {
  Activity,
  ArrowUpRight,
  Ban,
  CheckCircle2,
  Clock3,
  KeyRound,
  LockKeyhole,
  RotateCcw,
  SearchIcon,
  ShieldAlert,
  ShieldCheck,
  UserPlus,
  UsersRound,
} from "lucide-react";
import {
  IconTile,
  SoftCard,
  StatusPill,
  V3Button,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3MetricCard,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  V3Tabs,
  WorkSurface,
  type V3Tone,
} from "@/components/superteam";
import {
  UserIdentity,
  UserIdentityAvatar,
  getUserIdentityLabel,
  type UserIdentityData,
} from "@/components/superteam/user-identity";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  listAuthzDecisions,
  listAuthzMembers,
  listLoginLogs,
  createUser,
  listUserProjectTeamScopes,
  listUsers,
  resetUserPassword,
  updateUserStatus,
  type AuthzDecisionRecord,
  type AuthzMemberRecord,
  type CreateUserRequest,
  type LoginLogRecord,
  type UserProjectTeamScope,
  type UserSummary,
} from "@/lib/api";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { cn } from "@/lib/utils";
import {
  CreateUserDrawer,
  type CreateUserDraft,
} from "./components/create-user-drawer";

const apiBaseUrl = resolveControlPlaneUrl();

type UserStatusFilter = "all" | "active" | "disabled";

type UserManagementFilters = {
  q: string;
  status: UserStatusFilter;
};

type UsersViewProps = {
  fetcher?: typeof fetch;
};

const defaultUserFilters: UserManagementFilters = {
  q: "",
  status: "all",
};

const v3TabTriggerClass =
  "min-w-max flex-1 rounded-[10px] px-4 py-2 text-[13px] font-semibold text-v3-ink-2 shadow-none transition-colors data-[state=active]:bg-v3-brand-soft data-[state=active]:text-v3-brand-deep data-[state=active]:shadow-none";

export function Users() {
  return <UsersView />;
}

export function UsersView({ fetcher }: UsersViewProps = {}) {
  const queryClient = useQueryClient();
  const [filters, setFilters] = useState<UserManagementFilters>(defaultUserFilters);
  const [selectedUserId, setSelectedUserId] = useState<string>();
  const [pendingCreatedUserId, setPendingCreatedUserId] = useState<string>();
  const [resetPasswordOpen, setResetPasswordOpen] = useState(false);
  const [resetPasswordValue, setResetPasswordValue] = useState("");
  const [createUserOpen, setCreateUserOpen] = useState(false);
  const apiOptions = useMemo(
    () => ({
      baseUrl: apiBaseUrl,
      fetcher,
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
        status: filters.status === "all" ? undefined : filters.status,
      }),
    queryKey: ["users", "management", filters],
  });
  const authzMembersQuery = useQuery({
    queryFn: () =>
      listAuthzMembers({
        ...apiOptions,
        limit: 100,
        offset: 0,
      }),
    queryKey: ["users", "authz-members"],
  });
  const loginLogsQuery = useQuery({
    queryFn: () =>
      listLoginLogs({
        ...apiOptions,
        limit: 50,
        offset: 0,
      }),
    queryKey: ["users", "login-logs"],
  });

  const users = usersQuery.data?.items ?? [];
  const authzMembersByUserId = useMemo(() => {
    return new Map((authzMembersQuery.data?.items ?? []).map((member) => [member.user_id, member]));
  }, [authzMembersQuery.data?.items]);
  const selectedUser = users.find((user) => user.id === selectedUserId) ?? users[0];
  const selectedMember = selectedUser ? authzMembersByUserId.get(selectedUser.id) : undefined;
  const selectedIdentity = selectedUser ? mergeUserIdentity(selectedUser, selectedMember) : undefined;
  const selectedLoginLogs = useMemo(() => {
    if (!selectedUser) {
      return [];
    }

    return (loginLogsQuery.data?.items ?? [])
      .filter((record) => record.user_id === selectedUser.id || record.username === selectedUser.username)
      .slice(0, 5);
  }, [loginLogsQuery.data?.items, selectedUser]);
  const deniedDecisionsQuery = useQuery({
    enabled: Boolean(selectedUser?.id),
    queryFn: () =>
      listAuthzDecisions({
        ...apiOptions,
        actor_id: selectedUser?.id,
        actor_type: "user",
        limit: 8,
        offset: 0,
        result: "failed",
      }),
    queryKey: ["users", "authz-denied-decisions", selectedUser?.id],
  });
  const projectTeamScopesQuery = useQuery({
    enabled: Boolean(selectedUser?.id),
    queryFn: () => listUserProjectTeamScopes(apiOptions, selectedUser?.id ?? ""),
    queryKey: ["users", "project-team-scopes", selectedUser?.id],
  });
  const deniedDecisions = deniedDecisionsQuery.data?.items ?? [];
  const stats = getUserStats(users, authzMembersQuery.data?.items ?? []);

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
    },
  });
  const resetPasswordMutation = useMutation({
    mutationFn: (input: { password: string; userId: string }) =>
      resetUserPassword(apiOptions, input.userId, input.password),
    onSuccess: () => {
      setResetPasswordOpen(false);
      setResetPasswordValue("");
      void invalidateUserWorkspace();
    },
  });
  const createUserMutation = useMutation({
    mutationFn: (input: CreateUserDraft) => {
      const payload: CreateUserRequest = {
        username: input.username,
        display_name: input.display_name,
        password: input.password,
        avatar: input.avatar,
        selectable_team_ids: input.selectable_team_ids,
      };
      return createUser(apiOptions, payload);
    },
    onSuccess: async (response) => {
      setCreateUserOpen(false);
      setFilters(defaultUserFilters);
      setPendingCreatedUserId(response.user.id);
      setSelectedUserId(response.user.id);
      await invalidateUserWorkspace();
    },
  });

  const handleCreateUserOpenChange = (open: boolean) => {
    createUserMutation.reset();
    setCreateUserOpen(open);
  };

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main className="min-w-0 overflow-x-hidden" fluid>
        <div className="mb-4 flex min-w-0 flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex min-w-0 items-center gap-3">
            <IconTile tone="mute" size="lg">
              <UsersRound />
            </IconTile>
            <div className="min-w-0">
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <h1 className="text-2xl font-bold tracking-normal text-v3-ink">用户管理</h1>
                <StatusPill tone="info" showDot={false}>用户 360</StatusPill>
              </div>
              <p className="text-sm text-v3-ink-2">
                平台人类用户、账号状态、可选团队、登录审计与权限诊断。
              </p>
            </div>
          </div>
          <V3Button onClick={() => handleCreateUserOpenChange(true)} type="button">
            <UserPlus data-icon="inline-start" />
            新建用户
          </V3Button>
        </div>

        <div className="mb-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
          <UserMetric icon={<CheckCircle2 />} label="活跃用户" tone="ok" value={stats.active} />
          <UserMetric icon={<Ban />} label="禁用用户" tone="danger" value={stats.disabled} />
          <UserMetric icon={<ShieldCheck />} label="控制台访问" tone="brand" value={stats.consoleAccess} />
          <UserMetric icon={<KeyRound />} label="成员身份" tone="artifact" value={stats.tenantRoles} />
          <UserMetric icon={<ShieldAlert />} label="近期拒绝" tone="warn" value={deniedDecisions.length} />
        </div>

        <div
          className="grid min-w-0 gap-4 xl:grid-cols-[340px_minmax(0,1fr)_300px]"
          data-columns="wide-list-balanced-detail"
          data-testid="users-management-layout"
        >
          <UserListRail
            filters={filters}
            isError={usersQuery.isError}
            isLoading={usersQuery.isLoading}
            onFiltersChange={setFilters}
            onSelectUser={setSelectedUserId}
            selectedUserId={selectedUser?.id}
            users={users}
          />

          <section className="min-w-0">
            {selectedIdentity && selectedUser ? (
              <SelectedUserWorkspace
                authzMembersError={authzMembersQuery.isError}
                deniedDecisions={deniedDecisions}
                deniedDecisionsError={deniedDecisionsQuery.isError}
                isStatusPending={statusMutation.isPending}
                loginLogs={selectedLoginLogs}
                loginLogsError={loginLogsQuery.isError}
                member={selectedMember}
                onResetPassword={() => setResetPasswordOpen(true)}
                onToggleStatus={() =>
                  statusMutation.mutate({
                    status: selectedUser.status === "active" ? "disabled" : "active",
                    userId: selectedUser.id,
                  })
                }
                projectTeamScopes={projectTeamScopesQuery.data?.items ?? []}
                projectTeamScopesError={projectTeamScopesQuery.isError}
                projectTeamScopesLoading={projectTeamScopesQuery.isLoading}
                user={selectedIdentity}
              />
            ) : (
              <SoftCard className="min-h-[420px]">
                {usersQuery.isLoading ? (
                  <V3LoadingState className="min-h-[420px]" label="加载用户中" />
                ) : (
                  <V3EmptyState className="min-h-[420px]" title="请选择一个用户查看详情" />
                )}
              </SoftCard>
            )}
          </section>

          <UserDiagnosticsRail
            deniedDecisions={deniedDecisions}
            deniedDecisionsError={deniedDecisionsQuery.isError}
            member={selectedMember}
            user={selectedIdentity}
          />
        </div>

        {selectedUser ? (
          <ResetPasswordDialog
            error={resetPasswordMutation.error}
            isOpen={resetPasswordOpen}
            isPending={resetPasswordMutation.isPending}
            onOpenChange={setResetPasswordOpen}
            onSubmit={() =>
              resetPasswordMutation.mutate({
                password: resetPasswordValue,
                userId: selectedUser.id,
              })
            }
            password={resetPasswordValue}
            setPassword={setResetPasswordValue}
            username={selectedUser.username}
          />
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

function UserListRail({
  filters,
  isError,
  isLoading,
  onFiltersChange,
  onSelectUser,
  selectedUserId,
  users,
}: {
  filters: UserManagementFilters;
  isError: boolean;
  isLoading: boolean;
  onFiltersChange: (filters: UserManagementFilters) => void;
  onSelectUser: (userId: string) => void;
  selectedUserId?: string;
  users: UserSummary[];
}) {
  return (
    <SoftCard className="min-w-0 p-4">
      <div className="flex flex-col gap-3">
        <div className="flex items-center justify-between gap-3">
          <div>
            <h2 className="text-base font-bold text-v3-ink">用户列表</h2>
            <p className="text-sm text-v3-ink-2">按账号状态和关键字快速定位。</p>
          </div>
          <StatusPill tone="mute" showDot={false}>{users.length}</StatusPill>
        </div>
        <div className="flex items-center gap-2 rounded-[10px] bg-v3-card-soft px-3 py-2 text-v3-ink-3">
          <SearchIcon className="size-4" />
          <Input
            aria-label="搜索用户"
            className="h-7 border-0 bg-transparent p-0 text-v3-ink shadow-none focus-visible:ring-0"
            onChange={(event) =>
              onFiltersChange({
                ...filters,
                q: event.target.value,
              })
            }
            placeholder="搜索用户名"
            type="search"
            value={filters.q}
          />
        </div>
        <div className="grid grid-cols-3 gap-2">
          {[
            ["all", "全部"],
            ["active", "活跃"],
            ["disabled", "禁用"],
          ].map(([status, label]) => (
            <V3Button
              aria-pressed={filters.status === status}
              key={status}
              onClick={() =>
                onFiltersChange({
                  ...filters,
                  status: status as UserStatusFilter,
                })
              }
              size="sm"
              type="button"
              variant={filters.status === status ? "primary" : "outline"}
            >
              {label}
            </V3Button>
          ))}
        </div>
      </div>
      <div className="mt-4">
        {isLoading ? (
          <V3LoadingState className="py-10" label="加载用户中" />
        ) : null}
        {isError ? (
          <V3ErrorState title="用户列表加载失败" description="请刷新页面或检查 Control Plane 连接。" />
        ) : null}
        {!isLoading && users.length === 0 ? (
          <V3EmptyState className="py-10" title="暂无匹配用户。" />
        ) : (
          <div className="flex min-w-0 flex-col gap-2">
            {users.map((user) => (
              <button
                className={cn(
                  "flex w-full min-w-0 items-center justify-between gap-3 rounded-v3-inner border p-3 text-left transition-colors",
                  selectedUserId === user.id
                    ? "border-v3-brand bg-v3-brand-soft"
                    : "border-v3-line bg-v3-card hover:bg-v3-card-soft",
                )}
                key={user.id}
                onClick={() => onSelectUser(user.id)}
                type="button"
              >
                <UserIdentity
                  className="min-w-0 flex-1"
                  showSecondary
                  user={{
                    avatar: user.avatar,
                    avatar_asset_id: user.avatar_asset_id,
                    display_name: user.display_name,
                    email: user.email,
                    id: user.id,
                    status: user.status,
                    username: user.username,
                  }}
                />
                <StatusPill tone={userStatusTone(user.status)}>
                  {formatUserStatus(user.status)}
                </StatusPill>
              </button>
            ))}
          </div>
        )}
      </div>
    </SoftCard>
  );
}

function SelectedUserWorkspace({
  authzMembersError,
  deniedDecisions,
  deniedDecisionsError,
  isStatusPending,
  loginLogs,
  loginLogsError,
  member,
  onResetPassword,
  onToggleStatus,
  projectTeamScopes,
  projectTeamScopesError,
  projectTeamScopesLoading,
  user,
}: {
  authzMembersError: boolean;
  deniedDecisions: AuthzDecisionRecord[];
  deniedDecisionsError: boolean;
  isStatusPending: boolean;
  loginLogs: LoginLogRecord[];
  loginLogsError: boolean;
  member?: AuthzMemberRecord;
  onResetPassword: () => void;
  onToggleStatus: () => void;
  projectTeamScopes: UserProjectTeamScope[];
  projectTeamScopesError: boolean;
  projectTeamScopesLoading: boolean;
  user: UserIdentityData;
}) {
  const label = getUserIdentityLabel(user);

  return (
    <Tabs className="gap-4" defaultValue="overview">
      <SoftCard className="p-6">
        <div className="flex min-w-0 flex-col gap-4">
          <div className="flex min-w-0 flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
            <div className="flex min-w-0 items-start gap-4">
              <UserIdentityAvatar className="size-16" user={user} />
              <div className="min-w-0">
                <div className="flex min-w-0 flex-wrap items-center gap-2">
                  <h2 className="text-2xl font-semibold tracking-normal text-v3-ink">{label.primary}</h2>
                  <StatusPill tone={userStatusTone(user.status)}>
                    {formatUserStatus(user.status)}
                  </StatusPill>
                  <StatusPill tone={member?.console_access ? "ok" : "mute"}>
                    控制台访问：{member?.console_access ? "允许" : "未确认"}
                  </StatusPill>
                </div>
                <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-sm text-v3-ink-2">
                  <span>{user.username ?? user.id}</span>
                  <span>{label.secondary}</span>
                  <span>用户 ID：{shortId(user.id)}</span>
                </div>
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              <V3Button
                disabled={isStatusPending}
                onClick={onToggleStatus}
                type="button"
                variant={user.status === "active" ? "danger" : "outline"}
              >
                {user.status === "active" ? <Ban data-icon="inline-start" /> : <RotateCcw data-icon="inline-start" />}
                {user.status === "active" ? "禁用账号" : "启用账号"}
              </V3Button>
              <V3Button onClick={onResetPassword} type="button" variant="outline">
                <LockKeyhole data-icon="inline-start" />
                重置密码
              </V3Button>
              <V3Button asChild variant="outline">
                <Link to="/teams">
                  <UsersRound data-icon="inline-start" />
                  去团队管理分配
                </Link>
              </V3Button>
            </div>
          </div>
          <div className="border-t border-v3-line pt-4">
            <V3Tabs className="w-full">
              <TabsList className="flex h-auto w-full flex-wrap justify-start gap-1 bg-transparent p-0">
                <TabsTrigger className={v3TabTriggerClass} value="overview">概览</TabsTrigger>
                <TabsTrigger className={v3TabTriggerClass} value="selectable-teams">可选团队</TabsTrigger>
                <TabsTrigger className={v3TabTriggerClass} value="sessions">登录与会话</TabsTrigger>
                <TabsTrigger className={v3TabTriggerClass} value="audit">审计记录</TabsTrigger>
              </TabsList>
            </V3Tabs>
          </div>
        </div>
      </SoftCard>

      <TabsContent value="overview">
        <div className="flex min-w-0 flex-col gap-4">
          <div
            className="grid min-w-0 gap-6 [grid-template-columns:repeat(auto-fit,minmax(min(100%,15rem),1fr))]"
            data-layout="equal-three-cards"
            data-testid="users-overview-hero"
          >
            <BasicInfoCard user={user} />
            <PermissionSnapshotCard member={member} user={user} />
            <AccountTimeline
              className="min-w-0"
              decisions={deniedDecisions}
              error={deniedDecisionsError}
              logs={loginLogs}
            />
          </div>
          <div className="grid min-w-0 gap-4 lg:grid-cols-2">
            <MembershipTable error={authzMembersError} member={member} />
            <LoginLogTable error={loginLogsError} logs={loginLogs} />
          </div>
        </div>
      </TabsContent>
      <TabsContent value="selectable-teams">
        <SelectableTeamScopesCard
          error={projectTeamScopesError}
          isLoading={projectTeamScopesLoading}
          scopes={projectTeamScopes}
        />
      </TabsContent>
      <TabsContent value="sessions">
        <LoginLogTable error={loginLogsError} logs={loginLogs} />
      </TabsContent>
      <TabsContent value="audit">
        <AccountTimeline decisions={deniedDecisions} error={deniedDecisionsError} logs={loginLogs} />
      </TabsContent>
    </Tabs>
  );
}

function BasicInfoCard({ user }: { user: UserIdentityData }) {
  const label = getUserIdentityLabel(user);

  return (
    <SoftCard className="min-w-0 p-5" data-testid="users-overview-basic-card">
      <div className="mb-4">
        <h3 className="text-base font-bold text-v3-ink">基本信息</h3>
        <p className="text-sm text-v3-ink-2">来自用户列表和权限中心成员视图。</p>
      </div>
      <div className="grid gap-3 text-sm">
        <InfoRow label="姓名" value={label.primary} />
        <InfoRow label="用户名" value={user.username ?? "-"} />
        <InfoRow label="邮箱/标识" value={label.secondary} />
        <InfoRow label="账号状态" value={formatUserStatus(user.status)} />
        <InfoRow label="用户 ID" value={user.id} />
      </div>
    </SoftCard>
  );
}

function PermissionSnapshotCard({
  member,
  user,
}: {
  member?: AuthzMemberRecord;
  user: UserIdentityData;
}) {
  const firstMembership = member?.memberships[0];

  return (
    <SoftCard className="min-w-0 p-5" data-testid="users-overview-permission-card">
      <div className="mb-4">
        <h3 className="text-base font-bold text-v3-ink">权限快速校验</h3>
        <p className="text-sm text-v3-ink-2">用于检查账号是否能进入关键管理动作。</p>
      </div>
      <div className="grid gap-3 text-sm">
        <InfoRow label="actor" value={`user:${shortId(user.id)}`} />
        <InfoRow label="action" value={firstMembership?.team_id ? "team.member.add" : "console.access"} />
        <InfoRow label="resource" value={firstMembership?.team_id ? `team:${shortId(firstMembership.team_id)}` : "console:web"} />
        <div className="flex items-center justify-between rounded-v3-inner border border-v3-line bg-v3-card-soft px-3 py-2">
          <span className="text-v3-ink-2">结果</span>
          <StatusPill tone={member?.console_access ? "ok" : "warn"}>
            {member?.console_access ? "允许" : "待诊断"}
          </StatusPill>
        </div>
      </div>
    </SoftCard>
  );
}

function MembershipTable({
  error,
  member,
}: {
  error: boolean;
  member?: AuthzMemberRecord;
}) {
  const memberships = member?.memberships ?? [];

  return (
    <WorkSurface className="min-w-0">
      <div className="border-b border-v3-line p-4">
        <h3 className="text-base font-bold text-v3-ink">成员身份记录</h3>
        <p className="text-sm text-v3-ink-2">只读审计视图，角色调整仍通过团队管理页完成。</p>
      </div>
        {error ? (
          <V3ErrorState className="m-4" title="成员角色加载失败" description="权限中心成员视图暂不可用。" />
        ) : memberships.length === 0 ? (
          <V3EmptyState title="暂无团队或成员身份记录。" />
        ) : (
          <V3Table>
            <thead>
              <V3Tr>
                <V3Th>范围</V3Th>
                <V3Th>角色</V3Th>
                <V3Th>状态</V3Th>
              </V3Tr>
            </thead>
            <tbody>
                {memberships.map((membership) => (
                  <V3Tr key={`${membership.tenant_id}-${membership.team_id ?? "tenant"}-${membership.role}`}>
                    <V3Td className="min-w-36">{formatMembershipScope(membership)}</V3Td>
                    <V3Td>
                      <StatusPill tone="mute">{membership.role}</StatusPill>
                    </V3Td>
                    <V3Td>
                      <StatusPill tone={membership.status === "active" ? "ok" : "mute"}>{membership.status}</StatusPill>
                    </V3Td>
                  </V3Tr>
                ))}
            </tbody>
          </V3Table>
        )}
    </WorkSurface>
  );
}

function SelectableTeamScopesCard({
  error,
  isLoading,
  scopes,
}: {
  error: boolean;
  isLoading: boolean;
  scopes: UserProjectTeamScope[];
}) {
  return (
    <SoftCard className="min-w-0 p-5">
      <div className="mb-4">
        <h3 className="text-base font-bold text-v3-ink">可选团队</h3>
        <p className="text-sm text-v3-ink-2">当前用户创建或协作项目时可选择的团队范围。</p>
      </div>
        {isLoading ? <V3LoadingState className="py-10" label="加载可选团队中" /> : null}
        {error ? (
          <V3ErrorState title="可选团队加载失败" description="请检查用户团队范围接口。" />
        ) : null}
        {!isLoading && !error && scopes.length === 0 ? (
          <V3EmptyState className="py-10" title="暂无可选团队。" />
        ) : null}
        {scopes.length > 0 ? (
          <div className="grid gap-3 md:grid-cols-2">
            {scopes.map((scope) => (
              <div className="min-w-0 rounded-v3-inner border border-v3-line bg-v3-card-soft p-3" key={scope.id}>
                <div className="flex min-w-0 items-start justify-between gap-3">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium text-v3-ink">{scope.team.name}</p>
                    <p className="mt-1 truncate text-xs text-v3-ink-2">
                      {scope.team.slug} / {scope.team.governance_status}
                    </p>
                  </div>
                  <StatusPill tone={scope.status === "active" ? "ok" : "mute"}>
                    {scope.status}
                  </StatusPill>
                </div>
                <div className="mt-3 grid grid-cols-2 gap-2 text-xs text-v3-ink-2">
                  <span>员工 {scope.team.digital_employee_count}</span>
                  <span>草稿 {scope.team.pending_draft_count}</span>
                  <span className="col-span-2 truncate">{scope.team.risk_summary}</span>
                </div>
              </div>
            ))}
          </div>
        ) : null}
    </SoftCard>
  );
}

function LoginLogTable({
  error,
  logs,
}: {
  error: boolean;
  logs: LoginLogRecord[];
}) {
  return (
    <WorkSurface className="min-w-0">
      <div className="border-b border-v3-line p-4">
        <h3 className="text-base font-bold text-v3-ink">最近登录日志</h3>
        <p className="text-sm text-v3-ink-2">按当前用户从 Web 登录日志中匹配。</p>
      </div>
        {error ? (
          <V3ErrorState className="m-4" title="登录日志加载失败" description="请检查 Auth API 连接。" />
        ) : logs.length === 0 ? (
          <V3EmptyState title="暂无登录记录。" />
        ) : (
          <V3Table>
            <thead>
              <V3Tr>
                <V3Th>时间</V3Th>
                <V3Th>IP 地址</V3Th>
                <V3Th>设备 / 浏览器</V3Th>
                <V3Th>结果</V3Th>
              </V3Tr>
            </thead>
            <tbody>
                {logs.map((log) => (
                  <V3Tr key={log.id} tone={log.result === "succeeded" ? undefined : "danger"}>
                    <V3Td className="min-w-32 tabular-nums">{formatDateTime(log.created_at)}</V3Td>
                    <V3Td>{log.client_ip ?? "-"}</V3Td>
                    <V3Td className="min-w-40">{log.user_agent ?? "-"}</V3Td>
                    <V3Td>
                      <StatusPill tone={log.result === "succeeded" ? "ok" : "danger"}>
                        {log.result === "succeeded" ? "成功" : "失败"}
                      </StatusPill>
                    </V3Td>
                  </V3Tr>
                ))}
            </tbody>
          </V3Table>
        )}
    </WorkSurface>
  );
}

function AccountTimeline({
  className,
  decisions,
  error,
  logs,
}: {
  className?: string;
  decisions: AuthzDecisionRecord[];
  error: boolean;
  logs: LoginLogRecord[];
}) {
  if (error) {
    return (
      <V3ErrorState title="审计记录加载失败" description="授权拒绝记录暂不可用。" />
    );
  }

  const events = [
    ...decisions.map((decision) => ({
      at: decision.created_at,
      description: decision.reason ?? decision.result,
      title: decision.action,
      tone: "danger" as const,
    })),
    ...logs.slice(0, 3).map((log) => ({
      at: log.created_at,
      description: `${log.user_agent ?? "unknown"} / ${log.client_ip ?? "-"}`,
      title: `login.${log.result}`,
      tone: log.result === "succeeded" ? ("ok" as const) : ("danger" as const),
    })),
  ].sort((a, b) => new Date(b.at).getTime() - new Date(a.at).getTime());

  return (
    <SoftCard className={cn("min-w-0 p-5", className)} data-testid="users-overview-timeline-card">
      <div className="mb-4">
        <h3 className="text-base font-bold text-v3-ink">账号操作记录</h3>
        <p className="text-sm text-v3-ink-2">登录事件与授权拒绝事件的合并视图。</p>
      </div>
        {events.length === 0 ? (
          <V3EmptyState className="py-10" title="暂无可展示事件。" />
        ) : (
          <div className="flex flex-col gap-3">
            {events.map((event) => (
              <div className="flex gap-3 text-sm" key={`${event.title}-${event.at}`}>
                <span className={cn("mt-1 size-2 rounded-full bg-current", event.tone === "ok" ? "text-v3-ok" : "text-v3-danger")} />
                <div className="min-w-0 flex-1">
                  <div className="flex min-w-0 flex-wrap items-center gap-2">
                    <span className="truncate font-medium text-v3-ink">{event.title}</span>
                    <StatusPill tone={event.tone}>{event.tone === "ok" ? "成功" : "需关注"}</StatusPill>
                  </div>
                  <p className="truncate text-xs text-v3-ink-2">{event.description}</p>
                  <p className="text-xs text-v3-ink-2">{formatDateTime(event.at)}</p>
                </div>
              </div>
            ))}
          </div>
        )}
    </SoftCard>
  );
}

function UserDiagnosticsRail({
  deniedDecisions,
  deniedDecisionsError,
  member,
  user,
}: {
  deniedDecisions: AuthzDecisionRecord[];
  deniedDecisionsError: boolean;
  member?: AuthzMemberRecord;
  user?: UserIdentityData;
}) {
  const allowRate = member?.console_access ? "可访问" : "未确认";

  return (
    <aside className="flex min-w-0 flex-col gap-4">
      <SoftCard className="p-5">
        <div className="mb-4">
          <h3 className="flex items-center gap-2 text-base font-bold text-v3-ink">
            <ShieldCheck />
            权限诊断
          </h3>
          <p className="text-sm text-v3-ink-2">从权限中心聚合当前用户的可访问性和拒绝信号。</p>
        </div>
        <div className="grid gap-3 text-sm">
          <div className="rounded-v3-inner border border-v3-line bg-v3-card-soft p-3">
            <p className="text-xs text-v3-ink-2">控制台访问</p>
            <p className="mt-1 text-xl font-semibold text-v3-ink">{allowRate}</p>
          </div>
          <InfoRow label="账号状态" value={user ? formatUserStatus(user.status) : "-"} />
          <InfoRow label="成员身份数量" value={String(member?.memberships.length ?? 0)} />
          <InfoRow label="最近拒绝" value={member?.recent_denied_reason ?? "暂无"} />
        </div>
      </SoftCard>

      <SoftCard className="p-5">
        <div className="mb-4">
          <h3 className="flex items-center gap-2 text-base font-bold text-v3-ink">
            <ShieldAlert />
            近期拒绝
          </h3>
          <p className="text-sm text-v3-ink-2">最近 8 条失败授权决策。</p>
        </div>
          {deniedDecisionsError ? (
            <V3ErrorState title="拒绝事件加载失败。" />
          ) : deniedDecisions.length === 0 ? (
            <V3EmptyState className="py-10" title="暂无拒绝事件。" />
          ) : (
            <div className="flex flex-col gap-2">
              {deniedDecisions.map((decision) => (
                <div className="rounded-v3-inner border border-v3-line bg-v3-card-soft p-3 text-sm" key={decision.id}>
                  <div className="flex items-center justify-between gap-3">
                    <span className="min-w-0 truncate font-medium text-v3-ink">{decision.action}</span>
                    <StatusPill tone="danger">拒绝</StatusPill>
                  </div>
                  <p className="mt-1 truncate text-xs text-v3-ink-2">{decision.reason ?? "-"}</p>
                  <p className="mt-1 text-xs text-v3-ink-2">{formatDateTime(decision.created_at)}</p>
                </div>
              ))}
            </div>
          )}
      </SoftCard>

      <SoftCard className="p-5">
        <div className="mb-4">
          <h3 className="flex items-center gap-2 text-base font-bold text-v3-ink">
            <Activity />
            下一步建议
          </h3>
        </div>
        <div className="flex flex-col gap-2 text-sm text-v3-ink">
          <Recommendation icon={<KeyRound />} text="定期轮换密码或接入 SSO 后迁移认证策略。" />
          <Recommendation icon={<UsersRound />} text="团队可选范围变更优先在团队管理中走审批链路。" />
          <Recommendation icon={<Clock3 />} text="导出最近 30 天登录与拒绝记录，供审计复核。" />
          <V3Button asChild className="mt-2" variant="outline">
            <Link to="/permissions">
              查看权限中心
              <ArrowUpRight data-icon="inline-end" />
            </Link>
          </V3Button>
        </div>
      </SoftCard>
    </aside>
  );
}

function Recommendation({ icon, text }: { icon: ReactNode; text: string }) {
  return (
    <div className="flex gap-2 rounded-v3-inner border border-v3-line bg-v3-card-soft p-3">
      <span className="text-v3-brand">{icon}</span>
      <span>{text}</span>
    </div>
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
  username,
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
            <V3Button onClick={() => onOpenChange(false)} type="button" variant="outline">
              取消
            </V3Button>
            <V3Button disabled={isPending || password.length < 4} type="submit">
              确认重置
            </V3Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function UserMetric({
  icon,
  label,
  tone,
  value,
}: {
  icon: ReactNode;
  label: string;
  tone: V3Tone;
  value: number;
}) {
  return (
    <V3MetricCard
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
    <div className="flex min-w-0 items-center justify-between gap-3 rounded-v3-inner border border-v3-line bg-v3-card-soft px-3 py-2">
      <span className="shrink-0 text-v3-ink-2">{label}</span>
      <span className="min-w-0 truncate text-right font-medium text-v3-ink">{value}</span>
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
    username: member?.username ?? user.username,
  };
}

function getUserStats(users: UserSummary[], members: AuthzMemberRecord[]) {
  const active = users.filter((user) => user.status === "active").length;
  const disabled = users.filter((user) => user.status === "disabled").length;
  const consoleAccess = members.filter((member) => member.console_access).length;
  const tenantRoles = members.reduce(
    (count, member) => count + member.memberships.filter((membership) => !membership.team_id).length,
    0,
  );

  return {
    active,
    consoleAccess,
    disabled,
    tenantRoles,
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

function userStatusTone(status: string): V3Tone {
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
    return `团队 ${shortId(membership.team_id)}`;
  }

  return `租户 ${shortId(membership.tenant_id)}`;
}

function shortId(value: string) {
  if (value.length <= 10) {
    return value;
  }

  return `${value.slice(0, 8)}...${value.slice(-4)}`;
}

function formatDateTime(value: string) {
  const date = new Date(value);

  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat("zh-CN", {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "2-digit",
    year: "numeric",
  }).format(date);
}

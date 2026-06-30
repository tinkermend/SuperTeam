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
  UsersRound,
} from "lucide-react";
import {
  SoftCard,
  StatusPill,
  V3Button,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3MetricCard,
  V3PageHeader,
  V3Segmented,
  V3Table,
  V3Td,
  V3Th,
  V3ToolbarSearch,
  V3Tr,
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
import {
  listAuthzMembers,
  createUser,
  listUsers,
  resetUserPassword,
  updateUserStatus,
  type AuthzMemberRecord,
  type CreateUserRequest,
  type UserSummary,
} from "@/lib/api";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import {
  CreateUserDrawer,
  type CreateUserDraft,
} from "./components/create-user-drawer";

const apiBaseUrl = resolveControlPlaneUrl();

type UserStatusFilter = "all" | "active" | "disabled";
type UserTableDensity = "comfortable" | "compact";

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
  const [tableDensity, setTableDensity] = useState<UserTableDensity>("comfortable");
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

  const users = usersQuery.data?.items ?? [];
  const authzMembersByUserId = useMemo(() => {
    return new Map((authzMembersQuery.data?.items ?? []).map((member) => [member.user_id, member]));
  }, [authzMembersQuery.data?.items]);
  const selectedUser = users.find((user) => user.id === selectedUserId) ?? users[0];
  const selectedMember = selectedUser ? authzMembersByUserId.get(selectedUser.id) : undefined;
  const selectedIdentity = selectedUser ? mergeUserIdentity(selectedUser, selectedMember) : undefined;
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
        <V3PageHeader
          className="mb-4"
          icon={<UsersRound />}
          iconTone="mute"
          title={
            <span className="inline-flex flex-wrap items-center gap-2">
              用户管理
              <StatusPill className="align-middle" tone="info" showDot={false}>用户治理台</StatusPill>
            </span>
          }
          subtitle="管理平台人类用户、账号状态、控制台访问与成员身份；本页只处理账号治理动作。"
          actions={
            <V3Button onClick={() => handleCreateUserOpenChange(true)} type="button">
              <UserPlus data-icon="inline-start" />
              新建用户
            </V3Button>
          }
        />

        <div className="mb-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
          <UserMetric icon={<CheckCircle2 />} label="活跃用户" tone="ok" value={stats.active} />
          <UserMetric icon={<Ban />} label="禁用用户" tone="danger" value={stats.disabled} />
          <UserMetric icon={<ShieldCheck />} label="控制台访问" tone="brand" value={stats.consoleAccess} />
          <UserMetric icon={<KeyRound />} label="成员身份" tone="artifact" value={stats.tenantRoles} />
        </div>

        <div
          className="grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1fr)_324px]"
          data-layout="table-governance"
          data-testid="users-management-layout"
        >
          <UserGovernanceTable
            authzMembersByUserId={authzMembersByUserId}
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
                userId: user.id,
              })
            }
            selectedUserId={selectedUser?.id}
            users={users}
          />

          <UserGovernancePreview
            isStatusPending={statusMutation.isPending}
            member={selectedMember}
            onResetPassword={() => setResetPasswordOpen(true)}
            onToggleStatus={() => {
              if (!selectedUser) {
                return;
              }
              statusMutation.mutate({
                status: selectedUser.status === "active" ? "disabled" : "active",
                userId: selectedUser.id,
              });
            }}
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

function UserGovernanceTable({
  authzMembersByUserId,
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
  users,
}: {
  authzMembersByUserId: Map<string, AuthzMemberRecord>;
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
      <div className="flex flex-col gap-3 border-b border-v3-line p-4">
        <div className="flex min-w-0 flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div className="min-w-0">
            <h2 className="text-base font-bold text-v3-ink">用户治理表</h2>
            <p className="text-sm text-v3-ink-2">逐行治理人类用户的账号状态、控制台访问与成员身份。</p>
          </div>
          <V3Segmented
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
          <V3ToolbarSearch
            aria-label="搜索用户"
            onChange={(event) =>
              onFiltersChange({
                ...filters,
                q: event.target.value,
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
      </div>

      {isLoading ? <V3LoadingState className="min-h-[360px]" label="加载用户中" /> : null}
      {isError ? (
        <V3ErrorState className="m-4" title="用户列表加载失败" description="请刷新页面或检查 Control Plane 连接。" />
      ) : null}
      {!isLoading && !isError && users.length === 0 ? (
        <V3EmptyState className="min-h-[360px]" title="暂无匹配用户。" />
      ) : null}
      {!isLoading && !isError && users.length > 0 ? (
        <>
          <V3Table className={density === "compact" ? "[&_td]:py-2 [&_th]:py-2" : undefined}>
            <thead>
              <V3Tr>
                <V3Th>用户</V3Th>
                <V3Th>状态</V3Th>
                <V3Th>控制台访问</V3Th>
                <V3Th>成员身份</V3Th>
                <V3Th>操作</V3Th>
              </V3Tr>
            </thead>
            <tbody>
              {users.map((user) => {
                const member = authzMembersByUserId.get(user.id);
                const selected = user.id === selectedUserId;
                const identity = mergeUserIdentity(user, member);
                return (
                  <V3Tr
                    className={selected ? "[&>td]:bg-v3-brand-soft/55" : undefined}
                    key={user.id}
                    onClick={() => onSelectUser(user.id)}
                    tone={user.status === "disabled" ? "danger" : undefined}
                  >
                    <V3Td className="min-w-[220px]">
                      <UserIdentity className="min-w-0" showSecondary user={identity} />
                    </V3Td>
                    <V3Td>
                      <StatusPill tone={userStatusTone(user.status)}>{formatUserStatus(user.status)}</StatusPill>
                    </V3Td>
                    <V3Td>
                      <StatusPill tone={member?.console_access ? "ok" : "mute"}>
                        {member?.console_access ? "允许" : "未确认"}
                      </StatusPill>
                    </V3Td>
                    <V3Td className="min-w-[150px]">{formatMembershipSummary(member)}</V3Td>
                    <V3Td>
                      <div className="flex min-w-max gap-2">
                        <V3Button
                          onClick={(event) => {
                            event.stopPropagation();
                            onSelectUser(user.id);
                          }}
                          size="sm"
                          type="button"
                          variant="outline"
                        >
                          详情
                        </V3Button>
                        <V3Button
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
                        </V3Button>
                        <V3Button
                          onClick={(event) => {
                            event.stopPropagation();
                            onResetPassword(user.id);
                          }}
                          size="sm"
                          type="button"
                          variant="outline"
                        >
                          重置密码
                        </V3Button>
                      </div>
                    </V3Td>
                  </V3Tr>
                );
              })}
            </tbody>
          </V3Table>
          <div className="flex flex-col gap-2 border-t border-v3-line px-4 py-3 text-sm text-v3-ink-2 sm:flex-row sm:items-center sm:justify-between">
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
  isStatusPending,
  member,
  onResetPassword,
  onToggleStatus,
  user,
}: {
  isStatusPending: boolean;
  member?: AuthzMemberRecord;
  onResetPassword: () => void;
  onToggleStatus: () => void;
  user?: UserIdentityData;
}) {
  if (!user) {
    return (
      <aside className="flex min-w-0 flex-col gap-4">
        <SoftCard className="min-h-[420px]">
          <V3EmptyState className="min-h-[420px]" title="请选择一个用户查看详情" />
        </SoftCard>
      </aside>
    );
  }

  const label = getUserIdentityLabel(user);
  const memberships = member?.memberships ?? [];

  return (
    <aside className="flex min-w-0 flex-col gap-4">
      <SoftCard className="p-5">
        <div className="mb-4 flex min-w-0 items-start gap-3">
          <UserIdentityAvatar className="size-14" user={user} />
          <div className="min-w-0">
            <h2 className="truncate text-xl font-bold tracking-normal text-v3-ink">{label.primary}</h2>
            <div className="mt-2 flex flex-wrap gap-2">
              <StatusPill tone={userStatusTone(user.status)}>{formatUserStatus(user.status)}</StatusPill>
              <StatusPill tone={member?.console_access ? "ok" : "mute"}>
                控制台访问：{member?.console_access ? "允许" : "未确认"}
              </StatusPill>
            </div>
          </div>
        </div>
        <div className="grid gap-3 text-sm">
          <InfoRow label="邮箱/标识" value={label.secondary} />
          <InfoRow label="用户名" value={user.username ?? "-"} />
          <InfoRow label="用户 ID" value={shortId(user.id)} />
          <InfoRow label="成员身份" value={formatMembershipSummary(member)} />
        </div>
        <div className="mt-4 flex flex-wrap gap-2">
          <V3Button
            disabled={isStatusPending}
            onClick={onToggleStatus}
            size="sm"
            type="button"
            variant={user.status === "active" ? "danger" : "outline"}
          >
            {user.status === "active" ? <Ban data-icon="inline-start" /> : <RotateCcw data-icon="inline-start" />}
            {user.status === "active" ? "禁用账号" : "启用账号"}
          </V3Button>
          <V3Button onClick={onResetPassword} size="sm" type="button" variant="outline">
            <LockKeyhole data-icon="inline-start" />
            重置密码
          </V3Button>
          <V3Button asChild size="sm" variant="outline">
            <Link to="/teams">
              <UsersRound data-icon="inline-start" />
              去团队管理分配
            </Link>
          </V3Button>
        </div>
      </SoftCard>

      <SoftCard className="p-5">
        <div className="mb-4">
          <h3 className="text-base font-bold text-v3-ink">成员身份</h3>
          <p className="text-sm text-v3-ink-2">来自权限中心成员视图；角色调整仍通过团队管理页完成。</p>
        </div>
        {memberships.length === 0 ? (
          <V3EmptyState className="py-8" title="暂无成员身份。" />
        ) : (
          <div className="flex flex-col gap-2">
            {memberships.map((membership) => (
              <div
                className="flex items-center justify-between gap-3 rounded-v3-inner border border-v3-line bg-v3-card-soft px-3 py-2 text-sm"
                key={`${membership.tenant_id}-${membership.team_id ?? "tenant"}-${membership.role}`}
              >
                <span className="truncate text-v3-ink">{formatMembershipScope(membership)}</span>
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

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
  LiquidPill,
  LiquidTabsList,
  LiquidTabsTrigger,
  SemanticIconTile,
  StatusBadge,
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
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
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
import { Separator } from "@/components/ui/separator";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import {
  createUser,
  listAuthzDecisions,
  listAuthzMembers,
  listLoginLogs,
  listUsers,
  resetUserPassword,
  updateUserStatus,
  type AuthzDecisionRecord,
  type AuthzMemberRecord,
  type LoginLogRecord,
  type UserSummary,
} from "@/lib/api";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { cn } from "@/lib/utils";

const apiBaseUrl = resolveControlPlaneUrl();

type UserStatusFilter = "all" | "active" | "disabled";

type UserManagementFilters = {
  q: string;
  status: UserStatusFilter;
};

type UsersViewProps = {
  fetcher?: typeof fetch;
};

export function Users() {
  return <UsersView />;
}

export function UsersView({ fetcher }: UsersViewProps = {}) {
  const queryClient = useQueryClient();
  const [filters, setFilters] = useState<UserManagementFilters>({
    q: "",
    status: "all",
  });
  const [selectedUserId, setSelectedUserId] = useState<string>();
  const [resetPasswordOpen, setResetPasswordOpen] = useState(false);
  const [resetPasswordValue, setResetPasswordValue] = useState("");
  const [createUserOpen, setCreateUserOpen] = useState(false);
  const [createDraft, setCreateDraft] = useState({
    avatarSeed: "",
    password: "",
    username: "",
  });
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
  const deniedDecisions = deniedDecisionsQuery.data?.items ?? [];
  const stats = getUserStats(users, authzMembersQuery.data?.items ?? []);

  useEffect(() => {
    if (users.length === 0) {
      setSelectedUserId(undefined);
      return;
    }

    if (!selectedUserId || !users.some((user) => user.id === selectedUserId)) {
      setSelectedUserId(users[0].id);
    }
  }, [selectedUserId, users]);

  const invalidateUserWorkspace = () => {
    void queryClient.invalidateQueries({ queryKey: ["users"] });
  };
  const statusMutation = useMutation({
    mutationFn: (input: { status: UserSummary["status"]; userId: string }) =>
      updateUserStatus(apiOptions, input.userId, input.status),
    onSuccess: invalidateUserWorkspace,
  });
  const resetPasswordMutation = useMutation({
    mutationFn: (input: { password: string; userId: string }) =>
      resetUserPassword(apiOptions, input.userId, input.password),
    onSuccess: () => {
      setResetPasswordOpen(false);
      setResetPasswordValue("");
      invalidateUserWorkspace();
    },
  });
  const createUserMutation = useMutation({
    mutationFn: (input: { avatarSeed: string; password: string; username: string }) =>
      createUser(apiOptions, {
        avatar: {
          provider: "dicebear",
          seed: input.avatarSeed.trim() || input.username.trim(),
          style: "adventurer",
        },
        password: input.password,
        username: input.username.trim(),
      }),
    onSuccess: (response) => {
      setCreateUserOpen(false);
      setCreateDraft({ avatarSeed: "", password: "", username: "" });
      setSelectedUserId(response.user.id);
      invalidateUserWorkspace();
    },
  });

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main className="min-w-0 overflow-x-hidden" fluid>
        <div className="mb-4 flex min-w-0 flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex min-w-0 items-center gap-3">
            <SemanticIconTile tone="neutral" size="lg">
              <UsersRound />
            </SemanticIconTile>
            <div className="min-w-0">
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <h1 className="text-2xl font-bold tracking-normal">用户管理</h1>
                <LiquidPill className="py-1 text-xs">用户 360</LiquidPill>
              </div>
              <p className="text-sm text-muted-foreground">
                平台人类用户、账号状态、团队角色、登录审计与权限诊断。
              </p>
            </div>
          </div>
          <Button onClick={() => setCreateUserOpen(true)} type="button">
            <UserPlus data-icon="inline-start" />
            新建用户
          </Button>
        </div>

        <div className="mb-4 grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
          <UserMetric icon={<CheckCircle2 />} label="活跃用户" tone="success" value={stats.active} />
          <UserMetric icon={<Ban />} label="禁用用户" tone="danger" value={stats.disabled} />
          <UserMetric icon={<ShieldCheck />} label="控制台访问" tone="decision" value={stats.consoleAccess} />
          <UserMetric icon={<KeyRound />} label="租户级角色" tone="artifact" value={stats.tenantRoles} />
          <UserMetric icon={<ShieldAlert />} label="近期拒绝" tone="warning" value={deniedDecisions.length} />
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
                user={selectedIdentity}
              />
            ) : (
              <Card className="min-h-[420px] rounded-md">
                <CardContent className="flex min-h-[420px] items-center justify-center text-sm text-muted-foreground">
                  {usersQuery.isLoading ? "加载用户中" : "请选择一个用户查看详情"}
                </CardContent>
              </Card>
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
        <CreateUserDialog
          draft={createDraft}
          error={createUserMutation.error}
          isOpen={createUserOpen}
          isPending={createUserMutation.isPending}
          onDraftChange={setCreateDraft}
          onOpenChange={setCreateUserOpen}
          onSubmit={() => createUserMutation.mutate(createDraft)}
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
    <Card className="min-w-0 rounded-md">
      <CardHeader className="gap-3">
        <div className="flex items-center justify-between gap-3">
          <div>
            <CardTitle className="text-base">用户列表</CardTitle>
            <CardDescription>按账号状态和关键字快速定位。</CardDescription>
          </div>
          <Badge variant="secondary">{users.length}</Badge>
        </div>
        <div className="flex items-center gap-2 rounded-full border bg-background/70 px-3 py-2">
          <SearchIcon className="text-muted-foreground" />
          <Input
            aria-label="搜索用户"
            className="h-7 border-0 bg-transparent p-0 shadow-none focus-visible:ring-0"
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
            <Button
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
              variant={filters.status === status ? "default" : "outline"}
            >
              {label}
            </Button>
          ))}
        </div>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <p className="text-sm text-muted-foreground">加载用户中</p>
        ) : null}
        {isError ? (
          <Alert variant="destructive">
            <ShieldAlert />
            <AlertTitle>用户列表加载失败</AlertTitle>
            <AlertDescription>请刷新页面或检查 Control Plane 连接。</AlertDescription>
          </Alert>
        ) : null}
        {!isLoading && users.length === 0 ? (
          <p className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">暂无匹配用户。</p>
        ) : (
          <div className="flex min-w-0 flex-col gap-2">
            {users.map((user) => (
              <button
                className={cn(
                  "flex w-full min-w-0 items-center justify-between gap-3 rounded-md border p-3 text-left transition-colors",
                  selectedUserId === user.id
                    ? "border-primary bg-secondary/70"
                    : "bg-background/60 hover:bg-accent/60",
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
                    id: user.id,
                    status: user.status,
                    username: user.username,
                  }}
                />
                <StatusBadge tone={user.status === "active" ? "success" : "danger"}>
                  {formatUserStatus(user.status)}
                </StatusBadge>
              </button>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
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
  user: UserIdentityData;
}) {
  const label = getUserIdentityLabel(user);

  return (
    <Tabs className="gap-4" defaultValue="overview">
      <Card className="rounded-md">
        <CardContent className="flex min-w-0 flex-col gap-4 p-6">
          <div className="flex min-w-0 flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
            <div className="flex min-w-0 items-start gap-4">
              <UserIdentityAvatar className="size-16" user={user} />
              <div className="min-w-0">
                <div className="flex min-w-0 flex-wrap items-center gap-2">
                  <h2 className="text-2xl font-semibold tracking-normal">{label.primary}</h2>
                  <StatusBadge tone={user.status === "active" ? "success" : "danger"}>
                    {formatUserStatus(user.status)}
                  </StatusBadge>
                  <StatusBadge tone={member?.console_access ? "decision" : "neutral"}>
                    控制台访问：{member?.console_access ? "允许" : "未确认"}
                  </StatusBadge>
                </div>
                <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-sm text-muted-foreground">
                  <span>{user.username ?? user.id}</span>
                  <span>{label.secondary}</span>
                  <span>用户 ID：{shortId(user.id)}</span>
                </div>
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button
                disabled={isStatusPending}
                onClick={onToggleStatus}
                type="button"
                variant={user.status === "active" ? "destructive" : "outline"}
              >
                {user.status === "active" ? <Ban data-icon="inline-start" /> : <RotateCcw data-icon="inline-start" />}
                {user.status === "active" ? "禁用账号" : "启用账号"}
              </Button>
              <Button onClick={onResetPassword} type="button" variant="outline">
                <LockKeyhole data-icon="inline-start" />
                重置密码
              </Button>
              <Button asChild variant="outline">
                <Link to="/teams">
                  <UsersRound data-icon="inline-start" />
                  去团队管理分配
                </Link>
              </Button>
            </div>
          </div>
          <Separator />
          <LiquidTabsList>
            <LiquidTabsTrigger value="overview">概览</LiquidTabsTrigger>
            <LiquidTabsTrigger value="roles">团队与角色</LiquidTabsTrigger>
            <LiquidTabsTrigger value="sessions">登录与会话</LiquidTabsTrigger>
            <LiquidTabsTrigger value="audit">审计记录</LiquidTabsTrigger>
          </LiquidTabsList>
        </CardContent>
      </Card>

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
      <TabsContent value="roles">
        <MembershipTable error={authzMembersError} member={member} />
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
    <Card className="min-w-0 rounded-md" data-testid="users-overview-basic-card">
      <CardHeader>
        <CardTitle className="text-base">基本信息</CardTitle>
        <CardDescription>来自用户列表和权限中心成员视图。</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3 text-sm">
        <InfoRow label="姓名" value={label.primary} />
        <InfoRow label="用户名" value={user.username ?? "-"} />
        <InfoRow label="邮箱/标识" value={label.secondary} />
        <InfoRow label="账号状态" value={formatUserStatus(user.status)} />
        <InfoRow label="用户 ID" value={user.id} />
      </CardContent>
    </Card>
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
    <Card className="min-w-0 rounded-md" data-testid="users-overview-permission-card">
      <CardHeader>
        <CardTitle className="text-base">权限快速校验</CardTitle>
        <CardDescription>用于检查账号是否能进入关键管理动作。</CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3 text-sm">
        <InfoRow label="actor" value={`user:${shortId(user.id)}`} />
        <InfoRow label="action" value={firstMembership?.team_id ? "team.member.add" : "console.access"} />
        <InfoRow label="resource" value={firstMembership?.team_id ? `team:${shortId(firstMembership.team_id)}` : "console:web"} />
        <div className="flex items-center justify-between rounded-md border bg-muted/30 px-3 py-2">
          <span className="text-muted-foreground">结果</span>
          <StatusBadge tone={member?.console_access ? "success" : "warning"}>
            {member?.console_access ? "允许" : "待诊断"}
          </StatusBadge>
        </div>
      </CardContent>
    </Card>
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
    <Card className="min-w-0 rounded-md">
      <CardHeader>
        <CardTitle className="text-base">所属团队 & 角色</CardTitle>
        <CardDescription>深度角色调整仍通过团队管理页完成。</CardDescription>
      </CardHeader>
      <CardContent>
        {error ? (
          <Alert variant="destructive">
            <ShieldAlert />
            <AlertTitle>成员角色加载失败</AlertTitle>
            <AlertDescription>权限中心成员视图暂不可用。</AlertDescription>
          </Alert>
        ) : memberships.length === 0 ? (
          <p className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">暂无租户或团队角色。</p>
        ) : (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>范围</TableHead>
                  <TableHead>角色</TableHead>
                  <TableHead>状态</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {memberships.map((membership) => (
                  <TableRow key={`${membership.tenant_id}-${membership.team_id ?? "tenant"}-${membership.role}`}>
                    <TableCell className="min-w-36">{formatMembershipScope(membership)}</TableCell>
                    <TableCell>
                      <Badge variant="secondary">{membership.role}</Badge>
                    </TableCell>
                    <TableCell>{membership.status}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
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
    <Card className="min-w-0 rounded-md">
      <CardHeader>
        <CardTitle className="text-base">最近登录日志</CardTitle>
        <CardDescription>按当前用户从 Web 登录日志中匹配。</CardDescription>
      </CardHeader>
      <CardContent>
        {error ? (
          <Alert variant="destructive">
            <ShieldAlert />
            <AlertTitle>登录日志加载失败</AlertTitle>
            <AlertDescription>请检查 Auth API 连接。</AlertDescription>
          </Alert>
        ) : logs.length === 0 ? (
          <p className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">暂无登录记录。</p>
        ) : (
          <div className="overflow-x-auto">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>时间</TableHead>
                  <TableHead>IP 地址</TableHead>
                  <TableHead>设备 / 浏览器</TableHead>
                  <TableHead>结果</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {logs.map((log) => (
                  <TableRow key={log.id}>
                    <TableCell className="min-w-32">{formatDateTime(log.created_at)}</TableCell>
                    <TableCell>{log.client_ip ?? "-"}</TableCell>
                    <TableCell className="min-w-40">{log.user_agent ?? "-"}</TableCell>
                    <TableCell>
                      <StatusBadge tone={log.result === "succeeded" ? "success" : "danger"}>
                        {log.result === "succeeded" ? "成功" : "失败"}
                      </StatusBadge>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </CardContent>
    </Card>
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
      <Alert variant="destructive">
        <ShieldAlert />
        <AlertTitle>审计记录加载失败</AlertTitle>
        <AlertDescription>授权拒绝记录暂不可用。</AlertDescription>
      </Alert>
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
      tone: log.result === "succeeded" ? ("success" as const) : ("danger" as const),
    })),
  ].sort((a, b) => new Date(b.at).getTime() - new Date(a.at).getTime());

  return (
    <Card className={cn("min-w-0 rounded-md", className)} data-testid="users-overview-timeline-card">
      <CardHeader>
        <CardTitle className="text-base">账号操作记录</CardTitle>
        <CardDescription>登录事件与授权拒绝事件的合并视图。</CardDescription>
      </CardHeader>
      <CardContent>
        {events.length === 0 ? (
          <p className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">暂无可展示事件。</p>
        ) : (
          <div className="flex flex-col gap-3">
            {events.map((event) => (
              <div className="flex gap-3 text-sm" key={`${event.title}-${event.at}`}>
                <span className="mt-1 size-2 rounded-full bg-current text-primary" />
                <div className="min-w-0 flex-1">
                  <div className="flex min-w-0 flex-wrap items-center gap-2">
                    <span className="truncate font-medium">{event.title}</span>
                    <StatusBadge tone={event.tone}>{event.tone === "success" ? "成功" : "需关注"}</StatusBadge>
                  </div>
                  <p className="truncate text-xs text-muted-foreground">{event.description}</p>
                  <p className="text-xs text-muted-foreground">{formatDateTime(event.at)}</p>
                </div>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
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
      <Card className="rounded-md">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ShieldCheck />
            权限诊断
          </CardTitle>
          <CardDescription>从权限中心聚合当前用户的可访问性和拒绝信号。</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 text-sm">
          <div className="rounded-md border bg-secondary/40 p-3">
            <p className="text-xs text-muted-foreground">控制台访问</p>
            <p className="mt-1 text-xl font-semibold">{allowRate}</p>
          </div>
          <InfoRow label="账号状态" value={user ? formatUserStatus(user.status) : "-"} />
          <InfoRow label="角色数量" value={String(member?.memberships.length ?? 0)} />
          <InfoRow label="最近拒绝" value={member?.recent_denied_reason ?? "暂无"} />
        </CardContent>
      </Card>

      <Card className="rounded-md">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <ShieldAlert />
            近期拒绝
          </CardTitle>
          <CardDescription>最近 8 条失败授权决策。</CardDescription>
        </CardHeader>
        <CardContent>
          {deniedDecisionsError ? (
            <p className="text-sm text-destructive">拒绝事件加载失败。</p>
          ) : deniedDecisions.length === 0 ? (
            <p className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">暂无拒绝事件。</p>
          ) : (
            <div className="flex flex-col gap-2">
              {deniedDecisions.map((decision) => (
                <div className="rounded-md border bg-background/70 p-3 text-sm" key={decision.id}>
                  <div className="flex items-center justify-between gap-3">
                    <span className="min-w-0 truncate font-medium">{decision.action}</span>
                    <StatusBadge tone="danger">拒绝</StatusBadge>
                  </div>
                  <p className="mt-1 truncate text-xs text-muted-foreground">{decision.reason ?? "-"}</p>
                  <p className="mt-1 text-xs text-muted-foreground">{formatDateTime(decision.created_at)}</p>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Card className="rounded-md">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            <Activity />
            下一步建议
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-2 text-sm">
          <Recommendation icon={<KeyRound />} text="定期轮换密码或接入 SSO 后迁移认证策略。" />
          <Recommendation icon={<UsersRound />} text="团队角色变更优先在团队管理中走审批链路。" />
          <Recommendation icon={<Clock3 />} text="导出最近 30 天登录与拒绝记录，供审计复核。" />
          <Button asChild className="mt-2" variant="outline">
            <Link to="/permissions">
              查看权限中心
              <ArrowUpRight data-icon="inline-end" />
            </Link>
          </Button>
        </CardContent>
      </Card>
    </aside>
  );
}

function Recommendation({ icon, text }: { icon: ReactNode; text: string }) {
  return (
    <div className="flex gap-2 rounded-md border bg-background/70 p-3">
      <span className="text-primary">{icon}</span>
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

function CreateUserDialog({
  draft,
  error,
  isOpen,
  isPending,
  onDraftChange,
  onOpenChange,
  onSubmit,
}: {
  draft: { avatarSeed: string; password: string; username: string };
  error: unknown;
  isOpen: boolean;
  isPending: boolean;
  onDraftChange: (draft: { avatarSeed: string; password: string; username: string }) => void;
  onOpenChange: (open: boolean) => void;
  onSubmit: () => void;
}) {
  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>新建用户</DialogTitle>
          <DialogDescription>创建平台人类用户。团队归属和高权限角色请在团队管理中审批分配。</DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            onSubmit();
          }}
        >
          <div className="flex flex-col gap-2">
            <Label htmlFor="create-username">用户名</Label>
            <Input
              id="create-username"
              onChange={(event) => onDraftChange({ ...draft, username: event.target.value })}
              placeholder="operator"
              required
              value={draft.username}
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="create-password">临时密码</Label>
            <Input
              id="create-password"
              minLength={4}
              onChange={(event) => onDraftChange({ ...draft, password: event.target.value })}
              placeholder="输入临时密码"
              required
              type="password"
              value={draft.password}
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="create-avatar-seed">头像种子</Label>
            <Input
              id="create-avatar-seed"
              onChange={(event) => onDraftChange({ ...draft, avatarSeed: event.target.value })}
              placeholder="默认使用用户名"
              value={draft.avatarSeed}
            />
          </div>
          {error instanceof Error ? <p className="text-sm text-destructive">{error.message}</p> : null}
          <DialogFooter>
            <Button onClick={() => onOpenChange(false)} type="button" variant="outline">
              取消
            </Button>
            <Button disabled={isPending || draft.username.trim().length === 0 || draft.password.length < 4} type="submit">
              创建用户
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
  tone,
  value,
}: {
  icon: ReactNode;
  label: string;
  tone: "artifact" | "danger" | "decision" | "success" | "warning";
  value: number;
}) {
  return (
    <Card className="rounded-md">
      <CardContent className="flex items-center justify-between gap-3 p-4">
        <div>
          <p className="text-xs text-muted-foreground">{label}</p>
          <p className="mt-1 text-2xl font-semibold">{value}</p>
        </div>
        <SemanticIconTile tone={tone} size="sm">
          {icon}
        </SemanticIconTile>
      </CardContent>
    </Card>
  );
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex min-w-0 items-center justify-between gap-3 rounded-md border bg-background/60 px-3 py-2">
      <span className="shrink-0 text-muted-foreground">{label}</span>
      <span className="min-w-0 truncate text-right font-medium">{value}</span>
    </div>
  );
}

function mergeUserIdentity(user: UserSummary, member?: AuthzMemberRecord): UserIdentityData {
  return {
    avatar: user.avatar,
    display_name: member?.display_name ?? undefined,
    email: member?.email ?? undefined,
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

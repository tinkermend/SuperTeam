import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Clock3, KeyRound, Save, ShieldCheck, UserRound } from "lucide-react";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import { SemanticIconTile, StatusBadge } from "@/components/superteam";
import { UserIdentityAvatar, getUserIdentityLabel } from "@/components/superteam/user-identity";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  listCurrentUserLoginLogs,
  updateCurrentUserPassword,
  updateCurrentUserProfile,
  type ApiClientOptions,
  type LoginLogRecord,
} from "@/lib/api";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { cn } from "@/lib/utils";
import { useAuth } from "@/features/auth/use-auth";

const apiBaseUrl = resolveControlPlaneUrl();

type AccountSettingsProps = {
  fetcher?: ApiClientOptions["fetcher"];
};

type ProfileDraft = {
  avatarSeed: string;
  displayName: string;
  email: string;
};

type PasswordDraft = {
  currentPassword: string;
  password: string;
};

export function AccountSettings({ fetcher }: AccountSettingsProps = {}) {
  const { refreshCurrentUser, user } = useAuth();
  const queryClient = useQueryClient();
  const apiOptions = useMemo(
    () => ({
      baseUrl: apiBaseUrl,
      fetcher,
    }),
    [fetcher],
  );
  const [profileDraft, setProfileDraft] = useState<ProfileDraft>({
    avatarSeed: "",
    displayName: "",
    email: "",
  });
  const [passwordDraft, setPasswordDraft] = useState<PasswordDraft>({
    currentPassword: "",
    password: "",
  });
  const [profileMessage, setProfileMessage] = useState("");
  const [passwordMessage, setPasswordMessage] = useState("");

  useEffect(() => {
    if (!user) {
      return;
    }
    setProfileDraft({
      avatarSeed: user.avatar.seed,
      displayName: user.display_name ?? user.username,
      email: user.email ?? "",
    });
  }, [user?.avatar.seed, user?.display_name, user?.email, user?.id, user?.username]);

  const loginLogsQuery = useQuery({
    enabled: Boolean(user),
    queryFn: () =>
      listCurrentUserLoginLogs({
        ...apiOptions,
        limit: 10,
        offset: 0,
      }),
    queryKey: ["account", "login-logs", user?.id],
  });

  const profileMutation = useMutation({
    mutationFn: (draft: ProfileDraft) =>
      updateCurrentUserProfile(apiOptions, {
        avatar: {
          provider: "dicebear",
          seed: draft.avatarSeed.trim() || user?.username || "account",
          style: "adventurer",
        },
        display_name: draft.displayName.trim(),
        email: draft.email.trim(),
      }),
    onSuccess: async () => {
      setProfileMessage("资料已保存");
      await refreshCurrentUser({ showLoading: false });
      void queryClient.invalidateQueries({ queryKey: ["account", "login-logs"] });
    },
  });

  const passwordMutation = useMutation({
    mutationFn: (draft: PasswordDraft) =>
      updateCurrentUserPassword(apiOptions, {
        current_password: draft.currentPassword,
        password: draft.password,
      }),
    onSuccess: () => {
      setPasswordDraft({ currentPassword: "", password: "" });
      setPasswordMessage("密码已更新");
    },
  });

  if (!user) {
    return (
      <>
        <Header>
          <Search />
          <ThemeSwitch />
        </Header>
        <Main className="min-w-0 overflow-x-hidden" fluid>
          <Card className="rounded-md">
            <CardContent className="p-6 text-sm text-muted-foreground">未登录</CardContent>
          </Card>
        </Main>
      </>
    );
  }

  const identity = getUserIdentityLabel(user);
  const loginLogs = loginLogsQuery.data?.items ?? [];

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
              <UserRound />
            </SemanticIconTile>
            <div className="min-w-0">
              <h1 className="text-2xl font-bold tracking-normal">账户设置</h1>
              <div className="mt-1 flex min-w-0 flex-wrap items-center gap-2 text-sm text-muted-foreground">
                <span className="truncate">{identity.primary}</span>
                <span aria-hidden="true">/</span>
                <span className="truncate">{identity.secondary}</span>
              </div>
            </div>
          </div>
          <StatusBadge tone={user.status === "active" ? "success" : "danger"}>{user.status}</StatusBadge>
        </div>

        <div className="grid gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(360px,0.85fr)]">
          <Card className="min-w-0 rounded-md">
            <CardHeader className="flex flex-row items-center gap-3">
              <UserIdentityAvatar className="size-14" user={user} />
              <div className="min-w-0">
                <CardTitle className="text-base">个人资料</CardTitle>
                <p className="truncate text-sm text-muted-foreground">{user.username}</p>
              </div>
            </CardHeader>
            <CardContent>
              <form
                className="grid gap-4 md:grid-cols-2"
                onSubmit={(event) => {
                  event.preventDefault();
                  setProfileMessage("");
                  profileMutation.mutate(profileDraft);
                }}
              >
                <Field label="展示名称">
                  <Input
                    aria-label="展示名称"
                    value={profileDraft.displayName}
                    onChange={(event) => setProfileDraft((draft) => ({ ...draft, displayName: event.target.value }))}
                  />
                </Field>
                <Field label="邮箱">
                  <Input
                    aria-label="邮箱"
                    type="email"
                    value={profileDraft.email}
                    onChange={(event) => setProfileDraft((draft) => ({ ...draft, email: event.target.value }))}
                  />
                </Field>
                <Field className="md:col-span-2" label="头像 Seed">
                  <Input
                    aria-label="头像 Seed"
                    value={profileDraft.avatarSeed}
                    onChange={(event) => setProfileDraft((draft) => ({ ...draft, avatarSeed: event.target.value }))}
                  />
                </Field>
                <div className="flex items-center gap-3 md:col-span-2">
                  <Button disabled={profileMutation.isPending} type="submit">
                    <Save data-icon="inline-start" />
                    保存资料
                  </Button>
                  <MutationMessage
                    error={profileMutation.error}
                    message={profileMessage}
                  />
                </div>
              </form>
            </CardContent>
          </Card>

          <Card className="min-w-0 rounded-md">
            <CardHeader className="flex flex-row items-center gap-3">
              <SemanticIconTile tone="decision">
                <KeyRound />
              </SemanticIconTile>
              <CardTitle className="text-base">密码</CardTitle>
            </CardHeader>
            <CardContent>
              <form
                className="grid gap-4"
                onSubmit={(event) => {
                  event.preventDefault();
                  setPasswordMessage("");
                  passwordMutation.mutate(passwordDraft);
                }}
              >
                <Field label="当前密码">
                  <Input
                    aria-label="当前密码"
                    type="password"
                    value={passwordDraft.currentPassword}
                    onChange={(event) => setPasswordDraft((draft) => ({ ...draft, currentPassword: event.target.value }))}
                  />
                </Field>
                <Field label="新密码">
                  <Input
                    aria-label="新密码"
                    type="password"
                    value={passwordDraft.password}
                    onChange={(event) => setPasswordDraft((draft) => ({ ...draft, password: event.target.value }))}
                  />
                </Field>
                <div className="flex items-center gap-3">
                  <Button disabled={passwordMutation.isPending} type="submit">
                    <ShieldCheck data-icon="inline-start" />
                    修改密码
                  </Button>
                  <MutationMessage
                    error={passwordMutation.error}
                    message={passwordMessage}
                  />
                </div>
              </form>
            </CardContent>
          </Card>
        </div>

        <Card className="mt-4 rounded-md">
          <CardHeader className="flex flex-row items-center gap-3">
            <SemanticIconTile tone="artifact">
              <Clock3 />
            </SemanticIconTile>
            <CardTitle className="text-base">最近登录</CardTitle>
          </CardHeader>
          <CardContent>
            {loginLogsQuery.isError ? (
              <Alert variant="destructive">
                <AlertDescription>登录记录加载失败</AlertDescription>
              </Alert>
            ) : loginLogs.length === 0 ? (
              <div className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
                {loginLogsQuery.isLoading ? "加载登录记录中" : "暂无登录记录"}
              </div>
            ) : (
              <div className="divide-y rounded-md border">
                {loginLogs.map((record) => (
                  <LoginLogRow key={record.id} record={record} />
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </Main>
    </>
  );
}

function Field({
  children,
  className,
  label,
}: {
  children: ReactNode;
  className?: string;
  label: string;
}) {
  return (
    <div className={cn("grid gap-2", className)}>
      <Label>{label}</Label>
      {children}
    </div>
  );
}

function MutationMessage({ error, message }: { error: Error | null; message: string }) {
  if (error) {
    return <span className="text-sm text-destructive">{error.message}</span>;
  }
  if (message) {
    return <span className="text-sm text-[color:var(--superteam-success)]">{message}</span>;
  }
  return null;
}

function LoginLogRow({ record }: { record: LoginLogRecord }) {
  return (
    <div className="grid gap-2 p-4 text-sm sm:grid-cols-[minmax(0,1fr)_180px_180px] sm:items-center">
      <div className="min-w-0">
        <div className="font-medium">{formatLoginEvent(record.event_type)}</div>
        <div className="truncate text-muted-foreground">{record.user_agent ?? "未知设备"}</div>
      </div>
      <div className="text-muted-foreground">{record.client_ip ?? "未知 IP"}</div>
      <time className="text-muted-foreground" dateTime={record.created_at}>
        {formatDateTime(record.created_at)}
      </time>
    </div>
  );
}

function formatLoginEvent(eventType: LoginLogRecord["event_type"]) {
  if (eventType === "login_succeeded") {
    return "登录成功";
  }
  if (eventType === "logout_succeeded") {
    return "退出登录";
  }
  return "登录失败";
}

function formatDateTime(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    month: "2-digit",
    year: "numeric",
  }).format(new Date(value));
}

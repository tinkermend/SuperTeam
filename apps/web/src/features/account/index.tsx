import { useEffect, useMemo, useState, type ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Clock3, KeyRound, Save, ShieldCheck, UserRound } from "lucide-react";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import {
  IconTile,
  SoftCard,
  StatusPill,
  V3Button,
  V3EmptyState,
  V3ErrorState,
  V3LoadingState,
  V3PageHeader,
  V3Table,
  V3Td,
  V3Th,
  V3Tr,
  WorkSurface,
} from "@/components/superteam";
import { UserIdentityAvatar, getUserIdentityLabel } from "@/components/superteam/user-identity";
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
          <SoftCard className="p-6 text-sm text-v3-ink-2">未登录</SoftCard>
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
        <V3PageHeader
          icon={<UserRound />}
          iconTone="mute"
          title="账户设置"
          subtitle={
            <span className="inline-flex flex-wrap items-center gap-2">
              <span className="truncate">{identity.primary}</span>
              <span aria-hidden="true">/</span>
              <span className="truncate">{identity.secondary}</span>
            </span>
          }
          actions={
            <StatusPill tone={user.status === "active" ? "ok" : "danger"}>{user.status}</StatusPill>
          }
        />
        <div className="grid gap-4 xl:grid-cols-[minmax(0,1.15fr)_minmax(360px,0.85fr)]">
          <SoftCard className="min-w-0 p-6">
            <div className="mb-5 flex items-center gap-3">
              <UserIdentityAvatar className="size-14" user={user} />
              <div className="min-w-0">
                <h2 className="text-base font-bold text-v3-ink">个人资料</h2>
                <p className="truncate text-sm text-v3-ink-2">{user.username}</p>
              </div>
            </div>
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
                  <V3Button disabled={profileMutation.isPending} type="submit">
                    <Save data-icon="inline-start" />
                    保存资料
                  </V3Button>
                  <MutationMessage
                    error={profileMutation.error}
                    message={profileMessage}
                  />
                </div>
              </form>
          </SoftCard>

          <SoftCard className="min-w-0 p-6">
            <div className="mb-5 flex items-center gap-3">
              <IconTile tone="brand">
                <KeyRound />
              </IconTile>
              <h2 className="text-base font-bold text-v3-ink">密码</h2>
            </div>
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
                  <V3Button disabled={passwordMutation.isPending} type="submit">
                    <ShieldCheck data-icon="inline-start" />
                    修改密码
                  </V3Button>
                  <MutationMessage
                    error={passwordMutation.error}
                    message={passwordMessage}
                  />
                </div>
              </form>
          </SoftCard>
        </div>

        <SoftCard className="mt-4 min-w-0 p-6">
          <div className="mb-5 flex items-center gap-3">
            <IconTile tone="artifact">
              <Clock3 />
            </IconTile>
            <h2 className="text-base font-bold text-v3-ink">最近登录</h2>
          </div>
          <WorkSurface>
            {loginLogsQuery.isError ? (
              <V3ErrorState title="登录记录加载失败" />
            ) : loginLogsQuery.isLoading ? (
              <V3LoadingState label="加载登录记录中" />
            ) : loginLogs.length === 0 ? (
              <V3EmptyState title="暂无登录记录" />
            ) : (
              <V3Table>
                <thead>
                  <V3Tr>
                    <V3Th>事件</V3Th>
                    <V3Th>IP 地址</V3Th>
                    <V3Th>时间</V3Th>
                  </V3Tr>
                </thead>
                <tbody>
                {loginLogs.map((record) => (
                  <LoginLogRow key={record.id} record={record} />
                ))}
                </tbody>
              </V3Table>
            )}
          </WorkSurface>
        </SoftCard>
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
    return <span className="text-sm text-v3-ok">{message}</span>;
  }
  return null;
}

function LoginLogRow({ record }: { record: LoginLogRecord }) {
  return (
    <V3Tr tone={record.result === "failed" ? "danger" : undefined}>
      <V3Td className="min-w-56">
        <div className="font-medium text-v3-ink">{formatLoginEvent(record.event_type)}</div>
        <div className="truncate text-v3-ink-2">{record.user_agent ?? "未知设备"}</div>
      </V3Td>
      <V3Td className="text-v3-ink-2">{record.client_ip ?? "未知 IP"}</V3Td>
      <V3Td className="text-v3-ink-2 tabular-nums">
        <time dateTime={record.created_at}>
        {formatDateTime(record.created_at)}
      </time>
      </V3Td>
    </V3Tr>
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

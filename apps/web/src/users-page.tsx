"use client";

import type { ApiClientOptions, UserSummary } from "@superteam/api-client";
import { createUser, listUsers, resetUserPassword, updateUserStatus } from "@superteam/api-client";
import { StatusPill } from "@superteam/ui";
import { PlatformEmptyState, PlatformErrorState, PlatformLoadingState } from "@superteam/views";
import { KeyRound, Plus, RefreshCw, ShieldCheck, ShieldOff } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { ConsoleAppShell } from "@/console-app-shell";
import { resolveControlPlaneUrl } from "@/control-plane-url";

type UsersPageProps = {
  apiBaseUrl?: string;
  fetcher?: ApiClientOptions["fetcher"];
};

const userStatusLabels: Record<UserSummary["status"], string> = {
  active: "已启用",
  disabled: "已禁用",
};

export function UsersPage({ apiBaseUrl, fetcher }: UsersPageProps) {
  const [users, setUsers] = useState<UserSummary[]>([]);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [resetPasswords, setResetPasswords] = useState<Record<number, string>>({});
  const [error, setError] = useState<string | null>(null);
  const [listError, setListError] = useState<string | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const resolvedApiBaseUrl = apiBaseUrl ?? resolveControlPlaneUrl();
  const options = useMemo(() => ({ baseUrl: resolvedApiBaseUrl, fetcher }), [fetcher, resolvedApiBaseUrl]);

  async function loadUsers() {
    setIsLoading(true);
    setListError(null);
    try {
      const response = await listUsers({ ...options, limit: 100, offset: 0 });
      setUsers(response.items);
    } catch {
      setListError("用户列表加载失败");
    } finally {
      setIsLoading(false);
    }
  }

  useEffect(() => {
    void loadUsers();
  }, [options]);

  async function handleCreateUser(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!username || !password || isSubmitting) {
      return;
    }
    setIsSubmitting(true);
    setError(null);
    try {
      await createUser(options, { password, username });
      setUsername("");
      setPassword("");
      await loadUsers();
    } catch {
      setError("用户创建失败");
    } finally {
      setIsSubmitting(false);
    }
  }

  async function handleToggleStatus(target: UserSummary) {
    const nextStatus = target.status === "active" ? "disabled" : "active";
    setError(null);
    try {
      const response = await updateUserStatus(options, target.id, nextStatus);
      setUsers((items) => items.map((item) => (item.id === target.id ? response.user : item)));
    } catch {
      setError("用户状态更新失败");
    }
  }

  async function handleResetPassword(target: UserSummary) {
    const nextPassword = resetPasswords[target.id];
    if (!nextPassword) {
      return;
    }
    setError(null);
    try {
      await resetUserPassword(options, target.id, nextPassword);
      setResetPasswords((current) => ({ ...current, [target.id]: "" }));
    } catch {
      setError("密码重置失败");
    }
  }

  return (
    <ConsoleAppShell
      pageDescription="管理 Web 控制台平台账号。角色与细粒度权限后续接入统一授权接口，本页先完成用户生命周期最小闭环。"
      pageTitle="用户管理"
    >
      <section className="grid gap-4 rounded-md border bg-card p-4 text-card-foreground shadow-sm">
        <div className="flex min-w-0 flex-col gap-1">
          <h2 className="text-base font-semibold">创建用户</h2>
          <p className="text-sm text-muted-foreground">新账号默认启用，后续可在列表中禁用或重置密码。</p>
        </div>
        <form className="grid gap-3 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto]" onSubmit={handleCreateUser}>
          <label className="flex flex-col gap-2 text-sm font-medium">
            新用户账号
            <input
              className="h-9 rounded-md border bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
              onChange={(event) => setUsername(event.target.value)}
              value={username}
            />
          </label>
          <label className="flex flex-col gap-2 text-sm font-medium">
            初始密码
            <input
              className="h-9 rounded-md border bg-background px-3 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
              onChange={(event) => setPassword(event.target.value)}
              type="password"
              value={password}
            />
          </label>
          <div className="flex items-end">
            <Button disabled={isSubmitting || !username || !password} type="submit">
              <Plus data-icon="inline-start" aria-hidden="true" />
              创建用户
            </Button>
          </div>
        </form>
        {error ? <p className="text-sm text-destructive">{error}</p> : null}
      </section>

      <section className="overflow-hidden rounded-md border bg-card text-card-foreground shadow-sm">
        <header className="flex min-w-0 items-center justify-between gap-3 border-b px-4 py-3">
          <div className="min-w-0">
            <h2 className="truncate text-base font-semibold">用户列表</h2>
            <p className="text-sm text-muted-foreground">{isLoading ? "正在加载..." : `共 ${users.length} 个用户`}</p>
          </div>
          <Button onClick={() => void loadUsers()} type="button" variant="outline">
            <RefreshCw data-icon="inline-start" aria-hidden="true" />
            刷新
          </Button>
        </header>
        <div className="overflow-x-auto">
          {isLoading ? (
            <PlatformLoadingState description="正在从 Control Plane 拉取平台账号。" title="正在加载用户列表" />
          ) : listError ? (
            <PlatformErrorState
              action={
                <Button onClick={() => void loadUsers()} type="button" variant="outline">
                  <RefreshCw data-icon="inline-start" aria-hidden="true" />
                  重新加载
                </Button>
              }
              description="请检查 Control Plane 服务或网络连接后重试。"
              title={listError}
            />
          ) : users.length === 0 ? (
            <PlatformEmptyState description="当前租户还没有可管理的平台账号。" title="暂无用户" />
          ) : (
            <table className="w-full min-w-[760px] border-collapse text-sm">
              <thead className="bg-muted/40 text-left text-muted-foreground">
                <tr>
                  <th className="px-4 py-3 font-medium">账号</th>
                  <th className="px-4 py-3 font-medium">状态</th>
                  <th className="px-4 py-3 font-medium">重置密码</th>
                  <th className="px-4 py-3 text-right font-medium">操作</th>
                </tr>
              </thead>
              <tbody>
                {users.map((item) => {
                  const isActive = item.status === "active";
                  return (
                    <tr className="border-t" key={item.id}>
                      <td className="px-4 py-3 font-medium">{item.username}</td>
                      <td className="px-4 py-3">
                        <StatusPill tone={isActive ? "success" : "warning"}>{userStatusLabels[item.status]}</StatusPill>
                      </td>
                      <td className="px-4 py-3">
                        <label className="sr-only" htmlFor={`reset-password-${item.id}`}>
                          {item.username} 的新密码
                        </label>
                        <input
                          className="h-8 w-full rounded-md border bg-background px-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
                          id={`reset-password-${item.id}`}
                          onChange={(event) => setResetPasswords((current) => ({ ...current, [item.id]: event.target.value }))}
                          type="password"
                          value={resetPasswords[item.id] ?? ""}
                        />
                      </td>
                      <td className="px-4 py-3">
                        <div className="flex justify-end gap-2">
                          <Button onClick={() => void handleToggleStatus(item)} size="sm" type="button" variant="outline">
                            {isActive ? (
                              <ShieldOff data-icon="inline-start" aria-hidden="true" />
                            ) : (
                              <ShieldCheck data-icon="inline-start" aria-hidden="true" />
                            )}
                            {isActive ? `禁用 ${item.username}` : `启用 ${item.username}`}
                          </Button>
                          <Button
                            disabled={!resetPasswords[item.id]}
                            onClick={() => void handleResetPassword(item)}
                            size="sm"
                            type="button"
                            variant="outline"
                          >
                            <KeyRound data-icon="inline-start" aria-hidden="true" />
                            重置 {item.username} 密码
                          </Button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </div>
      </section>
    </ConsoleAppShell>
  );
}

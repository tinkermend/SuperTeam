"use client";

import type { FormEvent } from "react";
import { useState } from "react";
import { LockKeyhole, Sparkles, UserRound } from "lucide-react";

type LoginPageProps = {
  error: string | null;
  isPending: boolean;
  onSubmit: (credentials: { password: string; username: string }) => Promise<void> | void;
};

export function LoginPage({ error, isPending, onSubmit }: LoginPageProps) {
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("admin");

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (isPending) {
      return;
    }
    void onSubmit({ password, username });
  }

  return (
    <main className="dark flex min-h-screen items-center justify-center bg-background px-4 py-8 text-foreground sm:px-6">
      <section className="grid w-full max-w-5xl overflow-hidden rounded-md border bg-card text-card-foreground shadow-sm lg:grid-cols-[minmax(0,1fr)_420px]">
        <div className="hidden min-h-[540px] flex-col justify-between bg-sidebar p-10 text-sidebar-foreground lg:flex">
          <div className="flex items-center gap-3">
            <div className="flex size-10 items-center justify-center rounded-md bg-sidebar-primary text-sidebar-primary-foreground">
              <Sparkles aria-hidden="true" />
            </div>
            <div className="min-w-0">
              <p className="truncate text-sm font-semibold">SuperTeam</p>
              <p className="truncate text-xs text-muted-foreground">企业级数字员工控制平面</p>
            </div>
          </div>
          <div className="max-w-md">
            <h1 className="text-3xl font-semibold">账号登录</h1>
            <p className="mt-4 text-sm leading-6 text-muted-foreground">
              进入控制台后可管理任务输入、人工确认、Runtime Agent 执行、工件回传和验收闭环。
            </p>
          </div>
          <p className="text-xs text-muted-foreground">默认开发账号：admin / admin</p>
        </div>

        <form aria-label="登录表单" className="flex min-h-[540px] flex-col justify-center gap-6 p-6 sm:p-10" onSubmit={handleSubmit}>
          <div>
            <div className="mb-5 flex size-11 items-center justify-center rounded-md bg-secondary text-secondary-foreground lg:hidden">
              <Sparkles aria-hidden="true" />
            </div>
            <h2 className="text-2xl font-semibold">账号登录</h2>
            <p className="mt-2 text-sm text-muted-foreground">使用平台账号访问 SuperTeam 控制台。</p>
          </div>

          <div className="flex flex-col gap-4">
            <label className="flex flex-col gap-2 text-sm font-medium">
              账号
              <span className="relative">
                <UserRound className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
                <input
                  aria-invalid={Boolean(error)}
                  autoComplete="username"
                  className="h-11 w-full rounded-md border bg-background px-10 text-sm outline-none transition-colors focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
                  name="username"
                  onChange={(event) => setUsername(event.target.value)}
                  value={username}
                />
              </span>
            </label>
            <label className="flex flex-col gap-2 text-sm font-medium">
              密码
              <span className="relative">
                <LockKeyhole className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-muted-foreground" aria-hidden="true" />
                <input
                  aria-invalid={Boolean(error)}
                  autoComplete="current-password"
                  className="h-11 w-full rounded-md border bg-background px-10 text-sm outline-none transition-colors focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50"
                  name="password"
                  onChange={(event) => setPassword(event.target.value)}
                  type="password"
                  value={password}
                />
              </span>
            </label>
          </div>

          {error ? (
            <div role="alert" className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {error}
            </div>
          ) : null}

          <button
            className="inline-flex h-11 items-center justify-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground transition-opacity disabled:cursor-not-allowed disabled:opacity-60"
            disabled={isPending}
            type="submit"
          >
            {isPending ? "登录中..." : "登录"}
          </button>
        </form>
      </section>
    </main>
  );
}

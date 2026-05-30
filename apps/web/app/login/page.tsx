"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@superteam/core/auth";
import { LoginPage } from "@superteam/views";

export default function Page() {
  const router = useRouter();
  const { login } = useAuth();
  const [error, setError] = useState<string | null>(null);
  const [isPending, setIsPending] = useState(false);

  async function handleSubmit(credentials: { password: string; username: string }) {
    setIsPending(true);
    setError(null);
    try {
      await login(credentials);
      router.replace("/");
    } catch {
      setError("用户名或密码错误");
    } finally {
      setIsPending(false);
    }
  }

  return <LoginPage error={error} isPending={isPending} onSubmit={handleSubmit} />;
}

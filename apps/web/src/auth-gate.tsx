"use client";

import { useEffect } from "react";
import { usePathname, useRouter } from "next/navigation";
import { useAuth } from "@superteam/core/auth";

const publicPlaceholderPathnames = new Set([
  "/",
  "/workspace",
  "/tasks",
  "/employees",
  "/workflows",
  "/capabilities",
  "/approvals",
  "/audit",
]);

export function AuthGate({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isLoading } = useAuth();
  const pathname = usePathname();
  const router = useRouter();
  const isLoginPage = pathname === "/login";
  const isPublicPlaceholderPage = publicPlaceholderPathnames.has(pathname);
  const requiresAuthentication = !isLoginPage && !isPublicPlaceholderPage;

  useEffect(() => {
    if (!isLoading && !isAuthenticated && requiresAuthentication) {
      router.replace("/login");
    }
    if (!isLoading && isAuthenticated && isLoginPage) {
      router.replace("/");
    }
  }, [isAuthenticated, isLoading, isLoginPage, requiresAuthentication, router]);

  if (isLoading) {
    return <div className="dark flex min-h-screen items-center justify-center bg-background text-sm text-muted-foreground">加载中...</div>;
  }

  if (!isAuthenticated && requiresAuthentication) {
    return null;
  }

  return <>{children}</>;
}

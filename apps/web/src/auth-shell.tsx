"use client";

import { AuthProvider } from "@superteam/core/auth";

export function AuthShell({ children }: { children: React.ReactNode }) {
  return <AuthProvider apiBaseUrl={process.env.NEXT_PUBLIC_CONTROL_PLANE_URL ?? "http://localhost:8080"}>{children}</AuthProvider>;
}

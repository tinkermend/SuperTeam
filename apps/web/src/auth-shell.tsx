"use client";

import { AuthProvider } from "@superteam/core/auth";

import { resolveControlPlaneUrl } from "./control-plane-url";

export function AuthShell({ children }: { children: React.ReactNode }) {
  return <AuthProvider apiBaseUrl={resolveControlPlaneUrl()}>{children}</AuthProvider>;
}

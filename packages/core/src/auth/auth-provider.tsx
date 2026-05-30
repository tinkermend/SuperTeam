"use client";

import type { ReactNode } from "react";
import { useCallback, useEffect, useMemo, useState } from "react";
import type { UserSummary } from "@superteam/api-client";
import { getCurrentUser, login as loginRequest, logout as logoutRequest } from "@superteam/api-client";

import { AuthContext } from "./auth-context";

type AuthProviderProps = {
  apiBaseUrl: string;
  children: ReactNode;
};

export function AuthProvider({ apiBaseUrl, children }: AuthProviderProps) {
  const [user, setUser] = useState<UserSummary | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    let isMounted = true;

    async function loadCurrentUser() {
      try {
        const response = await getCurrentUser({ baseUrl: apiBaseUrl });
        if (isMounted) {
          setUser(response.user);
        }
      } catch {
        if (isMounted) {
          setUser(null);
        }
      } finally {
        if (isMounted) {
          setIsLoading(false);
        }
      }
    }

    void loadCurrentUser();

    return () => {
      isMounted = false;
    };
  }, [apiBaseUrl]);

  const login = useCallback(
    async (credentials: { password: string; username: string }) => {
      const response = await loginRequest({ baseUrl: apiBaseUrl }, credentials);
      setUser(response.user);
    },
    [apiBaseUrl],
  );

  const logout = useCallback(async () => {
    await logoutRequest({ baseUrl: apiBaseUrl });
    setUser(null);
  }, [apiBaseUrl]);

  const value = useMemo(
    () => ({
      isAuthenticated: Boolean(user),
      isLoading,
      login,
      logout,
      user,
    }),
    [isLoading, login, logout, user],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

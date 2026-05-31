"use client";

import type { ReactNode } from "react";
import { useCallback, useEffect, useMemo, useState } from "react";
import type { ApiClientOptions, UserSummary } from "@superteam/api-client";
import { ApiRequestError, getCurrentUser, login as loginRequest, logout as logoutRequest } from "@superteam/api-client";

import { AuthContext } from "./auth-context";

type AuthProviderProps = {
  apiBaseUrl: string;
  children: ReactNode;
  fetcher?: ApiClientOptions["fetcher"];
};

export function AuthProvider({ apiBaseUrl, children, fetcher }: AuthProviderProps) {
  const [user, setUser] = useState<UserSummary | null>(null);
  const [isLoading, setIsLoading] = useState(true);

  const loadCurrentUser = useCallback(
    async (options?: { showLoading?: boolean }) => {
      if (options?.showLoading ?? true) {
        setIsLoading(true);
      }
      try {
        const response = await getCurrentUser({ baseUrl: apiBaseUrl, fetcher });
        setUser(response.user);
      } catch (error) {
        if (error instanceof ApiRequestError && error.status === 401) {
          setUser(null);
        }
      } finally {
        if (options?.showLoading ?? true) {
          setIsLoading(false);
        }
      }
    },
    [apiBaseUrl, fetcher],
  );

  useEffect(() => {
    let isMounted = true;

    async function loadInitialUser() {
      try {
        const response = await getCurrentUser({ baseUrl: apiBaseUrl, fetcher });
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

    void loadInitialUser();

    return () => {
      isMounted = false;
    };
  }, [apiBaseUrl, fetcher]);

  useEffect(() => {
    function refreshSession() {
      void loadCurrentUser({ showLoading: false });
    }

    window.addEventListener("focus", refreshSession);
    return () => window.removeEventListener("focus", refreshSession);
  }, [loadCurrentUser]);

  const login = useCallback(
    async (credentials: { password: string; username: string }) => {
      const response = await loginRequest({ baseUrl: apiBaseUrl, fetcher }, credentials);
      setUser(response.user);
    },
    [apiBaseUrl, fetcher],
  );

  const logout = useCallback(async () => {
    await logoutRequest({ baseUrl: apiBaseUrl, fetcher });
    setUser(null);
  }, [apiBaseUrl, fetcher]);

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

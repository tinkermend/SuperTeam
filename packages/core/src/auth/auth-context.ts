import { createContext } from "react";
import type { UserSummary } from "@superteam/api-client";

export type AuthContextValue = {
  isAuthenticated: boolean;
  isLoading: boolean;
  login: (credentials: { password: string; username: string }) => Promise<void>;
  logout: () => Promise<void>;
  user: UserSummary | null;
};

export const AuthContext = createContext<AuthContextValue | null>(null);

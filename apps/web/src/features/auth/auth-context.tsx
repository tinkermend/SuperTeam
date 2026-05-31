import { createContext } from 'react'
import type { UserSummary } from '@/lib/api'

export type AuthContextValue = {
  isAuthenticated: boolean
  isLoading: boolean
  login: (credentials: { password: string; username: string }) => Promise<void>
  logout: () => Promise<void>
  refreshCurrentUser: (options?: { showLoading?: boolean }) => Promise<void>
  user: UserSummary | null
}

export const AuthContext = createContext<AuthContextValue | null>(null)

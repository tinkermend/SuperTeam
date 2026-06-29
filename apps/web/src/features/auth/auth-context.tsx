import { createContext } from 'react'
import type { UserSummary } from '@/lib/api'

export type LoginCredentials = {
  captcha_code?: string
  captcha_id?: string
  password: string
  username: string
}

export type AuthContextValue = {
  apiBaseUrl: string
  isAuthenticated: boolean
  isLoading: boolean
  login: (credentials: LoginCredentials) => Promise<void>
  logout: () => Promise<void>
  refreshCurrentUser: (options?: { showLoading?: boolean }) => Promise<void>
  user: UserSummary | null
}

export const AuthContext = createContext<AuthContextValue | null>(null)

import {
  useCallback,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'
import {
  ApiRequestError,
  getCurrentUser,
  login as loginRequest,
  logout as logoutRequest,
  type ApiClientOptions,
  type UserSummary,
} from '@/lib/api'
import { AuthContext } from './auth-context'

type AuthProviderProps = {
  apiBaseUrl: string
  children: ReactNode
  fetcher?: ApiClientOptions['fetcher']
}

export function AuthProvider({
  apiBaseUrl,
  children,
  fetcher,
}: AuthProviderProps) {
  const [user, setUser] = useState<UserSummary | null>(null)
  const [isLoading, setIsLoading] = useState(true)

  const refreshCurrentUser = useCallback(
    async (options?: { showLoading?: boolean }) => {
      const showLoading = options?.showLoading ?? true

      if (showLoading) {
        setIsLoading(true)
      }

      try {
        const response = await getCurrentUser({ baseUrl: apiBaseUrl, fetcher })
        setUser(response.user)
      } catch (error) {
        if (error instanceof ApiRequestError && error.status === 401) {
          setUser(null)
          return
        }

        throw error
      } finally {
        if (showLoading) {
          setIsLoading(false)
        }
      }
    },
    [apiBaseUrl, fetcher]
  )

  const login = useCallback(
    async (credentials: { password: string; username: string }) => {
      const response = await loginRequest(
        { baseUrl: apiBaseUrl, fetcher },
        credentials
      )
      setUser(response.user)
    },
    [apiBaseUrl, fetcher]
  )

  const logout = useCallback(async () => {
    try {
      await logoutRequest({ baseUrl: apiBaseUrl, fetcher })
    } finally {
      setUser(null)
    }
  }, [apiBaseUrl, fetcher])

  useEffect(() => {
    let isMounted = true

    async function loadCurrentUser() {
      setIsLoading(true)
      try {
        const response = await getCurrentUser({ baseUrl: apiBaseUrl, fetcher })
        if (isMounted) {
          setUser(response.user)
        }
      } catch {
        if (isMounted) {
          setUser(null)
        }
      } finally {
        if (isMounted) {
          setIsLoading(false)
        }
      }
    }

    void loadCurrentUser()

    return () => {
      isMounted = false
    }
  }, [apiBaseUrl, fetcher])

  useEffect(() => {
    function handleFocus() {
      void refreshCurrentUser({ showLoading: false })
    }

    window.addEventListener('focus', handleFocus)
    return () => window.removeEventListener('focus', handleFocus)
  }, [refreshCurrentUser])

  const value = useMemo(
    () => ({
      isAuthenticated: Boolean(user),
      isLoading,
      login,
      logout,
      refreshCurrentUser,
      user,
    }),
    [isLoading, login, logout, refreshCurrentUser, user]
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

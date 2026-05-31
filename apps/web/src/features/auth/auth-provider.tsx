import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
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
  const requestSequenceRef = useRef(0)

  const startAuthRequest = useCallback(() => {
    requestSequenceRef.current += 1
    return requestSequenceRef.current
  }, [])

  const isCurrentRequest = useCallback((requestId: number) => {
    return requestSequenceRef.current === requestId
  }, [])

  const refreshCurrentUser = useCallback(
    async (options?: { showLoading?: boolean }) => {
      const showLoading = options?.showLoading ?? true
      const requestId = startAuthRequest()

      if (showLoading) {
        setIsLoading(true)
      }

      try {
        const response = await getCurrentUser({ baseUrl: apiBaseUrl, fetcher })
        if (isCurrentRequest(requestId)) {
          setUser(response.user)
        }
      } catch (error) {
        if (error instanceof ApiRequestError && error.status === 401) {
          if (isCurrentRequest(requestId)) {
            setUser(null)
          }
          return
        }

        throw error
      } finally {
        if (isCurrentRequest(requestId)) {
          setIsLoading(false)
        }
      }
    },
    [apiBaseUrl, fetcher, isCurrentRequest, startAuthRequest]
  )

  const login = useCallback(
    async (credentials: { password: string; username: string }) => {
      const requestId = startAuthRequest()
      const response = await loginRequest(
        { baseUrl: apiBaseUrl, fetcher },
        credentials
      )
      if (isCurrentRequest(requestId)) {
        setUser(response.user)
        setIsLoading(false)
      }
    },
    [apiBaseUrl, fetcher, isCurrentRequest, startAuthRequest]
  )

  const logout = useCallback(async () => {
    const requestId = startAuthRequest()
    try {
      await logoutRequest({ baseUrl: apiBaseUrl, fetcher })
    } finally {
      if (isCurrentRequest(requestId)) {
        setUser(null)
        setIsLoading(false)
      }
    }
  }, [apiBaseUrl, fetcher, isCurrentRequest, startAuthRequest])

  useEffect(() => {
    let isMounted = true
    const requestId = startAuthRequest()

    async function loadCurrentUser() {
      setIsLoading(true)
      try {
        const response = await getCurrentUser({ baseUrl: apiBaseUrl, fetcher })
        if (isMounted && isCurrentRequest(requestId)) {
          setUser(response.user)
        }
      } catch {
        if (isMounted && isCurrentRequest(requestId)) {
          setUser(null)
        }
      } finally {
        if (isMounted && isCurrentRequest(requestId)) {
          setIsLoading(false)
        }
      }
    }

    void loadCurrentUser()

    return () => {
      isMounted = false
    }
  }, [apiBaseUrl, fetcher, isCurrentRequest, startAuthRequest])

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

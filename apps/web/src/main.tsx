import { StrictMode } from 'react'
import ReactDOM from 'react-dom/client'
import {
  QueryCache,
  QueryClient,
  QueryClientProvider
} from '@tanstack/react-query'
import { RouterProvider, createRouter } from '@tanstack/react-router'
import { toast } from 'sonner'
import { AuthProvider } from '@/features/auth/auth-provider'
import { ApiRequestError } from '@/lib/api'
import { redirectToCanonicalLocalDevHost } from '@/lib/config/canonical-local-dev-host'
import { resolveControlPlaneUrl } from '@/lib/config/control-plane-url'
import { shouldRetryQuery } from '@/query-client'
import { DirectionProvider } from './context/direction-provider'
import { FontProvider } from './context/font-provider'
import { ThemeProvider } from './context/theme-provider'
// Generated Routes
import { routeTree } from './routeTree.gen'
// Fonts
import '@fontsource-variable/inter'
import '@fontsource-variable/manrope'
// Styles
import './styles/index.css'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: shouldRetryQuery,
      refetchOnWindowFocus: import.meta.env.PROD,
      staleTime: 10 * 1000, // 10s
    },
    mutations: {
      onError: (error) => {
        if (error instanceof ApiRequestError) {
          toast.error(error.message)
          return
        }

        toast.error('Something went wrong!')
      }
}
},
  queryCache: new QueryCache({
    onError: (error) => {
      if (error instanceof ApiRequestError && error.status === 401) {
        const redirect = router.history.location.href
        void router.navigate({ to: '/login', search: { redirect } })
      }
      if (
        error instanceof ApiRequestError &&
        error.status === 500 &&
        import.meta.env.PROD
      ) {
        void router.navigate({ to: '/500' })
      }
    }
})
})

// Create a new router instance
const router = createRouter({
  routeTree,
  context: { queryClient },
  defaultPreload: 'intent',
  defaultPreloadStaleTime: 0
})

// Register the router instance for type safety
declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

// 本地开发强制 127.0.0.1，避免与 localhost 双 cookie 导致 badge/SSE 401。
// replace 进行中则停止挂载，避免在旧宿主上再开一条无效会话流。
if (!redirectToCanonicalLocalDevHost()) {
  const rootElement = document.getElementById('root')!
  if (!rootElement.innerHTML) {
    const root = ReactDOM.createRoot(rootElement)
    root.render(
      <StrictMode>
        <QueryClientProvider client={queryClient}>
          <ThemeProvider>
            <FontProvider>
              <DirectionProvider>
                <AuthProvider apiBaseUrl={resolveControlPlaneUrl()}>
                  <RouterProvider router={router} />
                </AuthProvider>
              </DirectionProvider>
            </FontProvider>
          </ThemeProvider>
        </QueryClientProvider>
      </StrictMode>
    )
  }
}

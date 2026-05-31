import { StrictMode } from 'react'
import ReactDOM from 'react-dom/client'
import {
  QueryCache,
  QueryClient,
  QueryClientProvider,
} from '@tanstack/react-query'
import { RouterProvider, createRouter } from '@tanstack/react-router'
import { toast } from 'sonner'
import { AuthProvider } from '@/features/auth/auth-provider'
import { ApiRequestError } from '@/lib/api'
import { resolveControlPlaneUrl } from '@/lib/config/control-plane-url'
import { DirectionProvider } from './context/direction-provider'
import { FontProvider } from './context/font-provider'
import { ThemeProvider } from './context/theme-provider'
// Generated Routes
import { routeTree } from './routeTree.gen'
// Styles
import './styles/index.css'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (failureCount, error) => {
        if (error instanceof ApiRequestError && [401, 403].includes(error.status)) {
          return false
        }
        return failureCount < 2
      },
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
      },
    },
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
    },
  }),
})

// Create a new router instance
const router = createRouter({
  routeTree,
  context: { queryClient },
  defaultPreload: 'intent',
  defaultPreloadStaleTime: 0,
})

// Register the router instance for type safety
declare module '@tanstack/react-router' {
  interface Register {
    router: typeof router
  }
}

// Render the app
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

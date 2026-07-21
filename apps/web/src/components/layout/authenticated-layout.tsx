import { Navigate, Outlet, useLocation } from '@tanstack/react-router'
import { useAuth } from '@/features/auth/use-auth'
import { useInboxChangeStream } from '@/features/inbox/use-inbox-change-stream'
import { getCookie } from '@/lib/cookies'
import { cn } from '@/lib/utils'
import { LayoutProvider } from '@/context/layout-provider'
import { SearchProvider } from '@/context/search-provider'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { AppSidebar } from '@/components/layout/app-sidebar'
import { AuroraBackground } from '@/components/layout/aurora-background'
import { SkipToMain } from '@/components/skip-to-main'

type AuthenticatedLayoutProps = {
  children?: React.ReactNode
  /** 测试注入；生产不传，hook 用原生 EventSource。 */
  inboxEventSourceFactory?: (url: string) => EventSource
}

function AuthenticatedShell({
  children,
  inboxEventSourceFactory,
}: {
  children?: React.ReactNode
  inboxEventSourceFactory?: (url: string) => EventSource
}) {
  // 全局一条收件箱 SSE：任意路由都能让侧栏角标与收件箱列表秒级感知变更。
  useInboxChangeStream({ eventSourceFactory: inboxEventSourceFactory })

  const defaultOpen = getCookie('sidebar_state') !== 'false'
  return (
    <SearchProvider>
      <LayoutProvider>
        <SidebarProvider
          data-slot='v3-authenticated-shell'
          defaultOpen={defaultOpen}
          className='text-v3-ink'
        >
          <AuroraBackground />
          <SkipToMain />
          <AppSidebar />
          <SidebarInset
            className={cn(
              // Set content container, so we can use container queries
              '@container/content',
              'min-w-0 overflow-x-hidden bg-transparent text-v3-ink',

              // If layout is fixed, set the height
              // to 100svh to prevent overflow
              'has-data-[layout=fixed]:h-svh',

              // If layout is fixed and sidebar is inset,
              // keep the height flush because the acrylic shell has no outer inset gap.
              'peer-data-[variant=inset]:has-data-[layout=fixed]:h-svh'
            )}
          >
            {children ?? <Outlet />}
          </SidebarInset>
        </SidebarProvider>
      </LayoutProvider>
    </SearchProvider>
  )
}

export function AuthenticatedLayout({
  children,
  inboxEventSourceFactory,
}: AuthenticatedLayoutProps) {
  const { isAuthenticated, isLoading } = useAuth()
  const location = useLocation()
  const isAuthRoute =
    location.pathname === '/login' || location.pathname === '/sign-in'

  if (isLoading) {
    return (
      <div className='flex h-svh items-center justify-center text-sm text-muted-foreground'>
        加载中...
      </div>
    )
  }

  if (!isAuthenticated) {
    if (isAuthRoute) {
      return null
    }

    return <Navigate to='/login' search={{ redirect: location.href }} replace />
  }

  return (
    <AuthenticatedShell inboxEventSourceFactory={inboxEventSourceFactory}>
      {children}
    </AuthenticatedShell>
  )
}

import type { ReactNode } from 'react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-react'
import { AuthenticatedLayout } from './authenticated-layout'

const mocks = vi.hoisted(() => ({
  auth: {
    isAuthenticated: false,
    isLoading: false,
  },
  location: {
    href: '/',
    pathname: '/',
  },
  navigateProps: [] as unknown[],
  sidebarProviderProps: [] as Array<{ className?: string; dataSlot?: string }>,
  sidebarInsetProps: [] as Array<{ className?: string; dataSlot?: string }>,
  inboxStreamCalls: [] as Array<{ url: string }>,
}))

vi.mock('@/features/auth/use-auth', () => ({
  useAuth: () => mocks.auth,
}))

vi.mock('@/lib/cookies', () => ({
  getCookie: () => 'true',
}))

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    Navigate: (props: unknown) => {
      mocks.navigateProps.push(props)
      return <div>redirecting</div>
    },
    Outlet: () => <div>outlet</div>,
    useLocation: () => mocks.location,
  }
})

vi.mock('@/context/layout-provider', () => ({
  LayoutProvider: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}))

vi.mock('@/context/search-provider', () => ({
  SearchProvider: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}))

vi.mock('@/components/ui/sidebar', () => ({
  SidebarInset: ({
    children,
    className,
    'data-slot': dataSlot,
  }: {
    children: ReactNode
    className?: string
    'data-slot'?: string
  }) => {
    mocks.sidebarInsetProps.push({ className, dataSlot })
    return <main>{children}</main>
  },
  SidebarProvider: ({
    children,
    className,
    'data-slot': dataSlot,
  }: {
    children: ReactNode
    className?: string
    'data-slot'?: string
  }) => {
    mocks.sidebarProviderProps.push({ className, dataSlot })
    return <div>{children}</div>
  },
}))

vi.mock('@/components/layout/app-sidebar', () => ({
  AppSidebar: () => <aside>sidebar</aside>,
}))

vi.mock('@/components/skip-to-main', () => ({
  SkipToMain: () => <a href='#main'>skip</a>,
}))

vi.mock('@/components/layout/aurora-background', () => ({
  AuroraBackground: () => null,
}))

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      mutations: { retry: false },
      queries: { retry: false },
    },
  })
}

function fakeEventSourceFactory(url: string) {
  mocks.inboxStreamCalls.push({ url })
  return {
    addEventListener: () => {},
    removeEventListener: () => {},
    close: () => {},
  } as unknown as EventSource
}

describe('AuthenticatedLayout', () => {
  beforeEach(() => {
    mocks.auth.isAuthenticated = false
    mocks.auth.isLoading = false
    mocks.location.href = '/'
    mocks.location.pathname = '/'
    mocks.navigateProps = []
    mocks.sidebarProviderProps = []
    mocks.sidebarInsetProps = []
    mocks.inboxStreamCalls = []
  })

  it('redirects unauthenticated protected routes to login once', async () => {
    await render(
      <QueryClientProvider client={createQueryClient()}>
        <AuthenticatedLayout inboxEventSourceFactory={fakeEventSourceFactory} />
      </QueryClientProvider>,
    )

    expect(mocks.navigateProps).toHaveLength(1)
    expect(mocks.navigateProps[0]).toMatchObject({
      replace: true,
      search: { redirect: '/' },
      to: '/login',
    })
    expect(mocks.inboxStreamCalls).toHaveLength(0)
  })

  it('does not recursively redirect while the login route is active', async () => {
    mocks.location.href = '/login?redirect=%2F'
    mocks.location.pathname = '/login'

    await render(
      <QueryClientProvider client={createQueryClient()}>
        <AuthenticatedLayout inboxEventSourceFactory={fakeEventSourceFactory} />
      </QueryClientProvider>,
    )

    expect(mocks.navigateProps).toHaveLength(0)
    expect(mocks.inboxStreamCalls).toHaveLength(0)
  })

  it('applies the v3 soft-flat shell surface to authenticated routes', async () => {
    mocks.auth.isAuthenticated = true

    await render(
      <QueryClientProvider client={createQueryClient()}>
        <AuthenticatedLayout inboxEventSourceFactory={fakeEventSourceFactory} />
      </QueryClientProvider>,
    )

    expect(mocks.sidebarProviderProps).toHaveLength(1)
    expect(mocks.sidebarProviderProps[0]).toMatchObject({
      dataSlot: 'v3-authenticated-shell',
    })
    expect(mocks.sidebarProviderProps[0]?.className).toContain('text-v3-ink')
    expect(mocks.sidebarProviderProps[0]?.className).not.toContain('bg-v3-bg')
    expect(mocks.sidebarInsetProps[0]?.className).toContain('bg-transparent')
  })

  it('opens a global inbox change stream once authenticated', async () => {
    mocks.auth.isAuthenticated = true

    await render(
      <QueryClientProvider client={createQueryClient()}>
        <AuthenticatedLayout inboxEventSourceFactory={fakeEventSourceFactory} />
      </QueryClientProvider>,
    )

    expect(mocks.inboxStreamCalls).toHaveLength(1)
    expect(mocks.inboxStreamCalls[0]?.url).toMatch(/\/api\/v1\/inbox\/stream$/)
  })
})

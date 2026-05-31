import type { ReactNode } from 'react'
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
  SidebarInset: ({ children }: { children: ReactNode }) => <main>{children}</main>,
  SidebarProvider: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}))

vi.mock('@/components/layout/app-sidebar', () => ({
  AppSidebar: () => <aside>sidebar</aside>,
}))

vi.mock('@/components/skip-to-main', () => ({
  SkipToMain: () => <a href='#main'>skip</a>,
}))

describe('AuthenticatedLayout', () => {
  beforeEach(() => {
    mocks.auth.isAuthenticated = false
    mocks.auth.isLoading = false
    mocks.location.href = '/'
    mocks.location.pathname = '/'
    mocks.navigateProps = []
  })

  it('redirects unauthenticated protected routes to login once', async () => {
    await render(<AuthenticatedLayout />)

    expect(mocks.navigateProps).toHaveLength(1)
    expect(mocks.navigateProps[0]).toMatchObject({
      replace: true,
      search: { redirect: '/' },
      to: '/login',
    })
  })

  it('does not recursively redirect while the login route is active', async () => {
    mocks.location.href = '/login?redirect=%2F'
    mocks.location.pathname = '/login'

    await render(<AuthenticatedLayout />)

    expect(mocks.navigateProps).toHaveLength(0)
  })
})

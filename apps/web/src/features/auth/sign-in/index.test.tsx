import type { ReactNode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-react'
import { SignIn } from './index'

vi.mock('@tanstack/react-router', () => ({
  useSearch: () => ({ redirect: '/projects' }),
}))

vi.mock('./components/user-auth-form', () => ({
  UserAuthForm: ({ redirectTo }: { redirectTo?: string }) => (
    <form aria-label='登录表单' data-redirect-to={redirectTo}>
      <button type='submit'>登录</button>
    </form>
  ),
}))

vi.mock('../auth-layout', () => ({
  AuthLayout: ({ children }: { children: ReactNode }) => (
    <section data-testid='auth-layout'>{children}</section>
  ),
}))

describe('SignIn', () => {
  it('renders the login form inside a reusable v3 soft card', async () => {
    const screen = await render(<SignIn />)

    await expect.element(screen.getByRole('heading', { name: '账号登录' })).toBeVisible()
    const card = document.querySelector('[data-slot="v3-soft-card"]')
    expect(card).not.toBeNull()
    expect(card?.getAttribute('class')).toContain('rounded-v3-card')
    await expect.element(screen.getByRole('form', { name: '登录表单' })).toHaveAttribute(
      'data-redirect-to',
      '/projects',
    )
  })
})

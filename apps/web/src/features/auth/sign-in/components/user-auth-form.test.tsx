import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-react'
import { userEvent } from 'vitest/browser'
import { UserAuthForm } from './user-auth-form'

const login = vi.fn()
const navigate = vi.fn()

vi.mock('@/features/auth/use-auth', () => ({
  useAuth: () => ({ login }),
}))

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => navigate,
  }
})

describe('UserAuthForm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows validation messages when submitting empty form', async () => {
    const screen = await render(<UserAuthForm />)

    await userEvent.click(screen.getByRole('button', { name: /^Sign in$/i }))

    await expect.element(screen.getByText('请输入用户名。')).toBeVisible()
    await expect.element(screen.getByText('请输入密码。')).toBeVisible()
  })

  it('logs in with username and password, then navigates home', async () => {
    const screen = await render(<UserAuthForm />)

    await userEvent.fill(
      screen.getByRole('textbox', { name: /^Username$/i }),
      'admin'
    )
    await userEvent.fill(screen.getByLabelText(/^Password$/i), 'admin')
    await userEvent.click(screen.getByRole('button', { name: /^Sign in$/i }))

    await vi.waitFor(() =>
      expect(login).toHaveBeenCalledWith({
        username: 'admin',
        password: 'admin',
      })
    )
    expect(navigate).toHaveBeenCalledWith({ to: '/', replace: true })
  })

  it('navigates to redirectTo after successful login', async () => {
    const screen = await render(<UserAuthForm redirectTo='/tasks' />)

    await userEvent.fill(
      screen.getByRole('textbox', { name: /^Username$/i }),
      'admin'
    )
    await userEvent.fill(screen.getByLabelText(/^Password$/i), 'admin')
    await userEvent.click(screen.getByRole('button', { name: /^Sign in$/i }))

    await vi.waitFor(() =>
      expect(navigate).toHaveBeenCalledWith({ to: '/tasks', replace: true })
    )
  })

  it('renders a form-level error when login fails', async () => {
    login.mockRejectedValueOnce(new Error('invalid credentials'))
    const screen = await render(<UserAuthForm />)

    await userEvent.fill(
      screen.getByRole('textbox', { name: /^Username$/i }),
      'admin'
    )
    await userEvent.fill(screen.getByLabelText(/^Password$/i), 'wrong')
    await userEvent.click(screen.getByRole('button', { name: /^Sign in$/i }))

    await expect.element(screen.getByText('用户名或密码不正确')).toBeVisible()
    expect(navigate).not.toHaveBeenCalled()
  })
})

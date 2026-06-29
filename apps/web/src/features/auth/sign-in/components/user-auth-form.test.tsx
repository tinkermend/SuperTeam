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

    await expect
      .element(screen.getByPlaceholder('请输入账号'))
      .toBeVisible()

    await userEvent.click(screen.getByRole('button', { name: /^登录$/i }))

    await expect.element(screen.getByText('请输入用户名。')).toBeVisible()
    await expect.element(screen.getByText('请输入密码。')).toBeVisible()
  })

  it('uses v3 form controls while keeping the accessible login form', async () => {
    const screen = await render(<UserAuthForm />)

    const username = screen.getByRole('textbox', { name: /^账号$/i })
    await expect.element(username).toHaveClass('bg-v3-card-soft')
    await expect.element(username).toHaveClass('rounded-xl')

    const password = screen.getByLabelText(/^密码$/i)
    await expect.element(password).toHaveClass('bg-v3-card-soft')
    await expect.element(password).toHaveClass('rounded-xl')

    const submit = screen.getByRole('button', { name: /^登录$/i })
    await expect.element(submit).toHaveAttribute('data-slot', 'v3-button')
    await expect.element(submit).toHaveAttribute('data-variant', 'primary')
  })

  it('logs in with username and password, then navigates home', async () => {
    const screen = await render(<UserAuthForm />)

    await userEvent.fill(
      screen.getByRole('textbox', { name: /^账号$/i }),
      'admin'
    )
    await userEvent.fill(screen.getByLabelText(/^密码$/i), 'admin')
    await userEvent.click(screen.getByRole('button', { name: /^登录$/i }))

    await vi.waitFor(() =>
      expect(login).toHaveBeenCalledWith({
        username: 'admin',
        password: 'admin',
      })
    )
    expect(navigate).toHaveBeenCalledWith({ to: '/', replace: true })
  })

  it('ignores repeated submits while login is still running', async () => {
    login.mockImplementationOnce(() => new Promise(() => {}))
    const screen = await render(<UserAuthForm />)

    await userEvent.fill(
      screen.getByRole('textbox', { name: /^账号$/i }),
      'admin'
    )
    await userEvent.fill(screen.getByLabelText(/^密码$/i), 'admin')

    const form = screen.container.querySelector('form')
    expect(form).not.toBeNull()
    form?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    form?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))
    form?.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }))

    await vi.waitFor(() => expect(login).toHaveBeenCalledTimes(1))
    expect(navigate).not.toHaveBeenCalled()
  })

  it('navigates to redirectTo after successful login', async () => {
    const screen = await render(<UserAuthForm redirectTo='/tasks' />)

    await userEvent.fill(
      screen.getByRole('textbox', { name: /^账号$/i }),
      'admin'
    )
    await userEvent.fill(screen.getByLabelText(/^密码$/i), 'admin')
    await userEvent.click(screen.getByRole('button', { name: /^登录$/i }))

    await vi.waitFor(() =>
      expect(navigate).toHaveBeenCalledWith({ to: '/tasks', replace: true })
    )
  })

  it('renders a form-level error when login fails', async () => {
    login.mockRejectedValueOnce(new Error('invalid credentials'))
    const screen = await render(<UserAuthForm />)

    await userEvent.fill(
      screen.getByRole('textbox', { name: /^账号$/i }),
      'admin'
    )
    await userEvent.fill(screen.getByLabelText(/^密码$/i), 'wrong')
    await userEvent.click(screen.getByRole('button', { name: /^登录$/i }))

    await expect.element(screen.getByText('用户名或密码不正确')).toBeVisible()
    expect(navigate).not.toHaveBeenCalled()
  })
})

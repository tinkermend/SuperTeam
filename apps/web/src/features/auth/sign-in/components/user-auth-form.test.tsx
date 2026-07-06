import { beforeEach, describe, expect, it, vi } from 'vitest'
import { StrictMode } from 'react'
import { render } from 'vitest-browser-react'
import { userEvent } from 'vitest/browser'
import { ApiRequestError } from '@/lib/api'
import { UserAuthForm } from './user-auth-form'

const { getLoginCaptcha, login, navigate } = vi.hoisted(() => ({
  getLoginCaptcha: vi.fn(),
  login: vi.fn(),
  navigate: vi.fn(),
}))

vi.mock('@/features/auth/use-auth', () => ({
  useAuth: () => ({ apiBaseUrl: 'http://control-plane.local', login }),
}))

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    getLoginCaptcha,
  }
})

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => navigate,
  }
})

describe('UserAuthForm', () => {
  const defaultCaptcha = {
    enabled: true,
    captcha_id: '11111111-1111-4111-8111-111111111111',
    expires_at: '2026-06-30T08:00:00Z',
    image_data_url: 'data:image/svg+xml;base64,PHN2Zy8+',
  }

  beforeEach(() => {
    getLoginCaptcha.mockReset()
    login.mockReset()
    navigate.mockReset()
    getLoginCaptcha.mockResolvedValue(defaultCaptcha)
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

  it('loads and renders a login captcha challenge', async () => {
    const screen = await render(<UserAuthForm />)

    await expect
      .element(screen.getByRole('textbox', { name: /^图形验证码$/i }))
      .toBeVisible()

    const image = screen.getByRole('img', { name: /^图形验证码$/i })
    await expect.element(image).toBeVisible()
    await expect
      .element(image)
      .toHaveAttribute('src', defaultCaptcha.image_data_url)

    await expect
      .element(screen.getByRole('button', { name: /^刷新验证码$/i }))
      .toBeVisible()
    expect(getLoginCaptcha).toHaveBeenCalledWith({
      baseUrl: 'http://control-plane.local',
    })
  })

  it('does not render captcha controls before the server captcha state is known', async () => {
    let resolveCaptcha!: (value: typeof defaultCaptcha) => void
    getLoginCaptcha.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveCaptcha = resolve
      })
    )

    const screen = await render(<UserAuthForm />)

    await expect
      .element(screen.getByRole('textbox', { name: /^图形验证码$/i }))
      .not.toBeInTheDocument()
    await expect
      .element(screen.getByRole('button', { name: /^刷新验证码$/i }))
      .not.toBeInTheDocument()
    await expect
      .element(screen.getByRole('button', { name: /^登录$/i }))
      .toBeDisabled()

    resolveCaptcha(defaultCaptcha)

    await expect
      .element(screen.getByRole('textbox', { name: /^图形验证码$/i }))
      .toBeVisible()
  })

  it('shows a stable captcha failure state and keeps refresh usable', async () => {
    const refreshedCaptcha = {
      enabled: true,
      captcha_id: '22222222-2222-4222-8222-222222222222',
      expires_at: '2026-06-30T08:05:00Z',
      image_data_url: 'data:image/svg+xml;base64,PHN2ZyByZWZyZXNoZWQvPg==',
    }
    getLoginCaptcha
      .mockRejectedValueOnce(new Error('network failed'))
      .mockResolvedValueOnce(refreshedCaptcha)
    const screen = await render(<UserAuthForm />)

    await expect
      .element(screen.getByText('验证码加载失败，请刷新重试'))
      .toBeVisible()
    await expect
      .element(screen.getByTestId('captcha-placeholder'))
      .toBeVisible()
    await expect
      .element(screen.getByTestId('captcha-placeholder'))
      .toHaveTextContent('加载失败')
    await expect
      .element(screen.getByTestId('captcha-loading'))
      .not.toBeInTheDocument()

    const refresh = screen.getByRole('button', { name: /^刷新验证码$/i })
    await expect.element(refresh).toBeEnabled()
    await userEvent.click(refresh)

    await expect
      .element(screen.getByRole('img', { name: /^图形验证码$/i }))
      .toHaveAttribute('src', refreshedCaptcha.image_data_url)
  })

  it('keeps the latest captcha when an older refresh resolves later', async () => {
    let resolveInitialCaptcha!: (value: typeof defaultCaptcha) => void
    let resolveLatestCaptcha!: (value: typeof defaultCaptcha) => void
    const staleCaptcha = {
      enabled: true,
      captcha_id: '22222222-2222-4222-8222-222222222222',
      expires_at: '2026-06-30T08:05:00Z',
      image_data_url: 'data:image/svg+xml;base64,PHN2ZyBzdGFsZS8+',
    }
    const latestCaptcha = {
      enabled: true,
      captcha_id: '33333333-3333-4333-8333-333333333333',
      expires_at: '2026-06-30T08:10:00Z',
      image_data_url: 'data:image/svg+xml;base64,PHN2ZyBsYXRlc3QvPg==',
    }

    getLoginCaptcha
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveInitialCaptcha = resolve
        })
      )
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveLatestCaptcha = resolve
        })
      )

    const screen = await render(
      <StrictMode>
        <UserAuthForm />
      </StrictMode>
    )

    await vi.waitFor(() => expect(getLoginCaptcha).toHaveBeenCalledTimes(2))

    resolveLatestCaptcha(latestCaptcha)
    await expect
      .element(screen.getByRole('img', { name: /^图形验证码$/i }))
      .toHaveAttribute('src', latestCaptcha.image_data_url)

    const captchaCode = screen.getByRole('textbox', { name: /^图形验证码$/i })
    await userEvent.fill(captchaCode, 'A7K2')

    resolveInitialCaptcha(staleCaptcha)
    await new Promise((resolve) => window.setTimeout(resolve, 0))

    await expect
      .element(screen.getByRole('img', { name: /^图形验证码$/i }))
      .toHaveAttribute('src', latestCaptcha.image_data_url)
    await expect.element(captchaCode).toHaveValue('A7K2')
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
    await userEvent.fill(
      screen.getByRole('textbox', { name: /^图形验证码$/i }),
      'a7k2'
    )
    await userEvent.click(screen.getByRole('button', { name: /^登录$/i }))

    await vi.waitFor(() =>
      expect(login).toHaveBeenCalledWith({
        username: 'admin',
        password: 'admin',
        captcha_id: defaultCaptcha.captcha_id,
        captcha_code: 'A7K2',
      })
    )
    expect(navigate).toHaveBeenCalledWith({ to: '/', replace: true })
  })

  it('hides captcha controls and logs in without captcha when disabled', async () => {
    getLoginCaptcha.mockResolvedValueOnce({ enabled: false })
    const screen = await render(<UserAuthForm />)

    await expect
      .element(screen.getByRole('textbox', { name: /^图形验证码$/i }))
      .not.toBeInTheDocument()
    await expect
      .element(screen.getByRole('button', { name: /^刷新验证码$/i }))
      .not.toBeInTheDocument()

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
    await userEvent.fill(
      screen.getByRole('textbox', { name: /^图形验证码$/i }),
      'A7K2'
    )

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
    await userEvent.fill(
      screen.getByRole('textbox', { name: /^图形验证码$/i }),
      'A7K2'
    )
    await userEvent.click(screen.getByRole('button', { name: /^登录$/i }))

    await vi.waitFor(() =>
      expect(navigate).toHaveBeenCalledWith({ to: '/tasks', replace: true })
    )
  })

  it('renders a form-level error when login fails', async () => {
    const refreshedCaptcha = {
      enabled: true,
      captcha_id: '22222222-2222-4222-8222-222222222222',
      expires_at: '2026-06-30T08:05:00Z',
      image_data_url: 'data:image/svg+xml;base64,PHN2ZyByZWZyZXNoZWQvPg==',
    }
    getLoginCaptcha
      .mockResolvedValueOnce(defaultCaptcha)
      .mockResolvedValueOnce(refreshedCaptcha)
    login.mockRejectedValueOnce(new Error('invalid credentials'))
    const screen = await render(<UserAuthForm />)

    const username = screen.getByRole('textbox', { name: /^账号$/i })
    const password = screen.getByLabelText(/^密码$/i)
    const captchaCode = screen.getByRole('textbox', { name: /^图形验证码$/i })

    await userEvent.fill(username, 'admin')
    await userEvent.fill(password, 'wrong')
    await userEvent.fill(captchaCode, 'A7K2')
    await userEvent.click(screen.getByRole('button', { name: /^登录$/i }))

    await expect.element(screen.getByText('用户名或密码不正确')).toBeVisible()
    await vi.waitFor(() => expect(getLoginCaptcha).toHaveBeenCalledTimes(2))
    await expect.element(username).toHaveValue('admin')
    await expect.element(password).toHaveValue('wrong')
    await expect.element(captchaCode).toHaveValue('')
    await expect
      .element(screen.getByRole('img', { name: /^图形验证码$/i }))
      .toHaveAttribute('src', refreshedCaptcha.image_data_url)
    expect(navigate).not.toHaveBeenCalled()
  })

  it('shows the captcha error message when the captcha is invalid', async () => {
    const refreshedCaptcha = {
      enabled: true,
      captcha_id: '22222222-2222-4222-8222-222222222222',
      expires_at: '2026-06-30T08:05:00Z',
      image_data_url: 'data:image/svg+xml;base64,PHN2ZyByZWZyZXNoZWQvPg==',
    }
    getLoginCaptcha
      .mockResolvedValueOnce(defaultCaptcha)
      .mockResolvedValueOnce(refreshedCaptcha)
    login.mockRejectedValueOnce(
      new ApiRequestError('auth login', 401, '验证码不正确或已过期')
    )
    const screen = await render(<UserAuthForm />)

    const username = screen.getByRole('textbox', { name: /^账号$/i })
    const password = screen.getByLabelText(/^密码$/i)
    const captchaCode = screen.getByRole('textbox', { name: /^图形验证码$/i })

    await userEvent.fill(username, 'admin')
    await userEvent.fill(password, 'admin')
    await userEvent.fill(captchaCode, 'ZZZZ')
    await userEvent.click(screen.getByRole('button', { name: /^登录$/i }))

    await expect
      .element(screen.getByText('验证码不正确或已过期'))
      .toBeVisible()
    await expect
      .element(screen.getByText('用户名或密码不正确'))
      .not.toBeInTheDocument()
    await vi.waitFor(() => expect(getLoginCaptcha).toHaveBeenCalledTimes(2))
    await expect.element(username).toHaveValue('admin')
    await expect.element(password).toHaveValue('admin')
    await expect.element(captchaCode).toHaveValue('')
    await expect
      .element(screen.getByRole('img', { name: /^图形验证码$/i }))
      .toHaveAttribute('src', refreshedCaptcha.image_data_url)
    expect(navigate).not.toHaveBeenCalled()
  })
})

import { describe, expect, it } from 'vitest'
import { render } from 'vitest-browser-react'
import { AuthLayout } from './auth-layout'

describe('AuthLayout', () => {
  it('renders the SuperTeam brand identity for the login page', async () => {
    const screen = await render(
      <AuthLayout>
        <p>登录表单</p>
      </AuthLayout>
    )

    await expect.element(screen.getByText('SuperTeam')).toBeVisible()
    await expect
      .element(screen.getByText('企业级数字员工控制平面'))
      .toBeVisible()
    await expect.element(screen.getByText('登录表单')).toBeVisible()
  })
})

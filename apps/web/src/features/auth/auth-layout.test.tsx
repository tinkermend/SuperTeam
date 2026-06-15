import { describe, expect, it } from 'vitest'
import { render } from 'vitest-browser-react'
import { AuthLayout } from './auth-layout'

describe('AuthLayout', () => {
  it('renders the Jushu Platform brand assets for the login page', async () => {
    const screen = await render(
      <AuthLayout>
        <p>登录表单</p>
      </AuthLayout>
    )

    await expect
      .element(screen.getByRole('img', { name: '炬枢平台 - 新炬网络横幅' }))
      .toBeVisible()
    const headerClass = document
      .querySelector("img[alt='炬枢平台 - 新炬网络横幅']")
      ?.getAttribute('class')
    await expect
      .element(screen.getByRole('img', { name: '炬枢平台 - 新炬网络', exact: true }))
      .toBeVisible()
    const logoClass = document
      .querySelector("img[alt='炬枢平台 - 新炬网络']")
      ?.getAttribute('class')

    expect(headerClass).toContain('opacity-100')
    expect(headerClass).toContain('contrast-[1.08]')
    expect(headerClass).not.toContain('mix-blend-multiply')
    expect(headerClass).not.toContain('bg-white')
    expect(headerClass).not.toContain('border')
    expect(logoClass).toContain('opacity-100')
    expect(logoClass).toContain('contrast-[1.08]')
    expect(logoClass).not.toContain('mix-blend-multiply')
    expect(logoClass).not.toContain('bg-white')
    expect(logoClass).not.toContain('border')
    await expect.element(screen.getByText('登录表单')).toBeVisible()
  })
})

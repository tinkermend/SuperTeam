import { describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-react'
import { SidebarProvider } from '@/components/ui/sidebar'
import { AppTitle } from './app-title'
import '@/styles/index.css'

vi.mock('@tanstack/react-router', () => ({
  Link: ({
    children,
    ...props
  }: React.AnchorHTMLAttributes<HTMLAnchorElement>) => (
    <a href='/' {...props}>
      {children}
    </a>
  ),
}))

describe('AppTitle', () => {
  it('renders the Jushu Platform brand in the authenticated shell header', async () => {
    const screen = await render(
      <SidebarProvider>
        <AppTitle />
      </SidebarProvider>
    )

    await expect
      .element(screen.getByRole('link', { name: '炬枢平台 - 新炬网络' }))
      .toBeVisible()
    await expect
      .element(screen.getByText('炬枢平台'))
      .toBeVisible()
    await expect
      .element(screen.getByText('新炬网络'))
      .toBeVisible()
    const image = document.querySelector("img[src='/images/brand/jushu-platform-mark.png']")
    const imageClass = image?.getAttribute('class')
    const titleClass = screen.getByText('炬枢平台').element().getAttribute('class')
    const subtitleClass = screen.getByText('新炬网络').element().getAttribute('class')

    expect(image).toBeTruthy()
    expect(imageClass).toContain('h-14')
    expect(imageClass).toContain('w-14')
    expect(imageClass).toContain('object-contain')
    expect(imageClass).toContain('opacity-100')
    expect(imageClass).not.toContain('mix-blend-multiply')
    expect(imageClass).not.toContain('bg-white')
    expect(imageClass).not.toContain('border')
    expect(titleClass).toContain('text-[1.25rem]')
    expect(subtitleClass).toContain('text-[0.82rem]')
    expect(document.body.textContent).not.toContain('SuperTeam')
  })
})

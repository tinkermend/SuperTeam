import { describe, expect, it } from 'vitest'
import { render } from 'vitest-browser-react'
import { SidebarProvider } from '@/components/ui/sidebar'
import { AppTitle } from './app-title'
import '@/styles/index.css'

describe('AppTitle', () => {
  it('renders the Jushu Platform brand as a static shell header lockup', async () => {
    const screen = await render(
      <SidebarProvider>
        <AppTitle />
      </SidebarProvider>
    )

    await expect
      .element(screen.getByTestId('app-title-brand-lockup'))
      .toBeVisible()
    await expect
      .element(screen.getByText('炬枢平台'))
      .toBeVisible()
    await expect
      .element(screen.getByText('新炬网络'))
      .toBeVisible()
    const image = document.querySelector("img[src='/images/brand/jushu-platform-mark.png']")
    const brandLockup = screen.getByTestId('app-title-brand-lockup').element()
    const imageClass = image?.getAttribute('class')
    const titleClass = screen.getByText('炬枢平台').element().getAttribute('class')
    const subtitleClass = screen.getByText('新炬网络').element().getAttribute('class')
    const lockupClass = brandLockup.getAttribute('class')

    expect(document.querySelector('a[aria-label="炬枢平台 - 新炬网络"]')).toBeNull()
    expect(document.querySelector('button[aria-label="炬枢平台 - 新炬网络"]')).toBeNull()
    expect(brandLockup.tagName).toBe('DIV')
    expect(brandLockup.getAttribute('href')).toBeNull()
    expect(brandLockup.getAttribute('aria-label')).toBe('炬枢平台 - 新炬网络')
    expect(lockupClass).toContain('rounded-[18px]')
    expect(lockupClass).toContain('border-[var(--v3-shell-glass-border)]')
    expect(lockupClass).toContain('rgba(233,239,255,0.30)')
    expect(lockupClass).toContain('0_8px_20px_rgba(47,95,255,0.045)')
    expect(lockupClass).not.toContain('0_14px_34px')
    expect(lockupClass).toContain('group-data-[collapsible=icon]:bg-transparent')
    expect(lockupClass).toContain('group-data-[collapsible=icon]:shadow-none')
    expect(image).toBeTruthy()
    expect(imageClass).toContain('h-[46px]')
    expect(imageClass).toContain('w-[46px]')
    expect(imageClass).toContain('group-data-[collapsible=icon]:h-11')
    expect(imageClass).toContain('group-data-[collapsible=icon]:w-11')
    expect(imageClass).toContain('object-contain')
    expect(imageClass).toContain('opacity-100')
    expect(imageClass).toContain('drop-shadow-none')
    expect(imageClass).not.toContain('drop-shadow-[')
    expect(imageClass).not.toContain('mix-blend-multiply')
    expect(imageClass).not.toContain('bg-white')
    expect(imageClass).not.toContain('border')
    expect(titleClass).toContain('text-[22px]')
    expect(titleClass).toContain('text-v3-brand-deep')
    expect(titleClass).not.toContain('text-[#0a806f]')
    expect(subtitleClass).toContain('text-[14px]')
    expect(subtitleClass).toContain('font-semibold')
    expect(subtitleClass).toContain('text-v3-ink-2')
    expect(document.querySelector('[data-testid="brand-accent-line"]')).toBeNull()
    expect(document.body.textContent).not.toContain('控制台')
    expect(document.body.textContent).not.toContain('运行正常')
    expect(document.body.textContent).not.toContain('SuperTeam')
  })
})

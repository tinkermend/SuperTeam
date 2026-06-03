import { describe, expect, it } from 'vitest'
import { render } from 'vitest-browser-react'
import './index.css'

describe('liquid glass dark theme styles', () => {
  it('does not reuse light-mode glare on dark sidebar and card surfaces', async () => {
    await render(
      <div className='dark'>
        <aside
          data-sidebar='sidebar'
          data-testid='sidebar-inner'
          data-slot='sidebar-inner'
        >
          sidebar
        </aside>
        <section data-testid='liquid-card' className='superteam-liquid-card'>
          card
        </section>
        <span data-testid='icon-tile' className='superteam-icon-tile'>
          icon
        </span>
        <section data-testid='auth-shell' className='superteam-auth-shell'>
          shell
        </section>
        <span data-testid='auth-mark' className='superteam-auth-mark'>
          ST
        </span>
      </div>
    )

    const sidebarInner = document.querySelector(
      '[data-testid="sidebar-inner"]'
    )
    const liquidCard = document.querySelector('[data-testid="liquid-card"]')
    const iconTile = document.querySelector('[data-testid="icon-tile"]')
    const authShell = document.querySelector('[data-testid="auth-shell"]')
    const authMark = document.querySelector('[data-testid="auth-mark"]')

    expect(sidebarInner).toBeInstanceOf(HTMLElement)
    expect(liquidCard).toBeInstanceOf(HTMLElement)
    expect(iconTile).toBeInstanceOf(HTMLElement)
    expect(authShell).toBeInstanceOf(HTMLElement)
    expect(authMark).toBeInstanceOf(HTMLElement)

    const sidebarBackground = getComputedStyle(
      sidebarInner as HTMLElement
    ).backgroundImage
    const cardGlare = getComputedStyle(
      liquidCard as HTMLElement,
      '::before'
    ).backgroundImage
    const iconBackground = getComputedStyle(
      iconTile as HTMLElement
    ).backgroundImage
    const authShellBackground = getComputedStyle(
      authShell as HTMLElement
    ).backgroundImage
    const authMarkBackground = getComputedStyle(
      authMark as HTMLElement
    ).backgroundImage

    expect(sidebarBackground).toContain('linear-gradient')
    expect(sidebarBackground).not.toContain('rgba(255, 255, 255, 0.66)')
    expect(sidebarBackground).not.toContain('rgba(232, 248, 244, 0.76)')

    expect(cardGlare).toContain('linear-gradient')
    expect(cardGlare).not.toContain('rgba(255, 255, 255, 0.92)')
    expect(cardGlare).not.toContain('rgba(190, 246, 225, 0.3)')

    expect(iconBackground).toContain('linear-gradient')
    expect(iconBackground).not.toContain('rgba(255, 255, 255, 0.9)')
    expect(iconBackground).not.toContain('rgba(230, 251, 245, 0.74)')

    expect(authShellBackground).toContain('linear-gradient')
    expect(authShellBackground).not.toContain('rgba(255, 255, 255, 0.96)')
    expect(authShellBackground).not.toContain('rgba(232, 248, 244, 0.9)')

    expect(authMarkBackground).toContain('linear-gradient')
    expect(authMarkBackground).not.toContain('rgba(255, 255, 255, 0.96)')
    expect(authMarkBackground).not.toContain('rgba(156, 242, 218, 0.42)')
  })
})

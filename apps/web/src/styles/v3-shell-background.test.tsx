import { describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-react'
import { Sidebar, SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import './index.css'

describe('authenticated v3 shell background styles', () => {
  function forceDesktopSidebar() {
    const originalMatchMedia = window.matchMedia

    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      writable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: false,
        media: query,
        onchange: null,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
        addListener: vi.fn(),
        removeListener: vi.fn(),
        dispatchEvent: vi.fn(),
      })),
    })

    return () => {
      Object.defineProperty(window, 'matchMedia', {
        configurable: true,
        writable: true,
        value: originalMatchMedia,
      })
    }
  }

  it('uses the acrylic v3 shell background', async () => {
    const restoreMatchMedia = forceDesktopSidebar()

    try {
      await render(
        <SidebarProvider
          data-slot='v3-authenticated-shell'
          data-testid='sidebar-wrapper'
        >
          <Sidebar collapsible='icon' variant='inset'>
            Sidebar
          </Sidebar>
          <SidebarInset>Shell</SidebarInset>
        </SidebarProvider>
      )
    } finally {
      restoreMatchMedia()
    }

    const sidebarWrapper = document.querySelector(
      '[data-testid="sidebar-wrapper"]'
    )

    expect(sidebarWrapper).toBeInstanceOf(HTMLElement)
    expect((sidebarWrapper as HTMLElement).dataset.slot).toBe(
      'v3-authenticated-shell'
    )

    const bodyBackground = getComputedStyle(document.body).backgroundImage
    const bodyColor = getComputedStyle(document.body).backgroundColor
    const sidebarBackground = getComputedStyle(
      sidebarWrapper as HTMLElement
    ).backgroundImage
    const sidebarColor = getComputedStyle(sidebarWrapper as HTMLElement)
      .backgroundColor

    expect(bodyBackground).toContain('radial-gradient')
    expect(bodyBackground).toContain('linear-gradient')
    expect(bodyColor).toBe('rgb(246, 248, 251)')
    expect(sidebarBackground).toContain('radial-gradient')
    expect(sidebarBackground).toContain('linear-gradient')
    expect(sidebarColor).toBe('rgb(246, 248, 251)')
  })

  it('lets the header blend into the shell while keeping search prominent', async () => {
    await render(
      <div data-testid='sidebar-wrapper' data-slot='sidebar-wrapper'>
        <div className='peer' data-state='expanded' data-variant='inset' />
        <SidebarInset data-testid='sidebar-inset'>
          <header data-testid='header' data-slot='v3-shell-header'>
            <button
              type='button'
              data-slot='button'
              data-testid='search'
              className='border border-[var(--v3-shell-search-border)] bg-[var(--v3-shell-search)] text-v3-ink-2 shadow-[var(--v3-shell-search-shadow)] backdrop-blur-md'
            >
              Search
            </button>
          </header>
        </SidebarInset>
      </div>
    )

    const sidebarInset = document.querySelector('[data-testid="sidebar-inset"]')
    const sidebarWrapper = document.querySelector(
      '[data-testid="sidebar-wrapper"]'
    )
    const header = document.querySelector('[data-testid="header"]')
    const search = document.querySelector('[data-testid="search"]')

    expect(sidebarWrapper).toBeInstanceOf(HTMLElement)
    expect(sidebarInset).toBeInstanceOf(HTMLElement)
    expect(header).toBeInstanceOf(HTMLElement)
    expect(search).toBeInstanceOf(HTMLElement)

    const insetStyle = getComputedStyle(sidebarInset as HTMLElement)
    const headerStyle = getComputedStyle(header as HTMLElement)
    const searchElement = search as HTMLElement
    const searchStyle = getComputedStyle(searchElement)

    expect((sidebarInset as HTMLElement).className).not.toContain('shadow')
    expect(insetStyle.backgroundImage).toBe('none')
    expect(insetStyle.backgroundColor).toBe('rgba(0, 0, 0, 0)')
    expect(insetStyle.boxShadow).toBe('none')
    expect(headerStyle.backgroundImage).toBe('none')
    expect(headerStyle.backgroundColor).toBe('rgba(0, 0, 0, 0)')
    expect(headerStyle.borderBottomWidth).toBe('0px')
    expect(headerStyle.backdropFilter).toBe('none')
    expect(headerStyle.boxShadow).toBe('none')
    expect(searchStyle.backgroundColor).toBe('rgba(255, 255, 255, 0.82)')
    expect(searchStyle.borderColor).toBe('rgba(47, 95, 255, 0.14)')
    expect(searchStyle.boxShadow).toContain('rgba(39, 54, 75, 0.1)')
    expect(searchStyle.backdropFilter).toContain('blur')
  })

  it('keeps inset sidebars flush instead of adding a floating inner layer', async () => {
    const restoreMatchMedia = forceDesktopSidebar()

    try {
      await render(
        <SidebarProvider>
          <Sidebar collapsible='icon' variant='inset'>
            Sidebar
          </Sidebar>
          <SidebarInset>Shell</SidebarInset>
        </SidebarProvider>
      )
    } finally {
      restoreMatchMedia()
    }

    const sidebarContainer = document.querySelector(
      '[data-slot="sidebar-container"]'
    )
    const sidebarGap = document.querySelector('[data-slot="sidebar-gap"]')
    const sidebarInset = document.querySelector('[data-slot="sidebar-inset"]')

    expect(sidebarContainer).toBeInstanceOf(HTMLElement)
    expect(sidebarGap).toBeInstanceOf(HTMLElement)
    expect(sidebarInset).toBeInstanceOf(HTMLElement)

    const containerStyle = getComputedStyle(sidebarContainer as HTMLElement)
    const insetStyle = getComputedStyle(sidebarInset as HTMLElement)

    expect(containerStyle.paddingTop).toBe('0px')
    expect(containerStyle.paddingRight).toBe('0px')
    expect(containerStyle.paddingBottom).toBe('0px')
    expect(containerStyle.paddingLeft).toBe('0px')
    expect(insetStyle.marginTop).toBe('0px')
    expect(insetStyle.marginRight).toBe('0px')
    expect(insetStyle.marginBottom).toBe('0px')
    expect(insetStyle.marginLeft).toBe('0px')
    expect(insetStyle.borderTopLeftRadius).toBe('0px')
    expect(insetStyle.borderTopRightRadius).toBe('0px')
    expect((sidebarGap as HTMLElement).className).not.toContain(
      '--spacing(4)'
    )
  })

  it('keeps the sidebar panel distinct from the shell with one soft divider', async () => {
    await render(
      <aside data-testid='sidebar-container' data-slot='sidebar-container'>
        <div
          className='flex h-full w-full flex-col'
          data-sidebar='sidebar'
          data-testid='sidebar-inner'
          data-slot='sidebar-inner'
        >
          Sidebar
        </div>
      </aside>
    )

    const sidebarContainer = document.querySelector(
      '[data-testid="sidebar-container"]'
    )
    const sidebarInner = document.querySelector('[data-testid="sidebar-inner"]')

    expect(sidebarContainer).toBeInstanceOf(HTMLElement)
    expect(sidebarInner).toBeInstanceOf(HTMLElement)

    const containerStyle = getComputedStyle(sidebarContainer as HTMLElement)
    const innerStyle = getComputedStyle(sidebarInner as HTMLElement)

    expect(containerStyle.borderInlineEndWidth).toBe('1px')
    expect(containerStyle.borderInlineEndColor).toBe(
      'rgba(126, 143, 172, 0.22)'
    )
    expect(containerStyle.boxShadow).toBe('none')
    expect(innerStyle.borderInlineEndWidth).toBe('0px')
    expect(innerStyle.boxShadow).toBe('none')
    expect(innerStyle.backgroundColor).toBe('rgba(249, 251, 255, 0.9)')
    expect(innerStyle.backgroundImage).toContain('linear-gradient')
    expect(innerStyle.backdropFilter).toContain('blur')
  })

  it('uses a floating glass sidebar panel for the default task hub shell', async () => {
    const restoreMatchMedia = forceDesktopSidebar()

    try {
      await render(
        <SidebarProvider>
          <Sidebar collapsible='icon' variant='floating'>
            Sidebar
          </Sidebar>
          <SidebarInset>Shell</SidebarInset>
        </SidebarProvider>
      )
    } finally {
      restoreMatchMedia()
    }

    const sidebarContainer = document.querySelector(
      '[data-slot="sidebar-container"]'
    )
    const sidebarInner = document.querySelector('[data-slot="sidebar-inner"]')

    expect(sidebarContainer).toBeInstanceOf(HTMLElement)
    expect(sidebarInner).toBeInstanceOf(HTMLElement)

    const containerStyle = getComputedStyle(sidebarContainer as HTMLElement)
    const innerStyle = getComputedStyle(sidebarInner as HTMLElement)

    expect(containerStyle.paddingTop).toBe('8px')
    expect(containerStyle.borderInlineEndWidth).toBe('0px')
    expect(innerStyle.borderTopLeftRadius).toBe('26px')
    expect(innerStyle.borderTopRightRadius).toBe('26px')
    expect(innerStyle.borderTopWidth).toBe('1px')
    expect(innerStyle.borderTopColor).toBe('rgba(126, 143, 172, 0.22)')
    expect(innerStyle.backgroundColor).toBe('rgba(255, 255, 255, 0.96)')
    expect(innerStyle.backgroundImage).toContain('radial-gradient')
    expect(innerStyle.backgroundImage).toContain('linear-gradient')
    expect(innerStyle.boxShadow).toContain('rgba(16, 24, 40, 0.09)')
    expect(innerStyle.backdropFilter).toContain('blur')
  })

  it('keeps active sidebar items translucent over the acrylic panel', async () => {
    await render(
      <aside data-testid='sidebar-container' data-slot='sidebar-container'>
        <div
          className='flex h-full w-full flex-col'
          data-sidebar='sidebar'
          data-testid='sidebar-inner'
          data-slot='sidebar-inner'
        >
          <a
            data-active='true'
            data-sidebar='menu-button'
            data-slot='sidebar-menu-button'
            href='/skills'
          >
            技能管理
          </a>
        </div>
      </aside>
    )

    const menuButton = document.querySelector('[data-slot="sidebar-menu-button"]')

    expect(menuButton).toBeInstanceOf(HTMLElement)

    const buttonStyle = getComputedStyle(menuButton as HTMLElement)

    expect(buttonStyle.backgroundColor).toBe('rgba(225, 234, 255, 0.88)')
    expect(buttonStyle.backgroundImage).toBe('none')
    expect(buttonStyle.borderColor).toBe('rgba(47, 95, 255, 0.22)')
    expect(buttonStyle.boxShadow).toBe('none')
  })
})

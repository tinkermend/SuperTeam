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

  it('uses a neutral v3 shell background', async () => {
    await render(
      <div data-testid='sidebar-wrapper' data-slot='sidebar-wrapper'>
        shell
      </div>
    )

    const sidebarWrapper = document.querySelector(
      '[data-testid="sidebar-wrapper"]'
    )

    expect(sidebarWrapper).toBeInstanceOf(HTMLElement)

    const bodyBackground = getComputedStyle(document.body).backgroundImage
    const bodyColor = getComputedStyle(document.body).backgroundColor
    const sidebarBackground = getComputedStyle(
      sidebarWrapper as HTMLElement
    ).backgroundImage
    const sidebarColor = getComputedStyle(sidebarWrapper as HTMLElement)
      .backgroundColor

    expect(bodyBackground).toBe('none')
    expect(bodyColor).toBe('rgb(248, 250, 252)')
    expect(sidebarBackground).toBe('none')
    expect(sidebarColor).toBe('rgb(248, 250, 252)')
  })

  it('keeps the header as a v3 white surface over the neutral shell', async () => {
    await render(
      <div data-testid='sidebar-wrapper' data-slot='sidebar-wrapper'>
        <div className='peer' data-state='expanded' data-variant='inset' />
        <SidebarInset data-testid='sidebar-inset'>
          <header data-testid='header' data-slot='v3-shell-header'>
            <button
              type='button'
              data-slot='button'
              data-testid='search'
              className='bg-v3-card text-v3-ink-2'
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
    expect(headerStyle.backgroundColor).toBe('rgb(255, 255, 255)')
    expect(headerStyle.borderBottomColor).toBe('rgb(238, 241, 244)')
    expect(headerStyle.boxShadow).toContain('rgba(16, 24, 40, 0.04)')
    expect(searchStyle.backgroundColor).toBe('rgb(255, 255, 255)')
  })

  it('keeps inset sidebars flush instead of adding a floating inner layer', async () => {
    const restoreMatchMedia = forceDesktopSidebar()

    try {
      await render(
        <SidebarProvider>
          <Sidebar collapsible='icon' variant='inset'>
            Sidebar
          </Sidebar>
        </SidebarProvider>
      )
    } finally {
      restoreMatchMedia()
    }

    const sidebarContainer = document.querySelector(
      '[data-slot="sidebar-container"]'
    )
    const sidebarGap = document.querySelector('[data-slot="sidebar-gap"]')

    expect(sidebarContainer).toBeInstanceOf(HTMLElement)
    expect(sidebarGap).toBeInstanceOf(HTMLElement)

    const containerStyle = getComputedStyle(sidebarContainer as HTMLElement)

    expect(containerStyle.paddingTop).toBe('0px')
    expect(containerStyle.paddingRight).toBe('0px')
    expect(containerStyle.paddingBottom).toBe('0px')
    expect(containerStyle.paddingLeft).toBe('0px')
    expect((sidebarGap as HTMLElement).className).not.toContain(
      '--spacing(4)'
    )
  })

  it('keeps the sidebar panel white with one soft v3 divider and no heavy shadow', async () => {
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
    expect(containerStyle.borderInlineEndColor).toBe('rgb(223, 228, 234)')
    expect(containerStyle.boxShadow).toBe('none')
    expect(innerStyle.borderInlineEndWidth).toBe('0px')
    expect(innerStyle.boxShadow).toBe('none')
    expect(innerStyle.backgroundColor).toBe('rgb(255, 255, 255)')
    expect(innerStyle.backgroundImage).toBe('none')
  })
})

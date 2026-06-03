import { describe, expect, it } from 'vitest'
import { render } from 'vitest-browser-react'
import { SidebarInset } from '@/components/ui/sidebar'
import './index.css'

describe('liquid glass shell background styles', () => {
  it('keeps the light shell aligned with the soft mint and warm cream reference palette', async () => {
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
    const sidebarBackground = getComputedStyle(
      sidebarWrapper as HTMLElement
    ).backgroundImage

    expect(bodyBackground).toContain('circle at 18% 4%')
    expect(bodyBackground).toContain('rgba(192, 246, 239, 0.36)')
    expect(bodyBackground).toContain('circle at 58% 0%')
    expect(bodyBackground).toContain('rgba(255, 239, 207, 0.5)')
    expect(bodyBackground).toContain('circle at 94% 5%')
    expect(bodyBackground).toContain('rgba(255, 244, 220, 0.44)')
    expect(bodyBackground).toContain('rgba(202, 241, 238, 0.28)')
    expect(sidebarBackground).toContain('circle at 18% 4%')
    expect(sidebarBackground).toContain('rgba(192, 246, 239, 0.36)')
    expect(sidebarBackground).toContain('circle at 58% 0%')
    expect(sidebarBackground).toContain('rgba(255, 239, 207, 0.5)')
    expect(sidebarBackground).toContain('circle at 94% 5%')
    expect(sidebarBackground).toContain('rgba(255, 244, 220, 0.44)')
    expect(sidebarBackground).toContain('rgba(202, 241, 238, 0.28)')
  })

  it('uses the wrapper as the single authenticated shell background source', async () => {
    await render(
      <div data-testid='sidebar-wrapper' data-slot='sidebar-wrapper'>
        <div className='peer' data-state='expanded' data-variant='inset' />
        <SidebarInset data-testid='sidebar-inset'>
          <header data-testid='header' className='superteam-header-glass'>
            <button
              type='button'
              data-slot='button'
              data-testid='search'
              className='superteam-search-glass'
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

    const wrapperBackground = getComputedStyle(
      sidebarWrapper as HTMLElement
    ).backgroundImage
    const insetStyle = getComputedStyle(sidebarInset as HTMLElement)
    const headerStyle = getComputedStyle(header as HTMLElement)
    const searchBackground = getComputedStyle(
      search as HTMLElement
    ).backgroundImage

    expect(wrapperBackground).toContain('circle at 58% 0%')
    expect(wrapperBackground).toContain('circle at 94% 5%')
    expect((sidebarInset as HTMLElement).className).not.toContain('shadow')
    expect(insetStyle.backgroundImage).toBe('none')
    expect(insetStyle.backgroundColor).toBe('rgba(0, 0, 0, 0)')
    expect(insetStyle.boxShadow).toBe('none')
    expect(headerStyle.backgroundImage).toBe('none')
    expect(headerStyle.borderBottomStyle).toBe('none')
    expect(headerStyle.borderBottomWidth).toBe('0px')
    expect(headerStyle.boxShadow).not.toContain('inset')
    expect(searchBackground).toContain('rgba(255, 255, 255, 0.62)')
    expect(searchBackground).not.toContain('rgba(255, 255, 255, 0.88)')
  })

  it('keeps the sidebar panel softly tinted without a hard divider line', async () => {
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

    expect(containerStyle.borderInlineEndColor).toBe(
      'rgba(156, 242, 218, 0.14)'
    )
    expect(containerStyle.boxShadow).toContain('rgba(255, 255, 255, 0.46)')
    expect(innerStyle.borderInlineEndColor).toBe('rgba(255, 255, 255, 0.5)')
    expect(innerStyle.backgroundColor).toBe('rgba(236, 250, 246, 0.64)')
    expect(innerStyle.backgroundImage).toContain(
      'rgba(192, 246, 239, 0.42)'
    )
    expect(innerStyle.backgroundImage).toContain(
      'rgba(255, 244, 220, 0.34)'
    )
    expect(innerStyle.backgroundImage).not.toContain(
      'rgba(255, 255, 255, 0.66)'
    )
  })
})

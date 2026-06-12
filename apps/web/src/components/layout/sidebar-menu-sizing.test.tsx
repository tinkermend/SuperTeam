import { Activity } from 'lucide-react'
import { describe, expect, it } from 'vitest'
import { render } from 'vitest-browser-react'
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
} from '@/components/ui/sidebar'
import '@/styles/index.css'

describe('sidebar menu sizing', () => {
  it('uses 16px labels and a taller menu row for expanded navigation', async () => {
    await render(
      <SidebarProvider>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton data-testid='sidebar-menu-button'>
              <Activity />
              <span>工作台</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarProvider>
    )

    const button = document.querySelector(
      '[data-testid="sidebar-menu-button"]'
    )
    const label = document.querySelector(
      '[data-testid="sidebar-menu-button"] span'
    )

    expect(button).toBeInstanceOf(HTMLElement)
    expect(label).toBeInstanceOf(HTMLElement)

    const buttonStyle = getComputedStyle(button as HTMLElement)
    const labelStyle = getComputedStyle(label as HTMLElement)

    expect(buttonStyle.height).toBe('44px')
    expect(buttonStyle.fontSize).toBe('16px')
    expect(labelStyle.fontSize).toBe('16px')
  })

  it('keeps active menu labels crisp on the selected gradient', async () => {
    await render(
      <SidebarProvider>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton isActive data-testid='active-sidebar-menu-button'>
              <Activity />
              <span>数字员工</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarProvider>
    )

    const button = document.querySelector(
      '[data-testid="active-sidebar-menu-button"]'
    )
    const label = document.querySelector(
      '[data-testid="active-sidebar-menu-button"] span'
    )
    const icon = document.querySelector(
      '[data-testid="active-sidebar-menu-button"] svg'
    )

    expect(button).toBeInstanceOf(HTMLElement)
    expect(label).toBeInstanceOf(HTMLElement)
    expect(icon).toBeInstanceOf(SVGElement)

    const buttonStyle = getComputedStyle(button as HTMLElement)
    const labelStyle = getComputedStyle(label as HTMLElement)
    const iconStyle = getComputedStyle(icon as SVGElement)

    expect(buttonStyle.color).toBe('rgb(255, 255, 255)')
    expect(labelStyle.color).toBe('rgb(255, 255, 255)')
    expect(labelStyle.textShadow).toBe('none')
    expect(iconStyle.color).toBe('rgb(255, 255, 255)')
  })

  it('keeps a badged inbox row at the standard navigation size', async () => {
    await render(
      <SidebarProvider>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton data-testid='badged-sidebar-menu-button'>
              <Activity />
              <span>收件箱</span>
              <span className='ms-auto rounded-full px-1.5 py-0 text-xs'>
                12
              </span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarProvider>
    )

    const button = document.querySelector(
      '[data-testid="badged-sidebar-menu-button"]'
    )
    const label = document.querySelector(
      '[data-testid="badged-sidebar-menu-button"] span'
    )
    const badge = document.querySelector(
      '[data-testid="badged-sidebar-menu-button"] span:last-child'
    )

    expect(button).toBeInstanceOf(HTMLElement)
    expect(label).toBeInstanceOf(HTMLElement)
    expect(badge).toBeInstanceOf(HTMLElement)

    const buttonStyle = getComputedStyle(button as HTMLElement)
    const labelStyle = getComputedStyle(label as HTMLElement)
    const badgeStyle = getComputedStyle(badge as HTMLElement)

    expect(buttonStyle.height).toBe('44px')
    expect(labelStyle.fontSize).toBe('16px')
    expect(badgeStyle.fontSize).toBe('12px')
  })
})

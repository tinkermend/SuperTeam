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
  it('uses compact 15px labels and a 44px menu row for expanded navigation', async () => {
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
    expect(buttonStyle.fontSize).toBe('15px')
    expect(labelStyle.fontSize).toBe('15px')
  })

  it('uses a restrained node-active state for selected menu rows', async () => {
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
    const beforeStyle = getComputedStyle(button as HTMLElement, '::before')

    expect(buttonStyle.backgroundColor).toBe('rgb(233, 239, 255)')
    expect(buttonStyle.color).toBe('rgb(35, 72, 224)')
    expect(buttonStyle.boxShadow).toBe('none')
    expect(beforeStyle.content).not.toBe('none')
    expect(beforeStyle.width).toBe('3px')
    expect(beforeStyle.height).toBe('20px')
    expect(beforeStyle.backgroundColor).toBe('rgb(47, 95, 255)')
    expect(labelStyle.color).toBe('rgb(35, 72, 224)')
    expect(labelStyle.fontWeight).toBe('600')
    expect(labelStyle.textShadow).toBe('none')
    expect(iconStyle.color).toBe('rgb(35, 72, 224)')
    expect(iconStyle.width).toBe('20px')
    expect(iconStyle.height).toBe('20px')
  })

  it('keeps a badged inbox row at the standard navigation size', async () => {
    await render(
      <SidebarProvider>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton data-testid='badged-sidebar-menu-button'>
              <Activity />
              <span>收件箱</span>
              <span className='ms-auto h-5 min-w-5 rounded-full bg-v3-brand-soft px-1.5 py-0 text-xs font-bold text-v3-brand-deep'>
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
    expect(labelStyle.fontSize).toBe('15px')
    expect(badgeStyle.minWidth).toBe('20px')
    expect(badgeStyle.height).toBe('20px')
    expect(badgeStyle.backgroundColor).toBe('rgb(233, 239, 255)')
    expect(badgeStyle.color).toBe('rgb(35, 72, 224)')
    expect(badgeStyle.fontSize).toBe('12px')
    expect(badgeStyle.fontWeight).toBe('700')
  })

  it('keeps default navigation icons neutral until hover or active states', async () => {
    await render(
      <SidebarProvider>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton data-testid='neutral-sidebar-menu-button'>
              <Activity />
              <span>外部能力</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarProvider>
    )

    const icon = document.querySelector(
      '[data-testid="neutral-sidebar-menu-button"] svg'
    )

    expect(icon).toBeInstanceOf(SVGElement)

    const iconStyle = getComputedStyle(icon as SVGElement)

    expect(iconStyle.color).toBe('rgb(100, 116, 139)')
    expect(iconStyle.width).toBe('20px')
    expect(iconStyle.height).toBe('20px')
  })
})

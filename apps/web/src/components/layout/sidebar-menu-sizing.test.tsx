import { Activity } from 'lucide-react'
import { beforeEach, describe, expect, it } from 'vitest'
import { render } from 'vitest-browser-react'
import { userEvent } from 'vitest/browser'
import {
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider
} from '@/components/ui/sidebar'
import '@/styles/index.css'

describe('sidebar menu sizing', () => {
  beforeEach(async () => {
    // 断言的是明亮主题的绝对色值；共享 document 可能残留其它测试文件切换的
    // dark class（浏览器模式串行跑时同页复用），先归位到默认明亮态。
    document.documentElement.classList.remove('light', 'dark')
    // 共享页面的鼠标会停留在上一个测试文件最后一次点击的坐标；本文件把菜单按钮
    // 渲染在页面左上角，若按钮恰好出现在陈旧指针位置下方，:hover 规则
    // （index.css: color: var(--ink) !important）会盖掉静息态颜色，产生
    // rgb(11,13,18) ≠ rgb(31,41,55) 的全量跑偶发失败。把指针停到右下角远离按钮。
    const pointerParking = document.createElement('div')
    pointerParking.style.cssText =
      'position:fixed;right:0;bottom:0;width:4px;height:4px;z-index:9999;'
    document.body.appendChild(pointerParking)
    await userEvent.hover(pointerParking)
    pointerParking.remove()
  })

  it('uses readable 15px labels and a 40px menu row for expanded navigation', async () => {
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

    const menu = document.querySelector('[data-slot="sidebar-menu"]')
    const button = document.querySelector(
      '[data-testid="sidebar-menu-button"]'
    )
    const label = document.querySelector(
      '[data-testid="sidebar-menu-button"] span'
    )

    expect(menu).toBeInstanceOf(HTMLElement)
    expect(button).toBeInstanceOf(HTMLElement)
    expect(label).toBeInstanceOf(HTMLElement)

    const menuStyle = getComputedStyle(menu as HTMLElement)
    const buttonStyle = getComputedStyle(button as HTMLElement)
    const labelStyle = getComputedStyle(label as HTMLElement)

    expect(menuStyle.gap).toBe('2px')
    expect(buttonStyle.height).toBe('40px')
    expect(buttonStyle.fontSize).toBe('15px')
    expect(buttonStyle.color).toBe('rgb(31, 41, 55)')
    expect(buttonStyle.fontWeight).toBe('500')
    expect(labelStyle.fontSize).toBe('15px')
  })

  it('keeps group labels quiet and close to their menu group', async () => {
    await render(
      <SidebarProvider>
        <SidebarGroup>
          <SidebarGroupLabel data-testid='first-sidebar-group-label'>
            工作区
          </SidebarGroupLabel>
        </SidebarGroup>
        <SidebarGroup>
          <SidebarGroupLabel data-testid='sidebar-group-label'>
            编排
          </SidebarGroupLabel>
        </SidebarGroup>
      </SidebarProvider>
    )

    const firstLabel = document.querySelector(
      '[data-testid="first-sidebar-group-label"]'
    )
    const label = document.querySelector('[data-testid="sidebar-group-label"]')

    expect(firstLabel).toBeInstanceOf(HTMLElement)
    expect(label).toBeInstanceOf(HTMLElement)

    const firstLabelStyle = getComputedStyle(firstLabel as HTMLElement)
    const labelStyle = getComputedStyle(label as HTMLElement)

    expect(firstLabelStyle.marginBlockStart).toBe('0px')
    expect(labelStyle.height).toBe('24px')
    expect(labelStyle.marginBlockStart).toBe('6px')
    expect(labelStyle.color).toBe('rgb(109, 117, 128)')
    expect(labelStyle.fontSize).toBe('12px')
    expect(labelStyle.fontWeight).toBe('500')
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

    expect(buttonStyle.position).toBe('relative')
    expect(buttonStyle.backgroundColor).toBe('rgba(47, 95, 255, 0.1)')
    expect(buttonStyle.backgroundImage).toBe('none')
    expect(buttonStyle.color).toBe('rgb(35, 72, 224)')
    expect(buttonStyle.boxShadow).toBe('none')
    expect(beforeStyle.content).not.toBe('none')
    expect(beforeStyle.width).toBe('3px')
    expect(beforeStyle.height).toBe('18px')
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
              <span className='ms-auto h-5 min-w-5 rounded-full bg-brand-soft px-1.5 py-0 text-xs font-bold text-brand-deep'>
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

    expect(buttonStyle.height).toBe('40px')
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

    expect(iconStyle.color).toBe('rgb(91, 102, 122)')
    expect(iconStyle.width).toBe('20px')
    expect(iconStyle.height).toBe('20px')
  })

  it('keeps inactive navigation items readable in dark mode', async () => {
    document.documentElement.classList.add('dark')

    try {
      await render(
        <SidebarProvider>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton data-testid='dark-sidebar-menu-button'>
                <Activity />
                <span>审计日志</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarProvider>
      )

      const button = document.querySelector(
        '[data-testid="dark-sidebar-menu-button"]'
      )
      const icon = document.querySelector(
        '[data-testid="dark-sidebar-menu-button"] svg'
      )

      expect(button).toBeInstanceOf(HTMLElement)
      expect(icon).toBeInstanceOf(SVGElement)

      const buttonStyle = getComputedStyle(button as HTMLElement)
      const iconStyle = getComputedStyle(icon as SVGElement)

      expect(buttonStyle.color).toBe('rgb(243, 245, 248)')
      expect(iconStyle.color).toBe('rgb(154, 166, 180)')
    } finally {
      document.documentElement.classList.remove('dark')
    }
  })
})

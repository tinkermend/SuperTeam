import { Activity, ChevronsUpDown } from 'lucide-react'
import { describe, expect, it } from 'vitest'
import { render } from 'vitest-browser-react'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider
} from '@/components/ui/sidebar'
import '@/styles/index.css'

function centerX(element: Element) {
  const rect = element.getBoundingClientRect()
  return rect.left + rect.width / 2
}

describe('collapsed sidebar alignment', () => {
  it('centers navigation icons in the collapsed sidebar panel', async () => {
    await render(
      <SidebarProvider defaultOpen={false}>
        <div
          className='group'
          data-state='collapsed'
          data-collapsible='icon'
          data-variant='sidebar'
        >
          <aside
            data-slot='sidebar-inner'
            style={{ width: '3rem', height: '12rem' }}
          >
            <SidebarContent>
              <SidebarGroup>
                <SidebarGroupLabel>工作区</SidebarGroupLabel>
                <SidebarMenu>
                  <SidebarMenuItem>
                    <SidebarMenuButton isActive data-testid='collapsed-item'>
                      <Activity />
                      <span>工作台</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                </SidebarMenu>
              </SidebarGroup>
            </SidebarContent>
          </aside>
        </div>
      </SidebarProvider>
    )

    const sidebar = document.querySelector("[data-slot='sidebar-inner']")
    const icon = document.querySelector(
      "[data-testid='collapsed-item'] svg"
    )

    expect(sidebar).toBeInstanceOf(HTMLElement)
    expect(icon).toBeInstanceOf(SVGElement)

    const iconOffset = Math.abs(centerX(icon!) - centerX(sidebar!))

    expect(iconOffset).toBeLessThanOrEqual(1)
  })

  it('hides link labels and badges before centering collapsed navigation icons', async () => {
    await render(
      <SidebarProvider defaultOpen={false}>
        <div
          className='group'
          data-state='collapsed'
          data-collapsible='icon'
          data-variant='sidebar'
        >
          <aside
            data-slot='sidebar-inner'
            style={{ width: '4.5rem', height: '12rem' }}
          >
            <SidebarContent>
              <SidebarGroup>
                <SidebarGroupLabel>工作区</SidebarGroupLabel>
                <SidebarMenu>
                  <SidebarMenuItem>
                    <SidebarMenuButton asChild data-testid='collapsed-link'>
                      <a href='/inbox'>
                        <Activity data-testid='collapsed-link-icon' />
                        <span data-testid='collapsed-link-label'>收件箱</span>
                        <span
                          className='ms-auto inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-brand-soft px-1.5 py-0 text-xs font-bold text-brand-deep'
                          data-testid='collapsed-link-badge'
                        >
                          4
                        </span>
                      </a>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                </SidebarMenu>
              </SidebarGroup>
            </SidebarContent>
          </aside>
        </div>
      </SidebarProvider>
    )

    const sidebar = document.querySelector("[data-slot='sidebar-inner']")
    const icon = document.querySelector("[data-testid='collapsed-link-icon']")
    const label = document.querySelector("[data-testid='collapsed-link-label']")
    const badge = document.querySelector("[data-testid='collapsed-link-badge']")

    expect(sidebar).toBeInstanceOf(HTMLElement)
    expect(icon).toBeInstanceOf(SVGElement)
    expect(label).toBeInstanceOf(HTMLElement)
    expect(badge).toBeInstanceOf(HTMLElement)

    expect(getComputedStyle(label as HTMLElement).display).toBe('none')
    expect(getComputedStyle(badge as HTMLElement).display).toBe('none')
    expect(Math.abs(centerX(icon!) - centerX(sidebar!))).toBeLessThanOrEqual(1)
  })

  it('centers the footer account avatar when the dropdown trigger owns the menu button', async () => {
    await render(
      <SidebarProvider defaultOpen={false}>
        <div
          className='group'
          data-state='collapsed'
          data-collapsible='icon'
          data-variant='sidebar'
        >
          <aside
            data-slot='sidebar-inner'
            style={{ width: '4.5rem', height: '12rem' }}
          >
            <SidebarFooter>
              <SidebarMenu>
                <SidebarMenuItem>
                  <SidebarMenuButton
                    data-slot='dropdown-menu-trigger'
                    data-testid='collapsed-account-trigger'
                    size='lg'
                  >
                    <Avatar
                      className='size-8 rounded-lg'
                      data-testid='collapsed-account-avatar'
                    >
                      <AvatarFallback className='rounded-lg bg-brand text-white'>
                        开发
                      </AvatarFallback>
                    </Avatar>
                    <div
                      className='grid flex-1 text-start text-sm leading-tight'
                      data-testid='collapsed-account-labels'
                    >
                      <span className='truncate font-semibold text-ink'>
                        开发管理员
                      </span>
                      <span className='truncate text-xs text-ink-2'>
                        admin@superteam.local
                      </span>
                    </div>
                    <ChevronsUpDown
                      className='ms-auto size-4 text-ink-3'
                      data-testid='collapsed-account-chevron'
                    />
                  </SidebarMenuButton>
                </SidebarMenuItem>
              </SidebarMenu>
            </SidebarFooter>
          </aside>
        </div>
      </SidebarProvider>
    )

    const sidebar = document.querySelector("[data-slot='sidebar-inner']")
    const avatar = document.querySelector(
      "[data-testid='collapsed-account-avatar']"
    )
    const labels = document.querySelector(
      "[data-testid='collapsed-account-labels']"
    )
    const chevron = document.querySelector(
      "[data-testid='collapsed-account-chevron']"
    )

    expect(sidebar).toBeInstanceOf(HTMLElement)
    expect(avatar).toBeInstanceOf(HTMLElement)
    expect(labels).toBeInstanceOf(HTMLElement)
    expect(chevron).toBeInstanceOf(SVGElement)

    expect(getComputedStyle(labels as HTMLElement).display).toBe('none')
    expect(getComputedStyle(chevron as SVGElement).display).toBe('none')
    expect(Math.abs(centerX(avatar!) - centerX(sidebar!))).toBeLessThanOrEqual(1)
  })
})

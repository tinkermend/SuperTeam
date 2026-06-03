import { Activity } from 'lucide-react'
import { describe, expect, it } from 'vitest'
import { render } from 'vitest-browser-react'
import {
  SidebarContent,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
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
})

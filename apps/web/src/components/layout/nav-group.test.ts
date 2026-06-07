import { describe, expect, it } from 'vitest'
import { checkIsActive } from './nav-group'
import type { NavCollapsible, NavLink } from './types'

describe('checkIsActive', () => {
  it('keeps top-level menu items active on detail and create routes', () => {
    const teamsItem: NavLink = { title: '团队管理', url: '/teams' }
    const employeesItem: NavLink = { title: '数字员工', url: '/employees' }

    expect(checkIsActive('/teams/team-1', teamsItem)).toBe(true)
    expect(checkIsActive('/teams/team-1?tab=members', teamsItem)).toBe(true)
    expect(checkIsActive('/employees/new', employeesItem)).toBe(true)
    expect(checkIsActive('/teams-archive', teamsItem)).toBe(false)
  })

  it('does not make the root dashboard active for every route', () => {
    const dashboardItem: NavLink = { title: '工作台', url: '/' }

    expect(checkIsActive('/', dashboardItem)).toBe(true)
    expect(checkIsActive('/teams/team-1', dashboardItem)).toBe(false)
  })

  it('keeps collapsible parents active when a child route is active', () => {
    const parentItem: NavCollapsible = {
      title: '平台管理',
      items: [{ title: '权限中心', url: '/permissions' }],
    }

    expect(checkIsActive('/permissions/diagnostics', parentItem)).toBe(true)
    expect(checkIsActive('/teams/team-1', parentItem)).toBe(false)
  })
})

import { describe, expect, it } from 'vitest'
import { sidebarData } from './data/sidebar-data'

const expectedIconTones = new Map([
  ['工作台', 'primary'],
  ['任务中心', 'task'],
  ['数字员工', 'employee'],
  ['团队管理', 'permission'],
  ['流程编排', 'workflow'],
  ['外部能力', 'capability'],
  ['审批中心', 'approval'],
  ['Runtime 节点', 'runtime'],
  ['权限中心', 'permission'],
  ['用户管理', 'neutral'],
  ['审计日志', 'audit'],
])

describe('sidebarData', () => {
  it('places team management after digital employees in the workspace group', () => {
    const workspaceItems = sidebarData.navGroups.find(
      (group) => group.title === '工作区'
    )?.items

    expect(workspaceItems?.map((item) => item.title)).toEqual([
      '工作台',
      '任务中心',
      '数字员工',
      '团队管理',
    ])
    expect(workspaceItems?.[3]).toMatchObject({
      title: '团队管理',
      url: '/teams',
      iconTone: 'permission',
    })
  })

  it('assigns each primary menu item a design-system icon tone', () => {
    const items = sidebarData.navGroups.flatMap((group) => group.items)

    for (const item of items) {
      const expectedTone = expectedIconTones.get(item.title)
      expect(expectedTone, `${item.title} should have an expected tone`).toBe(
        item.iconTone
      )
    }
  })
})

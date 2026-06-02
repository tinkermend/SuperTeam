import { describe, expect, it } from 'vitest'
import { sidebarData } from './data/sidebar-data'

const expectedIconTones = new Map([
  ['工作台', 'primary'],
  ['任务中心', 'task'],
  ['数字员工', 'employee'],
  ['流程编排', 'workflow'],
  ['外部能力', 'capability'],
  ['审批中心', 'approval'],
  ['Runtime 节点', 'runtime'],
  ['权限中心', 'permission'],
  ['用户管理', 'neutral'],
  ['审计日志', 'audit'],
])

describe('sidebarData', () => {
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

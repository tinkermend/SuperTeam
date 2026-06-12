import { describe, expect, it } from 'vitest'
import { buildSidebarData, sidebarData } from './data/sidebar-data'

const expectedIconTones = new Map([
  ['工作台', 'primary'],
  ['收件箱', 'approval'],
  ['任务发起', 'task'],
  ['数字员工', 'employee'],
  ['技能管理', 'capability'],
  ['项目管理', 'workflow'],
  ['团队管理', 'permission'],
  ['流程编排', 'workflow'],
  ['外部能力', 'capability'],
  ['协作集成', 'approval'],
  ['审批中心', 'approval'],
  ['Runtime 节点', 'runtime'],
  ['权限中心', 'permission'],
  ['成本管理', 'audit'],
  ['用户管理', 'neutral'],
  ['审计日志', 'audit'],
])

describe('sidebarData', () => {
  it('places inbox between dashboard and task launch in the workspace group', () => {
    const workspaceItems = sidebarData.navGroups.find(
      (group) => group.title === '工作区'
    )?.items

    expect(workspaceItems?.map((item) => item.title)).toEqual([
      '工作台',
      '收件箱',
      '任务发起',
      '项目管理',
      '数字员工',
      '技能管理',
      '团队管理',
    ])
    expect(workspaceItems?.[1]).toMatchObject({
      title: '收件箱',
      url: '/inbox',
      iconTone: 'approval',
    })
    expect(workspaceItems?.[2]).toMatchObject({
      title: '任务发起',
      url: '/task-launches',
      iconTone: 'task',
    })
    expect(workspaceItems?.[6]).toMatchObject({
      title: '团队管理',
      url: '/teams',
      iconTone: 'permission',
    })
  })

  it('places collaboration integration before approval center in the core navigation group', () => {
    const coreItems = sidebarData.navGroups.find(
      (group) => group.title === '核心导航'
    )?.items

    expect(coreItems?.map((item) => item.title)).toEqual([
      '流程编排',
      '外部能力',
      '协作集成',
      '审批中心',
      'Runtime 节点',
    ])
    expect(coreItems?.[2]).toMatchObject({
      title: '协作集成',
      url: '/collaboration',
      iconTone: 'approval',
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

  it('adds the inbox badge only when one is provided', () => {
    const workspaceItems = buildSidebarData({ inboxBadge: '12' }).navGroups.find(
      (group) => group.title === '工作区'
    )?.items
    const inboxItem = workspaceItems?.find((item) => item.title === '收件箱')
    const defaultInboxItem = sidebarData.navGroups
      .find((group) => group.title === '工作区')
      ?.items.find((item) => item.title === '收件箱')

    expect(inboxItem).toMatchObject({
      title: '收件箱',
      badge: '12',
    })
    expect(defaultInboxItem).not.toHaveProperty('badge')
  })

  it('places cost management in the platform management group', () => {
    const platformItems = sidebarData.navGroups.find(
      (group) => group.title === '平台管理'
    )?.items

    expect(platformItems?.map((item) => item.title)).toEqual([
      '权限中心',
      '成本管理',
      '用户管理',
      '审计日志',
    ])
    expect(platformItems?.[1]).toMatchObject({
      title: '成本管理',
      url: '/costs',
      iconTone: 'audit',
    })
  })
})

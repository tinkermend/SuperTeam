import { describe, expect, it } from 'vitest'
import { buildSidebarData, sidebarData } from './data/sidebar-data'

describe('sidebarData', () => {
  it('uses task hub as the homepage and removes the duplicate task launch menu item', () => {
    const workspaceItems = sidebarData.navGroups.find(
      (group) => group.title === '工作区'
    )?.items

    expect(workspaceItems?.map((item) => item.title)).toEqual([
      '任务中枢',
      '收件箱',
      '项目管理',
      '数字员工',
      '技能管理',
      '团队管理',
    ])
    expect(workspaceItems?.[0]).toMatchObject({
      title: '任务中枢',
      url: '/',
      iconTone: 'neutral',
    })
    expect(workspaceItems?.[1]).toMatchObject({
      title: '收件箱',
      url: '/inbox',
      iconTone: 'neutral',
    })
    expect(workspaceItems?.[2]).toMatchObject({
      title: '项目管理',
      url: '/projects',
      iconTone: 'neutral',
    })
    expect(workspaceItems?.[5]).toMatchObject({
      title: '团队管理',
      url: '/teams',
      iconTone: 'neutral',
    })
    expect(workspaceItems?.some((item) => item.title === '任务发起')).toBe(false)
  })

  it('places automation tasks between workflows and external capabilities in the core navigation group', () => {
    const coreItems = sidebarData.navGroups.find(
      (group) => group.title === '核心导航'
    )?.items

    expect(coreItems?.map((item) => item.title)).toEqual([
      '流程编排',
      '自动化任务',
      '外部能力',
      'MCP 管理',
      '协作集成',
      '审批中心',
      'Runtime 节点',
    ])
    expect(coreItems?.[1]).toMatchObject({
      title: '自动化任务',
      url: '/automations',
      iconTone: 'neutral',
    })
    expect(coreItems?.[2]).toMatchObject({
      title: '外部能力',
      url: '/capabilities',
      iconTone: 'neutral',
    })
  })

  it('keeps normal navigation icons neutral instead of assigning per-module colors', () => {
    const items = sidebarData.navGroups.flatMap((group) => group.items)

    for (const item of items) {
      expect(item.iconTone, `${item.title} should use the neutral nav tone`).toBe(
        'neutral'
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
      iconTone: 'neutral',
    })
  })
})

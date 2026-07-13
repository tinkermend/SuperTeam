import { describe, expect, it } from 'vitest'
import { buildSidebarData, sidebarData } from './data/sidebar-data'

describe('sidebarData', () => {
  it('groups daily workbench navigation without exposing the unfinished tasks route', () => {
    const workspaceItems = sidebarData.navGroups.find(
      (group) => group.title === '工作台'
    )?.items

    expect(workspaceItems?.map((item) => item.title)).toEqual([
      '任务中枢',
      '收件箱',
      '运行总览',
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
      title: '运行总览',
      url: '/run-overview',
      iconTone: 'neutral',
    })
    expect(workspaceItems?.some((item) => item.url === '/tasks')).toBe(false)
    expect(workspaceItems?.some((item) => item.title === '任务发起')).toBe(false)
  })

  it('groups collaboration objects separately from workflow capabilities', () => {
    const objectItems = sidebarData.navGroups.find(
      (group) => group.title === '协作对象'
    )?.items

    expect(objectItems?.map((item) => item.title)).toEqual([
      '项目管理',
      '数字员工',
      '技能管理',
      '团队管理',
    ])
    expect(objectItems?.[0]).toMatchObject({
      title: '项目管理',
      url: '/projects',
      iconTone: 'neutral',
    })
    expect(objectItems?.[3]).toMatchObject({
      title: '团队管理',
      url: '/teams',
      iconTone: 'neutral',
    })
  })

  it('places automation tasks between workflows and external capabilities in the workflow capability group', () => {
    const coreItems = sidebarData.navGroups.find(
      (group) => group.title === '流程能力'
    )?.items

    expect(coreItems?.map((item) => item.title)).toEqual([
      '流程编排',
      '自动化任务',
      '外部能力',
      'MCP 管理',
      '场景模板',
      '协作集成',
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
      (group) => group.title === '工作台'
    )?.items
    const inboxItem = workspaceItems?.find((item) => item.title === '收件箱')
    const defaultInboxItem = sidebarData.navGroups
      .find((group) => group.title === '工作台')
      ?.items.find((item) => item.title === '收件箱')

    expect(inboxItem).toMatchObject({
      title: '收件箱',
      badge: '12',
    })
    expect(defaultInboxItem).not.toHaveProperty('badge')
  })

  it('places governance and operations entries in the governance platform group', () => {
    const platformItems = sidebarData.navGroups.find(
      (group) => group.title === '治理平台'
    )?.items

    expect(platformItems?.map((item) => item.title)).toEqual([
      '审批中心',
      'Runtime 节点',
      '权限中心',
      '成本管理',
      '用户管理',
      '审计中心',
      '日志管理',
    ])
    expect(platformItems?.[0]).toMatchObject({
      title: '审批中心',
      url: '/approvals',
      iconTone: 'neutral',
    })
    expect(platformItems?.[3]).toMatchObject({
      title: '成本管理',
      url: '/costs',
      iconTone: 'neutral',
    })
  })

  it('keeps 日志管理 as a flat entry pointing at /logs', () => {
    const platformItems = sidebarData.navGroups.find(
      (group) => group.title === '治理平台'
    )?.items

    const logMenu = platformItems?.find((item) => item.title === '日志管理')
    expect(logMenu).toMatchObject({ title: '日志管理', url: '/logs' })
    expect(logMenu && 'items' in logMenu ? logMenu.items : undefined).toBeUndefined()
  })
})

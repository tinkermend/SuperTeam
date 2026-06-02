import { Activity, CheckCircle, Search } from 'lucide-react'
import { describe, expect, it } from 'vitest'
import { render } from 'vitest-browser-react'
import {
  LiquidCard,
  LiquidPill,
  MetricCard,
  PrimaryLiquidButton,
  SemanticIconTile,
  StatusBadge,
} from './liquid-components'

describe('SuperTeam liquid design components', () => {
  it('renders liquid cards with the shared glass surface class', async () => {
    const { getByText } = await render(
      <LiquidCard className='min-h-20'>任务概览</LiquidCard>
    )

    const card = getByText('任务概览')
    await expect.element(card).toBeInTheDocument()
    await expect.element(card).toHaveAttribute('data-slot', 'liquid-card')
    await expect.element(card).toHaveClass('superteam-liquid-card')
    await expect.element(card).toHaveClass('min-h-20')
  })

  it('renders liquid pills for compact filters and labels', async () => {
    const { getByText } = await render(<LiquidPill>最近 30 天</LiquidPill>)

    const pill = getByText('最近 30 天')
    await expect.element(pill).toHaveAttribute('data-slot', 'liquid-pill')
    await expect.element(pill).toHaveClass('superteam-liquid-pill')
  })

  it('renders primary liquid buttons on top of the shared button primitive', async () => {
    const { getByRole } = await render(
      <PrimaryLiquidButton>
        <Search data-icon='inline-start' />
        搜索
      </PrimaryLiquidButton>
    )

    const button = getByRole('button', { name: '搜索' })
    await expect.element(button).toHaveClass('superteam-primary-action')
    await expect.element(button).toHaveAttribute('data-slot', 'button')
  })

  it('renders semantic icon tiles with stable tone classes', async () => {
    const { getByLabelText } = await render(
      <SemanticIconTile tone='success' aria-label='健康状态'>
        <CheckCircle />
      </SemanticIconTile>
    )

    const tile = getByLabelText('健康状态')
    await expect.element(tile).toHaveAttribute('data-slot', 'semantic-icon-tile')
    await expect.element(tile).toHaveClass('superteam-icon-tile')
    await expect
      .element(tile)
      .toHaveClass('text-[color:var(--superteam-success)]')
  })

  it('renders status badges with a visible status dot', async () => {
    const { getByText } = await render(
      <StatusBadge tone='warning'>等待确认</StatusBadge>
    )

    const badge = getByText('等待确认')
    await expect.element(badge).toHaveAttribute('data-slot', 'status-badge')
    await expect.element(badge).toHaveClass('rounded-full')
    await expect.element(badge).toHaveClass('text-[color:var(--superteam-warning)]')
  })

  it('renders metric cards as reusable dashboard summaries', async () => {
    const { getByText } = await render(
      <MetricCard
        title='Control Plane'
        description='后端健康状态'
        icon={<Activity />}
        iconTone='info'
        value='ok'
        meta='健康'
        statusTone='success'
      />
    )

    await expect.element(getByText('Control Plane')).toBeInTheDocument()
    await expect.element(getByText('后端健康状态')).toBeInTheDocument()
    await expect.element(getByText('ok')).toHaveClass('text-5xl')
    await expect
      .element(getByText('健康', { exact: true }))
      .toHaveAttribute('data-slot', 'status-badge')
  })
})

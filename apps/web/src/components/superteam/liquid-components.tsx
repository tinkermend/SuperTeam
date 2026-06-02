import { type ComponentProps, type ReactNode } from 'react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { cn } from '@/lib/utils'

type Tone =
  | 'primary'
  | 'info'
  | 'success'
  | 'warning'
  | 'danger'
  | 'artifact'
  | 'decision'
  | 'neutral'

const toneTextClass: Record<Tone, string> = {
  primary: 'text-primary',
  info: 'text-[color:var(--superteam-info)]',
  success: 'text-[color:var(--superteam-success)]',
  warning: 'text-[color:var(--superteam-warning)]',
  danger: 'text-[color:var(--superteam-danger)]',
  artifact: 'text-[color:var(--superteam-artifact)]',
  decision: 'text-[color:var(--superteam-decision)]',
  neutral: 'text-[color:var(--superteam-neutral)]',
}

const iconTileSizeClass = {
  sm: 'size-10 rounded-xl [&_svg]:size-4',
  default: 'size-12 [&_svg]:size-5',
  lg: 'size-14 [&_svg]:size-6',
} as const

function LiquidCard({ className, ...props }: ComponentProps<typeof Card>) {
  return (
    <Card
      {...props}
      data-slot='liquid-card'
      className={cn('rounded-[1.75rem]', className)}
    />
  )
}

function LiquidPill({
  className,
  ...props
}: ComponentProps<'span'>) {
  return (
    <span
      {...props}
      data-slot='liquid-pill'
      className={cn(
        'superteam-liquid-pill inline-flex items-center gap-2 px-3 py-1.5 text-sm font-medium text-foreground',
        className
      )}
    />
  )
}

type PrimaryLiquidButtonProps = Omit<ComponentProps<typeof Button>, 'variant'>

function PrimaryLiquidButton({
  className,
  ...props
}: PrimaryLiquidButtonProps) {
  return (
    <Button
      {...props}
      variant='default'
      className={cn('superteam-primary-action', className)}
    />
  )
}

type SemanticIconTileProps = ComponentProps<'span'> & {
  tone?: Tone
  size?: keyof typeof iconTileSizeClass
}

function SemanticIconTile({
  className,
  tone = 'primary',
  size = 'default',
  ...props
}: SemanticIconTileProps) {
  return (
    <span
      {...props}
      data-slot='semantic-icon-tile'
      className={cn(
        'superteam-icon-tile shrink-0',
        iconTileSizeClass[size],
        toneTextClass[tone],
        className
      )}
    />
  )
}

type StatusBadgeProps = ComponentProps<typeof Badge> & {
  tone?: Tone
  showDot?: boolean
}

function StatusBadge({
  className,
  children,
  tone = 'neutral',
  showDot = true,
  ...props
}: StatusBadgeProps) {
  return (
    <Badge
      {...props}
      data-slot='status-badge'
      variant='outline'
      className={cn(
        'rounded-full border-current/20 bg-white/55 px-2.5 py-1 text-xs shadow-none backdrop-blur-sm',
        toneTextClass[tone],
        className
      )}
    >
      {showDot ? (
        <span aria-hidden='true' className='size-1.5 rounded-full bg-current' />
      ) : null}
      {children}
    </Badge>
  )
}

type MetricCardProps = Omit<ComponentProps<typeof Card>, 'children'> & {
  title: string
  description?: string
  icon?: ReactNode
  iconTone?: Tone
  value: ReactNode
  meta?: ReactNode
  statusTone?: Tone
  isError?: boolean
  children?: ReactNode
}

function MetricCard({
  className,
  title,
  description,
  icon,
  iconTone = 'primary',
  value,
  meta,
  statusTone = 'neutral',
  isError,
  children,
  ...props
}: MetricCardProps) {
  return (
    <Card
      {...props}
      data-slot='metric-card'
      className={cn('superteam-metric-card rounded-[1.75rem]', className)}
    >
      <CardHeader>
        <CardTitle className='text-xl tracking-normal'>{title}</CardTitle>
        {description ? <CardDescription>{description}</CardDescription> : null}
        {icon ? (
          <CardAction>
            <SemanticIconTile tone={iconTone} size='lg'>
              {icon}
            </SemanticIconTile>
          </CardAction>
        ) : null}
      </CardHeader>
      <CardContent className='flex flex-col gap-5'>
        <div className='flex flex-wrap items-baseline gap-3'>
          <span
            className={cn(
              'text-5xl font-bold tracking-normal text-foreground',
              isError && 'text-destructive'
            )}
          >
            {value}
          </span>
          {meta ? <StatusBadge tone={statusTone}>{meta}</StatusBadge> : null}
        </div>
        {children}
      </CardContent>
    </Card>
  )
}

export {
  LiquidCard,
  LiquidPill,
  MetricCard,
  PrimaryLiquidButton,
  SemanticIconTile,
  StatusBadge,
}
export type { MetricCardProps, SemanticIconTileProps, StatusBadgeProps, Tone }

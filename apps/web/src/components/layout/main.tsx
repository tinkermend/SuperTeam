import { cn } from '@/lib/utils'

export type MainWidth = 'contained' | 'wide' | 'canvas'

type MainProps = React.HTMLAttributes<HTMLElement> & {
  fixed?: boolean
  contained?: boolean
  fluid?: boolean
  /**
   * 页面宽度档位（布局宪法，见 docs/design-system/layout-density.md）：
   * contained ≈1280 表单/设置/单对象详情；wide ≈1680 主从工作台；
   * canvas 不限宽（仅图形/拓扑画布）。未传时保持全宽（存量页面待迁移）。
   */
  width?: MainWidth
  ref?: React.Ref<HTMLElement>
}

export function Main({ fixed, className, contained, fluid, width, ...props }: MainProps) {
  const resolvedWidth: MainWidth | undefined =
    width ?? ((contained ?? fluid === false) ? 'contained' : undefined)

  return (
    <main
      data-layout={fixed ? 'fixed' : 'auto'}
      data-width={resolvedWidth}
      className={cn(
        'w-full px-4 py-6',

        // If layout is fixed, make the main container flex and grow
        fixed && 'flex grow flex-col overflow-hidden',

        // Most console pages use the full content area. Narrow pages opt in.
        resolvedWidth === 'contained' &&
          '@7xl/content:mx-auto @7xl/content:max-w-(--v3-layout-contained)',
        resolvedWidth === 'wide' && 'mx-auto max-w-(--v3-layout-wide)',
        className
      )}
      {...props}
    />
  )
}

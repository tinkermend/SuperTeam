import { cn } from '@/lib/utils'

export type MainWidth = 'contained' | 'wide' | 'canvas'

type MainProps = React.HTMLAttributes<HTMLElement> & {
  fixed?: boolean
  /** @deprecated 改用 width="contained"。 */
  contained?: boolean
  /** @deprecated 改用 width="canvas"。 */
  fluid?: boolean
  /**
   * 页面宽度档位（布局宪法，见 docs/design-system/layout-density.md）：
   * contained ≈1280 表单/设置/单对象详情；wide ≈1680 主从工作台/密集表格；
   * canvas 不限宽（图形/拓扑类画布、多面板驾驶舱型工作台）。
   * 未传时默认 contained——安全默认，工作台页必须显式选 wide/canvas。
   */
  width?: MainWidth
  ref?: React.Ref<HTMLElement>
}

export function Main({ fixed, className, contained, fluid, width, ...props }: MainProps) {
  const legacyWidth: MainWidth | undefined = contained
    ? 'contained'
    : fluid
      ? 'canvas'
      : undefined
  const resolvedWidth: MainWidth = width ?? legacyWidth ?? 'contained'

  return (
    <main
      data-layout={fixed ? 'fixed' : 'auto'}
      data-width={resolvedWidth}
      className={cn(
        'w-full px-4 py-6',

        // If layout is fixed, make the main container flex and grow
        fixed && 'flex grow flex-col overflow-hidden',

        resolvedWidth === 'contained' &&
          '@7xl/content:mx-auto @7xl/content:max-w-(--layout-contained)',
        resolvedWidth === 'wide' && 'mx-auto max-w-(--layout-wide)',
        className
      )}
      {...props}
    />
  )
}

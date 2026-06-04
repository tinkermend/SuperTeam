import { cn } from '@/lib/utils'

type MainProps = React.HTMLAttributes<HTMLElement> & {
  fixed?: boolean
  contained?: boolean
  fluid?: boolean
  ref?: React.Ref<HTMLElement>
}

export function Main({ fixed, className, contained, fluid, ...props }: MainProps) {
  const isContained = contained ?? fluid === false

  return (
    <main
      data-layout={fixed ? 'fixed' : 'auto'}
      className={cn(
        'w-full px-4 py-6',

        // If layout is fixed, make the main container flex and grow
        fixed && 'flex grow flex-col overflow-hidden',

        // Most console pages use the full content area. Narrow pages opt in.
        isContained && '@7xl/content:mx-auto @7xl/content:max-w-7xl',
        className
      )}
      {...props}
    />
  )
}

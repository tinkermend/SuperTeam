import { useEffect, useState } from 'react'
import { cn } from '@/lib/utils'
import { Separator } from '@/components/ui/separator'
import { SidebarTrigger } from '@/components/ui/sidebar'

type HeaderProps = React.HTMLAttributes<HTMLElement> & {
  fixed?: boolean
  ref?: React.Ref<HTMLElement>
}

export function Header({ className, fixed, children, ...props }: HeaderProps) {
  const [offset, setOffset] = useState(0)

  useEffect(() => {
    const onScroll = () => {
      setOffset(document.body.scrollTop || document.documentElement.scrollTop)
    }

    // Add scroll listener to the body
    document.addEventListener('scroll', onScroll, { passive: true })

    // Clean up the event listener on unmount
    return () => document.removeEventListener('scroll', onScroll)
  }, [])

  return (
    <header
      data-slot='v3-shell-header'
      className={cn(
        'z-50 h-16 border-b border-v3-line bg-v3-card text-v3-ink shadow-v3',
        fixed && 'header-fixed peer/header sticky top-0 w-[inherit]',
        offset > 10 && fixed ? 'shadow-sm' : 'shadow-none',
        className
      )}
      {...props}
    >
      <div
        className={cn(
          'relative flex h-full items-center gap-3 px-4 py-3 sm:gap-4',
          offset > 10 &&
            fixed &&
            'after:absolute after:inset-0 after:-z-10 after:bg-v3-card/90 after:backdrop-blur-md'
        )}
      >
        <SidebarTrigger
          variant='ghost'
          className='rounded-xl border border-v3-line bg-v3-card-soft text-v3-ink-2 shadow-none hover:bg-v3-brand-soft hover:text-v3-brand-deep max-md:scale-125'
        />
        <Separator
          orientation='vertical'
          className='h-6 bg-v3-line-strong'
        />
        {children}
      </div>
    </header>
  )
}

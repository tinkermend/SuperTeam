import { useContext, useEffect, useState } from 'react'
import { Bell } from 'lucide-react'
import { Link } from '@tanstack/react-router'
import { cn, getDisplayNameInitials } from '@/lib/utils'
import { AuthContext } from '@/features/auth/auth-context'
import useDialogState from '@/hooks/use-dialog-state'
import { Avatar, AvatarFallback } from '@/components/ui/avatar'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { SidebarTrigger } from '@/components/ui/sidebar'
import { Search } from '@/components/search'
import { SignOutDialog } from '@/components/sign-out-dialog'
import { ThemeSwitch } from '@/components/theme-switch'

type HeaderProps = React.HTMLAttributes<HTMLElement> & {
  fixed?: boolean
  showSidebarTrigger?: boolean
  ref?: React.Ref<HTMLElement>
}

export function Header({
  className,
  fixed,
  showSidebarTrigger = true,
  children,
  ...props
}: HeaderProps) {
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
      data-variant='global'
      className={cn(
        'z-50 h-14 text-v3-ink',
        fixed && 'header-fixed peer/header sticky top-0 w-[inherit]',
        className
      )}
      {...props}
    >
      <div
        className={cn(
          showSidebarTrigger
            ? 'relative grid h-full grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3 px-4 py-2 sm:px-5 lg:grid-cols-[minmax(14rem,1fr)_minmax(16rem,42rem)_minmax(14rem,1fr)]'
            : 'relative grid h-full grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-3 px-4 py-2 sm:px-5 md:grid-cols-[minmax(12rem,1fr)_minmax(12rem,20rem)_minmax(12rem,1fr)]',
          offset > 10 &&
            fixed &&
            'after:absolute after:inset-0 after:-z-10 after:border-b after:border-[var(--v3-shell-glass-border)] after:bg-[var(--v3-shell-glass)] after:backdrop-blur-md'
        )}
      >
        {showSidebarTrigger ? (
          <SidebarTrigger
            variant='ghost'
            className='justify-self-start rounded-xl border border-[var(--v3-shell-control-border)] bg-[var(--v3-shell-control)] text-v3-ink-2 shadow-none backdrop-blur-md hover:bg-[var(--v3-shell-control-hover)] hover:text-v3-brand-deep max-md:scale-125'
          />
        ) : (
          <div className='min-w-0 w-full'>{children}</div>
        )}
        <Search
          className={cn(
            'h-10 w-full rounded-full border-[var(--v3-shell-search-border)] bg-[var(--v3-shell-search)] px-4 [box-shadow:var(--v3-shell-search-shadow)] backdrop-blur-xl',
            showSidebarTrigger
              ? 'mx-auto max-w-2xl sm:w-full md:w-full lg:w-full xl:w-full'
              : 'max-w-sm justify-self-center md:w-64 lg:w-72 xl:w-80 max-md:size-10 max-md:flex-none max-md:justify-center max-md:px-0 max-md:[&>span]:sr-only max-md:[&_svg]:left-1/2 max-md:[&_svg]:-translate-x-1/2'
          )}
        />
        <div className='flex min-w-0 items-center justify-end gap-2'>
          <div className='inline-flex shrink-0'>
            <ThemeSwitch />
          </div>
          <NotificationButton />
          <HeaderUserMenu />
        </div>
      </div>
    </header>
  )
}

function NotificationButton() {
  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger asChild>
        <button
          type='button'
          aria-label='通知'
          className='relative inline-flex size-10 shrink-0 items-center justify-center rounded-xl border border-[var(--v3-shell-control-border)] bg-[var(--v3-shell-control)] text-v3-ink-2 shadow-none backdrop-blur-md transition-colors hover:bg-[var(--v3-shell-control-hover)] hover:text-v3-brand-deep focus-visible:ring-2 focus-visible:ring-v3-brand/30 focus-visible:outline-none'
        >
          <Bell className='size-[1.15rem]' aria-hidden='true' />
          <span className='absolute top-1.5 right-1.5 size-2 rounded-full border border-white/80 bg-v3-artifact' />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align='end'
        className='min-w-56 rounded-v3-inner border-v3-line bg-v3-card text-v3-ink shadow-v3-pop'
      >
        <DropdownMenuLabel className='px-2 py-1.5 text-xs font-semibold text-v3-ink-2'>
          通知
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem disabled className='text-v3-ink-2'>
          暂无新通知
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function HeaderUserMenu() {
  const auth = useContext(AuthContext)
  const user = auth?.user
  const [open, setOpen] = useDialogState()
  const displayName = user?.display_name || user?.username || '未登录'
  const displayEmail =
    user?.email || user?.username || (user?.status === 'active' ? 'active' : 'disabled')
  const fallback = getDisplayNameInitials(displayName)

  return (
    <>
      <DropdownMenu modal={false}>
        <DropdownMenuTrigger asChild>
          <button
            type='button'
            aria-label={`用户信息：${displayName}`}
            className='flex h-10 min-w-0 shrink-0 items-center gap-2 rounded-full border border-[var(--v3-shell-control-border)] bg-[var(--v3-shell-control)] py-1 pr-3 pl-1 text-v3-ink shadow-none backdrop-blur-md transition-colors hover:bg-[var(--v3-shell-control-hover)] focus-visible:ring-2 focus-visible:ring-v3-brand/30 focus-visible:outline-none max-md:pr-1'
          >
            <Avatar className='size-8 rounded-full'>
              <AvatarFallback className='rounded-full bg-v3-brand text-xs font-semibold text-white'>
                {fallback}
              </AvatarFallback>
            </Avatar>
            <span className='min-w-0 max-w-28 truncate text-sm font-semibold max-md:hidden'>
              {displayName}
            </span>
          </button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align='end'
          className='min-w-56 rounded-v3-inner border-v3-line bg-v3-card text-v3-ink shadow-v3-pop'
        >
          <DropdownMenuLabel className='p-0 font-normal'>
            <div className='flex items-center gap-2 px-1 py-1.5 text-start text-sm'>
              <Avatar className='size-8 rounded-lg'>
                <AvatarFallback className='rounded-lg bg-v3-brand text-white'>
                  {fallback}
                </AvatarFallback>
              </Avatar>
              <div className='grid min-w-0 flex-1 text-start text-sm leading-tight'>
                <span className='truncate font-semibold'>{displayName}</span>
                <span className='truncate text-xs text-v3-ink-2'>{displayEmail}</span>
              </div>
            </div>
          </DropdownMenuLabel>
          <DropdownMenuSeparator />
          <DropdownMenuItem asChild>
            <Link to='/settings/account'>账户</Link>
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem variant='destructive' onSelect={() => setTimeout(() => setOpen(true), 0)}>
            退出登录
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      {auth ? <SignOutDialog open={!!open} onOpenChange={setOpen} /> : null}
    </>
  )
}

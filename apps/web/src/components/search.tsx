import { SearchIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useSearch } from '@/context/search-provider'
import { Button } from './ui/button'

export function Search({
  className = '',
  placeholder = '搜索任务、数字员工、能力、文档或快捷命令...',
  ...props
}: React.ComponentProps<'button'> & { placeholder?: string }) {
  const { setOpen } = useSearch()
  return (
    <Button
      {...props}
      variant='outline'
      className={cn(
        'superteam-search-glass group relative h-9 w-full flex-1 justify-start rounded-lg text-sm font-normal text-muted-foreground shadow-none hover:text-foreground focus-visible:ring-primary/30 sm:w-48 sm:pe-12 md:flex-none lg:w-72 xl:w-96',
        className
      )}
      aria-keyshortcuts='Meta+K Control+K'
      onClick={() => setOpen(true)}
    >
      <SearchIcon
        aria-hidden='true'
        className='absolute inset-s-1.5 top-1/2 -translate-y-1/2'
        size={16}
      />
      <span className='ms-4 truncate'>{placeholder}</span>
      <kbd className='pointer-events-none absolute inset-e-[0.35rem] top-[0.35rem] hidden h-5 items-center gap-1 rounded-md border border-[color:var(--superteam-glass-border)] bg-[color:var(--superteam-primary-soft)] px-1.5 font-mono text-[10px] font-medium text-primary opacity-100 select-none sm:flex'>
        <span className='text-xs'>⌘</span>K
      </kbd>
    </Button>
  )
}

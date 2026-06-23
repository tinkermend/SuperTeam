import { SearchIcon } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useSearch } from '@/context/search-provider'
import { V3Button } from './superteam'

export function Search({
  className = '',
  placeholder = '搜索任务、数字员工、能力、文档或快捷命令...',
  ...props
}: React.ComponentProps<'button'> & { placeholder?: string }) {
  const { setOpen } = useSearch()
  return (
    <V3Button
      {...props}
      variant='outline'
      className={cn(
        'group relative h-9 min-w-0 flex-1 justify-start rounded-xl border-v3-line-strong bg-v3-card text-sm font-normal text-v3-ink-2 shadow-v3 hover:bg-v3-card-soft hover:text-v3-ink focus-visible:ring-v3-brand/30 sm:w-48 sm:pe-12 md:flex-none lg:w-72 xl:w-96',
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
      <span className='ms-4 min-w-0 truncate'>{placeholder}</span>
      <kbd className='pointer-events-none absolute inset-e-[0.35rem] top-[0.35rem] hidden h-5 items-center gap-1 rounded-md border border-v3-line bg-v3-brand-soft px-1.5 font-mono text-[10px] font-medium text-v3-brand-deep opacity-100 select-none sm:flex'>
        <span className='text-xs'>⌘</span>K
      </kbd>
    </V3Button>
  )
}

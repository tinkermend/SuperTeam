import * as React from 'react'
import { cn } from '@/lib/utils'

function Textarea({ className, ...props }: React.ComponentProps<'textarea'>) {
  return (
    <textarea
      data-slot='textarea'
      className={cn(
        'flex field-sizing-content min-h-16 w-full rounded-[10px] border border-line-strong bg-card px-3 py-2 text-base text-ink shadow-none transition-[color,box-shadow] outline-none placeholder:text-ink-3 focus-visible:border-brand focus-visible:ring-2 focus-visible:ring-brand/25 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 md:text-sm',
        className
      )}
      {...props}
    />
  )
}

export { Textarea }

import * as React from 'react'
import { Eye, EyeOff } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from './ui/button'

type PasswordInputProps = Omit<
  React.InputHTMLAttributes<HTMLInputElement>,
  'type'
> & {
  ref?: React.Ref<HTMLInputElement>
}

export function PasswordInput({
  className,
  disabled,
  ref,
  ...props
}: PasswordInputProps) {
  const [showPassword, setShowPassword] = React.useState(false)

  return (
    <div className='relative rounded-full'>
      <input
        type={showPassword ? 'text' : 'password'}
        data-slot='input'
        className={cn(
          'flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 pe-9 text-sm shadow-xs transition-colors file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-muted-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-hidden disabled:cursor-not-allowed disabled:opacity-50',
          className
        )}
        ref={ref}
        disabled={disabled}
        {...props}
      />
      <Button
        type='button'
        size='icon'
        variant='ghost'
        disabled={disabled}
        className='absolute inset-e-2 top-1/2 size-7 -translate-y-1/2 rounded-full text-muted-foreground'
        onClick={() => setShowPassword((prev) => !prev)}
      >
        {showPassword ? <Eye /> : <EyeOff />}
        <span className='sr-only'>
          {showPassword ? 'Hide password' : 'Show password'}
        </span>
      </Button>
    </div>
  )
}

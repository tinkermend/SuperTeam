import { useState } from 'react'
import { z } from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useNavigate } from '@tanstack/react-router'
import { Loader2, LogIn } from 'lucide-react'
import { useAuth } from '@/features/auth/use-auth'
import { cn } from '@/lib/utils'
import { V3Button } from '@/components/superteam'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { PasswordInput } from '@/components/password-input'

const formSchema = z.object({
  username: z.string().min(1, '请输入用户名。'),
  password: z.string().min(1, '请输入密码。'),
})

interface UserAuthFormProps extends React.HTMLAttributes<HTMLFormElement> {
  redirectTo?: string
}

export function UserAuthForm({
  className,
  redirectTo,
  ...props
}: UserAuthFormProps) {
  const [isLoading, setIsLoading] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)
  const navigate = useNavigate()
  const { login } = useAuth()

  const form = useForm<z.infer<typeof formSchema>>({
    resolver: zodResolver(formSchema),
    defaultValues: {
      username: '',
      password: '',
    },
  })

  async function onSubmit(data: z.infer<typeof formSchema>) {
    setIsLoading(true)
    setFormError(null)

    try {
      await login({ username: data.username, password: data.password })
      navigate({ to: redirectTo || '/', replace: true })
    } catch {
      setFormError('用户名或密码不正确')
    } finally {
      setIsLoading(false)
    }
  }

  return (
    <Form {...form}>
      <form
        onSubmit={form.handleSubmit(onSubmit)}
        className={cn('flex flex-col gap-4', className)}
        {...props}
      >
        <FormField
          control={form.control}
          name='username'
          render={({ field }) => (
            <FormItem>
              <FormLabel className='text-v3-ink-2'>账号</FormLabel>
              <FormControl>
                <Input
                  className='h-12 rounded-xl border-v3-line-strong bg-v3-card-soft px-4 text-v3-ink shadow-none placeholder:text-v3-ink-3 focus-visible:border-v3-brand focus-visible:ring-v3-brand/20'
                  placeholder='请输入账号'
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        <FormField
          control={form.control}
          name='password'
          render={({ field }) => (
            <FormItem>
              <FormLabel className='text-v3-ink-2'>密码</FormLabel>
              <FormControl>
                <PasswordInput
                  className='h-12 rounded-xl border-v3-line-strong bg-v3-card-soft px-4 pe-11 text-v3-ink shadow-none placeholder:text-v3-ink-3 focus-visible:border-v3-brand focus-visible:ring-v3-brand/20'
                  placeholder='请输入密码'
                  {...field}
                />
              </FormControl>
              <FormMessage />
            </FormItem>
          )}
        />
        {formError ? (
          <p
            className='rounded-xl bg-v3-danger-soft px-3 py-2 text-sm font-bold text-v3-danger'
            role='alert'
          >
            {formError}
          </p>
        ) : null}
        <V3Button className='mt-1 h-12 text-base' disabled={isLoading} type='submit'>
          {isLoading ? (
            <Loader2 className='animate-spin' data-icon='inline-start' />
          ) : (
            <LogIn data-icon='inline-start' />
          )}
          登录
        </V3Button>
      </form>
    </Form>
  )
}

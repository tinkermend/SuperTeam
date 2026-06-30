import { useCallback, useEffect, useRef, useState } from 'react'
import { z } from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useNavigate } from '@tanstack/react-router'
import { Loader2, LogIn, RefreshCw } from 'lucide-react'
import { useAuth } from '@/features/auth/use-auth'
import {
  ApiRequestError,
  getLoginCaptcha,
  type CaptchaChallengeResponse,
} from '@/lib/api'
import { cn } from '@/lib/utils'
import { V3Button, V3IconButton } from '@/components/superteam'
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

const baseFormSchema = z.object({
  username: z.string().min(1, '请输入用户名。'),
  password: z.string().min(1, '请输入密码。'),
  captcha_code: z.string(),
})

type LoginFormValues = z.infer<typeof baseFormSchema>

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
  const [captcha, setCaptcha] = useState<CaptchaChallengeResponse | null>(null)
  const [isCaptchaLoading, setIsCaptchaLoading] = useState(false)
  const [captchaError, setCaptchaError] = useState<string | null>(null)
  const captchaRequestIdRef = useRef(0)
  const submitInFlightRef = useRef(false)
  const navigate = useNavigate()
  const { apiBaseUrl, login } = useAuth()
  const isCaptchaEnabled = captcha?.enabled !== false

  const form = useForm<LoginFormValues>({
    resolver: zodResolver(baseFormSchema),
    defaultValues: {
      username: '',
      password: '',
      captcha_code: '',
    },
  })

  const refreshCaptcha = useCallback(
    async (options?: { clearInput?: boolean }) => {
      const clearInput = options?.clearInput ?? true
      const requestId = captchaRequestIdRef.current + 1
      captchaRequestIdRef.current = requestId

      setIsCaptchaLoading(true)
      setCaptchaError(null)

      try {
        const nextCaptcha = await getLoginCaptcha({ baseUrl: apiBaseUrl })
        if (captchaRequestIdRef.current !== requestId) {
          return
        }

        setCaptcha(nextCaptcha)
        if (!nextCaptcha.enabled || clearInput) {
          form.setValue('captcha_code', '', {
            shouldDirty: false,
            shouldTouch: false,
            shouldValidate: false,
          })
        }
      } catch {
        if (captchaRequestIdRef.current !== requestId) {
          return
        }

        setCaptcha(null)
        setCaptchaError('验证码加载失败，请刷新重试')
      } finally {
        if (captchaRequestIdRef.current === requestId) {
          setIsCaptchaLoading(false)
        }
      }
    },
    [apiBaseUrl, form]
  )

  useEffect(() => {
    void refreshCaptcha({ clearInput: false })
  }, [refreshCaptcha])

  async function onSubmit(data: LoginFormValues) {
    if (submitInFlightRef.current) {
      return
    }
    if (!captcha && isCaptchaEnabled) {
      setCaptchaError('验证码加载失败，请刷新重试')
      return
    }
    if (isCaptchaEnabled) {
      const captchaCode = data.captcha_code.trim()
      if (captchaCode === '') {
        form.setError('captcha_code', { message: '请输入图形验证码。' })
        return
      }
      if (captchaCode.length !== 4) {
        form.setError('captcha_code', { message: '请输入 4 位图形验证码。' })
        return
      }
    }

    submitInFlightRef.current = true
    setIsLoading(true)
    setFormError(null)

    try {
      const credentials = {
        username: data.username,
        password: data.password,
        ...(captcha?.enabled
          ? {
              captcha_id: captcha.captcha_id,
              captcha_code: data.captcha_code.toUpperCase(),
            }
          : {}),
      }
      await login(credentials)
      navigate({ to: redirectTo || '/', replace: true })
    } catch (error) {
      setFormError(loginErrorMessage(error))
      if (isCaptchaEnabled) {
        void refreshCaptcha()
      }
    } finally {
      submitInFlightRef.current = false
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
        {isCaptchaEnabled ? (
          <FormField
            control={form.control}
            name='captcha_code'
            render={({ field }) => (
              <FormItem>
                <FormLabel className='text-v3-ink-2'>图形验证码</FormLabel>
                <div className='flex min-w-0 items-center gap-2'>
                  <FormControl>
                    <Input
                      className='h-12 min-w-0 flex-1 rounded-xl border-v3-line-strong bg-v3-card-soft px-4 text-v3-ink shadow-none placeholder:text-v3-ink-3 focus-visible:border-v3-brand focus-visible:ring-v3-brand/20'
                      maxLength={4}
                      {...field}
                      onChange={(event) =>
                        field.onChange(event.target.value.toUpperCase())
                      }
                    />
                  </FormControl>
                  <div className='flex h-12 w-32 shrink-0 items-center justify-center overflow-hidden rounded-xl border border-v3-line bg-v3-card-soft'>
                    {captcha?.enabled ? (
                      <img
                        alt='图形验证码'
                        className='h-full w-full object-contain'
                        src={captcha.image_data_url}
                      />
                    ) : isCaptchaLoading ? (
                      <Loader2
                        className='size-5 animate-spin text-v3-ink-3'
                        data-testid='captcha-loading'
                      />
                    ) : (
                      <span
                        className='text-xs font-semibold text-v3-ink-3'
                        data-testid='captcha-placeholder'
                      >
                        加载失败
                      </span>
                    )}
                  </div>
                  <V3IconButton
                    aria-label='刷新验证码'
                    className='size-12 shrink-0 disabled:pointer-events-none disabled:opacity-55'
                    disabled={isCaptchaLoading}
                    onClick={() => void refreshCaptcha()}
                    type='button'
                  >
                    <RefreshCw
                      className={cn(
                        'size-5',
                        isCaptchaLoading && 'animate-spin'
                      )}
                    />
                  </V3IconButton>
                </div>
                <FormMessage />
                {captchaError ? (
                  <p className='text-sm font-medium text-v3-danger' role='alert'>
                    {captchaError}
                  </p>
                ) : null}
              </FormItem>
            )}
          />
        ) : null}
        {formError ? (
          <p
            className='rounded-xl bg-v3-danger-soft px-3 py-2 text-sm font-bold text-v3-danger'
            role='alert'
          >
            {formError}
          </p>
        ) : null}
        <V3Button
          className='mt-1 h-12 text-base'
          disabled={isLoading || isCaptchaLoading || (!captcha && isCaptchaEnabled)}
          type='submit'
        >
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

function loginErrorMessage(error: unknown) {
  if (
    error instanceof ApiRequestError &&
    error.message.includes('验证码不正确或已过期')
  ) {
    return '验证码不正确或已过期'
  }
  return '用户名或密码不正确'
}

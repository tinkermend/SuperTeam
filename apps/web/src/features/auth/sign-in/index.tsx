import { useSearch } from '@tanstack/react-router'
import { SoftCard } from '@/components/superteam'
import { AuthLayout } from '../auth-layout'
import { UserAuthForm } from './components/user-auth-form'

export function SignIn() {
  const { redirect } = useSearch({ from: '/(auth)/login' })

  return (
    <AuthLayout>
      <SoftCard className='w-full p-7'>
        <div className='mb-5 text-center'>
          <h1 className='text-2xl font-extrabold tracking-tight text-v3-ink'>
            账号登录
          </h1>
        </div>
        <UserAuthForm redirectTo={redirect} />
      </SoftCard>
    </AuthLayout>
  )
}

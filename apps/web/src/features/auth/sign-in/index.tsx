import { useSearch } from '@tanstack/react-router'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { AuthLayout } from '../auth-layout'
import { UserAuthForm } from './components/user-auth-form'

export function SignIn() {
  const { redirect } = useSearch({ from: '/(auth)/login' })

  return (
    <AuthLayout>
      <Card className='superteam-auth-card w-full gap-5 rounded-xl border-white/80 bg-white/82 shadow-2xl shadow-slate-950/8 backdrop-blur-xl'>
        <CardHeader className='items-center text-center'>
          <CardTitle className='text-2xl tracking-normal'>账号登录</CardTitle>
          <CardDescription className='text-sm'>
            使用 Control Plane 账号进入 SuperTeam 控制台。
          </CardDescription>
        </CardHeader>
        <CardContent>
          <UserAuthForm redirectTo={redirect} />
        </CardContent>
      </Card>
    </AuthLayout>
  )
}

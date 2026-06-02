import { Network } from 'lucide-react'

type AuthLayoutProps = {
  children: React.ReactNode
}

export function AuthLayout({ children }: AuthLayoutProps) {
  return (
    <div className='superteam-auth-shell relative grid min-h-svh place-items-center overflow-hidden px-4 py-10'>
      <div className='relative z-10 flex w-full max-w-[27rem] flex-col items-center'>
        <div className='mb-6 flex flex-col items-center text-center'>
          <div className='superteam-auth-mark mb-3' aria-hidden='true'>
            <span className='text-[2rem] font-black leading-none text-primary'>
              ST
            </span>
            <Network className='absolute -right-2 -top-2 text-primary' />
          </div>
          <h1 className='text-4xl font-bold tracking-normal text-foreground'>
            SuperTeam
          </h1>
          <p className='mt-2 rounded-full bg-[color:var(--superteam-primary-soft)] px-3 py-1 text-sm font-semibold text-primary'>
            企业级数字员工控制平面
          </p>
        </div>
        {children}
      </div>
    </div>
  )
}

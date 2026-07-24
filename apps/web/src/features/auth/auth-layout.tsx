type AuthLayoutProps = {
  children: React.ReactNode
}

const LOGIN_DISPLAY_SRC = '/images/brand/jushu-platform-logo.png'

export function AuthLayout({ children }: AuthLayoutProps) {
  return (
    <div
      data-slot='auth-shell'
      className='relative grid min-h-svh place-items-center overflow-x-hidden overflow-y-auto bg-background px-4 py-6 text-ink sm:py-10'
    >
      <div className='relative z-10 flex w-full max-w-[27rem] flex-col items-center'>
        <div className='mb-0 flex flex-col items-center text-center'>
          <img
            src={LOGIN_DISPLAY_SRC}
            alt='炬枢平台 - 新炬网络'
            className='h-auto max-h-[54svh] w-[22rem] max-w-[88vw] object-contain opacity-100 drop-shadow-none contrast-[1.08] saturate-[1.08] dark:brightness-[1.24] dark:contrast-[1.08] dark:saturate-[1.08] sm:w-[24rem]'
          />
        </div>
        {children}
      </div>
    </div>
  )
}

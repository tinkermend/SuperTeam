type AuthLayoutProps = {
  children: React.ReactNode
}

const LOGIN_BANNER_SRC = '/images/brand/jushu-platform-header.png'
const LOGIN_DISPLAY_SRC = '/images/brand/jushu-platform-logo.png'

export function AuthLayout({ children }: AuthLayoutProps) {
  return (
    <div className='superteam-auth-shell relative grid min-h-svh place-items-center overflow-hidden px-4 py-10'>
      <img
        src={LOGIN_BANNER_SRC}
        alt='炬枢平台 - 新炬网络横幅'
        className='pointer-events-none absolute right-6 top-5 z-10 hidden h-12 w-auto max-w-[38vw] object-contain opacity-100 contrast-[1.08] saturate-[1.08] drop-shadow-[0_14px_30px_rgba(15,118,110,0.22)] dark:brightness-[1.22] dark:contrast-[1.12] dark:saturate-[1.12] sm:block lg:right-10 lg:top-7 lg:h-14'
      />
      <div className='relative z-10 flex w-full max-w-[27rem] flex-col items-center'>
        <div className='relative mb-5 flex flex-col items-center text-center before:absolute before:inset-[-1.5rem] before:-z-10 before:rounded-full before:bg-[radial-gradient(circle,rgba(255,255,255,0.72)_0%,rgba(245,255,252,0.42)_46%,rgba(245,255,252,0)_72%)] before:content-[""] dark:before:bg-[radial-gradient(circle,rgba(14,165,153,0.22)_0%,rgba(15,23,42,0.18)_46%,rgba(15,23,42,0)_72%)]'>
          <img
            src={LOGIN_DISPLAY_SRC}
            alt='炬枢平台 - 新炬网络'
            className='h-auto w-[16.25rem] max-w-[76vw] object-contain opacity-100 contrast-[1.08] saturate-[1.08] drop-shadow-[0_18px_44px_rgba(15,118,110,0.24)] dark:brightness-[1.22] dark:contrast-[1.12] dark:saturate-[1.12] sm:w-[17.5rem]'
          />
        </div>
        {children}
      </div>
    </div>
  )
}

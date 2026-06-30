import { useNavigate, useLocation } from '@tanstack/react-router'
import { useAuth } from '@/features/auth/use-auth'
import { ConfirmDialog } from '@/components/confirm-dialog'

interface SignOutDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function SignOutDialog({ open, onOpenChange }: SignOutDialogProps) {
  const navigate = useNavigate()
  const location = useLocation()
  const { logout } = useAuth()

  const handleSignOut = async () => {
    await logout()
    // Preserve current location for redirect after sign-in
    const currentPath = location.href
    navigate({
      to: '/login',
      search: { redirect: currentPath },
      replace: true,
    })
  }

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title='退出登录'
      desc='确定要退出登录吗？您需要重新登录才能继续访问。'
      confirmText='退出登录'
      destructive
      handleConfirm={handleSignOut}
      className='sm:max-w-sm'
    />
  )
}

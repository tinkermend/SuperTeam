import * as AlertDialogPrimitive from '@radix-ui/react-alert-dialog'
import { cn } from '@/lib/utils'
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { V3Button } from '@/components/superteam'

type ConfirmDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  title: React.ReactNode
  disabled?: boolean
  desc: React.JSX.Element | string
  cancelBtnText?: string
  confirmText?: React.ReactNode
  destructive?: boolean
  isLoading?: boolean
  className?: string
  children?: React.ReactNode
} & (
  | { form: string; handleConfirm?: undefined }
  | { form?: undefined; handleConfirm: () => void }
)

export function ConfirmDialog(props: ConfirmDialogProps) {
  const {
    title,
    desc,
    children,
    className,
    confirmText,
    cancelBtnText,
    destructive,
    isLoading,
    disabled = false,
    form,
    handleConfirm,
    ...actions
  } = props
  return (
    <AlertDialog {...actions}>
      <AlertDialogContent className={cn('bg-v3-card border border-v3-line shadow-v3-pop rounded-v3-card sm:rounded-[22px] p-6', className)}>
        <AlertDialogHeader className='text-start'>
          <AlertDialogTitle className="text-xl font-extrabold tracking-tight text-v3-ink">{title}</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div className="text-[15px] text-v3-ink-2">{desc}</div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        {children}
        <AlertDialogFooter className="mt-6 flex flex-col-reverse gap-3 sm:flex-row sm:justify-end sm:gap-2">
          <AlertDialogPrimitive.Cancel asChild>
            <V3Button variant="outline" disabled={isLoading}>
              {cancelBtnText ?? '取消'}
            </V3Button>
          </AlertDialogPrimitive.Cancel>
          <V3Button
            type={form ? 'submit' : 'button'}
            form={form}
            onClick={handleConfirm}
            variant={destructive ? 'danger' : 'primary'}
            disabled={disabled || isLoading}
          >
            {confirmText ?? '继续'}
          </V3Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

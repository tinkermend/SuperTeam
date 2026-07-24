import * as AlertDialogPrimitive from '@radix-ui/react-alert-dialog'
import { cn } from '@/lib/utils'
import {
  AlertDialog,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle
} from '@/components/ui/alert-dialog'
import { Button } from '@/components/superteam'

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
      <AlertDialogContent className={cn('bg-card border border-line shadow-pop rounded-card sm:rounded-[22px] p-6', className)}>
        <AlertDialogHeader className='text-start'>
          <AlertDialogTitle className="text-xl font-extrabold tracking-tight text-ink">{title}</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div className="text-[15px] text-ink-2">{desc}</div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        {children}
        <AlertDialogFooter className="mt-6 flex flex-col-reverse gap-3 sm:flex-row sm:justify-end sm:gap-2">
          <AlertDialogPrimitive.Cancel asChild>
            <Button variant="outline" disabled={isLoading}>
              {cancelBtnText ?? '取消'}
            </Button>
          </AlertDialogPrimitive.Cancel>
          <Button
            type={form ? 'submit' : 'button'}
            form={form}
            onClick={handleConfirm}
            variant={destructive ? 'danger' : 'primary'}
            disabled={disabled || isLoading}
          >
            {confirmText ?? '继续'}
          </Button>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

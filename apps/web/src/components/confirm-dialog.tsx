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

/**
 * 一站式确认框（危险/普通）。保留 AlertDialog 语义（role=alertdialog）。
 * 视觉与 SoftDialog 对齐（rounded-card / shadow-pop / 底栏节奏）。
 * 自定义结构弹窗请用 SoftDialog* / SoftSheet*，不要在此扩展业务字段。
 */
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
      <AlertDialogContent
        className={cn(
          'gap-0 overflow-hidden rounded-card border border-line bg-card p-0 text-ink shadow-pop sm:max-w-md sm:rounded-card',
          className,
        )}
      >
        <AlertDialogHeader className="gap-1.5 border-b border-line px-6 py-5 text-start">
          <AlertDialogTitle className="text-xl font-extrabold tracking-tight text-ink">
            {title}
          </AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div className="text-[15px] leading-snug text-ink-2">{desc}</div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        {children ? (
          <div data-slot="confirm-dialog-body" className="px-6 py-4">
            {children}
          </div>
        ) : null}
        <AlertDialogFooter
          className={cn(
            'flex flex-col-reverse gap-3 border-t border-line bg-card-soft/60 px-6 py-4 sm:flex-row sm:justify-end sm:gap-2',
            !children && 'border-t-0 bg-transparent',
          )}
        >
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

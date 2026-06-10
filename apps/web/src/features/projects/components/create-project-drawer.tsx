import { useEffect, useState, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Textarea } from "@/components/ui/textarea";
import type { CreateProjectInput } from "@/lib/api/projects";

type CreateProjectDrawerProps = {
  isSubmitting?: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (input: CreateProjectInput) => void;
  open: boolean;
  submitError?: string;
};

const emptyDraft: CreateProjectInput = {
  goal: "",
  human_owner_user_id: "",
  name: "",
};

export function CreateProjectDrawer({
  isSubmitting,
  onOpenChange,
  onSubmit,
  open,
  submitError,
}: CreateProjectDrawerProps) {
  const [draft, setDraft] = useState<CreateProjectInput>(emptyDraft);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) {
      setDraft(emptyDraft);
      setError("");
    }
  }, [open]);

  function submit() {
    if (!draft.name.trim() || !draft.goal.trim() || !draft.human_owner_user_id.trim()) {
      setError("项目名称、目标和人类 Owner 不能为空");
      return;
    }
    setError("");
    onSubmit({
      ...draft,
      goal: draft.goal.trim(),
      human_owner_user_id: draft.human_owner_user_id.trim(),
      name: draft.name.trim(),
    });
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="flex w-full flex-col gap-0 p-0 sm:max-w-[620px]">
        <SheetHeader className="border-b px-6 py-5">
          <SheetTitle>创建项目</SheetTitle>
          <SheetDescription>
            建立项目事实容器，并注册虚拟协调线程。
          </SheetDescription>
        </SheetHeader>
        <div className="grid flex-1 content-start gap-5 overflow-y-auto p-6">
          <Field label="项目名称">
            <Input
              value={draft.name}
              onChange={(event) =>
                setDraft((current) => ({ ...current, name: event.target.value }))
              }
              placeholder="客户接入验收"
            />
          </Field>
          <Field label="目标">
            <Textarea
              value={draft.goal}
              onChange={(event) =>
                setDraft((current) => ({ ...current, goal: event.target.value }))
              }
              placeholder="说明项目要达成的闭环目标"
            />
          </Field>
          <Field label="人类 Owner 用户 ID">
            <Input
              value={draft.human_owner_user_id}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  human_owner_user_id: event.target.value,
                }))
              }
              placeholder="UUID"
            />
          </Field>
          <Field label="描述">
            <Textarea
              value={draft.description ?? ""}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  description: event.target.value,
                }))
              }
              placeholder="可选：背景、边界和验收说明"
            />
          </Field>
          {error || submitError ? (
            <p className="text-sm text-destructive">{error || submitError}</p>
          ) : null}
        </div>
        <div className="flex justify-end gap-2 border-t p-4">
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button disabled={isSubmitting} type="button" onClick={submit}>
            创建项目
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  );
}

function Field({ children, label }: { children: ReactNode; label: string }) {
  return (
    <Label className="grid gap-2">
      <span>{label}</span>
      {children}
    </Label>
  );
}

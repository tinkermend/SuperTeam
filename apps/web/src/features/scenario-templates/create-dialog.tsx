import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Button } from "@/components/superteam";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from "@/components/ui/dialog";
import { ApiRequestError } from "@/lib/api/client";
import { createScenarioTemplate } from "@/lib/api/scenario-templates";

/** Minimal valid v2 spec skeleton: structurally accepted by ParseSpec and
 * declares no required_capabilities, so it also clears vocabulary
 * validation untouched — a safe starting point to edit from. */
export const SCENARIO_TEMPLATE_SPEC_SKELETON = {
  spec_version: 2,
  roles: [{ key: "executor", title: "执行者" }],
  skeleton: [{ step: "execute", role: "executor" }],
  exits: [{ deliverable: "outcome", label: "完成" }],
  constraints: [],
  collapse_rules: [],
  default_acceptance_criteria: [{ statement: "工作按验收判据完成并留痕" }]
};

const SPEC_SKELETON_TEXT = JSON.stringify(SCENARIO_TEMPLATE_SPEC_SKELETON, null, 2);

type CreateScenarioTemplateDialogProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const EMPTY_FORM = {
  template_key: "",
  name: "",
  description: "",
  specText: SPEC_SKELETON_TEXT
};

const inputClass = "h-10 w-full rounded-md border bg-background px-3 text-sm";

function FieldLabel({ children, required }: { children: string; required?: boolean }) {
  return (
    <span className="text-sm text-muted-foreground">
      {children}
      {required ? <span className="ml-0.5 text-destructive">*</span> : null}
    </span>
  );
}

export function CreateScenarioTemplateDialog({
  apiBaseUrl,
  fetcher,
  open,
  onOpenChange
}: CreateScenarioTemplateDialogProps) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState(EMPTY_FORM);
  const [formError, setFormError] = useState<string | null>(null);

  const resetAndClose = () => {
    setForm(EMPTY_FORM);
    setFormError(null);
    onOpenChange(false);
  };

  const createMutation = useMutation({
    mutationFn: (spec: Record<string, unknown>) =>
      createScenarioTemplate(
        { baseUrl: apiBaseUrl, fetcher },
        {
          template_key: form.template_key.trim(),
          name: form.name.trim(),
          description: form.description.trim(),
          spec
},
      ),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["scenario-templates"] });
      resetAndClose();
    },
    onError: (error: unknown) => {
      setFormError(
        error instanceof ApiRequestError && error.detail
          ? error.detail
          : error instanceof Error
            ? error.message
            : "创建场景模板失败",
      );
    }
});

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          resetAndClose();
          return;
        }
        onOpenChange(next);
      }}
    >
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>新建场景模板</DialogTitle>
          <DialogDescription>
            定义场景的角色、分解骨架与默认验收判据（spec v2），保存后作为 v1 版本生效。
          </DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            setFormError(null);
            let spec: Record<string, unknown>;
            try {
              spec = JSON.parse(form.specText) as Record<string, unknown>;
            } catch {
              setFormError("spec 不是合法 JSON，请检查格式");
              return;
            }
            createMutation.mutate(spec);
          }}
        >
          <div className="grid gap-4 sm:grid-cols-2">
            <label className="flex flex-col gap-1">
              <FieldLabel required>template_key</FieldLabel>
              <input
                className={`${inputClass} font-mono`}
                placeholder="如 ops_review，仅字母数字_-"
                value={form.template_key}
                onChange={(event) =>
                  setForm((prev) => ({ ...prev, template_key: event.target.value }))
                }
                required
                pattern="[A-Za-z0-9_\-]+"
                title="仅允许字母、数字、下划线和连字符"
              />
            </label>
            <label className="flex flex-col gap-1">
              <FieldLabel required>名称</FieldLabel>
              <input
                className={inputClass}
                placeholder="如 运维评审"
                value={form.name}
                onChange={(event) => setForm((prev) => ({ ...prev, name: event.target.value }))}
                required
              />
            </label>
            <label className="flex flex-col gap-1 sm:col-span-2">
              <FieldLabel>描述</FieldLabel>
              <textarea
                className="min-h-16 w-full rounded-md border bg-background px-3 py-2 text-sm"
                placeholder="这个场景解决什么问题、适合哪些项目"
                value={form.description}
                onChange={(event) =>
                  setForm((prev) => ({ ...prev, description: event.target.value }))
                }
              />
            </label>
            <label className="flex flex-col gap-1 sm:col-span-2">
              <FieldLabel required>spec（JSON）</FieldLabel>
              <textarea
                className="min-h-64 w-full rounded-md border bg-background px-3 py-2 font-mono text-xs"
                value={form.specText}
                onChange={(event) =>
                  setForm((prev) => ({ ...prev, specText: event.target.value }))
                }
                spellCheck={false}
                required
              />
            </label>
          </div>
          {formError ? <p className="text-sm text-destructive">{formError}</p> : null}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={resetAndClose}>
              取消
            </Button>
            <Button type="submit" disabled={createMutation.isPending}>
              {createMutation.isPending ? "创建中…" : "创建"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

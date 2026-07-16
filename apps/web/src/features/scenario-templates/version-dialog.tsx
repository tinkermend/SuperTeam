import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { V3Button } from "@/components/superteam";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ApiRequestError } from "@/lib/api/client";
import {
  createScenarioTemplateVersion,
  type ScenarioTemplate,
} from "@/lib/api/scenario-templates";

type CreateScenarioTemplateVersionDialogProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  template: ScenarioTemplate | null;
  onOpenChange: (open: boolean) => void;
};

export function CreateScenarioTemplateVersionDialog({
  apiBaseUrl,
  fetcher,
  template,
  onOpenChange,
}: CreateScenarioTemplateVersionDialogProps) {
  const queryClient = useQueryClient();
  const [specText, setSpecText] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  useEffect(() => {
    if (template) {
      setSpecText(JSON.stringify(template.spec, null, 2));
      setFormError(null);
    }
  }, [template]);

  const close = () => {
    setFormError(null);
    onOpenChange(false);
  };

  const versionMutation = useMutation({
    mutationFn: (spec: Record<string, unknown>) => {
      if (!template) {
        return Promise.reject(new Error("没有选中的模板"));
      }
      return createScenarioTemplateVersion(
        { baseUrl: apiBaseUrl, fetcher },
        template.template_key,
        { spec },
      );
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["scenario-templates"] });
      if (template) {
        void queryClient.invalidateQueries({
          queryKey: ["scenario-template-versions", template.template_key],
        });
      }
      close();
    },
    onError: (error: unknown) => {
      setFormError(
        error instanceof ApiRequestError && error.detail
          ? error.detail
          : error instanceof Error
            ? error.message
            : "创建新版本失败",
      );
    },
  });

  return (
    <Dialog
      open={template !== null}
      onOpenChange={(next) => {
        if (!next) {
          close();
        }
      }}
    >
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>升版 {template?.template_key}</DialogTitle>
          <DialogDescription>
            编辑 spec 后提交为新版本，新版本立即生效为当前活跃版本。
          </DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            setFormError(null);
            let spec: Record<string, unknown>;
            try {
              spec = JSON.parse(specText) as Record<string, unknown>;
            } catch {
              setFormError("spec 不是合法 JSON，请检查格式");
              return;
            }
            versionMutation.mutate(spec);
          }}
        >
          <label className="flex flex-col gap-1">
            <span className="text-sm text-muted-foreground">spec（JSON）</span>
            <textarea
              className="min-h-72 w-full rounded-md border bg-background px-3 py-2 font-mono text-xs"
              value={specText}
              onChange={(event) => setSpecText(event.target.value)}
              spellCheck={false}
              required
            />
          </label>
          {formError ? <p className="text-sm text-destructive">{formError}</p> : null}
          <DialogFooter>
            <V3Button type="button" variant="ghost" onClick={close}>
              取消
            </V3Button>
            <V3Button type="submit" disabled={versionMutation.isPending}>
              {versionMutation.isPending ? "提交中…" : "提交新版本"}
            </V3Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

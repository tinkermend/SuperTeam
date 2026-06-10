import { useEffect, useState, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type {
  ProjectDemandSourceType,
  SubmitProjectDemandInput,
} from "@/lib/api/projects";

type SubmitDemandDialogProps = {
  isSubmitting?: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (input: SubmitProjectDemandInput) => void;
  open: boolean;
  projectName?: string;
  submitError?: string;
};

const sourceTypes: ProjectDemandSourceType[] = [
  "manual",
  "github",
  "ticket",
  "document",
  "log",
];

export function SubmitDemandDialog({
  isSubmitting,
  onOpenChange,
  onSubmit,
  open,
  projectName,
  submitError,
}: SubmitDemandDialogProps) {
  const [title, setTitle] = useState("");
  const [content, setContent] = useState("");
  const [sourceType, setSourceType] =
    useState<ProjectDemandSourceType>("manual");
  const [sourceRefs, setSourceRefs] = useState("{}");
  const [attachments, setAttachments] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    if (!open) {
      setTitle("");
      setContent("");
      setSourceType("manual");
      setSourceRefs("{}");
      setAttachments("");
      setError("");
    }
  }, [open]);

  function submit() {
    if (!title.trim()) {
      setError("需求标题不能为空");
      return;
    }
    let refs: Record<string, unknown> | undefined;
    try {
      const parsed = JSON.parse(sourceRefs || "{}") as unknown;
      if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
        setError("来源引用必须是 JSON object");
        return;
      }
      refs = parsed as Record<string, unknown>;
    } catch {
      setError("来源引用不是有效 JSON");
      return;
    }

    setError("");
    onSubmit({
      attachments: attachments
        .split("\n")
        .map((item) => item.trim())
        .filter(Boolean),
      content: content.trim() || undefined,
      source_refs: refs,
      source_type: sourceType,
      title: title.trim(),
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[620px]">
        <DialogHeader>
          <DialogTitle>提交需求</DialogTitle>
          <DialogDescription>
            {projectName ? `记录到「${projectName}」` : "记录到当前项目"}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <Field label="需求标题">
            <Input
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              placeholder="补充验收证据"
            />
          </Field>
          <Field label="需求内容">
            <Textarea
              value={content}
              onChange={(event) => setContent(event.target.value)}
              placeholder="描述需要项目处理或记录的事项"
            />
          </Field>
          <div className="grid gap-4 md:grid-cols-[180px_minmax(0,1fr)]">
            <Field label="来源">
              <Select
                value={sourceType}
                onValueChange={(value) =>
                  setSourceType(value as ProjectDemandSourceType)
                }
              >
                <SelectTrigger className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {sourceTypes.map((source) => (
                    <SelectItem key={source} value={source}>
                      {source}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field label="来源引用 JSON">
              <Input
                value={sourceRefs}
                onChange={(event) => setSourceRefs(event.target.value)}
                placeholder='{"ticket":"SUP-1"}'
              />
            </Field>
          </div>
          <Field label="附件引用">
            <Textarea
              value={attachments}
              onChange={(event) => setAttachments(event.target.value)}
              placeholder="每行一个附件或对象存储引用"
            />
          </Field>
          {error || submitError ? (
            <p className="text-sm text-destructive">{error || submitError}</p>
          ) : null}
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button disabled={isSubmitting} type="button" onClick={submit}>
            提交
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
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

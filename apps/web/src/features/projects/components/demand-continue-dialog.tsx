import { useEffect, useState } from "react";
import { useMutation } from "@tanstack/react-query";

import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Button, notifySuccess } from "@/components/superteam";
import type { ApiClientOptions } from "@/lib/api/client";
import { createProjectDemandContinuation, type ProjectDemand } from "@/lib/api/projects";

export type DemandContinueDialogProps = {
  apiOptions: ApiClientOptions;
  demandId: string;
  demandTitle: string;
  onContinued: (demand: ProjectDemand) => void;
  onOpenChange: (open: boolean) => void;
  open: boolean;
};

/**
 * 「继续这一单」弹层。
 *
 * 刻意只有一个输入框：接续**继承**剧本、协调模式与血缘，不重走向导、不重选
 * 剧本收口（基线 §4.3「接续继承，不重选」）。要重新选打法的，那不是接续，
 * 是另开一单。
 *
 * 诉求每次都要重新说清楚——继承的是"怎么打"，不是"要打什么"。
 */
export function DemandContinueDialog({
  apiOptions,
  demandId,
  demandTitle,
  onContinued,
  onOpenChange,
  open
}: DemandContinueDialogProps) {
  const [content, setContent] = useState("");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setContent("");
      setError(null);
    }
  }, [open, demandId]);

  const mutation = useMutation({
    mutationFn: () =>
      createProjectDemandContinuation(apiOptions, demandId, { content: content.trim() }),
    onError: (mutationError: unknown) => {
      setError(
        mutationError instanceof Error
          ? mutationError.message
          : "接续失败，请稍后重试"
      );
    },
    onSuccess: (demand) => {
      notifySuccess("已接续，新的一单已创建并开始规划");
      onOpenChange(false);
      onContinued(demand);
    }
  });

  const trimmed = content.trim();

  return (
    <Dialog onOpenChange={onOpenChange} open={open}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>继续这一单</DialogTitle>
          <DialogDescription>
            接着「{demandTitle}」新开一单，沿用同一套剧本与血缘。
            派发时原来的数字员工会回到自己上次的会话继续，换人则从新会话开始。
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-2">
          <Label htmlFor="demand-continue-content">接着要做什么</Label>
          <Textarea
            data-testid="demand-continue-content"
            id="demand-continue-content"
            onChange={(event) => setContent(event.target.value)}
            placeholder="例如：按上一轮结论把方案落成可执行的改造清单"
            rows={5}
            value={content}
          />
          {error ? (
            <p className="text-[12px] text-danger" data-testid="demand-continue-error">
              {error}
            </p>
          ) : null}
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} type="button" variant="ghost">
            取消
          </Button>
          <Button
            data-testid="demand-continue-submit"
            disabled={trimmed.length === 0 || mutation.isPending}
            onClick={() => {
              setError(null);
              mutation.mutate();
            }}
            type="button"
            variant="primary"
          >
            {mutation.isPending ? "创建中…" : "创建接续单"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

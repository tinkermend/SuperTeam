import { Play } from "lucide-react";
import { useState } from "react";
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { V3Button } from "@/components/superteam";

type StartTaskDrawerProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  canStartTask: boolean;
  disabledReasons: string[];
  isPending: boolean;
  isError: boolean;
  onSubmit: (input: { objective: string; prompt: string }) => void;
};

export function StartTaskDrawer({
  open,
  onOpenChange,
  canStartTask,
  disabledReasons,
  isPending,
  isError,
  onSubmit,
}: StartTaskDrawerProps) {
  const [objective, setObjective] = useState("");
  const [prompt, setPrompt] = useState("");
  const trimmedObjective = objective.trim();
  const canSubmit = canStartTask && Boolean(trimmedObjective) && !isPending;

  return (
    <Sheet onOpenChange={onOpenChange} open={open}>
      <SheetContent className="w-full sm:max-w-md" side="right">
        <SheetHeader>
          <SheetTitle>开始任务</SheetTitle>
        </SheetHeader>
        <form
          className="flex flex-col gap-3 px-4 pb-6"
          onSubmit={(event) => {
            event.preventDefault();
            if (canSubmit) {
              onSubmit({ objective: trimmedObjective, prompt: prompt.trim() });
              setObjective("");
              setPrompt("");
            }
          }}
        >
          <div className="space-y-2">
            <Label htmlFor="run-objective">任务目标</Label>
            <Textarea
              disabled={!canStartTask || isPending}
              id="run-objective"
              onChange={(event) => setObjective(event.target.value)}
              rows={2}
              value={objective}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="run-prompt">任务提示</Label>
            <Textarea
              disabled={!canStartTask || isPending}
              id="run-prompt"
              onChange={(event) => setPrompt(event.target.value)}
              rows={4}
              value={prompt}
            />
          </div>
          <V3Button disabled={!canSubmit} type="submit">
            <Play className="size-4" />
            开始任务
          </V3Button>
          {disabledReasons.map((reason) => (
            <p className="text-xs text-v3-ink-3" key={reason}>
              {reason}
            </p>
          ))}
          {isError ? <p className="text-sm text-destructive">开始任务失败</p> : null}
        </form>
      </SheetContent>
    </Sheet>
  );
}

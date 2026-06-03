import { useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import type { UserSummary } from "@/lib/api/auth";
import type { InitialTeamMemberInput } from "@/lib/api/teams";
import { CreateTeamBasicStep } from "./create-team-basic-step";
import { CreateTeamMembersStep } from "./create-team-members-step";

export type CreateTeamDraft = {
  name: string;
  slug: string;
  owner?: UserSummary;
  initial_members: InitialTeamMemberInput[];
};

type CreateTeamDrawerProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  isSubmitting?: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (draft: CreateTeamDraft) => void;
  open: boolean;
  submitError?: string;
};

const emptyDraft: CreateTeamDraft = { name: "", slug: "", initial_members: [] };

export function CreateTeamDrawer(props: CreateTeamDrawerProps) {
  const [step, setStep] = useState<"basic" | "members">("basic");
  const [draft, setDraft] = useState<CreateTeamDraft>(emptyDraft);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const canSubmit = useMemo(
    () => Boolean(draft.name.trim() && draft.slug.trim() && draft.owner),
    [draft],
  );

  function handleOpenChange(open: boolean) {
    props.onOpenChange(open);
    if (!open) {
      setStep("basic");
      setDraft(emptyDraft);
      setErrors({});
    }
  }

  function nextStep() {
    const nextErrors: Record<string, string> = {};
    if (!draft.name.trim()) nextErrors.name = "团队名称不能为空";
    if (!draft.slug.trim()) nextErrors.slug = "团队标识不能为空";
    if (!draft.owner) nextErrors.owner = "请选择负责人";
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length === 0) setStep("members");
  }

  return (
    <Sheet open={props.open} onOpenChange={handleOpenChange}>
      <SheetContent className="flex w-full flex-col gap-0 p-0 sm:max-w-[560px]">
        <SheetHeader className="border-b px-6 py-5">
          <SheetTitle>新建团队</SheetTitle>
          <SheetDescription className="sr-only">
            填写团队基础信息并选择初始成员
          </SheetDescription>
        </SheetHeader>
        <div className="flex-1 overflow-y-auto p-6">
          {step === "basic" ? (
            <CreateTeamBasicStep
              apiBaseUrl={props.apiBaseUrl}
              errors={errors}
              fetcher={props.fetcher}
              onChange={setDraft}
              value={draft}
            />
          ) : (
            <CreateTeamMembersStep
              apiBaseUrl={props.apiBaseUrl}
              draft={draft}
              fetcher={props.fetcher}
              onChange={setDraft}
            />
          )}
          {props.submitError ? (
            <p className="mt-4 text-sm text-destructive">{props.submitError}</p>
          ) : null}
        </div>
        <div className="flex justify-between gap-3 border-t p-4">
          <Button
            onClick={() => handleOpenChange(false)}
            type="button"
            variant="outline"
          >
            取消
          </Button>
          <div className="flex gap-2">
            {step === "members" ? (
              <Button
                onClick={() => setStep("basic")}
                type="button"
                variant="outline"
              >
                上一步
              </Button>
            ) : null}
            {step === "basic" ? (
              <Button onClick={nextStep} type="button">
                下一步
              </Button>
            ) : (
              <Button
                disabled={!canSubmit || props.isSubmitting}
                onClick={() => props.onSubmit(draft)}
                type="button"
              >
                创建团队
              </Button>
            )}
          </div>
        </div>
      </SheetContent>
    </Sheet>
  );
}

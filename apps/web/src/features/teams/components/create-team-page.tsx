import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ChevronRight, Check, BadgeCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { TeamOverview } from "@/lib/api/teams";
import { createTeam } from "@/lib/api/teams";
import { cn } from "@/lib/utils";

import { CreateTeamStepIdentity } from "./create-team-step-identity";
import { CreateTeamStepMembers } from "./create-team-step-members";
import { CreateTeamStepReview } from "./create-team-step-review";
import {
  type CreateTeamDraft,
  emptyCreateTeamDraft,
  toCreateTeamInput,
} from "./create-team-draft";

export type CreateTeamCreatedHandler = (
  overview: TeamOverview,
  options: { goToGovernance: boolean },
) => void;

type CreateTeamViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  onCancel?: () => void;
  onCreated?: CreateTeamCreatedHandler;
};

const STEPS = [
  { id: 1, label: "团队信息" },
  { id: 2, label: "负责人和数字员工" },
  { id: 3, label: "权限与设置" },
  { id: 4, label: "确认并创建" },
];

export function CreateTeamView({
  apiBaseUrl,
  fetcher,
  onCancel,
  onCreated,
}: CreateTeamViewProps) {
  const queryClient = useQueryClient();
  const [currentStep, setCurrentStep] = useState(1);
  const [draft, setDraft] = useState<CreateTeamDraft>(emptyCreateTeamDraft);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [goToGovernance, setGoToGovernance] = useState(false);

  const createMutation = useMutation({
    mutationFn: () =>
      createTeam({ baseUrl: apiBaseUrl, fetcher }, toCreateTeamInput(draft)),
    onSuccess: (overview) => {
      void queryClient.invalidateQueries({ queryKey: ["team-summaries"] });
      onCreated?.(overview, { goToGovernance });
    },
  });

  function validateStep(step: number): boolean {
    const nextErrors: Record<string, string> = {};
    if (step === 1) {
      if (!draft.name.trim()) nextErrors.name = "团队名称不能为空";
      if (!draft.slug.trim()) {
        nextErrors.slug = "团队标识不能为空";
      } else if (!/^[a-zA-Z0-9\-_]+$/.test(draft.slug.trim())) {
        nextErrors.slug = "团队标识只允许包含英文、数字、中划线和下划线";
      }
    } else if (step === 2) {
      if (draft.owners.length === 0) nextErrors.owner = "请至少选择一位负责人";
    }
    setErrors(nextErrors);
    return Object.keys(nextErrors).length === 0;
  }

  function handleNext() {
    if (validateStep(currentStep)) {
      setCurrentStep((c) => Math.min(c + 1, STEPS.length));
    }
  }

  function handlePrev() {
    setCurrentStep((c) => Math.max(c - 1, 1));
  }

  function handleSubmit() {
    if (validateStep(1) && validateStep(2)) {
      createMutation.mutate();
    }
  }

  const submitError =
    createMutation.error instanceof Error ? createMutation.error.message : undefined;

  return (
    <div className="mx-auto flex w-full max-w-6xl flex-col gap-6 pb-20">
      {/* Header and Stepper */}
      <div className="flex flex-col items-start gap-4 lg:flex-row lg:items-end lg:justify-between">
        <div>
          <p className="text-xs font-medium text-muted-foreground">
            团队管理 › 新建团队
          </p>
          <h1 className="mt-2 text-2xl font-bold tracking-tight">新建团队</h1>
        </div>

        {/* Horizontal Stepper */}
        <div className="flex items-center gap-2 rounded-full border bg-card px-4 py-2 shadow-sm lg:gap-4">
          {STEPS.map((step, index) => {
            const isCompleted = currentStep > step.id;
            const isCurrent = currentStep === step.id;

            return (
              <div className="flex items-center gap-2 lg:gap-4" key={step.id}>
                <div
                  className={cn(
                    "flex items-center gap-2 text-sm font-medium",
                    isCurrent
                      ? "text-primary"
                      : isCompleted
                        ? "text-primary"
                        : "text-muted-foreground",
                  )}
                >
                  <span
                    className={cn(
                      "flex size-6 items-center justify-center rounded-full text-xs",
                      isCurrent
                        ? "bg-primary text-primary-foreground"
                        : isCompleted
                          ? "bg-primary/20 text-primary"
                          : "bg-muted text-muted-foreground",
                    )}
                  >
                    {isCompleted ? <Check className="size-3" /> : step.id}
                  </span>
                  <span className="hidden sm:inline">{step.label}</span>
                </div>
                {index < STEPS.length - 1 && (
                  <ChevronRight className="size-4 text-muted-foreground/50" />
                )}
              </div>
            );
          })}
        </div>
      </div>

      <div className="flex min-w-0 flex-col gap-6">
        {submitError && (
          <div className="rounded-xl border border-destructive/20 bg-destructive/10 p-4 text-sm text-destructive">
            {submitError}
          </div>
        )}

        {/* Step Content */}
        {currentStep === 1 && (
          <CreateTeamStepIdentity
            draft={draft}
            errors={errors}
            onChange={setDraft}
          />
        )}
        {currentStep === 2 && (
          <CreateTeamStepMembers
            apiBaseUrl={apiBaseUrl}
            draft={draft}
            errors={errors}
            fetcher={fetcher}
            onChange={setDraft}
          />
        )}
        {currentStep === 3 && (
          <div className="rounded-xl border bg-card p-10 text-center text-muted-foreground shadow-sm">
            <h3 className="mb-2 text-lg font-semibold text-foreground">权限与设置</h3>
            <p className="text-sm">本团队暂时没有特殊的初始配置项，可以直接进入下一步进行确认。</p>
          </div>
        )}
        {currentStep === 4 && (
          <CreateTeamStepReview
            draft={draft}
            goToGovernance={goToGovernance}
            setGoToGovernance={setGoToGovernance}
          />
        )}

        {/* Bottom Navigation */}
        <div className="flex items-center justify-end gap-3 border-t pt-6">
          {onCancel && currentStep === 1 && (
            <Button onClick={onCancel} type="button" variant="outline">
              取消
            </Button>
          )}
          {currentStep > 1 && (
            <Button onClick={handlePrev} type="button" variant="outline">
              上一步
            </Button>
          )}
          {currentStep < STEPS.length ? (
            <Button onClick={handleNext} type="button">
              下一步: {STEPS[currentStep].label}
            </Button>
          ) : (
            <Button
              disabled={createMutation.isPending}
              onClick={handleSubmit}
              type="button"
            >
              <BadgeCheck data-icon="inline-start" />
              确认并创建
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}

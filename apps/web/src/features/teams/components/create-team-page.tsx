import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ChevronRight, Check, BadgeCheck } from "lucide-react";
import { Button } from "@/components/ui/button";
import type { TeamOverview } from "@/lib/api/teams";
import { createTeam } from "@/lib/api/teams";
import { cn } from "@/lib/utils";

import { CreateTeamConfigurationCanvas } from "./create-team-configuration-canvas";
import { CreateTeamStepReview } from "./create-team-step-review";
import {
  type CreateTeamDraft,
  emptyCreateTeamDraft,
  toCreateTeamInput,
} from "./create-team-draft";

export type CreateTeamCreatedHandler = (
  overview: TeamOverview,
  options: { goToConstitution: boolean },
) => void;

type CreateTeamViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  onCancel?: () => void;
  onCreated?: CreateTeamCreatedHandler;
  showHeading?: boolean;
};

const STEPS = [
  { id: 1, label: "配置团队" },
  { id: 2, label: "确认并创建" },
];

export function CreateTeamView({
  apiBaseUrl,
  fetcher,
  onCancel,
  onCreated,
  showHeading = true,
}: CreateTeamViewProps) {
  const queryClient = useQueryClient();
  const [currentStep, setCurrentStep] = useState(1);
  const [draft, setDraft] = useState<CreateTeamDraft>(emptyCreateTeamDraft);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [goToConstitution, setGoToConstitution] = useState(false);

  const createMutation = useMutation({
    mutationFn: () =>
      createTeam({ baseUrl: apiBaseUrl, fetcher }, toCreateTeamInput(draft)),
    onSuccess: (overview) => {
      void queryClient.invalidateQueries({ queryKey: ["team-summaries"] });
      onCreated?.(overview, { goToConstitution });
    },
  });

  function validateBasics(): Record<string, string> {
    const nextErrors: Record<string, string> = {};
    if (!draft.name.trim()) nextErrors.name = "团队名称不能为空";
    const slug = draft.slug.trim();
    if (!slug) {
      nextErrors.slug = "团队标识不能为空";
    } else if (slug.length < 3 || slug.length > 64) {
      nextErrors.slug = "团队标识需为 3-64 个字符";
    } else if (!/^[a-z][a-z0-9-]*[a-z0-9]$/.test(slug)) {
      nextErrors.slug =
        "团队标识需以小写字母开头，仅含小写字母、数字和中划线，且以字母或数字结尾";
    }
    if (draft.owners.length === 0) nextErrors.owner = "请至少选择一位负责人";
    return nextErrors;
  }

  function validateStep(step: number): boolean {
    const nextErrors = step === 1 ? validateBasics() : {};
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
    if (validateStep(1)) {
      createMutation.mutate();
    }
  }

  const submitError =
    createMutation.error instanceof Error ? createMutation.error.message : undefined;

  return (
    <div
      className="flex min-h-0 w-full min-w-0 flex-1 flex-col gap-4"
      data-testid="create-team-view"
    >
      <div
        className={cn(
          "flex shrink-0 flex-wrap items-center gap-3",
          showHeading ? "justify-between" : "justify-start",
        )}
      >
        {showHeading ? (
          <div className="min-w-0">
            <p className="text-xs font-medium text-muted-foreground">
              团队管理 › 新建团队
            </p>
            <h1 className="mt-2 text-2xl font-bold tracking-tight">新建团队</h1>
          </div>
        ) : (
          <span className="sr-only">新建团队步骤</span>
        )}

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
                {index < STEPS.length - 1 ? (
                  <ChevronRight className="size-4 text-muted-foreground/50" />
                ) : null}
              </div>
            );
          })}
        </div>
      </div>

      <div
        className="flex min-h-0 flex-1 flex-col gap-5 overflow-y-auto pb-4"
        data-testid="create-team-scroll-region"
      >
        {submitError ? (
          <div className="rounded-xl border border-destructive/20 bg-destructive/10 p-4 text-sm text-destructive">
            {submitError}
          </div>
        ) : null}

        {currentStep === 1 ? (
          <CreateTeamConfigurationCanvas
            apiBaseUrl={apiBaseUrl}
            draft={draft}
            errors={errors}
            fetcher={fetcher}
            onChange={setDraft}
          />
        ) : null}

        {currentStep === 2 ? (
          <CreateTeamStepReview
            draft={draft}
            goToConstitution={goToConstitution}
            setGoToConstitution={setGoToConstitution}
          />
        ) : null}

      </div>

      <div
        className="sticky bottom-0 z-10 flex shrink-0 items-center justify-end gap-3 border-t border-v3-line px-4 py-4 shadow-[0_-12px_24px_rgba(15,23,42,0.06)] sm:px-6"
        data-testid="create-team-actions"
      >
        {onCancel && currentStep === 1 ? (
          <Button onClick={onCancel} type="button" variant="outline">
            取消
          </Button>
        ) : null}
        {currentStep > 1 ? (
          <Button onClick={handlePrev} type="button" variant="outline">
            上一步
          </Button>
        ) : null}
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
  );
}

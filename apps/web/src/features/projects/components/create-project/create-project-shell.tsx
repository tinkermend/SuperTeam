import { useEffect, useMemo, useState } from "react";
import { ArrowLeft, ArrowRight, Check, X } from "lucide-react";
import type { UserSummary, UserProjectTeamScope } from "@/lib/api";
import type { CreateProjectInput } from "@/lib/api/projects";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import {
  activeSelectableTeams,
  buildProjectCreateInput,
  emptyProjectCreateDraft,
  projectCreateSteps,
  projectCreateValidation,
  type ProjectCreateDraft,
  type ProjectCreateStep,
} from "./create-project-draft";
import { ProjectBasicsStep } from "./project-basics-step";
import { ProjectDigitalEmployeesStep } from "./project-digital-employees-step";
import { ProjectHumanOwnersStep } from "./project-human-owners-step";
import { ProjectPolicyStep } from "./project-policy-step";
import { ProjectReviewPanel } from "./project-review-panel";

type CreateProjectShellProps = {
  apiBaseUrl: string;
  availableTeams?: UserProjectTeamScope[];
  currentUser?: UserSummary;
  currentUserError?: string;
  fetcher?: typeof fetch;
  isCurrentUserLoading?: boolean;
  isSubmitting?: boolean;
  isTeamsLoading?: boolean;
  onCancel: () => void;
  onSubmit: (input: CreateProjectInput) => void;
  submitError?: string;
  teamsError?: string;
};

export function CreateProjectShell({
  apiBaseUrl,
  availableTeams,
  currentUser,
  currentUserError,
  fetcher,
  isCurrentUserLoading,
  isSubmitting,
  isTeamsLoading,
  onCancel,
  onSubmit,
  submitError,
  teamsError,
}: CreateProjectShellProps) {
  const selectableTeams = useMemo(() => activeSelectableTeams(availableTeams), [availableTeams]);
  const [draft, setDraft] = useState<ProjectCreateDraft>(emptyProjectCreateDraft);
  const [activeStep, setActiveStep] = useState<ProjectCreateStep>("basics");
  const [localError, setLocalError] = useState("");
  const validation = projectCreateValidation(draft, currentUser?.id, selectableTeams);
  const activeIndex = projectCreateSteps.findIndex((step) => step.id === activeStep);
  const isAuthorizationLoading = Boolean(isCurrentUserLoading || isTeamsLoading);
  const authorizationError = currentUserError || teamsError;

  useEffect(() => {
    setDraft((current) => {
      const selectableTeamIds = new Set(selectableTeams.map((scope) => scope.team_id));
      const sourceTeamIds = current.sourceTeamIds.filter((teamId) => selectableTeamIds.has(teamId));
      if (selectableTeams.length === 0) {
        if (current.sourceTeamIds.length === 0 && current.selectedDigitalEmployees.length === 0) {
          return current;
        }
        return { ...current, sourceTeamIds: [], selectedDigitalEmployees: [] };
      }
      const allSelectedTeamsGone = current.sourceTeamIds.length > 0 && sourceTeamIds.length === 0;
      const nextSourceTeamIds = sourceTeamIds.length > 0
        ? sourceTeamIds
        : [selectableTeams[0].team_id];
      const selectedDigitalEmployees = allSelectedTeamsGone
        ? []
        : current.selectedDigitalEmployees.filter(
            (employee) => !employee.team_id || nextSourceTeamIds.includes(employee.team_id),
          );

      if (
        arraysEqual(current.sourceTeamIds, nextSourceTeamIds) &&
        selectedDigitalEmployees.length === current.selectedDigitalEmployees.length
      ) {
        return current;
      }
      return { ...current, sourceTeamIds: nextSourceTeamIds, selectedDigitalEmployees };
    });
  }, [selectableTeams]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") onCancel();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onCancel]);

  function goNext() {
    if (activeIndex < projectCreateSteps.length - 1) {
      setActiveStep(projectCreateSteps[activeIndex + 1].id);
    }
  }

  function goBack() {
    if (activeIndex > 0) {
      setActiveStep(projectCreateSteps[activeIndex - 1].id);
    }
  }

  function submit() {
    if (!currentUser) {
      setLocalError("当前用户未加载，无法创建项目");
      return;
    }
    if (!validation.basics) {
      setLocalError("请补齐项目名称和目标");
      setActiveStep("basics");
      return;
    }
    if (!validation.teamAuthorized) {
      setLocalError("请选择已授权的数字员工来源团队");
      setActiveStep("digitalEmployees");
      return;
    }
    setLocalError("");
    onSubmit(buildProjectCreateInput(draft, currentUser));
  }

  const canSubmit =
    validation.basics &&
    validation.currentUser &&
    validation.teamAuthorized &&
    !authorizationError &&
    !isAuthorizationLoading;

  return (
    <div
      aria-labelledby="project-create-title"
      className="min-h-[calc(100svh-7rem)] overflow-hidden rounded-v3-card bg-v3-bg shadow-v3"
      data-testid="project-create-page"
    >
      <div className="flex min-h-[calc(100svh-7rem)] flex-col">
        <header className="border-b border-v3-line bg-v3-card/90 px-4 py-5 backdrop-blur lg:px-8">
          <div className="flex items-start justify-between gap-4">
            <div>
              <p className="text-sm font-medium text-v3-brand">项目管理 / 新建项目</p>
              <h2 className="mt-2 text-3xl font-semibold tracking-tight text-v3-ink" id="project-create-title">新建项目</h2>
              <p className="mt-2 text-sm text-v3-ink-2">建立项目事实容器，配置负责人、团队、数字员工池与策略预设。</p>
            </div>
            <Button aria-label="关闭新建项目" className="size-10 rounded-xl" onClick={onCancel} size="icon" type="button" variant="ghost">
              <X className="size-5" />
            </Button>
          </div>
          <nav aria-label="新建项目步骤" className="mt-8 flex items-center gap-3">
            {projectCreateSteps.map((step, index) => {
              const active = step.id === activeStep;
              const done = index < activeIndex;
              return (
                <button
                  className={cn(
                    "flex min-w-0 items-center gap-2 rounded-xl px-3 py-2 text-sm font-medium transition",
                    active ? "bg-v3-brand-soft text-v3-brand" : "text-v3-ink-2 hover:bg-v3-card-soft",
                  )}
                  key={step.id}
                  onClick={() => setActiveStep(step.id)}
                  type="button"
                >
                  <span
                    className={cn(
                      "grid size-7 shrink-0 place-items-center rounded-full border text-xs",
                      active && "border-v3-brand bg-v3-brand text-white",
                      done && "border-v3-ok bg-v3-ok text-white",
                      !active && !done && "border-v3-line-strong bg-v3-card text-v3-ink-2",
                    )}
                  >
                    {done ? <Check className="size-4" /> : index + 1}
                  </span>
                  <span className="truncate">{step.label}</span>
                </button>
              );
            })}
          </nav>
        </header>

        <main className="grid min-h-0 flex-1 grid-cols-1 gap-6 overflow-y-auto px-4 py-5 lg:grid-cols-[minmax(0,1fr)_420px] lg:px-8 lg:py-6">
          <section className="min-w-0 rounded-v3-card border border-v3-line bg-v3-card p-8 shadow-v3">
            {authorizationError ? (
              <div className="mb-5 rounded-xl border border-v3-danger/20 bg-v3-danger-soft px-3 py-2 text-sm text-v3-danger">
                {currentUserError ? "加载当前用户失败" : "加载可选团队失败"}
              </div>
            ) : null}
            {activeStep === "basics" ? (
              <ProjectBasicsStep draft={draft} onChange={setDraft} />
            ) : activeStep === "owners" ? (
              <ProjectHumanOwnersStep
                apiBaseUrl={apiBaseUrl}
                currentUser={currentUser}
                draft={draft}
                fetcher={fetcher}
                onChange={setDraft}
              />
            ) : activeStep === "digitalEmployees" ? (
              <ProjectDigitalEmployeesStep
                apiBaseUrl={apiBaseUrl}
                draft={draft}
                fetcher={fetcher}
                onChange={setDraft}
                selectableTeams={selectableTeams}
              />
            ) : activeStep === "policies" ? (
              <ProjectPolicyStep draft={draft} onChange={setDraft} />
            ) : (
              <div className="rounded-xl border border-dashed border-v3-line-strong bg-v3-card-soft p-8 text-sm text-v3-ink-2">
                {projectCreateSteps.find((step) => step.id === activeStep)?.label} 步骤将在后续任务中接入。
              </div>
            )}
          </section>

          <ProjectReviewPanel currentUser={currentUser} draft={draft} selectableTeams={selectableTeams} />
        </main>

        <footer className="flex flex-col gap-3 border-t border-v3-line bg-v3-card px-4 py-4 sm:flex-row sm:items-center sm:justify-between lg:px-8">
          <Button onClick={onCancel} type="button" variant="ghost">
            返回项目列表
          </Button>
          <div className="flex items-center gap-3">
            {(localError || submitError) ? <p className="text-sm text-v3-danger">{localError || submitError}</p> : null}
            <Button disabled={activeIndex === 0} onClick={goBack} type="button" variant="outline">
              <ArrowLeft className="mr-2 size-4" />
              上一步
            </Button>
            {activeIndex === projectCreateSteps.length - 1 ? (
              <Button disabled={isSubmitting || !canSubmit} onClick={submit} type="button">
                创建项目
              </Button>
            ) : (
              <Button onClick={goNext} type="button">
                下一步
                <ArrowRight className="ml-2 size-4" />
              </Button>
            )}
          </div>
        </footer>
      </div>
    </div>
  );
}

function arraysEqual(left: string[], right: string[]) {
  return left.length === right.length && left.every((item, index) => item === right[index]);
}

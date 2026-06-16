import { useEffect, useMemo, useState, type ReactNode } from "react";
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
import type { UserProjectTeamScope } from "@/lib/api";
import type { CreateProjectInput } from "@/lib/api/projects";

type CreateProjectDrawerProps = {
  availableTeams?: UserProjectTeamScope[];
  currentUserError?: string;
  currentUserId?: string;
  isCurrentUserLoading?: boolean;
  isSubmitting?: boolean;
  isTeamsLoading?: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (input: CreateProjectInput) => void;
  open: boolean;
  submitError?: string;
  teamsError?: string;
};

const emptyDraft: CreateProjectInput = {
  acceptance_user_id: "",
  goal: "",
  human_owner_user_id: "",
  leader_user_id: "",
  name: "",
  team_id: "",
};

export function CreateProjectDrawer({
  availableTeams,
  currentUserError,
  currentUserId,
  isCurrentUserLoading,
  isSubmitting,
  isTeamsLoading,
  onOpenChange,
  onSubmit,
  open,
  submitError,
  teamsError,
}: CreateProjectDrawerProps) {
  const [draft, setDraft] = useState<CreateProjectInput>(emptyDraft);
  const [error, setError] = useState("");
  const selectableTeams = useMemo(
    () =>
      (availableTeams ?? []).filter(
        (scope) =>
          scope.status === "active" &&
          !scope.revoked_at &&
          scope.team.status === "active",
      ),
    [availableTeams],
  );
  const authorizedTeamIds = useMemo(
    () => new Set(selectableTeams.map((scope) => scope.team_id)),
    [selectableTeams],
  );
  const isLoadingAuthorization = Boolean(isCurrentUserLoading || isTeamsLoading);
  const authorizationError = currentUserError || teamsError;
  const hasSelectableTeams = selectableTeams.length > 0;
  const canSubmit =
    Boolean(draft.name.trim()) &&
    Boolean(draft.goal.trim()) &&
    Boolean(currentUserId) &&
    Boolean(draft.team_id) &&
    !isLoadingAuthorization &&
    !authorizationError &&
    hasSelectableTeams &&
    authorizedTeamIds.has(draft.team_id ?? "");

  useEffect(() => {
    if (!open) {
      setDraft(emptyDraft);
      setError("");
    }
  }, [open]);

  useEffect(() => {
    if (!open) {
      return;
    }

    setDraft((current) => {
      if (current.team_id && authorizedTeamIds.has(current.team_id)) {
        return current;
      }
      return {
        ...current,
        team_id: selectableTeams[0]?.team_id ?? "",
      };
    });
  }, [authorizedTeamIds, open, selectableTeams]);

  function submit() {
    const selectedTeamId = draft.team_id?.trim();
    if (!draft.name.trim() || !draft.goal.trim() || !selectedTeamId) {
      setError("项目名称、目标和可选团队不能为空");
      return;
    }
    if (!currentUserId) {
      setError("当前用户未加载，无法创建项目");
      return;
    }
    if (!authorizedTeamIds.has(selectedTeamId)) {
      setError("请选择授权团队");
      return;
    }
    setError("");
    onSubmit({
      ...draft,
      acceptance_user_id: draft.acceptance_user_id?.trim() || undefined,
      goal: draft.goal.trim(),
      human_owner_user_id: currentUserId,
      leader_user_id: draft.leader_user_id?.trim() || undefined,
      name: draft.name.trim(),
      team_id: selectedTeamId,
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
          <Field label="可选团队">
            <select
              aria-label="可选团队"
              className={selectClassName}
              disabled={isLoadingAuthorization || Boolean(authorizationError) || !hasSelectableTeams}
              value={draft.team_id ?? ""}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  team_id: event.target.value,
                }))
              }
            >
              {hasSelectableTeams ? null : <option value="">无可选项</option>}
              {selectableTeams.map((scope) => (
                <option key={scope.id} value={scope.team_id}>
                  {scope.team.name}
                </option>
              ))}
            </select>
            {isCurrentUserLoading ? (
              <p className="text-sm text-muted-foreground">正在加载当前用户...</p>
            ) : null}
            {!isCurrentUserLoading && isTeamsLoading ? (
              <p className="text-sm text-muted-foreground">正在加载可选团队...</p>
            ) : null}
            {!isLoadingAuthorization && authorizationError ? (
              <p className="text-sm text-destructive">
                {currentUserError ? "加载当前用户失败" : "加载可选团队失败"}
              </p>
            ) : null}
            {!isLoadingAuthorization && !authorizationError && !hasSelectableTeams ? (
              <p className="text-sm text-muted-foreground">暂无可选团队</p>
            ) : null}
          </Field>
          <Field label="Leader 用户 ID">
            <Input
              value={draft.leader_user_id ?? ""}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  leader_user_id: event.target.value,
                }))
              }
              placeholder="可选 UUID"
            />
          </Field>
          <Field label="验收人用户 ID">
            <Input
              value={draft.acceptance_user_id ?? ""}
              onChange={(event) =>
                setDraft((current) => ({
                  ...current,
                  acceptance_user_id: event.target.value,
                }))
              }
              placeholder="可选 UUID"
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
          <Button disabled={isSubmitting || !canSubmit} type="button" onClick={submit}>
            创建项目
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  );
}

const selectClassName =
  "h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm shadow-xs outline-none transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50";

function Field({ children, label }: { children: ReactNode; label: string }) {
  return (
    <Label className="grid gap-2">
      <span>{label}</span>
      {children}
    </Label>
  );
}

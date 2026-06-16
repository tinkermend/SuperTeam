import { useCallback, useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ShieldAlert } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
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
import {
  listDigitalEmployeeAvatarAssets,
  listTeamSummaries,
  type DigitalEmployeeAvatarAsset,
} from "@/lib/api";
import { cn } from "@/lib/utils";
import { SelectableTeamList } from "./selectable-team-list";

export type CreateUserDraft = {
  avatar_asset_id: string;
  display_name: string;
  password: string;
  selectable_team_ids: string[];
  username: string;
};

type CreateUserDrawerProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  isSubmitting?: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (draft: CreateUserDraft) => void;
  open: boolean;
  submitError?: string;
};

const emptyDraft: CreateUserDraft = {
  avatar_asset_id: "",
  display_name: "",
  password: "",
  selectable_team_ids: [],
  username: "",
};

export function CreateUserDrawer({
  apiBaseUrl,
  fetcher,
  isSubmitting = false,
  onOpenChange,
  onSubmit,
  open,
  submitError,
}: CreateUserDrawerProps) {
  const [draft, setDraft] = useState<CreateUserDraft>(emptyDraft);
  const apiOptions = useMemo(
    () => ({
      baseUrl: apiBaseUrl,
      fetcher,
    }),
    [apiBaseUrl, fetcher],
  );
  const avatarAssetsQuery = useQuery({
    enabled: open,
    queryFn: () => listDigitalEmployeeAvatarAssets(apiOptions),
    queryKey: ["users", "create", "avatar-assets"],
  });
  const teamsQuery = useQuery({
    enabled: open,
    queryFn: () => listTeamSummaries(apiOptions),
    queryKey: ["users", "create", "teams"],
  });
  const avatarAssets = avatarAssetsQuery.data ?? [];
  const teams = teamsQuery.data ?? [];
  const avatarAssetsHasError = Boolean(avatarAssetsQuery.error);
  const teamsHasError = Boolean(teamsQuery.error);
  const avatarAssetsReady =
    avatarAssetsQuery.isSuccess &&
    !avatarAssetsQuery.isFetching &&
    !avatarAssetsHasError &&
    avatarAssets.length > 0;
  const teamsReady = teamsQuery.isSuccess && !teamsQuery.isFetching && !teamsHasError && teams.length > 0;
  const currentAvatarAssets = avatarAssetsReady ? avatarAssets : [];
  const currentTeams = teamsReady ? teams : [];
  const selectedTeamIdsAreCurrent =
    draft.selectable_team_ids.length > 0 &&
    draft.selectable_team_ids.every((teamId) => currentTeams.some((team) => team.id === teamId));
  const canSubmit = Boolean(
    draft.username.trim() &&
      draft.display_name.trim() &&
      draft.password.trim() &&
      draft.avatar_asset_id.trim() &&
      avatarAssetsReady &&
      currentAvatarAssets.some((asset) => asset.id === draft.avatar_asset_id) &&
      teamsReady &&
      selectedTeamIdsAreCurrent,
  );

  const resetDraft = useCallback(() => {
    setDraft(emptyDraft);
  }, []);

  useEffect(() => {
    if (!open) {
      resetDraft();
    }
  }, [open, resetDraft]);

  function handleOpenChange(nextOpen: boolean) {
    onOpenChange(nextOpen);
    if (!nextOpen) {
      resetDraft();
    }
  }

  return (
    <Sheet open={open} onOpenChange={handleOpenChange}>
      <SheetContent className="flex w-full flex-col gap-0 p-0 sm:max-w-[680px]">
        <SheetHeader className="border-b px-6 py-5">
          <SheetTitle>新建用户</SheetTitle>
          <SheetDescription className="sr-only">创建平台人类用户</SheetDescription>
        </SheetHeader>
        <form
          className="flex min-h-0 flex-1 flex-col"
          onSubmit={(event) => {
            event.preventDefault();
            if (canSubmit && !isSubmitting) {
              onSubmit({
                avatar_asset_id: draft.avatar_asset_id,
                display_name: draft.display_name.trim(),
                password: draft.password,
                selectable_team_ids: draft.selectable_team_ids,
                username: draft.username.trim(),
              });
            }
          }}
        >
          <div className="flex-1 overflow-y-auto p-6">
            <div className="grid gap-5">
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="flex flex-col gap-2">
                  <Label htmlFor="create-user-username">用户名</Label>
                  <Input
                    autoComplete="username"
                    id="create-user-username"
                    onChange={(event) => setDraft({ ...draft, username: event.target.value })}
                    value={draft.username}
                  />
                </div>
                <div className="flex flex-col gap-2">
                  <Label htmlFor="create-user-display-name">名称</Label>
                  <Input
                    autoComplete="name"
                    id="create-user-display-name"
                    onChange={(event) => setDraft({ ...draft, display_name: event.target.value })}
                    value={draft.display_name}
                  />
                </div>
              </div>
              <div className="flex flex-col gap-2">
                <Label htmlFor="create-user-password">密码</Label>
                <Input
                  autoComplete="new-password"
                  id="create-user-password"
                  onChange={(event) => setDraft({ ...draft, password: event.target.value })}
                  type="password"
                  value={draft.password}
                />
              </div>

              <AvatarSelection
                assets={currentAvatarAssets}
                isError={avatarAssetsHasError}
                isLoading={avatarAssetsQuery.isLoading || avatarAssetsQuery.isFetching}
                onSelect={(avatarAssetId) => setDraft({ ...draft, avatar_asset_id: avatarAssetId })}
                selectedAssetId={draft.avatar_asset_id}
              />

              <section className="rounded-md border p-3">
                <div className="mb-3 flex min-w-0 items-center justify-between gap-3">
                  <h3 className="text-sm font-medium">选择可选团队</h3>
                  <span className="text-xs text-muted-foreground">
                    已选 {draft.selectable_team_ids.length}
                  </span>
                </div>
                {teamsQuery.isLoading || teamsQuery.isFetching ? (
                  <p className="text-sm text-muted-foreground">加载可选团队中</p>
                ) : null}
                {teamsHasError ? (
                  <Alert variant="destructive">
                    <ShieldAlert />
                    <AlertTitle>可选团队加载失败</AlertTitle>
                    <AlertDescription>请检查团队服务后重试。</AlertDescription>
                  </Alert>
                ) : null}
                {!teamsQuery.isLoading && !teamsQuery.isFetching && !teamsHasError && teams.length === 0 ? (
                  <p className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
                    暂无可选团队。
                  </p>
                ) : null}
                {currentTeams.length > 0 ? (
                  <SelectableTeamList
                    disabled={isSubmitting}
                    onChange={(selectableTeamIds) =>
                      setDraft({ ...draft, selectable_team_ids: selectableTeamIds })
                    }
                    selectedTeamIds={draft.selectable_team_ids}
                    teams={currentTeams}
                  />
                ) : null}
              </section>

              {submitError ? <p className="text-sm text-destructive">{submitError}</p> : null}
            </div>
          </div>
          <div className="flex justify-between gap-3 border-t p-4">
            <Button onClick={() => handleOpenChange(false)} type="button" variant="outline">
              取消
            </Button>
            <Button disabled={!canSubmit || isSubmitting} type="submit">
              创建用户
            </Button>
          </div>
        </form>
      </SheetContent>
    </Sheet>
  );
}

function AvatarSelection({
  assets,
  isError,
  isLoading,
  onSelect,
  selectedAssetId,
}: {
  assets: DigitalEmployeeAvatarAsset[];
  isError: boolean;
  isLoading: boolean;
  onSelect: (value: string) => void;
  selectedAssetId: string;
}) {
  return (
    <fieldset className="rounded-md border p-3">
      <legend className="px-1 text-sm font-medium">头像</legend>
      {isLoading ? <p className="mt-3 text-sm text-muted-foreground">加载头像中</p> : null}
      {isError ? (
        <Alert className="mt-3" variant="destructive">
          <ShieldAlert />
          <AlertTitle>头像加载失败</AlertTitle>
          <AlertDescription>请检查头像资产接口后重试。</AlertDescription>
        </Alert>
      ) : null}
      <div className="mt-3 flex flex-wrap gap-3">
        {assets.map((asset) => {
          const selected = asset.id === selectedAssetId;
          return (
            <button
              aria-label={`选择头像 ${asset.label}`}
              aria-pressed={selected}
              className={cn(
                "flex size-20 shrink-0 items-center justify-center rounded-full border bg-muted p-0.5 transition",
                selected ? "border-primary ring-2 ring-primary/30" : "hover:border-primary/60",
              )}
              key={asset.id}
              onClick={() => onSelect(asset.id)}
              type="button"
            >
              <img alt={asset.label} className="size-full rounded-full object-cover" src={asset.thumbnail_url} />
            </button>
          );
        })}
      </div>
      {!isLoading && !isError && assets.length === 0 ? (
        <p className="mt-2 text-sm text-muted-foreground">暂无可选头像</p>
      ) : null}
    </fieldset>
  );
}

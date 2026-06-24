import type { ReactNode } from "react";
import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  BadgeCheck,
  Boxes,
  Plug,
  Plus,
  ScrollText,
  Share2,
  ShieldCheck,
  UserCog,
  UsersRound,
} from "lucide-react";
import { TeamIconPicker } from "@/components/superteam/team-icon-picker";
import { TeamIconTile } from "@/components/superteam/team-icon-tile";
import { UserIdentity } from "@/components/superteam/user-identity";
import { UserSearchSelect } from "@/components/superteam/user-search-select";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { TeamOverview } from "@/lib/api/teams";
import { createTeam } from "@/lib/api/teams";
import { cn } from "@/lib/utils";
import { CreateTeamMembersStep } from "./create-team-members-step";
import {
  type CreateTeamDraft,
  emptyCreateTeamDraft,
  inferTeamDisplay,
  slugify,
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

export function CreateTeamView({
  apiBaseUrl,
  fetcher,
  onCancel,
  onCreated,
}: CreateTeamViewProps) {
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<CreateTeamDraft>(emptyCreateTeamDraft);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [goToGovernance, setGoToGovernance] = useState(false);
  const [editingOwner, setEditingOwner] = useState(false);
  const [addingMembers, setAddingMembers] = useState(false);

  const createMutation = useMutation({
    mutationFn: () =>
      createTeam({ baseUrl: apiBaseUrl, fetcher }, toCreateTeamInput(draft)),
    onSuccess: (overview) => {
      void queryClient.invalidateQueries({ queryKey: ["team-summaries"] });
      onCreated?.(overview, { goToGovernance });
    },
  });

  function updateName(name: string) {
    setDraft((current) => ({
      ...current,
      display: current.displayTouched
        ? current.display
        : inferTeamDisplay(`${name} ${current.slug}`),
      name,
      slug: current.slugTouched ? current.slug : slugify(name),
    }));
  }

  function updateSlug(slug: string) {
    setDraft((current) => ({
      ...current,
      display: current.displayTouched
        ? current.display
        : inferTeamDisplay(`${current.name} ${slug}`),
      slug,
      slugTouched: true,
    }));
  }

  function updateIcon(iconKey: string) {
    setDraft((current) => ({
      ...current,
      display: { ...current.display, icon_key: iconKey },
      displayTouched: true,
    }));
  }

  function updateOwner(owner: CreateTeamDraft["owner"]) {
    if (!owner) return;
    setDraft((current) => {
      const memberUsers = { ...current.memberUsers };
      delete memberUsers[owner.id];
      return {
        ...current,
        initial_members: current.initial_members.filter(
          (member) => member.user_id !== owner.id,
        ),
        memberUsers,
        owner,
      };
    });
  }

  function handleSubmit() {
    const nextErrors: Record<string, string> = {};
    if (!draft.name.trim()) nextErrors.name = "团队名称不能为空";
    if (!draft.slug.trim()) nextErrors.slug = "团队标识不能为空";
    if (!draft.owner) nextErrors.owner = "请选择负责人";
    setErrors(nextErrors);
    if (Object.keys(nextErrors).length > 0) return;
    createMutation.mutate();
  }

  const submitError =
    createMutation.error instanceof Error ? createMutation.error.message : undefined;
  const previewName = draft.name.trim() || "未命名团队";
  const previewSlug = draft.slug.trim() || "team-slug";

  return (
    <div className="mx-auto w-full max-w-[1180px]">
      <div className="mb-6 flex items-end justify-between gap-4">
        <div>
          <p className="text-xs font-medium text-muted-foreground">
            团队管理 › 新建团队
          </p>
          <h1 className="mt-2 text-2xl font-bold tracking-tight">新建团队</h1>
          <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
            团队创建后进入治理生命周期：先确定身份与成员，再配置治理策略、对外借调与数字员工。右侧可预览创建结果与后续待办。
          </p>
        </div>
        {onCancel ? (
          <Button onClick={onCancel} type="button" variant="outline">
            取消
          </Button>
        ) : null}
      </div>

      <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-[minmax(0,1fr)_360px]">
        <div className="flex min-w-0 flex-col gap-5">
          {/* identity */}
          <section className="rounded-xl border bg-card shadow-sm">
            <header className="flex items-center gap-3 border-b px-5 py-4">
              <span className="flex size-8 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <ShieldCheck className="size-4" />
              </span>
              <div>
                <h2 className="text-sm font-semibold">团队身份</h2>
                <p className="text-xs text-muted-foreground">
                  名称、标识与配色，用于全站识别这个团队。
                </p>
              </div>
            </header>
            <div className="flex flex-col gap-5 p-5">
              <div className="grid gap-2">
                <Label htmlFor="team-name">团队名称</Label>
                <Input
                  id="team-name"
                  onChange={(event) => updateName(event.target.value)}
                  value={draft.name}
                />
                {errors.name ? (
                  <span className="text-sm text-destructive">{errors.name}</span>
                ) : (
                  <span className="text-xs text-muted-foreground">
                    用于全站展示，可随时修改。
                  </span>
                )}
              </div>

              <div className="grid gap-1.5">
                <Label
                  className="text-xs font-medium text-muted-foreground"
                  htmlFor="team-slug"
                >
                  团队标识 slug
                </Label>
                <div className="flex items-center gap-2 rounded-lg border bg-muted/30 px-2.5 py-1">
                  <span className="select-none font-mono text-xs text-muted-foreground">
                    /teams/
                  </span>
                  <Input
                    className="h-8 border-0 bg-transparent px-0 font-mono text-sm shadow-none focus-visible:ring-0"
                    id="team-slug"
                    onChange={(event) => updateSlug(event.target.value)}
                    placeholder="team-slug"
                    value={draft.slug}
                  />
                </div>
                {errors.slug ? (
                  <span className="text-sm text-destructive">{errors.slug}</span>
                ) : (
                  <span className="text-xs text-muted-foreground">
                    用于网址与接口标识，默认按名称生成，可手动修改。
                  </span>
                )}
              </div>

              <div className="grid gap-2">
                <Label>团队图标</Label>
                <TeamIconPicker
                  colorTone={draft.display.color_tone}
                  onSelect={updateIcon}
                  value={draft.display.icon_key}
                />
                <span className="text-xs text-muted-foreground">
                  点击展开图标库，可搜索全部 lucide 图标。
                </span>
              </div>
            </div>
          </section>

          {/* owner */}
          <section className="rounded-xl border bg-card shadow-sm">
            <header className="flex items-center gap-3 border-b px-5 py-4">
              <span className="flex size-8 items-center justify-center rounded-lg bg-emerald-500/10 text-emerald-600">
                <UserCog className="size-4" />
              </span>
              <div>
                <h2 className="text-sm font-semibold">团队负责人</h2>
                <p className="text-xs text-muted-foreground">
                  团队的固定人类管理者，负责成员的添加、删除与团队配置；审批 / 驳回 / 补证 / 验收由项目负责人在项目内承担。
                </p>
              </div>
            </header>
            <div className="flex flex-col gap-2 p-5">
              {draft.owner && !editingOwner ? (
                <div className="flex items-center gap-3 rounded-lg border bg-muted/30 px-3 py-2">
                  <UserIdentity showSecondary size="sm" user={draft.owner} />
                  <span className="ml-auto rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2.5 py-0.5 text-xs font-medium text-emerald-700">
                    人类 · 团队管理者
                  </span>
                  <Button
                    onClick={() => setEditingOwner(true)}
                    size="sm"
                    type="button"
                    variant="ghost"
                  >
                    更换
                  </Button>
                </div>
              ) : (
                <>
                  <Label>负责人</Label>
                  <UserSearchSelect
                    apiBaseUrl={apiBaseUrl}
                    excludedUserIds={draft.owner ? [draft.owner.id] : []}
                    fetcher={fetcher}
                    onSelect={(user) => {
                      updateOwner(user);
                      setEditingOwner(false);
                    }}
                    placeholder="负责人"
                  />
                  {errors.owner ? (
                    <span className="text-sm text-destructive">{errors.owner}</span>
                  ) : null}
                </>
              )}
            </div>
          </section>

          {/* members */}
          <section className="rounded-xl border bg-card shadow-sm">
            <header className="flex items-center gap-3 border-b px-5 py-4">
              <span className="flex size-8 items-center justify-center rounded-lg bg-teal-500/10 text-teal-600">
                <UsersRound className="size-4" />
              </span>
              <div>
                <h2 className="text-sm font-semibold">
                  初始成员{" "}
                  <span className="text-xs font-normal text-muted-foreground">
                    （可选）
                  </span>
                </h2>
                <p className="text-xs text-muted-foreground">
                  创建时仅可加入「普通成员」与「只读观察者」。
                </p>
              </div>
            </header>
            <div className="p-5">
              {addingMembers || draft.initial_members.length > 0 ? (
                <CreateTeamMembersStep
                  apiBaseUrl={apiBaseUrl}
                  draft={draft}
                  fetcher={fetcher}
                  onChange={setDraft}
                />
              ) : (
                <Button
                  className="gap-2"
                  onClick={() => setAddingMembers(true)}
                  type="button"
                  variant="outline"
                >
                  <Plus data-icon="inline-start" />
                  添加初始成员
                </Button>
              )}
            </div>
          </section>

          {submitError ? (
            <p className="text-sm text-destructive">{submitError}</p>
          ) : null}
        </div>

        {/* right rail */}
        <aside className="flex flex-col gap-5 lg:sticky lg:top-5">
          <div className="overflow-hidden rounded-xl bg-gradient-to-br from-primary to-primary/70 p-5 text-primary-foreground shadow-md">
            <p className="text-xs font-semibold tracking-wide opacity-80">
              实时预览
            </p>
            <div className="mt-3 flex items-center gap-3">
              <TeamIconTile
                className="size-12 rounded-xl border-white/40 bg-white/80 [&_svg]:size-5"
                metadata={{ display: draft.display }}
              />
              <div className="min-w-0">
                <div className="truncate text-base font-bold">{previewName}</div>
                <div className="truncate font-mono text-xs opacity-90">
                  {previewSlug}
                </div>
              </div>
            </div>
            <div className="mt-4 flex gap-6 border-t border-white/20 pt-3">
              <PreviewStat label="负责人" value={draft.owner ? 1 : 0} />
              <PreviewStat label="初始成员" value={draft.initial_members.length} />
              <PreviewStat label="数字员工" value={0} />
            </div>
          </div>

          <div className="rounded-xl border bg-card p-4 shadow-sm">
            <h3 className="text-sm font-semibold">创建后解锁</h3>
            <p className="mb-3 mt-1 text-xs text-muted-foreground">
              下列能力在团队创建后开放，可按需逐步配置：
            </p>
            <ul className="flex flex-col">
              <LifecycleRow
                desc="章程 · 能力策略 · 审批策略 · 工件契约"
                icon={<ShieldCheck className="size-4" />}
                state="创建后配置"
                stateTone="neutral"
                title="治理策略"
                tone="text-emerald-600 bg-emerald-500/10"
              />
              <LifecycleRow
                desc="本团队员工可被哪些项目调用 · 预算 / 能力上限 · 超纲走团队负责人审批"
                icon={<Share2 className="size-4" />}
                state="创建后配置"
                stateTone="neutral"
                title="对外借调策略"
                tone="text-blue-600 bg-blue-500/10"
              />
              <LifecycleRow
                desc="从员工池调度可执行的数字员工"
                icon={<Boxes className="size-4" />}
                state="0 已加入"
                stateTone="neutral"
                title="数字员工"
                tone="text-teal-600 bg-teal-500/10"
              />
              <LifecycleRow
                desc="绑定 HTTP 连接器与外部系统能力"
                icon={<Plug className="size-4" />}
                state="0 已绑定"
                stateTone="neutral"
                title="外部能力"
                tone="text-amber-600 bg-amber-500/10"
              />
              <LifecycleRow
                desc="团队内所有操作自动留痕"
                icon={<ScrollText className="size-4" />}
                state="自动开启"
                stateTone="neutral"
                title="审计日志"
                tone="text-slate-500 bg-slate-500/10"
              />
            </ul>
          </div>

          <div className="rounded-xl border bg-card p-4 shadow-sm">
            <label className="mb-3 flex cursor-pointer items-start gap-2.5">
              <Checkbox
                checked={goToGovernance}
                className="mt-0.5"
                onCheckedChange={(checked) => setGoToGovernance(checked === true)}
              />
              <span className="text-sm">
                <span className="font-medium">创建后前往治理配置</span>
                <span className="mt-0.5 block text-xs text-muted-foreground">
                  立即进入治理草稿，补齐章程与审批策略。
                </span>
              </span>
            </label>
            <Button
              className="w-full"
              disabled={createMutation.isPending}
              onClick={handleSubmit}
              type="button"
            >
              <BadgeCheck data-icon="inline-start" />
              创建团队
            </Button>
          </div>
        </aside>
      </div>
    </div>
  );
}

function PreviewStat({ label, value }: { label: string; value: number }) {
  return (
    <div>
      <div className="text-lg font-bold">{value}</div>
      <div className="text-xs opacity-85">{label}</div>
    </div>
  );
}

function LifecycleRow({
  desc,
  icon,
  state,
  stateTone,
  title,
  tone,
}: {
  desc: string;
  icon: ReactNode;
  state: string;
  stateTone: "neutral" | "warning";
  title: string;
  tone: string;
}) {
  return (
    <li className="flex gap-3 border-t py-3 first:border-t-0">
      <span
        className={cn(
          "flex size-8 flex-none items-center justify-center rounded-lg",
          tone,
        )}
      >
        {icon}
      </span>
      <div className="min-w-0">
        <div className="text-sm font-medium">{title}</div>
        <div className="mt-0.5 text-xs text-muted-foreground">{desc}</div>
      </div>
      <span
        className={cn(
          "ml-auto self-center whitespace-nowrap rounded-full border px-2.5 py-0.5 text-xs font-medium",
          stateTone === "warning"
            ? "border-amber-500/30 bg-amber-500/10 text-amber-700"
            : "border-slate-300/60 bg-slate-500/10 text-slate-600",
        )}
      >
        {state}
      </span>
    </li>
  );
}

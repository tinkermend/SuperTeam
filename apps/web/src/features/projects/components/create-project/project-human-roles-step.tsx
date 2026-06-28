import { X } from "lucide-react";
import type { UserSummary } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { UserIdentity } from "@/components/superteam/user-identity";
import { UserSearchSelect } from "@/components/superteam/user-search-select";
import type { ProjectCreateDraft } from "./create-project-draft";

type ProjectHumanRolesStepProps = {
  apiBaseUrl: string;
  currentUser?: UserSummary;
  draft: ProjectCreateDraft;
  fetcher?: typeof fetch;
  onChange: (draft: ProjectCreateDraft) => void;
};

export function ProjectHumanRolesStep({
  apiBaseUrl,
  currentUser,
  draft,
  fetcher,
  onChange,
}: ProjectHumanRolesStepProps) {
  const excludedUserIds = [
    currentUser?.id,
    draft.leaderUser?.id,
    draft.acceptanceUser?.id,
    ...draft.reviewerUsers.map((user) => user.id),
  ].filter(Boolean) as string[];

  return (
    <div className="grid gap-6">
      <section className="grid gap-2">
        <Label>固定负责人（当前创建人）</Label>
        <div className="rounded-xl border border-v3-line bg-v3-card-soft px-3 py-2">
          {currentUser ? <UserIdentity showSecondary user={currentUser} /> : <p className="text-sm text-v3-ink-3">正在加载当前用户...</p>}
        </div>
      </section>

      <section className="grid gap-2">
        <Label>项目负责人（Leader）</Label>
        <UserSearchSelect
          apiBaseUrl={apiBaseUrl}
          excludedUserIds={excludedUserIds.filter((id) => id !== draft.leaderUser?.id)}
          fetcher={fetcher}
          inputLabel="搜索项目负责人"
          onSelect={(leaderUser) => onChange({ ...draft, leaderUser })}
          placeholder="搜索人类用户作为项目负责人"
          value={draft.leaderUser}
        />
      </section>

      <section className="grid gap-2">
        <Label>验收负责人</Label>
        <UserSearchSelect
          apiBaseUrl={apiBaseUrl}
          excludedUserIds={excludedUserIds.filter((id) => id !== draft.acceptanceUser?.id)}
          fetcher={fetcher}
          inputLabel="搜索验收负责人"
          onSelect={(acceptanceUser) => onChange({ ...draft, acceptanceUser })}
          placeholder="搜索人类用户作为验收负责人"
          value={draft.acceptanceUser}
        />
      </section>

      <section className="grid gap-3">
        <div>
          <Label>审核人</Label>
          <p className="mt-1 text-xs text-v3-ink-3">可选。用于后续审批、补证或风险确认，不替代固定负责人。</p>
        </div>
        <UserSearchSelect
          apiBaseUrl={apiBaseUrl}
          excludedUserIds={excludedUserIds}
          fetcher={fetcher}
          inputLabel="搜索审核人"
          onSelect={(reviewer) => {
            if (draft.reviewerUsers.some((user) => user.id === reviewer.id)) return;
            onChange({ ...draft, reviewerUsers: [...draft.reviewerUsers, reviewer] });
          }}
          placeholder="搜索后添加审核人"
        />
        {draft.reviewerUsers.length > 0 ? (
          <ul className="grid gap-2">
            {draft.reviewerUsers.map((reviewer) => (
              <li className="flex items-center justify-between gap-3 rounded-xl border border-v3-line bg-v3-card-soft px-3 py-2" key={reviewer.id}>
                <UserIdentity showSecondary user={reviewer} />
                <Button
                  aria-label={`移除审核人 ${reviewer.username}`}
                  className="size-8"
                  onClick={() => onChange({ ...draft, reviewerUsers: draft.reviewerUsers.filter((user) => user.id !== reviewer.id) })}
                  size="icon"
                  type="button"
                  variant="ghost"
                >
                  <X className="size-4" />
                </Button>
              </li>
            ))}
          </ul>
        ) : null}
      </section>
    </div>
  );
}

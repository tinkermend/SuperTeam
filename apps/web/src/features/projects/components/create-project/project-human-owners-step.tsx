import { X } from "lucide-react";
import type { UserSummary } from "@/lib/api";
import { Label } from "@/components/ui/label";
import { UserIdentity } from "@/components/superteam/user-identity";
import { UserSearchSelect } from "@/components/superteam/user-search-select";
import type { ProjectCreateDraft } from "./create-project-draft";
import { Button } from "@/components/superteam";

type ProjectHumanOwnersStepProps = {
  apiBaseUrl: string;
  currentUser?: UserSummary;
  draft: ProjectCreateDraft;
  fetcher?: typeof fetch;
  onChange: (draft: ProjectCreateDraft) => void;
};

export function ProjectHumanOwnersStep({
  apiBaseUrl,
  currentUser,
  draft,
  fetcher,
  onChange
}: ProjectHumanOwnersStepProps) {
  const excludedUserIds = [
    currentUser?.id,
    ...draft.ownerUsers.map((user) => user.id),
  ].filter(Boolean) as string[];

  return (
    <div className="grid gap-6">
      <section className="grid gap-2">
        <Label>主负责人（当前创建人）</Label>
        <div className="rounded-xl border border-line bg-card-soft px-3 py-2">
          {currentUser ? <UserIdentity showSecondary user={currentUser} /> : <p className="text-sm text-ink-3">正在加载当前用户...</p>}
        </div>
        <p className="text-xs text-ink-3">主负责人用于默认审批、需求确认和最终验收；其他负责人作为项目 owner 成员参与管理。</p>
      </section>

      <section className="grid gap-3">
        <div>
          <Label>项目人类负责人</Label>
          <p className="mt-1 text-xs text-ink-3">可选。额外负责人会以 owner 成员加入项目，不拆分 Leader、验收负责人或审核人。</p>
        </div>
        <UserSearchSelect
          apiBaseUrl={apiBaseUrl}
          excludedUserIds={excludedUserIds}
          fetcher={fetcher}
          inputLabel="搜索项目人类负责人"
          onSelect={(owner) => {
            if (draft.ownerUsers.some((user) => user.id === owner.id)) return;
            onChange({ ...draft, ownerUsers: [...draft.ownerUsers, owner] });
          }}
          placeholder="搜索后添加项目负责人"
        />
        {draft.ownerUsers.length > 0 ? (
          <ul className="grid gap-2">
            {draft.ownerUsers.map((owner) => (
              <li className="flex items-center justify-between gap-3 rounded-xl border border-line bg-card-soft px-3 py-2" key={owner.id}>
                <UserIdentity showSecondary user={owner} />
                <Button
                  aria-label={`移除项目负责人 ${owner.username}`}
                  className="size-8"
                  onClick={() => onChange({ ...draft, ownerUsers: draft.ownerUsers.filter((user) => user.id !== owner.id) })}
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

import { useQuery } from "@tanstack/react-query";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { listUsers } from "@/lib/api/auth";
import type { InitialTeamMemberInput } from "@/lib/api/teams";
import type { CreateTeamDraft } from "./create-team-drawer";

type CreateTeamMembersStepProps = {
  apiBaseUrl: string;
  draft: CreateTeamDraft;
  fetcher?: typeof fetch;
  onChange: (draft: CreateTeamDraft) => void;
};

export function CreateTeamMembersStep({
  apiBaseUrl,
  draft,
  fetcher,
  onChange,
}: CreateTeamMembersStepProps) {
  const users = useQuery({
    queryKey: ["team-member-candidates"],
    queryFn: () =>
      listUsers({
        baseUrl: apiBaseUrl,
        fetcher,
        limit: 50,
        offset: 0,
        status: "active",
      }),
  });

  function addMember(member: InitialTeamMemberInput) {
    if (member.user_id === draft.owner?.id) return;
    if (draft.initial_members.some((item) => item.user_id === member.user_id))
      return;
    onChange({ ...draft, initial_members: [...draft.initial_members, member] });
  }

  return (
    <div className="space-y-5">
      <div className="rounded-md border px-3 py-2 text-sm text-muted-foreground">
        负责人、管理员、审批人需创建后发起特权角色申请。
      </div>
      <div className="space-y-2">
        <h3 className="text-sm font-medium">候选用户</h3>
        {(users.data?.items ?? []).map((user) => {
          const isOwner = user.id === draft.owner?.id;
          return (
            <div
              className="flex items-center justify-between gap-3 rounded-md border px-3 py-2"
              key={user.id}
            >
              <div>
                <div className="text-sm font-medium">{user.username}</div>
                <div className="text-xs text-muted-foreground">
                  {isOwner ? "负责人" : user.status}
                </div>
              </div>
              <div className="flex flex-wrap gap-2">
                <Button
                  disabled={isOwner}
                  onClick={() =>
                    addMember({ user_id: user.id, role: "member" })
                  }
                  size="sm"
                  type="button"
                  variant="outline"
                >
                  添加 {user.username} 为普通成员
                </Button>
                <Button
                  disabled={isOwner}
                  onClick={() =>
                    addMember({ user_id: user.id, role: "viewer" })
                  }
                  size="sm"
                  type="button"
                  variant="outline"
                >
                  添加 {user.username} 为只读观察者
                </Button>
              </div>
            </div>
          );
        })}
      </div>
      <div className="space-y-2">
        <h3 className="text-sm font-medium">
          已选择的初始成员（{draft.initial_members.length}）
        </h3>
        {draft.initial_members.map((member) => (
          <div
            className="flex items-center justify-between rounded-md border px-3 py-2"
            key={member.user_id}
          >
            <span className="text-sm">{member.user_id}</span>
            <Badge variant="secondary">
              {member.role === "member" ? "普通成员" : "只读观察者"}
            </Badge>
          </div>
        ))}
      </div>
    </div>
  );
}

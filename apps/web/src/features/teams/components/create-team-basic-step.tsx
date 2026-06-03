import { useQuery } from "@tanstack/react-query";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { listUsers } from "@/lib/api/auth";
import type { CreateTeamDraft } from "./create-team-drawer";

type CreateTeamBasicStepProps = {
  apiBaseUrl: string;
  errors: Record<string, string>;
  fetcher?: typeof fetch;
  onChange: (draft: CreateTeamDraft) => void;
  value: CreateTeamDraft;
};

export function CreateTeamBasicStep({
  apiBaseUrl,
  errors,
  fetcher,
  onChange,
  value,
}: CreateTeamBasicStepProps) {
  const owners = useQuery({
    queryKey: ["team-owner-candidates", value.name],
    queryFn: () =>
      listUsers({
        baseUrl: apiBaseUrl,
        fetcher,
        limit: 20,
        offset: 0,
        q: value.name,
        status: "active",
      }),
  });

  return (
    <div className="space-y-5">
      <div className="grid gap-2">
        <Label htmlFor="team-name">团队名称</Label>
        <Input
          id="team-name"
          value={value.name}
          onChange={(event) => onChange({ ...value, name: event.target.value })}
        />
        {errors.name ? (
          <span className="text-sm text-destructive">{errors.name}</span>
        ) : null}
      </div>
      <div className="grid gap-2">
        <Label htmlFor="team-slug">团队标识 slug</Label>
        <Input
          id="team-slug"
          value={value.slug}
          onChange={(event) => onChange({ ...value, slug: event.target.value })}
        />
        {errors.slug ? (
          <span className="text-sm text-destructive">{errors.slug}</span>
        ) : null}
      </div>
      <div className="grid gap-2">
        <span className="text-sm font-medium">负责人</span>
        <div className="divide-y rounded-md border">
          {(owners.data?.items ?? []).map((user) => (
            <button
              className="flex w-full items-center justify-between px-3 py-2 text-left text-sm hover:bg-muted"
              key={user.id}
              onClick={() => onChange({ ...value, owner: user })}
              type="button"
            >
              <span>{user.username}</span>
              <span className="text-muted-foreground">{user.status}</span>
            </button>
          ))}
        </div>
        {value.owner ? (
          <span className="text-sm text-muted-foreground">
            已选择：{value.owner.username}
          </span>
        ) : null}
        {errors.owner ? (
          <span className="text-sm text-destructive">{errors.owner}</span>
        ) : null}
      </div>
    </div>
  );
}

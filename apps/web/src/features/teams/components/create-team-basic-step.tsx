import { useState } from "react";
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
  const [ownerQuery, setOwnerQuery] = useState("");
  const owners = useQuery({
    queryKey: ["team-owner-candidates", ownerQuery],
    queryFn: () =>
      listUsers({
        baseUrl: apiBaseUrl,
        fetcher,
        limit: 20,
        offset: 0,
        q: ownerQuery,
        status: "active",
      }),
  });
  const ownerItems = owners.data?.items ?? [];

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
        <Label htmlFor="team-owner-search">负责人</Label>
        <Input
          id="team-owner-search"
          placeholder="搜索负责人用户名、姓名或邮箱"
          value={ownerQuery}
          onChange={(event) => setOwnerQuery(event.target.value)}
        />
        <div className="divide-y rounded-md border">
          {ownerItems.map((user) => {
            const selected = user.id === value.owner?.id;
            return (
              <button
                className={[
                  "flex w-full items-center justify-between px-3 py-2 text-left text-sm hover:bg-muted",
                  selected ? "bg-[var(--superteam-menu-accent-soft)]" : "",
                ].join(" ")}
                key={user.id}
                onClick={() => onChange({ ...value, owner: user })}
                type="button"
              >
                <span>{user.username}</span>
                <span className="text-muted-foreground">
                  {selected ? "已选择" : user.status}
                </span>
              </button>
            );
          })}
          {owners.isLoading ? (
            <div className="px-3 py-2 text-sm text-muted-foreground">
              加载负责人候选中
            </div>
          ) : null}
          {!owners.isLoading && ownerItems.length === 0 ? (
            <div className="px-3 py-2 text-sm text-muted-foreground">
              未找到匹配的负责人
            </div>
          ) : null}
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

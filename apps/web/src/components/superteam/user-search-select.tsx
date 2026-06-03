import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import type { UserSummary } from "@/lib/api";
import { listUsers } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { UserIdentity } from "./user-identity";

export type UserSearchSelectProps = {
  apiBaseUrl: string;
  excludedUserIds?: string[];
  fetcher?: typeof fetch;
  onSelect: (user: UserSummary) => void;
  placeholder?: string;
  value?: UserSummary;
};

export function UserSearchSelect({
  apiBaseUrl,
  excludedUserIds = [],
  fetcher,
  onSelect,
  placeholder = "搜索用户",
  value,
}: UserSearchSelectProps) {
  const [q, setQ] = useState("");
  const usersQuery = useQuery({
    queryFn: () =>
      listUsers({
        baseUrl: apiBaseUrl,
        fetcher,
        limit: 20,
        offset: 0,
        q,
        status: "active",
      }),
    queryKey: ["superteam", "user-search-select", apiBaseUrl, q],
  });
  const excluded = new Set(excludedUserIds);
  const users = (usersQuery.data?.items ?? []).filter((user) => !excluded.has(user.id));

  return (
    <div className="flex min-w-0 flex-col gap-2">
      {value ? (
        <div className="rounded-md border bg-muted/30 p-2">
          <UserIdentity showSecondary user={value} />
        </div>
      ) : null}
      <Input
        aria-label={placeholder}
        onChange={(event) => setQ(event.target.value)}
        placeholder={placeholder}
        type="search"
        value={q}
      />
      <div className="flex min-w-0 flex-col gap-1">
        {usersQuery.isLoading ? (
          <p className="px-2 py-1.5 text-sm text-muted-foreground">加载用户中</p>
        ) : usersQuery.isError ? (
          <p className="px-2 py-1.5 text-sm text-destructive">用户加载失败</p>
        ) : users.length === 0 ? (
          <p className="px-2 py-1.5 text-sm text-muted-foreground">暂无匹配用户</p>
        ) : (
          users.map((user) => (
            <Button
              aria-label={`选择 ${user.username}`}
              className="h-auto justify-start rounded-md px-2 py-2"
              key={user.id}
              onClick={() => onSelect(user)}
              type="button"
              variant="ghost"
            >
              <UserIdentity showSecondary user={user} />
            </Button>
          ))
        )}
      </div>
    </div>
  );
}

import { useQuery } from "@tanstack/react-query";
import { listUsers } from "@/lib/api";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";

const apiBaseUrl = resolveControlPlaneUrl();

export function Users() {
  const usersQuery = useQuery({
    queryKey: ["users"],
    queryFn: () => listUsers({ baseUrl: apiBaseUrl, limit: 50, offset: 0 }),
  });
  const users = usersQuery.data?.items ?? [];

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        <div className="mb-4">
          <h1 className="text-2xl font-bold tracking-tight">用户管理</h1>
          <p className="text-sm text-muted-foreground">展示 Control Plane 返回的真实用户列表。</p>
        </div>
        <Card>
          <CardHeader>
            <CardTitle>用户列表</CardTitle>
            <CardDescription>当前阶段只读展示，写操作后续按权限和审计要求接入。</CardDescription>
          </CardHeader>
          <CardContent>
            {usersQuery.isLoading ? (
              <p className="text-sm text-muted-foreground">加载中...</p>
            ) : usersQuery.isError ? (
              <p className="text-sm text-destructive">用户列表加载失败。</p>
            ) : users.length === 0 ? (
              <p className="text-sm text-muted-foreground">暂无用户记录。</p>
            ) : (
              <div className="divide-y rounded-md border">
                {users.map((user) => (
                  <div key={user.id} className="flex items-center justify-between p-3 text-sm">
                    <span className="font-medium">{user.username}</span>
                    <Badge variant={user.status === "active" ? "default" : "secondary"}>{user.status}</Badge>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </Main>
    </>
  );
}

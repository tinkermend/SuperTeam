import { PlatformForbiddenState } from "@superteam/views";

import { ConsoleAppShell } from "@/console-app-shell";

export default function ForbiddenPage() {
  return (
    <ConsoleAppShell
      pageDescription="当前账号没有访问该资源的权限。细粒度角色权限后续接入统一授权接口。"
      pageTitle="权限不足"
    >
      <PlatformForbiddenState description="请联系平台管理员确认账号权限或切换到具备访问权限的账号。" title="权限不足" />
    </ConsoleAppShell>
  );
}

import { PlatformLoadingState } from "@superteam/views";

import { ConsoleAppShell } from "@/console-app-shell";

export default function Loading() {
  return (
    <ConsoleAppShell pageTitle="加载中">
      <PlatformLoadingState description="正在准备控制台页面。" title="页面加载中" />
    </ConsoleAppShell>
  );
}

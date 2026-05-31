import { PlatformErrorState } from "@superteam/views";

import { ConsoleAppShell } from "@/console-app-shell";

export default function NotFound() {
  return (
    <ConsoleAppShell pageDescription="请求的控制台页面不存在或尚未开放。" pageTitle="页面不存在">
      <PlatformErrorState description="请返回首页或从左侧导航进入已开放的控制台模块。" title="页面不存在" />
    </ConsoleAppShell>
  );
}

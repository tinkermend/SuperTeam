"use client";

import { RefreshCw } from "lucide-react";

import { PlatformErrorState } from "@superteam/views";

import { Button } from "@/components/ui/button";
import { ConsoleAppShell } from "@/console-app-shell";

export default function ErrorPage({ reset }: { error: Error & { digest?: string }; reset: () => void }) {
  return (
    <ConsoleAppShell pageDescription="控制台渲染过程中发生异常。" pageTitle="页面异常">
      <PlatformErrorState
        action={
          <Button onClick={reset} type="button" variant="outline">
            <RefreshCw data-icon="inline-start" aria-hidden="true" />
            重试
          </Button>
        }
        description="请重试当前操作；如果问题持续出现，请保留审计时间和页面路径。"
        title="页面异常"
      />
    </ConsoleAppShell>
  );
}

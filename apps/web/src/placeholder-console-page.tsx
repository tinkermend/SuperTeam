"use client";

import { Construction } from "lucide-react";

import { IconBadge, SectionPanel } from "@superteam/ui";

import { ConsoleAppShell } from "@/console-app-shell";

type PlaceholderConsolePageProps = {
  description: string;
  title: string;
};

export function PlaceholderConsolePage({ description, title }: PlaceholderConsolePageProps) {
  return (
    <ConsoleAppShell
      pageDescription={description}
      pageTitle={title}
    >
      <SectionPanel
        description="当前先保留一级菜单、路由和页面说明，具体业务功能会在主链路稳定后逐步接入。"
        title="页面功能暂不开发"
      >
        <div className="flex min-h-64 min-w-0 flex-col items-center justify-center gap-4 rounded-md border border-dashed bg-muted/20 px-4 py-10 text-center">
          <IconBadge label={`${title} 占位`} tone="neutral">
            <Construction aria-hidden="true" />
          </IconBadge>
          <div className="min-w-0 max-w-md">
            <p className="text-base font-medium">{title}</p>
            <p className="mt-2 break-all text-sm text-muted-foreground sm:break-normal">{description}</p>
          </div>
        </div>
      </SectionPanel>
    </ConsoleAppShell>
  );
}

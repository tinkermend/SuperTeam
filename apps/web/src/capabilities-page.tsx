"use client";

import { Braces, Network } from "lucide-react";

import { IconBadge, SectionPanel } from "@superteam/ui";

import { ConsoleAppShell } from "@/console-app-shell";

const capabilityTargets = [
  "Dify Workflow",
  "Deephub Agent",
  "企业内部 HTTP 接口",
  "数据分析服务",
  "ITSM 工单接口",
  "CMDB / 监控 / 日志平台",
  "自研脚本服务",
  "后续的 MCP Server / Connector",
];

export function CapabilitiesPage() {
  return (
    <ConsoleAppShell
      pageDescription="页面功能暂不开发，后续用于统一注册、授权、调用和审计外部能力。"
      pageTitle="外部能力"
    >
      <SectionPanel
        description="这里会承载 Capability Integration Layer 的外部能力目录、调用配置、授权边界和审计入口。"
        title="外部能力扩展范围"
      >
        <div className="flex min-w-0 flex-col gap-4">
          <div className="flex min-w-0 items-start gap-3 rounded-md border border-dashed bg-muted/20 p-4">
            <IconBadge label="外部能力占位" tone="accent">
              <Network aria-hidden="true" />
            </IconBadge>
            <div className="min-w-0">
              <p className="text-base font-medium">暂不开发具体页面功能</p>
              <p className="mt-1 text-sm text-muted-foreground">
                当前仅保留一级菜单和说明入口，避免提前固化外部能力的注册表、鉴权、调用编排和审计模型。
              </p>
            </div>
          </div>

          <ul aria-label="外部能力后续扩展对象" className="grid gap-3 md:grid-cols-2">
            {capabilityTargets.map((target) => (
              <li className="flex min-w-0 items-center gap-3 rounded-md border bg-background p-3" key={target}>
                <IconBadge label={`${target} 接入对象`} tone="neutral">
                  <Braces aria-hidden="true" />
                </IconBadge>
                <span className="min-w-0 break-words text-sm font-medium">{target}</span>
              </li>
            ))}
          </ul>
        </div>
      </SectionPanel>
    </ConsoleAppShell>
  );
}

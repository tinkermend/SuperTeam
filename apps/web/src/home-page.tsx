"use client";

import {
  AlertTriangle,
  CheckCircle2,
  ClipboardCheck,
  Factory,
  Gauge,
} from "lucide-react";

import { IconBadge, MetricTile, SectionPanel, StatusPill, TimelineItem } from "@superteam/ui";
import { ConsoleHealthView } from "@superteam/views";

import { Button } from "@/components/ui/button";
import { ConsoleAppShell, DefaultConsolePageActions } from "@/console-app-shell";

type HomePageProps = {
  summary: string;
};

const metrics = [
  {
    icon: <IconBadge label="待确认任务" tone="warning"><ClipboardCheck aria-hidden="true" /></IconBadge>,
    label: "待确认任务",
    meta: "需求分析后等待人工确认",
    status: <StatusPill tone="warning">待处理</StatusPill>,
    value: "3",
  },
  {
    icon: <IconBadge label="Runtime 节点" tone="info"><Gauge aria-hidden="true" /></IconBadge>,
    label: "Runtime 节点",
    meta: "服务器与开发机节点占位",
    status: <StatusPill tone="info">待注册</StatusPill>,
    value: "0",
  },
  {
    icon: <IconBadge label="执行工件" tone="success"><CheckCircle2 aria-hidden="true" /></IconBadge>,
    label: "执行工件",
    meta: "日志、报告和附件统一归档",
    status: <StatusPill tone="success">可追踪</StatusPill>,
    value: "12",
  },
  {
    icon: <IconBadge label="风险事件" tone="danger"><AlertTriangle aria-hidden="true" /></IconBadge>,
    label: "风险事件",
    meta: "高风险动作进入审批",
    status: <StatusPill tone="danger">需确认</StatusPill>,
    value: "1",
  },
];

const timelineItems = [
  {
    title: "需求澄清进入人工确认",
    description: "数字员工已生成结构化 DecisionRequest，等待负责人处理。",
    status: "待确认",
    time: "11:24",
  },
  {
    title: "Runtime Agent 注册接口开发中",
    description: "节点心跳、claim 与 lease 链路由后端工程师推进。",
    status: "进行中",
    time: "10:42",
  },
  {
    title: "Web 登录页前后端联调",
    description: "登录完成后将进入当前控制台外壳。",
    status: "进行中",
    time: "09:18",
  },
];

export function HomePage({ summary }: HomePageProps) {
  return (
    <ConsoleAppShell
      pageActions={<DefaultConsolePageActions />}
      pageDescription="外部系统骨架和复用组件已先行沉淀，真实任务、审批、Runtime、工件和审计数据可在接口稳定后逐步接入。"
      pageTitle="首页"
    >
      <section className="grid min-w-0 gap-3 md:grid-cols-2 xl:grid-cols-4">
        {metrics.map((metric) => (
          <MetricTile
            icon={metric.icon}
            key={metric.label}
            label={metric.label}
            meta={metric.meta}
            status={metric.status}
            value={metric.value}
          />
        ))}
      </section>

      <div className="grid min-w-0 gap-5 xl:grid-cols-[minmax(0,1fr)_360px]">
        <SectionPanel
          action={
            <Button variant="outline">
              <CheckCircle2 data-icon="inline-start" aria-hidden="true" />
              查看建设清单
            </Button>
          }
          description="固定 MVP 链路：需求分析 -> 人类确认 -> Runtime Agent 执行 -> 工件回传 -> 人类验收。"
          title="页面正在开发中"
        >
          <div className="flex min-h-72 min-w-0 flex-col items-center justify-center gap-4 rounded-md border border-dashed bg-muted/20 px-4 text-center">
            <IconBadge label="主链路工作区" tone="accent">
              <Factory aria-hidden="true" />
            </IconBadge>
            <div className="min-w-0 max-w-md">
              <p className="text-base font-medium">标准任务工作区</p>
              <p className="mt-2 break-all text-sm text-muted-foreground sm:break-normal">
                这里会承载任务详情、审批、执行日志、工件和验收结果。当前只使用 mock 内容，不接入认证或真实 API。
              </p>
            </div>
          </div>
        </SectionPanel>

        <div className="flex min-w-0 flex-col gap-5">
          <ConsoleHealthView summary={summary} />
          <SectionPanel description="等待真实审计事件接入。" title="近期活动">
            <ol className="flex flex-col gap-3">
              {timelineItems.map((item) => (
                <TimelineItem
                  description={item.description}
                  key={item.title}
                  status={item.status}
                  time={item.time}
                  title={item.title}
                />
              ))}
            </ol>
          </SectionPanel>
        </div>
      </div>
    </ConsoleAppShell>
  );
}

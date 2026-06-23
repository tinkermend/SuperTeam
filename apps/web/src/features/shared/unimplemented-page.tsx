import type { LucideIcon } from "lucide-react";
import { IconTile, SoftCard, type V3Tone } from "@/components/superteam";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";

type LegacyTone = "primary" | "success" | "warning" | "neutral" | "decision";
type UnimplementedTone = V3Tone | LegacyTone;

const toneMap: Record<UnimplementedTone, V3Tone> = {
  artifact: "artifact",
  brand: "brand",
  danger: "danger",
  decision: "ok",
  info: "info",
  mute: "mute",
  neutral: "mute",
  ok: "ok",
  primary: "brand",
  success: "ok",
  warn: "warn",
  warning: "warn",
};

type UnimplementedPageProps = {
  description: string;
  icon: LucideIcon;
  title: string;
  tone?: UnimplementedTone;
};

export function UnimplementedPage({ description, icon: Icon, title, tone = "neutral" }: UnimplementedPageProps) {
  const v3Tone = toneMap[tone];

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        <div className="mb-5 flex min-w-0 items-center gap-3">
          <IconTile tone={v3Tone} size="lg">
            <Icon />
          </IconTile>
          <div>
            <h1 className="text-[28px] leading-tight font-extrabold tracking-tight text-v3-ink">{title}</h1>
            <p className="mt-1 text-[13px] text-v3-ink-2">{description}</p>
          </div>
        </div>
        <SoftCard className="p-6">
          <div className="flex flex-col gap-2">
            <h2 className="text-base font-bold text-v3-ink">功能建设中</h2>
            <p className="text-[13px] text-v3-ink-2">当前页面只保留导航入口，不使用 mock 数据冒充真实业务能力。</p>
          </div>
          <p className="mt-5 max-w-2xl text-[13px] leading-6 text-v3-ink-2">
            后续实现会从 Control Plane API 获取真实数据，并按任务、审批、审计和工件边界逐步接入。
          </p>
        </SoftCard>
      </Main>
    </>
  );
}

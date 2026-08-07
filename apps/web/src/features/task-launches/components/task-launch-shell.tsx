import type { ReactNode } from "react";
import { SendHorizontal } from "lucide-react";
import { Main, type MainWidth } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import { cn } from "@/lib/utils";
import "./task-launch-aurora.css";

type TaskLaunchShellProps = {
  children: ReactNode;
  description?: string;
  /** 页签条（任务中枢「提出任务 / 流程实例」）：渲染在画布顶部、hero 之上。 */
  tabs?: ReactNode;
  title: string;
  /**
   * 内容宽度：compose 用 canvas（极光画布），instances 用 wide（河道宽台）。
   * 两态共享同一壳与极光背景，仅宽度按内容需求区分。
   */
  width?: MainWidth;
};

export function TaskLaunchShell({
  children,
  description,
  tabs,
  title,
  width = "canvas",
}: TaskLaunchShellProps) {
  return (
    <>
      <ShellPageHeader
        icon={<SendHorizontal />}
        iconTone="brand"
        subtitle={description}
        title={title}
      />
      <Main
        width={width}
        className={cn(
          "tl-aurora p-0",
          width === "wide" && "min-w-0 overflow-x-hidden",
        )}
      >
        {tabs ? (
          <div className="relative z-[1] mx-auto mb-6 w-full max-w-[940px]">
            {tabs}
          </div>
        ) : null}
        <div
          className={
            width === "wide" ? "relative z-[1] w-full min-w-0" : "tl-stage"
          }
        >
          {children}
        </div>
      </Main>
    </>
  );
}

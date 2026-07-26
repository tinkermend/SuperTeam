import type { ReactNode } from "react";
import { SendHorizontal } from "lucide-react";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";
import "./task-launch-aurora.css";

type TaskLaunchShellProps = {
  children: ReactNode;
  description?: string;
  /** 页签条（任务中枢「提出任务 / 流程实例」）：渲染在画布顶部、hero 之上。 */
  tabs?: ReactNode;
  title: string;
};

export function TaskLaunchShell({
  children,
  description,
  tabs,
  title
}: TaskLaunchShellProps) {
  return (
    <>
      <ShellPageHeader
        icon={<SendHorizontal />}
        iconTone="brand"
        subtitle={description}
        title={title}
      />
      <Main width="canvas" className="tl-aurora p-0">
        {tabs ? (
          <div className="relative z-[1] mx-auto mb-6 w-full max-w-[940px]">{tabs}</div>
        ) : null}
        <div className="tl-stage">{children}</div>
      </Main>
    </>
  );
}

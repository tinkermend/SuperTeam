import type { ReactNode } from "react";
import { GitBranch } from "lucide-react";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";

type WorkflowShellProps = {
  children: ReactNode;
};

export function WorkflowShell({ children }: WorkflowShellProps) {
  return (
    <>
      <ShellPageHeader
        icon={<GitBranch />}
        iconTone="info"
        subtitle="查看需求触发的规划、执行、阻塞和结果状态"
        title="流程编排"
      />
      <Main className="min-w-0 overflow-x-hidden">
        <div className="flex min-w-0 flex-col gap-5">
          {children}
        </div>
      </Main>
    </>
  );
}

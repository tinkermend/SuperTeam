import type { ReactNode } from "react";
import { GitBranch } from "lucide-react";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { IconTile, V3PageHeader } from "@/components/superteam";
import { ThemeSwitch } from "@/components/theme-switch";

type WorkflowShellProps = {
  children: ReactNode;
};

export function WorkflowShell({ children }: WorkflowShellProps) {
  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main className="min-w-0 overflow-x-hidden bg-v3-bg">
        <div className="flex min-w-0 flex-col gap-5">
          <V3PageHeader
            back={
              <IconTile tone="info" size="lg">
                <GitBranch />
              </IconTile>
            }
            subtitle="查看需求触发的规划、执行、阻塞和结果状态"
            title="流程编排"
          />
          {children}
        </div>
      </Main>
    </>
  );
}

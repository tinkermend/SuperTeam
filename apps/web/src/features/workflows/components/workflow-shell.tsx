import type { ReactNode } from "react";
import { GitBranch } from "lucide-react";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { SemanticIconTile } from "@/components/superteam";
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
      <Main className="min-w-0 overflow-x-hidden">
        <div className="flex flex-col gap-5">
          <div className="flex min-w-0 items-center gap-3">
            <SemanticIconTile tone="info" size="lg">
              <GitBranch />
            </SemanticIconTile>
            <div className="min-w-0">
              <h1 className="text-2xl font-bold tracking-normal">流程编排</h1>
              <p className="text-sm text-muted-foreground">
                查看需求触发的规划、执行、阻塞和结果状态
              </p>
            </div>
          </div>
          {children}
        </div>
      </Main>
    </>
  );
}

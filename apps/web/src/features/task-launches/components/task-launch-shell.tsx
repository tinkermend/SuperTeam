import type { ReactNode } from "react";
import { SendHorizontal } from "lucide-react";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";

type TaskLaunchShellProps = {
  children: ReactNode;
  description?: string;
  title: string;
};

export function TaskLaunchShell({
  children,
  description,
  title,
}: TaskLaunchShellProps) {
  return (
    <>
      <ShellPageHeader
        icon={<SendHorizontal />}
        iconTone="brand"
        subtitle={description}
        title={title}
      />
      <Main>
        <div className="flex flex-col gap-5">
          {children}
        </div>
      </Main>
    </>
  );
}

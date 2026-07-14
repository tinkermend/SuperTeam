import type { ReactNode } from "react";
import { FolderKanban } from "lucide-react";
import { Main } from "@/components/layout/main";
import { ShellPageHeader } from "@/components/layout/shell-page-header";

type ProjectManagementShellProps = {
  actions?: ReactNode;
  back?: ReactNode;
  children: ReactNode;
  description?: string;
  title: string;
};

export function ProjectManagementShell({
  actions,
  back,
  children,
  description,
  title,
}: ProjectManagementShellProps) {
  return (
    <>
      <ShellPageHeader
        back={back}
        icon={<FolderKanban />}
        iconTone="brand"
        subtitle={description}
        title={title}
      />
      <Main width="wide">
        <div className="flex flex-col gap-5">
          {actions ? (
            <div className="flex flex-wrap items-center justify-start gap-2 sm:justify-end">
              {actions}
            </div>
          ) : null}
          {children}
        </div>
      </Main>
    </>
  );
}

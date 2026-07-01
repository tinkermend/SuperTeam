import type { ReactNode } from "react";
import { FolderKanban } from "lucide-react";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import { V3PageHeader } from "@/components/superteam";

type ProjectManagementShellProps = {
  actions?: ReactNode;
  children: ReactNode;
  description?: string;
  title: string;
};

export function ProjectManagementShell({
  actions,
  children,
  description,
  title,
}: ProjectManagementShellProps) {
  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        <div className="flex flex-col gap-5">
          <V3PageHeader
            icon={<FolderKanban />}
            iconTone="brand"
            title={title}
            subtitle={description}
            actions={actions}
          />
          {children}
        </div>
      </Main>
    </>
  );
}

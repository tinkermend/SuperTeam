import type { ReactNode } from "react";
import { SendHorizontal } from "lucide-react";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import { IconTile } from "@/components/superteam";

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
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main>
        <div className="flex flex-col gap-5">
          <div className="flex min-w-0 items-center gap-3">
            <IconTile tone="brand" size="lg">
              <SendHorizontal />
            </IconTile>
            <div className="min-w-0">
              <h1 className="text-2xl font-extrabold tracking-normal text-v3-ink">{title}</h1>
              {description ? (
                <p className="text-sm text-v3-ink-2">{description}</p>
              ) : null}
            </div>
          </div>
          {children}
        </div>
      </Main>
    </>
  );
}

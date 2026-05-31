import { Bot } from "lucide-react";

type AuthLayoutProps = {
  children: React.ReactNode;
};

export function AuthLayout({ children }: AuthLayoutProps) {
  return (
    <div className="container grid h-svh max-w-none items-center justify-center">
      <div className="mx-auto flex w-full flex-col justify-center space-y-2 py-8 sm:p-8">
        <div className="mb-4 flex items-center justify-center gap-2">
          <div className="flex size-9 items-center justify-center rounded-md border bg-muted">
            <Bot className="size-5" />
          </div>
          <h1 className="text-xl font-medium">SuperTeam</h1>
        </div>
        {children}
      </div>
    </div>
  );
}

import type { AnchorHTMLAttributes, ComponentProps, ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";
import { Header } from "@/components/layout/header";
import { cn } from "@/lib/utils";
import { Button, PageHeader } from "@/components/superteam";

type ShellPageHeaderProps = Omit<ComponentProps<typeof PageHeader>, "action" | "variant">;

type ShellPageHeaderBackProps = Omit<
  AnchorHTMLAttributes<HTMLAnchorElement>,
  "aria-label" | "children" | "href"
> & {
  ariaLabel: string;
  children?: ReactNode;
  className?: string;
  hash?: string;
  params?: Record<string, string>;
  search?: unknown;
  to: string;
};

export function ShellPageHeader(props: ShellPageHeaderProps) {
  return (
    <Header showSidebarTrigger={false}>
      <PageHeader {...props} variant="shell" />
    </Header>
  );
}

export function ShellPageHeaderBack({
  ariaLabel,
  children,
  className,
  ...props
}: ShellPageHeaderBackProps) {
  const linkProps = props as ComponentProps<typeof Link>;

  return (
    <Button
      asChild
      className={cn("h-10 w-10 shrink-0 border-line", className)}
      size="icon"
      variant="outline"
    >
      <Link aria-label={ariaLabel} {...linkProps}>
        {children ?? <ArrowLeft aria-hidden className="size-4" />}
      </Link>
    </Button>
  );
}

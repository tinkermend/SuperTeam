import type { AnchorHTMLAttributes, ComponentProps, ReactNode } from "react";
import { Link } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";
import { Header } from "@/components/layout/header";
import { cn } from "@/lib/utils";
import { Button, PageHeader } from "@/components/superteam";

/**
 * 壳顶栏页头：只承载标题 / 副标题 / 返回 / 图标。
 * 故意去掉 `action`/`actions`——顶栏留给命令搜索、主题与账户；
 * 新建/上传/刷新/分段筛选等一律放 `Main` 内容区（见 page-archetypes「实体目录」）。
 */
type ShellPageHeaderProps = Omit<
  ComponentProps<typeof PageHeader>,
  "action" | "actions" | "variant"
>;

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

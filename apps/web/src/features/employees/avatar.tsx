import { Bot } from "lucide-react";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import type { DigitalEmployeeAvatarAsset } from "@/lib/api/employees";
import { cn } from "@/lib/utils";

type EmployeeAvatarSize = "sm" | "md" | "lg" | "xl" | "xxl" | "hero" | "detail";

type EmployeeAvatarProps = {
  asset?: DigitalEmployeeAvatarAsset | null;
  name: string;
  size?: EmployeeAvatarSize;
};

const sizeClass: Record<EmployeeAvatarSize, string> = {
  sm: "size-9",
  md: "size-10",
  lg: "size-12",
  xl: "size-16",
  xxl: "size-[72px]",
  hero: "size-20",
  detail: "size-24",
};

const iconClass: Record<EmployeeAvatarSize, string> = {
  sm: "size-4",
  md: "size-4",
  lg: "size-5",
  xl: "size-6",
  xxl: "size-7",
  hero: "size-7",
  detail: "size-8",
};

export function EmployeeAvatar({ asset, name, size = "md" }: EmployeeAvatarProps) {
  return (
    <Avatar className={cn("border bg-muted shadow-sm", sizeClass[size])}>
      {asset?.thumbnail_url ? (
        <AvatarImage alt={`${name} 的头像`} className="object-cover" src={asset.thumbnail_url} />
      ) : null}
      <AvatarFallback aria-label={`${name} 的头像`} className="text-muted-foreground">
        <Bot className={iconClass[size]} />
      </AvatarFallback>
    </Avatar>
  );
}

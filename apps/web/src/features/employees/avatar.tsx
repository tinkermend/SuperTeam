import { Bot } from "lucide-react";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import type { DigitalEmployeeAvatarAsset } from "@/lib/api/employees";
import { cn } from "@/lib/utils";

type EmployeeAvatarProps = {
  asset?: DigitalEmployeeAvatarAsset | null;
  name: string;
  size?: "sm" | "md" | "lg";
};

const sizeClass = {
  sm: "size-9",
  md: "size-10",
  lg: "size-12",
};

export function EmployeeAvatar({ asset, name, size = "md" }: EmployeeAvatarProps) {
  return (
    <Avatar className={cn("border bg-muted shadow-sm", sizeClass[size])}>
      {asset?.thumbnail_url ? (
        <AvatarImage alt={`${name} 的头像`} className="object-cover" src={asset.thumbnail_url} />
      ) : null}
      <AvatarFallback aria-label={`${name} 的头像`} className="text-muted-foreground">
        <Bot />
      </AvatarFallback>
    </Avatar>
  );
}

import { useMemo, useState } from "react";
import { ChevronDown, Search } from "lucide-react";
import { DynamicIcon, iconNames } from "lucide-react/dynamic";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";
import { TeamIconTile } from "./team-icon-tile";

const allIconNames = iconNames as readonly string[];
const iconNameSet = new Set(allIconNames);

/** 默认平铺的精选图标（lucide kebab 名），按当前版本是否存在过滤。 */
const curatedIcons = [
  "users-round", "code", "code-xml", "terminal", "server", "server-cog",
  "cpu", "database", "cloud", "boxes", "package", "layers",
  "git-branch", "bot", "brain", "shield", "shield-check", "lock",
  "key", "bug", "wrench", "hammer", "settings", "cog",
  "briefcase", "building-2", "globe", "network", "rocket", "sparkles",
  "zap", "flame", "star", "target", "flag", "compass",
  "gauge", "activity", "chart-line", "chart-pie", "trending-up", "megaphone",
  "mail", "message-square", "phone", "headphones", "user-cog", "graduation-cap",
  "stethoscope", "pill", "leaf", "palette", "pen-tool", "wand-sparkles",
].filter((name) => iconNameSet.has(name));

const MAX_SEARCH_RESULTS = 60;

type TeamIconPickerProps = {
  colorTone: string;
  onSelect: (iconKey: string) => void;
  value: string;
};

export function TeamIconPicker({
  colorTone,
  onSelect,
  value,
}: TeamIconPickerProps) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");

  const results = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return curatedIcons;
    return allIconNames
      .filter((name) => name.includes(q))
      .slice(0, MAX_SEARCH_RESULTS);
  }, [query]);

  return (
    <Popover onOpenChange={setOpen} open={open}>
      <PopoverTrigger asChild>
        <Button
          aria-label="选择团队图标"
          className="h-auto w-fit justify-start gap-2.5 px-2.5 py-2"
          type="button"
          variant="outline"
        >
          <TeamIconTile
            metadata={{ display: { color_tone: colorTone, icon_key: value } }}
          />
          <span className="font-mono text-xs text-muted-foreground">{value}</span>
          <ChevronDown className="size-4 text-muted-foreground" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-[320px] p-3">
        <div className="relative">
          <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            aria-label="搜索图标"
            className="pl-8"
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索图标（英文名）"
            type="search"
            value={query}
          />
        </div>
        <ScrollArea className="mt-3 h-56">
          {results.length === 0 ? (
            <p className="px-2 py-6 text-center text-sm text-muted-foreground">
              未找到匹配图标
            </p>
          ) : (
            <div className="grid grid-cols-6 gap-1.5 pr-2">
              {results.map((name) => {
                const selected = name === value;
                return (
                  <button
                    aria-label={`选择图标 ${name}`}
                    aria-pressed={selected}
                    className={cn(
                      "flex aspect-square items-center justify-center rounded-md border text-foreground transition [&_svg]:size-4",
                      selected
                        ? "border-primary ring-2 ring-ring/30"
                        : "hover:bg-muted",
                    )}
                    key={name}
                    onClick={() => {
                      onSelect(name);
                      setOpen(false);
                    }}
                    title={name}
                    type="button"
                  >
                    <DynamicIcon name={name as (typeof iconNames)[number]} />
                  </button>
                );
              })}
            </div>
          )}
        </ScrollArea>
        <p className="mt-2 text-[11px] text-muted-foreground">
          来自 lucide 图标库，搜索可检索全部 {allIconNames.length} 个图标。
        </p>
      </PopoverContent>
    </Popover>
  );
}

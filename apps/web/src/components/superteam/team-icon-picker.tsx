import { Button } from '@/components/superteam';
import { useMemo, useState } from "react";
import { ChevronDown, Search, Check } from "lucide-react";
import { Input } from "@/components/ui/input";
import {
  Popover,
  PopoverContent,
  PopoverTrigger
} from "@/components/ui/popover";
import { ScrollArea } from "@/components/ui/scroll-area";
import { cn } from "@/lib/utils";
import { TeamIconTile } from "./team-icon-tile";
import { getTeamRoleIcon, teamRoleIcons } from "./team-role-icon-catalog";

const MAX_SEARCH_RESULTS = 60;

type TeamIconPickerProps = {
  colorTone: string;
  onSelect: (iconKey: string) => void;
  value: string;
};

export function TeamIconPicker({
  colorTone,
  onSelect,
  value
}: TeamIconPickerProps) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const selectedRoleIcon = getTeamRoleIcon(value);

  const results = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (q) {
      const roleResults = teamRoleIcons
        .filter((icon) =>
          [icon.key, icon.label, ...icon.keywords]
            .join(" ")
            .toLowerCase()
            .includes(q),
        )
        .map((icon) => icon.key);
      return roleResults.slice(0, MAX_SEARCH_RESULTS);
    }

    return teamRoleIcons.map((icon) => icon.key);
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
          <span className="font-mono text-xs text-muted-foreground">
            {selectedRoleIcon?.label ?? value}
          </span>
          <ChevronDown className="size-4 text-muted-foreground" />
        </Button>
      </PopoverTrigger>
      <PopoverContent align="start" className="w-[420px] p-0">
        <div className="p-3">
          <div className="relative mb-3">
            <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
            <Input
              aria-label="搜索图标"
              className="h-9 pl-8 text-xs"
              onChange={(event) => setQuery(event.target.value)}
              placeholder="搜索图标（支持名称或关键词）"
              type="search"
              value={query}
            />
          </div>
          
          <ScrollArea className="h-64">
            {results.length === 0 ? (
              <p className="px-2 py-6 text-center text-sm text-muted-foreground">
                未找到匹配图标
              </p>
            ) : (
              <div className="grid grid-cols-6 gap-2 pr-3 pb-2">
                {results.map((name) => {
                  const roleIcon = getTeamRoleIcon(name);
                  const label = roleIcon?.label ?? name;
                  const selected = name === value;
                  return (
                    <button
                      aria-label={`选择图标 ${label}`}
                      aria-pressed={selected}
                      className={cn(
                        "relative flex aspect-square flex-col items-center justify-center gap-1 rounded-lg border bg-card text-foreground transition-all hover:bg-muted/80",
                        selected
                          ? "border-primary ring-1 ring-primary"
                          : "border-border shadow-sm",
                      )}
                      key={name}
                      onClick={() => {
                        onSelect(name);
                        setOpen(false);
                      }}
                      title={label}
                      type="button"
                    >
                      <img
                        alt=""
                        className="size-8 object-contain"
                        decoding="async"
                        height={32}
                        loading="lazy"
                        src={roleIcon?.src}
                        width={32}
                      />
                      <span className="max-w-[50px] truncate text-[9px] text-muted-foreground">
                        {label}
                      </span>
                      {selected && (
                        <div className="absolute -right-1.5 -top-1.5 rounded-full bg-primary p-0.5 text-primary-foreground">
                          <Check className="!size-2.5" />
                        </div>
                      )}
                    </button>
                  );
                })}
              </div>
            )}
          </ScrollArea>
        </div>
      </PopoverContent>
    </Popover>
  );
}

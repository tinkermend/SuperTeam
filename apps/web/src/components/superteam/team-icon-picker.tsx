import { useMemo, useState } from "react";
import { ChevronDown, Search, Check } from "lucide-react";
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

const CATEGORIES = [
  {
    id: "general",
    label: "通用",
    icons: [
      "user", "users", "users-round", "network", "building-2", "briefcase",
      "folder", "file", "target", "settings", "database", "inbox",
    ],
  },
  {
    id: "rd",
    label: "研发",
    icons: [
      "code", "code-xml", "terminal", "bug", "git-branch", "git-commit",
      "braces", "blocks", "box", "cpu", "sparkles", "wand-sparkles",
    ],
  },
  {
    id: "ops",
    label: "运维",
    icons: [
      "server", "server-cog", "cloud", "cloud-cog", "container", "layers",
      "activity", "gauge", "zap", "radio", "hard-drive", "webhook",
    ],
  },
  {
    id: "security",
    label: "安全",
    icons: [
      "shield", "shield-check", "shield-alert", "lock", "unlock", "key",
      "fingerprint", "scan-face", "eye", "eye-off", "file-key", "user-round-search",
    ],
  },
  {
    id: "delivery",
    label: "交付",
    icons: [
      "package", "package-check", "truck", "plane", "rocket", "box-select",
      "check-circle", "check-square", "clipboard-check", "milestone", "flag", "compass",
    ],
  },
];

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
  const [activeTab, setActiveTab] = useState("general");

  const results = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (q) {
      return allIconNames
        .filter((name) => name.includes(q))
        .slice(0, MAX_SEARCH_RESULTS);
    }
    
    const category = CATEGORIES.find(c => c.id === activeTab);
    return (category?.icons || CATEGORIES[0].icons).filter(name => iconNameSet.has(name));
  }, [query, activeTab]);

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
      <PopoverContent align="start" className="w-[420px] p-0">
        <div className="flex border-b">
          {CATEGORIES.map((cat) => (
            <button
              key={cat.id}
              onClick={() => {
                setQuery("");
                setActiveTab(cat.id);
              }}
              className={cn(
                "flex-1 border-b-2 px-3 py-2.5 text-xs font-medium transition-colors",
                !query && activeTab === cat.id
                  ? "border-primary text-primary"
                  : "border-transparent text-muted-foreground hover:text-foreground hover:bg-muted/50"
              )}
            >
              {cat.label}
            </button>
          ))}
        </div>
        
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
                  const selected = name === value;
                  return (
                    <button
                      aria-label={`选择图标 ${name}`}
                      aria-pressed={selected}
                      className={cn(
                        "relative flex aspect-square flex-col items-center justify-center gap-1 rounded-lg border bg-card text-foreground transition-all hover:bg-muted/80 [&_svg]:size-5",
                        selected
                          ? "border-primary ring-1 ring-primary"
                          : "border-border shadow-sm",
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
                      <span className="max-w-[50px] truncate text-[9px] text-muted-foreground">
                        {name}
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

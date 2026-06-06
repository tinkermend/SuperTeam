import { useEffect, useState } from "react";
import Editor from "@monaco-editor/react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Blocks,
  Bot,
  ChevronDown,
  ChevronRight,
  FileCode2,
  FileText,
  FlaskConical,
  Folder,
  RefreshCw,
  Save,
  Search as SearchIcon,
  ServerCog,
  ShieldCheck,
  Stethoscope,
  TriangleAlert,
  UploadCloud,
  type LucideIcon,
} from "lucide-react";
import {
  LiquidCard,
  LiquidTabsList,
  LiquidTabsTrigger,
  SemanticIconTile,
  StatusBadge,
  type Tone,
} from "@/components/superteam";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Separator } from "@/components/ui/separator";
import { Tabs, TabsContent } from "@/components/ui/tabs";
import { Textarea } from "@/components/ui/textarea";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import { ThemeSwitch } from "@/components/theme-switch";
import { listSkills, updateSkillFile, uploadSkill, type Skill, type SkillFile } from "@/lib/api/skills";
import { listTeams, type TeamListItem } from "@/lib/api/teams";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { cn } from "@/lib/utils";

type SkillsViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
};

type SkillTab = "installed" | "market";

type TreeNode = {
  children: Map<string, TreeNode>;
  name: string;
  path: string;
  type: "dir" | "file";
};

const iconMap: Record<string, LucideIcon> = {
  blocks: Blocks,
  flask: FlaskConical,
  "server-cog": ServerCog,
  "shield-check": ShieldCheck,
  stethoscope: Stethoscope,
};

const toneByColor: Record<string, Tone> = {
  blue: "info",
  cyan: "info",
  emerald: "success",
  teal: "primary",
  violet: "artifact",
};

export function SkillsPage() {
  const apiBaseUrl = resolveControlPlaneUrl();

  return <SkillsView apiBaseUrl={apiBaseUrl} />;
}

export function SkillsView({ apiBaseUrl, fetcher }: SkillsViewProps) {
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<SkillTab>("installed");
  const [query, setQuery] = useState("");
  const [selectedSkillId, setSelectedSkillId] = useState<string>();
  const [selectedPath, setSelectedPath] = useState("SKILL.md");
  const [expandedSkills, setExpandedSkills] = useState<Set<string>>(new Set());
  const [expandedFolders, setExpandedFolders] = useState<Set<string>>(new Set());
  const [draftContent, setDraftContent] = useState("");
  const [uploadOpen, setUploadOpen] = useState(false);

  const apiOptions = { baseUrl: apiBaseUrl, fetcher };
  const skills = useQuery({
    queryKey: ["skills", query],
    queryFn: () => listSkills(apiOptions, { q: query }),
  });
  const teams = useQuery({
    enabled: uploadOpen,
    queryKey: ["skill-upload-teams"],
    queryFn: () => listTeams(apiOptions),
  });

  const skillRows = skills.data ?? [];
  const installedSkills = skillRows.filter((skill) => skill.status === "installed");
  const marketplaceSkills = skillRows;
  const activeSkills = tab === "installed" ? installedSkills : marketplaceSkills;
  const selectedSkill = skillRows.find((skill) => skill.id === selectedSkillId) ?? activeSkills[0];
  const selectedFile = selectedSkill?.files.find((file) => file.path === selectedPath) ?? selectedSkill?.files.find((file) => file.path === "SKILL.md") ?? selectedSkill?.files[0];
  const skillsError = skills.error instanceof Error ? skills.error.message : undefined;

  useEffect(() => {
    if (!selectedSkill && activeSkills[0]) {
      setSelectedSkillId(activeSkills[0].id);
    }
  }, [activeSkills, selectedSkill]);

  useEffect(() => {
    if (selectedSkill) {
      setExpandedSkills((current) => new Set(current).add(selectedSkill.id));
    }
  }, [selectedSkill]);

  useEffect(() => {
    setDraftContent(selectedFile?.content ?? "");
  }, [selectedFile?.content, selectedFile?.path, selectedSkill?.id]);

  const saveFile = useMutation({
    mutationFn: () => {
      if (!selectedSkill || !selectedFile) {
        throw new Error("未选择技能文件");
      }
      return updateSkillFile(apiOptions, selectedSkill.id, selectedFile.path, draftContent);
    },
    onSuccess: (updatedFile) => {
      queryClient.setQueryData<Skill[]>(["skills", query], (current) =>
        current?.map((skill) =>
          skill.id === selectedSkill?.id
            ? {
                ...skill,
                files: skill.files.map((file) => (file.path === updatedFile.path ? { ...file, ...updatedFile } : file)),
              }
            : skill,
        ),
      );
    },
  });

  return (
    <>
      <Header>
        <Search />
        <ThemeSwitch />
      </Header>
      <Main className="min-w-0 overflow-x-hidden">
        <div className="flex min-w-0 flex-col gap-5">
          <div className="flex min-w-0 flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
            <div className="flex items-start gap-3">
              <SemanticIconTile tone="primary" size="sm">
                <Blocks />
              </SemanticIconTile>
              <div className="min-w-0">
                <h1 className="text-2xl font-bold tracking-normal">技能管理</h1>
                <p className="text-sm text-muted-foreground">管理可安装技能、技能市场、文件内容和 Agent 绑定关系。</p>
              </div>
            </div>
            <div className="flex flex-wrap gap-2">
              <Button onClick={() => setUploadOpen(true)} type="button">
                <UploadCloud data-icon="inline-start" />
                上传技能
              </Button>
            </div>
          </div>

          <div className="grid gap-3 md:grid-cols-4">
            <SkillMetric icon={Blocks} label="已安装" value={installedSkills.length} tone="primary" />
            <SkillMetric icon={Bot} label="已绑定 Agent" value={countAgentBindings(skillRows)} tone="info" />
            <SkillMetric icon={RefreshCw} label="待更新" value={0} tone="warning" />
            <SkillMetric icon={UploadCloud} label="市场技能" value={marketplaceSkills.length} tone="artifact" />
          </div>

          {skills.isError ? (
            <Alert variant="destructive">
              <TriangleAlert />
              <AlertTitle>技能数据加载失败</AlertTitle>
              <AlertDescription>{skillsError ?? "请检查 Control Plane 技能接口和数据库迁移状态。"}</AlertDescription>
            </Alert>
          ) : null}

          <Tabs value={tab} onValueChange={(value) => setTab(value as SkillTab)}>
            <LiquidTabsList className="max-w-md">
              <LiquidTabsTrigger value="installed">已安装技能</LiquidTabsTrigger>
              <LiquidTabsTrigger value="market">技能市场</LiquidTabsTrigger>
            </LiquidTabsList>

            <TabsContent value="installed">
              <div className="grid min-h-[650px] gap-4 xl:grid-cols-[280px_minmax(0,1fr)_340px]">
                <SkillTreePanel
                  expandedFolders={expandedFolders}
                  expandedSkills={expandedSkills}
                  onQueryChange={setQuery}
                  onSelectFile={(skill, file) => {
                    setSelectedSkillId(skill.id);
                    setSelectedPath(file.path);
                  }}
                  onToggleFolder={(key) =>
                    setExpandedFolders((current) => toggleSetValue(current, key))
                  }
                  onToggleSkill={(skill) => {
                    setSelectedSkillId(skill.id);
                    setExpandedSkills((current) => toggleSetValue(current, skill.id));
                  }}
                  query={query}
                  selectedPath={selectedFile?.path}
                  selectedSkillId={selectedSkill?.id}
                  skills={installedSkills}
                />
                <SkillEditorPanel
                  content={draftContent}
                  file={selectedFile}
                  isSaving={saveFile.isPending}
                  onChange={setDraftContent}
                  onSave={() => saveFile.mutate()}
                  saveError={saveFile.error instanceof Error ? saveFile.error.message : undefined}
                  skill={selectedSkill}
                />
                <SkillSidePanel skill={selectedSkill} />
              </div>
            </TabsContent>

            <TabsContent value="market">
              <SkillMarket skills={marketplaceSkills} />
            </TabsContent>
          </Tabs>
        </div>
      </Main>
      <SkillUploadDialog
        apiOptions={apiOptions}
        onUploaded={(skill) => {
          setSelectedSkillId(skill.id);
          setSelectedPath("SKILL.md");
          setUploadOpen(false);
          void queryClient.invalidateQueries({ queryKey: ["skills"] });
        }}
        onOpenChange={setUploadOpen}
        open={uploadOpen}
        teams={teams.data ?? []}
      />
    </>
  );
}

function SkillMetric({ icon: Icon, label, tone, value }: { icon: LucideIcon; label: string; tone: Tone; value: number }) {
  return (
    <LiquidCard role="group" aria-label={`${label} ${value}`} className="rounded-lg">
      <CardContent className="flex items-center gap-3 p-4">
        <SemanticIconTile tone={tone} size="sm">
          <Icon />
        </SemanticIconTile>
        <div>
          <p className="text-sm text-muted-foreground">{label}</p>
          <p className="text-2xl font-semibold tracking-normal">{value}</p>
        </div>
      </CardContent>
    </LiquidCard>
  );
}

function SkillTreePanel({
  expandedFolders,
  expandedSkills,
  onQueryChange,
  onSelectFile,
  onToggleFolder,
  onToggleSkill,
  query,
  selectedPath,
  selectedSkillId,
  skills,
}: {
  expandedFolders: Set<string>;
  expandedSkills: Set<string>;
  onQueryChange: (value: string) => void;
  onSelectFile: (skill: Skill, file: SkillFile) => void;
  onToggleFolder: (key: string) => void;
  onToggleSkill: (skill: Skill) => void;
  query: string;
  selectedPath?: string;
  selectedSkillId?: string;
  skills: Skill[];
}) {
  return (
    <LiquidCard className="min-w-0 rounded-lg">
      <CardHeader className="gap-3 border-b">
        <CardTitle className="text-base">已安装技能</CardTitle>
        <div className="flex items-center gap-2 rounded-md border bg-background px-2">
          <SearchIcon className="size-4 text-muted-foreground" />
          <Input
            aria-label="搜索技能或文件"
            className="border-0 px-0 shadow-none focus-visible:ring-0"
            onChange={(event) => onQueryChange(event.target.value)}
            placeholder="搜索技能或文件"
            value={query}
          />
        </div>
      </CardHeader>
      <CardContent className="p-0">
        <ScrollArea className="h-[560px]">
          <div className="flex flex-col gap-1 p-3">
            {skills.map((skill) => {
              const isExpanded = expandedSkills.has(skill.id);
              return (
                <div className="flex flex-col gap-1" key={skill.id}>
                  <button
                    aria-expanded={isExpanded}
                    className={cn(
                      "flex min-w-0 items-start gap-2 rounded-md px-2 py-2 text-left text-sm hover:bg-muted",
                      selectedSkillId === skill.id && "bg-primary/10 text-primary",
                    )}
                    onClick={() => onToggleSkill(skill)}
                    type="button"
                  >
                    {isExpanded ? <ChevronDown className="mt-0.5 size-4 shrink-0" /> : <ChevronRight className="mt-0.5 size-4 shrink-0" />}
                    <SkillIcon skill={skill} size="sm" />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate font-medium">{skill.name}</span>
                      <span className="block truncate text-xs text-muted-foreground">已绑定 {skill.agent_bindings.length} 个 Agent</span>
                    </span>
                  </button>
                  {isExpanded ? (
                    <div className="ms-6 flex flex-col gap-1 border-s ps-2">
                      {renderTreeNodes({
                        expandedFolders,
                        nodes: buildFileTree(skill.files).children,
                        onSelectFile,
                        onToggleFolder,
                        selectedPath,
                        skill,
                      })}
                    </div>
                  ) : null}
                </div>
              );
            })}
          </div>
        </ScrollArea>
      </CardContent>
    </LiquidCard>
  );
}

function SkillEditorPanel({
  content,
  file,
  isSaving,
  onChange,
  onSave,
  saveError,
  skill,
}: {
  content: string;
  file?: SkillFile;
  isSaving: boolean;
  onChange: (value: string) => void;
  onSave: () => void;
  saveError?: string;
  skill?: Skill;
}) {
  return (
    <LiquidCard className="min-w-0 rounded-lg">
      <CardHeader className="gap-3 border-b">
        <div className="flex min-w-0 flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div className="min-w-0">
            <CardTitle className="truncate text-base">
              {skill?.name ?? "未选择技能"} / {file?.path ?? "SKILL.md"}
            </CardTitle>
            <div className="mt-2 flex flex-wrap gap-2">
              {skill?.status ? <StatusBadge tone="success">{skill.status === "installed" ? "已安装" : "可安装"}</StatusBadge> : null}
              {file ? <Badge variant="secondary">{languageForFile(file.path)}</Badge> : null}
              {file ? <Badge variant="outline">{file.size_bytes} bytes</Badge> : null}
            </div>
          </div>
          <Button disabled={!file || isSaving} onClick={onSave} type="button">
            <Save data-icon="inline-start" />
            保存
          </Button>
        </div>
      </CardHeader>
      <CardContent className="p-0">
        {file ? (
          <div className="h-[560px] min-w-0 overflow-hidden rounded-b-lg border-t bg-background">
            <Editor
              height="100%"
              language={languageForFile(file.path)}
              onChange={(value: string | undefined) => onChange(value ?? "")}
              options={{
                fontSize: 13,
                minimap: { enabled: false },
                scrollBeyondLastLine: false,
                wordWrap: "on",
              }}
              theme="vs"
              value={content}
            />
          </div>
        ) : (
          <div className="flex h-[560px] min-w-0 items-center justify-center rounded-b-lg border-t bg-background p-6 text-center text-sm text-muted-foreground">
            请选择左侧技能文件，或先处理技能数据加载失败。
          </div>
        )}
        {saveError ? <p className="p-3 text-sm text-destructive">保存失败：{saveError}</p> : null}
      </CardContent>
    </LiquidCard>
  );
}

function SkillSidePanel({ skill }: { skill?: Skill }) {
  return (
    <div className="flex min-w-0 flex-col gap-4">
      <LiquidCard className="rounded-lg">
        <CardHeader className="border-b">
          <CardTitle className="text-base">Agent 绑定</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3 p-4">
          {(skill?.agent_bindings ?? []).map((binding) => (
            <div className="flex items-center justify-between gap-3 rounded-md border bg-background p-3" key={binding.agent_id}>
              <div className="flex min-w-0 items-center gap-3">
                <SemanticIconTile tone="info" size="sm">
                  <Bot />
                </SemanticIconTile>
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{binding.agent_name}</p>
                  <p className="truncate text-xs text-muted-foreground">{binding.team_name || "未归属团队"}</p>
                </div>
              </div>
              <StatusBadge tone={binding.status === "enabled" ? "success" : "neutral"}>{binding.status === "enabled" ? "已启用" : binding.status}</StatusBadge>
            </div>
          ))}
          {skill && skill.agent_bindings.length === 0 ? <p className="text-sm text-muted-foreground">暂无 Agent 绑定</p> : null}
        </CardContent>
      </LiquidCard>

      <LiquidCard className="rounded-lg">
        <CardHeader className="border-b">
          <CardTitle className="text-base">技能信息</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3 p-4 text-sm">
          <InfoRow label="版本" value={skill?.version ?? "-"} />
          <InfoRow label="来源" value={skill?.source ?? "-"} />
          <InfoRow label="风险等级" value={skill?.risk_level ?? "-"} />
          <Separator />
          <div className="flex flex-wrap gap-2">
            {(skill?.tags ?? []).map((tag) => (
              <Badge key={tag} variant="outline">{tag}</Badge>
            ))}
          </div>
        </CardContent>
      </LiquidCard>
    </div>
  );
}

function SkillMarket({ skills }: { skills: Skill[] }) {
  const uploadedTags = [...new Set(skills.flatMap((skill) => skill.tags))];

  return (
    <div className="grid gap-4 xl:grid-cols-[240px_minmax(0,1fr)]">
      <LiquidCard className="rounded-lg">
        <CardHeader className="border-b">
          <CardTitle className="text-base">上传标签</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-2 p-4 text-sm">
          {uploadedTags.map((tag) => (
            <button className="rounded-md px-2 py-2 text-left hover:bg-muted" key={tag} type="button">
              {tag}
            </button>
          ))}
          {uploadedTags.length === 0 ? <p className="text-sm text-muted-foreground">暂无上传标签</p> : null}
        </CardContent>
      </LiquidCard>
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
        {skills.map((skill) => (
          <LiquidCard className="rounded-lg" key={skill.id}>
            <CardContent className="flex min-h-48 flex-col gap-4 p-4">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <h2 className="truncate text-lg font-semibold tracking-normal">{skill.name}</h2>
                  <p className="mt-2 line-clamp-2 text-sm text-muted-foreground">{skill.description}</p>
                </div>
                <SkillIcon skill={skill} />
              </div>
              <div className="flex flex-wrap gap-2">
                {skill.tags.map((tag) => (
                  <Badge key={tag} variant="outline">{tag}</Badge>
                ))}
              </div>
              <div className="mt-auto flex items-center justify-between gap-3">
                <span className="text-xs text-muted-foreground">{skill.files.length} 个文件</span>
                <Button size="sm" type="button" variant={skill.status === "installed" ? "outline" : "default"}>
                  {skill.status === "installed" ? "已安装 / 更新" : "安装"}
                </Button>
              </div>
            </CardContent>
          </LiquidCard>
        ))}
      </div>
    </div>
  );
}

function SkillUploadDialog({
  apiOptions,
  onOpenChange,
  onUploaded,
  open,
  teams,
}: {
  apiOptions: { baseUrl: string; fetcher?: typeof fetch };
  onOpenChange: (open: boolean) => void;
  onUploaded: (skill: Skill) => void;
  open: boolean;
  teams: TeamListItem[];
}) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [tags, setTags] = useState("");
  const [file, setFile] = useState<File | null>(null);
  const [teamIds, setTeamIds] = useState<Set<string>>(new Set());
  const upload = useMutation({
    mutationFn: () => {
      if (!file) {
        throw new Error("请选择技能 zip 包");
      }
      return uploadSkill(apiOptions, {
        description,
        file,
        name,
        tags: tags.split(",").map((tag) => tag.trim()).filter(Boolean),
        team_ids: [...teamIds],
      });
    },
    onSuccess: (skill) => {
      setName("");
      setDescription("");
      setTags("");
      setFile(null);
      setTeamIds(new Set());
      onUploaded(skill);
    },
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>上传技能</DialogTitle>
          <DialogDescription>通过 zip 包导入技能，并补充描述、标签和归属团队。</DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 md:grid-cols-2">
          <div className="flex flex-col gap-2 md:col-span-2">
            <Label htmlFor="skill-zip">技能 zip 包</Label>
            <Input
              accept=".zip,application/zip"
              id="skill-zip"
              onChange={(event) => setFile(event.target.files?.[0] ?? null)}
              type="file"
            />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="skill-name">技能名称</Label>
            <Input id="skill-name" onChange={(event) => setName(event.target.value)} value={name} />
          </div>
          <div className="flex flex-col gap-2">
            <Label htmlFor="skill-tags">标签</Label>
            <Input id="skill-tags" onChange={(event) => setTags(event.target.value)} placeholder="诊断,自动化" value={tags} />
          </div>
          <div className="flex flex-col gap-2 md:col-span-2">
            <Label htmlFor="skill-description">技能描述</Label>
            <Textarea id="skill-description" onChange={(event) => setDescription(event.target.value)} value={description} />
          </div>
          <div className="flex flex-col gap-2 md:col-span-2">
            <p className="text-sm font-medium">归属团队</p>
            <div className="grid gap-2 sm:grid-cols-2">
              {teams.map((team) => (
                <label className="flex items-center gap-2 rounded-md border bg-background px-3 py-2 text-sm" key={team.id}>
                  <Checkbox
                    checked={teamIds.has(team.id)}
                    onCheckedChange={() => setTeamIds((current) => toggleSetValue(current, team.id))}
                  />
                  {team.name}
                </label>
              ))}
            </div>
          </div>
        </div>
        {upload.error instanceof Error ? <p className="text-sm text-destructive">{upload.error.message}</p> : null}
        <DialogFooter>
          <Button onClick={() => onOpenChange(false)} type="button" variant="outline">取消</Button>
          <Button disabled={!file || !name.trim() || upload.isPending} onClick={() => upload.mutate()} type="button">
            <UploadCloud data-icon="inline-start" />
            上传并安装
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function renderTreeNodes({
  expandedFolders,
  nodes,
  onSelectFile,
  onToggleFolder,
  selectedPath,
  skill,
}: {
  expandedFolders: Set<string>;
  nodes: Map<string, TreeNode>;
  onSelectFile: (skill: Skill, file: SkillFile) => void;
  onToggleFolder: (key: string) => void;
  selectedPath?: string;
  skill: Skill;
}) {
  return [...nodes.values()].map((node) => {
    if (node.type === "dir") {
      const key = `${skill.id}:${node.path}`;
      const expanded = expandedFolders.has(key);
      return (
        <div className="flex flex-col gap-1" key={node.path}>
          <button className="flex items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted" onClick={() => onToggleFolder(key)} type="button">
            {expanded ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
            <Folder className="size-4 text-muted-foreground" />
            {node.name}
          </button>
          {expanded ? (
            <div className="ms-5 flex flex-col gap-1 border-s ps-2">
              {renderTreeNodes({ expandedFolders, nodes: node.children, onSelectFile, onToggleFolder, selectedPath, skill })}
            </div>
          ) : null}
        </div>
      );
    }
    const file = skill.files.find((item) => item.path === node.path);
    if (!file) return null;
    return (
      <button
        className={cn("flex items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-muted", selectedPath === file.path && "bg-primary/10 text-primary")}
        key={node.path}
        onClick={() => onSelectFile(skill, file)}
        type="button"
      >
        {file.path.endsWith(".md") ? <FileText className="size-4" /> : <FileCode2 className="size-4" />}
        {node.name}
      </button>
    );
  });
}

function buildFileTree(files: SkillFile[]) {
  const root: TreeNode = { children: new Map(), name: "", path: "", type: "dir" };
  for (const file of files) {
    const parts = file.path.split("/");
    let current = root;
    parts.forEach((part, index) => {
      const isFile = index === parts.length - 1;
      const currentPath = parts.slice(0, index + 1).join("/");
      if (!current.children.has(part)) {
        current.children.set(part, {
          children: new Map(),
          name: part,
          path: currentPath,
          type: isFile ? "file" : "dir",
        });
      }
      current = current.children.get(part)!;
    });
  }
  return root;
}

function SkillIcon({ size = "default", skill }: { size?: "sm" | "default"; skill: Skill }) {
  const Icon = iconMap[skill.icon_key] ?? Blocks;
  return (
    <SemanticIconTile aria-label={`${skill.name} 图标`} tone={toneByColor[skill.color_token] ?? "primary"} size={size}>
      <Icon />
    </SemanticIconTile>
  );
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="text-muted-foreground">{label}</span>
      <span className="truncate font-medium">{value}</span>
    </div>
  );
}

function countAgentBindings(skills: Skill[]) {
  return skills.reduce((total, skill) => total + skill.agent_bindings.length, 0);
}

function languageForFile(path: string) {
  if (path.endsWith(".md")) return "markdown";
  if (path.endsWith(".sh")) return "shell";
  if (path.endsWith(".py")) return "python";
  if (path.endsWith(".json")) return "json";
  return "plaintext";
}

function toggleSetValue(current: Set<string>, value: string) {
  const next = new Set(current);
  if (next.has(value)) {
    next.delete(value);
  } else {
    next.add(value);
  }
  return next;
}

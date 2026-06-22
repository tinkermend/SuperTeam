import { useMemo, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import {
  ArrowLeft,
  CheckCircle2,
  FileArchive,
  Info,
  PackageCheck,
  UploadCloud,
  X,
} from "lucide-react";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import {
  LiquidCard,
  SemanticIconTile,
} from "@/components/superteam";
import { ThemeSwitch } from "@/components/theme-switch";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { uploadSkill, type Skill } from "@/lib/api/skills";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { cn } from "@/lib/utils";

type SkillUploadViewProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  onUploaded?: (skill: Skill) => void;
};

type ApiOpts = { baseUrl: string; fetcher?: typeof fetch };

const riskOptions = [
  { label: "低", value: "low" },
  { label: "中", value: "medium" },
  { label: "高", value: "high" },
] as const;

export function SkillUploadPage() {
  const apiBaseUrl = resolveControlPlaneUrl();
  const navigate = useNavigate();

  return (
    <SkillUploadView
      apiBaseUrl={apiBaseUrl}
      onUploaded={() => {
        void navigate({ to: "/skills" });
      }}
    />
  );
}

export function SkillUploadView({ apiBaseUrl, fetcher, onUploaded }: SkillUploadViewProps) {
  const apiOptions = useMemo<ApiOpts>(() => ({ baseUrl: apiBaseUrl, fetcher }), [apiBaseUrl, fetcher]);
  const [file, setFile] = useState<File | null>(null);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [riskLevel, setRiskLevel] = useState("medium");
  const [tags, setTags] = useState("");
  const [runtimeTools, setRuntimeTools] = useState("");
  const [runtimeEnv, setRuntimeEnv] = useState("");

  const packageDisplayName = packageNameFromFile(file);
  const tagItems = splitCommaInput(tags);
  const runtimeToolItems = splitCommaInput(runtimeTools);
  const runtimeEnvItems = splitCommaInput(runtimeEnv);
  const dependencyCount = runtimeToolItems.length + runtimeEnvItems.length;
  const canPublish = Boolean(file && name.trim());

  const upload = useMutation({
    mutationFn: () => {
      if (!file) {
        throw new Error("请选择技能 zip 包");
      }
      if (!name.trim()) {
        throw new Error("请填写技能中文名称");
      }
      return uploadSkill(apiOptions, {
        description,
        file,
        name,
        risk_level: riskLevel,
        runtime_dependencies: {
          env: runtimeEnvItems,
          tools: runtimeToolItems,
        },
        tags: tagItems,
      });
    },
    onSuccess: (skill) => {
      onUploaded?.(skill);
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
          <div className="flex min-w-0 flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
            <div className="min-w-0">
              <div className="mb-2 flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
                <a className="hover:text-foreground" href="/skills">技能市场</a>
                <span>/</span>
                <span>上传技能</span>
              </div>
              <div className="flex min-w-0 items-start gap-3">
                <SemanticIconTile tone="primary" size="lg">
                  <UploadCloud />
                </SemanticIconTile>
                <div className="min-w-0">
                  <h1 className="text-2xl font-bold tracking-normal">上传技能</h1>
                  <p className="text-sm text-muted-foreground">
                    导入技能 zip 包，确认中文名称、描述和运行依赖声明后发布到技能市场。
                  </p>
                </div>
              </div>
            </div>
            <Button asChild className="self-start" variant="outline">
              <a href="/skills">
                <ArrowLeft data-icon="inline-start" />
                返回技能市场
              </a>
            </Button>
          </div>

          <div className="grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
            <div className="flex min-w-0 flex-col gap-4">
              <LiquidCard className="rounded-lg">
                <CardHeader className="border-b">
                  <CardTitle className="flex items-center gap-2 text-base">
                    <FileArchive className="size-4 text-primary" />
                    技能包
                  </CardTitle>
                </CardHeader>
                <CardContent className="grid gap-4 p-4 md:grid-cols-[minmax(0,1fr)_minmax(220px,280px)]">
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="skill-upload-file">技能 zip 包</Label>
                    <Input
                      accept=".zip,application/zip"
                      id="skill-upload-file"
                      onChange={(event) => setFile(event.target.files?.[0] ?? null)}
                      type="file"
                    />
                    <p className="text-xs text-muted-foreground">
                      zip 包必须包含 SKILL.md。页面不解包预检，发布时由 Control Plane 校验和归档。
                    </p>
                  </div>
                  <div className="rounded-md border bg-background/70 p-3">
                    <p className="text-xs text-muted-foreground">技能包描述名称</p>
                    <p className="mt-1 truncate text-sm font-medium">{packageDisplayName || "选择 zip 后自动生成"}</p>
                    <p className="mt-2 text-xs text-muted-foreground">
                      来自 zip 文件名，去掉 .zip 后缀，用于和归档包对应。
                    </p>
                  </div>
                </CardContent>
              </LiquidCard>

              <LiquidCard className="rounded-lg">
                <CardHeader className="border-b">
                  <CardTitle className="text-base">技能信息</CardTitle>
                </CardHeader>
                <CardContent className="grid gap-4 p-4 md:grid-cols-2">
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="skill-upload-name">技能中文名称</Label>
                    <Input
                      id="skill-upload-name"
                      onChange={(event) => setName(event.target.value)}
                      placeholder="例如：接口文档生成"
                      value={name}
                    />
                    <p className="text-xs text-muted-foreground">
                      用于市场展示、安装选择和后续管理，请填写清晰的中文名称。
                    </p>
                  </div>
                  <div className="flex flex-col gap-2">
                    <Label>风险等级</Label>
                    <div className="grid grid-cols-3 rounded-md border bg-background p-1" role="group" aria-label="风险等级">
                      {riskOptions.map((option) => (
                        <button
                          className={cn(
                            "h-8 rounded-sm text-sm transition-colors",
                            riskLevel === option.value
                              ? "bg-primary text-primary-foreground"
                              : "text-muted-foreground hover:bg-muted",
                          )}
                          key={option.value}
                          onClick={() => setRiskLevel(option.value)}
                          type="button"
                        >
                          {option.label}
                        </button>
                      ))}
                    </div>
                  </div>
                  <div className="flex flex-col gap-2 md:col-span-2">
                    <Label htmlFor="skill-upload-description">技能描述</Label>
                    <Textarea
                      id="skill-upload-description"
                      onChange={(event) => setDescription(event.target.value)}
                      placeholder="描述技能解决的问题、输入输出和适用场景。留空时发布时从 SKILL.md 首段读取。"
                      value={description}
                    />
                  </div>
                  <div className="flex flex-col gap-2 md:col-span-2">
                    <Label htmlFor="skill-upload-tags">标签</Label>
                    <Input
                      id="skill-upload-tags"
                      onChange={(event) => setTags(event.target.value)}
                      placeholder="文档生成,API,OpenAPI"
                      value={tags}
                    />
                    <TokenPreview emptyText="暂无标签" items={tagItems} />
                  </div>
                </CardContent>
              </LiquidCard>

              <LiquidCard className="rounded-lg">
                <CardHeader className="border-b">
                  <CardTitle className="text-base">运行依赖声明</CardTitle>
                </CardHeader>
                <CardContent className="grid gap-4 p-4 md:grid-cols-2">
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="skill-upload-runtime-tools">CLI 依赖</Label>
                    <Input
                      id="skill-upload-runtime-tools"
                      onChange={(event) => setRuntimeTools(event.target.value)}
                      placeholder="gh,node"
                      value={runtimeTools}
                    />
                    <TokenPreview emptyText="未声明 CLI" items={runtimeToolItems} prefix="CLI" />
                  </div>
                  <div className="flex flex-col gap-2">
                    <Label htmlFor="skill-upload-runtime-env">环境变量</Label>
                    <Input
                      id="skill-upload-runtime-env"
                      onChange={(event) => setRuntimeEnv(event.target.value)}
                      placeholder="GH_TOKEN,OPENAI_API_KEY"
                      value={runtimeEnv}
                    />
                    <TokenPreview emptyText="未声明环境变量" items={runtimeEnvItems} prefix="ENV" />
                  </div>
                  <div className="flex items-start gap-2 rounded-md border border-primary/20 bg-primary/5 p-3 text-sm md:col-span-2">
                    <Info className="mt-0.5 size-4 shrink-0 text-primary" />
                    <span className="text-muted-foreground">仅声明变量名，运行时由数字员工配置注入值。</span>
                  </div>
                </CardContent>
              </LiquidCard>
            </div>

            <aside className="min-w-0">
              <LiquidCard className="sticky top-4 rounded-lg">
                <CardHeader className="border-b">
                  <CardTitle className="flex items-center gap-2 text-base">
                    <PackageCheck className="size-4 text-primary" />
                    发布摘要
                  </CardTitle>
                </CardHeader>
                <CardContent className="flex flex-col gap-4 p-4 text-sm">
                  <SummaryRow label="归档包" value={file?.name ?? "未选择"} />
                  <SummaryRow label="技能包描述名称" value={packageDisplayName || "待生成"} />
                  <SummaryRow label="技能中文名称" value={name.trim() || "待填写"} />
                  <SummaryRow label="风险等级" value={riskLabel(riskLevel)} />
                  <SummaryRow label="依赖声明" value={`${dependencyCount} 项`} />
                  <div className="flex items-start gap-2 rounded-md border bg-background/70 p-3">
                    <CheckCircle2 className="mt-0.5 size-4 shrink-0 text-superteam-success" />
                    <div className="min-w-0">
                      <p className="font-medium">SKILL.md 为必需文件</p>
                      <p className="mt-1 text-xs text-muted-foreground">
                        名称和描述不会在前端解包预填；服务端发布时会在需要时读取 SKILL.md 兜底。
                      </p>
                    </div>
                  </div>
                  <div className="rounded-md border border-primary/20 bg-primary/5 p-3 text-xs text-muted-foreground">
                    发布到技能市场后，可在技能详情页安装到团队或数字员工。
                  </div>
                  {upload.error instanceof Error ? (
                    <Alert variant="destructive">
                      <AlertTitle>上传失败</AlertTitle>
                      <AlertDescription>{upload.error.message}</AlertDescription>
                    </Alert>
                  ) : null}
                  <Button
                    disabled={!canPublish || upload.isPending}
                    onClick={() => upload.mutate()}
                    type="button"
                  >
                    <UploadCloud data-icon="inline-start" />
                    发布到技能市场
                  </Button>
                </CardContent>
              </LiquidCard>
            </aside>
          </div>
        </div>
      </Main>
    </>
  );
}

function TokenPreview({
  emptyText,
  items,
  prefix,
}: {
  emptyText: string;
  items: string[];
  prefix?: string;
}) {
  if (items.length === 0) {
    return <p className="text-xs text-muted-foreground">{emptyText}</p>;
  }
  return (
    <div className="flex flex-wrap gap-2">
      {items.map((item) => (
        <Badge className="gap-1" key={`${prefix ?? "tag"}-${item}`} variant="outline">
          {prefix ? `${prefix} ${item}` : item}
          <X className="size-3 text-muted-foreground" />
        </Badge>
      ))}
    </div>
  );
}

function SummaryRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <span className="text-muted-foreground">{label}</span>
      <span className="min-w-0 truncate text-right font-medium">{value}</span>
    </div>
  );
}

function packageNameFromFile(file: File | null) {
  return file?.name.replace(/\.zip$/i, "") ?? "";
}

function splitCommaInput(value: string): string[] {
  const seen = new Set<string>();
  const items: string[] = [];
  for (const item of value.split(",")) {
    const trimmed = item.trim();
    if (trimmed && !seen.has(trimmed)) {
      seen.add(trimmed);
      items.push(trimmed);
    }
  }
  return items;
}

function riskLabel(value: string) {
  switch (value) {
    case "low":
      return "低风险";
    case "high":
      return "高风险";
    default:
      return "中风险";
  }
}

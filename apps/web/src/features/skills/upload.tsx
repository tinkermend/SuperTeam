import { type ReactNode, useMemo, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Link, useNavigate } from "@tanstack/react-router";
import {
  ArrowLeft,
  BadgeCheck,
  CheckCircle2,
  CircleCheck,
  FileArchive,
  Info,
  PackageCheck,
  Pencil,
  Rocket,
  ShieldCheck,
  Tag,
  Terminal,
  X,
} from "lucide-react";
import { Header } from "@/components/layout/header";
import { Main } from "@/components/layout/main";
import { Search } from "@/components/search";
import {
  IconTile,
  SignatureCard,
  SoftCard,
  StatusPill,
  V3Button,
  type V3Tone,
} from "@/components/superteam";
import { ThemeSwitch } from "@/components/theme-switch";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CardContent } from "@/components/ui/card";
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
      <Main className="min-w-0 overflow-x-hidden bg-v3-bg">
        <div className="flex min-w-0 flex-col gap-4">
          <div className="flex min-w-0 flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
            <div className="min-w-0">
              <div className="mb-2 flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
                <Link className="hover:text-foreground" to="/skills">技能市场</Link>
                <span>/</span>
                <span>上传技能</span>
              </div>
              <h1 className="text-2xl font-bold tracking-normal text-foreground">上传技能</h1>
              <p className="mt-1 text-sm text-muted-foreground">
                上传技能包并完善元数据与运行依赖声明，发布后可安装到团队或数字员工。
              </p>
            </div>
            <Button asChild className="self-start" variant="outline">
              <Link to="/skills">
                <ArrowLeft data-icon="inline-start" />
                返回技能市场
              </Link>
            </Button>
          </div>

          <PackageStatusBand
            canPublish={canPublish}
            dependencyCount={dependencyCount}
            file={file}
            metadataReady={Boolean(name.trim())}
            packageDisplayName={packageDisplayName}
            onFileChange={setFile}
          />

          <div className="grid min-w-0 gap-4 xl:grid-cols-[minmax(0,1fr)_400px]">
            <SoftCard className="min-w-0 overflow-hidden py-0">
              <CardContent className="p-0">
                <section className="p-5">
                  <SectionTitle
                    icon={<BadgeCheck />}
                    title="技能信息"
                  />

                  <FormRow
                    help="用于管理与展示"
                    htmlFor="skill-upload-name"
                    label="技能中文名称"
                    required
                  >
                    <div className="relative">
                      <Input
                        className="h-10 pr-14"
                        id="skill-upload-name"
                        onChange={(event) => setName(event.target.value)}
                        placeholder="例如：接口文档生成"
                        value={name}
                      />
                      <span className="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">
                        {name.length}/64
                      </span>
                    </div>
                  </FormRow>

                  <FormRow
                    help="选填，发布时可从 SKILL.md 读取"
                    htmlFor="skill-upload-description"
                    label="技能描述"
                  >
                    <div className="relative">
                      <Textarea
                        className="min-h-20 resize-none pr-16 leading-6"
                        id="skill-upload-description"
                        onChange={(event) => setDescription(event.target.value)}
                        placeholder="描述技能解决的问题、输入输出和适用场景。"
                        value={description}
                      />
                      <span className="pointer-events-none absolute bottom-3 right-3 text-xs text-muted-foreground">
                        {description.length}/500
                      </span>
                    </div>
                  </FormRow>

                  <FormRow
                    help="评估技能执行风险"
                    label="风险等级"
                    required
                  >
                    <div className="grid h-10 grid-cols-3 rounded-xl bg-v3-card-soft p-1" role="group" aria-label="风险等级">
                      {riskOptions.map((option) => (
                        <button
                          className={cn(
                            "rounded-[10px] text-sm font-semibold transition-colors",
                            riskLevel === option.value
                              ? "bg-v3-brand text-white shadow-v3"
                              : "text-v3-ink-2 hover:bg-v3-card hover:text-v3-ink",
                          )}
                          key={option.value}
                          onClick={() => setRiskLevel(option.value)}
                          type="button"
                        >
                          {option.label}
                        </button>
                      ))}
                    </div>
                  </FormRow>

                  <FormRow
                    help="用于检索与分类"
                    htmlFor="skill-upload-tags"
                    label="标签"
                  >
                    <div className="space-y-2">
                      <Input
                        className="h-10"
                        id="skill-upload-tags"
                        onChange={(event) => setTags(event.target.value)}
                        placeholder="文档生成,API,OpenAPI"
                        value={tags}
                      />
                      <TokenPreview emptyText="暂无标签" items={tagItems} onRemove={(item) => setTags(removeCommaItem(tags, item))} />
                    </div>
                  </FormRow>
                </section>

                <section className="border-t p-5">
                  <SectionTitle
                    description="声明运行该技能所需的 CLI 工具与环境变量（仅声明，不校验值）"
                    icon={<Terminal />}
                    title="运行依赖声明"
                  />

                  <FormRow
                    help="请输入或以逗号分隔"
                    htmlFor="skill-upload-runtime-tools"
                    label="CLI 依赖"
                  >
                    <DependencyInput
                      emptyText="未声明 CLI"
                      id="skill-upload-runtime-tools"
                      items={runtimeToolItems}
                      onChange={setRuntimeTools}
                      placeholder="输入依赖名称后回车或逗号分隔"
                    />
                  </FormRow>

                  <FormRow
                    help="请输入或以逗号分隔"
                    htmlFor="skill-upload-runtime-env"
                    label="环境变量"
                  >
                    <DependencyInput
                      emptyText="未声明环境变量"
                      id="skill-upload-runtime-env"
                      items={runtimeEnvItems}
                      onChange={setRuntimeEnv}
                      placeholder="输入变量名后回车或逗号分隔"
                    />
                  </FormRow>

                  <div className="mt-4 flex items-start gap-2 border-t pt-4 text-sm text-muted-foreground">
                    <Info className="mt-0.5 size-4 shrink-0 text-v3-info" />
                    <span>仅声明变量名，运行时由数字员工配置注入值。以上依赖为声明，不校验值；运行时由安装方或平台提供对应值。</span>
                  </div>
                </section>
              </CardContent>
            </SoftCard>

            <aside className="min-w-0">
              <SoftCard className="sticky top-4 py-0">
                <CardContent className="flex flex-col gap-4 p-5 text-sm">
                  <h2 className="text-base font-semibold tracking-normal">发布摘要</h2>
                  <div className={cn(
                    "flex items-start gap-3 rounded-md border p-3",
                    canPublish
                      ? "border-v3-ok/30 bg-v3-ok-soft text-v3-ok"
                      : "border-border bg-muted/40 text-muted-foreground",
                  )}>
                    <CheckCircle2 className="mt-0.5 size-5 shrink-0 text-v3-ok" />
                    <div>
                      <p className="text-lg font-semibold tracking-normal">{canPublish ? "可发布" : "待完善"}</p>
                      <p className="mt-1 text-xs">{canPublish ? "元数据与依赖声明已就绪" : "请选择 zip 包并填写技能中文名称"}</p>
                    </div>
                  </div>

                  <div className="space-y-3 py-1">
                    <SummaryRow icon={<FileArchive />} label="归档包" value={file ? `${file.name}（${formatBytes(file.size)}）` : "未选择"} />
                    <SummaryRow icon={<PackageCheck />} label="技能包描述名称" value={packageDisplayName || "待生成"} />
                    <SummaryRow icon={<BadgeCheck />} label="技能中文名称" value={name.trim() || "待填写"} />
                    <SummaryRow icon={<ShieldCheck />} label="风险等级" value={riskLabel(riskLevel)} valueTone={riskTone(riskLevel)} />
                    <SummaryRow icon={<Tag />} label="标签" value={tagItems.length ? tagItems.join(", ") : "未设置"} />
                    <SummaryRow icon={<Terminal />} label="依赖声明" value={`${dependencyCount} 项`} valueTone="info" />
                    <SummarySubRow label="CLI 依赖" value={runtimeToolItems.length ? runtimeToolItems.join(", ") : "未声明"} />
                    <SummarySubRow label="环境变量" value={runtimeEnvItems.length ? runtimeEnvItems.join(", ") : "未声明"} />
                  </div>

                  <div className="flex items-start gap-2 rounded-md border border-v3-info/20 bg-v3-info-soft p-3 text-xs leading-5 text-v3-ink">
                    <Info className="mt-0.5 size-4 shrink-0 text-v3-info" />
                    <p>
                      发布后，平台将从 SKILL.md 读取并补齐技能名称与描述（如未填写）。
                      <br />
                      发布后可安装到团队或数字员工使用。
                    </p>
                  </div>
                  {upload.error instanceof Error ? (
                    <Alert variant="destructive">
                      <AlertTitle>上传失败</AlertTitle>
                      <AlertDescription>{upload.error.message}</AlertDescription>
                    </Alert>
                  ) : null}
                  <V3Button
                    data-skill-upload-publish
                    disabled={!canPublish || upload.isPending}
                    className="h-11 text-base"
                    onClick={() => upload.mutate()}
                    type="button"
                  >
                    <Rocket data-icon="inline-start" />
                    发布到技能市场
                  </V3Button>
                </CardContent>
              </SoftCard>
            </aside>
          </div>
        </div>
      </Main>
    </>
  );
}

function PackageStatusBand({
  canPublish,
  dependencyCount,
  file,
  metadataReady,
  onFileChange,
  packageDisplayName,
}: {
  canPublish: boolean;
  dependencyCount: number;
  file: File | null;
  metadataReady: boolean;
  onFileChange: (file: File | null) => void;
  packageDisplayName: string;
}) {
  const releaseSteps = [
    {
      description: file ? file.name : "等待 ZIP",
      done: Boolean(file),
      label: "上传文件",
    },
    {
      description: packageDisplayName || "选择后解析",
      done: Boolean(file),
      label: "解析信息",
    },
    {
      description: metadataReady ? "名称已填写" : "待填写名称",
      done: metadataReady,
      label: "完善资料",
    },
    {
      description: dependencyCount ? "依赖声明已记录" : "无依赖声明",
      disabled: dependencyCount === 0,
      done: dependencyCount > 0,
      label: "校验依赖",
    },
    {
      description: canPublish ? "可以发布" : "等待就绪",
      done: canPublish,
      label: "发布确认",
    },
  ];

  return (
    <SignatureCard className="p-6 lg:p-7">
      <Label className="sr-only" htmlFor="skill-upload-file">技能 zip 包</Label>
      <input
        accept=".zip,application/zip"
        className="sr-only"
        id="skill-upload-file"
        onChange={(event) => onFileChange(event.target.files?.[0] ?? null)}
        type="file"
      />
      <div className="relative grid gap-6 xl:grid-cols-[minmax(0,1.05fr)_minmax(0,1.35fr)] xl:items-center">
        <div className="flex min-w-0 items-center gap-4">
          <label
            className="flex size-[76px] shrink-0 cursor-pointer flex-col items-center justify-center rounded-2xl text-white shadow-[0_10px_24px_-6px_rgba(47,95,255,0.5)] transition hover:scale-[1.03]"
            htmlFor="skill-upload-file"
            style={{ background: "var(--v3-brand-grad)" }}
          >
            <FileArchive className="size-7" />
            <span className="mt-1 text-xs font-bold tracking-normal">ZIP</span>
          </label>
          <div className="min-w-0">
            <p className="truncate text-lg font-extrabold tracking-tight text-v3-ink">
              {file?.name ?? "选择技能 zip 包"}
            </p>
            <p className="mt-1 text-sm text-v3-ink-2">
              {file ? formatBytes(file.size) : "拖拽或点击上传，必须包含 SKILL.md"}
            </p>
            <div className="mt-2 flex min-w-0 items-center gap-2 text-v3-ink-2">
              <span className="text-xs uppercase tracking-wide text-v3-ink-3">包名</span>
              <p className="truncate text-sm font-semibold text-v3-ink">
                {packageDisplayName || "选择 zip 后自动生成"}
              </p>
              <Pencil className="size-3.5 shrink-0 text-v3-brand" />
            </div>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2 lg:justify-end">
          <SignaturePill done={Boolean(file)} label={file ? "ZIP 已选择" : "等待 ZIP"} />
          <SignaturePill done={Boolean(file)} label={file ? "包含 SKILL.md" : "需包含 SKILL.md"} />
          <SignaturePill done label="服务端发布校验" icon={<ShieldCheck className="size-3.5" />} />
        </div>
      </div>
      <div className="mt-6 border-t border-[color:var(--v3-signature-border)] pt-5">
        <ol className="grid gap-2 sm:grid-cols-5">
          {releaseSteps.map((step, index) => (
            <li
              aria-disabled={step.disabled ? "true" : "false"}
              className={cn(
                "min-w-0 rounded-xl border p-3",
                step.disabled
                  ? "border-v3-line bg-v3-card-soft text-v3-ink-2"
                  : step.done
                  ? "border-[color:var(--v3-signature-border)] bg-v3-card text-v3-ink"
                  : "border-v3-line bg-v3-card-soft text-v3-ink-2",
              )}
              data-release-step={step.label}
              key={step.label}
            >
              <div className="flex items-center gap-2">
                <span
                  className={cn(
                    "grid size-6 shrink-0 place-items-center rounded-lg text-xs font-extrabold tabular-nums",
                    step.disabled
                      ? "bg-v3-card text-v3-ink-3 ring-1 ring-v3-line-strong"
                      : step.done
                      ? "bg-v3-brand text-white"
                      : "bg-v3-card text-v3-ink-3 ring-1 ring-v3-line-strong",
                  )}
                >
                  {index + 1}
                </span>
                <span className="truncate text-sm font-bold" data-release-step-title>{step.label}</span>
              </div>
              <p className="mt-2 truncate text-xs text-v3-ink-2" data-release-step-description>{step.description}</p>
            </li>
          ))}
        </ol>
      </div>
    </SignatureCard>
  );
}

function SignaturePill({
  done,
  icon,
  label,
}: {
  done: boolean;
  icon?: ReactNode;
  label: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-bold",
        done
          ? "bg-v3-brand-soft text-v3-brand-deep ring-1 ring-[color:var(--v3-signature-border)]"
          : "bg-v3-card-soft text-v3-ink-3",
      )}
    >
      {icon ?? <CircleCheck className="size-3.5" />}
      {label}
    </span>
  );
}

function SectionTitle({
  description,
  icon,
  title,
}: {
  description?: string;
  icon: ReactNode;
  title: string;
}) {
  return (
    <div className="mb-4 flex min-w-0 items-center gap-3">
      <IconTile className="size-8 rounded-lg [&_svg]:size-4" tone="brand">
        {icon}
      </IconTile>
      <div className="min-w-0">
        <h2 className="text-lg font-semibold tracking-normal text-foreground">{title}</h2>
        {description ? <p className="mt-0.5 text-sm text-muted-foreground">{description}</p> : null}
      </div>
    </div>
  );
}

function FormRow({
  children,
  help,
  htmlFor,
  label,
  required,
}: {
  children: ReactNode;
  help: string;
  htmlFor?: string;
  label: string;
  required?: boolean;
}) {
  return (
    <div className="grid gap-3 border-t py-3.5 first:border-t-0 first:pt-0 md:grid-cols-[210px_minmax(0,1fr)] md:items-start">
      <div className="min-w-0">
        <Label className="text-sm font-semibold text-foreground" htmlFor={htmlFor}>
          {label}
          {required ? <span className="ml-1 text-destructive">*</span> : null}
        </Label>
        <p className="mt-1 text-xs text-muted-foreground">{help}</p>
      </div>
      <div className="min-w-0">{children}</div>
    </div>
  );
}

function DependencyInput({
  emptyText,
  id,
  items,
  onChange,
  placeholder,
}: {
  emptyText: string;
  id: string;
  items: string[];
  onChange: (value: string) => void;
  placeholder: string;
}) {
  const [draft, setDraft] = useState("");
  const [committedItems, setCommittedItems] = useState(items);

  const syncValue = (nextCommittedItems: string[], nextDraft: string) => {
    onChange(mergeCommaItems(nextCommittedItems, splitCommaInput(nextDraft)).join(","));
  };

  const commitDraft = (rawValue: string) => {
    const nextItems = mergeCommaItems(committedItems, splitCommaInput(rawValue));
    setCommittedItems(nextItems);
    onChange(nextItems.join(","));
    setDraft("");
  };

  const removeItem = (itemToRemove: string) => {
    const nextItems = committedItems.filter((item) => item !== itemToRemove);
    setCommittedItems(nextItems);
    syncValue(nextItems, draft);
  };

  return (
    <div className="flex min-h-11 flex-wrap items-center gap-2 rounded-[10px] border border-v3-line-strong bg-v3-card px-2 py-1.5 focus-within:border-v3-brand focus-within:ring-2 focus-within:ring-v3-brand/25">
      {committedItems.map((item) => (
        <Badge className="h-7 gap-1 rounded-md border-border/80 bg-background px-2.5 text-sm font-medium shadow-none" key={item} variant="outline">
          {item}
          <button
            aria-label={`移除 ${item}`}
            className="-mr-1 inline-flex size-4 items-center justify-center rounded-sm text-muted-foreground hover:bg-muted hover:text-foreground"
            onClick={() => removeItem(item)}
            type="button"
          >
            <X className="size-3" />
          </button>
        </Badge>
      ))}
      <Input
        className="h-7 min-w-[220px] flex-1 border-0 bg-transparent px-1 py-0 shadow-none focus-visible:ring-0"
        id={id}
        onBlur={(event) => {
          const nextTarget = event.relatedTarget instanceof HTMLElement ? event.relatedTarget : null;
          if (nextTarget?.closest("[data-skill-upload-publish]")) {
            return;
          }
          if (draft.trim()) {
            commitDraft(draft);
          }
        }}
        onChange={(event) => {
          const nextDraft = event.target.value;
          if (nextDraft.includes(",")) {
            commitDraft(nextDraft);
            return;
          }
          setDraft(nextDraft);
          syncValue(committedItems, nextDraft);
        }}
        onKeyDown={(event) => {
          if (event.key === "Enter" || event.key === ",") {
            event.preventDefault();
            commitDraft(draft);
          }
          if (event.key === "Backspace" && !draft && committedItems.length > 0) {
            removeItem(committedItems[committedItems.length - 1]);
          }
        }}
        placeholder={committedItems.length ? placeholder : emptyText}
        value={draft}
      />
    </div>
  );
}

function TokenPreview({
  emptyText,
  items,
  onRemove,
  prefix,
}: {
  emptyText: string;
  items: string[];
  onRemove?: (item: string) => void;
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
          {onRemove ? (
            <button
              aria-label={`移除 ${item}`}
              className="-mr-1 inline-flex size-4 items-center justify-center rounded-sm text-muted-foreground hover:bg-muted hover:text-foreground"
              onClick={() => onRemove(item)}
              type="button"
            >
              <X className="size-3" />
            </button>
          ) : null}
        </Badge>
      ))}
    </div>
  );
}

function SummaryRow({
  icon,
  label,
  value,
  valueTone,
}: {
  icon: ReactNode;
  label: string;
  value: string;
  valueTone?: V3Tone;
}) {
  return (
    <div className="grid grid-cols-[18px_minmax(110px,1fr)_minmax(0,1.35fr)] items-center gap-3" data-summary-row={label}>
      <span className="text-muted-foreground [&_svg]:size-4">{icon}</span>
      <span className="text-foreground">{label}</span>
      {valueTone ? (
        <span className="min-w-0 justify-self-end">
          <StatusPill tone={valueTone}>{value}</StatusPill>
        </span>
      ) : (
        <span className="min-w-0 truncate text-right text-muted-foreground">{value}</span>
      )}
    </div>
  );
}

function SummarySubRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="grid grid-cols-[18px_minmax(110px,1fr)_minmax(0,1.35fr)] items-center gap-3 pl-[30px] text-sm">
      <span />
      <span className="text-muted-foreground">{label}</span>
      <span className="min-w-0 break-words text-right text-muted-foreground">{value}</span>
    </div>
  );
}

function packageNameFromFile(file: File | null) {
  return file?.name.replace(/\.zip$/i, "") ?? "";
}

function formatBytes(bytes: number) {
  if (bytes < 1024) {
    return `${bytes} B`;
  }
  const units = ["KB", "MB", "GB"];
  let value = bytes / 1024;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }
  return `${value >= 10 ? value.toFixed(1) : value.toFixed(2)} ${units[unitIndex]}`;
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

function removeCommaItem(value: string, itemToRemove: string): string {
  return splitCommaInput(value).filter((item) => item !== itemToRemove).join(",");
}

function mergeCommaItems(existingItems: string[], incomingItems: string[]): string[] {
  const seen = new Set<string>();
  const merged: string[] = [];
  for (const item of [...existingItems, ...incomingItems]) {
    if (!seen.has(item)) {
      seen.add(item);
      merged.push(item);
    }
  }
  return merged;
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

function riskTone(value: string): V3Tone {
  switch (value) {
    case "low":
      return "ok";
    case "high":
      return "danger";
    default:
      return "warn";
  }
}

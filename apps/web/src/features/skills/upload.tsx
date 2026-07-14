import { type DragEvent as ReactDragEvent, type ReactNode, useMemo, useRef, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { useNavigate } from "@tanstack/react-router";
import {
  BadgeCheck,
  FileArchive,
  Info,
  PackageCheck,
  Rocket,
  ShieldCheck,
  Tag,
  Terminal,
  X,
} from "lucide-react";
import { Main } from "@/components/layout/main";
import {
  ShellPageHeader,
  ShellPageHeaderBack,
} from "@/components/layout/shell-page-header";
import {
  GlassCard,
  IconTile,
  MasterDetailLayout,
  SignatureCard,
  StatusPill,
  V3Button,
  type V3Tone,
} from "@/components/superteam";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { ApiRequestError } from "@/lib/api/client";
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

// 选中态按风险语义着色：低=绿(ok)、中=橙(warn)、高=红(danger)，与右侧摘要 StatusPill tone 一致。
const riskActiveClass: Record<string, string> = {
  low: "bg-v3-ok",
  medium: "bg-v3-warn",
  high: "bg-v3-danger",
};

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
      <ShellPageHeader
        back={<ShellPageHeaderBack ariaLabel="返回技能市场" to="/skills" />}
        title="上传技能"
        subtitle="上传技能包并完善元数据与运行依赖声明，发布后可安装到团队或数字员工。"
      />
      <Main width="wide" className="min-w-0 overflow-x-hidden">
        <div className="flex min-w-0 flex-col gap-4">
          <PackageStatusBand
            canPublish={canPublish}
            dependencyCount={dependencyCount}
            file={file}
            metadataReady={Boolean(name.trim())}
            packageDisplayName={packageDisplayName}
            onFileChange={setFile}
          />

          <MasterDetailLayout
            narrowDetail="stack"
            rail="lg"
            master={
              <GlassCard className="min-w-0">
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
                          aria-pressed={riskLevel === option.value}
                          className={cn(
                            "rounded-[10px] text-sm font-semibold transition-colors",
                            riskLevel === option.value
                              ? cn(riskActiveClass[option.value], "text-white shadow-v3")
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
                    help="回车或逗号分隔，用于检索与分类"
                    htmlFor="skill-upload-tags"
                    label="标签"
                  >
                    <DependencyInput
                      emptyText="未添加标签"
                      id="skill-upload-tags"
                      items={tagItems}
                      onChange={setTags}
                      placeholder="输入标签后回车或逗号分隔"
                    />
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
                    help="声明该技能需要的变量名；值由数字员工配置并在运行时注入"
                    htmlFor="skill-upload-runtime-env"
                    label="环境变量"
                  >
                    <DependencyInput
                      emptyText="未声明环境变量"
                      id="skill-upload-runtime-env"
                      items={runtimeEnvItems}
                      onChange={setRuntimeEnv}
                      placeholder="输入变量名后回车，如 GH_TOKEN"
                    />
                  </FormRow>

                  <div className="mt-4 flex items-start gap-2 border-t pt-4 text-sm text-muted-foreground">
                    <Info className="mt-0.5 size-4 shrink-0 text-v3-info" />
                    <span>仅声明变量名，运行时由数字员工配置注入值。以上依赖为声明，不校验值；运行时由安装方或平台提供对应值。</span>
                  </div>
                </section>
              </CardContent>
              </GlassCard>
            }
            detail={
              <aside className="min-w-0 @5xl/master-detail:sticky @5xl/master-detail:top-4 @5xl/master-detail:max-h-[calc(100svh-2rem)] @5xl/master-detail:overflow-y-auto">
              <GlassCard>
                <CardContent className="flex flex-col gap-4 p-5 text-sm">
                  <h2 className="text-base font-semibold tracking-normal">发布摘要</h2>

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
                      <AlertDescription>{skillUploadErrorMessage(upload.error)}</AlertDescription>
                    </Alert>
                  ) : null}
                  <div className="flex flex-col gap-2.5 border-t pt-4">
                    <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-xs">
                      <span className={cn("size-1.5 shrink-0 rounded-full", canPublish ? "bg-v3-ok" : "bg-v3-ink-3")} />
                      <span className="font-semibold text-v3-ink">{canPublish ? "可发布" : "待完善"}</span>
                      <span className="text-v3-ink-3">{canPublish ? "元数据与依赖声明已就绪" : "请选择 zip 包并填写技能中文名称"}</span>
                    </div>
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
                  </div>
                </CardContent>
              </GlassCard>
              </aside>
            }
          />
        </div>
      </Main>
    </>
  );
}

function skillUploadErrorMessage(error: Error): string {
  if (
    error instanceof ApiRequestError
    && error.status === 400
    && error.detail?.includes("zip archive must include SKILL.md")
  ) {
    return "上传失败：技能压缩包必须包含 SKILL.md 文件";
  }
  return error.message;
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
  const [dragActive, setDragActive] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  const clearFile = () => {
    onFileChange(null);
    if (inputRef.current) {
      inputRef.current.value = "";
    }
  };

  const acceptDroppedFile = (event: ReactDragEvent) => {
    event.preventDefault();
    setDragActive(false);
    const dropped = event.dataTransfer.files?.[0];
    if (dropped) {
      onFileChange(dropped);
    }
  };

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

  const zipTile = (
    <span
      className="flex size-14 shrink-0 flex-col items-center justify-center rounded-xl text-white shadow-[0_8px_20px_-6px_rgba(47,95,255,0.5)] transition group-hover:scale-[1.03]"
      style={{ background: "var(--v3-brand-grad)" }}
    >
      <FileArchive className="size-5" />
      <span className="mt-0.5 text-[10px] font-bold tracking-normal">ZIP</span>
    </span>
  );

  const fileMeta = (
    <div className="min-w-0 flex-1">
      <p className="truncate text-base font-extrabold tracking-tight text-v3-ink">
        {file?.name ?? "选择技能 zip 包"}
      </p>
      <p className="mt-0.5 text-sm text-v3-ink-2">
        {file ? formatBytes(file.size) : "拖拽 ZIP 到此处，或点击选择，必须包含 SKILL.md"}
      </p>
      {file ? (
        <div className="mt-1 flex min-w-0 items-center gap-2 text-v3-ink-2">
          <span className="text-xs uppercase tracking-wide text-v3-ink-3">包名</span>
          <p className="truncate text-sm font-semibold text-v3-ink">
            {packageDisplayName || "选择 zip 后自动生成"}
          </p>
          <span className="shrink-0 text-xs text-v3-ink-3">· 自动生成</span>
        </div>
      ) : null}
    </div>
  );

  return (
    <SignatureCard
      className={cn(
        "p-5 transition-shadow",
        dragActive && "ring-2 ring-v3-brand ring-offset-2 ring-offset-[color:var(--v3-signature-surface)]",
      )}
      onDragLeave={() => setDragActive(false)}
      onDragOver={(event) => {
        event.preventDefault();
        setDragActive(true);
      }}
      onDrop={acceptDroppedFile}
    >
      <Label className="sr-only" htmlFor="skill-upload-file">技能 zip 包</Label>
      <input
        accept=".zip,application/zip"
        className="sr-only"
        id="skill-upload-file"
        onChange={(event) => onFileChange(event.target.files?.[0] ?? null)}
        ref={inputRef}
        type="file"
      />
      <div
        className={cn(
          "relative flex min-w-0 flex-col gap-3 rounded-xl border border-dashed p-3 transition-colors sm:flex-row sm:items-center",
          dragActive
            ? "border-v3-brand bg-v3-brand-soft/50"
            : "border-v3-line-strong/70 hover:border-v3-brand/60 hover:bg-v3-brand-soft/15",
        )}
      >
        {file ? (
          <>
            <label className="group shrink-0 cursor-pointer" htmlFor="skill-upload-file">{zipTile}</label>
            {fileMeta}
            <div className="flex shrink-0 items-center gap-2 sm:flex-col sm:items-end sm:gap-1.5">
              <label
                className="cursor-pointer rounded-lg bg-v3-brand-soft px-2.5 py-1.5 text-xs font-bold text-v3-brand-deep transition-colors hover:bg-v3-brand-soft/80"
                htmlFor="skill-upload-file"
              >
                更换
              </label>
              <button
                className="rounded-lg px-2.5 py-1.5 text-xs font-semibold text-v3-ink-3 transition-colors hover:bg-v3-card hover:text-v3-ink"
                onClick={clearFile}
                type="button"
              >
                移除
              </button>
            </div>
          </>
        ) : (
          <label className="group flex min-w-0 flex-1 cursor-pointer items-center gap-4" htmlFor="skill-upload-file">
            {zipTile}
            {fileMeta}
          </label>
        )}
      </div>
      <div className="mt-4 border-t border-[color:var(--v3-signature-border)] pt-4">
        <ol className="flex flex-col gap-3 sm:flex-row sm:gap-0">
          {releaseSteps.map((step, index) => (
            <li
              aria-disabled={step.disabled ? "true" : "false"}
              className={cn(
                "flex min-w-0 flex-1 flex-col gap-1.5",
                step.done ? "text-v3-ink" : "text-v3-ink-2",
              )}
              data-release-step={step.label}
              key={step.label}
            >
              <div className="flex items-center gap-2.5">
                <span
                  className={cn(
                    "grid size-6 shrink-0 place-items-center rounded-full text-[11px] font-extrabold tabular-nums transition-colors",
                    step.done
                      ? "bg-v3-brand text-white shadow-v3"
                      : "bg-v3-card text-v3-ink-3 ring-1 ring-v3-line-strong",
                  )}
                >
                  {index + 1}
                </span>
                {index < releaseSteps.length - 1 ? (
                  <span
                    className={cn(
                      "hidden h-px flex-1 rounded-full sm:block",
                      step.done ? "bg-v3-brand/45" : "bg-v3-line",
                    )}
                  />
                ) : null}
              </div>
              <div className="min-w-0 pr-3">
                <span className="block truncate text-sm font-bold" data-release-step-title>{step.label}</span>
                <p className="mt-0.5 truncate text-xs text-v3-ink-2" data-release-step-description>{step.description}</p>
              </div>
            </li>
          ))}
        </ol>
      </div>
    </SignatureCard>
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
        <Badge
          className="h-7 gap-1 rounded-lg border-transparent bg-v3-brand-soft py-0 pl-2.5 pr-1.5 text-sm font-semibold text-v3-brand-deep shadow-none ring-1 ring-inset ring-[color:var(--v3-signature-border)]"
          key={item}
          variant="outline"
        >
          {item}
          <button
            aria-label={`移除 ${item}`}
            className="inline-flex size-4 items-center justify-center rounded-full text-v3-brand-deep/60 transition-colors hover:bg-v3-danger hover:text-white"
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

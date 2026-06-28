import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { listPromptTemplates, type PromptTemplate } from "@/lib/api/prompt-templates";
import { resolveControlPlaneUrl } from "@/lib/config/control-plane-url";
import { SoftCard, V3Button } from "@/components/superteam";
import { ArrowLeft, Loader2, Sparkles } from "lucide-react";

type PromptTemplateDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onInsert: (text: string, templateId: string) => void;
};

export function PromptTemplateDialog({
  open,
  onOpenChange,
  onInsert,
}: PromptTemplateDialogProps) {
  const [selectedCategory, setSelectedCategory] = useState<string | null>(null);
  const [selectedTemplate, setSelectedTemplate] = useState<PromptTemplate | null>(null);
  const [variableValues, setVariableValues] = useState<Record<string, string>>({});

  const apiOptions = useMemo(
    () => ({ baseUrl: resolveControlPlaneUrl() }),
    [],
  );

  const { data: templates = [], isLoading } = useQuery({
    queryKey: ["prompt-templates"],
    queryFn: () => listPromptTemplates(apiOptions),
    enabled: open,
  });

  const categories = useMemo(() => {
    const cats = Array.from(new Set(templates.map((t) => t.category_code)));
    return cats.sort();
  }, [templates]);

  const activeCategory = selectedCategory || categories[0] || "";

  const filteredTemplates = useMemo(() => {
    return templates.filter((t) => t.category_code === activeCategory);
  }, [templates, activeCategory]);

  function handleSelectTemplate(template: PromptTemplate) {
    setSelectedTemplate(template);
    const initialValues: Record<string, string> = {};
    if (template.variables) {
      for (const v of template.variables) {
        initialValues[v.name] = v.default || "";
      }
    }
    setVariableValues(initialValues);
  }

  function handleConfirm() {
    if (!selectedTemplate) return;
    
    let result = selectedTemplate.content;
    if (selectedTemplate.variables) {
      for (const v of selectedTemplate.variables) {
        const val = variableValues[v.name] || "";
        result = result.replace(new RegExp(`\\{\\{${v.name}\\}\\}`, 'g'), val);
      }
    }
    
    onInsert(result, selectedTemplate.id);
    onOpenChange(false);
    
    // Reset state after a delay
    setTimeout(() => {
      setSelectedTemplate(null);
      setVariableValues({});
    }, 200);
  }

  const isFormValid = useMemo(() => {
    if (!selectedTemplate) return false;
    if (!selectedTemplate.variables) return true;
    return selectedTemplate.variables.every((v) => {
      if (v.required && !variableValues[v.name]?.trim()) {
        return false;
      }
      return true;
    });
  }, [selectedTemplate, variableValues]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[85vh] min-h-[60vh] w-[90vw] max-w-[900px] flex-col overflow-hidden p-0">
        <DialogHeader className="border-b border-v3-line px-6 py-4">
          <DialogTitle className="flex items-center gap-2 text-xl">
            <Sparkles className="size-5 text-v3-brand" />
            浏览模板库
          </DialogTitle>
          <DialogDescription>
            选择预置的提示词模板，加速任务创建
          </DialogDescription>
        </DialogHeader>

        <div className="flex min-h-0 flex-1">
          {isLoading ? (
            <div className="flex flex-1 items-center justify-center">
              <Loader2 className="size-8 animate-spin text-v3-brand/50" />
            </div>
          ) : selectedTemplate ? (
            <div className="flex w-full flex-col p-6">
              <div className="mb-6 flex items-center gap-4">
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-8 rounded-full"
                  onClick={() => setSelectedTemplate(null)}
                >
                  <ArrowLeft className="size-4" />
                </Button>
                <div>
                  <h3 className="text-lg font-bold text-v3-ink">{selectedTemplate.title}</h3>
                  <p className="text-sm text-v3-ink-2">填写变量以生成最终提示词</p>
                </div>
              </div>
              <ScrollArea className="flex-1">
                <div className="grid gap-6 pr-4">
                  {selectedTemplate.variables && selectedTemplate.variables.length > 0 ? (
                    <div className="grid gap-4">
                      {selectedTemplate.variables.map((v) => (
                        <div key={v.name} className="grid gap-2">
                          <Label>
                            {v.name}
                            {v.required && <span className="ml-1 text-destructive">*</span>}
                          </Label>
                          {v.description && (
                            <p className="text-xs text-v3-ink-2">{v.description}</p>
                          )}
                          {v.type === "text" ? (
                            <Textarea
                              value={variableValues[v.name] || ""}
                              onChange={(e) =>
                                setVariableValues((prev) => ({
                                  ...prev,
                                  [v.name]: e.target.value,
                                }))
                              }
                              placeholder={`输入 ${v.name}...`}
                              className="min-h-[100px]"
                            />
                          ) : v.type === "select" ? (
                            <Select
                              value={variableValues[v.name] || ""}
                              onValueChange={(val) =>
                                setVariableValues((prev) => ({
                                  ...prev,
                                  [v.name]: val,
                                }))
                              }
                            >
                              <SelectTrigger>
                                <SelectValue placeholder={`选择 ${v.name}...`} />
                              </SelectTrigger>
                              <SelectContent>
                                {v.options?.map((opt) => (
                                  <SelectItem key={opt} value={opt}>
                                    {opt}
                                  </SelectItem>
                                ))}
                              </SelectContent>
                            </Select>
                          ) : (
                            <Input
                              value={variableValues[v.name] || ""}
                              onChange={(e) =>
                                setVariableValues((prev) => ({
                                  ...prev,
                                  [v.name]: e.target.value,
                                }))
                              }
                              placeholder={`输入 ${v.name}...`}
                            />
                          )}
                        </div>
                      ))}
                    </div>
                  ) : (
                    <div className="rounded-lg bg-v3-card-soft p-4 text-sm text-v3-ink-2">
                      此模板无需变量，直接确认即可。
                    </div>
                  )}
                  <div className="mt-4 rounded-xl border border-v3-line bg-v3-card p-4">
                    <Label className="mb-2 block text-xs font-bold text-v3-ink-3">模板内容预览</Label>
                    <div className="whitespace-pre-wrap text-sm text-v3-ink-2 opacity-70">
                      {selectedTemplate.content}
                    </div>
                  </div>
                </div>
              </ScrollArea>
              <div className="mt-6 flex justify-end gap-3 pt-4">
                <V3Button variant="outline" onClick={() => setSelectedTemplate(null)}>
                  返回列表
                </V3Button>
                <V3Button disabled={!isFormValid} onClick={handleConfirm}>
                  插入模板
                </V3Button>
              </div>
            </div>
          ) : (
            <>
              {/* Sidebar */}
              <div className="w-48 border-r border-v3-line bg-v3-card-soft py-4">
                <ScrollArea className="h-full px-2">
                  <div className="grid gap-1">
                    {categories.map((cat) => (
                      <button
                        key={cat}
                        onClick={() => setSelectedCategory(cat)}
                        className={`rounded-lg px-3 py-2 text-left text-sm font-medium transition-colors ${
                          activeCategory === cat
                            ? "bg-v3-brand-soft text-v3-brand-deep"
                            : "text-v3-ink-2 hover:bg-v3-card hover:text-v3-ink"
                        }`}
                      >
                        {cat}
                      </button>
                    ))}
                    {categories.length === 0 && (
                      <div className="px-3 py-2 text-sm text-v3-ink-3">无可用分类</div>
                    )}
                  </div>
                </ScrollArea>
              </div>

              {/* Grid */}
              <div className="flex-1 bg-v3-bg py-4">
                <ScrollArea className="h-full px-6">
                  <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
                    {filteredTemplates.map((template) => (
                      <SoftCard
                        key={template.id}
                        onClick={() => handleSelectTemplate(template)}
                        className="group cursor-pointer border border-v3-line bg-v3-card p-4 transition-all hover:border-v3-brand/30 hover:shadow-md"
                      >
                        <h4 className="font-bold text-v3-ink group-hover:text-v3-brand">
                          {template.title}
                        </h4>
                        <p className="mt-2 line-clamp-3 text-xs leading-relaxed text-v3-ink-2">
                          {template.content}
                        </p>
                        <div className="mt-4 flex items-center justify-between text-xs text-v3-ink-3">
                          <span>使用 {template.use_count} 次</span>
                          <span>{template.variables?.length || 0} 个变量</span>
                        </div>
                      </SoftCard>
                    ))}
                    {filteredTemplates.length === 0 && (
                      <div className="col-span-full py-10 text-center text-sm text-v3-ink-3">
                        该分类下没有可用模板
                      </div>
                    )}
                  </div>
                </ScrollArea>
              </div>
            </>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

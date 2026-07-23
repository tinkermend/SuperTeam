import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { V3Button } from "@/components/superteam";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { ApiRequestError } from "@/lib/api/client";
import {
  isHighDangerConfig,
  isStringConfig,
  updateSystemConfig,
  type SystemConfigItem,
} from "@/lib/api/system-config";
import { displayDefaultValue, displayEffectiveValue, formatConfigValue, unitFor } from "./units";

type EditSystemConfigDialogProps = {
  apiBaseUrl: string;
  item: SystemConfigItem | null;
  onOpenChange: (open: boolean) => void;
};

export function EditSystemConfigDialog({
  apiBaseUrl,
  item,
  onOpenChange,
}: EditSystemConfigDialogProps) {
  const queryClient = useQueryClient();
  const [inputValue, setInputValue] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const unit = item ? unitFor(item) : { label: "", factor: 1 };
  const stringMode = item ? isStringConfig(item) : false;

  useEffect(() => {
    if (!item) return;
    if (isStringConfig(item)) {
      setInputValue(item.effective_string_value ?? item.default_string_value ?? "");
    } else {
      setInputValue(String(item.effective_value / unitFor(item).factor));
    }
    setFormError(null);
  }, [item]);

  const updateMutation = useMutation({
    mutationFn: ({
      key,
      value,
      stringValue,
    }: {
      key: string;
      value?: number;
      stringValue?: string;
    }) => {
      if (stringValue !== undefined) {
        return updateSystemConfig({ baseUrl: apiBaseUrl }, key, { string_value: stringValue });
      }
      return updateSystemConfig({ baseUrl: apiBaseUrl }, key, { value: value ?? 0 });
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["system-configs"] });
      onOpenChange(false);
    },
    onError: (error: unknown) => {
      setFormError(
        error instanceof ApiRequestError && error.detail
          ? error.detail
          : error instanceof Error
            ? error.message
            : "保存配置失败",
      );
    },
  });

  if (!item) return null;

  const minInUnit = item.min_value / unit.factor;
  const maxInUnit = item.max_value / unit.factor;
  const maxStringLength = item.max_string_length ?? 1024;

  const submit = () => {
    if (stringMode) {
      const trimmed = inputValue.trim();
      if (!trimmed) {
        setFormError("请输入非空路径");
        return;
      }
      if ([...trimmed].length > maxStringLength) {
        setFormError(`最长 ${maxStringLength} 个字符`);
        return;
      }
      updateMutation.mutate({ key: item.key, stringValue: trimmed });
      return;
    }
    const parsed = Number(inputValue);
    if (!Number.isFinite(parsed)) {
      setFormError("请输入数字");
      return;
    }
    const raw = Math.round(parsed * unit.factor);
    if (raw < item.min_value || raw > item.max_value) {
      setFormError(
        `取值必须在 ${formatConfigValue(item, item.min_value)} 到 ${formatConfigValue(item, item.max_value)} 之间`,
      );
      return;
    }
    updateMutation.mutate({ key: item.key, value: raw });
  };

  return (
    <Dialog
      open={item !== null}
      onOpenChange={(next) => {
        if (!next) setFormError(null);
        onOpenChange(next);
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>修改「{item.label}」</DialogTitle>
          <DialogDescription>{item.description}</DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            submit();
          }}
        >
          {isHighDangerConfig(item) ? (
            <Alert variant="destructive">
              <AlertTitle>高危配置</AlertTitle>
              <AlertDescription>
                不宜改动。改后不自动迁存量数据；节点本地 config.yaml /
                RUNTIME_AGENT_WORKSPACE_DIR 仍可覆盖平台值。
              </AlertDescription>
            </Alert>
          ) : null}
          <label className="flex flex-col gap-1.5">
            <span className="text-sm text-muted-foreground">
              {stringMode
                ? `路径文本，最长 ${maxStringLength} 字符`
                : `取值${unit.label ? `（${unit.label}）` : ""}，范围 ${minInUnit} – ${maxInUnit}`}
            </span>
            <div className="flex items-center gap-2">
              <input
                autoFocus
                inputMode={stringMode ? "text" : "decimal"}
                className="h-10 w-full rounded-md border bg-background px-3 text-sm tabular-nums"
                value={inputValue}
                onChange={(event) => setInputValue(event.target.value)}
              />
              {unit.label ? (
                <span className="shrink-0 text-sm text-muted-foreground">{unit.label}</span>
              ) : null}
            </div>
          </label>
          <p className="text-xs text-muted-foreground">
            默认值 {displayDefaultValue(item)}
            ，当前生效 {displayEffectiveValue(item)}。保存后约 15 秒内生效。
          </p>
          {formError ? <p className="text-sm text-destructive">{formError}</p> : null}
          <DialogFooter>
            <V3Button
              type="button"
              variant="ghost"
              onClick={() => onOpenChange(false)}
              disabled={updateMutation.isPending}
            >
              取消
            </V3Button>
            <V3Button type="submit" disabled={updateMutation.isPending}>
              {updateMutation.isPending ? "保存中…" : "保存"}
            </V3Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

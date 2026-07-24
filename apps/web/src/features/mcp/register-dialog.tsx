import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { KeyRound } from "lucide-react";
import { Button } from "@/components/superteam";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from "@/components/ui/dialog";
import {
  createMcpServerDefinition,
  type CreateMcpServerDefinitionInput
} from "@/lib/api/capabilities";

type RegisterMcpDialogProps = {
  apiBaseUrl: string;
  fetcher?: typeof fetch;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const EMPTY_FORM: CreateMcpServerDefinitionInput = {
  name: "",
  server_key: "",
  description: "",
  transport: "streamable_http",
  url: "",
  auth_strategy: "none",
  required_env_vars: [],
  risk_level: "medium"
};

const inputClass = "h-10 w-full rounded-md border bg-background px-3 text-sm";

function FieldLabel({ children, required }: { children: string; required?: boolean }) {
  return (
    <span className="text-sm text-muted-foreground">
      {children}
      {required ? <span className="ml-0.5 text-destructive">*</span> : null}
    </span>
  );
}

export function RegisterMcpDialog({
  apiBaseUrl,
  fetcher,
  open,
  onOpenChange
}: RegisterMcpDialogProps) {
  const queryClient = useQueryClient();
  const [form, setForm] = useState<CreateMcpServerDefinitionInput>(EMPTY_FORM);
  const [requiredEnvInput, setRequiredEnvInput] = useState("");
  const [formError, setFormError] = useState<string | null>(null);

  const resetAndClose = () => {
    setForm(EMPTY_FORM);
    setRequiredEnvInput("");
    setFormError(null);
    onOpenChange(false);
  };

  const createMutation = useMutation({
    mutationFn: (input: CreateMcpServerDefinitionInput) =>
      createMcpServerDefinition({ baseUrl: apiBaseUrl, fetcher }, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["mcp-server-definitions"] });
      resetAndClose();
    },
    onError: (error: unknown) => {
      setFormError(error instanceof Error ? error.message : "创建 MCP 失败");
    }
});

  const addRequiredEnv = () => {
    const name = requiredEnvInput.trim();
    if (!name || form.required_env_vars?.includes(name)) {
      setRequiredEnvInput("");
      return;
    }
    setForm((prev) => ({
      ...prev,
      required_env_vars: [...(prev.required_env_vars ?? []), name]
}));
    setRequiredEnvInput("");
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) {
          resetAndClose();
          return;
        }
        onOpenChange(next);
      }}
    >
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>注册新 MCP</DialogTitle>
          <DialogDescription>
            登记服务器上已部署的 HTTP/streamable HTTP MCP 能力，注册后可绑定到团队或数字员工。
          </DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            createMutation.mutate(form);
          }}
        >
          <div className="grid gap-4 sm:grid-cols-2">
            <label className="flex flex-col gap-1">
              <FieldLabel required>名称</FieldLabel>
              <input
                className={inputClass}
                placeholder="如 GitHub MCP"
                value={form.name}
                onChange={(event) => setForm((prev) => ({ ...prev, name: event.target.value }))}
                required
              />
            </label>
            <label className="flex flex-col gap-1">
              <FieldLabel required>server_key</FieldLabel>
              <input
                className={`${inputClass} font-mono`}
                placeholder="如 github-mcp，仅字母数字_-"
                value={form.server_key}
                onChange={(event) =>
                  setForm((prev) => ({ ...prev, server_key: event.target.value }))
                }
                required
                pattern="[A-Za-z0-9_\-]+"
                title="仅允许字母、数字、下划线和连字符"
              />
            </label>
            <label className="flex flex-col gap-1 sm:col-span-2">
              <FieldLabel required>URL</FieldLabel>
              <input
                className={`${inputClass} font-mono`}
                placeholder="https://mcp.example.com/github"
                value={form.url}
                onChange={(event) => setForm((prev) => ({ ...prev, url: event.target.value }))}
                required
                type="url"
              />
            </label>
            <label className="flex flex-col gap-1 sm:col-span-2">
              <FieldLabel>描述</FieldLabel>
              <textarea
                className="min-h-16 w-full rounded-md border bg-background px-3 py-2 text-sm"
                placeholder="这个 MCP 提供什么能力、适合哪些场景"
                value={form.description}
                onChange={(event) =>
                  setForm((prev) => ({ ...prev, description: event.target.value }))
                }
              />
            </label>
          </div>
          <div className="grid gap-4 sm:grid-cols-3">
            <label className="flex flex-col gap-1">
              <FieldLabel>传输方式</FieldLabel>
              <select
                className={inputClass}
                value={form.transport}
                onChange={(event) =>
                  setForm((prev) => ({
                    ...prev,
                    transport: event.target.value as CreateMcpServerDefinitionInput["transport"]
}))
                }
              >
                <option value="streamable_http">streamable_http</option>
                <option value="http">http</option>
              </select>
            </label>
            <label className="flex flex-col gap-1">
              <FieldLabel>鉴权方式</FieldLabel>
              <select
                className={inputClass}
                value={form.auth_strategy}
                onChange={(event) =>
                  setForm((prev) => ({
                    ...prev,
                    auth_strategy:
                      event.target.value as CreateMcpServerDefinitionInput["auth_strategy"]
}))
                }
              >
                <option value="none">none</option>
                <option value="bearer_env">bearer_env</option>
                <option value="headers_env">headers_env</option>
              </select>
            </label>
            <label className="flex flex-col gap-1">
              <FieldLabel>风险等级</FieldLabel>
              <select
                className={inputClass}
                value={form.risk_level}
                onChange={(event) =>
                  setForm((prev) => ({ ...prev, risk_level: event.target.value }))
                }
              >
                <option value="low">low</option>
                <option value="medium">medium</option>
                <option value="high">high</option>
              </select>
            </label>
          </div>
          <div className="flex flex-col gap-2">
            <FieldLabel>必需环境变量</FieldLabel>
            {(form.required_env_vars ?? []).length > 0 ? (
              <div className="flex flex-wrap gap-2">
                {(form.required_env_vars ?? []).map((name) => (
                  <span
                    key={name}
                    className="inline-flex items-center gap-1 rounded-md border bg-muted px-2 py-1 font-mono text-xs"
                  >
                    <KeyRound className="size-3" />
                    {name}
                    <button
                      type="button"
                      aria-label={`移除环境变量 ${name}`}
                      className="text-muted-foreground hover:text-foreground"
                      onClick={() =>
                        setForm((prev) => ({
                          ...prev,
                          required_env_vars: (prev.required_env_vars ?? []).filter(
                            (value) => value !== name,
                          )
}))
                      }
                    >
                      ×
                    </button>
                  </span>
                ))}
              </div>
            ) : null}
            <div className="flex gap-2">
              <input
                className={`${inputClass} max-w-72 font-mono`}
                placeholder="例如 GITHUB_TOKEN"
                aria-label="必需环境变量输入"
                value={requiredEnvInput}
                onChange={(event) => setRequiredEnvInput(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault();
                    addRequiredEnv();
                  }
                }}
              />
              <Button type="button" variant="outline" onClick={addRequiredEnv}>
                添加
              </Button>
            </div>
          </div>
          {formError ? <p className="text-sm text-destructive">{formError}</p> : null}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={resetAndClose}>
              取消
            </Button>
            <Button type="submit" disabled={createMutation.isPending}>
              {createMutation.isPending ? "创建中…" : "创建"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

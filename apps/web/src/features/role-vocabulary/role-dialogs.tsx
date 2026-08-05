import { useEffect, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/superteam";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import type { RoleVocabularyEntry } from "@/lib/api/casting";
import { isValidRoleKey } from "./role-key";

type CreateInput = {
  role_key: string;
  title: string;
  description: string;
};

type EditInput = {
  title: string;
  description: string;
};

export function CreateRoleDialog({
  open,
  onOpenChange,
  isSubmitting,
  onSubmit,
  error,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  isSubmitting: boolean;
  onSubmit: (input: CreateInput) => void;
  error: string | null;
}) {
  const [roleKey, setRoleKey] = useState("");
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [localError, setLocalError] = useState<string | null>(null);

  useEffect(() => {
    if (!open) {
      setRoleKey("");
      setTitle("");
      setDescription("");
      setLocalError(null);
    }
  }, [open]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>新建角色</DialogTitle>
          <DialogDescription>
            role_key 创建后不可改，须为下划线小写（与能力词表同规范）。
          </DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            const key = roleKey.trim();
            const name = title.trim();
            if (!isValidRoleKey(key)) {
              setLocalError("role_key 须为小写字母开头的 snake_case（如 network_diagnostics）");
              return;
            }
            if (!name) {
              setLocalError("中文名不能为空");
              return;
            }
            setLocalError(null);
            onSubmit({ role_key: key, title: name, description: description.trim() });
          }}
        >
          <div className="space-y-2">
            <Label htmlFor="role-key">role_key</Label>
            <Input
              id="role-key"
              className="font-mono"
              placeholder="network_diagnostics"
              value={roleKey}
              onChange={(event) => setRoleKey(event.target.value)}
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="role-title">中文名</Label>
            <Input
              id="role-title"
              placeholder="网络诊断"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="role-description">说明</Label>
            <Textarea
              id="role-description"
              rows={3}
              placeholder="该角色在剧本中的职责边界"
              value={description}
              onChange={(event) => setDescription(event.target.value)}
            />
          </div>
          {localError || error ? (
            <p className="text-sm text-destructive">{localError || error}</p>
          ) : null}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? "创建中…" : "创建"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function EditRoleDialog({
  entry,
  onOpenChange,
  isSubmitting,
  onSubmit,
  error,
}: {
  entry: RoleVocabularyEntry | null;
  onOpenChange: (open: boolean) => void;
  isSubmitting: boolean;
  onSubmit: (input: EditInput) => void;
  error: string | null;
}) {
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [localError, setLocalError] = useState<string | null>(null);

  useEffect(() => {
    if (entry) {
      setTitle(entry.title);
      setDescription(entry.description ?? "");
      setLocalError(null);
    }
  }, [entry]);

  return (
    <Dialog open={entry !== null} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>编辑角色</DialogTitle>
          <DialogDescription>
            仅可改中文名与说明；role_key{" "}
            <span className="font-mono">{entry?.role_key}</span> 为稳定标识，不可改。
          </DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-4"
          onSubmit={(event) => {
            event.preventDefault();
            const name = title.trim();
            if (!name) {
              setLocalError("中文名不能为空");
              return;
            }
            setLocalError(null);
            onSubmit({ title: name, description: description.trim() });
          }}
        >
          <div className="space-y-2">
            <Label htmlFor="edit-role-title">中文名</Label>
            <Input
              id="edit-role-title"
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              required
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="edit-role-description">说明</Label>
            <Textarea
              id="edit-role-description"
              rows={3}
              value={description}
              onChange={(event) => setDescription(event.target.value)}
            />
          </div>
          {localError || error ? (
            <p className="text-sm text-destructive">{localError || error}</p>
          ) : null}
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button type="submit" disabled={isSubmitting}>
              {isSubmitting ? "保存中…" : "保存"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

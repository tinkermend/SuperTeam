import { Link } from "@tanstack/react-router";
import { Checkbox } from "@/components/ui/checkbox";
import type { RoleVocabularyEntry } from "@/lib/api/casting";

type RoleKeysPickerProps = {
  options: RoleVocabularyEntry[];
  selected: string[];
  loading?: boolean;
  error?: boolean;
  required?: boolean;
  onChange: (next: string[]) => void;
};

export function RoleKeysPicker({
  options,
  selected,
  loading,
  error,
  required,
  onChange,
}: RoleKeysPickerProps) {
  const active = options.filter((entry) => entry.status === "active" || selected.includes(entry.role_key));

  function toggle(roleKey: string) {
    onChange(
      selected.includes(roleKey)
        ? selected.filter((key) => key !== roleKey)
        : [...selected, roleKey],
    );
  }

  if (loading) {
    return <p className="text-sm text-ink-2">加载角色词表…</p>;
  }
  if (error) {
    return <p className="text-sm text-destructive">无法加载角色词表</p>;
  }
  if (active.length === 0) {
    return (
      <p className="text-sm text-ink-2">
        暂无启用中的角色。请先在{" "}
        <Link className="underline" to="/role-vocabulary">
          角色词表
        </Link>{" "}
        注册。
      </p>
    );
  }

  return (
    <div className="flex flex-wrap gap-2" role="group" aria-required={required}>
      {active.map((entry) => {
        const checked = selected.includes(entry.role_key);
        return (
          <label
            key={entry.role_key}
            className={`inline-flex cursor-pointer items-center gap-2 rounded-inner border px-3 py-2 text-sm ${
              checked
                ? "border-brand/40 bg-brand/5 text-ink"
                : "border-line bg-card text-ink-2"
            }`}
          >
            <Checkbox
              checked={checked}
              onCheckedChange={(value) => {
                if (typeof value !== "boolean" || value === checked) return;
                toggle(entry.role_key);
              }}
              aria-label={`剧本角色 ${entry.title}`}
            />
            <span className="font-medium">{entry.title}</span>
          </label>
        );
      })}
    </div>
  );
}

export function firstRoleTitle(
  selected: string[],
  options: RoleVocabularyEntry[],
): string {
  const key = selected[0];
  if (!key) return "";
  return options.find((entry) => entry.role_key === key)?.title || key;
}

import { stringArray } from "../config-utils";

export type PermissionPolicySlice = {
  grants: string[];
  allowed_actions: string[];
};

export function policySlice(value: unknown): PermissionPolicySlice {
  const record = value && typeof value === "object" ? (value as Record<string, unknown>) : {};
  return {
    grants: stringArray(record.grants),
    allowed_actions: stringArray(record.allowed_actions),
  };
}

function DiffList({
  title,
  current,
  target,
}: {
  title: string;
  current: string[];
  target: string[];
}) {
  const currentSet = new Set(current);
  const targetSet = new Set(target);
  const added = target.filter((item) => !currentSet.has(item));
  const removed = current.filter((item) => !targetSet.has(item));
  const kept = current.filter((item) => targetSet.has(item));

  return (
    <div className="space-y-2">
      <h4 className="text-xs font-semibold text-ink-2">{title}</h4>
      {added.length + removed.length + kept.length === 0 ? (
        <p className="text-xs text-ink-3">无变更</p>
      ) : (
        <ul className="space-y-1 font-mono text-xs text-ink">
          {added.map((item) => (
            <li key={`add-${item}`}>新增 {item}</li>
          ))}
          {removed.map((item) => (
            <li key={`rm-${item}`}>移除 {item}</li>
          ))}
          {kept.map((item) => (
            <li key={`keep-${item}`}>保持 {item}</li>
          ))}
        </ul>
      )}
    </div>
  );
}

export function PermissionChangeDiff({
  current,
  target,
  currentRole,
  targetRole,
}: {
  current?: unknown;
  target?: unknown;
  currentRole?: string;
  targetRole?: string;
}) {
  const from = policySlice(current);
  const to = policySlice(target);
  return (
    <div className="space-y-4">
      {currentRole || targetRole ? (
        <p className="text-xs text-ink-2">
          在途职责描述：{currentRole || "—"} → {targetRole || "—"}
        </p>
      ) : null}
      <DiffList title="资源授权" current={from.grants} target={to.grants} />
      <DiffList title="动作白名单" current={from.allowed_actions} target={to.allowed_actions} />
    </div>
  );
}

import type { ComponentType, ReactNode, SVGProps } from "react";
import { AlertTriangle, Ban, Inbox, LoaderCircle } from "lucide-react";

import { IconBadge } from "@superteam/ui";

type StateIcon = ComponentType<SVGProps<SVGSVGElement>>;
type StateTone = "info" | "warning" | "danger" | "neutral";

type PlatformStateProps = {
  action?: ReactNode;
  description?: ReactNode;
  icon?: StateIcon;
  title: ReactNode;
};

function PlatformStateFrame({
  action,
  description,
  icon: Icon,
  role,
  title,
  tone,
}: Omit<PlatformStateProps, "icon"> & { icon: StateIcon; role?: "alert" | "status"; tone: StateTone }) {
  return (
    <section
      className="flex min-h-56 min-w-0 flex-col items-center justify-center gap-4 rounded-md border border-dashed bg-muted/20 px-4 py-10 text-center"
      role={role}
    >
      <IconBadge label={typeof title === "string" ? title : "平台状态"} tone={tone}>
        <Icon aria-hidden="true" />
      </IconBadge>
      <div className="flex min-w-0 max-w-md flex-col gap-2">
        <h2 className="break-words text-base font-semibold">{title}</h2>
        {description ? <p className="break-all text-sm text-muted-foreground sm:break-normal">{description}</p> : null}
      </div>
      {action ? <div className="flex flex-wrap items-center justify-center gap-2">{action}</div> : null}
    </section>
  );
}

export function PlatformEmptyState({ action, description, icon = Inbox, title }: PlatformStateProps) {
  return <PlatformStateFrame action={action} description={description} icon={icon} title={title} tone="neutral" />;
}

export function PlatformLoadingState({ action, description, icon = LoaderCircle, title }: PlatformStateProps) {
  return <PlatformStateFrame action={action} description={description} icon={icon} role="status" title={title} tone="info" />;
}

export function PlatformErrorState({ action, description, icon = AlertTriangle, title }: PlatformStateProps) {
  return <PlatformStateFrame action={action} description={description} icon={icon} role="alert" title={title} tone="danger" />;
}

export function PlatformForbiddenState({ action, description, icon = Ban, title }: PlatformStateProps) {
  return <PlatformStateFrame action={action} description={description} icon={icon} role="alert" title={title} tone="warning" />;
}

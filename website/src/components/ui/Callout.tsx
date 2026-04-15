import { cn } from "@/lib/cn";
import { Info, AlertTriangle, Lightbulb } from "lucide-react";

type CalloutType = "tip" | "warning" | "info";

const CALLOUT_CONFIG: Record<
  CalloutType,
  { icon: typeof Info; borderClass: string; iconClass: string; bgClass: string }
> = {
  tip: {
    icon: Lightbulb,
    borderClass: "border-willow-accent/20",
    iconClass: "text-willow-accent",
    bgClass: "bg-willow-accent/[0.04]",
  },
  warning: {
    icon: AlertTriangle,
    borderClass: "border-status-wait/20",
    iconClass: "text-status-wait",
    bgClass: "bg-status-wait/[0.04]",
  },
  info: {
    icon: Info,
    borderClass: "border-willow-text-3/20",
    iconClass: "text-willow-text-3",
    bgClass: "bg-willow-bg-soft",
  },
};

interface CalloutProps {
  type?: CalloutType;
  children: React.ReactNode;
}

export function Callout({ type = "tip", children }: CalloutProps) {
  const config = CALLOUT_CONFIG[type];
  const Icon = config.icon;

  return (
    <div
      className={cn(
        "my-6 flex gap-3 rounded-lg border p-4",
        config.borderClass,
        config.bgClass,
      )}
    >
      <Icon className={cn("mt-0.5 h-5 w-5 shrink-0", config.iconClass)} />
      <div className="text-sm text-willow-text-2 [&>p]:m-0">{children}</div>
    </div>
  );
}

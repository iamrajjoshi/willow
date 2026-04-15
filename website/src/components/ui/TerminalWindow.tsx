import { cn } from "@/lib/cn";

interface TerminalWindowProps {
  title?: string;
  children: React.ReactNode;
  className?: string;
}

export function TerminalWindow({
  title = "willow",
  children,
  className,
}: TerminalWindowProps) {
  return (
    <div
      className={cn(
        "overflow-hidden rounded-xl border border-willow-border bg-willow-bg-code",
        className,
      )}
    >
      <div className="flex items-center gap-2 border-b border-willow-border bg-willow-bg-mute px-4 py-3">
        <span className="h-3 w-3 rounded-full bg-willow-text-dim" />
        <span className="h-3 w-3 rounded-full bg-willow-text-dim" />
        <span className="h-3 w-3 rounded-full bg-willow-text-dim" />
        <span className="flex-1 text-center font-mono text-xs text-willow-text-3 mr-[52px]">
          {title}
        </span>
      </div>
      <div>{children}</div>
    </div>
  );
}

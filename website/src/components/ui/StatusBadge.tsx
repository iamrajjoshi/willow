"use client";

import { motion } from "framer-motion";
import { cn } from "@/lib/cn";

type Status = "busy" | "done" | "wait" | "idle";

const STATUS_CONFIG: Record<
  Status,
  { icon: string; label: string; colorClass: string; bgClass: string; borderClass: string }
> = {
  busy: {
    icon: "🤖",
    label: "BUSY",
    colorClass: "text-status-busy",
    bgClass: "bg-status-busy/[0.06]",
    borderClass: "border-status-busy/[0.12]",
  },
  done: {
    icon: "✅",
    label: "DONE",
    colorClass: "text-status-done",
    bgClass: "bg-status-done/[0.06]",
    borderClass: "border-status-done/[0.12]",
  },
  wait: {
    icon: "⏳",
    label: "WAIT",
    colorClass: "text-status-wait",
    bgClass: "bg-status-wait/[0.06]",
    borderClass: "border-status-wait/[0.12]",
  },
  idle: {
    icon: "🟡",
    label: "IDLE",
    colorClass: "text-status-idle",
    bgClass: "bg-status-idle/[0.06]",
    borderClass: "border-status-idle/[0.12]",
  },
};

interface StatusBadgeProps {
  status: Status;
}

export function StatusBadge({ status }: StatusBadgeProps) {
  const config = STATUS_CONFIG[status];

  return (
    <motion.span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-full border px-3 py-1 font-mono text-xs font-semibold tracking-wider",
        config.colorClass,
        config.bgClass,
        config.borderClass,
      )}
      {...(status === "busy" && {
        animate: { scale: [1, 1.04, 1] },
        transition: { duration: 2, repeat: Infinity, ease: "easeInOut" },
      })}
    >
      {config.icon} {config.label}
    </motion.span>
  );
}

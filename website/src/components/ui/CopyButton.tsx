"use client";

import { Check, Copy } from "lucide-react";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";
import { cn } from "@/lib/cn";

interface CopyButtonProps {
  text: string;
  className?: string;
}

export function CopyButton({ text, className }: CopyButtonProps) {
  const { copied, copy } = useCopyToClipboard();

  return (
    <button
      onClick={() => copy(text)}
      className={cn(
        "text-willow-text-3 transition-colors hover:text-willow-text-1",
        className,
      )}
      title="Copy to clipboard"
    >
      {copied ? (
        <Check className="h-4 w-4 text-willow-accent" />
      ) : (
        <Copy className="h-4 w-4" />
      )}
    </button>
  );
}

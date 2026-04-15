"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { getPrevNext } from "@/lib/docs-nav";
import { ArrowLeft, ArrowRight } from "lucide-react";

export function PrevNextLinks() {
  const pathname = usePathname();
  const { prev, next } = getPrevNext(pathname);

  if (!prev && !next) return null;

  return (
    <div className="mt-16 flex justify-between gap-4 border-t border-willow-border pt-6">
      {prev ? (
        <Link
          href={prev.href}
          className="flex items-center gap-2 text-sm text-willow-text-3 transition-colors hover:text-willow-accent"
        >
          <ArrowLeft className="h-4 w-4" />
          {prev.title}
        </Link>
      ) : (
        <div />
      )}
      {next ? (
        <Link
          href={next.href}
          className="flex items-center gap-2 text-sm text-willow-text-3 transition-colors hover:text-willow-accent"
        >
          {next.title}
          <ArrowRight className="h-4 w-4" />
        </Link>
      ) : (
        <div />
      )}
    </div>
  );
}

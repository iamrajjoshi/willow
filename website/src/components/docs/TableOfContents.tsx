"use client";

import { useState, useEffect } from "react";
import { usePathname } from "next/navigation";
import { useScrollSpy } from "@/hooks/use-scroll-spy";
import { cn } from "@/lib/cn";

interface TocItem {
  id: string;
  text: string;
  level: number;
}

export function TableOfContents() {
  const pathname = usePathname();
  const [headings, setHeadings] = useState<TocItem[]>([]);

  useEffect(() => {
    // Re-scan headings whenever the page changes
    const timer = setTimeout(() => {
      const article = document.querySelector("article");
      if (!article) return;

      const elements = article.querySelectorAll("h2[id], h3[id]");
      const items: TocItem[] = Array.from(elements).map((el) => ({
        id: el.id,
        text: el.textContent || "",
        level: el.tagName === "H2" ? 2 : 3,
      }));

      setHeadings(items);
    }, 100); // small delay to ensure DOM is updated after navigation

    return () => clearTimeout(timer);
  }, [pathname]);

  const activeId = useScrollSpy(
    headings.map((h) => h.id),
    80,
  );

  if (headings.length === 0) return null;

  return (
    <nav className="space-y-1">
      <h4 className="mb-3 text-xs font-semibold uppercase tracking-wider text-willow-text-3">
        On this page
      </h4>
      {headings.map((heading) => (
        <a
          key={heading.id}
          href={`#${heading.id}`}
          onClick={(e) => {
            e.preventDefault();
            document.getElementById(heading.id)?.scrollIntoView({
              behavior: "smooth",
              block: "start",
            });
            // Update URL hash without jump
            history.pushState(null, "", `#${heading.id}`);
          }}
          className={cn(
            "block text-[13px] leading-relaxed transition-colors duration-150",
            heading.level === 3 ? "pl-3" : "",
            activeId === heading.id
              ? "text-willow-accent"
              : "text-willow-text-dim hover:text-willow-text-2",
          )}
        >
          {heading.text}
        </a>
      ))}
    </nav>
  );
}

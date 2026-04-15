"use client";

import { usePathname } from "next/navigation";
import { useState, useEffect } from "react";
import { DOCS_NAV } from "@/lib/docs-nav";
import { useScrollSpy } from "@/hooks/use-scroll-spy";
import { cn } from "@/lib/cn";

interface TocItem {
  id: string;
  text: string;
  level: number;
}

export function Sidebar() {
  const pathname = usePathname();
  const [headings, setHeadings] = useState<TocItem[]>([]);

  useEffect(() => {
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
    }, 100);

    return () => clearTimeout(timer);
  }, [pathname]);

  const activeId = useScrollSpy(
    headings.map((h) => h.id),
    80,
  );

  const currentPage = DOCS_NAV.flatMap((g) => g.items).find(
    (item) => item.href === pathname,
  );

  if (headings.length === 0) return null;

  return (
    <nav>
      <h4 className="mb-3 text-xs font-semibold uppercase tracking-wider text-willow-text-3">
        {currentPage?.title ?? "On this page"}
      </h4>
      <ul className="space-y-0.5">
        {headings.map((heading) => (
          <li key={heading.id}>
            <a
              href={`#${heading.id}`}
              onClick={(e) => {
                e.preventDefault();
                document.getElementById(heading.id)?.scrollIntoView({
                  behavior: "smooth",
                  block: "start",
                });
                history.pushState(null, "", `#${heading.id}`);
              }}
              className={cn(
                "block py-0.5 text-[13px] leading-snug transition-colors duration-150",
                heading.level === 3 ? "pl-6" : "pl-3",
                activeId === heading.id
                  ? "text-willow-accent"
                  : "text-willow-text-dim hover:text-willow-text-2",
              )}
            >
              {heading.text}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  );
}

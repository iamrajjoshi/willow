"use client";

import { usePathname } from "next/navigation";
import { DOCS_NAV, getPageHeadings } from "@/lib/docs-nav";
import { useScrollSpy } from "@/hooks/use-scroll-spy";
import { cn } from "@/lib/cn";

export function Sidebar() {
  const pathname = usePathname();
  const headings = getPageHeadings(pathname);
  const activeId = useScrollSpy(
    headings.map((h) => h.id),
    0,
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
        {headings.map((heading, i) => {
          const isSection = heading.level === 2;
          return (
            <li
              key={heading.id}
              className={cn(isSection && i > 0 && "mt-4")}
            >
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
                  "block py-0.5 leading-snug transition-colors duration-150",
                  isSection
                    ? "pl-3 text-[11px] font-semibold uppercase tracking-wider"
                    : "pl-6 text-[13px]",
                  activeId === heading.id
                    ? "text-willow-accent"
                    : isSection
                      ? "text-willow-text-2 hover:text-willow-text-1"
                      : "text-willow-text-dim hover:text-willow-text-2",
                )}
              >
                {heading.text}
              </a>
            </li>
          );
        })}
      </ul>
    </nav>
  );
}

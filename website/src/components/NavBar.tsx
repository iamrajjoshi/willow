"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { cn } from "@/lib/cn";
import { NAV_ITEMS } from "@/lib/constants";
import { Menu, X, Github } from "lucide-react";
import { useState, useEffect } from "react";

export function NavBar() {
  const pathname = usePathname();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [scrolled, setScrolled] = useState(false);

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 40);
    onScroll(); // check initial state
    window.addEventListener("scroll", onScroll, { passive: true });
    return () => window.removeEventListener("scroll", onScroll);
  }, []);

  return (
    <nav
      className={cn(
        "fixed top-0 left-0 right-0 z-50 transition-all duration-300",
        scrolled ? "top-3 px-4" : "top-0 px-0",
      )}
    >
      <div
        className={cn(
          "mx-auto flex h-14 max-w-5xl items-center justify-between transition-all duration-300",
          scrolled
            ? "rounded-full border border-white/[0.08] bg-willow-bg/70 px-5 shadow-lg shadow-black/20 backdrop-blur-xl"
            : "border-b border-transparent bg-transparent px-6",
        )}
      >
        <Link href="/" className="flex items-center gap-2">
          <img src="/favicon.svg" alt="willow" className="h-5 w-5" />
          <span className="font-heading text-sm font-bold text-willow-text-1">
            willow
          </span>
        </Link>

        {/* Desktop nav */}
        <div className="hidden items-center gap-1 md:flex">
          {NAV_ITEMS.map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className={cn(
                "rounded-full px-3.5 py-1.5 text-[13px] font-medium transition-colors",
                pathname.startsWith(item.href)
                  ? "bg-white/[0.06] text-willow-text-1"
                  : "text-willow-text-3 hover:text-willow-text-1",
              )}
            >
              {item.label}
            </Link>
          ))}

          <div className="mx-2 h-4 w-px bg-white/[0.08]" />

          <a
            href="https://github.com/iamrajjoshi/willow"
            target="_blank"
            rel="noopener noreferrer"
            className="rounded-full p-2 text-willow-text-3 transition-colors hover:bg-white/[0.06] hover:text-willow-text-1"
          >
            <Github className="h-4 w-4" />
          </a>
        </div>

        {/* Mobile toggle */}
        <button
          onClick={() => setMobileOpen(!mobileOpen)}
          className="rounded-full p-2 text-willow-text-2 md:hidden"
        >
          {mobileOpen ? (
            <X className="h-4 w-4" />
          ) : (
            <Menu className="h-4 w-4" />
          )}
        </button>
      </div>

      {/* Mobile menu */}
      {mobileOpen && (
        <div className="mx-auto mt-2 max-w-5xl overflow-hidden rounded-2xl border border-white/[0.08] bg-willow-bg/90 backdrop-blur-xl md:hidden">
          <div className="space-y-1 p-3">
            {NAV_ITEMS.map((item) => (
              <Link
                key={item.href}
                href={item.href}
                onClick={() => setMobileOpen(false)}
                className={cn(
                  "block rounded-lg px-4 py-2.5 text-sm transition-colors",
                  pathname.startsWith(item.href)
                    ? "bg-white/[0.06] text-willow-accent"
                    : "text-willow-text-2 hover:bg-white/[0.04] hover:text-willow-text-1",
                )}
              >
                {item.label}
              </Link>
            ))}
            <a
              href="https://github.com/iamrajjoshi/willow"
              target="_blank"
              rel="noopener noreferrer"
              className="flex items-center gap-2 rounded-lg px-4 py-2.5 text-sm text-willow-text-2 hover:text-willow-text-1"
            >
              <Github className="h-4 w-4" />
              GitHub
            </a>
          </div>
        </div>
      )}
    </nav>
  );
}

"use client";

import { useState } from "react";
import { ScrollReveal } from "@/components/ui/ScrollReveal";
import { SectionHeading } from "@/components/ui/SectionHeading";
import { CopyButton } from "@/components/ui/CopyButton";
import { cn } from "@/lib/cn";

const INSTALL_TABS = [
  {
    label: "Homebrew",
    command: "brew install iamrajjoshi/tap/willow",
  },
  {
    label: "From source",
    command: "go install github.com/iamrajjoshi/willow/cmd/willow@latest",
  },
  {
    label: "Shell integration",
    command: 'eval "$(willow shell-init)"',
  },
] as const;

export function InstallCTA() {
  const [activeTab, setActiveTab] = useState(0);

  return (
    <section className="py-24">
      <div className="mx-auto max-w-2xl px-6">
        <ScrollReveal>
          <SectionHeading title="Install" />
        </ScrollReveal>

        <ScrollReveal className="mt-10">
          <div className="overflow-hidden rounded-xl border border-willow-border bg-willow-bg-code">
            <div className="flex border-b border-willow-border bg-willow-bg-mute">
              {INSTALL_TABS.map((tab, i) => (
                <button
                  key={tab.label}
                  onClick={() => setActiveTab(i)}
                  className={cn(
                    "border-b-2 px-5 py-3 font-mono text-xs font-medium transition-colors",
                    activeTab === i
                      ? "border-willow-accent text-willow-text-1"
                      : "border-transparent text-willow-text-3 hover:text-willow-text-1",
                  )}
                >
                  {tab.label}
                </button>
              ))}
            </div>
            <div className="flex items-center justify-between p-5">
              <code className="font-mono text-sm text-willow-accent">
                {INSTALL_TABS[activeTab].command}
              </code>
              <CopyButton text={INSTALL_TABS[activeTab].command} />
            </div>
          </div>
        </ScrollReveal>
      </div>
    </section>
  );
}

"use client";

import { useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { SectionHeading } from "@/components/ui/SectionHeading";
import { ScrollReveal } from "@/components/ui/ScrollReveal";
import { COMMANDS } from "@/lib/constants";
import { cn } from "@/lib/cn";

export function CommandShowcase() {
  const [activeTab, setActiveTab] = useState(0);

  return (
    <section className="py-24">
      <div className="mx-auto max-w-4xl px-6">
        <ScrollReveal>
          <SectionHeading
            title="Commands"
            subtitle="Everything you need to manage worktrees and monitor AI agents."
          />
        </ScrollReveal>

        <ScrollReveal className="mt-12">
          <div className="overflow-hidden rounded-xl border border-willow-border bg-willow-bg-code">
            {/* Tab bar */}
            <div className="flex items-stretch border-b border-willow-border bg-willow-bg-mute">
              <div className="flex items-center gap-2 px-4">
                <span className="h-3 w-3 rounded-full bg-willow-text-dim" />
                <span className="h-3 w-3 rounded-full bg-willow-text-dim" />
                <span className="h-3 w-3 rounded-full bg-willow-text-dim" />
              </div>
              <div className="flex flex-wrap">
                {COMMANDS.map((cmd, i) => (
                  <button
                    key={cmd.name}
                    onClick={() => setActiveTab(i)}
                    className={cn(
                      "border-b-2 px-5 py-3 font-mono text-xs font-medium transition-colors",
                      activeTab === i
                        ? "border-willow-accent text-willow-text-1"
                        : "border-transparent text-willow-text-3 hover:text-willow-text-1",
                    )}
                  >
                    {cmd.name}
                  </button>
                ))}
              </div>
            </div>

            {/* Tab content */}
            <div className="p-5 font-mono text-sm leading-[1.7]">
              <AnimatePresence mode="wait">
                <motion.div
                  key={activeTab}
                  initial={{ opacity: 0, y: 8 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, y: -8 }}
                  transition={{ duration: 0.2 }}
                >
                  <div className="mb-2 text-willow-accent">
                    {COMMANDS[activeTab].command}
                  </div>
                  <pre className="whitespace-pre overflow-x-auto text-willow-text-2">
                    {COMMANDS[activeTab].output}
                  </pre>
                </motion.div>
              </AnimatePresence>
            </div>
          </div>
        </ScrollReveal>
      </div>
    </section>
  );
}

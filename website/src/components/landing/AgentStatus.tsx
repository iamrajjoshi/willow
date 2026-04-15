"use client";

import { ScrollReveal } from "@/components/ui/ScrollReveal";
import { SectionHeading } from "@/components/ui/SectionHeading";
import { StatusBadge } from "@/components/ui/StatusBadge";
import { TerminalWindow } from "@/components/ui/TerminalWindow";

export function AgentStatus() {
  return (
    <section className="py-24">
      <div className="mx-auto max-w-4xl px-6">
        <ScrollReveal>
          <SectionHeading
            title="Agent status"
            subtitle="After running ww cc-setup, Claude Code automatically reports its state."
          />
        </ScrollReveal>

        <ScrollReveal className="mt-10">
          <div className="flex flex-wrap justify-center gap-3">
            <StatusBadge status="busy" />
            <StatusBadge status="done" />
            <StatusBadge status="wait" />
            <StatusBadge status="idle" />
          </div>
        </ScrollReveal>

        <ScrollReveal className="mt-10">
          <TerminalWindow title="ww status">
            <img
              src="/demo-status.gif"
              alt="ww status demo"
              className="block w-full"
            />
          </TerminalWindow>
        </ScrollReveal>

        <ScrollReveal className="mt-6">
          <TerminalWindow title="ww dashboard">
            <img
              src="/demo-dashboard.gif"
              alt="ww dashboard demo"
              className="block w-full"
            />
          </TerminalWindow>
        </ScrollReveal>
      </div>
    </section>
  );
}

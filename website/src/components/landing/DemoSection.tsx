"use client";

import { ScrollReveal } from "@/components/ui/ScrollReveal";
import { TerminalWindow } from "@/components/ui/TerminalWindow";

export function DemoSection() {
  return (
    <section className="py-24">
      <div className="mx-auto max-w-4xl px-6">
        <ScrollReveal>
          <TerminalWindow title="ww new + ww ls + ww status">
            <img
              src="/demo-workflow.gif"
              alt="willow workflow demo"
              className="block w-full"
            />
          </TerminalWindow>
        </ScrollReveal>
      </div>
    </section>
  );
}

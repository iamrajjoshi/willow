"use client";

import { motion } from "framer-motion";
import { staggerContainer, fadeUp } from "@/lib/animations";
import { SectionHeading } from "@/components/ui/SectionHeading";
import { ScrollReveal } from "@/components/ui/ScrollReveal";
import { STEPS } from "@/lib/constants";

export function HowItWorks() {
  return (
    <section className="py-24">
      <div className="mx-auto max-w-6xl px-6">
        <ScrollReveal>
          <SectionHeading
            title="How it works"
            subtitle="Three steps from clone to full agent orchestration."
          />
        </ScrollReveal>

        <motion.div
          variants={staggerContainer}
          initial="hidden"
          whileInView="visible"
          viewport={{ once: true, margin: "-80px" }}
          className="relative mt-16 grid gap-6 md:grid-cols-3"
        >
          {/* Connecting line (desktop only) */}
          <div className="absolute top-14 left-[16.67%] right-[16.67%] hidden h-px md:block">
            <motion.div
              className="h-full bg-gradient-to-r from-transparent via-willow-accent/30 to-transparent"
              initial={{ scaleX: 0 }}
              whileInView={{ scaleX: 1 }}
              viewport={{ once: true }}
              transition={{ duration: 1, delay: 0.5, ease: "easeOut" }}
            />
          </div>

          {STEPS.map((step, i) => (
            <motion.div
              key={step.title}
              variants={fadeUp}
              className="relative rounded-2xl border border-willow-border bg-willow-bg-soft p-7 transition-colors hover:border-willow-border-hover"
            >
              <div className="mx-auto mb-4 flex h-8 w-8 items-center justify-center rounded-full bg-willow-accent font-mono text-sm font-bold text-willow-bg">
                {i + 1}
              </div>
              <h3 className="text-center font-heading text-lg font-semibold text-willow-text-1">
                {step.title}
              </h3>
              <p className="mt-2 text-center text-sm text-willow-text-2">
                {step.description}
              </p>
              <code className="mt-4 block overflow-x-auto whitespace-nowrap rounded-lg border border-willow-border bg-willow-bg-code px-3 py-2 text-center font-mono text-xs text-willow-accent">
                {step.command}
              </code>
            </motion.div>
          ))}
        </motion.div>
      </div>
    </section>
  );
}

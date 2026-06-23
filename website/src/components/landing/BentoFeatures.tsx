"use client";

import { motion } from "framer-motion";
import { staggerContainer, fadeUp } from "@/lib/animations";
import { SectionHeading } from "@/components/ui/SectionHeading";
import { BentoCard } from "./BentoCard";
import { FEATURES } from "@/lib/constants";
import { ScrollReveal } from "@/components/ui/ScrollReveal";

export function BentoFeatures() {
  return (
    <section className="py-24">
      <div className="mx-auto max-w-6xl px-6">
        <ScrollReveal>
          <SectionHeading
            title="Features"
            subtitle="Create, switch, monitor, and clean up worktrees."
          />
        </ScrollReveal>

        <motion.div
          variants={staggerContainer}
          initial="hidden"
          whileInView="visible"
          viewport={{ once: true, margin: "-80px" }}
          className="mt-16 grid gap-4 md:grid-cols-4"
        >
          {FEATURES.map((feature) => {
            const isLarge = "large" in feature && feature.large;
            const gif = "gif" in feature ? (feature.gif as string) : undefined;
            return (
              <motion.div
                key={feature.title}
                variants={fadeUp}
                className={isLarge ? "md:col-span-2 md:row-span-2" : ""}
              >
                <BentoCard
                  icon={feature.icon}
                  title={feature.title}
                  description={feature.description}
                  large={isLarge || undefined}
                  gif={gif}
                />
              </motion.div>
            );
          })}
        </motion.div>
      </div>
    </section>
  );
}

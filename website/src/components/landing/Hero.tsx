"use client";

import dynamic from "next/dynamic";
import { motion } from "framer-motion";
import { staggerContainer, fadeUp } from "@/lib/animations";
import { InstallCommand } from "@/components/ui/InstallCommand";
import { ArrowRight, Github } from "lucide-react";
import Link from "next/link";

const ShaderGradientBg = dynamic(
  () => import("@/components/ShaderGradientBg"),
  { ssr: false },
);

export function Hero() {
  return (
    <section className="relative flex min-h-screen items-center justify-center overflow-hidden">
      {/* Shader gradient background — contained to hero */}
      <div className="absolute inset-0 -z-10">
        <div
          className="absolute inset-0"
          style={{
            background:
              "radial-gradient(ellipse at 30% 40%, rgba(94,234,212,0.15) 0%, rgba(6,182,212,0.08) 40%, transparent 70%)",
          }}
        />
        <ShaderGradientBg />
      </div>

      <motion.div
        variants={staggerContainer}
        initial="hidden"
        animate="visible"
        className="mx-auto max-w-4xl px-6 pt-24 text-center"
      >
        <motion.div variants={fadeUp}>
          <Link href="/" className="inline-block">
            <img
              src="/favicon.svg"
              alt="willow"
              className="mx-auto mb-6 h-14 w-14"
            />
          </Link>
        </motion.div>

        <motion.h1
          variants={fadeUp}
          className="font-heading text-6xl font-extrabold tracking-tight sm:text-7xl lg:text-8xl"
        >
          <span className="bg-gradient-to-r from-willow-accent to-willow-accent-deep bg-clip-text text-transparent">
            willow
          </span>
        </motion.h1>

        <motion.div
          variants={fadeUp}
          className="mt-6 space-y-1 text-2xl font-medium tracking-tight text-willow-text-1 sm:text-3xl lg:text-4xl"
        >
          <p className="font-heading">Manage worktrees.</p>
          <p className="font-heading italic text-willow-accent">
            Monitor agents.
          </p>
          <p className="font-heading">Ship faster.</p>
        </motion.div>

        <motion.p
          variants={fadeUp}
          className="mx-auto mt-6 max-w-xl text-lg text-willow-text-2"
        >
          A git worktree manager built for AI agent workflows. Spin up isolated
          worktrees, switch between them with fzf, and see agent status in real
          time.
        </motion.p>

        <motion.div
          variants={fadeUp}
          className="mt-10 flex flex-col items-center gap-4 sm:flex-row sm:justify-center"
        >
          <Link
            href="/guide/"
            className="inline-flex items-center gap-2 rounded-full bg-willow-accent px-6 py-3 text-sm font-semibold text-willow-bg transition-colors hover:bg-willow-accent-mid"
          >
            Get Started
            <ArrowRight className="h-4 w-4" />
          </Link>
          <a
            href="https://github.com/iamrajjoshi/willow"
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-2 rounded-full border border-willow-border px-6 py-3 text-sm font-medium text-willow-text-1 transition-colors hover:border-willow-border-hover hover:bg-willow-bg-soft"
          >
            <Github className="h-4 w-4" />
            View on GitHub
          </a>
        </motion.div>

        <motion.div variants={fadeUp} className="mt-8">
          <InstallCommand />
        </motion.div>
      </motion.div>
    </section>
  );
}

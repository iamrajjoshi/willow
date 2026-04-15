import { Hero } from "@/components/landing/Hero";
import { DemoSection } from "@/components/landing/DemoSection";
import { BentoFeatures } from "@/components/landing/BentoFeatures";
import { HowItWorks } from "@/components/landing/HowItWorks";
import { CommandShowcase } from "@/components/landing/CommandShowcase";
import { AgentStatus } from "@/components/landing/AgentStatus";
import { InstallCTA } from "@/components/landing/InstallCTA";
import { NavBar } from "@/components/NavBar";
import { Footer } from "@/components/Footer";

export default function Home() {
  return (
    <>
      <NavBar />
      <main>
        <Hero />
        <DemoSection />
        <BentoFeatures />
        <HowItWorks />
        <CommandShowcase />
        <AgentStatus />
        <InstallCTA />
      </main>
      <Footer />
    </>
  );
}

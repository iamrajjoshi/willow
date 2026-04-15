import { GlowCard } from "@/components/ui/GlowCard";
import { TerminalWindow } from "@/components/ui/TerminalWindow";
import { cn } from "@/lib/cn";

interface BentoCardProps {
  icon: string;
  title: string;
  description: string;
  large?: boolean;
  gif?: string;
}

export function BentoCard({
  icon,
  title,
  description,
  large,
  gif,
}: BentoCardProps) {
  return (
    <GlowCard
      className={cn(large && "md:col-span-2 md:row-span-2")}
    >
      <div className={cn("p-6", large && "md:p-8")}>
        <div className="mb-3 text-2xl">{icon}</div>
        <h3
          className={cn(
            "font-heading font-semibold text-willow-text-1",
            large ? "text-xl" : "text-base",
          )}
        >
          {title}
        </h3>
        <p
          className={cn(
            "mt-2 text-willow-text-3",
            large ? "text-base" : "text-sm",
          )}
        >
          {description}
        </p>
        {large && gif && (
          <div className="mt-6">
            <TerminalWindow title={title.toLowerCase()}>
              <img
                src={gif}
                alt={`${title} demo`}
                className="block w-full"
              />
            </TerminalWindow>
          </div>
        )}
      </div>
    </GlowCard>
  );
}

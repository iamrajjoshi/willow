import { cn } from "@/lib/cn";

interface SectionHeadingProps {
  title: string;
  subtitle?: string;
  className?: string;
}

export function SectionHeading({
  title,
  subtitle,
  className,
}: SectionHeadingProps) {
  return (
    <div className={cn("text-center", className)}>
      <h2 className="font-heading text-3xl font-bold tracking-tight text-willow-text-1 sm:text-4xl">
        {title}
      </h2>
      {subtitle && (
        <p className="mx-auto mt-4 max-w-2xl text-lg text-willow-text-3">
          {subtitle}
        </p>
      )}
    </div>
  );
}

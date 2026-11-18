import type { ReactNode } from "react";
import type { LucideIcon } from "lucide-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { TERMS, type Tone } from "@/lib/words";

export const TONE: Record<Tone, string> = {
  signal: "bg-signal text-[#231a00]",
  live: "bg-live/10 text-live",
  review: "bg-review/10 text-review",
  go: "bg-go/10 text-go",
  focus: "bg-ring/10 text-ring",
  stop: "bg-destructive/10 text-destructive",
  caution: "bg-caution/15 text-caution-ink",
  muted: "bg-muted text-ink2 ring-1 ring-inset ring-border",
};

/** DOT is a tone as a solid mark: a point on a timeline. */
export const DOT: Record<Tone, string> = {
  signal: "bg-signal",
  live: "bg-live",
  review: "bg-review",
  go: "bg-go",
  focus: "bg-ring",
  stop: "bg-destructive",
  caution: "bg-caution",
  muted: "bg-border",
};

/** Term is a word of AWK with its meaning a hover or a tab away. */
export function Term({ children }: { children: keyof typeof TERMS & string }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          tabIndex={0}
          className="cursor-help underline decoration-muted-foreground decoration-dotted decoration-[1.5px] underline-offset-4"
        >
          {children}
        </span>
      </TooltipTrigger>
      <TooltipContent className="max-w-72 text-[13.5px] leading-relaxed">{TERMS[children]}</TooltipContent>
    </Tooltip>
  );
}

export function Tag({ tone = "muted", icon: Icon, children, className }: { tone?: Tone; icon?: LucideIcon; children: ReactNode; className?: string }) {
  return (
    <span className={cn("inline-flex shrink-0 items-center gap-1 whitespace-nowrap rounded-md px-1.5 py-[2px] text-[12.5px] font-bold", TONE[tone], className)}>
      {Icon && <Icon className="size-3.5" aria-hidden />}
      {children}
    </span>
  );
}

export function Panel({ children, className, ...rest }: { children: ReactNode; className?: string } & React.HTMLAttributes<HTMLElement>) {
  return (
    <section className={cn("rounded-xl border border-border bg-card text-card-foreground", className)} {...rest}>
      {children}
    </section>
  );
}

export function PanelHead({ title, sub, right }: { title: ReactNode; sub?: ReactNode; right?: ReactNode }) {
  return (
    <header className="flex items-center gap-3 px-5 pb-3 pt-4">
      <div className="min-w-0">
        <h2 className="text-[16px] font-extrabold tracking-tight">{title}</h2>
        {sub && <p className="mt-0.5 text-[13.5px] text-muted-foreground">{sub}</p>}
      </div>
      {right && <div className="ml-auto flex items-center gap-2">{right}</div>}
    </header>
  );
}

export function PageHead({ title, sub, right }: { title: ReactNode; sub?: ReactNode; right?: ReactNode }) {
  return (
    <div className="mb-6 flex flex-wrap items-end gap-4">
      <div className="min-w-0 max-w-3xl">
        <h1 className="text-[28px] font-black leading-tight tracking-tight">{title}</h1>
        {sub && <p className="mt-1.5 text-[15px] text-ink2">{sub}</p>}
      </div>
      {right && <div className="ml-auto flex flex-wrap items-center gap-2">{right}</div>}
    </div>
  );
}

/** Empty says why there is nothing, and what brings something. */
export function Empty({ icon: Icon, title, children }: { icon: LucideIcon; title: string; children?: ReactNode }) {
  return (
    <div className="grid place-items-center px-6 py-12 text-center">
      <span className="grid size-11 place-items-center rounded-lg bg-muted text-muted-foreground ring-1 ring-inset ring-border">
        <Icon className="size-5" aria-hidden />
      </span>
      <div className="mt-3 text-[15px] font-bold">{title}</div>
      {children && <div className="mt-1 max-w-md text-[14px] text-muted-foreground">{children}</div>}
    </div>
  );
}

/** Problem says what went wrong when something could not be read, in place of what it would have shown. */
export function Problem({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div role="alert" className="flex gap-3 rounded-lg border-l-4 border-destructive bg-destructive/5 px-4 py-3 text-[14px]">
      <div>
        <div className="font-bold">{title}</div>
        <div className="mt-0.5 text-ink2">{children}</div>
      </div>
    </div>
  );
}

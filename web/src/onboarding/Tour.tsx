import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { ArrowLeft, ArrowRight, Flag, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useProject } from "@/app/project";
import { useOnboarding } from "@/onboarding/state";
import { TOUR } from "@/onboarding/steps";
import { href, navigate, useRoute } from "@/lib/router";
import { cn } from "@/lib/utils";

interface Box {
  x: number;
  y: number;
  w: number;
  h: number;
}

const PAD = 8;
const HEADER = 72;

/** Where the target is now, with a little room around it. */
function boxOf(el: Element | null): Box | null {
  if (!el || !el.isConnected) return null;
  const r = el.getBoundingClientRect();
  if (r.width === 0 && r.height === 0) return null;
  return { x: r.left - PAD, y: r.top - PAD, w: r.width + PAD * 2, h: r.height + PAD * 2 };
}

function holePath(box: Box | null, vw: number, vh: number, r = 12): string {
  const frame = `M0 0H${vw}V${vh}H0Z`;
  if (!box) return frame;
  const { x, y, w, h } = box;
  return `${frame} M${x + r} ${y} H${x + w - r} A${r} ${r} 0 0 1 ${x + w} ${y + r} V${y + h - r} A${r} ${r} 0 0 1 ${x + w - r} ${y + h} H${x + r} A${r} ${r} 0 0 1 ${x} ${y + h - r} V${y + r} A${r} ${r} 0 0 1 ${x + r} ${y} Z`;
}

/**
 * Below the target, else above, else beside it; where there is no room at
 * all, a wide and short bar at the foot of the window.
 */
function place(pop: HTMLElement, box: Box | null) {
  pop.removeAttribute("data-dock");
  let w = pop.offsetWidth;
  let h = pop.offsetHeight;
  const gap = 14;
  const vw = window.innerWidth;
  const vh = window.innerHeight;
  let x: number;
  let y: number;
  if (!box) {
    x = (vw - w) / 2;
    y = (vh - h) / 2;
  } else if (box.y + box.h + gap + h < vh - 12) {
    x = box.x;
    y = box.y + box.h + gap;
  } else if (box.y - gap - h > 12) {
    x = box.x;
    y = box.y - gap - h;
  } else if (box.x + box.w + gap + w < vw - 12) {
    x = box.x + box.w + gap;
    y = box.y;
  } else if (box.x - gap - w > 12) {
    x = box.x - gap - w;
    y = box.y;
  } else {
    pop.setAttribute("data-dock", "");
    w = pop.offsetWidth;
    h = pop.offsetHeight;
    x = box.x + (box.w - w) / 2;
    y = vh - h - 16;
  }
  pop.style.left = `${Math.max(16, Math.min(x, vw - w - 16))}px`;
  pop.style.top = `${Math.max(16, Math.min(y, vh - h - 16))}px`;
}

/**
 * Tour walks the person through the workbench, one thing at a time: it goes
 * to the page of the step, darkens everything but what the step is about,
 * and says what that is. Esc leaves it at its step to go on later; the
 * arrow keys step through it.
 */
export function Tour() {
  const { state, step, next, back, pause, skipAll } = useOnboarding();
  const { project } = useProject();
  const route = useRoute();
  const active = state.status === "touring";
  const current = TOUR[step];
  const onPage = route.project === project.id && route.page === current.page && route.rest.length === 0;

  const [target, setTarget] = useState<Element | null>(null);
  const [box, setBox] = useState<Box | null>(null);
  const [view, setView] = useState({ w: 0, h: 0 });
  const popRef = useRef<HTMLDivElement>(null);
  const nextRef = useRef<HTMLButtonElement>(null);
  const boxRef = useRef<Box | null>(null);

  // The step's page first.
  useEffect(() => {
    if (active && !onPage) navigate(href({ project: project.id, page: current.page, rest: [] }));
  }, [active, onPage, project.id, current.page]);

  // Then what it points at, once the page has drawn it. The best target may
  // come with data a moment later; a stand-in is taken after a second, and
  // none at all after three, when the card shows in the middle.
  useEffect(() => {
    setTarget(null);
    if (!active || !onPage) return;
    let tries = 0;
    let timer = 0;
    const find = () => {
      tries++;
      const found = current.targets.map((t) => document.querySelector(`[data-tour="${t}"]`));
      const best = found[0];
      const standIn = found.find(Boolean) ?? null;
      if (best || (standIn && tries >= 10) || tries >= 30) {
        const el = best ?? standIn;
        if (el) {
          const r = el.getBoundingClientRect();
          const tall = r.height + 280 > window.innerHeight - HEADER;
          if (r.top < HEADER || r.bottom > window.innerHeight - 24 || (tall && r.top > HEADER + 16)) {
            window.scrollTo({ top: Math.max(0, r.top + window.scrollY - HEADER - 16), behavior: "instant" });
          }
        }
        setTarget(el);
        return;
      }
      timer = window.setTimeout(find, 100);
    };
    find();
    return () => window.clearTimeout(timer);
  }, [active, onPage, step, current]);

  const measure = useCallback(() => {
    const next = boxOf(target);
    boxRef.current = next;
    setBox(next);
    setView({ w: document.documentElement.clientWidth, h: window.innerHeight });
    if (popRef.current) place(popRef.current, next);
  }, [target]);

  // Scrolling, resizing, and a page still settling all move the spotlight
  // and the card; they are measured again, not rebuilt, so focus stays put.
  useLayoutEffect(() => {
    if (!active || !onPage) return;
    measure();
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(measure);
    [popRef.current, target, document.getElementById("main")].forEach((el) => el && observer?.observe(el));
    window.addEventListener("scroll", measure, { passive: true });
    window.addEventListener("resize", measure);
    const late = [window.setTimeout(measure, 60), window.setTimeout(measure, 300)];
    return () => {
      observer?.disconnect();
      window.removeEventListener("scroll", measure);
      window.removeEventListener("resize", measure);
      late.forEach((t) => window.clearTimeout(t));
    };
  }, [active, onPage, target, measure]);

  useEffect(() => {
    if (active && onPage) nextRef.current?.focus({ preventScroll: true });
  }, [active, onPage, step, target]);

  useEffect(() => {
    if (!active) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        pause();
      } else if (event.key === "ArrowRight") {
        event.preventDefault();
        next();
      } else if (event.key === "ArrowLeft") {
        event.preventDefault();
        back();
      } else if (event.key === "Tab" && popRef.current) {
        // Focus stays in the card while the rest of the page is dark.
        const items = [...popRef.current.querySelectorAll<HTMLElement>("button:not(:disabled)")];
        if (items.length === 0) return;
        const at = items.indexOf(document.activeElement as HTMLElement);
        const to = event.shiftKey ? (at <= 0 ? items.length - 1 : at - 1) : at === items.length - 1 ? 0 : at + 1;
        event.preventDefault();
        items[to].focus({ preventScroll: true });
      }
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, [active, next, back, pause]);

  if (!active || !onPage) return null;
  const last = step === TOUR.length - 1;

  return createPortal(
    <div data-testid="tour">
      <div className="fixed inset-0 z-[70]" aria-hidden onClick={() => nextRef.current?.focus()} />
      <svg className="pointer-events-none fixed inset-0 z-[70]" width={view.w} height={view.h} aria-hidden>
        <path fill="rgb(8 13 19 / 0.6)" fillRule="evenodd" d={holePath(box, view.w, view.h)} />
      </svg>
      {box && <div className="tour-spot pointer-events-none fixed z-[71] rounded-xl" style={{ left: box.x, top: box.y, width: box.w, height: box.h }} />}
      <div
        ref={popRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby="tour-title"
        aria-describedby="tour-body"
        className="tour-pop fixed z-[72] rounded-xl bg-popover text-popover-foreground shadow-2xl ring-1 ring-border"
        style={{ left: -9999, top: 0 }}
      >
        <div className="flex items-center gap-3 pr-8">
          <span className="text-[13px] font-bold text-muted-foreground tnum">
            第 {step + 1} 步，共 {TOUR.length} 步
          </span>
          <span className="flex gap-1" aria-hidden>
            {TOUR.map((_, i) => (
              <span key={i} className={cn("h-1.5 w-4 rounded-full", i < step ? "bg-foreground" : i === step ? "bg-signal ring-1 ring-[#D29A00]" : "bg-border")} />
            ))}
          </span>
        </div>
        <h2 id="tour-title" className="mt-2 text-[19px] font-extrabold leading-snug">
          {current.title}
        </h2>
        <p id="tour-body" className="mt-1.5 text-[15px] leading-relaxed text-ink2">
          {current.body}
        </p>
        <div className="mt-5 flex flex-wrap items-center gap-2">
          <button type="button" onClick={skipAll} className="text-[13.5px] font-bold text-muted-foreground underline-offset-4 hover:text-foreground hover:underline">
            跳過全部教學
          </button>
          <span className="ml-auto" />
          {step > 0 && (
            <Button variant="ghost" onClick={back}>
              <ArrowLeft aria-hidden /> 上一步
            </Button>
          )}
          <Button ref={nextRef} onClick={next} data-testid="tour-next">
            {last ? (
              <>
                <Flag aria-hidden /> 完成導覽
              </>
            ) : (
              <>
                下一步 <ArrowRight aria-hidden />
              </>
            )}
          </Button>
        </div>
        <button
          type="button"
          onClick={pause}
          aria-label="先停在這裡，下次接著看"
          title="先停在這裡，下次接著看"
          className="absolute right-3 top-3 grid size-8 place-items-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground"
        >
          <X className="size-[17px]" aria-hidden />
        </button>
      </div>
    </div>,
    document.body,
  );
}

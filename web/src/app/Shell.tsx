import { useEffect, useState, type ReactNode } from "react";
import { ChevronRight, ChevronsUpDown, FolderOpen, Lock, MapPin, Menu, Route, Search, SunMoon, WifiOff } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "@/components/ui/sheet";
import { useProject } from "@/app/project";
import { PAGES, pageTitle } from "@/app/pages";
import { Palette } from "@/app/Palette";
import { Checklist } from "@/onboarding/Checklist";
import { Help } from "@/onboarding/Help";
import { useOnboarding } from "@/onboarding/state";
import type { ProjectView } from "@/lib/api";
import { href, navigate, useRoute } from "@/lib/router";
import { planStatus } from "@/pages/plans/status";
import { awaitingApproval, workingSteps } from "@/lib/engine";
import { useTheme } from "@/lib/theme";
import { useAfterClose } from "@/lib/after-close";
import { cn } from "@/lib/utils";

function Brand() {
  return (
    <div className="flex items-center gap-3 px-2 pb-4 pt-2">
      <span className="grid size-9 place-items-center rounded-lg bg-signal text-[#231a00]">
        <Route className="size-[19px]" aria-hidden />
      </span>
      <div className="leading-tight">
        <div className="text-[16px] font-extrabold">AWK 工作台</div>
        <div className="text-[12px] text-white/60">AI Workflow Kit</div>
      </div>
    </div>
  );
}

function Initial({ name }: { name: string }) {
  return <span className="grid size-8 shrink-0 place-items-center rounded-md bg-[#0092A6] text-[14px] font-black text-white">{name.slice(0, 1).toUpperCase()}</span>;
}

/** ProjectSwitcher names the project shown, and goes to another. */
function ProjectSwitcher({ current, projects, onGo }: { current?: ProjectView; projects: ProjectView[]; onGo: Go }) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button type="button" className="mb-2 flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left transition-colors hover:bg-white/[.07]">
          {current ? <Initial name={current.name} /> : <FolderOpen className="size-5 text-white/70" aria-hidden />}
          <span className="min-w-0 flex-1">
            <span className="block truncate text-[14px] font-bold">{current?.name ?? "選一個專案"}</span>
            <span className="block truncate text-[12px] text-white/60">{current ? "換專案" : `${projects.length} 個專案`}</span>
          </span>
          <ChevronsUpDown className="size-4 text-white/50" aria-hidden />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-60">
        <DropdownMenuLabel>你的專案</DropdownMenuLabel>
        {projects.map((p) => (
          <DropdownMenuItem
            key={p.id}
            disabled={!!p.problem}
            onSelect={() => onGo(() => navigate(href({ project: p.id, page: "overview", rest: [] })))}
          >
            <Initial name={p.name} />
            <span className="min-w-0">
              <span className="block truncate font-bold">{p.name}</span>
              {p.problem && <span className="block truncate text-[12px] text-muted-foreground">{p.problem}</span>}
            </span>
          </DropdownMenuItem>
        ))}
        <DropdownMenuSeparator />
        <DropdownMenuItem
          onSelect={() => onGo(() => navigate("#/projects"))}
        >
          <FolderOpen aria-hidden />
          加入或移出專案
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function NavLinks({ onGo }: { onGo: Go }) {
  const { project, tickets, plans, lessons, engine } = useProject();
  const route = useRoute();
  // As the inbox lists them: a change that waits for approval needs you too.
  const awaiting = awaitingApproval(engine.data);
  const needs = (tickets.data?.tickets?.open ?? []).filter((t) => t.stage === "needs_you" || awaiting.has(t.number)).length;
  const all = plans.data?.plans ?? [];
  const heldBack = (lessons.data?.lessons ?? []).filter((l) => l.status === "quarantined").length;
  const waiting = needs + all.filter((p) => planStatus(p).needsYou).length + heldBack;
  const meeting = all.some((p) => p.meeting?.state === "meeting");
  return (
    <nav aria-label="主要" className="flex-1 overflow-y-auto pb-3">
      {PAGES.map((p) => {
        const here = route.page === p.page;
        return (
          <div key={p.page}>
            {p.group && <div className="px-3 pb-1 pt-4 text-[12px] font-bold text-white/45">{p.group}</div>}
            <a
              href={href({ project: project.id, page: p.page, rest: [] })}
              onClick={() => onGo()}
              aria-current={here ? "page" : undefined}
              className={cn(
                "relative flex items-center gap-3 rounded-lg px-3 py-2 text-[14px] font-semibold transition-colors",
                here ? "bg-white/[.1] text-white" : "text-white/70 hover:bg-white/[.06] hover:text-white",
              )}
            >
              {here && <span className="absolute -left-3 bottom-2 top-2 w-[3px] rounded-r bg-white" aria-hidden />}
              <p.icon className="size-[17px]" aria-hidden />
              {p.title}
              {p.page === "inbox" && waiting > 0 && (
                <span className="ml-auto rounded-md bg-signal px-1.5 text-[12px] font-extrabold text-[#231a00] tnum" aria-label={`${waiting} 件`}>
                  {waiting}
                </span>
              )}
              {p.page === "plans" && meeting && (
                <span className="ml-auto flex items-center gap-1.5 text-[12px] font-bold text-white/80">
                  <span className="relative flex size-2" aria-hidden>
                    <span className="absolute inset-0 animate-ping rounded-full bg-live opacity-70" />
                    <span className="relative size-2 rounded-full bg-live" />
                  </span>
                  開會中
                </span>
              )}
            </a>
          </div>
        );
      })}
    </nav>
  );
}

function Foot({ children }: { children?: ReactNode }) {
  const [theme, toggle] = useTheme();
  return (
    <div className="grid gap-2 pt-3">
      {children}
      <div className="flex items-center gap-2 px-2 text-[12px] text-white/55">
        <Lock className="size-3.5" aria-hidden />
        只在這台電腦上，127.0.0.1
        <button
          type="button"
          onClick={toggle}
          aria-label={theme === "dark" ? "換成淺色" : "換成深色"}
          title={theme === "dark" ? "換成淺色" : "換成深色"}
          className="ml-auto grid size-8 place-items-center rounded-md hover:bg-white/10"
        >
          <SunMoon className="size-4" aria-hidden />
        </button>
      </div>
    </div>
  );
}

/**
 * Go is how the side bar leaves for a page: at once beside the page, and once
 * the menu is closed on a narrow window, so that what it opens (the tour, a
 * page) is not covered or robbed of focus by the closing menu.
 */
type Go = (then?: () => void) => void;

/** Frame is the page around every page: the side bar, and the bar on top. */
function Frame({ side, top, children }: { side: (onGo: Go) => ReactNode; top: ReactNode; children: ReactNode }) {
  const [menu, setMenu] = useState(false);
  const after = useAfterClose();
  const goNow: Go = (then) => then?.();
  const goFromMenu: Go = (then) => {
    if (then) after.later(then);
    setMenu(false);
  };
  return (
    <>
      <a href="#main" onClick={(e) => { e.preventDefault(); document.getElementById("main")?.focus(); }} className="sr-only focus:not-sr-only focus:fixed focus:left-3 focus:top-3 focus:z-[100] focus:rounded-md focus:bg-card focus:px-3 focus:py-2">
        跳到主要內容
      </a>
      <div className="min-h-screen lg:grid lg:grid-cols-[256px_minmax(0,1fr)]">
        <aside className="sticky top-0 hidden h-screen flex-col bg-rail p-3 text-white lg:flex">{side(goNow)}</aside>
        <div className="min-w-0 overflow-x-clip">
          <header className="sticky top-0 z-30 flex h-16 items-center gap-3 border-b border-border bg-background/90 px-4 backdrop-blur sm:px-8">
            <button
              type="button"
              onClick={() => setMenu(true)}
              className="grid size-10 shrink-0 place-items-center rounded-lg bg-rail text-white lg:hidden"
              aria-label="打開選單"
            >
              <Menu className="size-[19px]" aria-hidden />
            </button>
            {top}
          </header>
          <main id="main" tabIndex={-1} className="mx-auto max-w-[1360px] px-4 py-7 outline-none sm:px-8">
            {children}
          </main>
        </div>
      </div>
      <Sheet open={menu} onOpenChange={setMenu}>
        <SheetContent side="left" className="w-[280px] gap-0 border-0 bg-rail p-3 text-white [&>button]:text-white" onCloseAutoFocus={after.onCloseAutoFocus}>
          <SheetTitle className="sr-only">選單</SheetTitle>
          <SheetDescription className="sr-only">工作台的頁面</SheetDescription>
          <div className="flex h-full flex-col">{side(goFromMenu)}</div>
        </SheetContent>
      </Sheet>
    </>
  );
}

function Crumbs({ items }: { items: string[] }) {
  return (
    <div className="flex min-w-0 items-center gap-1.5 text-[14px] text-muted-foreground">
      {items.map((item, i) =>
        i === items.length - 1 ? (
          <span key={i} className="truncate font-bold text-foreground">
            {item}
          </span>
        ) : (
          <span key={i} className="hidden items-center gap-1.5 sm:flex">
            <span className="truncate">{item}</span>
            <ChevronRight className="size-4 opacity-60" aria-hidden />
          </span>
        ),
      )}
    </div>
  );
}

/** RunChip says whether a run works on the project now, and on which ticket. */
function RunChip() {
  const { project, status, engine, online } = useProject();
  if (!online) {
    return (
      <span className="hidden items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-[13.5px] font-bold text-muted-foreground sm:flex" title="畫面暫時不會自己更新；工作台恢復後會自己接上">
        <WifiOff className="size-4" aria-hidden />
        重新連線中
      </span>
    );
  }
  if (status.data?.run.state !== "running") return null;
  // The tickets the engine is on; for a run of the Principal's workflow, the
  // one whose Worker ran last.
  const steps = engine.data?.going ? workingSteps(engine.data.run) : [];
  const target = engine.data?.going ? steps[0]?.issue : status.data.target.issue_id;
  return (
    <a href={href({ project: project.id, page: "run", rest: [] })} className="hidden items-center gap-2 rounded-lg px-2.5 py-1.5 text-[13.5px] font-bold text-live hover:bg-live/10 sm:flex">
      <span className="relative flex size-2" aria-hidden>
        <span className="absolute inset-0 animate-ping rounded-full bg-live opacity-60" />
        <span className="relative size-2 rounded-full bg-live" />
      </span>
      {steps.length > 1 ? `工作中，${steps.length} 張工單` : target ? `工作中，工單 #${target}` : "工作中"}
    </a>
  );
}

function PausedBar() {
  const { state, step, startTour, end } = useOnboarding();
  if (state.status !== "paused") return null;
  return (
    <div className="fixed bottom-5 left-5 z-[60] flex items-center gap-3 rounded-xl bg-rail py-2 pl-4 pr-2 text-[14px] text-white shadow-2xl ring-1 ring-white/10 lg:left-[276px]">
      <MapPin className="size-4 text-signal" aria-hidden />
      導覽停在第 {step + 1} 步
      <button type="button" onClick={() => startTour(step)} className="rounded-md bg-signal px-3 py-1.5 font-extrabold text-[#231a00]">
        繼續
      </button>
      <button type="button" onClick={end} className="rounded-md px-2.5 py-1.5 font-bold text-white/75 hover:bg-white/10">
        不看了
      </button>
    </div>
  );
}

const typing = (el: Element | null) => !!el && (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement || (el as HTMLElement).isContentEditable);

/** ProjectShell is the frame of a project's pages, with the ways to find, ask and learn. */
export function ProjectShell({ projects, children }: { projects: ProjectView[]; children: ReactNode }) {
  const { project } = useProject();
  const { state } = useOnboarding();
  const route = useRoute();
  const [palette, setPalette] = useState(false);
  const [help, setHelp] = useState(false);
  const touring = state.status === "touring";

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if (touring) return;
      if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setPalette((open) => !open);
      } else if (event.key === "?" && !event.ctrlKey && !event.metaKey && !typing(document.activeElement) && !document.querySelector("[role=dialog]")) {
        event.preventDefault();
        setHelp(true);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [touring]);

  return (
    <>
      <Frame
        side={(onGo) => (
          <>
            <Brand />
            <ProjectSwitcher current={project} projects={projects} onGo={onGo} />
            <NavLinks onGo={onGo} />
            <Foot>
              <Checklist onGo={onGo} />
            </Foot>
          </>
        )}
        top={
          <>
            <Crumbs items={[project.name, pageTitle(route.page), ...(route.page === "plans" && route.rest[0] ? [route.rest[0] === "new" ? "開新的規劃" : route.rest[0]] : [])]} />
            <span className="ml-auto" />
            <RunChip />
            <button
              type="button"
              onClick={() => setPalette(true)}
              className="hidden h-10 w-60 items-center gap-2 whitespace-nowrap rounded-lg bg-card px-3 text-[14px] text-muted-foreground ring-1 ring-inset ring-border transition hover:ring-muted-foreground md:flex"
            >
              <Search className="size-4" aria-hidden />
              <span className="truncate">搜尋，或跳到別頁</span>
              <kbd className="ml-auto shrink-0 rounded bg-muted px-1.5 font-sans text-[12px] text-ink2 ring-1 ring-border">Ctrl K</kbd>
            </button>
            <Button variant="outline" size="icon" className="size-10 md:hidden" aria-label="搜尋" onClick={() => setPalette(true)}>
              <Search aria-hidden />
            </Button>
            <Help open={help} onOpenChange={setHelp} />
          </>
        }
      >
        {children}
      </Frame>
      <Palette open={palette} onOpenChange={setPalette} />
      <PausedBar />
    </>
  );
}

/** PlainShell is the frame where no project is open: the list of projects. */
export function PlainShell({ projects, children }: { projects: ProjectView[]; children: ReactNode }) {
  return (
    <Frame
      side={(onGo) => (
        <>
          <Brand />
          <ProjectSwitcher projects={projects} onGo={onGo} />
          <div className="flex-1" />
          <Foot />
        </>
      )}
      top={<Crumbs items={["你的專案"]} />}
    >
      {children}
    </Frame>
  );
}

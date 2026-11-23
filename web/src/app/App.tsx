import { useEffect, useState } from "react";
import { KeyRound, RefreshCw, Route as RouteIcon, Unplug } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Problem } from "@/components/bits";
import { ProjectProvider } from "@/app/project";
import { PlainShell, ProjectShell } from "@/app/Shell";
import { TicketSheetProvider } from "@/app/ticket-sheet";
import { OnboardingProvider } from "@/onboarding/state";
import { Tour } from "@/onboarding/Tour";
import { Welcome } from "@/onboarding/Welcome";
import { ActivityPage } from "@/pages/Activity";
import { LessonsPage } from "@/pages/lessons/LessonsPage";
import { Board } from "@/pages/Board";
import { HealthPage } from "@/pages/Health";
import { InboxPage } from "@/pages/Inbox";
import { Overview } from "@/pages/Overview";
import { NewPlan } from "@/pages/plans/NewPlan";
import { PlanPage } from "@/pages/plans/PlanPage";
import { PlansList } from "@/pages/plans/PlansList";
import { ProjectsPage } from "@/pages/Projects";
import { RunPage } from "@/pages/Run";
import { api, ApiError, LOCKED_EVENT, type Session } from "@/lib/api";
import { href, redirect, useRoute, type Route } from "@/lib/router";
import { useResource } from "@/lib/resource";

function Centered({ children }: { children: React.ReactNode }) {
  return <div className="grid min-h-screen place-items-center px-4 py-10">{children}</div>;
}

function Splash() {
  return (
    <Centered>
      <div className="flex items-center gap-3 text-muted-foreground" role="status">
        <span className="grid size-9 animate-pulse place-items-center rounded-lg bg-signal text-[#231a00]">
          <RouteIcon className="size-[19px]" aria-hidden />
        </span>
        打開工作台…
      </div>
    </Centered>
  );
}

/** Blocked says why the workbench cannot be shown, and what to do about it. */
function Blocked({ error, retry }: { error?: Error; retry: () => void }) {
  const locked = error instanceof ApiError && (error.status === 401 || error.status === 403);
  const Icon = locked ? KeyRound : Unplug;
  return (
    <Centered>
      <div className="w-full max-w-[520px] rounded-xl border border-border bg-card p-7">
        <span className="grid size-11 place-items-center rounded-lg bg-muted text-ink2 ring-1 ring-inset ring-border">
          <Icon className="size-5" aria-hidden />
        </span>
        <h1 className="mt-4 text-[22px] font-black">{locked ? "要用 awkit ui 給的網址打開" : "連不上工作台"}</h1>
        <p className="mt-2 text-[15px] leading-relaxed text-ink2">
          {locked
            ? "工作台只讓這台電腦上、拿到鑰匙的瀏覽器進來。每次執行 awkit ui，終端機會印出一個帶著新鑰匙的網址；用那個網址打開，或在終端機再執行一次 awkit ui。"
            : "awkit ui 還開著嗎？終端機關掉，工作台也跟著關掉。在專案資料夾裡再執行一次 awkit ui，用它印出的網址打開。"}
        </p>
        {!locked && (
          <Button className="mt-5" variant="outline" onClick={retry}>
            <RefreshCw aria-hidden /> 再試一次
          </Button>
        )}
        {error && !locked && <p className="mt-4 text-[13px] text-muted-foreground">{error.message}</p>}
      </div>
    </Centered>
  );
}

function ProjectPage({ route }: { route: Route }) {
  switch (route.page) {
    case "overview":
      return <Overview />;
    case "inbox":
      return <InboxPage />;
    case "board": {
      const n = Number(route.rest[0]);
      return <Board openTicket={Number.isInteger(n) && n > 0 ? n : undefined} />;
    }
    case "plans": {
      const [plan] = route.rest;
      if (plan === "new") return <NewPlan />;
      return plan ? <PlanPage key={plan} planId={plan} /> : <PlansList />;
    }
    case "run": {
      const n = Number(route.rest[0]);
      return <RunPage preset={Number.isInteger(n) && n > 0 ? n : undefined} />;
    }
    case "activity":
      return <ActivityPage />;
    case "lessons":
      return <LessonsPage />;
    case "health":
      return <HealthPage />;
    default:
      return null;
  }
}

const KNOWN = ["overview", "inbox", "plans", "board", "run", "activity", "lessons", "health"];

function Routes({ session, reload }: { session: Session; reload: () => void }) {
  const route = useRoute();
  const first = session.projects.find((p) => !p.problem);
  const project = route.project ? session.projects.find((p) => p.id === route.project) : undefined;

  // An address that names no page goes to the first project, or to the list
  // of projects when there is none to open.
  useEffect(() => {
    if (!route.project && route.page !== "projects") {
      redirect(first ? href({ project: first.id, page: "overview", rest: [] }) : "#/projects");
    } else if (project && !project.problem && !KNOWN.includes(route.page)) {
      redirect(href({ project: project.id, page: "overview", rest: [] }));
    }
  }, [route, first, project]);

  if (project && !project.problem) {
    return (
      <ProjectProvider key={project.id} project={project}>
        <TicketSheetProvider>
          <ProjectShell projects={session.projects}>
            <ProjectPage route={route} />
          </ProjectShell>
          <Welcome />
          <Tour />
        </TicketSheetProvider>
      </ProjectProvider>
    );
  }

  return (
    <PlainShell projects={session.projects}>
      {route.project && (
        <div className="mb-6">
          <Problem title={project ? `${project.name} 打不開` : "工作台裡沒有這個專案"}>
            {project?.problem ?? "它可能被移出了工作台。從下面的清單選一個，或把它加回來。"}
          </Problem>
        </div>
      )}
      <ProjectsPage session={session} onChanged={reload} />
    </PlainShell>
  );
}

/** App asks the workbench who is here and what they opened, then shows it. */
export function App() {
  const session = useResource(() => api.session(), []);
  const [locked, setLocked] = useState(false);
  useEffect(() => {
    const lock = () => setLocked(true);
    window.addEventListener(LOCKED_EVENT, lock);
    return () => window.removeEventListener(LOCKED_EVENT, lock);
  }, []);
  if (locked) return <Blocked error={new ApiError("locked", 401)} retry={session.reload} />;
  if (!session.data) {
    if (session.loading) return <Splash />;
    return <Blocked error={session.error} retry={session.reload} />;
  }
  return (
    <OnboardingProvider initial={session.data.onboarding}>
      <Routes session={session.data} reload={session.reload} />
    </OnboardingProvider>
  );
}

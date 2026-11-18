import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from "react";
import { api, type EventsView, type PlanView, type ProjectView, type StatusReport, type TicketsView } from "@/lib/api";
import { useResource, useStream, type Area, type Resource } from "@/lib/resource";
import { council, type RosterView } from "@/lib/council";

export interface ProjectData {
  project: ProjectView;
  status: Resource<StatusReport>;
  tickets: Resource<TicketsView>;
  events: Resource<EventsView>;
  plans: Resource<{ plans: PlanView[] }>;
  /** Who sits at the council's table. */
  roster: Resource<RosterView>;
  /** Whether the page hears the project's changes as they happen. */
  online: boolean;
  /** Ask GitHub again now, rather than when the server's answer gets old. */
  refreshTickets: () => void;
  /** Counts the times a meeting of the project moved: a page that shows one asks again when it changes. */
  meetingMoved: number;
}

const Context = createContext<ProjectData | null>(null);

/** ProjectProvider reads a project, and reads again what its stream says changed. */
export function ProjectProvider({ project, children }: { project: ProjectView; children: ReactNode }) {
  const fresh = useRef(false);
  const status = useResource(() => api.status(project.id), [project.id]);
  const tickets = useResource(() => {
    const askGitHub = fresh.current;
    fresh.current = false;
    return api.tickets(project.id, askGitHub);
  }, [project.id]);
  const events = useResource(() => api.events(project.id), [project.id]);
  const plans = useResource(() => api.plans(project.id), [project.id]);
  const [meetingMoved, setMeetingMoved] = useState(0);
  const roster = useResource(() => council.roster(project.id), [project.id]);

  const online = useStream(api.streamURL(project.id), (areas: Area[]) => {
    if (areas.includes("status")) status.reload();
    if (areas.includes("events")) events.reload();
    if (areas.includes("plans") || areas.includes("meeting")) plans.reload();
    if (areas.includes("meeting") || areas.includes("plans")) {
      setMeetingMoved((n) => n + 1);
      // A project's first plan writes its roster.
      if (roster.data?.missing) roster.reload();
    }
    // What a run does to a ticket shows in its state and events first; the
    // server asks GitHub again once its answer is old.
    if (areas.includes("tickets") || areas.includes("status") || areas.includes("events")) tickets.reload();
  });

  const reloadTickets = tickets.reload;
  const refreshTickets = useCallback(() => {
    fresh.current = true;
    reloadTickets();
  }, [reloadTickets]);

  const value = useMemo<ProjectData>(
    () => ({ project, status, tickets, events, plans, roster, online, refreshTickets, meetingMoved }),
    [project, status, tickets, events, plans, roster, online, refreshTickets, meetingMoved],
  );
  return <Context.Provider value={value}>{children}</Context.Provider>;
}

export function useProject(): ProjectData {
  const value = useContext(Context);
  if (!value) throw new Error("useProject outside a ProjectProvider");
  return value;
}

import { useCallback, useEffect, useRef, useState } from "react";
import { useProject } from "@/app/project";
import { council, type Entry, type PlanDetail } from "@/lib/council";
import { useResource, type Resource } from "@/lib/resource";

export interface PlanData {
  detail: Resource<PlanDetail>;
  /** What was said, in order; a step said again keeps its last entry. */
  entries: Entry[];
  /** Ask again now, after the page changed something. */
  refresh: () => void;
}

/** merge puts newer entries over older ones of the same step, in order. */
export function merge(known: Entry[], more: Entry[]): Entry[] {
  if (more.length === 0) return known;
  const steps = new Set(more.map((e) => e.step).filter(Boolean));
  return [...known.filter((e) => !e.step || !steps.has(e.step)), ...more].sort((a, b) => a.seq - b.seq);
}

/**
 * usePlan follows a plan: its detail, and what was said at its meeting. It
 * asks again whenever the project's stream says a meeting moved, and every
 * few seconds while a meeting goes on, in case the stream is down.
 */
export function usePlan(planId: string): PlanData {
  const { project, meetingMoved } = useProject();
  const [tick, setTick] = useState(0);
  const detail = useResource(() => council.plan(project.id, planId), [project.id, planId, meetingMoved, tick]);
  const [entries, setEntries] = useState<Entry[]>([]);
  const last = useRef(0);

  useEffect(() => {
    setEntries([]);
    last.current = 0;
  }, [project.id, planId]);

  useEffect(() => {
    let alive = true;
    council
      .transcript(project.id, planId, last.current)
      .then(({ entries: more }) => {
        if (!alive || more.length === 0) return;
        last.current = Math.max(last.current, ...more.map((e) => e.seq));
        setEntries((known) => merge(known, more));
      })
      .catch(() => {
        // Asked again with the next change.
      });
    return () => {
      alive = false;
    };
  }, [project.id, planId, meetingMoved, tick]);

  // Asked again while the meeting goes on, or is starting and has said
  // nothing yet; a plan held before meetings said where they are is not.
  const d = detail.data;
  const going = d?.meeting?.state === "meeting" || (!!d && !d.meeting && !d.record && d.tickets.length === 0);
  useEffect(() => {
    if (!going) return;
    const timer = window.setInterval(() => setTick((t) => t + 1), 8000);
    return () => window.clearInterval(timer);
  }, [going]);

  const refresh = useCallback(() => setTick((t) => t + 1), []);
  return { detail, entries, refresh };
}

/**
 * useOtherMeeting is the plan of the project whose meeting is held now, when
 * it is not this one: a project holds one meeting at a time, so this one
 * cannot start or go on until that one stops.
 */
export function useOtherMeeting(planId: string) {
  const { plans } = useProject();
  return (plans.data?.plans ?? []).find((p) => p.id !== planId && p.meeting?.state === "meeting");
}

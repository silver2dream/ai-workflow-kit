// The filters of the activity page: who did it, or what needs a look.
import type { Dispatcher, TraceEvent } from "./api";
import { dispatches } from "./words";

export interface Filter {
  key: string;
  label: string;
  /** keep says whether a filter shows an event, in a run the dispatcher dispatched. */
  keep: (e: TraceEvent, dispatcher?: Dispatcher) => boolean;
}

const role = (e: TraceEvent) => (typeof e.data?.role === "string" ? e.data.role : "");

export const FILTERS: Filter[] = [
  { key: "all", label: "全部", keep: () => true },
  // The engine's own, and the dispatching it did with the worker's functions.
  { key: "engine", label: "engine", keep: (e, by) => e.component === "engine" || (by === "engine" && dispatches(e)) },
  { key: "principal", label: "主控", keep: (e, by) => by !== "engine" && dispatches(e) },
  { key: "worker", label: "執行者", keep: (e) => e.component === "worker" || (e.component === "agent" && role(e) === "implementer") },
  { key: "reviewer", label: "審查", keep: (e) => e.component === "reviewer" || (e.component === "agent" && (role(e) === "reviewer" || role(e) === "resolver")) },
  { key: "guard", label: "防護", keep: (e) => e.component === "guard" },
  { key: "attention", label: "只看要注意的", keep: (e) => e.level === "error" || e.level === "warn" },
];

/** shownFilters are the filters of a run's events: the Principal's only in a run it dispatched. */
export function shownFilters(events: TraceEvent[], dispatcher?: Dispatcher): Filter[] {
  const principal = FILTERS.find((f) => f.key === "principal")!;
  const ran = events.some((e) => principal.keep(e, dispatcher));
  return FILTERS.filter((f) => f !== principal || ran);
}

// What the workbench's server answers, as internal/workbench writes it.
import type { ApprovalState, MeetingState } from "@/lib/council";

export type OnboardingStatus = "new" | "touring" | "paused" | "ended" | "done" | "skipped";

export interface Onboarding {
  status: OnboardingStatus;
  step: number;
  done: string[];
  list_hidden: boolean;
}

export interface ProjectView {
  id: string;
  path: string;
  name: string;
  problem?: string;
}

export interface Session {
  version: string;
  settings_path: string;
  projects: ProjectView[];
  onboarding: Onboarding;
}

/** The run's state: whether a run holds the project's lock, and whether it is alive. */
export type RunState = "running" | "no_lock" | "stale_lock" | "lock_corrupt" | "unknown";

export interface StatusReport {
  timestamp_utc: string;
  root: string;
  run: {
    state: RunState;
    pid?: number;
    process_alive?: boolean;
    start_time?: string;
    session: {
      session_id?: string;
      started_at?: string;
      ended_at?: string;
      exit_reason?: string;
      actions_count?: number;
    };
  };
  control: { stop_present: boolean; loop_count?: number; consecutive_failures?: number };
  target: { issue_id?: number; source?: string };
  suggestions?: string[];
  warnings?: string[];
}

export type Stage = "needs_you" | "in_review" | "in_progress" | "rework" | "ready" | "waiting";

export interface Ticket {
  number: number;
  title: string;
  url?: string;
  stage: Stage;
  why?: string;
  waits_for?: number[];
}

export interface TicketsView {
  /** done_titles are the titles of the tickets that are done, by number. */
  tickets?: { open: Ticket[]; done: number; done_titles?: Record<string, string> };
  stages: { stage: Stage; title: string }[];
  error?: string;
  asked_at: string;
}

export interface TraceEvent {
  seq: number;
  ts: string;
  component: string;
  type: string;
  level: string;
  issue_id?: number;
  pr_number?: number;
  data?: Record<string, unknown>;
}

/** Who dispatched a run: what the worker's functions write is under the Principal's name whichever calls them. */
export type Dispatcher = "engine" | "principal";

export interface EventsView {
  session?: string;
  dispatcher?: Dispatcher;
  events: TraceEvent[];
}

export interface Check {
  name: string;
  status: "ok" | "warning" | "error" | string;
  message: string;
  /** awkit reset clears what it found. */
  can_clean?: boolean;
}

/** awkit doctor, both halves. */
export interface Health {
  /** What a run needs: the tools, the configuration, the remote. */
  setup: Check[];
  /** What earlier runs left behind. */
  state: Check[];
}

export interface Milestone {
  key: string;
  title: string;
  goal: string;
  done_when: string[];
}

export interface PlanView {
  id: string;
  brief: string;
  milestones: Milestone[];
  tickets: { key: string; title: string; repo: string; issue?: number; depends_on?: string[] }[];
  approved: number;
  /** Where its meeting is; none for a plan held before meetings said so. */
  meeting?: MeetingState;
  /** Where its approval from the workbench is. */
  approval?: ApprovalState;
}

/** A log of a run: the run's own (started from the page), the Principal's, or a ticket's Worker's. */
export interface RunLog {
  name: string;
  issue?: number;
  size: number;
  updated: string;
}

/** A piece of a log, from where the page read last. */
export interface RunOutput {
  exists: boolean;
  text: string;
  next: number;
  /** The text does not follow what was read before: the log started over, or only its end is given. */
  fresh: boolean;
}

/** A decision of the engine, or of the Principal's workflow beside it, as internal/analyzer writes it. */
export interface Decision {
  next_action: string;
  issue_number: number;
  pr_number: number;
  exit_reason: string;
  merge_issue: string;
}

/** What the engine does in a step: the decision it acts on, and since when. */
export interface EngineStep {
  action: string;
  issue?: number;
  pr?: number;
  /** conflict or rebase: the step resolves the pull request. */
  merge?: string;
  since: string;
}

/** The engine's run file: its process, its beat, the steps that run (lanes run side by side), and how it ended. */
export interface EngineRun {
  pid: number;
  host: string;
  started: string;
  beat: string;
  only?: number;
  /** The steps that run, the oldest first. */
  steps?: EngineStep[];
  ended?: string;
  reason?: string;
}

export type LaneState =
  | "blocked"
  | "ready"
  | "implementing"
  | "checking"
  | "reviewing"
  | "review_retry"
  | "rework"
  | "awaiting_approval"
  | "queued"
  | "resolving"
  | "merged"
  | "completed"
  | "needs_you"
  | "paused"
  | "cancelled";

/** A guardrail's verdict on what was read or written for a ticket. */
export interface LaneVerdict {
  result: "block" | "flag" | string;
  class: string;
  stage: string;
  rule: string;
  piece?: string;
  source?: string;
  reason: string;
  at: string;
}

/** A ticket's way to the integration branch, as the engine keeps it. */
export interface Lane {
  issue: number;
  state: LaneState;
  pr?: number;
  /** conflict or rebase, while it is resolved. */
  resolve?: string;
  why?: string;
  note?: string;
  attempts: { failures?: number; reviews?: number; conflicts?: number; checks?: number };
  since: string;
  /** Its place in the merge queue, 1 the next to be merged; none when it is not in it. */
  queue?: number;
  /** What the engine does on it now. */
  step?: EngineStep;
  /** The latest verdict on its ticket in the last run that was not a pass. */
  verdict?: LaneVerdict;
}

export interface ShadowDifference {
  at: string;
  comparison: { engine: Decision | null; analyzer: Decision | null; same: boolean; known?: string; error?: string };
}

/** How the last run's decisions compared with the Principal's workflow's (AWKIT_ENGINE=shadow). */
export interface ShadowRun {
  started: string;
  steps: number;
  same: number;
  known: number;
  failed: number;
  differences: ShadowDifference[];
}

export interface EngineReply {
  id: string;
  kind: string;
  result: "applied" | "rejected" | string;
  reason?: string;
  at: string;
}

/** The engine's state, as `awkit status` reads it too. */
export interface EngineView {
  run?: EngineRun;
  going: boolean;
  /** Every lane not over, and those that ended in the last run, in the order they are shown. */
  lanes: Lane[];
  /** The lanes that ended before the last run: not listed. */
  earlier: number;
  limits: { failures: number; reviews: number; conflicts: number; checks?: number };
  shadow?: ShadowRun;
  pending: { id: string; kind: string; by: string; at: string }[];
  replies: EngineReply[];
  problems?: string[];
}

/** What became of a stop: given to the engine, and answered; or the run's process ended from the workbench. */
export interface StopView {
  engine: boolean;
  answered: boolean;
  reply?: EngineReply;
  ended: boolean;
}

/** An answer the server gave with an error: what it said, its status, and the code it tells the error apart by, when it gave one. */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code?: string,
  ) {
    super(message);
  }
}

/** LOCKED_EVENT is sent on the window when the workbench no longer knows this browser. */
export const LOCKED_EVENT = "awk:locked";

export async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (method !== "GET") {
    // The server takes a change only with this header: a page of another
    // site cannot send it.
    headers["X-AWK-Workbench"] = "1";
  }
  if (body !== undefined) headers["Content-Type"] = "application/json";
  const response = await fetch(path, {
    method,
    headers,
    credentials: "same-origin",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (response.status === 204) return undefined as T;
  let data: unknown = null;
  try {
    data = await response.json();
  } catch {
    // An answer that is not JSON is said by its status below.
  }
  if (!response.ok) {
    if (response.status === 401) {
      // The workbench was started again, and its new key is not this
      // browser's: only the address it printed lets the browser in. The
      // whole page says so, not only the part that asked.
      window.dispatchEvent(new Event(LOCKED_EVENT));
      throw new ApiError("工作台不認得這個瀏覽器了：請用終端機裡 awkit ui 最後印出的網址重新打開。", 401);
    }
    const said = data && typeof data === "object" && "error" in data ? String((data as { error: unknown }).error) : "";
    const code = data && typeof data === "object" && "code" in data ? String((data as { code: unknown }).code) : undefined;
    throw new ApiError(said || `工作台回答 ${response.status}`, response.status, code);
  }
  return data as T;
}

const project = (id: string) => `/api/projects/${encodeURIComponent(id)}`;

export const api = {
  session: () => request<Session>("GET", "/api/session"),
  saveOnboarding: (onboarding: Onboarding) => request<Onboarding>("PUT", "/api/onboarding", onboarding),
  addProject: (path: string) => request<ProjectView>("POST", "/api/projects", { path }),
  removeProject: (id: string) => request<void>("DELETE", project(id)),
  status: (id: string) => request<StatusReport>("GET", `${project(id)}/status`),
  tickets: (id: string, fresh = false) => request<TicketsView>("GET", `${project(id)}/tickets${fresh ? "?fresh=1" : ""}`),
  events: (id: string, last = 300) => request<EventsView>("GET", `${project(id)}/events?last=${last}`),
  health: (id: string) => request<Health>("GET", `${project(id)}/health`),
  plans: (id: string) => request<{ plans: PlanView[] }>("GET", `${project(id)}/plans`),
  startRun: (id: string, issue?: number) => request<void>("POST", `${project(id)}/run`, { issue: issue ?? 0 }),
  stopRun: (id: string, now: boolean) => request<StopView>("POST", `${project(id)}/run/stop`, { now }),
  engine: (id: string) => request<EngineView>("GET", `${project(id)}/engine`),
  runLogs: (id: string) => request<{ logs: RunLog[] }>("GET", `${project(id)}/run/logs`),
  runOutput: (id: string, log: string, since: number) => request<RunOutput>("GET", `${project(id)}/run/output?log=${encodeURIComponent(log)}&since=${since}`),
  lessons: (id: string) => request<LessonsView>("GET", `${project(id)}/lessons`),
  decideLesson: (id: string, lesson: string, decision: LessonDecision) =>
    request<LessonView>("POST", `${project(id)}/lessons/${encodeURIComponent(lesson)}/${decision}`),
  streamURL: (id: string) => `${project(id)}/stream`,
};

export type LessonStatus = "quarantined" | "candidate" | "active" | "proven" | "retired";
export type LessonDecision = "approve" | "reject";

/** A lesson the reviews taught, as internal/workbench writes it. */
export interface LessonView {
  id: string;
  title: string;
  description?: string;
  /** The text the AI reads, with characters a person cannot see shown as <U+200B>. */
  content: string;
  status: LessonStatus;
  hits: number;
  misses: number;
  categories: string[];
  scope: string[];
  created_at: string;
  last_hit_at?: string;
  /** distiller, human-correction or human. */
  source: string;
  /** Why the screening held it back; kept after a person turned it down. */
  held_back?: string;
  /** The text has characters a person cannot see. */
  hidden?: boolean;
  evidence: { issue: number; pr: number }[];
}

export interface LessonsView {
  lessons: LessonView[];
}

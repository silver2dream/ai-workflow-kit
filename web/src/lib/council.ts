// The council's plans, as internal/workbench and internal/council write them.
import { request } from "@/lib/api";

/** Where a meeting is: the council's states, or stopped when its process is gone. */
export type MeetingStateName = "meeting" | "agenda" | "questions" | "paused" | "finished" | "failed" | "stopped";

export interface MeetingState {
  state: MeetingStateName;
  note?: string;
  topic: number;
  topics: number;
  step?: string;
  speaker?: string;
  calls: number;
  limit: number;
  updated: string;
  /** How many questions wait for the owner's answers. */
  questions: number;
}

export interface Topic {
  id: string;
  title: string;
  question: string;
  lead: string;
  speakers: string[];
  reads: string[] | null;
  writes: string[];
}

export interface Agenda {
  plan: string;
  questions: string[] | null;
  topics: Topic[];
}

export interface Objection {
  from?: string;
  to: string;
  point: string;
  why?: string;
  wants?: string;
}

export interface Speech {
  role?: string;
  says: string;
  proposals: string[] | null;
  concerns: string[] | null;
  objections?: Objection[] | null;
}

export interface Exchange {
  stance: "concede" | "revise" | "hold" | string;
  reply: string;
  position?: string;
  answered: boolean;
  accepts: boolean;
  answer?: string;
  still_wants?: string;
}

export interface Summary {
  consensus: string[] | null;
  open: string[] | null;
  questions: string[] | null;
}

export interface Decision {
  id?: string;
  title: string;
  decision: string;
  rationale: string;
  dissent?: string;
  replaces?: string;
}

export interface Assumption {
  id?: string;
  question: string;
  assumption: string;
  if_wrong?: string;
}

export interface WrittenDocument {
  path: string;
  owner: string;
  unchanged?: boolean;
  refused?: string;
}

export interface RuleFinding {
  owner?: string;
  document: string;
  rule: string;
  point: string;
  fix: string;
}

export interface RuleCheck {
  owner: string;
  found: RuleFinding[] | null;
  settled: boolean;
  open: RuleFinding[] | null;
}

export interface OwnerWord {
  id: string;
  at: string;
  text: string;
  question?: string;
}

export interface Review {
  words: OwnerWord[];
  at: string;
  note: string;
  decisions: Decision[] | null;
  withdrawn: { id: string; why: string }[] | null;
  answered: { id: string; answer: string }[] | null;
  documents: WrittenDocument[] | null;
  rule_checks: RuleCheck[] | null;
}

export interface Milestone {
  key: string;
  title: string;
  goal: string;
  done_when: string[];
}

export interface Ticket {
  key: string;
  title: string;
  repo: string;
  summary: string;
  scope: string[] | null;
  acceptance: string[] | null;
  testing: string[] | null;
  reads?: string[] | null;
  depends_on?: string[] | null;
  /** What the ticket's work may change: globs from the project's root. */
  paths?: string[] | null;
  /** low, medium or high; empty on a ticket laid out before tickets had one. */
  risk?: string;
  issue?: number;
}

export type Risk = "low" | "medium" | "high";

/** What each risk of a ticket means, in a few words. */
export const RISKS: Record<Risk, { label: string; tone: "muted" | "caution"; means: string }> = {
  low: { label: "低風險", tone: "muted", means: "只動到文件或測試" },
  medium: { label: "中風險", tone: "muted", means: "一般的功能修改" },
  high: { label: "高風險", tone: "caution", means: "動到資料庫、認證、CI、建置、相依套件，或刪除、發版" },
};

/** The risk of a ticket, or null when it has none (or one this page does not know). */
export function riskOf(risk?: string | null) {
  return risk && Object.hasOwn(RISKS, risk) ? RISKS[risk as Risk] : null;
}

export interface Dispute {
  topic: string;
  challenger: string;
  defender: string;
  point: string;
  wants: string;
  exchanges: Exchange[];
  settled: boolean;
  outcome: string;
}

export interface TopicView {
  number: number;
  topic: Topic;
  started: boolean;
  held: boolean;
  proposal?: Speech;
  speeches: Speech[];
  disputes: Dispute[];
  summary: Summary;
  decisions: Decision[];
  assumptions: Assumption[];
  documents: WrittenDocument[];
  rule_checks: RuleCheck[];
}

export interface DecisionState extends Decision {
  topic?: string;
  replaced_by?: string;
  withdrawn?: string;
}

export interface AssumptionState extends Assumption {
  topic?: string;
  answer?: string;
}

export interface PlanRecord {
  id: string;
  brief: string;
  status: MeetingStateName;
  stopped?: string;
  agenda?: Agenda;
  answers: { question: string; answer: string }[];
  words: OwnerWord[];
  topics: TopicView[];
  revisions: Review[];
  decisions: DecisionState[];
  assumptions: AssumptionState[];
  documents: WrittenDocument[];
  findings: RuleFinding[];
  data_open: string[];
  pages: string[];
  milestones: Milestone[];
  tickets: Ticket[];
  /** What the table still holds against a ticket of the list. */
  list_objections: ListObjection[];
  /** What the list leaves for a later milestone. */
  later: string[];
  calls: number;
  updated: string;
}

export interface ListObjection {
  from?: string;
  ticket: string;
  point: string;
  wants: string;
}

export interface PlanDetail {
  id: string;
  brief: string;
  meeting?: MeetingState;
  record?: PlanRecord;
  agenda?: Agenda;
  questions: string[];
  words: OwnerWord[];
  milestones: Milestone[];
  tickets: Ticket[];
  list_problem?: string;
  log?: string[];
  approval?: ApprovalRecord;
}

/** The kinds of what was said, as the meeting's transcript names them. */
export type EntryKind =
  | "agenda"
  | "questions"
  | "answers"
  | "proposal"
  | "speech"
  | "exchange"
  | "summary"
  | "ruling"
  | "documents"
  | "word"
  | "review"
  | "worklist"
  | "stopped";

export interface Entry {
  seq: number;
  at: string;
  step: string;
  topic?: string;
  kind: EntryKind;
  role?: string;
  // What was said, in the form of its kind.
  data?: unknown;
}

export interface ExchangeEntry {
  challenger: string;
  defender: string;
  point: string;
  round: number;
  exchange: Exchange;
}

export interface RoleView {
  id: string;
  title: string;
  stance: string;
  owns: string[];
  decides?: boolean;
  plans?: boolean;
  model?: string;
  effort?: string;
}

export interface RosterView {
  missing?: boolean;
  roles: RoleView[];
  topics: number;
  calls: number;
}

export interface TicketText {
  title: string;
  summary: string;
  scope: string[];
  acceptance: string[];
  testing: string[];
  paths: string[];
  risk: string;
}

/** Where an approval started from the page is: `awkit plan approve --merge-docs` writes it as it goes. */
export type ApprovalStep = "check" | "issues" | "documents" | "checks" | "label" | "sync" | "done";
export type ApprovalStateName = "working" | "done" | "failed" | "stopped";

export interface ApprovalRecord {
  pid: number;
  state: ApprovalStateName;
  step: ApprovalStep;
  /** How it stopped: run, branch, checks, slow, refused, stopped, error. */
  why?: string;
  error?: string;
  created: number;
  issues: number[];
  pr?: number;
  url?: string;
  /** Why the working tree was not brought up to the integration branch. */
  left?: string;
  lines: string[];
  started: string;
  updated: string;
}

/** An approval in short, as the list of plans has it. */
export interface ApprovalState {
  state: ApprovalStateName;
  step: ApprovalStep;
  why?: string;
  left?: string;
}

const project = (id: string) => `/api/projects/${encodeURIComponent(id)}`;
const plan = (id: string, planId: string) => `${project(id)}/plans/${encodeURIComponent(planId)}`;

export const council = {
  roster: (id: string) => request<RosterView>("GET", `${project(id)}/roster`),
  start: (id: string, brief: string, reviewAgenda: boolean, ask: boolean) =>
    request<{ id: string }>("POST", `${project(id)}/plans`, { brief, review_agenda: reviewAgenda, ask }),
  plan: (id: string, planId: string) => request<PlanDetail>("GET", plan(id, planId)),
  transcript: (id: string, planId: string, since = 0) => request<{ entries: Entry[] }>("GET", `${plan(id, planId)}/transcript?since=${since}`),
  say: (id: string, planId: string, text: string, question?: string) => request<OwnerWord>("POST", `${plan(id, planId)}/say`, { text, question: question ?? "" }),
  pause: (id: string, planId: string) => request<void>("POST", `${plan(id, planId)}/pause`),
  goOn: (id: string, planId: string) => request<void>("POST", `${plan(id, planId)}/continue`),
  answer: (id: string, planId: string, answers: { question: string; answer: string }[]) => request<void>("POST", `${plan(id, planId)}/answers`, { answers }),
  saveAgenda: (id: string, planId: string, questions: string[], topics: Topic[]) => request<Agenda>("PUT", `${plan(id, planId)}/agenda`, { questions, topics }),
  editTicket: (id: string, planId: string, key: string, text: TicketText) =>
    request<{ tickets: Ticket[] }>("PUT", `${plan(id, planId)}/tickets/${encodeURIComponent(key)}`, text),
  dropTicket: (id: string, planId: string, key: string) => request<{ tickets: Ticket[] }>("DELETE", `${plan(id, planId)}/tickets/${encodeURIComponent(key)}`),
  approve: (id: string, planId: string) => request<void>("POST", `${plan(id, planId)}/approve`),
};

/** What a meeting's state means for its owner, in a few words. */
export const MEETING: Record<MeetingStateName, { label: string; tone: "live" | "signal" | "muted" | "go" | "stop" | "caution"; means: string }> = {
  meeting: { label: "開會中", tone: "live", means: "議會正在討論。你隨時可以說話，下一個發言的角色就會聽到。" },
  agenda: { label: "等你看議程", tone: "signal", means: "主持人排好了議題，等你看過、改好，再開始。" },
  questions: { label: "等你回答", tone: "signal", means: "議會有只有你答得出來的問題，答完才開始討論。" },
  paused: { label: "暫停", tone: "muted", means: "照你的意思停在這一步。接著開會從這裡繼續，說過的不會重來。" },
  finished: { label: "開完了", tone: "go", means: "議會擬好了工單，等你看過、核可。" },
  failed: { label: "停住了", tone: "stop", means: "會議遇到錯誤停下來，常見的是 Claude 暫時沒回應或用量到了上限。等一下再按「接著開會」，從停下的地方繼續，說過的不會重來。" },
  stopped: { label: "中斷了", tone: "caution", means: "開會的程序不在了（被關掉，或電腦重開）。接著開會從停的地方繼續。" },
};

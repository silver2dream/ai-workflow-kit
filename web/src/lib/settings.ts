import { request } from "@/lib/api";

/** The council's roster as the file has it: an empty value is the default, and is not written down. */
export interface Roster {
  language: string;
  model: string;
  effort: string;
  limits: Limits;
  roles: Role[];
}

/** The limits of one plan; null is the default. */
export interface Limits {
  topics: number | null;
  disputes: number | null;
  dispute_rounds: number | null;
  calls: number | null;
  parallel: number | null;
  timeout_seconds: number | null;
}

export interface Role {
  id: string;
  title: string;
  model: string;
  effort: string;
  stance: string;
  owns: string[];
  decides: boolean;
  plans: boolean;
  /** The id the role has in the file, for one whose id was changed: it keeps its place and its comments. */
  from?: string;
}

/** The part of the configuration a person changes here. The repos and branches are shown, not changed. */
export interface Config {
  name: string;
  description: string;
  integration_branch: string;
  release_branch: string;
  repos: RepoCommands[];
  worker: { backend: string; model: string; effort: string };
  /** The engine's reviewer and resolver: what is empty or null is the role's default, which follows the Worker. */
  agents: { reviewer: AgentRole; resolver: AgentRole };
  review: { score_threshold: number | null; merge_strategy: string };
  retries: { review_attempts: number | null; worker: number | null };
  verify_minutes: number | null;
  /** How much the engine does at once: null is one step at a time; a backend left out has no limit of its own. */
  engine: { max_parallel: number | null; max_per_backend: Record<string, number> };
}

export type AgentRoleName = "reviewer" | "resolver";

/** One of the engine's agents as the file sets it. */
export interface AgentRole {
  backend: string;
  model: string;
  effort: string;
  minutes: number | null;
  usd: number | null;
}

export interface RepoCommands {
  name: string;
  path: string;
  language: string;
  setup: string;
  build: string;
  test: string;
}

export interface Defaults {
  roster: { model: string; limits: Limits; efforts: string[] };
  config: {
    backend: string;
    model: string;
    score_threshold: number;
    merge_strategy: string;
    review_attempts: number;
    worker_retries: number;
    verify_minutes: number;
    max_parallel: number;
    backends: string[];
    strategies: string[];
    efforts: string[];
    /** What the agents' settings may be: their default and largest minutes by role, and codex's efforts (efforts are Claude Code's). */
    agents: { minutes: Record<AgentRoleName, number>; max_minutes: Record<AgentRoleName, number>; codex_efforts: string[] };
  };
}

export type ApplyStep = "check" | "change" | "checks" | "sync" | "done";
export type ApplyState = "working" | "done" | "failed" | "stopped";

/** Where a change of the settings is: `awkit settings apply` writes it as it goes. */
export interface ApplyRecord {
  pid: number;
  state: ApplyState;
  step: ApplyStep;
  what: ("roster" | "config")[];
  /** How it stopped: invalid, conflict, run, branch, checks, noci, slow, refused, stopped, error. */
  why?: string;
  error?: string;
  pr?: number;
  url?: string;
  unchanged?: boolean;
  /** Why the working tree was not brought up to the integration branch. */
  left?: string;
  lines: string[];
  started: string;
  updated: string;
}

/** What keeps the settings from being changed now. */
export interface Busy {
  kind: "run" | "meeting" | "approval" | "settings";
  plan?: string;
}

export interface SettingsView {
  version: string;
  roster: Roster | null;
  roster_missing?: boolean;
  roster_problem?: string;
  config: Config | null;
  config_problem?: string;
  defaults: Defaults;
  origin: boolean;
  busy: Busy | null;
  apply: ApplyRecord | null;
}

const project = (id: string) => `/api/projects/${encodeURIComponent(id)}`;

export const settings = {
  get: (id: string) => request<SettingsView>("GET", `${project(id)}/settings`),
  save: (id: string, version: string, roster?: Roster, config?: Config) => request<void>("PUT", `${project(id)}/settings`, { version, roster, config }),
  retry: (id: string) => request<void>("POST", `${project(id)}/settings/retry`),
};

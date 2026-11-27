import { existsSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { describe, expect, it } from "vitest";
import type { AgentRole, Config, Defaults, Roster } from "@/lib/settings";
import { AGENT_ROLES, agentDefaults, agentProfile, configProblems, decimalOf, editable, freeID, implementerOf, numberOf, rosterProblems, same, sendable, withLimit } from "./model";

const roster = (): Roster => ({
  language: "",
  model: "",
  effort: "",
  limits: { topics: null, disputes: null, dispute_rounds: null, calls: 60, parallel: null, timeout_seconds: null },
  roles: [
    { id: "owner", title: "Owner", model: "", effort: "", stance: "Outcome.", owns: ["docs/product/"], decides: true, plans: false },
    { id: "builder", title: "Builder", model: "", effort: "", stance: "How.", owns: ["docs/tech/"], decides: false, plans: true },
  ],
});

const config = (): Config => ({
  name: "game",
  description: "",
  integration_branch: "develop",
  release_branch: "main",
  repos: [{ name: "root", path: "./", language: "go", setup: "", build: "", test: "" }],
  worker: { backend: "codex", model: "", effort: "" },
  agents: { reviewer: role(), resolver: role() },
  review: { score_threshold: 7, merge_strategy: "squash" },
  retries: { review_attempts: null, worker: null },
  verify_minutes: null,
  engine: { max_parallel: null, max_per_backend: {} },
});

function role(set: Partial<AgentRole> = {}): AgentRole {
  return { backend: "", model: "", effort: "", minutes: null, usd: null, ...set };
}

/** The defaults the server gives, as DefaultsOfConfig has them. */
const defaults: Defaults["config"] = {
  backend: "codex",
  model: "sonnet",
  score_threshold: 7,
  merge_strategy: "squash",
  review_attempts: 3,
  worker_retries: 2,
  verify_minutes: 10,
  max_parallel: 1,
  backends: ["codex", "claude-code"],
  strategies: ["squash", "merge", "rebase"],
  efforts: ["low", "medium", "high", "xhigh", "max"],
  agents: { minutes: { reviewer: 20, resolver: 20 }, max_minutes: { reviewer: 120, resolver: 120 }, codex_efforts: ["minimal", "low", "medium", "high", "xhigh"] },
};

interface Shown {
  backend: string;
  model: string;
  effort: string;
  minutes: number;
  usd?: number;
}

/** repoRoot is the repository the page is in: the first folder up from here with go.mod. */
function repoRoot(): string {
  for (let dir = process.cwd(); ; dir = dirname(dir)) {
    if (existsSync(join(dir, "go.mod"))) return dir;
    if (dirname(dir) === dir) throw new Error(`no go.mod above ${process.cwd()}`);
  }
}

interface DefaultsCase {
  name: string;
  worker: { backend?: string; model?: string; effort?: string };
  reviewer_set?: Partial<Shown>;
  resolver_set?: Partial<Shown>;
  reviewer: Shown;
  resolver: Shown;
}

describe("the agents' defaults", () => {
  // The cases internal/agent reads too: the defaults the page shows are the
  // ones the engine runs with.
  const cases: DefaultsCase[] = JSON.parse(readFileSync(join(repoRoot(), "internal", "agent", "testdata", "defaults.json"), "utf8"));

  it("are the engine's, for every case", () => {
    expect(cases.length).toBeGreaterThanOrEqual(5);
    for (const c of cases) {
      const worker = { backend: c.worker.backend ?? "", model: c.worker.model ?? "", effort: c.worker.effort ?? "" };
      for (const name of AGENT_ROLES) {
        const set = c[`${name}_set`];
        const got = agentProfile(name, role({ ...set, minutes: set?.minutes ?? null, usd: set?.usd ?? null }), worker, defaults);
        expect(got, `${c.name}, the ${name}`).toEqual({ usd: 0, ...c[name] });
      }
    }
  });

  it("read the Worker as the form has it, saved or not", () => {
    const worker = { backend: "claude-code", model: "opus", effort: "" };
    expect(implementerOf(worker, defaults)).toEqual({ backend: "claude-code", model: "opus", effort: "" });
    expect(agentDefaults("reviewer", worker, defaults).model).toBe("sonnet");
    expect(agentDefaults("reviewer", { ...worker, model: "" }, defaults).model).toBe("opus");
    expect(implementerOf({ backend: "", model: "opus", effort: "high" }, defaults)).toEqual({ backend: "codex", model: "", effort: "" });
  });

  it("keep a number typed as it is, fraction and all", () => {
    expect(decimalOf("1.5")).toBe(1.5);
    expect(decimalOf(" ")).toBeNull();
    expect(decimalOf("x")).toBeNull();
  });
});

describe("rosterProblems", () => {
  it("passes a roster the council can meet with", () => {
    expect(rosterProblems(roster()).count).toBe(0);
  });

  it("says what keeps the council from meeting", () => {
    const r = roster();
    r.roles[0].decides = false;
    r.roles[1].id = "Builder";
    r.roles[1].stance = "  ";
    r.roles.push({ ...r.roles[0], id: "owner", owns: ["docs/tech/"] });
    const p = rosterProblems(r);
    expect(p.whole.join(" ")).toContain("裁定");
    expect(p.whole.join(" ")).toContain("docs/tech/");
    expect(p.roles[1].id).toContain("小寫");
    expect(p.roles[1].stance).toBeTruthy();
    expect(p.roles[2].id).toContain("重複");
  });

  it("needs two roles", () => {
    const r = roster();
    r.roles = r.roles.slice(0, 1);
    expect(rosterProblems(r).whole.join(" ")).toContain("兩個角色");
  });
});

describe("configProblems", () => {
  it("passes what a run can use", () => {
    expect(configProblems(config()).count).toBe(0);
  });

  it("says what is out of range", () => {
    const c = config();
    c.name = " ";
    c.review.score_threshold = 11;
    c.retries.worker = -1;
    c.verify_minutes = 0;
    const p = configProblems(c);
    expect(Object.keys(p.fields).sort()).toEqual(["minutes", "name", "retries", "score"]);
  });

  it("takes no retries", () => {
    const c = config();
    c.retries.worker = 0;
    expect(configProblems(c).count).toBe(0);
  });

  it("holds the agents to their limits", () => {
    const c = config();
    c.agents.reviewer = role({ minutes: 121, usd: -1 });
    c.agents.resolver = role({ minutes: 0 });
    const p = configProblems(c, defaults);
    expect(p.fields).toEqual({ "reviewer.minutes": "是 1 到 120 分鐘。", "reviewer.usd": "不能是負的；不限就空著。", "resolver.minutes": "是 1 到 120 分鐘。" });
    c.agents.reviewer = role({ minutes: 120, usd: 0.5 });
    c.agents.resolver = role({ minutes: 1 });
    expect(configProblems(c, defaults).count).toBe(0);
  });
});

describe("the engine's limits", () => {
  it("holds them to one step or more; a backend's is none when empty", () => {
    const c = config();
    c.engine = { max_parallel: 0, max_per_backend: { codex: 0, "claude-code": 2 } };
    expect(configProblems(c).fields).toEqual({ "engine.max_parallel": "至少是 1。", "engine.codex": "至少是 1；不限就空著。" });
    c.engine = { max_parallel: 3, max_per_backend: { codex: 1 } };
    expect(configProblems(c).count).toBe(0);
    c.engine = { max_parallel: null, max_per_backend: {} };
    expect(configProblems(c).count).toBe(0);
  });

  it("sets a backend's limit, takes it out, and keeps the order the file has", () => {
    const one = withLimit({}, "codex", 2);
    expect(one).toEqual({ codex: 2 });
    const two = withLimit(one, "claude-code", 1);
    expect(Object.keys(two)).toEqual(["claude-code", "codex"]);
    expect(withLimit(two, "codex", null)).toEqual({ "claude-code": 1 });
    expect(two).toEqual({ "claude-code": 1, codex: 2 });
    // Taken out and set again, it reads as the file has it.
    expect(same(withLimit(withLimit(two, "claude-code", null), "claude-code", 1), { "claude-code": 1, codex: 2 })).toBe(true);
  });
});

describe("drafts", () => {
  it("keeps the id the file has, and sends no keys", () => {
    const draft = editable(roster());
    expect(draft.roles[0].from).toBe("owner");
    expect(draft.roles[0].key).toBeTruthy();
    draft.roles[0].id = "producer";
    const sent = sendable(draft);
    expect(sent.roles[0]).not.toHaveProperty("key");
    expect(sent.roles[0].from).toBe("owner");
    // Made editable twice, a roster reads the same.
    expect(same(sendable(editable(roster())), sendable(editable(roster())))).toBe(true);
  });

  it("names a new role with a free id", () => {
    const r = roster();
    r.roles.push({ ...r.roles[0], id: "role-3" });
    expect(freeID(r)).toBe("role-4");
  });

  it("reads a number field", () => {
    expect(numberOf("")).toBeNull();
    expect(numberOf(" 12 ")).toBe(12);
    expect(numberOf("7.8")).toBe(7);
  });
});

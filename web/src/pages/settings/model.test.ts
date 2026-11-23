import { describe, expect, it } from "vitest";
import type { Config, Roster } from "@/lib/settings";
import { configProblems, editable, freeID, numberOf, rosterProblems, same, sendable } from "./model";

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
  review: { score_threshold: 7, merge_strategy: "squash" },
  retries: { review_attempts: null, worker: null },
  verify_minutes: null,
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

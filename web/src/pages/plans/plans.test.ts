import { describe, expect, it } from "vitest";
import type { Entry, Topic } from "@/lib/council";
import { byTopic } from "@/pages/plans/Meeting";
import { headline, planStatus } from "@/pages/plans/status";
import { merge } from "@/pages/plans/usePlan";

const topic = (id: string): Topic => ({ id, title: id, question: "", lead: "owner", speakers: [], reads: null, writes: [] });
const entry = (seq: number, kind: Entry["kind"], topic?: string, step = `s${seq}`): Entry => ({ seq, at: "", step, kind, topic });

describe("byTopic", () => {
  it("puts what was said outside a topic with the topic before it", () => {
    const topics = [topic("scope"), topic("build")];
    const groups = byTopic(
      [entry(1, "agenda"), entry(2, "answers"), entry(3, "proposal", "scope"), entry(4, "word"), entry(5, "speech", "build"), entry(6, "worklist")],
      topics,
    );
    expect(groups.get("scope")?.map((e) => e.seq)).toEqual([1, 2, 3, 4]);
    expect(groups.get("build")?.map((e) => e.seq)).toEqual([5, 6]);
  });

  it("keeps the order of what was said, whatever order it came in", () => {
    const groups = byTopic([entry(3, "speech", "scope"), entry(1, "agenda")], [topic("scope")]);
    expect(groups.get("scope")?.map((e) => e.seq)).toEqual([1, 3]);
  });
});

describe("merge", () => {
  it("keeps the last word of a step said again", () => {
    const merged = merge([entry(1, "speech", "scope", "a"), entry(2, "speech", "scope", "b")], [entry(3, "speech", "scope", "a")]);
    expect(merged.map((e) => `${e.step}${e.seq}`)).toEqual(["b2", "a3"]);
  });
});

describe("planStatus", () => {
  const meeting = (state: string) => ({ state, topic: 1, topics: 2, calls: 0, limit: 10, updated: "", questions: 2 }) as never;
  it("says what the owner is to do first", () => {
    expect(planStatus({ meeting: meeting("questions"), tickets: [] })).toMatchObject({ needsYou: true, tone: "signal" });
    expect(planStatus({ meeting: meeting("agenda"), tickets: [] })).toMatchObject({ needsYou: true, label: "等你看議程" });
    expect(planStatus({ meeting: meeting("stopped"), tickets: [] })).toMatchObject({ needsYou: true, tone: "caution" });
    expect(planStatus({ meeting: meeting("meeting"), tickets: [] })).toMatchObject({ needsYou: false, label: "開會中" });
  });
  it("waits for approval while a ticket is not on GitHub", () => {
    const tickets = [{ key: "T1", title: "", repo: "root", issue: 12 }, { key: "T2", title: "", repo: "root" }];
    expect(planStatus({ meeting: meeting("finished"), tickets })).toMatchObject({ needsYou: true, label: "等你核可" });
    expect(planStatus({ meeting: meeting("finished"), tickets: [tickets[0]] })).toMatchObject({ needsYou: false, label: "已核可" });
    expect(planStatus({ tickets: [] })).toMatchObject({ needsYou: false, label: "準備中" });
  });
  it("puts an approval under way or stopped short first", () => {
    const tickets = [{ key: "T1", title: "", repo: "root", issue: 12 }];
    expect(planStatus({ meeting: meeting("finished"), tickets, approval: { state: "working", step: "checks" } })).toMatchObject({ needsYou: false, label: "核可中" });
    expect(planStatus({ meeting: meeting("finished"), tickets, approval: { state: "failed", step: "checks", why: "checks" } })).toMatchObject({ needsYou: true, label: "核可沒完成" });
    expect(planStatus({ meeting: meeting("finished"), tickets, approval: { state: "stopped", step: "label" } })).toMatchObject({ needsYou: true });
    expect(planStatus({ meeting: meeting("finished"), tickets, approval: { state: "done", step: "done", left: "on branch main" } })).toMatchObject({ needsYou: true, label: "資料夾要更新" });
    expect(planStatus({ meeting: meeting("finished"), tickets, approval: { state: "done", step: "done" } })).toMatchObject({ needsYou: false, label: "已核可" });
  });
});

describe("headline", () => {
  it("is the first line of the brief, without a heading's marks", () => {
    expect(headline("\n# 網球遊戲\n細節", "x")).toBe("網球遊戲");
    expect(headline("", "規劃 p1")).toBe("規劃 p1");
    expect(headline("字".repeat(80), "x")).toHaveLength(59);
  });
});

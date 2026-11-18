import { describe as group, expect, it } from "vitest";
import type { TraceEvent } from "./api";
import { describe, when } from "./words";

const event = (e: Partial<TraceEvent>): TraceEvent => ({ seq: 1, ts: "2026-11-18T03:00:00Z", component: "principal", type: "x", level: "info", ...e });

group("describe", () => {
  it("names who did it and what, with the ticket and the PR told apart", () => {
    expect(describe(event({ component: "worker", type: "pr_create", issue_id: 306, pr_number: 313 }))).toEqual({
      who: "執行者",
      text: "開了 PR #313（工單 #306）",
      tone: "review",
    });
    expect(describe(event({ component: "worker", type: "pr_merge", issue_id: 302, pr_number: 311 })).text).toBe("合併了 PR #311，工單 #302 完成");
  });

  it("says the reason the event gives", () => {
    expect(describe(event({ component: "reviewer", type: "review_decision", pr_number: 313, data: { decision: "changes requested" } })).text).toBe(
      "審查 PR #313：changes requested",
    );
  });

  it("keeps the signal's yellow for what needs the person: a warning is caution, an error stop", () => {
    expect(describe(event({ type: "hook_failed", level: "warn" })).tone).toBe("caution");
    expect(describe(event({ type: "something_new", level: "warn" })).tone).toBe("caution");
    expect(describe(event({ type: "pr_merge_fail", level: "error", pr_number: 9 })).tone).toBe("stop");
    for (const type of ["worker_retry", "comment_fail", "hook_failed", "workflow_stop"]) {
      expect(describe(event({ type })).tone, type).not.toBe("signal");
    }
  });

  it("shows an event it has no words for as the event names itself", () => {
    expect(describe(event({ component: "audit", type: "drift_found", issue_id: 7, data: { message: "two configs" } }))).toEqual({
      who: "稽核",
      text: "drift_found #7 two configs",
      tone: "muted",
    });
  });
});

group("when", () => {
  const now = new Date(2026, 10, 18, 15, 0);

  it("says only the hour today", () => {
    expect(when(new Date(2026, 10, 18, 9, 5).toISOString(), now)).toBe("09:05");
  });

  it("adds the date on another day, and the year in another year", () => {
    expect(when(new Date(2026, 10, 17, 23, 30).toISOString(), now)).toBe("11/17 23:30");
    expect(when(new Date(2025, 0, 2, 8, 0).toISOString(), now)).toBe("2025/1/2 08:00");
  });

  it("says nothing of a time it cannot read", () => {
    expect(when("not a time", now)).toBe("");
  });
});

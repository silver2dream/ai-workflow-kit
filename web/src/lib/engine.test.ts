import { describe, expect, it } from "vitest";
import type { Decision, LaneState } from "./api";
import { decisionWords, endedWords, KNOWN, laneMeta, LANES, stepWords, WHY, whyWords } from "./engine";
import { counts, titleOf } from "@/pages/run/Lanes";

const decision = (d: Partial<Decision>): Decision => ({ next_action: "none", issue_number: 0, pr_number: 0, exit_reason: "", merge_issue: "", ...d });

describe("engine words", () => {
  it("says every state of a lane, and keeps the signal's yellow for what needs the person", () => {
    const states: LaneState[] = ["blocked", "ready", "implementing", "checking", "reviewing", "review_retry", "rework", "awaiting_approval", "queued", "resolving", "merged", "completed", "needs_you", "paused", "cancelled"];
    for (const s of states) {
      expect(LANES[s].label, s).toBeTruthy();
      expect(LANES[s].means, s).toBeTruthy();
      expect(LANES[s].tone === "signal", s).toBe(s === "needs_you" || s === "awaiting_approval");
    }
    expect(laneMeta({ state: "resolving", resolve: "conflict" }).label).toBe("PR 有衝突");
    expect(laneMeta({ state: "resolving", resolve: "rebase" }).label).toBe("PR 落後了");
    expect(laneMeta({ state: "implementing", resolve: "conflict" }).label).toBe("解衝突中");
    expect(laneMeta({ state: "implementing", resolve: "rebase" }).label).toBe("更新 PR 中");
    expect(laneMeta({ state: "implementing" }).label).toBe("寫程式中");
    expect(laneMeta({ state: "sideways" as LaneState }).label).toBe("sideways");
  });

  it("says why a lane needs the person, for every reason the engine has", () => {
    for (const why of ["worker_failed", "needs_human_review", "review_max_retries", "conflict_max_retries", "no_pr", "merge_blocked", "guard_blocked", "guard_output", "invalid_ticket", "dependency_cycle", "person", "agent_refused", "worker_lost"]) {
      expect(WHY[why], why).toBeTruthy();
    }
    expect(whyWords("something_new")).toBe("something_new");
    expect(whyWords(undefined)).toBe("");
  });

  it("says what the engine does in a step, with the ticket's title beside its number", () => {
    expect(stepWords({ action: "review_pr", issue: 5, pr: 12, since: "" })).toBe("審查者正在審工單 #5 的 PR #12");
    expect(stepWords({ action: "review_pr", issue: 5, pr: 12, since: "" }, "顯示房間代碼")).toBe("審查者正在審工單 #5「顯示房間代碼」的 PR #12");
    expect(stepWords({ action: "dispatch_worker", issue: 5, since: "" })).toBe("執行者正在寫工單 #5");
    expect(stepWords({ action: "check_result", issue: 5, since: "" }, "開場")).toBe("執行者正在寫工單 #5「開場」");
    expect(stepWords({ action: "dispatch_worker", issue: 5, pr: 12, merge: "conflict", since: "" })).toBe("正在解工單 #5 的 PR #12 和整合分支的衝突");
    expect(stepWords({ action: "dispatch_worker", issue: 5, pr: 12, merge: "rebase", since: "" })).toBe("正在把工單 #5 的 PR #12 更新到整合分支最新的樣子");
    // A step that ends the run, or settles a lane on the way, is no work on it.
    expect(stepWords({ action: "none", issue: 5, since: "" })).toBe("");
    expect(stepWords({ action: "all_complete", since: "" })).toBe("");
  });

  it("says how a run of the engine ended, and keeps a reason it does not know", () => {
    expect(endedWords("all_tasks_complete")).toBe("上一輪做完了：沒有可以做的工單了。");
    expect(endedWords("needs_human_review")).toBe("上一輪停了：有工單需要你。");
    expect(endedWords("user_stopped")).toBe("上一輪照你的要求停了。");
    expect(endedWords("something_new")).toBe("上一輪結束了（something_new）。");
  });

  it("says each side's decision of the shadow", () => {
    expect(decisionWords(null)).toBe("手上沒有票，接著挑下一張");
    expect(decisionWords(decision({ next_action: "review_pr", issue_number: 5, pr_number: 12 }))).toBe("審查工單 #5 的 PR #12");
    expect(decisionWords(decision({ next_action: "dispatch_worker", issue_number: 5, pr_number: 12, merge_issue: "conflict" }))).toBe("解工單 #5 的 PR #12 的衝突");
    expect(decisionWords(decision({ next_action: "dispatch_worker", issue_number: 5 }))).toBe("把工單 #5 交給執行者");
    expect(decisionWords(decision({ next_action: "check_result", issue_number: 5 }))).toBe("看工單 #5 的執行者做完了沒");
    expect(decisionWords(decision({ next_action: "none", issue_number: 7, exit_reason: "needs_human_review" }))).toBe("工單 #7：停下來，等人處理");
    expect(decisionWords(decision({ next_action: "none", exit_reason: "max_loop_reached" }))).toBe("停下來，max_loop_reached");
    expect(decisionWords(decision({ next_action: "all_complete" }))).toBe("全部做完了");
    expect(KNOWN.order && KNOWN.check_first).toBeTruthy();
  });
});

describe("lane rows", () => {
  it("lists only the counts a lane used, of its limits", () => {
    const limits = { failures: 3, reviews: 3, conflicts: 2 };
    expect(counts({ issue: 1, state: "reviewing", attempts: {}, since: "" }, limits)).toEqual([]);
    expect(counts({ issue: 1, state: "reviewing", attempts: { failures: 1, reviews: 2, conflicts: 2 }, since: "" }, limits)).toEqual([
      "執行者失敗 1／3 次",
      "重新審查 2／3 次",
      "送去解衝突 2／2 次",
    ]);
  });

  it("finds a ticket's title among the open ones, then the done ones", () => {
    const tickets = { tickets: { open: [{ number: 5, title: "open one", stage: "in_review" as const }], done: 1, done_titles: { "8": "done one" } }, stages: [], asked_at: "" };
    expect(titleOf(tickets, 5)).toBe("open one");
    expect(titleOf(tickets, 8)).toBe("done one");
    expect(titleOf(tickets, 9)).toBeUndefined();
    expect(titleOf(undefined, 5)).toBeUndefined();
  });
});

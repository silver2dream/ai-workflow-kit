import { describe as group, expect, it } from "vitest";
import type { TraceEvent } from "./api";
import { FILTERS, shownFilters } from "./activity";
import { describe, eventKey, whoOf } from "./words";

const event = (e: Partial<TraceEvent>): TraceEvent => ({ seq: 1, ts: "2026-11-27T03:00:00Z", component: "engine", type: "x", level: "info", ...e });

group("the engine's events", () => {
  it("says a lane's move: from where to where, and why", () => {
    expect(describe(event({ type: "lane_transition", issue_id: 5, data: { issue: 5, from: "reviewing", to: "merged", cause: "merged" } }))).toEqual({
      who: "engine",
      text: "工單 #5：審查中 → 已合併（合併了）",
      tone: "go",
    });
    expect(describe(event({ type: "lane_transition", issue_id: 5, data: { from: "ready", to: "implementing", cause: "dispatched" } })).text).toBe("工單 #5：可以開始 → 寫程式中（交給執行者）");
    expect(describe(event({ type: "lane_transition", issue_id: 9, data: { from: "reviewing", to: "resolving", resolve: "rebase", cause: "conflicting" } })).text).toBe(
      "工單 #9：審查中 → PR 落後了（和整合分支衝突或落後）",
    );
    // A lane opened has no from; one with no issue number of its own says the data's.
    expect(describe(event({ type: "lane_transition", data: { issue: 3, to: "ready", cause: "imported" } })).text).toBe("工單 #3：可以開始（從工單的標籤接手）");
  });

  it("keeps the signal's yellow for a lane that needs the person, and says why", () => {
    const d = describe(event({ type: "lane_transition", level: "warn", issue_id: 7, data: { from: "reviewing", to: "needs_you", cause: "escalated", why: "guard_output" } }));
    expect(d.tone).toBe("signal");
    expect(d.text).toBe("工單 #7：審查中 → 需要你（交給你）。防護擋下了要寫到 GitHub 的內容。");
    expect(describe(event({ type: "lane_transition", data: { from: "needs_you", to: "rework", cause: "released" } })).tone).not.toBe("signal");
  });

  it("says a run's start and end, a command and what became of it", () => {
    expect(describe(event({ type: "engine_run", data: { event: "started" } })).text).toBe("engine 開始派工");
    expect(describe(event({ type: "engine_run", data: { event: "ended", reason: "all_tasks_complete" } })).text).toBe("engine 停了：工單都做完了");
    expect(describe(event({ type: "engine_run", data: { event: "ended", reason: "error_exit", error: "the journal" } }))).toMatchObject({ text: "engine 停了：error_exit（the journal）", tone: "caution" });
    expect(describe(event({ type: "engine_command", data: { command: { kind: "stop_now", by: "the workbench" }, reply: { result: "applied" } } })).text).toBe("收到「立刻停」（工作台），收下了");
    expect(describe(event({ type: "engine_command", level: "warn", data: { command: { kind: "pause", by: "awkit lane" }, reply: { result: "rejected", reason: "from task 2.6 on" } } }))).toMatchObject({
      text: "收到「pause」（awkit lane），沒收下：from task 2.6 on",
      tone: "caution",
    });
  });

  it("says how a decision compared with the Principal's", () => {
    expect(describe(event({ type: "engine_shadow", data: { engine: null, analyzer: null, same: true } }))).toMatchObject({ text: "和主控的判斷一樣", tone: "muted" });
    expect(describe(event({ type: "engine_shadow", data: { same: false, known: "order" } })).text).toBe("和主控的判斷不同，原因已知：兩張工單的狀態一樣，兩邊先挑了不同的一張");
    // A difference is a warning, at whatever level it was written.
    const differs = describe(event({
      type: "engine_shadow",
      level: "info",
      data: { same: false, engine: { next_action: "review_pr", issue_number: 5, pr_number: 12, exit_reason: "", merge_issue: "" }, analyzer: null },
    }));
    expect(differs).toEqual({ who: "engine", text: "和主控的判斷不同：engine 要審查工單 #5 的 PR #12，主控要手上沒有票，接著挑下一張", tone: "caution" });
    expect(describe(event({ type: "engine_shadow", data: { error: "gh failed" } })).tone).toBe("caution");
  });

  it("says an agent's run by its role: with what, how long, how much; or why it did not finish", () => {
    const ok = describe(event({ component: "agent", type: "agent_run", issue_id: 5, data: { role: "reviewer", backend: "claude-code", model: "sonnet", status: "ok", usage: { duration_ms: 185000, cost_usd: 0.183 } } }));
    expect(ok).toEqual({ who: "審查", text: "工單 #5 跑完了（claude-code sonnet，3 分鐘，$0.18）", tone: "muted" });
    expect(describe(event({ component: "agent", type: "agent_run", level: "warn", data: { role: "resolver", status: "timeout", error: "20 minutes" } }))).toEqual({
      who: "解衝突",
      text: "沒跑完（timeout）：20 minutes",
      tone: "caution",
    });
    expect(whoOf(event({ component: "agent", type: "agent_run", data: { role: "planner" } }))).toBe("agent");
  });

  it("says a run's own start and end as the run's, not the Principal's", () => {
    for (const type of ["session_start", "session_end", "workflow_stop"]) {
      expect(whoOf(event({ component: "principal", type })), type).toBe("這一輪");
    }
    expect(whoOf(event({ component: "principal", type: "loop_start" }))).toBe("主控");
    expect(describe(event({ component: "principal", type: "deprecated_path", level: "warn", data: { path: "analyze-next" } })).text).toBe("用了舊的主控做法：analyze-next（下一版移除）");
  });

  it("tells apart events of different files that count alike", () => {
    expect(eventKey(event({ component: "engine", seq: 3 }))).not.toBe(eventKey(event({ component: "principal", seq: 3 })));
  });
});

group("who dispatched", () => {
  it("says the worker's functions' events, under the Principal's name, as the engine's in a run it dispatched", () => {
    const dispatch = event({ component: "principal", type: "dispatch_start", issue_id: 5 });
    expect(describe(dispatch, "engine")).toMatchObject({ who: "engine", text: "把工單 #5 交給執行者" });
    expect(whoOf(dispatch, "principal")).toBe("主控");
    expect(whoOf(dispatch)).toBe("主控");
    expect(whoOf(event({ component: "principal", type: "session_start" }), "engine")).toBe("這一輪");
  });
});

group("the activity's filters", () => {
  const run = [
    event({ component: "principal", type: "session_start" }),
    event({ component: "principal", type: "dispatch_start" }),
    event({ component: "engine", type: "lane_transition" }),
    event({ component: "agent", type: "agent_run", data: { role: "resolver" } }),
    event({ component: "agent", type: "agent_run", data: { role: "implementer" } }),
    event({ component: "reviewer", type: "review_start" }),
    event({ component: "guard", type: "guardrail_verdict", level: "error" }),
  ];
  const keep = (key: string, e: TraceEvent, by?: "engine" | "principal") => FILTERS.find((f) => f.key === key)!.keep(e, by);

  it("shows the Principal's only in a run it dispatched", () => {
    expect(shownFilters(run, "engine").map((f) => f.key)).toEqual(["all", "engine", "worker", "reviewer", "guard", "attention"]);
    expect(shownFilters(run, "principal").map((f) => f.key)).toContain("principal");
    expect(shownFilters([event({ component: "principal", type: "session_start" })], "principal").map((f) => f.key)).not.toContain("principal");
  });

  it("puts each event under who did it", () => {
    expect(run.filter((e) => keep("engine", e, "engine")).map((e) => e.type)).toEqual(["dispatch_start", "lane_transition"]);
    expect(run.filter((e) => keep("engine", e, "principal")).map((e) => e.type)).toEqual(["lane_transition"]);
    expect(run.filter((e) => keep("principal", e, "principal")).map((e) => e.type)).toEqual(["dispatch_start"]);
    expect(run.filter((e) => keep("principal", e, "engine"))).toEqual([]);
    expect(run.filter((e) => keep("reviewer", e)).map((e) => e.type)).toEqual(["agent_run", "review_start"]);
    expect(run.filter((e) => keep("worker", e)).map((e) => e.data?.role)).toEqual(["implementer"]);
    expect(run.filter((e) => keep("attention", e)).map((e) => e.type)).toEqual(["guardrail_verdict"]);
  });
});

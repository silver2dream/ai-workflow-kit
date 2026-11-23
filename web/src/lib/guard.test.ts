import { describe as group, expect, it } from "vitest";
import type { TraceEvent } from "./api";
import { heldBackWords, ruleWords, sourceWords } from "./guard";
import { describe } from "./words";

const event = (e: Partial<TraceEvent>): TraceEvent => ({ seq: 1, ts: "2026-11-23T03:00:00Z", component: "guard", type: "x", level: "info", ...e });

group("heldBackWords", () => {
  it("says an injection rule by its id, whatever the server's English", () => {
    expect(heldBackWords("tells the reader to ignore or override its instructions (injection-rules/1#override-en)")).toBe("叫讀的人忽略或推翻原本的指示");
    expect(heldBackWords("a long encoded block, which may hide text (injection-rules/1#encoded-blob)")).toBe("一大段編碼過的內容，可能藏著文字");
  });

  it("says the lesson content policy's sentences", () => {
    expect(heldBackWords("it reaches for another host")).toBe("要連到外面（網址或下載指令）");
    expect(heldBackWords("it speaks of the workflow's permissions and checks")).toContain("權限與檢查");
  });

  it("keeps the server's words for a reason it does not know", () => {
    expect(heldBackWords("something new (injection-rules/2#later)")).toBe("something new (injection-rules/2#later)");
    expect(heldBackWords("")).toBe("");
  });
});

group("ruleWords and sourceWords", () => {
  it("says each guard's rules, and falls back to the reason", () => {
    expect(ruleWords("outbound/1#github-token")).toContain("GitHub 的 token");
    expect(ruleWords("diff-signals/1#network")).toBe("變更裡新增了連到外面的呼叫");
    expect(ruleWords("tool-policy/1#merge")).toContain("合併");
    expect(ruleWords("tool-policy/1#git-config", "changes git's configuration")).toBe("changes git's configuration");
    expect(ruleWords("lesson-screen", "it reaches for another host")).toBe("要連到外面（網址或下載指令）");
  });

  it("says where the text came from", () => {
    expect(sourceWords("lesson L-004")).toBe("教訓 L-004");
    expect(sourceWords("review comments on issue #12")).toBe("工單 #12 的審查意見");
    expect(sourceWords("gh pr create")).toBe("要送到 GitHub 的 PR 內容");
    expect(sourceWords("gh issue comment")).toBe("要送到 GitHub 的工單留言");
    expect(sourceWords("the Worker's change")).toBe("執行者的變更");
    expect(sourceWords("codebase map")).toBe("codebase map");
  });
});

group("describe a guard's events", () => {
  it("says a block and a flag in words, with the ticket", () => {
    const blocked = describe(
      event({ type: "guardrail_verdict", level: "error", issue_id: 12, data: { result: "block", rule: "outbound/1#github-token", source: "gh pr create", reason: "x" } }),
    );
    expect(blocked).toEqual({ who: "防護", text: "工單 #12 擋下：要送到 GitHub 的 PR 內容（要寫到 GitHub 的內容裡有 GitHub 的 token）", tone: "stop" });
    const flagged = describe(event({ type: "guardrail_verdict", level: "warn", data: { result: "flag", rule: "diff-signals/1#network", source: "the Worker's change" } }));
    expect(flagged.text).toBe("標記為可疑：執行者的變更（變更裡新增了連到外面的呼叫）");
    expect(flagged.tone).toBe("caution");
  });

  it("says a person's decision as theirs, and where they made it", () => {
    expect(describe(event({ type: "guardrail_decision", data: { rule: "lesson-screen", piece: "L-4", decision: "approve", by: "workbench" } }))).toEqual({
      who: "你",
      text: "在工作台放行了被擋下的教訓 L-4",
      tone: "caution",
    });
    expect(describe(event({ type: "guardrail_decision", data: { rule: "lesson-screen", piece: "L-5", decision: "reject", by: "cli" } })).text).toBe(
      "在終端機不採用被擋下的教訓 L-5",
    );
  });
});

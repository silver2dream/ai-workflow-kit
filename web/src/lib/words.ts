import type { Decision, Dispatcher, LaneState, Stage, TraceEvent } from "./api";
import { byWords, CAUSES, COMMANDS, decisionWords, KNOWN, LANES, laneMeta, ROLES, runEndWords, whyWords } from "./engine";
import { ruleWords, sourceWords } from "./guard";

/** The words of AWK a newcomer meets, said plainly; shown where they appear, and in the glossary. */
export const TERMS: Record<string, string> = {
  議會: "六個 AI 角色一起開會規劃：製作人、遊戲設計、數值、文案、技術、品質。談不攏的事由負責裁定的角色決定。",
  決策: "議會定下的選擇，之後的文件和程式都照著做。每條有編號，例如 D3。",
  假設: "議會缺資訊時先替你假設的事。會列給你確認，不對就改。",
  里程碑: "做完能看到一個成果的階段，例如「兩台電腦能配對進場」。一次只替下一個里程碑開工單。",
  工單: "一件要做的事：做什麼、怎麼算完成。建在 GitHub 上，編號像 #306。",
  PR: "照著工單改好的程式，送去審查，通過才合併。和工單共用編號，所以畫面上一律寫清楚是「工單 #306」還是「PR #313」。",
  審查: "另一個 AI 檢查 PR：有沒有照工單做、測試夠不夠、CI 有沒有過。沒過會自動重做，幾次都沒過就停下來等你。",
  合併: "審查通過的 PR 放進專案的主線，那張工單就完成了。",
  執行者: "拿到一張工單、寫程式、開 PR 的 AI。",
  engine: "awkit 裡派工的程式：決定下一步做哪張工單、什麼時候送審、合併，或停下來等你。它不是 AI，同樣的情況每次都做同樣的決定。",
  主控: "舊的做法裡派工的 AI，只有設了 AWKIT_ENGINE=principal 才會用，下一版移除。平常派工的是 engine。",
  CI: "每個 PR 都會自動跑的建置與測試，綠燈代表通過。",
  教訓: "被審查退回、後來終於通過的變更，會濃縮成一條短的提醒。之後動到同樣檔案的執行者與審查者會看到相關的幾條。",
  隔離: "教訓是 AI 從審查的經過寫出來的，裡面可能夾帶別人寫的文字。給任何 AI 看之前會先檢查；看起來像在下指令、要連到外面、或碰流程本身的，就先隔離，等你讀過再決定。",
};

export type Tone = "signal" | "live" | "review" | "go" | "muted" | "stop" | "focus" | "caution";

export interface StageMeta {
  label: string;
  tone: Tone;
  /** What the stage means, for a person who sees it the first time. */
  means: string;
}

export const STAGES: Record<Stage | "done", StageMeta> = {
  needs_you: { label: "需要你", tone: "signal", means: "只有你能決定怎麼辦：審查幾次都沒過，或執行者做不下去。" },
  in_progress: { label: "寫程式中", tone: "live", means: "執行者正在寫這張。" },
  rework: { label: "照審查意見重做", tone: "live", means: "審查沒過，交回執行者照意見改。" },
  in_review: { label: "審查中", tone: "review", means: "PR 開好了，正在審查。" },
  ready: { label: "可以開始", tone: "muted", means: "前面的票都做完了，下一輪會輪到它。" },
  waiting: { label: "等前面的票", tone: "muted", means: "它依賴的工單還沒做完。" },
  done: { label: "已合併", tone: "go", means: "審查通過，已經合併。" },
};

/** Where a stage sits on the line from the brief to the merge. */
export const STATION_OF: Record<Stage, "tickets" | "code" | "review" | "siding"> = {
  ready: "tickets",
  waiting: "tickets",
  in_progress: "code",
  rework: "code",
  in_review: "review",
  needs_you: "siding",
};

export const WHO: Record<string, string> = {
  engine: "engine",
  principal: "主控",
  worker: "執行者",
  reviewer: "審查",
  hooks: "你的指令",
  audit: "稽核",
  guard: "防護",
};

/** when says a time the short way: the hour today, the date as well on another day. */
export function when(ts: string, now = new Date()): string {
  const d = new Date(ts);
  if (Number.isNaN(d.getTime())) return "";
  const time = d.toLocaleTimeString("zh-TW", { hour: "2-digit", minute: "2-digit", hourCycle: "h23" });
  if (d.toDateString() === now.toDateString()) return time;
  const date = `${d.getMonth() + 1}/${d.getDate()}`;
  return d.getFullYear() === now.getFullYear() ? `${date} ${time}` : `${d.getFullYear()}/${date} ${time}`;
}

const word = (data: Record<string, unknown> | undefined, ...keys: string[]) => {
  for (const key of keys) {
    const value = data?.[key];
    if (typeof value === "string" && value.trim()) return value.trim();
    if (typeof value === "number") return String(value);
  }
  return "";
};

/** inner is a record a field of an event's data holds: a command, a reply, a usage. */
const inner = (data: Record<string, unknown> | undefined, key: string): Record<string, unknown> | undefined => {
  const value = data?.[key];
  return value && typeof value === "object" ? (value as Record<string, unknown>) : undefined;
};

export interface Described {
  who: string;
  text: string;
  tone: Tone;
}

/** eventKey tells the events of a run apart: they come from more than one file, and each file counts its own. */
export const eventKey = (e: TraceEvent) => `${e.component}-${e.seq}-${e.ts}`;

/** The events of a run itself, which `awkit run` writes whoever dispatches: not the Principal's. */
export const RUN_EVENTS = new Set(["session_start", "session_end", "session_usage", "workflow_stop"]);

/**
 * dispatches says whether an event is the dispatcher's own: written under
 * the Principal's name, by the Principal, or by the worker's functions the
 * engine calls too. Not the run's own start and end.
 */
export const dispatches = (event: TraceEvent) => event.component === "principal" && !RUN_EVENTS.has(event.type);

/** whoOf says who did what an event says, in a run the dispatcher dispatched: the Principal's when it is not told. */
export function whoOf(event: TraceEvent, dispatcher?: Dispatcher): string {
  if (RUN_EVENTS.has(event.type)) return "這一輪";
  if (event.component === "agent") return ROLES[word(event.data, "role")] ?? "agent";
  if (dispatches(event) && dispatcher === "engine") return WHO.engine;
  return WHO[event.component] ?? event.component;
}

/** describe says an event in a sentence. What it cannot put in words it shows as the event names itself. */
export function describe(event: TraceEvent, dispatcher?: Dispatcher): Described {
  const issue = event.issue_id ? `#${event.issue_id}` : "";
  const pr = event.pr_number ? `PR #${event.pr_number}` : "";
  const data = event.data;
  const reason = word(data, "reason", "message", "error", "status", "decision", "action", "next_action", "name");
  const tone: Tone = event.level === "error" ? "stop" : event.level === "warn" ? "caution" : event.level === "decision" ? "focus" : "muted";
  const who = whoOf(event, dispatcher);
  const say = (text: string, t: Tone = tone): Described => ({ who, text, tone: t });
  switch (event.type) {
    case "session_start":
      return say("開始工作", "focus");
    case "session_end":
      return say(reason ? `這一輪結束：${reason}` : "這一輪結束");
    case "lane_transition": {
      const to = word(data, "to") as LaneState;
      const from = word(data, "from") as LaneState;
      const ticket = issue || (word(data, "issue") ? `#${word(data, "issue")}` : "");
      const cause = word(data, "cause");
      const said = [
        from ? `${LANES[from]?.label ?? from} → ` : "",
        laneMeta({ state: to, resolve: word(data, "resolve") }).label,
        cause ? `（${CAUSES[cause] ?? cause}）` : "",
        to === "needs_you" && word(data, "why") ? `。${whyWords(word(data, "why"))}` : "",
      ].join("");
      const t: Tone = to === "needs_you" ? "signal" : to === "merged" || to === "completed" ? "go" : to === "implementing" || to === "reviewing" ? "live" : "muted";
      return say(`工單 ${ticket}：${said}`, t);
    }
    case "engine_run":
      return word(data, "event") === "started"
        ? say("engine 開始派工", "focus")
        : say(`engine 停了：${runEndWords(word(data, "reason"))}${word(data, "error") ? `（${word(data, "error")}）` : ""}`, word(data, "error") ? "caution" : tone);
    case "engine_command": {
      const command = inner(data, "command");
      const reply = inner(data, "reply");
      const kind = word(command, "kind");
      const by = byWords(word(command, "by"));
      const what = `收到「${COMMANDS[kind] ?? kind}」${by ? `（${by}）` : ""}`;
      return word(reply, "result") === "applied" ? say(`${what}，收下了`) : say(`${what}，沒收下：${word(reply, "reason")}`, "caution");
    }
    case "engine_shadow": {
      if (word(data, "error")) return say(`沒和主控比成：${word(data, "error")}`, "caution");
      if (data?.same === true) return say("和主控的判斷一樣");
      const known = word(data, "known");
      if (known) return say(`和主控的判斷不同，原因已知：${KNOWN[known] ?? known}`);
      const mine = (data?.engine ?? null) as Decision | null;
      const theirs = (data?.analyzer ?? null) as Decision | null;
      return say(`和主控的判斷不同：engine 要${decisionWords(mine)}，主控要${decisionWords(theirs)}`, "caution");
    }
    case "agent_run": {
      const usage = inner(data, "usage");
      const minutes = Number(usage?.duration_ms ?? 0) / 60000;
      const cost = Number(usage?.cost_usd ?? 0);
      const how = [
        [word(data, "backend"), word(data, "model")].filter(Boolean).join(" "),
        minutes >= 1 ? `${Math.round(minutes)} 分鐘` : minutes > 0 ? "不到 1 分鐘" : "",
        cost > 0 ? `$${cost.toFixed(2)}` : "",
      ].filter(Boolean);
      const where = issue ? `工單 ${issue} ` : "";
      const status = word(data, "status");
      if (status === "ok") return say(`${where}跑完了${how.length ? `（${how.join("，")}）` : ""}`);
      return say(`${where}沒跑完（${status}）${word(data, "error") ? `：${word(data, "error")}` : ""}`, "caution");
    }
    case "deprecated_path":
      return say(`用了舊的主控做法：${word(data, "path")}（下一版移除）`, "caution");
    case "loop_start":
      return say(reason ? `問下一步（${reason}）` : "問下一步");
    case "loop_decision":
      return say(reason ? `決定下一步：${reason}` : "決定下一步", "focus");
    case "dispatch_start":
      return say(`把工單 ${issue} 交給執行者`, "live");
    case "dispatch_end":
      return say(reason ? `執行者交回工單 ${issue}：${reason}` : `執行者交回工單 ${issue}`);
    case "check_result":
      return say(reason ? `檢查工單 ${issue} 的結果：${reason}` : `檢查工單 ${issue} 的結果`);
    case "start":
      return say(`開始寫工單 ${issue}`, "live");
    case "step":
      return say(reason ? `工單 ${issue}：${reason}` : `工單 ${issue} 的下一步`);
    case "end":
      return say(reason ? `工單 ${issue} 寫完：${reason}` : `工單 ${issue} 寫完`);
    case "worker_retry":
      return say(`工單 ${issue} 再試一次`, "caution");
    case "comment_send":
      return say(`在 GitHub 的 ${issue || pr} 留言`);
    case "comment_fail":
      return say(`GitHub 的 ${issue || pr} 留言失敗`, "caution");
    case "label_update":
      return say(reason ? `工單 ${issue} 的狀態改成 ${reason}` : `更新工單 ${issue} 的狀態`);
    case "pr_create":
      return say(`開了 ${pr || "PR"}${issue ? `（工單 ${issue}）` : ""}`, "review");
    case "pr_merge":
      return say(`合併了 ${pr || "PR"}${issue ? `，工單 ${issue} 完成` : ""}`, "go");
    case "pr_merge_fail":
      return say(reason ? `${pr} 合併失敗：${reason}` : `${pr} 合併失敗`, "stop");
    case "review_start":
      return say(`開始審查 ${pr}`, "review");
    case "review_decision":
      return say(reason ? `審查 ${pr}：${reason}` : `審查 ${pr} 有了結果`, "review");
    case "review_end":
      return say(`審查 ${pr} 結束`);
    case "hook_fired":
      return say(reason ? `執行你設定的指令：${reason}` : "執行你設定的指令");
    case "hook_failed":
      return say(reason ? `你設定的指令失敗：${reason}` : "你設定的指令失敗", "caution");
    case "audit_triggered":
      return say("開始稽核");
    case "workflow_stop":
      return say(reason ? `工作停下來：${reason}` : "工作停下來", "caution");
    case "reconcile":
      return say("自動修正了一個不一致的狀態");
    case "guardrail_verdict": {
      const what = sourceWords(word(data, "source", "piece"));
      const why = ruleWords(word(data, "rule"), word(data, "reason"));
      const blocked = word(data, "result") === "block";
      const text = `${blocked ? "擋下" : "標記為可疑"}${what ? `：${what}` : ""}${why ? `（${why}）` : ""}`;
      return say(issue ? `工單 ${issue} ${text}` : text, blocked ? "stop" : "caution");
    }
    case "guardrail_decision": {
      const piece = word(data, "piece");
      const approved = word(data, "decision") === "approve";
      const where = word(data, "by") === "workbench" ? "在工作台" : "在終端機";
      const what = word(data, "rule") === "lesson-screen" ? `教訓 ${piece}` : piece;
      return { who: "你", text: `${where}${approved ? "放行了" : "不採用"}被擋下的${what}`, tone: approved ? "caution" : "muted" };
    }
    default:
      return say([event.type, issue || pr, reason].filter(Boolean).join(" "));
  }
}

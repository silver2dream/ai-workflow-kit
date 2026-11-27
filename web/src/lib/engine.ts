// What the engine does and where each ticket is in its hands, in words a
// person who does not write code reads. The server says it with the
// engine's own names (internal/engine); they are put in words here, and a
// name this page does not know is shown as it is.
import type { Decision, EngineStep, Lane, LaneState } from "./api";
import type { StageMeta } from "./words";

/** Where a ticket is in the engine's hands. */
export const LANES: Record<LaneState, StageMeta> = {
  blocked: { label: "等前面的票", tone: "muted", means: "它依賴的工單還沒合併。" },
  ready: { label: "可以開始", tone: "muted", means: "輪到它時，engine 會把它交給執行者。" },
  implementing: { label: "寫程式中", tone: "live", means: "執行者正在寫這張。" },
  checking: { label: "檢查中", tone: "live", means: "engine 在檢查改動：建置、測試、改了哪些檔案。" },
  reviewing: { label: "審查中", tone: "review", means: "PR 開好了，等著審查或正在審查。" },
  review_retry: { label: "要再審一次", tone: "review", means: "上一次審查沒做成（例如 CI 沒過，或審查出錯），會再審一次。" },
  rework: { label: "照審查意見重做", tone: "live", means: "審查要它改，或執行者上一次失敗了，等著再交給執行者。" },
  awaiting_approval: { label: "等你核可", tone: "signal", means: "改動有風險，合併前要你看過。" },
  queued: { label: "排隊合併", tone: "review", means: "審查通過，等輪到它合併。" },
  resolving: { label: "要解衝突", tone: "live", means: "它的 PR 和整合分支衝突，或落後了，等著處理。" },
  merged: { label: "已合併", tone: "go", means: "審查通過，已經合併。" },
  completed: { label: "不用改", tone: "go", means: "執行者看過之後，發現不用改任何東西。" },
  needs_you: { label: "需要你", tone: "signal", means: "只有你能決定接下來怎麼辦。" },
  paused: { label: "暫停", tone: "muted", means: "有人把它暫停了。" },
  cancelled: { label: "取消了", tone: "muted", means: "工單被關掉，或拿掉了工作標籤，engine 不再做它。" },
};

/** laneMeta is where a lane is, with what resolving does to its pull request. */
export function laneMeta(lane: Pick<Lane, "state" | "resolve">): StageMeta {
  if (lane.state === "resolving" && lane.resolve === "conflict")
    return { label: "PR 有衝突", tone: "live", means: "它的 PR 和整合分支衝突，等著解。" };
  if (lane.state === "resolving" && lane.resolve === "rebase")
    return { label: "PR 落後了", tone: "live", means: "整合分支有了新的改動，它的 PR 要先更新。" };
  if (lane.state === "implementing" && lane.resolve === "conflict")
    return { label: "解衝突中", tone: "live", means: "正在解它的 PR 和整合分支的衝突。" };
  if (lane.state === "implementing" && lane.resolve === "rebase")
    return { label: "更新 PR 中", tone: "live", means: "正在把它的 PR 更新到整合分支最新的樣子。" };
  return LANES[lane.state] ?? { label: lane.state, tone: "muted", means: "" };
}

/** Why a lane needs a person. */
export const WHY: Record<string, string> = {
  worker_failed: "執行者試了幾次都失敗。",
  needs_human_review: "流程把它交給你。",
  review_max_retries: "審查一直沒做成，可以再審的次數用完了。",
  conflict_max_retries: "PR 的衝突解了幾次都沒解掉。",
  no_pr: "找不到它的 PR。",
  merge_blocked: "GitHub 不讓它合併（分支保護，或必要的檢查沒過）。",
  guard_blocked: "防護擋下了要給 AI 讀的內容。",
  guard_output: "防護擋下了要寫到 GitHub 的內容。",
  invalid_ticket: "工單裡的 awk-ticket 區塊讀不懂。",
  dependency_cycle: "它依賴的工單繞成了一圈。",
  person: "有人在 GitHub 上把它標成要人處理。",
  agent_refused: "設定要 AI 遵守的限制，那個 AI 工具做不到，所以沒有執行。",
  worker_lost: "工單上寫著執行者在做，但過了時限都沒有結果。",
};

export function whyWords(why?: string): string {
  if (!why) return "";
  return WHY[why] ?? why;
}

/** stepWords says what the engine does in a step, with the ticket's title when it is known: "" for a step that ends the run. */
export function stepWords(step: EngineStep, title?: string): string {
  const ticket = `工單 #${step.issue ?? "?"}${title ? `「${title}」` : ""}`;
  // 「#5 的」 is spaced as a number is; 「」的」 is not.
  const pr = `${title ? "" : " "}的 PR #${step.pr ?? "?"}`;
  switch (step.action) {
    case "review_pr":
      return `審查者正在審${ticket}${pr}`;
    case "dispatch_worker":
      if (step.merge === "conflict") return `正在解${ticket}${pr} 和整合分支的衝突`;
      if (step.merge === "rebase") return `正在把${ticket}${pr} 更新到整合分支最新的樣子`;
      return `執行者正在寫${ticket}`;
    case "check_result":
      return `執行者正在寫${ticket}`;
    default:
      return "";
  }
}

/** How a run of the engine ended, by the reason it ended with. */
const ENDED: Record<string, string> = {
  all_tasks_complete: "上一輪做完了：沒有可以做的工單了。",
  user_stopped: "上一輪照你的要求停了。",
  interrupted: "上一輪被中斷了。",
  needs_human_review: "上一輪停了：有工單需要你。",
  escalation_triggered: "上一輪停了：有工單需要你。",
  review_max_retries: "上一輪停了：有張工單的審查一直沒做成，次數用完了。",
  worker_failed: "上一輪停了：有張工單的執行者試了幾次都失敗。",
  max_failures: "上一輪停了：有張工單的執行者試了幾次都失敗。",
  invalid_ticket: "上一輪停了：有張工單的內容讀不懂。",
  blocked_by_dependencies: "上一輪停了：開著的工單都在等還沒做完的工單。",
  no_actionable_tasks: "上一輪停了：開著的工單都不能做。",
  max_loop_reached: "上一輪停了：到了一輪的次數上限，可能有張工單一直在繞圈。",
  max_consecutive_failures: "上一輪停了：連續失敗太多次。",
  config_not_found: "上一輪停了：讀不到專案的設定。",
  config_invalid: "上一輪停了：專案的設定不能用。",
  github_api_error: "上一輪停了：問不到 GitHub。",
  error_exit: "上一輪出錯停下來了。看下面的輸出和活動紀錄，找出原因。",
};

export function endedWords(reason: string): string {
  return ENDED[reason] ?? `上一輪結束了（${reason}）。`;
}

/** Why a decision stops, as the shadow's differences say it. */
const STOPS: Record<string, string> = {
  needs_human_review: "等人處理",
  worker_failed: "執行者失敗的次數用完了",
  review_max_retries: "審查的次數用完了",
};

/** decisionWords says what one side of the shadow decided. */
export function decisionWords(d: Decision | null): string {
  if (!d) return "手上沒有票，接著挑下一張";
  const ticket = `工單 #${d.issue_number}`;
  const pr = `PR #${d.pr_number}`;
  switch (d.next_action) {
    case "review_pr":
      return `審查${ticket} 的 ${pr}`;
    case "dispatch_worker":
      if (d.merge_issue === "conflict") return `解${ticket} 的 ${pr} 的衝突`;
      if (d.merge_issue === "rebase") return `把${ticket} 的 ${pr} 更新到整合分支`;
      return `把${ticket} 交給執行者`;
    case "check_result":
      return `看${ticket} 的執行者做完了沒`;
    case "all_complete":
      return "全部做完了";
    case "none": {
      const why = d.exit_reason ? `停下來，${STOPS[d.exit_reason] ?? d.exit_reason}` : "停下來";
      return d.issue_number ? `${ticket}：${why}` : why;
    }
    default:
      return d.issue_number ? `${d.next_action} ${ticket}` : d.next_action;
  }
}

/** What moved a lane, as a transition of the activity says it. */
export const CAUSES: Record<string, string> = {
  opened: "開始追蹤",
  imported: "從工單的標籤接手",
  unblocked: "等的工單合併了",
  blocked: "等的工單又打開了",
  dispatched: "交給執行者",
  implemented: "PR 開好了",
  no_changes: "執行者發現不用改",
  implement_failed: "執行者沒做成",
  resolve_failed: "衝突沒解成",
  checked: "檢查通過",
  check_failed: "檢查沒過",
  approved: "審查通過",
  changes_requested: "審查要求修改",
  review_failed: "審查沒做成",
  review_retried: "再審一次",
  conflicting: "和整合分支衝突或落後",
  resolved: "衝突解好了",
  approval_needed: "要你核可",
  owner_approved: "你核可了",
  owner_returned: "你退回了",
  merged: "合併了",
  landed: "改動已經進了整合分支",
  escalated: "交給你",
  released: "你放行了",
  paused: "暫停",
  resumed: "繼續",
  cancelled: "取消",
  reopened: "工單重新打開",
  prioritized: "調了優先順序",
};

/** How a run of the engine ended, in a line of the activity. */
const RUN_ENDS: Record<string, string> = {
  all_tasks_complete: "工單都做完了",
  user_stopped: "照你的要求停了",
  interrupted: "被中斷了",
  needs_human_review: "有工單需要你",
  worker_failed: "有張工單的執行者試了幾次都失敗",
  review_max_retries: "有張工單的審查次數用完了",
};

export function runEndWords(reason: string): string {
  return RUN_ENDS[reason] ?? reason;
}

/** Who gave the engine a command. */
export function byWords(by: string): string {
  if (by === "the workbench") return "工作台";
  return by;
}

/** The kinds of command the engine takes. */
export const COMMANDS: Record<string, string> = {
  stop: "收尾後停",
  stop_now: "立刻停",
};

/** The roles of the agents the engine runs, by who the activity says did it. */
export const ROLES: Record<string, string> = {
  reviewer: "審查",
  resolver: "解衝突",
  implementer: "執行者",
};

/** The differences between the two the comparison knows the reason of. */
export const KNOWN: Record<string, string> = {
  order: "兩張工單的狀態一樣，兩邊先挑了不同的一張",
  check_first: "engine 先看執行者有沒有留下結果，主控直接再派一次",
};

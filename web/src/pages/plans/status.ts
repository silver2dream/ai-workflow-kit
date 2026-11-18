import type { PlanView } from "@/lib/api";
import { MEETING } from "@/lib/council";
import type { Tone } from "@/lib/words";

/** headline is the first line of a brief, short enough for a heading. */
export function headline(brief: string, fallback: string): string {
  const first =
    brief
      .split("\n")
      .map((l) => l.replace(/^#+\s*/, "").trim())
      .find(Boolean) ?? "";
  if (!first) return fallback;
  return first.length > 60 ? `${first.slice(0, 58)}…` : first;
}

export interface PlanStatus {
  label: string;
  tone: Tone;
  /** Whether the plan waits for its owner. */
  needsYou: boolean;
  /** What the owner is to do, when it waits for them. */
  todo?: string;
}

/** planStatus says where a plan is, as its owner sees it: what it waits for first. */
export function planStatus(plan: Pick<PlanView, "meeting" | "tickets">): PlanStatus {
  const state = plan.meeting?.state;
  const waiting = plan.tickets.filter((t) => !t.issue).length;
  switch (state) {
    case "meeting":
    case "paused":
      return { label: MEETING[state].label, tone: MEETING[state].tone, needsYou: false };
    case "agenda":
      return { label: MEETING.agenda.label, tone: "signal", needsYou: true, todo: "主持人排好了議程，等你看過再開始。" };
    case "questions":
      return { label: MEETING.questions.label, tone: "signal", needsYou: true, todo: `議會有 ${plan.meeting?.questions || "幾"} 個問題要問你，答完才開始討論。` };
    case "failed":
      return { label: MEETING.failed.label, tone: "stop", needsYou: true, todo: "會議遇到錯誤停下來了，看看原因再接著開。" };
    case "stopped":
      return { label: MEETING.stopped.label, tone: "caution", needsYou: true, todo: "開會的程序不在了，按「接著開會」從停的地方繼續。" };
  }
  if (plan.tickets.length > 0 && waiting === 0) return { label: "已核可", tone: "go", needsYou: false };
  if (waiting > 0) return { label: "等你核可", tone: "signal", needsYou: true, todo: `${waiting} 張工單等你核可，核可後才會建到 GitHub。` };
  if (state === "finished") return { label: MEETING.finished.label, tone: "go", needsYou: false };
  return { label: "準備中", tone: "muted", needsYou: false };
}

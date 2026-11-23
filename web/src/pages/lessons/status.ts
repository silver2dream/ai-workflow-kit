import type { LessonStatus, LessonView } from "@/lib/api";
import type { Tone } from "@/lib/words";

export interface LessonStatusMeta {
  label: string;
  tone: Tone;
  /** What the status means, for a person who sees it the first time. */
  means: string;
}

export const LESSON_STATUS: Record<LessonStatus, LessonStatusMeta> = {
  quarantined: { label: "隔離中", tone: "signal", means: "給 AI 看之前的檢查擋下了它，等你讀過再決定。" },
  candidate: { label: "試用中", tone: "muted", means: "新的教訓，一次最多給一個執行者試用，標明「未驗證」。" },
  active: { label: "有效", tone: "live", means: "試用之後幫上了忙，相關的執行者都會看到。" },
  proven: { label: "已證實", tone: "go", means: "幫上忙很多次，可以考慮變成固定的規則。" },
  retired: { label: "已停用", tone: "muted", means: "不再給任何 AI：沒擋住同樣的問題、太久沒用到，或你不採用。" },
};

/** Who wrote a lesson, said plainly. */
export function sourceOf(source: string): string {
  switch (source) {
    case "human":
      return "你自己加的";
    case "human-correction":
      return "AI 從有寫入權限的人在 PR 上的糾正寫的";
    default:
      return "AI 從審查的經過寫的";
  }
}

/** A piece of a lesson's text: what it says, or a character a person cannot see. */
export interface TextPart {
  text: string;
  hidden: boolean;
}

/** splitHidden finds the <U+200B> marks the server puts for characters a person cannot see. */
export function splitHidden(text: string): TextPart[] {
  return text
    .split(/(<U\+[0-9A-F]{4,6}>)/)
    .filter((part) => part !== "")
    .map((part) => ({ text: part, hidden: /^<U\+[0-9A-F]{4,6}>$/.test(part) }));
}

export interface LessonGroups {
  held: LessonView[];
  inUse: LessonView[];
  retired: LessonView[];
}

/** group sorts lessons the way the page shows them; the server already orders each kind. */
export function group(lessons: LessonView[]): LessonGroups {
  return {
    held: lessons.filter((l) => l.status === "quarantined"),
    inUse: lessons.filter((l) => l.status === "proven" || l.status === "active" || l.status === "candidate"),
    retired: lessons.filter((l) => l.status === "retired"),
  };
}

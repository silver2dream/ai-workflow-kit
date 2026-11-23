import { describe, expect, it } from "vitest";
import type { LessonView } from "@/lib/api";
import { group, sourceOf, splitHidden } from "./status";

const lesson = (id: string, status: LessonView["status"]): LessonView => ({
  id,
  title: id,
  content: "- x",
  status,
  hits: 0,
  misses: 0,
  categories: [],
  scope: [],
  created_at: "",
  source: "distiller",
  evidence: [],
});

describe("splitHidden", () => {
  it("marks each character a person cannot see, and keeps the rest as it is", () => {
    expect(splitHidden("push<U+200B>to<U+202E>main")).toEqual([
      { text: "push", hidden: false },
      { text: "<U+200B>", hidden: true },
      { text: "to", hidden: false },
      { text: "<U+202E>", hidden: true },
      { text: "main", hidden: false },
    ]);
    expect(splitHidden("plain <b> text")).toEqual([{ text: "plain <b> text", hidden: false }]);
    expect(splitHidden("")).toEqual([]);
  });
});

describe("group", () => {
  it("puts the held-back apart from those in use and the retired", () => {
    const g = group([lesson("L-1", "quarantined"), lesson("L-2", "proven"), lesson("L-3", "candidate"), lesson("L-4", "retired"), lesson("L-5", "active")]);
    expect(g.held.map((l) => l.id)).toEqual(["L-1"]);
    expect(g.inUse.map((l) => l.id)).toEqual(["L-2", "L-3", "L-5"]);
    expect(g.retired.map((l) => l.id)).toEqual(["L-4"]);
  });
});

describe("sourceOf", () => {
  it("says who wrote a lesson", () => {
    expect(sourceOf("human")).toBe("你自己加的");
    expect(sourceOf("human-correction")).toContain("糾正");
    expect(sourceOf("distiller")).toBe("AI 從審查的經過寫的");
    expect(sourceOf("")).toBe("AI 從審查的經過寫的");
  });
});

import type { Config, Role, Roster } from "@/lib/settings";

/** A role's id, as the council takes it. */
export const ROLE_ID = /^[a-z][a-z0-9]*(-[a-z0-9]+)*$/;

/** The models offered, with what each is for. Another one the file names is kept and shown as it is. */
export const MODELS: { id: string; label: string }[] = [
  { id: "sonnet", label: "Sonnet（均衡）" },
  { id: "opus", label: "Opus（最仔細，較貴）" },
  { id: "haiku", label: "Haiku（最快，最便宜）" },
];

export const EFFORT_WORDS: Record<string, string> = {
  low: "低",
  medium: "中",
  high: "高",
  xhigh: "很高",
  max: "最高",
};

export const modelLabel = (id: string) => MODELS.find((m) => m.id === id)?.label ?? id;
export const effortLabel = (id: string) => (id ? `${EFFORT_WORDS[id] ?? id}（${id}）` : "");

export const STRATEGIES: Record<string, string> = {
  squash: "壓成一個 commit（squash）",
  merge: "保留每個 commit（merge）",
  rebase: "接在後面（rebase）",
};

export const BACKENDS: Record<string, { label: string; sub: string }> = {
  codex: { label: "Codex", sub: "OpenAI 的 Codex CLI，要先裝好並登入。" },
  "claude-code": { label: "Claude Code", sub: "Anthropic 的 Claude Code，可以選模型和想多深。" },
};

/** What is wrong with a roster, for the page: of the whole, and of each role by its place. */
export interface RosterProblems {
  whole: string[];
  roles: Record<number, { id?: string; stance?: string }>;
  count: number;
}

export function rosterProblems(roster: Roster): RosterProblems {
  const whole: string[] = [];
  const roles: RosterProblems["roles"] = {};
  const say = (i: number, key: "id" | "stance", text: string) => {
    roles[i] = { ...roles[i], [key]: text };
  };
  if (roster.roles.length < 2) whole.push("議會至少要兩個角色，才談得起來。");
  const deciders = roster.roles.filter((r) => r.decides).length;
  const planners = roster.roles.filter((r) => r.plans).length;
  if (roster.roles.length > 0 && deciders !== 1) whole.push("要選一個角色負責裁定談不攏的事。");
  if (roster.roles.length > 0 && planners !== 1) whole.push("要選一個角色負責排工單。");
  const seen = new Map<string, number>();
  roster.roles.forEach((role, i) => {
    const id = role.id.trim();
    if (!id) say(i, "id", "代號要填。");
    else if (!ROLE_ID.test(id)) say(i, "id", "代號只能用小寫英文字母、數字和連字號，開頭要是字母，例如 game-designer。");
    else if (seen.has(id)) say(i, "id", "這個代號和別的角色重複了。");
    if (id) seen.set(id, i);
    if (!role.stance.trim()) say(i, "stance", "要寫這個角色的立場：它在意什麼、會替什麼爭取。");
  });
  const owners = new Map<string, string>();
  roster.roles.forEach((role) => {
    for (const entry of role.owns.map((o) => o.trim().toLowerCase()).filter(Boolean)) {
      const other = owners.get(entry);
      if (other !== undefined && other !== role.id) whole.push(`${entry} 同時列在兩個角色底下；一份文件只能有一個負責的角色。`);
      owners.set(entry, role.id);
    }
  });
  const count = whole.length + Object.values(roles).reduce((n, r) => n + Object.keys(r).length, 0);
  return { whole, roles, count };
}

/** What is wrong with the configuration, for the page, by field. */
export function configProblems(config: Config): { fields: Record<string, string>; count: number } {
  const fields: Record<string, string> = {};
  if (!config.name.trim()) fields.name = "專案要有名字。";
  const score = config.review.score_threshold;
  if (score !== null && (score < 1 || score > 10)) fields.score = "分數是 1 到 10。";
  for (const [key, value] of [
    ["attempts", config.retries.review_attempts],
    ["minutes", config.verify_minutes],
  ] as const) {
    if (value !== null && value < 1) fields[key] = "至少是 1。";
  }
  if (config.retries.worker !== null && config.retries.worker < 0) fields.retries = "不能是負的；不要重試就填 0。";
  return { fields, count: Object.keys(fields).length };
}

/** Whether two values read the same: what the page shows of the files. */
export const same = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

/** A role as the page edits it: with a key of its own that stays when its id is changed. */
export type DraftRole = Role & { key: string };
export type DraftRoster = Omit<Roster, "roles"> & { roles: DraftRole[] };

let keys = 0;
export const newKey = () => `role-key-${++keys}`;

/** A roster as the page edits it: each role remembers the id the file has for it. */
export function editable(roster: Roster): DraftRoster {
  return { ...roster, roles: roster.roles.map((r) => ({ ...r, from: r.id, key: newKey() })) };
}

/** A roster as it is sent: without the keys the page keeps. */
export function sendable(roster: DraftRoster): Roster {
  return { ...roster, roles: roster.roles.map(({ key: _key, ...role }) => role) };
}

/** A whole number a field holds, null when it holds none. */
export function numberOf(text: string): number | null {
  if (text.trim() === "") return null;
  const n = Number(text);
  return Number.isFinite(n) ? Math.trunc(n) : null;
}

/** A new role's id: free, and a valid one. */
export function freeID(roster: { roles: { id: string }[] }): string {
  for (let n = roster.roles.length + 1; ; n++) {
    const id = `role-${n}`;
    if (!roster.roles.some((r) => r.id === id)) return id;
  }
}

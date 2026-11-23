import { useEffect, useRef, useState } from "react";
import { ArrowDown, ArrowUp, Plus, Trash2, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Panel, PanelHead } from "@/components/bits";
import type { Defaults } from "@/lib/settings";
import { cn } from "@/lib/utils";
import { LINE, RoleBadge } from "@/pages/plans/roles";
import { EffortSelect, Field, ModelSelect, NumberField, TextField } from "./fields";
import { effortLabel, freeID, modelLabel, newKey, type DraftRole, type DraftRoster, type RosterProblems } from "./model";

const LIMITS: { key: keyof DraftRoster["limits"]; label: string; hint: string; unit?: string }[] = [
  { key: "topics", label: "議題最多幾個", hint: "議程上的題目數。題目越多，會開越久、花越多。" },
  { key: "disputes", label: "每題對辯幾個爭點", hint: "其餘的爭點直接交給負責裁定的角色決定。" },
  { key: "dispute_rounds", label: "每個爭點來回幾次", hint: "兩邊各說幾輪，才交給裁定。" },
  { key: "calls", label: "一場最多呼叫模型幾次", hint: "到了就停下來，避免花太多。" },
  { key: "parallel", label: "同時呼叫幾個", hint: "越多越快，但也越容易碰到用量限制。" },
  { key: "timeout_seconds", label: "一次呼叫最多等多久", hint: "要寫兩份文件的角色會答很久。", unit: "秒" },
];

/** OwnsEditor lists what only a role writes, and takes a path more. */
function OwnsEditor({ role, onChange, label, explain }: { role: DraftRole; onChange: (owns: string[]) => void; label: string; explain: boolean }) {
  const [adding, setAdding] = useState("");
  const add = () => {
    const path = adding.trim();
    if (!path) return;
    if (!role.owns.includes(path)) onChange([...role.owns, path]);
    setAdding("");
  };
  return (
    <Field label="負責寫的文件" hint={explain ? "從專案根目錄寫起。資料夾結尾加 /（例如 docs/design/），角色可以在裡面寫、也可以開新檔；也可以是一份檔案（docs/qa/risks.md）。" : undefined}>
      {(id, describedBy) => (
        <div className="grid gap-2">
          {role.owns.length > 0 && (
            <ul className="flex flex-wrap gap-1.5" aria-label={`${label}負責寫的文件`}>
              {role.owns.map((path) => (
                <li key={path} className="flex max-w-full items-center gap-1 rounded-md bg-muted py-0.5 pl-2 pr-0.5 ring-1 ring-inset ring-border">
                  <span className="min-w-0 break-all font-mono text-[12.5px]">{path}</span>
                  <button
                    type="button"
                    aria-label={`不再負責 ${path}`}
                    onClick={() => onChange(role.owns.filter((p) => p !== path))}
                    className="grid size-6 shrink-0 cursor-pointer place-items-center rounded hover:bg-card"
                  >
                    <X className="size-3.5" aria-hidden />
                  </button>
                </li>
              ))}
            </ul>
          )}
          <div className="flex gap-2">
            <Input
              id={id}
              value={adding}
              placeholder="docs/design/"
              aria-describedby={describedBy}
              onChange={(e) => setAdding(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  add();
                }
              }}
              className="h-9 font-mono text-[13px]"
            />
            <Button type="button" variant="outline" onClick={add} disabled={!adding.trim()}>
              <Plus aria-hidden /> 加
            </Button>
          </div>
        </div>
      )}
    </Field>
  );
}

/** RosterForm edits who sits at the council's table, and how far a plan may go. */
export function RosterForm({
  roster,
  onChange,
  defaults,
  problems,
}: {
  roster: DraftRoster;
  onChange: (roster: DraftRoster) => void;
  defaults: Defaults;
  problems: RosterProblems;
}) {
  const refocus = useRef<string | null>(null);
  useEffect(() => {
    if (!refocus.current) return;
    const target = document.querySelector<HTMLElement>(refocus.current);
    refocus.current = null;
    if (target && !(target as HTMLButtonElement).disabled) target.focus();
  });

  const setRole = (i: number, change: (role: DraftRole) => DraftRole) => onChange({ ...roster, roles: roster.roles.map((r, j) => (j === i ? change(r) : r)) });
  const only = (key: "decides" | "plans", i: number) => onChange({ ...roster, roles: roster.roles.map((r, j) => ({ ...r, [key]: j === i })) });
  const move = (i: number, to: number, button: "up" | "down") => {
    const roles = [...roster.roles];
    const [role] = roles.splice(i, 1);
    roles.splice(to, 0, role);
    refocus.current = `[data-role="${role.key}"] [data-move="${button}"]`;
    onChange({ ...roster, roles });
  };
  const rosterModel = roster.model || defaults.roster.model;
  const rosterEffort = roster.effort ? effortLabel(roster.effort) : "Claude CLI 自己的預設";

  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-5">
      <Panel>
        <PanelHead title="整場會議" sub="每個角色沒有另外指定時，都用這裡的。" />
        <div className="grid gap-4 px-5 pb-5 sm:grid-cols-3">
          <TextField label="寫文件用的語言" value={roster.language} placeholder="跟需求同一種語言" onChange={(language) => onChange({ ...roster, language })} hint="例如「繁體中文」。不填就跟你寫的需求一樣。" />
          <ModelSelect label="模型" value={roster.model} inherit={`預設（${modelLabel(defaults.roster.model)}）`} onChange={(model) => onChange({ ...roster, model })} />
          <EffortSelect
            label="想多深"
            value={roster.effort}
            inherit="預設（Claude CLI 自己決定）"
            efforts={defaults.roster.efforts}
            onChange={(effort) => onChange({ ...roster, effort })}
            hint="越高越仔細，也越慢、越花錢。"
          />
        </div>
      </Panel>

      <Panel data-tour="settings-roles">
        <PanelHead title={`角色（${roster.roles.length}）`} sub="照這個順序發言。一個角色負責裁定談不攏的事，一個負責排工單。" />
        {problems.whole.length > 0 && (
          <ul role="alert" className="mx-5 mb-3 grid gap-1 rounded-lg border-l-4 border-destructive bg-destructive/5 px-4 py-2.5 text-[13.5px] font-bold text-destructive">
            {problems.whole.map((p) => (
              <li key={p}>{p}</li>
            ))}
          </ul>
        )}
        <ol className="grid grid-cols-[minmax(0,1fr)] gap-3 px-5 pb-4">
          {roster.roles.map((role, i) => {
            const seat = { id: role.id, title: role.title || role.id || "新角色", color: LINE[i % LINE.length] };
            const name = role.title || role.id || `第 ${i + 1} 個角色`;
            const wrong = problems.roles[i] ?? {};
            // What a field is for is said once, on the first role.
            const explain = i === 0;
            return (
              <li key={role.key} data-role={role.key} className="min-w-0 rounded-lg border border-border bg-background p-4">
                <div className="flex items-center gap-3">
                  <RoleBadge seat={seat} size={34} />
                  <Input
                    aria-label={`${name}的名稱`}
                    value={role.title}
                    placeholder="角色名稱，例如 遊戲設計"
                    onChange={(e) => setRole(i, (r) => ({ ...r, title: e.target.value }))}
                    className="h-9 min-w-0 flex-1 text-[15.5px] font-extrabold"
                  />
                  <span className="flex shrink-0 items-center">
                    <Button type="button" variant="ghost" size="icon" data-move="up" aria-label={`${name}早一點發言`} disabled={i === 0} onClick={() => move(i, i - 1, "up")}>
                      <ArrowUp aria-hidden />
                    </Button>
                    <Button type="button" variant="ghost" size="icon" data-move="down" aria-label={`${name}晚一點發言`} disabled={i === roster.roles.length - 1} onClick={() => move(i, i + 1, "down")}>
                      <ArrowDown aria-hidden />
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="icon"
                      aria-label={`刪掉${name}`}
                      onClick={() => {
                        const near = roster.roles[i + 1] ?? roster.roles[i - 1];
                        refocus.current = near ? `[data-role="${near.key}"] input` : '[data-add-role]';
                        onChange({ ...roster, roles: roster.roles.filter((_, j) => j !== i) });
                      }}
                    >
                      <Trash2 aria-hidden />
                    </Button>
                  </span>
                </div>

                <div className="mt-4 grid gap-4 sm:grid-cols-2">
                  <TextField
                    label="代號"
                    mono
                    value={role.id}
                    error={wrong.id}
                    onChange={(id) => setRole(i, (r) => ({ ...r, id }))}
                    hint={explain ? "寫在檔案裡、也是其他角色提到它時用的名字。小寫英文和連字號。" : undefined}
                  />
                  <fieldset className="grid content-start gap-1.5">
                    <legend className="mb-1 text-[13px] font-bold text-muted-foreground">這個角色負責</legend>
                    <label className="flex cursor-pointer items-center gap-2 text-[14px]">
                      <input type="radio" name="decides" checked={role.decides} onChange={() => only("decides", i)} className="size-4 accent-[var(--foreground)]" />
                      裁定談不攏的事
                    </label>
                    <label className="flex cursor-pointer items-center gap-2 text-[14px]">
                      <input type="radio" name="plans" checked={role.plans} onChange={() => only("plans", i)} className="size-4 accent-[var(--foreground)]" />
                      排工單
                    </label>
                  </fieldset>
                  <ModelSelect label="模型" value={role.model} inherit={`跟整場一樣（${modelLabel(rosterModel)}）`} onChange={(model) => setRole(i, (r) => ({ ...r, model }))} />
                  <EffortSelect label="想多深" value={role.effort} inherit={`跟整場一樣（${rosterEffort}）`} efforts={defaults.roster.efforts} onChange={(effort) => setRole(i, (r) => ({ ...r, effort }))} />
                  <Field label="立場" error={wrong.stance} className="sm:col-span-2" hint={explain ? "它替什麼爭取、反對什麼、在意什麼。議會照這段話扮演它。" : undefined}>
                    {(id, describedBy) => (
                      <Textarea
                        id={id}
                        rows={4}
                        value={role.stance}
                        aria-invalid={wrong.stance ? true : undefined}
                        aria-describedby={describedBy}
                        onChange={(e) => setRole(i, (r) => ({ ...r, stance: e.target.value }))}
                        className="text-[14px] leading-relaxed"
                      />
                    )}
                  </Field>
                  <div className="sm:col-span-2">
                    <OwnsEditor role={role} label={name} explain={explain} onChange={(owns) => setRole(i, (r) => ({ ...r, owns }))} />
                  </div>
                </div>
              </li>
            );
          })}
        </ol>
        <div className="border-t border-border px-5 py-3">
          <Button
            type="button"
            variant="outline"
            data-add-role
            onClick={() => {
              const key = newKey();
              refocus.current = `[data-role="${key}"] input`;
              onChange({
                ...roster,
                roles: [...roster.roles, { key, id: freeID(roster), title: "", model: "", effort: "", stance: "", owns: [], decides: false, plans: false }],
              });
            }}
          >
            <Plus aria-hidden /> 加一個角色
          </Button>
        </div>
      </Panel>

      <Panel>
        <PanelHead title="一場會議的上限" sub="不填就用預設。這些數字決定一場會最多花多少。" />
        <div className={cn("grid gap-4 px-5 pb-5 sm:grid-cols-2 lg:grid-cols-3")}>
          {LIMITS.map((limit) => (
            <NumberField
              key={limit.key}
              label={limit.label}
              hint={limit.hint}
              unit={limit.unit}
              value={roster.limits[limit.key]}
              fallback={defaults.roster.limits[limit.key] ?? 0}
              onChange={(value) => onChange({ ...roster, limits: { ...roster.limits, [limit.key]: value } })}
            />
          ))}
        </div>
      </Panel>
    </div>
  );
}

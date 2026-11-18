import { useEffect, useMemo, useRef, useState } from "react";
import { ArrowDown, ArrowUp, Lock, Play, Plus, Trash2, X } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Panel, PanelHead, Problem } from "@/components/bits";
import { useProject } from "@/app/project";
import { council, type PlanDetail, type Topic } from "@/lib/council";
import { OneAtATime } from "@/pages/plans/OneAtATime";
import { RoleBadge, useSeats } from "@/pages/plans/roles";
import { useOtherMeeting } from "@/pages/plans/usePlan";

function move<T>(list: T[], from: number, to: number): T[] {
  if (to < 0 || to >= list.length) return list;
  const next = [...list];
  const [item] = next.splice(from, 1);
  next.splice(to, 0, item);
  return next;
}

/**
 * AgendaEditor lets the owner look at the agenda before the first topic and
 * change it: what each topic settles, who leads, who speaks and in which
 * order, which topics there are and in which order, and the questions only
 * the owner can answer. A topic that was taken up stays as it was. The
 * meeting checks the agenda before it holds it, and says what does not fit.
 */
export function AgendaEditor({
  detail,
  onDone,
  onCancel,
  askQuestions = true,
}: {
  detail: PlanDetail;
  onDone: () => void;
  /** Leave the agenda as it is: given where the editor stands in for another view. */
  onCancel?: () => void;
  /** Whether the questions to the owner are still to be asked, and so can be changed. */
  askQuestions?: boolean;
}) {
  const { project, roster } = useProject();
  const seat = useSeats();
  const roles = roster.data?.roles ?? [];
  const [topics, setTopics] = useState<Topic[]>(detail.agenda?.topics ?? []);
  const [questions, setQuestions] = useState<string[]>(detail.agenda?.questions ?? []);
  const [problem, setProblem] = useState<{ title: string; text: string } | null>(null);
  const other = useOtherMeeting(detail.id);
  const [busy, setBusy] = useState(false);
  // What the person changes stays while the page asks the plan again; the
  // page gives the editor a new key when the agenda itself changes.
  const started = useMemo(() => new Set((detail.record?.topics ?? []).filter((t) => t.started).map((t) => t.topic.id)), [detail.record]);

  const change = (i: number, edit: (t: Topic) => Topic) => setTopics((all) => all.map((t, j) => (j === i ? edit(t) : t)));

  // Moving a topic or a speaker moves its element, and a removed control
  // takes the focus with it: the focus is put back on the first of these
  // that is there and does something, so a keyboard keeps its place.
  const refocus = useRef<string[] | null>(null);
  useEffect(() => {
    const wanted = refocus.current;
    refocus.current = null;
    for (const selector of (wanted ?? []).filter(Boolean)) {
      const el = document.querySelector<HTMLElement>(selector);
      if (el && !(el as HTMLButtonElement).disabled) {
        el.focus();
        return;
      }
    }
  });
  const inTopic = (id: string | undefined, control: string) => (id ? `[data-topic="${CSS.escape(id)}"] ${control}` : "");

  async function save(goOn: boolean) {
    setBusy(true);
    setProblem(null);
    const asked = askQuestions ? questions.map((q) => q.trim()).filter(Boolean) : (detail.agenda?.questions ?? []);
    try {
      await council.saveAgenda(project.id, detail.id, asked, topics);
    } catch (e) {
      setProblem({ title: "議程還不能用", text: (e as Error).message });
      setBusy(false);
      return;
    }
    try {
      if (goOn) {
        await council.goOn(project.id, detail.id);
        toast.success("照這個議程開會了");
      } else {
        toast.success("議程存好了");
      }
      onDone();
    } catch (e) {
      setProblem({ title: "議程存好了，但會議開不起來", text: (e as Error).message });
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="grid gap-5" data-tour="agenda">
      <Panel>
        <PanelHead title="議程" sub="主持人排好的議題，照順序一題一題談。可以改每一題要回答什麼、誰主講、誰發言和順序，也可以刪題、調順序。" />
        <ol className="grid gap-3 px-5 pb-5">
          {topics.map((topic, i) => {
            const locked = started.has(topic.id);
            const others = roles.filter((r) => r.id !== topic.lead && !topic.speakers.includes(r.id));
            return (
              <li key={topic.id} data-topic={topic.id} className="rounded-lg border border-border p-4">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="grid size-7 place-items-center rounded-md bg-muted text-[13px] font-black ring-1 ring-inset ring-border tnum">{i + 1}</span>
                  {locked ? (
                    <span className="flex items-center gap-1.5 text-[13px] font-bold text-muted-foreground">
                      <Lock className="size-3.5" aria-hidden />
                      已經談過，維持原樣
                    </span>
                  ) : (
                    <span className="ml-auto flex gap-1">
                      <Button
                        size="icon"
                        variant="ghost"
                        className="size-8"
                        data-move="up"
                        aria-label={`第 ${i + 1} 題往前`}
                        disabled={i === 0}
                        onClick={() => {
                          refocus.current = [inTopic(topic.id, '[data-move="up"]'), inTopic(topic.id, '[data-move="down"]')];
                          setTopics((all) => move(all, i, i - 1));
                        }}
                      >
                        <ArrowUp aria-hidden />
                      </Button>
                      <Button
                        size="icon"
                        variant="ghost"
                        className="size-8"
                        data-move="down"
                        aria-label={`第 ${i + 1} 題往後`}
                        disabled={i === topics.length - 1}
                        onClick={() => {
                          refocus.current = [inTopic(topic.id, '[data-move="down"]'), inTopic(topic.id, '[data-move="up"]')];
                          setTopics((all) => move(all, i, i + 1));
                        }}
                      >
                        <ArrowDown aria-hidden />
                      </Button>
                      <Button
                        size="icon"
                        variant="ghost"
                        className="size-8"
                        data-move="drop"
                        aria-label={`刪掉第 ${i + 1} 題`}
                        disabled={topics.length === 1}
                        onClick={() => {
                          const near = topics[i + 1]?.id ?? topics[i - 1]?.id;
                          refocus.current = [inTopic(near, '[data-move="drop"]'), inTopic(near, "input")];
                          setTopics((all) => all.filter((_, j) => j !== i));
                        }}
                      >
                        <Trash2 aria-hidden />
                      </Button>
                    </span>
                  )}
                </div>
                <div className="mt-3 grid gap-3">
                  <label className="grid gap-1 text-[13px] font-bold text-muted-foreground">
                    議題
                    <Input value={topic.title} disabled={locked} onChange={(e) => change(i, (t) => ({ ...t, title: e.target.value }))} className="text-[15px] font-bold text-foreground" />
                  </label>
                  <label className="grid gap-1 text-[13px] font-bold text-muted-foreground">
                    要回答什麼
                    <Textarea value={topic.question} disabled={locked} rows={2} onChange={(e) => change(i, (t) => ({ ...t, question: e.target.value }))} className="text-[14.5px] text-foreground" />
                  </label>
                  <div className="grid gap-1.5">
                    <span className="text-[13px] font-bold text-muted-foreground">主講，然後依序發言</span>
                    <div className="flex flex-wrap items-center gap-1.5">
                      <select
                        aria-label={`第 ${i + 1} 題的主講`}
                        value={topic.lead}
                        disabled={locked}
                        onChange={(e) => change(i, (t) => ({ ...t, lead: e.target.value, speakers: t.speakers.filter((s) => s !== e.target.value) }))}
                        className="h-8 rounded-md border border-input bg-card px-2 text-[13.5px] font-bold"
                      >
                        {roles.map((r) => (
                          <option key={r.id} value={r.id}>
                            {r.title}（主講）
                          </option>
                        ))}
                      </select>
                      {topic.speakers.map((id, k) => (
                        <span key={id} className="flex items-center gap-1 rounded-md bg-muted py-0.5 pl-1 pr-0.5 text-[13px] font-bold ring-1 ring-inset ring-border">
                          <RoleBadge seat={seat(id)} size={20} />
                          {seat(id).title}
                          {!locked && (
                            <>
                              <button
                                type="button"
                                data-speaker={id}
                                data-move="earlier"
                                aria-label={`${seat(id).title}早一點發言`}
                                disabled={k === 0}
                                onClick={() => {
                                  refocus.current = [inTopic(topic.id, `[data-speaker="${CSS.escape(id)}"][data-move="earlier"]`), inTopic(topic.id, `[data-speaker="${CSS.escape(id)}"][data-move="later"]`)];
                                  change(i, (t) => ({ ...t, speakers: move(t.speakers, k, k - 1) }));
                                }}
                                className="grid size-6 cursor-pointer place-items-center rounded hover:bg-card disabled:cursor-default disabled:opacity-30"
                              >
                                <ArrowUp className="size-3.5 -rotate-90" aria-hidden />
                              </button>
                              <button
                                type="button"
                                data-speaker={id}
                                data-move="later"
                                aria-label={`${seat(id).title}晚一點發言`}
                                disabled={k === topic.speakers.length - 1}
                                onClick={() => {
                                  refocus.current = [inTopic(topic.id, `[data-speaker="${CSS.escape(id)}"][data-move="later"]`), inTopic(topic.id, `[data-speaker="${CSS.escape(id)}"][data-move="earlier"]`)];
                                  change(i, (t) => ({ ...t, speakers: move(t.speakers, k, k + 1) }));
                                }}
                                className="grid size-6 cursor-pointer place-items-center rounded hover:bg-card disabled:cursor-default disabled:opacity-30"
                              >
                                <ArrowDown className="size-3.5 -rotate-90" aria-hidden />
                              </button>
                              <button
                                type="button"
                                aria-label={`${seat(id).title}這題不發言`}
                                onClick={() => {
                                  const near = topic.speakers[k + 1] ?? topic.speakers[k - 1];
                                  refocus.current = [near ? inTopic(topic.id, `[data-speaker="${CSS.escape(near)}"][data-move="earlier"]`) : "", inTopic(topic.id, "select[data-add]")];
                                  change(i, (t) => ({ ...t, speakers: t.speakers.filter((s) => s !== id) }));
                                }}
                                className="grid size-6 cursor-pointer place-items-center rounded hover:bg-card"
                              >
                                <X className="size-3.5" aria-hidden />
                              </button>
                            </>
                          )}
                        </span>
                      ))}
                      {!locked && others.length > 0 && (
                        <select
                          data-add
                          aria-label={`第 ${i + 1} 題加一個發言的角色`}
                          value=""
                          onChange={(e) => e.target.value && change(i, (t) => ({ ...t, speakers: [...t.speakers, e.target.value] }))}
                          className="h-8 rounded-md border border-dashed border-input bg-card px-2 text-[13px] text-muted-foreground"
                        >
                          <option value="">＋ 加一個角色</option>
                          {others.map((r) => (
                            <option key={r.id} value={r.id}>
                              {r.title}
                            </option>
                          ))}
                        </select>
                      )}
                    </div>
                  </div>
                  <div className="text-[13px] text-muted-foreground">
                    寫進：<span className="font-mono text-[12.5px]">{topic.writes.join("、") || "（沒有）"}</span>
                  </div>
                </div>
              </li>
            );
          })}
        </ol>
      </Panel>

      {askQuestions && (
      <Panel>
        <PanelHead title="只有你答得出來的問題" sub="開會前會問你的；不想被問就刪掉，議會會先假設，會後給你確認。" />
        <div className="grid gap-2 px-5 pb-5">
          {questions.map((q, i) => (
            <div key={i} className="flex gap-2">
              <Input data-question={i} aria-label={`問題 ${i + 1}`} value={q} onChange={(e) => setQuestions((all) => all.map((x, j) => (j === i ? e.target.value : x)))} />
              <Button
                size="icon"
                variant="ghost"
                aria-label={`刪掉問題 ${i + 1}`}
                onClick={() => {
                  // The next question takes this one's place; the last gives way to the one before.
                  refocus.current = [`[data-question="${i < questions.length - 1 ? i : i - 1}"]`, "[data-add-question]"];
                  setQuestions((all) => all.filter((_, j) => j !== i));
                }}
              >
                <Trash2 aria-hidden />
              </Button>
            </div>
          ))}
          <Button
            variant="outline"
            className="justify-self-start"
            data-add-question
            onClick={() => {
              refocus.current = [`[data-question="${questions.length}"]`];
              setQuestions((all) => [...all, ""]);
            }}
          >
            <Plus aria-hidden /> 加一個問題
          </Button>
        </div>
      </Panel>
      )}

      {problem && <Problem title={problem.title}>{problem.text}</Problem>}
      {other && <OneAtATime other={other} then="開完再開這一場；議程可以先存起來" />}
      <div className="sticky bottom-0 z-10 -mx-1 flex flex-wrap gap-2 border-t border-border bg-background/95 px-1 py-3 backdrop-blur">
        <Button size="lg" onClick={() => save(true)} disabled={busy || !!other}>
          <Play aria-hidden /> 照這個議程{askQuestions ? "開會" : "接著開會"}
        </Button>
        <Button size="lg" variant="outline" onClick={() => save(false)} disabled={busy}>
          只存起來
        </Button>
        {onCancel && (
          <Button size="lg" variant="ghost" onClick={onCancel} disabled={busy}>
            不改了
          </Button>
        )}
      </div>
    </div>
  );
}

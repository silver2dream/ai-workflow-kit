import { useEffect, useState } from "react";
import { ArrowLeft, CirclePause, Gavel, ListOrdered, Loader2, Play, RefreshCcw, ScrollText, Ticket as TicketIcon, CircleHelp, FileText } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Panel, Problem, Tag } from "@/components/bits";
import { useProject } from "@/app/project";
import { useOnboarding } from "@/onboarding/state";
import { council, MEETING, type PlanDetail } from "@/lib/council";
import { href } from "@/lib/router";
import { when } from "@/lib/words";
import { AgendaEditor } from "@/pages/plans/AgendaEditor";
import { liveTopic, Proceedings, Roundtable, TopicStrip, Usage } from "@/pages/plans/Meeting";
import { Questions } from "@/pages/plans/Questions";
import { AssumptionsTab, DecisionsTab, DocumentsTab, TranscriptTab } from "@/pages/plans/Record";
import { SayBox } from "@/pages/plans/SayBox";
import { headline } from "@/pages/plans/status";
import { TicketsTab } from "@/pages/plans/Tickets";
import { OneAtATime } from "@/pages/plans/OneAtATime";
import { useOtherMeeting, usePlan } from "@/pages/plans/usePlan";


function Back() {
  const { project } = useProject();
  return (
    <a href={href({ project: project.id, page: "plans", rest: [] })} className="inline-flex items-center gap-1.5 text-[14px] font-bold text-muted-foreground hover:text-foreground">
      <ArrowLeft className="size-4" aria-hidden />
      規劃與議會
    </a>
  );
}

function Head({ detail }: { detail: PlanDetail }) {
  const [open, setOpen] = useState(false);
  const state = detail.meeting?.state;
  const info = state ? MEETING[state] : undefined;
  const long = detail.brief.split("\n").filter((l) => l.trim()).length > 1 || detail.brief.length > 60;
  return (
    <div className="mb-6">
      <Back />
      <div className="mt-3 flex flex-wrap items-start gap-3">
        <h1 className="min-w-0 max-w-4xl flex-1 text-[26px] font-black leading-tight tracking-tight">{headline(detail.brief, `規劃 ${detail.id}`)}</h1>
        {info && (
          <Tag tone={info.tone} className="mt-1.5 text-[13.5px]">
            {state === "meeting" && <Loader2 className="size-3.5 animate-spin" aria-hidden />}
            {info.label}
          </Tag>
        )}
      </div>
      <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-[13.5px] text-muted-foreground">
        <span className="font-mono text-[12.5px]">{detail.id}</span>
        {detail.meeting?.updated && <span>最後動靜 {when(detail.meeting.updated)}</span>}
        {long && (
          <button type="button" onClick={() => setOpen((o) => !o)} aria-expanded={open} className="cursor-pointer font-bold text-ring underline-offset-4 hover:underline">
            {open ? "收起需求" : "看完整需求"}
          </button>
        )}
      </div>
      {open && <p className="mt-3 max-w-4xl whitespace-pre-line rounded-lg border border-border bg-card p-4 text-[14.5px] leading-relaxed">{detail.brief}</p>}
    </div>
  );
}

/** Log is the end of what the meeting's process printed: why it stopped, when nothing else says. */
function Log({ lines, open = false }: { lines?: string[]; open?: boolean }) {
  if (!lines || lines.length === 0) return null;
  return (
    <details open={open} className="rounded-lg border border-border bg-card text-[13px]">
      <summary className="cursor-pointer px-4 py-2.5 font-bold text-muted-foreground">awkit 印出的最後幾行</summary>
      <pre className="max-h-72 overflow-auto whitespace-pre-wrap break-all border-t border-border px-4 py-3 font-mono text-[12px] leading-relaxed text-ink2">{lines.join("\n")}</pre>
    </details>
  );
}

/** Banner says what the meeting waits for, in the owner's yellow. */
function Banner({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="mb-5 rounded-xl border border-border border-l-[6px] border-l-signal bg-card px-5 py-4" role="status">
      <div className="text-[16px] font-extrabold">{title}</div>
      <p className="mt-0.5 text-[14.5px] text-ink2">{children}</p>
    </div>
  );
}

function Starting({ detail }: { detail: PlanDetail }) {
  return (
    <div className="grid gap-4">
      <Panel>
        <div className="flex items-center gap-3 p-5" role="status">
          <Loader2 className="size-5 animate-spin text-live" aria-hidden />
          <div>
            <div className="text-[15.5px] font-extrabold">議會入座中</div>
            <p className="text-[14px] text-ink2">通常幾秒內就開始；接著主持人排議程，要一兩分鐘。關掉這個頁面也不會停。</p>
          </div>
        </div>
      </Panel>
      {detail.log && detail.log.length > 0 && (
        <>
          <p className="text-[13.5px] text-muted-foreground">如果一直停在這裡，看看 awkit 印出了什麼：</p>
          <Log lines={detail.log} open />
        </>
      )}
    </div>
  );
}

/** pendingWords are what the owner said that the meeting has not taken in yet. */
function pendingWords(detail: PlanDetail) {
  const taken = new Set((detail.record?.words ?? []).map((w) => w.id));
  return detail.words.filter((w) => !taken.has(w.id));
}

/** Controls say where the meeting is, and let the owner pause it or go on with it. */
function Controls({ detail, onChanged, onEditAgenda }: { detail: PlanDetail; onChanged: () => void; onEditAgenda?: () => void }) {
  const { project } = useProject();
  const meeting = detail.meeting!;
  const info = MEETING[meeting.state];
  const [busy, setBusy] = useState(false);
  const [pausing, setPausing] = useState(false);
  useEffect(() => setPausing(false), [meeting.state]);
  const going = meeting.state === "meeting";
  const waiting = pendingWords(detail);
  const other = useOtherMeeting(detail.id);

  async function act(what: "pause" | "goOn") {
    setBusy(true);
    try {
      if (what === "pause") {
        await council.pause(project.id, detail.id);
        setPausing(true);
        toast.success("會暫停", { description: "正在發言的角色說完這一段就停，說過的都留著。" });
      } else {
        await council.goOn(project.id, detail.id);
        toast.success("接著開會了", { description: waiting.length ? "議會會先照你說的話重審。" : "從停下的地方繼續，說過的不會重來。" });
      }
      onChanged();
    } catch (e) {
      toast.error(what === "pause" ? "停不下來" : "開不起來", { description: (e as Error).message });
    } finally {
      setBusy(false);
    }
  }

  return (
    <Panel className="xl:col-start-2 xl:row-start-1">
      <div className="grid gap-3 p-5">
        <div className="flex items-center gap-2">
          <Tag tone={info.tone} className="text-[13.5px]">
            {going && <Loader2 className="size-3.5 animate-spin" aria-hidden />}
            {pausing ? "要暫停了" : info.label}
          </Tag>
          {meeting.topics > 0 && (
            <span className="ml-auto text-[13px] text-muted-foreground tnum">
              第 {Math.min(Math.max(meeting.topic, 1), meeting.topics)} / {meeting.topics} 題
            </span>
          )}
        </div>
        <p className="text-[14px] leading-relaxed text-ink2">{pausing ? "正在發言的角色說完這一段就會停。" : info.means}</p>
        {meeting.state === "failed" && meeting.note && (
          <details className="rounded-md bg-destructive/5 px-3 py-2 text-[13px]">
            <summary className="cursor-pointer font-bold text-muted-foreground">awkit 說的原文</summary>
            <p className="mt-1 break-words font-mono text-[12px] leading-relaxed text-ink2">{meeting.note}</p>
          </details>
        )}
        {waiting.length > 0 && !going && (
          <p className="rounded-md bg-signal/15 px-3 py-2 text-[13.5px] font-bold ring-1 ring-inset ring-[#D29A00]/50">你有 {waiting.length} 句話，接著開會時議會會先聽。</p>
        )}
        {!going && other && <OneAtATime other={other} />}
        <div className="flex flex-wrap gap-2">
          {going ? (
            <Button variant="outline" onClick={() => act("pause")} disabled={busy || pausing}>
              <CirclePause aria-hidden /> 暫停
            </Button>
          ) : (
            <Button onClick={() => act("goOn")} disabled={busy || !!other}>
              <Play aria-hidden /> 接著開會
            </Button>
          )}
          {onEditAgenda && !going && (
            <Button variant="outline" onClick={onEditAgenda} disabled={busy}>
              <ListOrdered aria-hidden /> 改還沒談的議題
            </Button>
          )}
        </div>
      </div>
    </Panel>
  );
}

function LiveView({ detail, entries, refresh }: { detail: PlanDetail; entries: ReturnType<typeof usePlan>["entries"]; refresh: () => void }) {
  const meeting = detail.meeting!;
  const topics = detail.agenda?.topics ?? [];
  const live = liveTopic(detail);
  const [picked, setPicked] = useState<number | null>(null);
  const [editing, setEditing] = useState(false);
  const shown = picked ?? live;
  const going = meeting.state === "meeting";
  const started = new Set((detail.record?.topics ?? []).filter((t) => t.started).map((t) => t.topic.id));
  const canEdit = topics.some((t) => !started.has(t.id));
  const answered = new Set((detail.record?.answers ?? []).filter((a) => a.answer.trim()).map((a) => a.question));
  const questions = [
    ...(detail.agenda?.questions ?? []).filter((q) => !answered.has(q)),
    ...(detail.record?.assumptions ?? []).filter((a) => !a.answer && a.question).map((a) => a.question),
  ].filter((q, i, all) => all.indexOf(q) === i);

  if (editing) {
    return (
      <>
        <Banner title="改還沒談的議題">談過的議題維持原樣；後面的可以改內容、順序，或刪掉。存好再接著開會。</Banner>
        <AgendaEditor
          key={JSON.stringify(detail.agenda)}
          detail={detail}
          askQuestions={false}
          onCancel={() => setEditing(false)}
          onDone={() => {
            setEditing(false);
            refresh();
          }}
        />
      </>
    );
  }

  return (
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_340px] xl:grid-rows-[auto_1fr]">
      <Controls detail={detail} onChanged={refresh} onEditAgenda={canEdit ? () => setEditing(true) : undefined} />
      <div className="grid min-w-0 grid-cols-[minmax(0,1fr)] content-start gap-5 xl:col-start-1 xl:row-span-2 xl:row-start-1">
        <TopicStrip detail={detail} selected={shown} onSelect={(i) => setPicked(i === live ? null : i)} />
        {picked !== null && picked !== live && (
          <button type="button" onClick={() => setPicked(null)} className="-mt-2 cursor-pointer justify-self-start text-[13.5px] font-bold text-ring underline-offset-4 hover:underline">
            回到{going ? "正在談" : "停下"}的那一題
          </button>
        )}
        <Panel>
          <div className="p-5">
            <Roundtable topic={topics[shown]} number={shown + 1} of={topics.length} speaking={going && shown === live ? meeting.speaker : undefined} />
          </div>
        </Panel>
        <Proceedings detail={detail} entries={entries} topicIndex={shown} />
        {!going && <Log lines={detail.log} open={meeting.state === "failed"} />}
      </div>
      <div className="grid min-w-0 grid-cols-[minmax(0,1fr)] content-start gap-5 self-start xl:sticky xl:top-20 xl:col-start-2 xl:row-start-2">
        <Panel>
          <div className="p-5">
            <SayBox planId={detail.id} going={going} questions={questions} onSaid={refresh} />
          </div>
        </Panel>
        <Usage meeting={meeting} decisions={(detail.record?.decisions ?? []).filter((d) => !d.replaced_by && !d.withdrawn).length} documents={new Set((detail.record?.documents ?? []).map((d) => d.path)).size} />
      </div>
    </div>
  );
}

function Count({ n }: { n: number }) {
  return <span className="rounded-md bg-muted px-1.5 text-[12px] font-extrabold text-ink2 ring-1 ring-inset ring-border tnum">{n}</span>;
}

/** OutcomeView is what a meeting came to: its tickets first, then what it decided, assumed and wrote. */
function OutcomeView({ detail, entries, refresh }: { detail: PlanDetail; entries: ReturnType<typeof usePlan>["entries"]; refresh: () => void }) {
  const { project } = useProject();
  const [tab, setTab] = useState("tickets");
  const [question, setQuestion] = useState("");
  const [busy, setBusy] = useState(false);
  const record = detail.record;
  const onGitHub = detail.tickets.some((t) => t.issue);
  const waiting = pendingWords(detail);
  const standing = (record?.decisions ?? []).filter((d) => !d.replaced_by && !d.withdrawn).length;
  const open = (record?.assumptions ?? []).filter((a) => !a.answer);
  const documents = new Set((record?.documents ?? []).map((d) => d.path)).size;
  const questions = open.filter((a) => a.question).map((a) => a.question);
  const other = useOtherMeeting(detail.id);

  async function review() {
    setBusy(true);
    try {
      await council.goOn(project.id, detail.id);
      toast.success("議會照你的話重審中", { description: "重審決策、重看受影響的文件，再重排工單。" });
      refresh();
    } catch (e) {
      toast.error("開不起來", { description: (e as Error).message });
    } finally {
      setBusy(false);
    }
  }

  function answer(q: string) {
    setQuestion(q);
    window.setTimeout(() => {
      const box = document.getElementById("say");
      box?.scrollIntoView({ behavior: "smooth", block: "center" });
      box?.focus({ preventScroll: true });
    }, 0);
  }

  return (
    <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_340px]">
      <div className="min-w-0">
        <Tabs value={tab} onValueChange={setTab}>
          <TabsList aria-label="規劃的結果">
            <TabsTrigger value="tickets">
              <TicketIcon aria-hidden /> 工單 <Count n={detail.tickets.length} />
            </TabsTrigger>
            {record && (
              <>
                <TabsTrigger value="decisions">
                  <Gavel aria-hidden /> 決策 <Count n={standing} />
                </TabsTrigger>
                <TabsTrigger value="assumptions">
                  <CircleHelp aria-hidden /> 假設 {open.length > 0 ? <span className="rounded-md bg-signal px-1.5 text-[12px] font-extrabold text-[#231a00] tnum">{open.length}</span> : <Count n={0} />}
                </TabsTrigger>
                <TabsTrigger value="documents">
                  <FileText aria-hidden /> 文件 <Count n={documents} />
                </TabsTrigger>
              </>
            )}
            <TabsTrigger value="transcript">
              <ScrollText aria-hidden /> 會議經過
            </TabsTrigger>
          </TabsList>
          <TabsContent value="tickets">
            <TicketsTab detail={detail} going={false} onChanged={refresh} />
          </TabsContent>
          {record && (
            <>
              <TabsContent value="decisions">
                <DecisionsTab detail={detail} />
              </TabsContent>
              <TabsContent value="assumptions">
                <AssumptionsTab detail={detail} onAnswer={answer} canAnswer={!onGitHub} />
              </TabsContent>
              <TabsContent value="documents">
                <DocumentsTab detail={detail} />
              </TabsContent>
            </>
          )}
          <TabsContent value="transcript">
            <TranscriptTab detail={detail} entries={entries} />
          </TabsContent>
        </Tabs>
      </div>
      <div className="grid min-w-0 grid-cols-[minmax(0,1fr)] content-start gap-5 self-start xl:sticky xl:top-20">
        {onGitHub ? (
          <Panel>
            <div className="p-5 text-[14px] leading-relaxed text-ink2">
              <div className="text-[15px] font-extrabold text-foreground">想改方向？</div>
              這次的工單已經在 GitHub 上，規劃就停在這裡。想加功能或改做法，開一個新的規劃；議會會讀這次的決策和文件，接著往下規劃。
              <Button className="mt-3" variant="outline" onClick={() => (window.location.hash = href({ project: project.id, page: "plans", rest: ["new"] }))}>
                開新的規劃
              </Button>
            </div>
          </Panel>
        ) : (
          <Panel>
            <div className="grid gap-4 p-5">
              <SayBox planId={detail.id} going={false} after questions={questions} question={question} onQuestion={setQuestion} onSaid={refresh} />
              {waiting.length > 0 && (
                <div className="grid gap-2 border-t border-border pt-4">
                  <p className="text-[13.5px] font-bold">你說了 {waiting.length} 句話，議會還沒聽。</p>
                  {other && <OneAtATime other={other} then="開完，再照你的話重審" />}
                  <Button onClick={review} disabled={busy || !!other}>
                    <RefreshCcw aria-hidden /> 照我的話重審
                  </Button>
                </div>
              )}
            </div>
          </Panel>
        )}
        {record && (
          <Panel>
            <dl className="grid grid-cols-2 gap-y-3 p-5 text-center">
              {[
                ["議題", detail.agenda?.topics.length ?? 0],
                ["決策", standing],
                ["文件", documents],
                ["模型呼叫", record.calls],
              ].map(([label, n]) => (
                <div key={label}>
                  <dd className="text-[22px] font-black tnum">{n}</dd>
                  <dt className="text-[12.5px] text-muted-foreground">{label}</dt>
                </div>
              ))}
            </dl>
          </Panel>
        )}
        <Log lines={detail.log} />
      </div>
    </div>
  );
}

/** PlanPage follows one plan: from its agenda, through its meeting, to the tickets it laid out. */
export function PlanPage({ planId }: { planId: string }) {
  const { detail, entries, refresh } = usePlan(planId);
  const { mark } = useOnboarding();
  useEffect(() => mark("plan"), [mark]);
  const d = detail.data;
  if (!d) {
    if (detail.loading)
      return (
        <div className="grid gap-4" role="status" aria-label="讀取中">
          <Skeleton className="h-5 w-32" />
          <Skeleton className="h-9 w-2/3" />
          <Skeleton className="h-40 w-full" />
        </div>
      );
    return (
      <>
        <div className="mb-5">
          <Back />
        </div>
        <Problem title="打不開這個規劃">{detail.error?.message ?? "工作台沒有回答"}</Problem>
      </>
    );
  }
  const state = d.meeting?.state;
  let body: React.ReactNode;
  if (state === "agenda") {
    body = (
      <>
        <Banner title="議程排好了，等你看">主持人排好了議題。看看順序、誰發言對不對，改好再開始；不用改就直接按「照這個議程開會」。</Banner>
        <AgendaEditor key={JSON.stringify(d.agenda)} detail={d} onDone={refresh} />
      </>
    );
  } else if (state === "questions") {
    body = (
      <>
        <Banner title="議會有問題要問你">答完才開始討論。不知道的可以留白，議會會先假設。</Banner>
        <Questions key={d.questions.join("\n")} detail={d} onDone={refresh} />
      </>
    );
  } else if (state === "meeting" || state === "paused" || state === "stopped" || state === "failed") {
    body = <LiveView detail={d} entries={entries} refresh={refresh} />;
  } else if (state === "finished" || d.record || d.tickets.length > 0) {
    body = <OutcomeView detail={d} entries={entries} refresh={refresh} />;
  } else {
    body = <Starting detail={d} />;
  }
  return (
    <>
      <Head detail={d} />
      {body}
    </>
  );
}

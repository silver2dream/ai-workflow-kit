import { useEffect, useRef, useState } from "react";
import { CircleStop, Loader2, OctagonX, Play, RefreshCw, TerminalSquare, TriangleAlert } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Empty, PageHead, Panel, PanelHead, Problem } from "@/components/bits";
import { useProject } from "@/app/project";
import { api, type EngineStep, type RunLog, type StopView, type Ticket } from "@/lib/api";
import { endedWords, stepWords, workingSteps } from "@/lib/engine";
import { useResource } from "@/lib/resource";
import { href } from "@/lib/router";
import { cn } from "@/lib/utils";
import { when } from "@/lib/words";
import { Checks } from "@/pages/Health";
import { RecentActivity } from "@/pages/Overview";
import { headline } from "@/pages/plans/status";
import { titleOf } from "@/lib/tickets";
import { Lanes } from "@/pages/run/Lanes";
import { Shadow } from "@/pages/run/Shadow";

/** How a run of the Principal's workflow ended, in words, by the reason its session ended with. */
const ENDED: Record<string, string> = {
  completed: "上一輪做完了：沒有可以做的工單了。",
  stopped: "上一輪停了：有人要它停，或有工單需要你。",
  failed: "上一輪出錯停下來了。看下面的輸出和活動紀錄，找出原因。",
  paused: "上一輪暫停了：判斷不出下一步，或到了次數上限。",
  interrupted: "上一輪被中斷了。",
  aborted: "上一輪沒有正常結束。",
};

/** The tickets a run limited to one can be given: ready to be written, or sent back to be written again. */
const startable = (t: Ticket) => t.stage === "ready" || t.stage === "rework";

/** Console follows one log of the run, a piece at a time, scrolled to its end while the person does not scroll up. */
function Console({ log, going }: { log: string; going: boolean }) {
  const { project } = useProject();
  const [text, setText] = useState("");
  const [missing, setMissing] = useState(false);
  const next = useRef(0);
  const box = useRef<HTMLPreElement>(null);
  const follow = useRef(true);

  useEffect(() => {
    let alive = true;
    next.current = 0;
    follow.current = true;
    setText("");
    setMissing(false);
    const read = async () => {
      try {
        const out = await api.runOutput(project.id, log, next.current);
        if (!alive) return;
        setMissing(!out.exists);
        if (out.exists) {
          next.current = out.next;
          if (out.fresh) setText(out.text);
          else if (out.text) setText((t) => (t + out.text).slice(-200_000));
        }
      } catch {
        // Read again with the next tick.
      }
    };
    void read();
    const timer = going ? window.setInterval(read, 2000) : undefined;
    return () => {
      alive = false;
      if (timer) window.clearInterval(timer);
    };
  }, [project.id, log, going]);

  useEffect(() => {
    const el = box.current;
    if (el && follow.current) el.scrollTop = el.scrollHeight;
  }, [text]);

  if (missing) return <Empty icon={TerminalSquare} title="這份輸出還沒有內容">開始工作之後，engine 和執行者說的話會出現在這裡。</Empty>;
  return (
    <pre
      ref={box}
      tabIndex={0}
      aria-label="輸出"
      onScroll={(e) => {
        const el = e.currentTarget;
        follow.current = el.scrollHeight - el.scrollTop - el.clientHeight < 24;
      }}
      className="max-h-[520px] min-h-[220px] overflow-auto whitespace-pre-wrap break-words bg-rail px-4 py-3 font-mono text-[12.5px] leading-relaxed text-white/85 outline-none focus-visible:ring-3 focus-visible:ring-ring/50"
    >
      {text || "…"}
    </pre>
  );
}

function logTitle(log: RunLog): string {
  if (log.name === "run") return "這一輪";
  if (log.name === "principal") return "主控";
  return `工單 #${log.issue}`;
}

/** Output is what the run says: its own words (the engine's), the Principal's when it ran one, and each ticket's Worker's. */
function Output({ going, target, prefer }: { going: boolean; target?: number; prefer?: string }) {
  const { project } = useProject();
  const [tick, setTick] = useState(0);
  // Asked again while the run goes, and when the page wants a log it may not
  // have known of: the one a run that did not start just wrote.
  const logs = useResource(() => api.runLogs(project.id), [project.id, tick, prefer, going]);
  const [picked, setPicked] = useState<string | null>(null);
  useEffect(() => {
    if (!going) return;
    const timer = window.setInterval(() => setTick((t) => t + 1), 5000);
    return () => window.clearInterval(timer);
  }, [going]);
  const list = (logs.data?.logs ?? []).filter((l) => l.name !== "run" || l.size > 0).slice(0, 7);
  const worker = target ? list.find((l) => l.issue === target) : undefined;
  const fallback = (prefer && list.find((l) => l.name === prefer)?.name) ?? worker?.name ?? list.find((l) => l.name === "run")?.name ?? list[0]?.name;
  const shown = picked && list.some((l) => l.name === picked) ? picked : fallback;
  return (
    <Panel data-tour="runoutput" className="overflow-hidden">
      <PanelHead title="即時輸出" sub="「這一輪」是 engine 派工的紀錄，「工單 #N」是那張工單的執行者說的話；工作中每兩秒更新一次。" />
      {list.length === 0 ? (
        <Empty icon={TerminalSquare} title="還沒有輸出">開始工作之後，engine 和執行者說的話會出現在這裡。</Empty>
      ) : (
        <>
          <div role="tablist" aria-label="哪一份輸出" className="flex gap-1 overflow-x-auto border-b border-border px-4">
            {list.map((log) => (
              <button
                key={log.name}
                type="button"
                role="tab"
                aria-selected={log.name === shown}
                onClick={() => setPicked(log.name)}
                className={cn(
                  "-mb-px shrink-0 cursor-pointer border-b-[3px] px-3 py-2 text-[13.5px] font-bold transition-colors",
                  log.name === shown ? "border-foreground text-foreground" : "border-transparent text-muted-foreground hover:text-foreground",
                )}
              >
                {logTitle(log)}
                {log.issue && log.issue === target && going && <span className="ml-1.5 inline-block size-1.5 rounded-full bg-live align-middle" aria-label="正在做" />}
              </button>
            ))}
          </div>
          {shown && <Console key={shown} log={shown} going={going} />}
        </>
      )}
    </Panel>
  );
}

/** StartDialog starts a run: of every ticket that can be worked on, or of one. */
function StartDialog({ open, onClose, preset, onStarted }: { open: boolean; onClose: () => void; preset?: number; onStarted: () => void }) {
  const { project, tickets, status } = useProject();
  const choices = (tickets.data?.tickets?.open ?? []).filter(startable);
  const [scope, setScope] = useState<"all" | "one">(preset ? "one" : "all");
  const [issue, setIssue] = useState<number | undefined>(preset);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (open) {
      setScope(preset ? "one" : "all");
      setIssue(preset ?? choices[0]?.number);
    }
    // The choices are read when the dialog opens, not as they change.
  }, [open, preset]); // eslint-disable-line react-hooks/exhaustive-deps

  async function start() {
    setBusy(true);
    try {
      await api.startRun(project.id, scope === "one" ? issue : undefined);
      toast.success("開始工作了", { description: "關掉工作台也會繼續。要停，在這一頁按停止。" });
      onStarted();
      onClose();
      window.setTimeout(status.reload, 1500);
    } catch (e) {
      toast.error("開不起來", { description: (e as Error).message });
    } finally {
      setBusy(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={(o) => !o && !busy && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>開始工作？</DialogTitle>
          <DialogDescription>engine 會一張一張把工單交給執行者，寫好的程式經過審查才合併。做到沒有工單、或有工單需要你，就會停下。執行者和審查是 AI，工單越多花越多。</DialogDescription>
        </DialogHeader>
        <fieldset className="grid gap-2">
          <legend className="sr-only">做哪些工單</legend>
          <label className="flex cursor-pointer gap-3 rounded-lg border border-border p-3 has-[:checked]:border-foreground/70">
            <input type="radio" name="scope" checked={scope === "all"} onChange={() => setScope("all")} className="mt-1 accent-[var(--foreground)]" />
            <span>
              <span className="block text-[15px] font-bold">所有可以做的工單</span>
              <span className="block text-[13.5px] text-muted-foreground">依編號順序，要等別張先做完的會先跳過。</span>
            </span>
          </label>
          <label className="flex cursor-pointer gap-3 rounded-lg border border-border p-3 has-[:checked]:border-foreground/70">
            <input type="radio" name="scope" checked={scope === "one"} onChange={() => setScope("one")} disabled={choices.length === 0} className="mt-1 accent-[var(--foreground)]" />
            <span className="min-w-0 flex-1">
              <span className="block text-[15px] font-bold">只做一張</span>
              <span className="block text-[13.5px] text-muted-foreground">做完這張（或它需要你）就停，不碰別張。</span>
              {scope === "one" && (
                <select
                  aria-label="做哪一張"
                  value={issue ?? ""}
                  onChange={(e) => setIssue(Number(e.target.value))}
                  className="mt-2 h-9 w-full min-w-0 rounded-md border border-input bg-card px-2 text-[14px]"
                >
                  {choices.map((t) => (
                    <option key={t.number} value={t.number}>
                      #{t.number} {t.title}
                    </option>
                  ))}
                </select>
              )}
            </span>
          </label>
        </fieldset>
        <DialogFooter>
          <Button variant="outline" onClick={onClose} disabled={busy}>
            先不要
          </Button>
          <Button onClick={start} disabled={busy || (scope === "one" && !issue)}>
            <Play aria-hidden /> {busy ? "開始中…" : scope === "one" ? `開始做 #${issue}` : "開始工作"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** RunPage starts the work and stops it, and shows what the run says while it goes. */
export function RunPage({ preset }: { preset?: number }) {
  const { project, status, engine, tickets, plans } = useProject();
  const [starting, setStarting] = useState(!!preset);
  const [stopping, setStopping] = useState<false | "now">(false);
  const [busy, setBusy] = useState(false);
  const report = status.data;
  const run = report?.run;
  const going = run?.state === "running";
  const winding = going && !!report?.control.stop_present;
  const target = report?.target.issue_id;
  const targetTicket = (tickets.data?.tickets?.open ?? []).find((t) => t.number === target);
  // The engine says what it does; a run of the Principal's workflow is
  // guessed at from the Worker that ran last.
  const view = engine.data;
  // Lanes run side by side: each step the engine is on, the oldest first.
  const steps = going && view?.going ? workingSteps(view.run) : [];
  const said = (s: EngineStep) => `${stepWords(s, s.issue ? titleOf(tickets.data, s.issue) : undefined)}，從 ${when(s.since)} 開始`;
  const health = useResource(() => (going ? Promise.resolve(undefined) : api.health(project.id)), [project.id, going]);
  // What a run's own checks stop at: the errors of the health check, and a
  // working tree that is not clean, which the health check only warns of.
  const setupProblems = (health.data?.setup ?? []).filter((c) => c.status === "error" || (c.status === "warning" && c.name === "Git Status"));
  // A run started here that never got as far as its session stopped at its own checks.
  const [startedAt, setStartedAt] = useState<number | null>(null);
  const sessionAt = run?.session.started_at ? Date.parse(run.session.started_at) : 0;
  const neverStarted = !going && startedAt !== null && sessionAt < startedAt - 2000 && Date.now() - startedAt > 4000;
  // The last run was the engine's when its run started with the session
  // (a minute after it, at most): it says why it ended.
  const engineEnded =
    !going && view?.run?.ended && view.run.reason && (!sessionAt || Date.parse(view.run.started) >= sessionAt - 60_000) ? view.run.reason : undefined;
  const meeting = (plans.data?.plans ?? []).find((p) => p.meeting?.state === "meeting");
  const approving = (plans.data?.plans ?? []).find((p) => p.approval?.state === "working");
  const noTickets = !!tickets.data?.tickets && (tickets.data.tickets.open?.length ?? 0) === 0;
  const blocked = noTickets
    ? "目前沒有開著的工單：先到「規劃與議會」核可一份規劃，工單建到 GitHub 之後就能開始。"
    : meeting
    ? `「${headline(meeting.brief, meeting.id)}」正在開會，它會改到工作要讀的文件：等它開完再開始。`
    : approving
      ? `「${headline(approving.brief, approving.id)}」正在核可，工單還沒準備好：等它做完再開始。`
      : setupProblems.length > 0
        ? `開工前的檢查有 ${setupProblems.length} 項沒過，先處理它們。`
        : undefined;

  useEffect(() => {
    if (preset) setStarting(true);
  }, [preset]);

  async function stop(now: boolean) {
    setBusy(true);
    try {
      const said: StopView = await api.stopRun(project.id, now);
      if (said.engine && !said.answered && !now) {
        toast.success("已經告訴 engine 要停了", { description: "它還沒回覆；手上正在做的事（執行者正在寫，或審查正在審）做完就會停。" });
      } else if (said.engine && !said.answered) {
        toast.success("停下來了", { description: "engine 沒有回覆，所以直接結束了它和它開出來的程式。做到一半的改動留在執行者自己的工作區，下一輪會從這張工單接著做。" });
      } else {
        toast.success(now ? "停下來了" : "會在手上正在做的事做完後停", {
          description: now ? "做到一半的改動留在它自己的工作區，下一輪會從這張工單接著做。" : "正在寫的執行者或正在審的審查做完，就不再接新的工作。",
        });
      }
      setStopping(false);
      status.reload();
      engine.reload();
    } catch (e) {
      toast.error("停不下來", { description: (e as Error).message });
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <PageHead title="執行" sub="開始工作、停止工作，看執行者現在在做什麼。關掉工作台，工作也不會停。" />
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_380px]">
        <div className="grid min-w-0 grid-cols-[minmax(0,1fr)] content-start gap-5">
          <Panel data-tour="runcontrols">
            <div className="grid gap-4 p-5">
              {status.loading && !report ? (
                <div className="flex items-center gap-2 text-muted-foreground">
                  <Loader2 className="size-4 animate-spin" aria-hidden />
                  讀取中…
                </div>
              ) : going ? (
                <>
                  <div className="flex flex-wrap items-center gap-3">
                    <span className="relative flex size-3" aria-hidden>
                      <span className={cn("absolute inset-0 rounded-full opacity-60", winding ? "bg-caution" : "animate-ping bg-live")} />
                      <span className={cn("relative size-3 rounded-full", winding ? "bg-caution" : "bg-live")} />
                    </span>
                    <span className={cn("text-[24px] font-black", winding ? "text-caution-ink" : "text-live")}>{winding ? "正在收尾" : "工作中"}</span>
                    {run?.start_time && <span className="text-[13.5px] text-muted-foreground">從 {when(run.start_time)} 開始</span>}
                  </div>
                  {!winding && steps.length > 1 ? (
                    <div className="grid gap-1.5 text-[15px] leading-relaxed">
                      <p>同時在做 {steps.length} 件事：</p>
                      <ul className="grid list-disc gap-1 pl-5">
                        {steps.map((s) => (
                          <li key={`${s.issue}-${s.action}-${s.merge ?? ""}`}>{said(s)}。</li>
                        ))}
                      </ul>
                    </div>
                  ) : (
                    <p className="text-[15px] leading-relaxed">
                      {winding
                        ? "收到停止了：手上正在做的事（執行者正在寫，或審查正在審）做完就停，不再接新的工作。等不及可以立刻停。"
                        : steps.length === 1
                          ? `${said(steps[0])}。`
                          : view?.going
                            ? "engine 在看下一步要做什麼。"
                            : target
                              ? `執行者正在處理工單 #${target}${targetTicket ? `「${targetTicket.title}」` : ""}。`
                              : "主控在看下一步要做什麼。"}
                    </p>
                  )}
                  <div className="flex flex-wrap gap-2">
                    {!winding && (
                      <Button variant="outline" onClick={() => stop(false)} disabled={busy}>
                        <CircleStop aria-hidden /> 收尾後停
                      </Button>
                    )}
                    <Button variant="destructive" onClick={() => setStopping("now")} disabled={busy}>
                      <OctagonX aria-hidden /> 立刻停
                    </Button>
                  </div>
                </>
              ) : (
                <>
                  <div className="flex flex-wrap items-center gap-3">
                    <span className="text-[24px] font-black">{run?.state === "stale_lock" ? "上一輪沒有正常結束" : "沒有在工作"}</span>
                    {run?.session.ended_at && <span className="text-[13.5px] text-muted-foreground">上一輪 {when(run.session.ended_at)} 結束</span>}
                  </div>
                  <p className="text-[15px] leading-relaxed text-ink2">
                    {run?.state === "stale_lock"
                      ? "上一輪的程序不在了（被立刻停下，或電腦重開）。做到一半的工單，下一輪會接著做；它留下的工作區可以在健康檢查清掉。"
                      : engineEnded
                        ? endedWords(engineEnded)
                        : run?.session.exit_reason
                          ? (ENDED[run.session.exit_reason] ?? `上一輪結束了（${run.session.exit_reason}）。`)
                          : "還沒有開始過。核可了規劃、工單在 GitHub 上之後，就可以開始工作。"}
                  </p>
                  {neverStarted && (
                    <Problem title="剛才那一輪沒有開起來">awkit run 在自己的開工檢查就停了。看下面「這一輪」的輸出是哪一項沒過，處理好再開始。</Problem>
                  )}
                  {blocked && <p className="text-[14px] font-bold">{blocked}</p>}
                  <div className="flex flex-wrap items-center gap-2">
                    <Button size="lg" onClick={() => setStarting(true)} disabled={!!blocked || health.loading}>
                      <Play aria-hidden /> 開始工作
                    </Button>
                    {health.loading && <span className="text-[13px] text-muted-foreground">先檢查一下開工需要的東西…</span>}
                  </div>
                </>
              )}
            </div>
            {!going && setupProblems.length > 0 && (
              <div className="border-t border-border">
                <div className="flex items-center gap-2 px-5 pt-4 text-[14px] font-bold">
                  <TriangleAlert className="size-4 text-destructive" aria-hidden />
                  開工前要先處理
                  <a href={href({ project: project.id, page: "health", rest: [] })} className="ml-auto text-[13.5px] text-ring underline-offset-4 hover:underline">
                    看全部檢查
                  </a>
                </div>
                <p className="px-5 pt-1 text-[13.5px] text-muted-foreground">
                  「Git Status」沒過，通常是專案資料夾有還沒 commit 的變更：議會寫的文件，到規劃頁按「核可」就會送進整合分支；其他的要自己處理。
                </p>
                <Checks checks={setupProblems} loading={false} />
              </div>
            )}
            {health.error && !going && (
              <div className="px-5 pb-5">
                <Problem title="檢查跑不完">{health.error.message}</Problem>
              </div>
            )}
          </Panel>
          <Lanes />
          <Output going={going} target={target} prefer={neverStarted ? "run" : undefined} />
        </div>
        <div className="grid min-w-0 grid-cols-[minmax(0,1fr)] content-start gap-5">
          <Panel>
            <dl className="grid grid-cols-2 gap-y-3 p-5 text-center">
              <div>
                <dd className="text-[22px] font-black tnum">{report?.control.loop_count ?? "—"}</dd>
                <dt className="text-[12.5px] text-muted-foreground">第幾輪</dt>
              </div>
              <div>
                <dd className={cn("text-[22px] font-black tnum", (report?.control.consecutive_failures ?? 0) > 0 && "text-caution-ink")}>{report?.control.consecutive_failures ?? "—"}</dd>
                <dt className="text-[12.5px] text-muted-foreground">連續失敗（到 5 次自己停）</dt>
              </div>
            </dl>
          </Panel>
          <Shadow />
          <RecentActivity limit={8} />
          <Button variant="ghost" className="justify-self-start" onClick={() => { status.reload(); engine.reload(); health.reload(); }}>
            <RefreshCw aria-hidden /> 重新讀一次
          </Button>
        </div>
      </div>

      <StartDialog
        open={starting}
        onClose={() => setStarting(false)}
        preset={preset}
        onStarted={() => {
          setStartedAt(Date.now());
          // Looked at again once its checks had time to say.
          window.setTimeout(() => status.reload(), 6000);
        }}
      />

      <Dialog open={stopping === "now"} onOpenChange={(o) => !o && !busy && setStopping(false)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>立刻停下？</DialogTitle>
            <DialogDescription>{view?.going ? "engine 會馬上結束執行者和審查，再結束它自己，" : "主控、執行者和審查會馬上被結束，"}不等手上這一步做完。</DialogDescription>
          </DialogHeader>
          <div className="flex gap-2.5 rounded-lg border-l-4 border-caution bg-caution/10 px-4 py-3 text-[14px]">
            <TriangleAlert className="mt-0.5 size-4 shrink-0 text-caution-ink" aria-hidden />
            <span>做到一半的改動會留在執行者自己的工作區，不會進 PR；下一輪會從這張工單重新開始。留下的工作區可以在健康檢查清掉。不急的話，用「收尾後停」比較乾淨。</span>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setStopping(false)} disabled={busy}>
              不停了
            </Button>
            {!winding && (
              <Button variant="outline" onClick={() => stop(false)} disabled={busy}>
                <CircleStop aria-hidden /> 改成收尾後停
              </Button>
            )}
            <Button variant="destructive" onClick={() => stop(true)} disabled={busy}>
              <OctagonX aria-hidden /> 立刻停
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

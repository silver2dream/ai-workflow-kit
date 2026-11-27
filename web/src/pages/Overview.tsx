import { Activity, ArrowRight, CloudOff, Hand, History, RefreshCw, TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Empty, Panel, PanelHead, Problem, Tag, Term } from "@/components/bits";
import { headline, planStatus } from "@/pages/plans/status";
import { useProject } from "@/app/project";
import { useTicketSheet } from "@/app/ticket-sheet";
import { href, navigate } from "@/lib/router";
import { describe, eventKey, STAGES, when } from "@/lib/words";
import type { Stage, Ticket, TicketsView } from "@/lib/api";
import { cn } from "@/lib/utils";
import { TONE } from "@/components/bits";

const MAX_TOKENS = 8;

/** unknown is true when GitHub could not be asked and nothing is known of the tickets. */
const unknown = (view?: TicketsView) => !!view?.error && !view.tickets;

/** Token is a ticket on the line: its number, coloured by its stage. */
export function Token({ ticket }: { ticket: Ticket }) {
  const { open } = useTicketSheet();
  const style: Record<Stage, string> = {
    needs_you: "bg-signal text-[#231a00]",
    in_progress: "bg-live text-white",
    rework: "bg-live/80 text-white",
    in_review: "bg-review/10 text-review ring-1 ring-inset ring-review/30",
    ready: "bg-card text-foreground ring-1 ring-inset ring-border",
    waiting: "bg-card text-ink2 ring-1 ring-inset ring-border",
  };
  return (
    <button
      type="button"
      onClick={() => open(ticket.number)}
      title={`工單 #${ticket.number}：${ticket.title}`}
      className={cn("relative inline-flex items-center gap-1.5 rounded-md px-2 py-[3px] text-[13px] font-extrabold tnum transition hover:brightness-95", style[ticket.stage])}
    >
      {ticket.stage === "in_progress" && (
        <span className="relative flex size-2" aria-hidden>
          <span className="absolute inset-0 animate-ping rounded-full bg-white opacity-70" />
          <span className="relative size-2 rounded-full bg-white" />
        </span>
      )}
      #{ticket.number}
    </button>
  );
}

function Tokens({ tickets }: { tickets: Ticket[] }) {
  const shown = tickets.slice(0, MAX_TOKENS);
  return (
    <div className="flex max-w-[170px] flex-wrap justify-center gap-1.5">
      {shown.map((t) => (
        <Token key={t.number} ticket={t} />
      ))}
      {tickets.length > shown.length && <span className="text-[12.5px] font-bold text-muted-foreground">還有 {tickets.length - shown.length} 張</span>}
    </div>
  );
}

function AttentionStrip({ needs }: { needs: Ticket[] }) {
  const { project, tickets } = useProject();
  const { open } = useTicketSheet();
  if (needs.length === 0 && unknown(tickets.data)) {
    return (
      <a
        data-tour="attention"
        href={href({ project: project.id, page: "health", rest: [] })}
        className="mb-6 flex items-center gap-2 rounded-xl border border-border bg-card px-4 py-3 text-[14.5px] text-ink2 transition-colors hover:border-muted-foreground"
      >
        <CloudOff className="size-4 text-muted-foreground" aria-hidden />
        GitHub 問不到工單，所以不知道有沒有事要你出手。到健康檢查看是哪裡沒接好。
        <ArrowRight className="ml-auto size-4 shrink-0" aria-hidden />
      </a>
    );
  }
  if (needs.length === 0) {
    return (
      <div data-tour="attention" className="mb-6 flex items-center gap-2 rounded-xl border border-border bg-card px-4 py-3 text-[14.5px] text-ink2">
        <Hand className="size-4 text-go" aria-hidden />
        目前沒有事要你出手。有的時候，這裡會變成一條黃色帶子。
      </div>
    );
  }
  const first = needs[0];
  return (
    <section data-tour="attention" aria-label="需要你處理" className="mb-6 flex items-stretch overflow-hidden rounded-xl bg-signal text-[#231a00]">
      <span className="hazard w-2.5 shrink-0" aria-hidden />
      <div className="flex flex-1 flex-wrap items-center gap-x-5 gap-y-2 px-4 py-3">
        <span className="flex items-center gap-2 text-[15.5px] font-extrabold">
          <Hand className="size-[18px]" aria-hidden />
          {needs.length} 張工單等你處理
        </span>
        <span className="text-[14.5px]">
          工單 #{first.number}：{first.title}
        </span>
        <span className="ml-auto flex items-center gap-1">
          <a href={href({ project: project.id, page: "inbox", rest: [] })} className="rounded-md px-3 py-1.5 text-[13.5px] font-extrabold underline-offset-4 hover:underline">
            看全部 {needs.length} 件
          </a>
          <button
            type="button"
            onClick={() => open(first.number)}
            className="inline-flex items-center gap-1.5 rounded-md bg-[#231a00] px-3 py-1.5 text-[13.5px] font-extrabold text-signal transition-colors hover:bg-[#3d2f00]"
          >
            先處理 #{first.number}
          </button>
        </span>
      </div>
    </section>
  );
}

function LineMap() {
  const { project, tickets, plans } = useProject();
  const open = tickets.data?.tickets?.open ?? [];
  const done = tickets.data?.tickets?.done ?? 0;
  const at = (stages: Stage[]) => open.filter((t) => stages.includes(t.stage));
  const latest = plans.data?.plans[0];
  const needs = at(["needs_you"]);
  // Where GitHub could not be asked, nothing is known of the tickets' stations.
  const blind = unknown(tickets.data);
  const notKnown = <p className="text-[12.5px] text-muted-foreground">—</p>;
  const meeting = (plans.data?.plans ?? []).find((p) => p.meeting?.state === "meeting");
  const shown = meeting ?? latest;
  const shownStatus = shown ? planStatus(shown) : undefined;
  const planLink = (rest: string[]) => href({ project: project.id, page: "plans", rest });
  const stations: { name: string; busy?: boolean; body: React.ReactNode }[] = [
    {
      name: "需求",
      body: shown ? (
        <a href={planLink([shown.id])} className="line-clamp-3 max-w-[160px] text-center text-[13px] leading-snug text-ink2 underline-offset-4 hover:text-foreground hover:underline">
          「{headline(shown.brief, shown.id)}」
        </a>
      ) : (
        <a href={planLink(["new"])} className="text-center text-[12.5px] font-bold text-ring underline-offset-4 hover:underline">
          開第一個規劃
        </a>
      ),
    },
    {
      name: "議會",
      busy: !!meeting,
      body: shown ? (
        <a href={planLink([shown.id])} className="flex flex-col items-center gap-1 text-center text-[12.5px] text-muted-foreground underline-offset-4 hover:underline">
          {shownStatus && <Tag tone={shownStatus.tone}>{shownStatus.label}</Tag>}
          {meeting?.meeting && meeting.meeting.topics > 0 ? (
            <span className="tnum">
              第 {Math.min(Math.max(meeting.meeting.topic, 1), meeting.meeting.topics)} / {meeting.meeting.topics} 題
            </span>
          ) : shown.tickets.length > 0 ? (
            <span className="tnum">
              {shown.milestones.length} 個里程碑，{shown.tickets.length} 張工單
            </span>
          ) : null}
        </a>
      ) : (
        <p className="text-[12.5px] text-muted-foreground">—</p>
      ),
    },
    { name: "工單", body: blind ? notKnown : <><Tokens tickets={at(["ready", "waiting"])} /><Caption n={at(["ready", "waiting"]).length} none="沒有排隊的票" some="排隊中" /></> },
    { name: "寫程式", busy: at(["in_progress", "rework"]).length > 0, body: blind ? notKnown : <><Tokens tickets={at(["in_progress", "rework"])} /><Caption n={at(["in_progress", "rework"]).length} none="沒有人在寫" some="執行者在寫" /></> },
    { name: "審查", busy: at(["in_review"]).length > 0, body: blind ? notKnown : <><Tokens tickets={at(["in_review"])} /><Caption n={at(["in_review"]).length} none="沒有在審的" some="審查中" /></> },
    { name: "合併", body: blind ? notKnown : <p className="text-[13px] font-bold text-go tnum">{done} 張已合併</p> },
  ];

  return (
    <Panel data-tour="linemap">
      <PanelHead
        title="工作路線"
        sub="每張工單是一個小牌子，從需求走到合併。點一張看它的經過。"
        right={
          <div className="hidden flex-wrap items-center gap-x-3 gap-y-1 text-[12.5px] text-muted-foreground xl:flex">
            <Legend className="bg-live" label="寫程式中" />
            <Legend className="bg-review/25 ring-1 ring-review/40" label="審查中" />
            <Legend className="bg-signal" label="卡住，等你" />
          </div>
        }
      />
      {tickets.error && !tickets.data ? (
        <div className="px-5 pb-5">
          <Problem title="讀不到工單">{tickets.error.message}</Problem>
        </div>
      ) : tickets.data?.error ? (
        <div className="px-5 pb-5">
          <Problem title="GitHub 問不到工單">{tickets.data.error}</Problem>
        </div>
      ) : null}
      <div className="relative mx-5 mb-6 mt-2 hidden md:block" style={{ height: needs.length ? 268 : 210 }}>
        <div className="absolute inset-x-0 top-0 grid grid-cols-6">
          {stations.map((s) => (
            <div key={s.name} className="text-center text-[15px] font-extrabold">
              {s.name}
            </div>
          ))}
        </div>
        <div className="absolute left-[8.333%] right-[8.333%] top-[42px] h-[5px] rounded-full bg-foreground" />
        {needs.length > 0 && (
          <svg className="absolute inset-0 h-full w-full overflow-visible" viewBox="0 0 600 268" preserveAspectRatio="none" aria-hidden>
            <path d="M 400 47 V 214 H 410" fill="none" stroke="#D29A00" strokeWidth="5" strokeLinecap="round" strokeDasharray="1 9" vectorEffect="non-scaling-stroke" />
          </svg>
        )}
        <div className="absolute inset-x-0 top-[33px] grid grid-cols-6">
          {stations.map((s) => (
            <div key={s.name} className="flex justify-center">
              <span className={cn("block size-[23px] rounded-full border-[5px] border-foreground bg-card", s.busy && "ring-[5px] ring-live/25")} />
            </div>
          ))}
        </div>
        <div className="absolute inset-x-0 top-[72px] grid grid-cols-6">
          {stations.map((s) => (
            <div key={s.name} className="flex flex-col items-center gap-1.5 px-1">
              {tickets.loading && !tickets.data ? <Skeleton className="h-6 w-16" /> : s.body}
            </div>
          ))}
        </div>
        {needs.length > 0 && (
          <div className="absolute left-[68.4%] right-0 top-[188px] flex items-stretch overflow-hidden rounded-lg bg-signal/20 ring-1 ring-inset ring-[#D29A00]/50">
            <span className="hazard w-2 shrink-0" aria-hidden />
            <div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-1 px-3 py-2.5">
              <Tokens tickets={needs} />
              <span className="min-w-0 flex-1 text-[13.5px] leading-snug">
                <b>卡在這裡。</b>
                {needs[0].why ?? STAGES.needs_you.means}
              </span>
            </div>
          </div>
        )}
      </div>
      <ol className="mx-5 mb-5 grid gap-3 md:hidden">
        {stations.map((s) => (
          <li key={s.name} className="flex gap-3">
            <span className="mt-1 size-4 shrink-0 rounded-full border-4 border-foreground bg-card" />
            <div className="flex flex-col items-start gap-1.5">
              <span className="font-extrabold">{s.name}</span>
              {s.body}
            </div>
          </li>
        ))}
        {needs.length > 0 && (
          <li className="flex items-center gap-2 rounded-lg bg-signal/20 p-2">
            <Tokens tickets={needs} />
            <span className="text-[13px]">卡住，等你處理</span>
          </li>
        )}
      </ol>
    </Panel>
  );
}

function Caption({ n, none, some }: { n: number; none: string; some: string }) {
  return <p className="text-[12.5px] text-muted-foreground">{n === 0 ? none : `${some}，${n} 張`}</p>;
}

function Legend({ className, label }: { className: string; label: string }) {
  return (
    <span className="flex items-center gap-1.5">
      <span className={cn("size-3 rounded-sm", className)} />
      {label}
    </span>
  );
}

export function RecentActivity({ limit }: { limit: number }) {
  const { events, project } = useProject();
  const list = [...(events.data?.events ?? [])].reverse().slice(0, limit);
  return (
    <Panel>
      <PanelHead title="剛剛發生的事" right={<a className="text-[14px] font-bold text-ring underline-offset-4 hover:underline" href={href({ project: project.id, page: "activity", rest: [] })}>活動紀錄</a>} />
      {events.loading && !events.data ? (
        <div className="grid gap-3 px-5 pb-5">{[0, 1, 2].map((i) => <Skeleton key={i} className="h-8" />)}</div>
      ) : list.length === 0 ? (
        <Empty icon={History} title="還沒有工作紀錄">開始工作之後，每一步都會出現在這裡。</Empty>
      ) : (
        <ol className="px-5 pb-4">
          {list.map((event) => {
            const d = describe(event, events.data?.dispatcher);
            return (
              <li key={eventKey(event)} className="flex gap-3 pb-3.5 last:pb-1">
                <span className={cn("mt-0.5 h-7 shrink-0 rounded-md px-1.5 text-[11.5px] font-bold leading-7", TONE[d.tone])}>{d.who}</span>
                <div className="min-w-0">
                  <p className="text-[14px] leading-snug">{d.text}</p>
                  <time dateTime={event.ts} title={new Date(event.ts).toLocaleString()} className="mt-0.5 block text-[12.5px] text-muted-foreground tnum">
                    {when(event.ts)}
                  </time>
                </div>
              </li>
            );
          })}
        </ol>
      )}
    </Panel>
  );
}

export function NeedsYouList({ limit }: { limit?: number }) {
  const { tickets } = useProject();
  const { open } = useTicketSheet();
  const needs = (tickets.data?.tickets?.open ?? []).filter((t) => t.stage === "needs_you");
  const shown = limit ? needs.slice(0, limit) : needs;
  if (tickets.loading && !tickets.data) return <div className="grid gap-3 px-5 pb-5">{[0, 1].map((i) => <Skeleton key={i} className="h-12" />)}</div>;
  if (shown.length === 0 && unknown(tickets.data)) return <Empty icon={CloudOff} title="不知道有沒有卡住的工單">GitHub 問不到工單，所以這裡看不出來。到「健康檢查」看是哪裡沒接好。</Empty>;
  if (shown.length === 0) return <Empty icon={Hand} title="沒有要你處理的事">工單卡住、只有你能決定怎麼辦時，會出現在這裡。</Empty>;
  return (
    <ul>
      {shown.map((t) => (
        <li key={t.number} className="flex items-start gap-3.5 border-t border-border px-5 py-3.5 first:border-t-0">
          <span className={cn("mt-0.5 grid size-9 shrink-0 place-items-center rounded-lg", TONE.signal)}>
            <Hand className="size-[18px]" aria-hidden />
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2 text-[15px] font-bold">
              工單 #{t.number} {t.title}
            </div>
            <p className="mt-0.5 text-[14px] text-ink2">{t.why ?? STAGES.needs_you.means}</p>
          </div>
          <Button size="sm" className="shrink-0 bg-signal text-[#231a00] hover:bg-signal/90" onClick={() => open(t.number)}>
            處理
          </Button>
        </li>
      ))}
    </ul>
  );
}

function RunState() {
  const { status } = useProject();
  const run = status.data?.run;
  const running = run?.state === "running";
  const failures = status.data?.control.consecutive_failures ?? 0;
  return (
    <div className="p-5">
      <div className="flex items-center gap-2 text-[13.5px] text-muted-foreground">
        <Activity className="size-4" aria-hidden />
        這一輪
      </div>
      <div className={cn("mt-1 text-[24px] font-black", running ? "text-live" : "")}>{status.loading && !status.data ? "…" : running ? "工作中" : run?.state === "stale_lock" ? "上次沒有正常結束" : "停著"}</div>
      <p className="mt-1 text-[13px] text-muted-foreground tnum">
        {status.data?.control.loop_count != null && `第 ${status.data.control.loop_count} 輪，`}
        連續失敗 {failures} 次{failures > 0 ? "（到 5 次會自己停下來）" : ""}
      </p>
    </div>
  );
}

function Tally() {
  const { tickets } = useProject();
  const open = tickets.data?.tickets?.open ?? [];
  const done = tickets.data?.tickets?.done ?? 0;
  const count = (s: Stage[]) => open.filter((t) => s.includes(t.stage)).length;
  const segments: [number, string][] = [
    [done, "bg-go"],
    [count(["in_review"]), "bg-review"],
    [count(["in_progress", "rework"]), "bg-live"],
    [count(["needs_you"]), "bg-signal"],
    [count(["ready", "waiting"]), "bg-border"],
  ];
  const total = done + open.length;
  if (unknown(tickets.data)) {
    return (
      <div className="p-5">
        <div className="text-[13.5px] text-muted-foreground">
          <Term>工單</Term>
        </div>
        <div className="mt-1 text-[32px] font-black leading-none text-muted-foreground">—</div>
        <p className="mt-2 text-[13px] text-muted-foreground">GitHub 問不到工單，數不出來。</p>
      </div>
    );
  }
  return (
    <div className="p-5">
      <div className="text-[13.5px] text-muted-foreground">
        <Term>工單</Term>
      </div>
      <div className="mt-1 flex items-baseline gap-1">
        <span className="text-[32px] font-black leading-none tnum">{done}</span>
        <span className="text-[16px] font-bold text-muted-foreground">/ {total} 張已合併</span>
      </div>
      <div className="mt-3 flex h-2.5 gap-0.5 overflow-hidden rounded-sm" aria-hidden>
        {total === 0 ? <span className="flex-1 bg-border" /> : segments.filter(([n]) => n > 0).map(([n, c]) => <span key={c} className={c} style={{ flex: n }} />)}
      </div>
      <p className="mt-2 text-[13px] text-muted-foreground">
        {count(["in_review"])} 張審查中、{count(["in_progress", "rework"])} 張寫程式中、{count(["needs_you"])} 張卡住、{count(["ready", "waiting"])} 張在等
      </p>
    </div>
  );
}

function Milestones() {
  const { plans } = useProject();
  // The milestones of the work under way: the newest plan that was approved,
  // or else the newest that laid milestones out.
  const laid = (plans.data?.plans ?? []).filter((p) => p.milestones.length > 0);
  const latest = laid.find((p) => p.tickets.some((t) => t.issue)) ?? laid[0];
  if (!latest || latest.milestones.length === 0) {
    return (
      <div className="p-5">
        <div className="text-[13.5px] text-muted-foreground">
          <Term>里程碑</Term>
        </div>
        <p className="mt-2 text-[14px] text-ink2">還沒有。議會規劃之後，會排出幾個看得到成果的階段。</p>
      </div>
    );
  }
  return (
    <div className="p-5">
      <div className="text-[13.5px] text-muted-foreground">
        <Term>里程碑</Term>，最新的規劃
      </div>
      <ol className="mt-3 grid gap-1.5">
        {latest.milestones.slice(0, 4).map((m, i) => (
          <li key={m.key} className="flex items-start gap-2 text-[13.5px]">
            <span className={cn("mt-0.5 grid size-5 shrink-0 place-items-center rounded text-[11px] font-black", i === 0 ? "bg-live text-white" : "bg-muted text-muted-foreground ring-1 ring-inset ring-border")}>{i + 1}</span>
            <span className={i === 0 ? "font-bold" : "text-ink2"}>{m.title}</span>
          </li>
        ))}
        {latest.milestones.length > 4 && <li className="text-[12.5px] text-muted-foreground">還有 {latest.milestones.length - 4} 個</li>}
      </ol>
    </div>
  );
}

export function Overview() {
  const { project, tickets, status, refreshTickets } = useProject();
  const open = tickets.data?.tickets?.open ?? [];
  const needs = open.filter((t) => t.stage === "needs_you");
  const target = status.data?.target.issue_id;
  const running = status.data?.run.state === "running";
  return (
    <>
      <AttentionStrip needs={needs} />
      <div className="mb-5 flex flex-wrap items-end gap-4">
        <div className="min-w-0">
          <h1 className="text-[34px] font-black leading-none tracking-tight">{project.name}</h1>
          <p className="mt-2 text-[15.5px] text-ink2">
            {running ? (target ? `工作中：執行者在處理工單 #${target}。` : "工作中。") : "目前沒有在工作。"}
            {tickets.data?.tickets && `已合併 ${tickets.data.tickets.done} 張，還有 ${open.length} 張沒完成。`}
          </p>
        </div>
        <div className="ml-auto flex gap-2">
          <Button variant="outline" onClick={refreshTickets}>
            <RefreshCw aria-hidden /> 重新問 GitHub
          </Button>
          <Button onClick={() => navigate(href({ project: project.id, page: "board", rest: [] }))}>
            看工作看板 <ArrowRight aria-hidden />
          </Button>
        </div>
      </div>
      <LineMap />
      <div className="mt-5 grid gap-5 lg:grid-cols-[minmax(0,1.35fr)_minmax(0,1fr)]">
        <Panel>
          <PanelHead title="需要你處理" right={<a className="text-[14px] font-bold text-ring underline-offset-4 hover:underline" href={href({ project: project.id, page: "inbox", rest: [] })}>全部</a>} />
          <NeedsYouList limit={3} />
        </Panel>
        <RecentActivity limit={6} />
      </div>
      <Panel className="mt-5">
        <div className="grid divide-y divide-border md:grid-cols-3 md:divide-x md:divide-y-0">
          <Tally />
          <RunState />
          <Milestones />
        </div>
      </Panel>
      {status.data?.warnings && status.data.warnings.length > 0 && (
        <a
          href={href({ project: project.id, page: "inbox", rest: [] })}
          className="mt-5 flex items-center gap-2 rounded-xl border border-border bg-card px-4 py-3 text-[14px] text-ink2 transition-colors hover:border-muted-foreground"
        >
          <TriangleAlert className="size-4 shrink-0 text-caution-ink" aria-hidden />
          awkit 有 {status.data.warnings.length} 個提醒，例如：{status.data.warnings[0]}
          <ArrowRight className="ml-auto size-4 shrink-0" aria-hidden />
        </a>
      )}
    </>
  );
}

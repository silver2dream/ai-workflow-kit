import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { ExternalLink, GitPullRequest, Hand, History, Link2, Play, ShieldAlert, Ticket as TicketIcon } from "lucide-react";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { DOT, Empty, Tag, TONE } from "@/components/bits";
import { useProject } from "@/app/project";
import { useOnboarding } from "@/onboarding/state";
import { approvalHow, awaitingApproval, laneMeta } from "@/lib/engine";
import { titleOf } from "@/lib/tickets";
import { describe, eventKey, STAGES, when } from "@/lib/words";
import { href, useRoute } from "@/lib/router";
import { cn } from "@/lib/utils";

interface SheetApi {
  open: (ticket: number) => void;
}

const Context = createContext<SheetApi>({ open: () => {} });

export function useTicketSheet() {
  return useContext(Context);
}

/** TicketSheetProvider shows a ticket beside the page, whichever page opened it. */
export function TicketSheetProvider({ children }: { children: ReactNode }) {
  const [number, setNumber] = useState<number | null>(null);
  const { mark } = useOnboarding();
  // A ticket belongs to the page it was opened on: going to another page
  // (by the address, or Back) closes it, rather than covering the new page.
  const route = useRoute();
  const here = `${route.project}/${route.page}`;
  const hereNow = useRef(here);
  hereNow.current = here;
  const [openedOn, setOpenedOn] = useState<string | null>(null);
  useEffect(() => {
    if (openedOn !== null && openedOn !== here) {
      setNumber(null);
      setOpenedOn(null);
    }
  }, [here, openedOn]);
  const open = useCallback(
    (ticket: number) => {
      setNumber(ticket);
      setOpenedOn(hereNow.current);
      mark("ticket");
    },
    [mark],
  );
  const value = useMemo(() => ({ open }), [open]);
  return (
    <Context.Provider value={value}>
      {children}
      <Sheet open={number !== null} onOpenChange={(o) => !o && setNumber(null)}>
        <SheetContent className="w-full gap-0 p-0 sm:max-w-[600px]">{number !== null && <TicketDetail number={number} onOpen={setNumber} />}</SheetContent>
      </Sheet>
    </Context.Provider>
  );
}

function TicketDetail({ number, onOpen }: { number: number; onOpen: (n: number) => void }) {
  const { project, tickets, events, status, engine } = useProject();
  const ticket = tickets.data?.tickets?.open?.find((t) => t.number === number);
  // A ticket that is done is still named.
  const title = titleOf(tickets.data, number);
  const history = (events.data?.events ?? []).filter((e) => e.issue_id === number);
  const prs = [...new Set(history.map((e) => e.pr_number).filter((n): n is number => !!n))];
  // A high-risk change that waits for its owner: its label says it needs
  // you, the engine says what for.
  const waiting = awaitingApproval(engine.data).get(number);
  const meta = waiting ? laneMeta(waiting) : ticket ? STAGES[ticket.stage] : STAGES.done;
  const prURL = (pr: number) => ticket?.url?.replace(/\/issues\/\d+$/, `/pull/${pr}`);

  return (
    <>
      <SheetHeader className="border-b border-border px-6 pb-4 pt-5">
        <div className="text-[13px] font-extrabold text-ink2 tnum">工單 #{number}</div>
        <SheetTitle className="text-[20px] font-extrabold leading-snug">{title ?? "這張工單不在進行中的清單裡"}</SheetTitle>
        <SheetDescription asChild>
          <div className="flex flex-wrap items-center gap-2">
            <Tag tone={meta.tone}>{ticket || waiting ? meta.label : "已合併或已關閉"}</Tag>
            <span className="text-[13.5px] text-muted-foreground">{ticket || waiting ? meta.means : "它可能已經合併，或在 GitHub 上被關掉了。"}</span>
          </div>
        </SheetDescription>
      </SheetHeader>
      <div className="flex-1 overflow-y-auto px-6 py-5">
        {waiting && (
          <div className="mb-6 overflow-hidden rounded-lg bg-signal/15 ring-1 ring-inset ring-[#D29A00]/50">
            <div className="hazard h-1.5" />
            <div className="p-4">
              <div className="flex items-center gap-2 text-[15px] font-extrabold">
                <ShieldAlert className="size-4" aria-hidden />
                等你核可
              </div>
              <p className="mt-1 text-[14.5px] leading-relaxed">審查通過了，但改動有風險，合併前要你看過。工單上的留言寫著是哪條規則、哪些檔案。</p>
              {waiting.note && <p className="mt-1 break-words text-[13px] text-muted-foreground">engine 的說明：{waiting.note}</p>}
              <p className="mt-2 text-[13.5px] leading-relaxed text-ink2">
                {approvalHow(number)}核可的會進合併佇列；退回的交回執行者，你寫的說明會交給它。拿掉 needs-human-review 標籤不算核可。在工作台裡直接核可或退回，會在之後的版本加上。
              </p>
            </div>
          </div>
        )}
        {!waiting && ticket?.stage === "needs_you" && (
          <div className="mb-6 overflow-hidden rounded-lg bg-signal/15 ring-1 ring-inset ring-[#D29A00]/50">
            <div className="hazard h-1.5" />
            <div className="p-4">
              <div className="flex items-center gap-2 text-[15px] font-extrabold">
                <Hand className="size-4" aria-hidden />
                要你決定怎麼辦
              </div>
              <p className="mt-1 text-[14.5px] leading-relaxed">{ticket.why ?? meta.means}</p>
              <p className="mt-2 text-[13.5px] leading-relaxed text-ink2">
                到 GitHub 看工單上的留言：可以補充說明，再把它交回執行者；也可以改工單的內容，或關掉它。在工作台裡直接處理，會在之後的版本加上。
              </p>
            </div>
          </div>
        )}
        {ticket && (ticket.stage === "ready" || ticket.stage === "rework") && status.data?.run.state !== "running" && (
          <div className="mb-6 flex flex-wrap items-center gap-3 rounded-lg border border-border p-3.5">
            <p className="min-w-0 flex-1 text-[14px] text-ink2">現在沒有在工作。可以只讓執行者做這一張，做完就停。</p>
            <a
              href={href({ project: project.id, page: "run", rest: [String(number)] })}
              className="inline-flex h-8 items-center gap-1.5 rounded-lg bg-primary px-2.5 text-sm font-medium text-primary-foreground hover:bg-primary/80"
            >
              <Play className="size-4" aria-hidden />
              只做這張
            </a>
          </div>
        )}
        {ticket?.waits_for && ticket.waits_for.length > 0 && (
          <>
            <h3 className="text-[13.5px] font-bold text-muted-foreground">要先完成</h3>
            <div className="mt-2 flex flex-wrap gap-1.5">
              {ticket.waits_for.map((n) => (
                <button key={n} type="button" onClick={() => onOpen(n)} className={cn("inline-flex items-center gap-1 rounded-md px-1.5 py-[2px] text-[12.5px] font-bold", TONE.muted)}>
                  <Link2 className="size-3.5" aria-hidden />
                  工單 #{n}
                </button>
              ))}
            </div>
          </>
        )}
        <h3 className={cn("text-[13.5px] font-bold text-muted-foreground", ticket?.waits_for?.length && "mt-6")}>經過</h3>
        {history.length === 0 ? (
          <Empty icon={History} title="這一輪還沒有它的紀錄">執行者開始寫、開 PR、審查時，每一步都會記在這裡。</Empty>
        ) : (
          <ol className="relative ml-3 mt-3 border-l-2 border-border">
            {history.map((event) => {
              const d = describe(event, events.data?.dispatcher);
              return (
                <li key={eventKey(event)} className="relative pb-4 pl-6 last:pb-0">
                  <span className={cn("absolute -left-[9px] top-1 size-4 rounded-full ring-2 ring-card", DOT[d.tone])} />
                  <p className="text-[14.5px]">
                    <b className="mr-1.5 text-[12.5px] text-ink2">{d.who}</b>
                    {d.text}
                  </p>
                  <time dateTime={event.ts} title={new Date(event.ts).toLocaleString()} className="mt-0.5 block text-[12.5px] text-muted-foreground tnum">
                    {when(event.ts)}
                  </time>
                </li>
              );
            })}
          </ol>
        )}
      </div>
      <footer className="border-t border-border px-6 py-4">
        <div className="mb-2 text-[13px] font-bold text-muted-foreground">在 GitHub 上看</div>
        <div className={cn("grid gap-2", prs.length > 0 && "sm:grid-cols-2")}>
          <GitHubLink icon={TicketIcon} href={ticket?.url} label={`工單 #${number}`} hint="要做什麼、驗收條件、留言" />
          {prs.map((pr) => (
            <GitHubLink key={pr} icon={GitPullRequest} href={prURL(pr)} label={`程式改動 PR #${pr}`} hint="改了哪些檔、審查意見、CI" />
          ))}
        </div>
      </footer>
    </>
  );
}

function GitHubLink({ icon: Icon, href, label, hint }: { icon: typeof TicketIcon; href?: string; label: ReactNode; hint: string }) {
  const body = (
    <>
      <span className="grid size-8 shrink-0 place-items-center rounded-md bg-muted text-ink2">
        <Icon className="size-4" aria-hidden />
      </span>
      <span className="min-w-0">
        <span className="block text-[14px] font-bold">{label}</span>
        <span className="block truncate text-[12.5px] text-muted-foreground">{href ? hint : "GitHub 沒有給這張工單的網址"}</span>
      </span>
      {href && <ExternalLink className="ml-auto size-4 text-muted-foreground" aria-hidden />}
    </>
  );
  return href ? (
    <a href={href} target="_blank" rel="noreferrer" className="flex items-center gap-3 rounded-lg px-3 py-2.5 ring-1 ring-inset ring-border transition-colors hover:bg-muted">
      {body}
    </a>
  ) : (
    <div className="flex items-center gap-3 rounded-lg px-3 py-2.5 opacity-70 ring-1 ring-inset ring-border">{body}</div>
  );
}

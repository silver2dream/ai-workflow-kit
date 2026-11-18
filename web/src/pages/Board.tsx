import { useEffect } from "react";
import { GitMerge, Hourglass, Inbox } from "lucide-react";
import { Skeleton } from "@/components/ui/skeleton";
import { PageHead, Problem, Term } from "@/components/bits";
import { useProject } from "@/app/project";
import { useTicketSheet } from "@/app/ticket-sheet";
import { STAGES } from "@/lib/words";
import type { Stage, Ticket } from "@/lib/api";
import { cn } from "@/lib/utils";

const COLUMNS: { key: string; title: string; stages: Stage[]; dot: string }[] = [
  { key: "needs", title: "需要你", stages: ["needs_you"], dot: "bg-signal" },
  { key: "code", title: "寫程式中", stages: ["in_progress", "rework"], dot: "bg-live" },
  { key: "review", title: "審查中", stages: ["in_review"], dot: "bg-review" },
  { key: "todo", title: "還沒開始", stages: ["ready", "waiting"], dot: "bg-border" },
];

function Card({ ticket }: { ticket: Ticket }) {
  const { open } = useTicketSheet();
  const meta = STAGES[ticket.stage];
  return (
    <button
      type="button"
      data-tour={ticket.stage === "needs_you" ? "ticketcard" : undefined}
      onClick={() => open(ticket.number)}
      className={cn(
        "w-full rounded-lg border bg-card p-3.5 text-left transition-colors",
        ticket.stage === "needs_you" ? "border-[#D29A00]/60" : "border-border hover:border-muted-foreground",
      )}
    >
      <div className="text-[12.5px] font-extrabold text-ink2 tnum">工單 #{ticket.number}</div>
      <div className="mt-1.5 text-[14.5px] font-bold leading-snug">{ticket.title}</div>
      {ticket.stage === "needs_you" ? (
        <div className="mt-3 rounded-md bg-signal/20 px-2.5 py-2 text-[13px] leading-snug text-signal-ink">
          <b>需要你。</b>
          {ticket.why ?? meta.means}
        </div>
      ) : ticket.waits_for && ticket.waits_for.length > 0 ? (
        <div className="mt-3 flex items-center gap-1.5 text-[12.5px] text-muted-foreground">
          <Hourglass className="size-3.5" aria-hidden />
          等 {ticket.waits_for.map((n) => `#${n}`).join("、")} 做完
        </div>
      ) : (
        <div className={cn("mt-3 text-[12.5px] font-bold", ticket.stage === "in_review" ? "text-review" : ticket.stage === "ready" ? "text-ink2" : "text-live")}>{meta.label}</div>
      )}
    </button>
  );
}

export function Board({ openTicket }: { openTicket?: number }) {
  const { tickets } = useProject();
  const { open } = useTicketSheet();
  useEffect(() => {
    if (openTicket) open(openTicket);
  }, [openTicket, open]);
  const list = tickets.data?.tickets?.open ?? [];
  const done = tickets.data?.tickets?.done ?? 0;

  return (
    <>
      <PageHead
        title="工作看板"
        sub={
          <>
            每一張進行中的<Term>工單</Term>。卡片會自己移動；點開看它的經過、在等誰、在 GitHub 上的工單和 <Term>PR</Term>。
          </>
        }
      />
      {tickets.error && !tickets.data && <Problem title="讀不到工單">{tickets.error.message}</Problem>}
      {tickets.data?.error && <Problem title="GitHub 問不到工單">{tickets.data.error}</Problem>}
      <div className="mt-2 grid gap-4 overflow-x-auto pb-2" style={{ gridTemplateColumns: "repeat(4, minmax(196px, 1fr)) minmax(148px, 0.75fr)" }} data-tour="board">
        {COLUMNS.map((column) => {
          const cards = list.filter((t) => column.stages.includes(t.stage));
          return (
            <section key={column.key} aria-label={column.title} className="min-h-[420px] rounded-xl bg-muted/70 p-2.5 ring-1 ring-inset ring-border">
              <h2 className="flex items-center gap-2 px-1.5 pb-3 pt-1 text-[14px] font-extrabold">
                <span className={cn("size-2.5 rounded-sm", column.dot)} />
                {column.title}
                <span className="ml-auto text-[13px] font-bold text-muted-foreground tnum">{cards.length}</span>
              </h2>
              <div className="grid gap-2.5">
                {tickets.loading && !tickets.data ? (
                  <Skeleton className="h-24" />
                ) : cards.length ? (
                  cards.map((t) => <Card key={t.number} ticket={t} />)
                ) : (
                  <p className="rounded-lg border-2 border-dashed border-border px-3 py-8 text-center text-[13px] text-muted-foreground">沒有</p>
                )}
              </div>
            </section>
          );
        })}
        <section aria-label="已合併" className="min-h-[420px] rounded-xl bg-muted/70 p-2.5 ring-1 ring-inset ring-border">
          <h2 className="flex items-center gap-2 px-1.5 pb-3 pt-1 text-[14px] font-extrabold">
            <span className="size-2.5 rounded-sm bg-go" />
            已合併
            <span className="ml-auto text-[13px] font-bold text-muted-foreground tnum">{done}</span>
          </h2>
          <div className="grid place-items-center rounded-lg bg-card px-3 py-8 text-center ring-1 ring-inset ring-border">
            <GitMerge className="size-6 text-go" aria-hidden />
            <p className="mt-2 text-[14px] font-bold tnum">{done} 張已合併</p>
            <p className="mt-1 text-[12.5px] text-muted-foreground">完成的工單在 GitHub 上已關閉，這裡只算數量。</p>
          </div>
        </section>
      </div>
      {!tickets.loading && list.length === 0 && done === 0 && !tickets.data?.error && (
        <p className="mt-4 flex items-center gap-2 text-[14px] text-muted-foreground">
          <Inbox className="size-4" aria-hidden />
          還沒有工單。議會規劃、你核可之後，工單會出現在這裡。
        </p>
      )}
    </>
  );
}

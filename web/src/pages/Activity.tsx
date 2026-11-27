import { useEffect, useState } from "react";
import { History } from "lucide-react";
import { Skeleton } from "@/components/ui/skeleton";
import { Empty, PageHead, Panel, Problem, TONE } from "@/components/bits";
import { useProject } from "@/app/project";
import { useTicketSheet } from "@/app/ticket-sheet";
import { useOnboarding } from "@/onboarding/state";
import { FILTERS, shownFilters } from "@/lib/activity";
import { describe, eventKey, when } from "@/lib/words";
import { cn } from "@/lib/utils";

export function ActivityPage() {
  const { events } = useProject();
  const { open } = useTicketSheet();
  const { mark } = useOnboarding();
  const [picked, setPicked] = useState("all");
  useEffect(() => mark("activity"), [mark]);

  const all = [...(events.data?.events ?? [])].reverse();
  const dispatcher = events.data?.dispatcher;
  const filters = shownFilters(all, dispatcher);
  // A filter the run has no use for (the Principal's, when the engine ran it) is no filter.
  const filter = filters.find((f) => f.key === picked) ?? FILTERS[0];
  const shown = all.filter((e) => filter.keep(e, dispatcher));

  return (
    <>
      <PageHead title="活動紀錄" sub="每一件事都記著：誰做的、做了什麼、結果如何。出問題時從這裡往回找。" />
      <div className="mb-4 flex flex-wrap gap-2" role="group" aria-label="篩選">
        {filters.map((f) => (
          <button
            key={f.key}
            type="button"
            aria-pressed={filter.key === f.key}
            onClick={() => setPicked(f.key)}
            className={cn(
              "rounded-md px-2.5 py-1 text-[13.5px] font-bold transition-colors",
              filter.key === f.key ? (f.key === "attention" ? TONE.caution : "bg-primary text-primary-foreground") : "bg-card text-ink2 ring-1 ring-inset ring-border hover:text-foreground",
            )}
          >
            {f.label}
          </button>
        ))}
        {events.data?.session && <span className="ml-auto self-center text-[12.5px] text-muted-foreground">這一輪：{events.data.session}</span>}
      </div>
      {events.error && !events.data && <Problem title="讀不到活動紀錄">{events.error.message}</Problem>}
      <Panel data-tour="activity">
        {events.loading && !events.data ? (
          <div className="grid gap-3 p-5">{[0, 1, 2, 3].map((i) => <Skeleton key={i} className="h-9" />)}</div>
        ) : shown.length === 0 ? (
          <Empty icon={History} title={all.length === 0 ? "還沒有工作紀錄" : "這一類沒有紀錄"}>
            {all.length === 0 ? "開始工作之後，engine、執行者、審查做的每一步都會記在這裡。" : "換一個篩選看看。"}
          </Empty>
        ) : (
          <ol className="px-5 py-1">
            {shown.map((event) => {
              const d = describe(event, dispatcher);
              return (
                <li key={eventKey(event)} className="flex items-start gap-4 border-t border-border py-3.5 first:border-t-0">
                  <time dateTime={event.ts} title={new Date(event.ts).toLocaleString()} className="w-[5.5rem] shrink-0 pt-1 text-[12.5px] text-muted-foreground tnum">
                    {when(event.ts)}
                  </time>
                  <span className={cn("mt-0.5 shrink-0 rounded-md px-1.5 py-0.5 text-[12px] font-bold", TONE[d.tone])}>{d.who}</span>
                  <p className="min-w-0 flex-1 break-words text-[15px]">{d.text}</p>
                  {event.issue_id ? (
                    <button type="button" onClick={() => open(event.issue_id!)} className="shrink-0 pt-0.5 text-[13px] font-bold text-ring underline-offset-4 hover:underline">
                      看工單 #{event.issue_id}
                    </button>
                  ) : null}
                </li>
              );
            })}
          </ol>
        )}
      </Panel>
    </>
  );
}

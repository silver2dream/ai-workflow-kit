import { ListChecks, ShieldAlert } from "lucide-react";
import { Skeleton } from "@/components/ui/skeleton";
import { Empty, Panel, PanelHead, Problem, Tag } from "@/components/bits";
import { useProject } from "@/app/project";
import { useTicketSheet } from "@/app/ticket-sheet";
import type { EngineView, Lane } from "@/lib/api";
import { laneMeta, stepWords, whyWords } from "@/lib/engine";
import { ruleWords, sourceWords } from "@/lib/guard";
import { titleOf } from "@/lib/tickets";
import { cn } from "@/lib/utils";
import { when } from "@/lib/words";

/** counts says how often a lane was tried again, of how often it may be: only the counts it used. */
export function counts(lane: Lane, limits: EngineView["limits"]): string[] {
  const out: string[] = [];
  const { failures = 0, reviews = 0, conflicts = 0 } = lane.attempts ?? {};
  if (failures > 0) out.push(`執行者失敗 ${failures}／${limits.failures} 次`);
  if (reviews > 0) out.push(`重新審查 ${reviews}／${limits.reviews} 次`);
  if (conflicts > 0) out.push(`送去解衝突 ${conflicts}／${limits.conflicts} 次`);
  return out;
}

function LaneRow({ lane, view }: { lane: Lane; view: EngineView }) {
  const { tickets } = useProject();
  const { open } = useTicketSheet();
  const meta = laneMeta(lane);
  const title = titleOf(tickets.data, lane.issue);
  const used = counts(lane, view.limits);
  const doing = lane.step ? stepWords(lane.step) : "";
  const over = lane.state === "merged" || lane.state === "completed" || lane.state === "cancelled";
  const verdict = lane.verdict;
  return (
    <li>
      <button
        type="button"
        onClick={() => open(lane.issue)}
        className={cn(
          "grid w-full gap-1.5 px-5 py-3.5 text-left transition-colors hover:bg-muted/60 focus-visible:bg-muted/60 focus-visible:outline-none",
          lane.state === "needs_you" && "bg-signal/10 hover:bg-signal/15",
        )}
      >
        <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1">
          <span className="text-[13px] font-extrabold text-ink2 tnum">工單 #{lane.issue}</span>
          <Tag tone={meta.tone}>{meta.label}</Tag>
          {lane.pr ? <span className="text-[13px] text-muted-foreground tnum">PR #{lane.pr}</span> : null}
          {lane.since && (
            <time dateTime={lane.since} title={new Date(lane.since).toLocaleString()} className="ml-auto text-[12.5px] text-muted-foreground tnum">
              {over ? when(lane.since) : `從 ${when(lane.since)}`}
            </time>
          )}
        </div>
        <div className={cn("min-w-0 break-words text-[15px] font-bold leading-snug", !title && "font-normal text-muted-foreground")}>{title ?? "標題還沒從 GitHub 讀到"}</div>
        {doing && (
          <div className="flex items-center gap-2 text-[13.5px] font-bold text-live">
            <span className="relative flex size-2 shrink-0" aria-hidden>
              <span className="absolute inset-0 animate-ping rounded-full bg-live opacity-60" />
              <span className="relative size-2 rounded-full bg-live" />
            </span>
            <span>
              {doing}
              {lane.step?.since ? `，從 ${when(lane.step.since)} 開始` : ""}
            </span>
          </div>
        )}
        {lane.state === "needs_you" && (
          <div className="text-[14px] leading-snug">
            <b>需要你。</b>
            {whyWords(lane.why) || meta.means}
            {/* What a guardrail stopped is said by its verdict, below. */}
            {lane.note && !(verdict && lane.why?.startsWith("guard_")) && (
              <span className="mt-0.5 block text-[12.5px] text-muted-foreground">engine 的說明：{lane.note}</span>
            )}
          </div>
        )}
        {used.length > 0 && <div className="text-[13px] text-ink2">{used.join(" · ")}</div>}
        {verdict && (
          <div className={cn("flex items-start gap-1.5 text-[13px] leading-snug", verdict.result === "block" ? "text-destructive" : "text-caution-ink")}>
            <ShieldAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden />
            <span>
              {verdict.result === "block" ? "防護擋下：" : "防護標記為可疑："}
              {sourceWords(verdict.source || verdict.piece || "") || "一段內容"}
              {ruleWords(verdict.rule, verdict.reason) ? `（${ruleWords(verdict.rule, verdict.reason)}）` : ""}
              <span className="text-muted-foreground"> · {when(verdict.at)}</span>
            </span>
          </div>
        )}
      </button>
    </li>
  );
}

/** Lanes are the tickets in the engine's hands, a row each: where each is, who works on it, how often it was tried. */
export function Lanes() {
  const { engine } = useProject();
  const view = engine.data;
  if (engine.loading && !view) {
    return (
      <Panel>
        <div className="grid gap-3 p-5">{[0, 1].map((i) => <Skeleton key={i} className="h-14" />)}</div>
      </Panel>
    );
  }
  // A project the engine never ran: nothing of it to show.
  if (!view || (!view.run && view.lanes.length === 0 && !view.problems?.length)) return null;
  const inHand = view.lanes.filter((l) => !["merged", "completed", "cancelled"].includes(l.state)).length;
  return (
    <Panel data-tour="runlanes" className="overflow-hidden">
      <PanelHead
        title="每張工單走到哪"
        sub={view.lanes.length > 0 ? `engine 手上有 ${inHand} 張，和上一輪做完的。點一列看那張工單。` : undefined}
      />
      {view.lanes.length === 0 ? (
        <Empty icon={ListChecks} title="engine 手上沒有工單">
          開始工作之後，engine 接下的每張工單會列在這裡：走到哪一步、誰在做、試了幾次。
        </Empty>
      ) : (
        <ul className="divide-y divide-border border-t border-border">
          {view.lanes.map((lane) => (
            <LaneRow key={lane.issue} lane={lane} view={view} />
          ))}
        </ul>
      )}
      {view.earlier > 0 && (
        <p className="border-t border-border px-5 py-3 text-[13px] text-muted-foreground">
          另外 {view.earlier} 張在上一輪之前就結束了；它們的經過在活動紀錄，或在終端機用 awkit events --issue 編號。
        </p>
      )}
      {view.problems && view.problems.length > 0 && (
        <div className="border-t border-border px-5 py-4">
          <Problem title="engine 的狀態有一部分讀不到">{view.problems.join("；")}</Problem>
        </div>
      )}
    </Panel>
  );
}

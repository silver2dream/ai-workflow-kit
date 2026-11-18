import { CircleCheck, Loader2 } from "lucide-react";
import { Panel, PanelHead } from "@/components/bits";
import { useProject } from "@/app/project";
import type { Entry, MeetingState, PlanDetail, Topic } from "@/lib/council";
import { cn } from "@/lib/utils";
import { EntryView } from "@/pages/plans/Entry";
import { RoleBadge, useSeats } from "@/pages/plans/roles";

/** liveTopic is the index of the topic the meeting is at, or was at when it stopped. */
export function liveTopic(detail: PlanDetail): number {
  const topics = detail.agenda?.topics ?? [];
  const at = (detail.meeting?.topic ?? 0) - 1;
  if (at >= 0 && at < topics.length) return at;
  // A meeting that stopped between topics, or never said where it was: the
  // last topic taken up.
  const started = (detail.record?.topics ?? []).filter((t) => t.started).map((t) => t.number - 1);
  return started.length ? Math.min(Math.max(...started), Math.max(0, topics.length - 1)) : 0;
}

/**
 * TopicStrip shows the topics of the agenda: done, under way, to come. A
 * topic picked shows what was said to it.
 */
export function TopicStrip({ detail, selected, onSelect }: { detail: PlanDetail; selected: number; onSelect: (i: number) => void }) {
  const topics = detail.agenda?.topics ?? [];
  const record = detail.record;
  const meeting = detail.meeting;
  if (topics.length === 0) return null;
  return (
    <ol className="grid gap-2 sm:grid-cols-2 2xl:grid-cols-4" aria-label="議題">
      {topics.map((topic, i) => {
        const held = record?.topics?.[i]?.held;
        const live = !held && meeting?.state === "meeting" && meeting.topic === i + 1;
        const started = record?.topics?.[i]?.started || live;
        const on = i === selected;
        return (
          <li key={topic.id}>
            <button
              type="button"
              onClick={() => onSelect(i)}
              aria-pressed={on}
              aria-current={live ? "step" : undefined}
              className={cn(
                "h-full w-full cursor-pointer rounded-lg border px-3.5 py-2.5 text-left transition-colors hover:border-muted-foreground",
                live ? "border-live bg-live/5" : "border-border bg-card",
                on && "ring-2 ring-foreground ring-offset-2 ring-offset-background",
              )}
            >
              <span className={cn("flex items-center gap-1.5 text-[12.5px] font-bold", live ? "text-live" : held ? "text-go" : "text-muted-foreground")}>
                {live ? <Loader2 className="size-3.5 animate-spin" aria-hidden /> : held ? <CircleCheck className="size-3.5" aria-hidden /> : null}
                {live ? "討論中" : held ? "談完了" : started ? "停在這題" : "還沒談"}
                <span className="ml-auto font-normal text-muted-foreground tnum">第 {i + 1} 題</span>
              </span>
              <span className="mt-1 line-clamp-2 block text-[14px] font-bold leading-snug">{topic.title}</span>
            </button>
          </li>
        );
      })}
    </ol>
  );
}

const seatAt = (i: number, n: number): [number, number] => {
  const angle = -Math.PI / 2 + (i / n) * 2 * Math.PI;
  return [50 + 41 * Math.cos(angle), 50 + 34 * Math.sin(angle)];
};

/** Roundtable shows who sits at the table, who speaks to the topic, and who speaks now. */
export function Roundtable({ topic, number, of, speaking }: { topic?: Topic; number: number; of: number; speaking?: string }) {
  const { roster } = useProject();
  const seat = useSeats();
  const roles = roster.data?.roles ?? [];
  const here = topic ? [topic.lead, ...topic.speakers] : [];
  const ids = roles.map((r) => r.id);
  return (
    <div data-tour="roundtable">
      {/* Narrow windows: a grid of seats. */}
      <div className="sm:hidden">
        <div className="text-[13px] font-bold text-muted-foreground">
          第 {number} 題，共 {of} 題
        </div>
        <div className="mt-0.5 text-[16px] font-extrabold leading-snug">{topic?.title ?? "—"}</div>
        <ul className="mt-3 grid grid-cols-3 gap-2">
          {ids.map((id) => {
            const on = id === speaking;
            const at = here.includes(id);
            return (
              <li key={id} className={cn("flex flex-col items-center gap-1 rounded-lg p-2 text-center", on && "bg-muted ring-2", !at && !on && "opacity-40")} style={{ ["--tw-ring-color" as string]: seat(id).color }}>
                <RoleBadge seat={seat(id)} size={34} />
                <span className="text-[12.5px] font-bold leading-tight">{seat(id).title}</span>
                <span className="text-[11.5px] text-muted-foreground">{on ? "發言中" : at ? "在座" : "這題沒參加"}</span>
              </li>
            );
          })}
        </ul>
      </div>
      <div className="relative mx-auto hidden aspect-[2/1] w-full max-w-[560px] sm:block">
        <div className="absolute inset-[16%_9%] rounded-[50%] border-[5px] border-foreground/80" />
        <div className="absolute inset-[27%_24%] grid place-items-center text-center">
          <div>
            <div className="text-[13px] font-bold text-muted-foreground">
              第 {number} 題，共 {of} 題
            </div>
            <div className="mx-auto mt-1 line-clamp-3 max-w-[230px] text-[16px] font-extrabold leading-snug">{topic?.title ?? "—"}</div>
            {speaking === "moderator" && <div className="mt-2 text-[12.5px] font-bold text-live">主持人整理中</div>}
          </div>
        </div>
        {ids.map((id, i) => {
          const [x, y] = seatAt(i, ids.length);
          const on = id === speaking;
          const at = here.includes(id);
          const s = seat(id);
          return (
            <div key={id} className="absolute flex -translate-x-1/2 -translate-y-1/2 flex-col items-center" style={{ left: `${x}%`, top: `${y}%` }}>
              <span className={cn("relative grid place-items-center rounded-[11px] bg-card p-1", on && "ring-[3px]")} style={{ ["--tw-ring-color" as string]: s.color }}>
                {on && <span className="absolute -inset-2 animate-ping rounded-xl opacity-40" style={{ background: s.color }} aria-hidden />}
                <span className={cn(!at && !on && "opacity-35 grayscale")}>
                  <RoleBadge seat={s} size={on ? 46 : 38} />
                </span>
              </span>
              <span className={cn("mt-1 whitespace-nowrap rounded-md px-2 py-0.5 text-[12.5px] font-bold", on ? "bg-foreground text-background" : at ? "bg-card text-ink2" : "bg-card text-muted-foreground")}>
                {s.title}
                {on ? "，發言中" : at ? "" : "，這題沒參加"}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}

/**
 * byTopic puts what was said with the topic it was said at. What was said
 * outside a topic goes with the topic before it: the agenda and the answers
 * open the first, the work list closes the last, and a word taken in between
 * two topics stays with the one that was just held.
 */
export function byTopic(entries: Entry[], topics: Topic[]): Map<string, Entry[]> {
  const groups = new Map<string, Entry[]>(topics.map((t) => [t.id, []]));
  let current = topics[0]?.id;
  for (const entry of [...entries].sort((a, b) => a.seq - b.seq)) {
    if (entry.topic && groups.has(entry.topic)) current = entry.topic;
    if (current) groups.get(current)?.push(entry);
  }
  return groups;
}

/** Thinking says who is asked now, while the meeting waits for an answer. */
function Thinking({ meeting }: { meeting: MeetingState }) {
  const seat = useSeats();
  if (meeting.state !== "meeting") return null;
  const who = meeting.speaker ? seat(meeting.speaker) : undefined;
  return (
    <div className="flex items-center gap-3 text-muted-foreground" role="status">
      {who ? <RoleBadge seat={who} size={28} /> : <Loader2 className="size-5 animate-spin" aria-hidden />}
      <span className="flex gap-1" aria-hidden>
        {[0, 1, 2].map((i) => (
          <span key={i} className="size-1.5 animate-bounce rounded-full bg-current" style={{ animationDelay: `${i * 150}ms` }} />
        ))}
      </span>
      <span className="text-[13.5px]">{who ? `${who.title}在想` : meeting.step?.includes("documents") ? "各角色在寫文件" : meeting.step?.includes("disputes") ? "兩邊在對辯" : "議會在處理"}</span>
    </div>
  );
}

/** Proceedings shows what was said to a topic, newest last. */
export function Proceedings({ detail, entries, topicIndex }: { detail: PlanDetail; entries: Entry[]; topicIndex: number }) {
  const topics = detail.agenda?.topics ?? [];
  const topic = topics[topicIndex];
  const shown = (topic ? byTopic(entries, topics).get(topic.id) : entries) ?? [];
  const meeting = detail.meeting;
  const here = meeting?.state === "meeting" && topicIndex === liveTopic(detail);
  return (
    <Panel data-tour="proceedings">
      <PanelHead
        title={topic ? `第 ${topicIndex + 1} 題的經過` : "會議經過"}
        sub={topic?.question}
        right={<span className="whitespace-nowrap text-[13px] text-muted-foreground tnum">{shown.length} 則</span>}
      />
      <div className="grid grid-cols-[minmax(0,1fr)] gap-5 px-5 pb-5">
        {shown.length === 0 && !here && <p className="text-[14px] text-muted-foreground">這一題還沒有人發言。</p>}
        {shown.map((entry) => (
          <EntryView key={`${entry.seq}-${entry.step}`} entry={entry} />
        ))}
        {here && meeting && <Thinking meeting={meeting} />}
      </div>
    </Panel>
  );
}

/** Usage shows the model calls of this run against what the roster allows. */
export function Usage({ meeting, decisions, documents }: { meeting: MeetingState; decisions: number; documents: number }) {
  const share = meeting.limit > 0 ? Math.min(100, Math.round((meeting.calls / meeting.limit) * 100)) : 0;
  return (
    <Panel>
      <div className="p-5">
        <div className="flex items-center justify-between text-[13.5px] text-muted-foreground">
          <span>這一輪的模型呼叫</span>
          <span className="font-bold text-foreground tnum">
            {meeting.calls} / {meeting.limit}
          </span>
        </div>
        <div className="mt-2 h-2 overflow-hidden rounded-full bg-muted ring-1 ring-inset ring-border">
          <div className={cn("h-full rounded-full", share > 85 ? "bg-caution" : "bg-live")} style={{ width: `${share}%` }} />
        </div>
        <div className="mt-4 grid grid-cols-2 text-center">
          <div>
            <div className="text-[22px] font-black tnum">{decisions}</div>
            <div className="text-[12.5px] text-muted-foreground">決策</div>
          </div>
          <div>
            <div className="text-[22px] font-black tnum">{documents}</div>
            <div className="text-[12.5px] text-muted-foreground">文件</div>
          </div>
        </div>
      </div>
    </Panel>
  );
}

import { useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { ArrowLeftRight, Check, CircleHelp, ClipboardList, FileText, Flag, Gavel, ListOrdered, MessageSquareQuote, PauseCircle, RefreshCcw, Ticket as TicketIcon, TriangleAlert } from "lucide-react";
import { Tag } from "@/components/bits";
import type {
  Agenda,
  Assumption,
  Decision,
  Entry,
  ExchangeEntry,
  Milestone,
  OwnerWord,
  Review,
  RuleCheck,
  Speech,
  Summary,
  Ticket,
  WrittenDocument,
} from "@/lib/council";
import { when } from "@/lib/words";
import { cn } from "@/lib/utils";
import { RoleBadge, RoleName, useSeats } from "@/pages/plans/roles";

const list = (items?: string[] | null) => (items ?? []).filter((s) => s && s.trim());

/**
 * Clamp shows the start of something long, fading out, with a button that
 * shows all of it: a role can say a page at a time, and the meeting is read
 * by skimming.
 */
export function Clamp({ children, max = 260 }: { children: ReactNode; max?: number }) {
  const ref = useRef<HTMLDivElement>(null);
  const [long, setLong] = useState(false);
  const [open, setOpen] = useState(false);
  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    const check = () => setLong(el.scrollHeight > max + 48);
    check();
    if (typeof ResizeObserver === "undefined") return;
    const watch = new ResizeObserver(check);
    watch.observe(el);
    return () => watch.disconnect();
  }, [max]);
  const shut = long && !open;
  const fade = "linear-gradient(to bottom, #000 60%, transparent)";
  return (
    <>
      <div ref={ref} className={shut ? "overflow-hidden" : undefined} style={shut ? { maxHeight: max, maskImage: fade, WebkitMaskImage: fade } : undefined}>
        {children}
      </div>
      {long && (
        <button type="button" onClick={() => setOpen((o) => !o)} aria-expanded={open} className="mt-1.5 cursor-pointer text-[13.5px] font-bold text-ring underline-offset-4 hover:underline">
          {open ? "收起" : "看全文"}
        </button>
      )}
    </>
  );
}

function Lines({ items, icon: Icon, className }: { items?: string[] | null; icon?: typeof Check; className?: string }) {
  const shown = list(items);
  if (shown.length === 0) return null;
  return (
    <ul className={cn("mt-1.5 grid gap-1", className)}>
      {shown.map((item, i) => (
        <li key={i} className="flex gap-1.5 text-[14px] leading-relaxed">
          {Icon ? <Icon className="mt-1 size-3.5 shrink-0" aria-hidden /> : <span aria-hidden className="mt-[9px] size-1 shrink-0 rounded-full bg-current opacity-60" />}
          <span className="min-w-0">{item}</span>
        </li>
      ))}
    </ul>
  );
}

function SpeechView({ speech, role, proposes }: { speech: Speech; role?: string; proposes?: boolean }) {
  const seat = useSeats();
  const me = seat(role ?? speech.role);
  return (
    <div className="flex gap-3">
      <RoleBadge seat={me} size={34} />
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2 text-[14px]">
          <span className="font-extrabold">{me.title}</span>
          {proposes && <Tag tone="focus">提出方案</Tag>}
        </div>
        <Clamp>
          <p className="mt-1 whitespace-pre-line text-[15px] leading-relaxed">{speech.says}</p>
          {list(speech.proposals).length > 0 && (
            <div className="mt-2 text-ink2">
              <div className="text-[12.5px] font-bold text-muted-foreground">提議</div>
              <Lines items={speech.proposals} />
            </div>
          )}
          {list(speech.concerns).length > 0 && (
            <div className="mt-2 text-ink2">
              <div className="text-[12.5px] font-bold text-muted-foreground">擔心</div>
              <Lines items={speech.concerns} />
            </div>
          )}
        </Clamp>
        {(speech.objections ?? []).map((o, i) => (
          <div key={i} className="mt-2.5 rounded-lg border-l-4 border-caution bg-caution/10 px-4 py-2.5">
            <div className="flex items-center gap-1.5 text-[13.5px] font-extrabold text-caution-ink">
              <Flag className="size-3.5" aria-hidden />對{seat(o.to).title}提出異議
            </div>
            <p className="mt-0.5 text-[14.5px]">{o.point}</p>
            {o.wants && <p className="mt-0.5 text-[13.5px] text-ink2">想要：{o.wants}</p>}
          </div>
        ))}
      </div>
    </div>
  );
}

const STANCE: Record<string, string> = { concede: "讓步", revise: "修正", hold: "堅持" };

function ExchangeView({ data, role }: { data: ExchangeEntry; role?: string }) {
  const seat = useSeats();
  const answering = role === data.defender;
  const ex = data.exchange;
  return (
    <div className="rounded-lg bg-muted px-4 py-3 ring-1 ring-inset ring-border">
      <div className="flex flex-wrap items-center gap-2 text-[13px] text-muted-foreground">
        <RoleBadge seat={seat(data.challenger)} size={22} />
        <ArrowLeftRight className="size-3.5" aria-hidden />
        <RoleBadge seat={seat(data.defender)} size={22} />
        <span className="font-bold">
          {seat(data.challenger).title} 對 {seat(data.defender).title}，對辯第 {data.round} 回合
        </span>
      </div>
      <p className="mt-1.5 text-[13.5px] leading-relaxed text-ink2">爭點：{data.point}</p>
      <Clamp max={220}>
        {answering ? (
          <p className="mt-2 whitespace-pre-line text-[14.5px] leading-relaxed">
            <b>{seat(data.defender).title}{STANCE[ex.stance] ? `（${STANCE[ex.stance]}）` : ""}：</b>
            {ex.reply}
          </p>
        ) : (
          <p className="mt-2 whitespace-pre-line text-[14.5px] leading-relaxed">
            <b>{seat(data.challenger).title}：</b>
            {ex.answer}
            {ex.accepts ? <Tag tone="go" className="ml-2">談妥了</Tag> : ex.still_wants ? <span className="text-ink2">（還是想要：{ex.still_wants}）</span> : null}
          </p>
        )}
      </Clamp>
    </div>
  );
}

function SummaryView({ summary }: { summary: Summary }) {
  return (
    <div className="rounded-lg border border-border p-4">
      <div className="flex items-center gap-2 text-[14px] font-extrabold">
        <ClipboardList className="size-4 text-muted-foreground" aria-hidden />
        主持人整理
      </div>
      <Clamp max={300}>
        <div className="mt-3 grid gap-4 text-[14px] md:grid-cols-3">
          <div className="min-w-0">
            <div className="text-[13px] font-bold text-go">談定了</div>
            {list(summary.consensus).length ? <Lines items={summary.consensus} icon={Check} className="text-foreground" /> : <p className="mt-1 text-muted-foreground">沒有</p>}
          </div>
          <div className="min-w-0">
            <div className="text-[13px] font-bold text-caution-ink">還有爭議</div>
            {list(summary.open).length ? <Lines items={summary.open} /> : <p className="mt-1 text-muted-foreground">沒有</p>}
          </div>
          <div className="min-w-0">
            <div className="text-[13px] font-bold text-ring">只有你答得出來</div>
            {list(summary.questions).length ? <Lines items={summary.questions} icon={CircleHelp} /> : <p className="mt-1 text-muted-foreground">沒有</p>}
          </div>
        </div>
      </Clamp>
    </div>
  );
}

export function DecisionLine({ decision, state }: { decision: Decision; state?: { replacedBy?: string; withdrawn?: string } }) {
  const gone = !!(state?.replacedBy || state?.withdrawn);
  return (
    <li className="flex gap-3">
      <span className={cn("mt-0.5 w-9 shrink-0 text-[13px] font-black tnum", gone ? "text-muted-foreground line-through" : "text-ring")}>{decision.id}</span>
      <div className="min-w-0">
        <div className={cn("text-[15px] font-bold", gone && "text-muted-foreground line-through")}>
          {decision.title}
          {decision.replaces && <span className="ml-2 text-[12.5px] font-bold text-caution-ink">取代 {decision.replaces}</span>}
        </div>
        <p className={cn("whitespace-pre-line text-[14.5px] leading-relaxed", gone ? "text-muted-foreground" : "text-ink2")}>{decision.decision}</p>
        {state?.replacedBy && <p className="mt-0.5 text-[13px] font-bold text-caution-ink">被 {state.replacedBy} 取代</p>}
        {state?.withdrawn && <p className="mt-0.5 text-[13px] font-bold text-caution-ink">撤回：{state.withdrawn}</p>}
        {decision.dissent && !gone && <p className="mt-1 text-[13px] text-muted-foreground">不同意見：{decision.dissent}</p>}
      </div>
    </li>
  );
}

export function AssumptionLine({ assumption, answer }: { assumption: Assumption; answer?: string }) {
  return (
    <li className="flex gap-3">
      <span className="mt-0.5 w-9 shrink-0 text-[13px] font-black text-ring tnum">{assumption.id}</span>
      <div className="min-w-0">
        <div className="text-[15px] font-bold">{assumption.question || assumption.assumption}</div>
        {answer ? (
          <p className="text-[14px] text-go">你答了：{answer}</p>
        ) : (
          <>
            {assumption.question && <p className="text-[14px] text-ink2">先假設：{assumption.assumption}</p>}
            {assumption.if_wrong && <p className="text-[13px] text-muted-foreground">不是的話：{assumption.if_wrong}</p>}
          </>
        )}
      </div>
    </li>
  );
}

function RulingView({ decisions, assumptions, role }: { decisions: Decision[]; assumptions: Assumption[]; role?: string }) {
  const seat = useSeats();
  return (
    <div className="rounded-lg border-2 border-ring/40 bg-ring/5 p-4">
      <div className="flex items-center gap-2 text-[14px] font-extrabold">
        <Gavel className="size-4 text-ring" aria-hidden />
        <RoleName seat={seat(role)} size={22} />
        裁定：{decisions.length} 條決策{assumptions.length ? `、${assumptions.length} 個假設` : ""}
      </div>
      <Clamp max={360}>
        <ul className="mt-3 grid gap-3">
          {decisions.map((d) => (
            <DecisionLine key={d.id ?? d.title} decision={d} />
          ))}
        </ul>
        {assumptions.length > 0 && (
          <>
            <div className="mt-4 text-[13px] font-bold text-muted-foreground">先假設，等你確認</div>
            <ul className="mt-2 grid gap-2.5">
              {assumptions.map((a) => (
                <AssumptionLine key={a.id ?? a.question} assumption={a} />
              ))}
            </ul>
          </>
        )}
      </Clamp>
    </div>
  );
}

export function DocumentList({ documents, check }: { documents: WrittenDocument[]; check?: RuleCheck | null }) {
  const open = check?.open ?? [];
  return (
    <ul className="mt-2 grid gap-1.5">
      {documents.map((d, i) => (
        <li key={`${d.path}-${i}`} className="flex flex-wrap items-center gap-2 text-[13.5px]">
          <FileText className="size-3.5 text-muted-foreground" aria-hidden />
          <span className="min-w-0 break-all font-mono text-[12.5px]">{d.path}</span>
          {d.refused ? <Tag tone="caution">沒寫：{d.refused}</Tag> : d.unchanged ? <Tag>沒有改</Tag> : <Tag tone="go">寫好了</Tag>}
          {open.filter((f) => f.document === d.path).length > 0 && <Tag tone="caution" icon={TriangleAlert}>{open.filter((f) => f.document === d.path).length} 處和規則不符</Tag>}
        </li>
      ))}
    </ul>
  );
}

function DocumentsView({ data, role }: { data: { documents: WrittenDocument[]; check?: RuleCheck | null }; role?: string }) {
  const seat = useSeats();
  const found = data.check?.found?.length ?? 0;
  return (
    <div className="flex gap-3">
      <RoleBadge seat={seat(role)} size={34} />
      <div className="min-w-0 flex-1">
        <div className="text-[14px] font-extrabold">{seat(role).title}寫文件</div>
        <DocumentList documents={data.documents ?? []} check={data.check} />
        {found > 0 && (
          <p className="mt-1.5 text-[13px] text-muted-foreground">
            照專案規則讀過：找到 {found} 處，{(data.check?.open?.length ?? 0) === 0 ? "都改好了" : `還有 ${data.check?.open?.length} 處沒改`}。
          </p>
        )}
      </div>
    </div>
  );
}

export function WordView({ word, mine = true }: { word: OwnerWord; mine?: boolean }) {
  return (
    <div className="rounded-lg bg-signal/15 px-4 py-3 ring-1 ring-inset ring-[#D29A00]/50">
      <div className="flex items-center gap-2 text-[13.5px] font-extrabold">
        <MessageSquareQuote className="size-4" aria-hidden />
        {mine ? "你說" : "擁有者說"}（{word.id}）
        <span className="font-normal text-muted-foreground tnum">{when(word.at)}</span>
      </div>
      {word.question && <p className="mt-1 text-[13px] text-ink2">回答：{word.question}</p>}
      <p className="mt-1 whitespace-pre-line text-[15px] leading-relaxed">{word.text}</p>
    </div>
  );
}

export function ReviewView({ review, role }: { review: Review; role?: string }) {
  const seat = useSeats();
  const decisions = review.decisions ?? [];
  const withdrawn = review.withdrawn ?? [];
  const answered = review.answered ?? [];
  const docs = review.documents ?? [];
  return (
    <div className="rounded-lg border-2 border-caution/50 bg-caution/5 p-4">
      <div className="flex flex-wrap items-center gap-2 text-[14px] font-extrabold">
        <RefreshCcw className="size-4 text-caution-ink" aria-hidden />
        <RoleName seat={seat(role)} size={22} />
        照你說的（{review.words.map((w) => w.id).join("、")}）重審了所有決策
      </div>
      <Clamp max={360}>
      {review.note && <p className="mt-2 whitespace-pre-line text-[14.5px] leading-relaxed">{review.note}</p>}
      {decisions.length + withdrawn.length + answered.length === 0 ? (
        <p className="mt-2 text-[14px] text-ink2">每一條決策都照舊。</p>
      ) : (
        <ul className="mt-3 grid gap-3">
          {decisions.map((d) => (
            <DecisionLine key={d.id ?? d.title} decision={d} />
          ))}
          {withdrawn.map((w) => (
            <li key={w.id} className="text-[14px]">
              <b className="text-caution-ink">{w.id} 撤回</b>：{w.why}
            </li>
          ))}
          {answered.map((a) => (
            <li key={a.id} className="text-[14px]">
              <b className="text-go">{a.id} 有答案了</b>：{a.answer}
            </li>
          ))}
        </ul>
      )}
      {docs.length > 0 && (
        <>
          <div className="mt-4 text-[13px] font-bold text-muted-foreground">整份重看的文件</div>
          <DocumentList documents={docs} />
        </>
      )}
      </Clamp>
    </div>
  );
}

function AgendaView({ agenda }: { agenda: Agenda }) {
  const seat = useSeats();
  return (
    <div className="rounded-lg border border-border p-4">
      <div className="flex items-center gap-2 text-[14px] font-extrabold">
        <ListOrdered className="size-4 text-muted-foreground" aria-hidden />
        主持人排了議程：{agenda.topics.length} 個議題
      </div>
      <ol className="mt-2 grid gap-1.5">
        {agenda.topics.map((t, i) => (
          <li key={t.id} className="flex items-start gap-2 text-[14px]">
            <span className="w-5 shrink-0 text-right font-bold text-muted-foreground tnum">{i + 1}.</span>
            <span className="min-w-0 flex-1 font-bold">{t.title}</span>
            <RoleName seat={seat(t.lead)} size={20} note={<span className="text-[12.5px] text-muted-foreground">主講</span>} />
          </li>
        ))}
      </ol>
    </div>
  );
}

function WorkListView({ data }: { data: { milestones?: Milestone[] | null; tickets?: Ticket[] | null } }) {
  return (
    <div className="flex items-center gap-3 rounded-lg border border-go/50 bg-go/5 p-4 text-[14.5px]">
      <TicketIcon className="size-5 text-go" aria-hidden />
      規劃角色排好了 {data.milestones?.length ?? 0} 個里程碑，第一個里程碑擬了 {data.tickets?.length ?? 0} 張工單。
    </div>
  );
}

/** EntryView shows one thing said at a meeting, in the form its kind has. */
export function EntryView({ entry }: { entry: Entry }) {
  const d = entry.data as never;
  switch (entry.kind) {
    case "proposal":
      return <SpeechView speech={d} role={entry.role} proposes />;
    case "speech":
      return <SpeechView speech={d} role={entry.role} />;
    case "exchange":
      return <ExchangeView data={d} role={entry.role} />;
    case "summary":
      return <SummaryView summary={d} />;
    case "ruling": {
      const r = entry.data as { decisions?: Decision[]; assumptions?: Assumption[] };
      return <RulingView decisions={r.decisions ?? []} assumptions={r.assumptions ?? []} role={entry.role} />;
    }
    case "documents":
      return <DocumentsView data={d} role={entry.role} />;
    case "word":
      return <WordView word={d} />;
    case "review":
      return <ReviewView review={d} role={entry.role} />;
    case "agenda":
      return <AgendaView agenda={d} />;
    case "worklist":
      return <WorkListView data={d} />;
    case "questions":
      return (
        <div className="rounded-lg border border-border p-4 text-[14px]">
          <div className="flex items-center gap-2 font-extrabold">
            <CircleHelp className="size-4 text-ring" aria-hidden />
            議會有問題要問你
          </div>
          <Lines items={entry.data as string[]} />
        </div>
      );
    case "stopped":
      return (
        <div className="flex items-center gap-2 text-[13.5px] text-muted-foreground">
          <PauseCircle className="size-4" aria-hidden />
          這一輪會議在 {when(entry.at)} 停下
        </div>
      );
    default:
      return null;
  }
}

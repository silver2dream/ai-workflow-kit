import { useState } from "react";
import { ArrowRight, CircleCheck, ExternalLink, Flag, Pencil, Rocket, Trash2, TriangleAlert } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Empty, Panel, PanelHead, Problem, Tag } from "@/components/bits";
import { useProject } from "@/app/project";
import { council, type ListObjection, type Milestone, type PlanDetail, type Ticket, type TicketText } from "@/lib/council";
import { href } from "@/lib/router";
import { cn } from "@/lib/utils";
import { useSeats } from "@/pages/plans/roles";

const lines = (items?: string[] | null) => (items ?? []).filter((s) => s.trim());

function Points({ title, items }: { title: string; items?: string[] | null }) {
  const shown = lines(items);
  if (shown.length === 0) return null;
  return (
    <div>
      <div className="text-[12.5px] font-bold text-muted-foreground">{title}</div>
      <ul className="mt-1 grid gap-1">
        {shown.map((item, i) => (
          <li key={i} className="flex gap-1.5 text-[14px] leading-relaxed">
            <span aria-hidden className="mt-[9px] size-1 shrink-0 rounded-full bg-current opacity-60" />
            <span className="min-w-0">{item}</span>
          </li>
        ))}
      </ul>
    </div>
  );
}

function TicketCard({
  ticket,
  objections,
  locked,
  onEdit,
  onDrop,
}: {
  ticket: Ticket;
  objections: ListObjection[];
  /** Why the ticket cannot be changed now, if it cannot. */
  locked?: string;
  onEdit: () => void;
  onDrop: () => void;
}) {
  const { project } = useProject();
  const seat = useSeats();
  const [open, setOpen] = useState(false);
  const more = lines(ticket.scope).length + lines(ticket.acceptance).length + lines(ticket.testing).length > 0;
  return (
    <li className="rounded-lg border border-border bg-card p-4" data-ticket={ticket.key}>
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-mono text-[12.5px] font-bold text-muted-foreground">{ticket.key}</span>
        <Tag>{ticket.repo}</Tag>
        {ticket.issue ? (
          <a href={href({ project: project.id, page: "board", rest: [String(ticket.issue)] })} className="inline-flex items-center gap-1 text-[13px] font-bold text-go underline-offset-4 hover:underline">
            <CircleCheck className="size-3.5" aria-hidden />
            已建成工單 #{ticket.issue}
          </a>
        ) : null}
        {!ticket.issue && (
          <span className="ml-auto flex gap-1">
            <Button size="sm" variant="ghost" onClick={onEdit} disabled={!!locked} title={locked} aria-label={`改工單 ${ticket.key}`}>
              <Pencil aria-hidden /> 改
            </Button>
            <Button size="sm" variant="ghost" onClick={onDrop} disabled={!!locked} title={locked} aria-label={`刪掉工單 ${ticket.key}`} className="text-destructive hover:text-destructive">
              <Trash2 aria-hidden /> 刪
            </Button>
          </span>
        )}
      </div>
      <h3 className="mt-1.5 text-[16px] font-extrabold leading-snug">{ticket.title}</h3>
      {ticket.summary && <p className="mt-1 whitespace-pre-line text-[14.5px] leading-relaxed text-ink2">{ticket.summary}</p>}
      {lines(ticket.depends_on).length > 0 && <p className="mt-1.5 text-[13px] text-muted-foreground">要等這些先做完：{lines(ticket.depends_on).join("、")}</p>}
      {objections.map((o, i) => (
        <div key={i} className="mt-2.5 rounded-md border-l-4 border-caution bg-caution/10 px-3 py-2 text-[13.5px]">
          <span className="flex items-center gap-1.5 font-bold text-caution-ink">
            <Flag className="size-3.5" aria-hidden />
            {o.from ? `${seat(o.from).title}還有意見` : "還有意見"}
          </span>
          {o.point}
          {o.wants && <span className="text-ink2">（想要：{o.wants}）</span>}
        </div>
      ))}
      {more && (
        <>
          <button type="button" onClick={() => setOpen((o) => !o)} aria-expanded={open} className="mt-2 cursor-pointer text-[13.5px] font-bold text-ring underline-offset-4 hover:underline">
            {open ? "收起範圍和驗收條件" : "看範圍、驗收條件和測試"}
          </button>
          {open && (
            <div className="mt-2 grid gap-3 rounded-md bg-muted/60 p-3">
              <Points title="範圍" items={ticket.scope} />
              <Points title="驗收條件" items={ticket.acceptance} />
              <Points title="怎麼測" items={ticket.testing} />
            </div>
          )}
        </>
      )}
    </li>
  );
}

const asLines = (text: string) => text.split("\n").map((s) => s.trim()).filter(Boolean);

/** TicketEditor changes what a ticket says before it is on GitHub. */
function TicketEditor({ planId, ticket, onClose, onSaved }: { planId: string; ticket: Ticket | null; onClose: () => void; onSaved: () => void }) {
  const { project } = useProject();
  const [draft, setDraft] = useState({ title: "", summary: "", scope: "", acceptance: "", testing: "" });
  const [problem, setProblem] = useState("");
  const [busy, setBusy] = useState(false);
  const [shown, setShown] = useState<string | null>(null);
  if (ticket && shown !== ticket.key) {
    setShown(ticket.key);
    setProblem("");
    setDraft({
      title: ticket.title,
      summary: ticket.summary,
      scope: lines(ticket.scope).join("\n"),
      acceptance: lines(ticket.acceptance).join("\n"),
      testing: lines(ticket.testing).join("\n"),
    });
  }
  if (!ticket && shown !== null) setShown(null);

  async function save() {
    if (!ticket) return;
    setBusy(true);
    setProblem("");
    const text: TicketText = { title: draft.title.trim(), summary: draft.summary.trim(), scope: asLines(draft.scope), acceptance: asLines(draft.acceptance), testing: asLines(draft.testing) };
    try {
      await council.editTicket(project.id, planId, ticket.key, text);
      toast.success(`工單 ${ticket.key} 改好了`);
      onSaved();
      onClose();
    } catch (e) {
      setProblem((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  const field = (key: keyof typeof draft, label: string, hint?: string, rows = 3) => (
    <label className="grid gap-1 text-[13px] font-bold text-muted-foreground">
      <span>
        {label}
        {hint && <span className="ml-1.5 font-normal">{hint}</span>}
      </span>
      <Textarea rows={rows} value={draft[key]} onChange={(e) => setDraft((d) => ({ ...d, [key]: e.target.value }))} className="text-[14.5px] text-foreground" />
    </label>
  );

  return (
    <Dialog open={!!ticket} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-[640px]">
        <DialogHeader>
          <DialogTitle>改工單 {ticket?.key}</DialogTitle>
          <DialogDescription>執行者照這張工單做事。驗收條件寫得越具體，做出來越接近你要的。</DialogDescription>
        </DialogHeader>
        <div className="grid gap-3">
          <label className="grid gap-1 text-[13px] font-bold text-muted-foreground">
            標題
            <Input value={draft.title} onChange={(e) => setDraft((d) => ({ ...d, title: e.target.value }))} className="text-[15px] font-bold text-foreground" />
          </label>
          {field("summary", "要做什麼")}
          {field("scope", "範圍", "一行一項")}
          {field("acceptance", "驗收條件", "一行一項")}
          {field("testing", "怎麼測", "一行一項", 2)}
        </div>
        {problem && <Problem title="沒存起來">{problem}</Problem>}
        <DialogFooter>
          <Button variant="outline" onClick={onClose} disabled={busy}>
            不改了
          </Button>
          <Button onClick={save} disabled={busy || !draft.title.trim()}>
            存起來
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function MilestoneLine({ milestones, later }: { milestones: Milestone[]; later: string[] }) {
  if (milestones.length === 0) return null;
  return (
    <Panel>
      <PanelHead title="里程碑" sub="每個里程碑做完，都有看得到的成果。這次的工單是第一個里程碑的；後面的，做完這一批再規劃。" />
      <ol className="px-5 pb-5">
        {milestones.map((m, i) => (
          <li key={m.key} className="relative flex gap-3 pb-4 last:pb-0">
            {i < milestones.length - 1 && <span aria-hidden className="absolute left-[11px] top-6 bottom-0 w-[3px] rounded bg-border" />}
            <span className={cn("relative mt-0.5 grid size-[25px] shrink-0 place-items-center rounded-full text-[12px] font-black", i === 0 ? "bg-live text-white" : "border-[3px] border-border bg-card text-muted-foreground")}>{i + 1}</span>
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2 text-[15px] font-extrabold">
                {m.title}
                {i === 0 && <Tag tone="live">這一批</Tag>}
              </div>
              {m.goal && <p className="text-[14px] text-ink2">{m.goal}</p>}
              {i === 0 && lines(m.done_when).length > 0 && (
                <ul className="mt-1 grid gap-0.5 text-[13.5px] text-muted-foreground">
                  {lines(m.done_when).map((d, k) => (
                    <li key={k}>✓ {d}</li>
                  ))}
                </ul>
              )}
            </div>
          </li>
        ))}
      </ol>
      {later.length > 0 && (
        <div className="border-t border-border px-5 py-3.5">
          <div className="text-[13px] font-bold text-muted-foreground">留到之後的</div>
          <ul className="mt-1 grid gap-0.5 text-[14px] text-ink2">
            {later.map((l, i) => (
              <li key={i}>・{l}</li>
            ))}
          </ul>
        </div>
      )}
    </Panel>
  );
}

/**
 * TicketsTab is the work list as it stands: the person may change a ticket or
 * drop it, and approve the list, which makes each ticket an issue on GitHub.
 */
export function TicketsTab({ detail, going, onChanged }: { detail: PlanDetail; going: boolean; onChanged: () => void }) {
  const { project, plans } = useProject();
  const [editing, setEditing] = useState<Ticket | null>(null);
  const [dropping, setDropping] = useState<Ticket | null>(null);
  const [approving, setApproving] = useState(false);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const tickets = detail.tickets;
  const waiting = tickets.filter((t) => !t.issue);
  const objections = detail.record?.list_objections ?? [];
  const locked = going ? "會議還在開，工單還沒排定" : undefined;
  const waiters = dropping ? tickets.filter((t) => (t.depends_on ?? []).includes(dropping.key)) : [];

  async function drop() {
    if (!dropping) return;
    setBusy(true);
    try {
      await council.dropTicket(project.id, detail.id, dropping.key);
      toast.success(`工單 ${dropping.key} 刪掉了`);
      setDropping(null);
      onChanged();
      plans.reload();
    } catch (e) {
      toast.error("刪不掉", { description: (e as Error).message });
    } finally {
      setBusy(false);
    }
  }

  async function approve() {
    setBusy(true);
    setProblem("");
    try {
      const done = await council.approve(project.id, detail.id);
      toast.success(done.created.length ? `建好了 ${done.created.length} 張工單` : "工單都已經在 GitHub 上了", {
        description: "執行者可以開始做了：在終端機執行 awkit run。",
      });
    } catch (e) {
      setProblem((e as Error).message);
    } finally {
      setBusy(false);
      setApproving(false);
      onChanged();
      plans.reload();
    }
  }

  if (tickets.length === 0) {
    return (
      <Panel>
        <Empty icon={Rocket} title="還沒有工單">
          {going ? "議題談完後，規劃角色會排出里程碑和工單。" : "這次規劃沒有排出工單。"}
        </Empty>
      </Panel>
    );
  }

  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-5">
      {waiting.length === 0 ? (
        <div className="flex flex-wrap items-center gap-3 rounded-xl border border-go/50 bg-go/5 px-5 py-4">
          <CircleCheck className="size-5 text-go" aria-hidden />
          <div className="min-w-0 flex-1">
            <div className="text-[15.5px] font-extrabold">核可了：{tickets.length} 張工單都在 GitHub 上</div>
            <p className="text-[14px] text-ink2">在終端機執行 awkit run，執行者就會一張一張做。</p>
          </div>
          <Button variant="outline" onClick={() => (window.location.hash = href({ project: project.id, page: "board", rest: [] }))}>
            看工作看板 <ArrowRight aria-hidden />
          </Button>
        </div>
      ) : (
        <div className="flex flex-wrap items-center gap-3 rounded-xl border border-border border-l-[6px] border-l-signal bg-card px-5 py-4" data-tour="approve">
          <div className="min-w-0 flex-1">
            <div className="text-[15.5px] font-extrabold">{waiting.length} 張工單等你核可</div>
            <p className="text-[14px] text-ink2">核可前可以改標題和內容，也可以刪掉不要的。核可後會建到 GitHub，執行者才會開始做。</p>
          </div>
          <Button size="lg" onClick={() => setApproving(true)} disabled={!!locked || !!detail.list_problem || busy} title={locked}>
            <Rocket aria-hidden /> 核可，建到 GitHub
          </Button>
        </div>
      )}
      {detail.list_problem && (
        <Problem title="工單清單有地方不對，還不能核可">
          {detail.list_problem}
          <span className="mt-1 block">改好有問題的工單（或刪掉它），這裡就會消失。</span>
        </Problem>
      )}
      {problem && (
        <Problem title="沒有全部建好">
          GitHub 沒有把工單建起來。常見的原因：這個專案沒有連到 GitHub、gh 沒有登入，或網路斷了；
          <a href={href({ project: project.id, page: "health", rest: [] })} className="font-bold text-ring underline-offset-4 hover:underline">
            到健康檢查看看
          </a>
          。已經建好的會留著，再按一次核可會接著建剩下的。
          <details className="mt-1.5">
            <summary className="cursor-pointer text-[13px] font-bold text-muted-foreground">awkit 說的原文</summary>
            <p className="mt-1 whitespace-pre-wrap break-words font-mono text-[12px]">{problem}</p>
          </details>
        </Problem>
      )}

      <div className="grid grid-cols-[minmax(0,1fr)] gap-5 2xl:grid-cols-[minmax(0,1fr)_340px]">
        <div className="min-w-0 content-start 2xl:col-start-2 2xl:row-start-1">
          <MilestoneLine milestones={detail.milestones} later={detail.record?.later ?? []} />
        </div>
        <ul className="grid min-w-0 content-start gap-3 2xl:col-start-1 2xl:row-start-1" aria-label="工單">
          {tickets.map((t) => (
            <TicketCard key={t.key} ticket={t} objections={objections.filter((o) => o.ticket === t.key)} locked={locked} onEdit={() => setEditing(t)} onDrop={() => setDropping(t)} />
          ))}
        </ul>
      </div>

      <TicketEditor planId={detail.id} ticket={editing} onClose={() => setEditing(null)} onSaved={() => { onChanged(); plans.reload(); }} />

      <Dialog open={dropping !== null} onOpenChange={(o) => !o && setDropping(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>刪掉工單 {dropping?.key}？</DialogTitle>
            <DialogDescription>「{dropping?.title}」會從這次的清單拿掉。它還沒建到 GitHub，所以不會留下任何東西。</DialogDescription>
          </DialogHeader>
          {waiters.length > 0 && (
            <div className="flex gap-2.5 rounded-lg border-l-4 border-caution bg-caution/10 px-4 py-3 text-[14px]">
              <TriangleAlert className="mt-0.5 size-4 shrink-0 text-caution-ink" aria-hidden />
              <span>
                {waiters.map((t) => t.key).join("、")} 要等它先做完，所以現在刪不掉。要刪，先刪掉 {waiters.map((t) => t.key).join("、")}。
              </span>
            </div>
          )}
          <DialogFooter>
            <Button variant="outline" onClick={() => setDropping(null)} disabled={busy}>
              留著
            </Button>
            <Button variant="destructive" onClick={drop} disabled={busy || waiters.length > 0}>
              <Trash2 aria-hidden /> 刪掉
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={approving} onOpenChange={(o) => !busy && setApproving(o)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>核可 {waiting.length} 張工單？</DialogTitle>
            <DialogDescription>每張工單會在 GitHub 建成一個 issue，執行者照它做事。</DialogDescription>
          </DialogHeader>
          <div className="flex gap-2.5 rounded-lg border-l-4 border-caution bg-caution/10 px-4 py-3 text-[14px]">
            <TriangleAlert className="mt-0.5 size-4 shrink-0 text-caution-ink" aria-hidden />
            <span>GitHub 上的 issue 建了就刪不掉，只能關掉。先確定標題和內容是你要的。</span>
          </div>
          <ul className="grid max-h-56 gap-1 overflow-y-auto text-[14px]">
            {waiting.map((t) => (
              <li key={t.key} className="flex gap-2">
                <span className="w-10 shrink-0 font-mono text-[12.5px] font-bold text-muted-foreground">{t.key}</span>
                <span className="min-w-0">{t.title}</span>
              </li>
            ))}
          </ul>
          <DialogFooter>
            <Button variant="outline" onClick={() => setApproving(false)} disabled={busy}>
              再看看
            </Button>
            <Button onClick={approve} disabled={busy}>
              <ExternalLink aria-hidden /> {busy ? "建立中…" : `核可，建 ${waiting.length} 張`}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

import { useState } from "react";
import { ChevronDown, GraduationCap, ShieldAlert } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { Empty, PageHead, Panel, PanelHead, Problem, Tag, Term } from "@/components/bits";
import { useProject } from "@/app/project";
import { api, ApiError, type LessonDecision, type LessonView } from "@/lib/api";
import { heldBackWords } from "@/lib/guard";
import { cn } from "@/lib/utils";
import { group, LESSON_STATUS, sourceOf, splitHidden } from "./status";

/** LessonsPage is what the reviews taught the AI: the lessons the screening held back first, for the person to decide on. */
export function LessonsPage() {
  const { lessons } = useProject();
  const [showRetired, setShowRetired] = useState(false);
  const all = lessons.data?.lessons ?? [];
  const { held, inUse, retired } = group(all);

  return (
    <>
      <PageHead
        title="教訓"
        sub={
          <>
            審查教給 AI 的事。之後動到同樣檔案的執行者與審查者，會看到最相關的幾條。給任何 AI 看之前都會先檢查，可疑的先<Term>隔離</Term>，等你決定。
          </>
        }
      />
      {lessons.error && !lessons.data && <Problem title="讀不到教訓">{lessons.error.message}</Problem>}
      {lessons.loading && !lessons.data ? (
        <Panel>
          <div className="grid gap-3 p-5">{[0, 1, 2].map((i) => <Skeleton key={i} className="h-12" />)}</div>
        </Panel>
      ) : all.length === 0 && lessons.data ? (
        <Panel>
          <Empty icon={GraduationCap} title="還沒有教訓">
            被審查退回、後來終於通過的變更，會濃縮成一條教訓，出現在這裡。也可以在終端機用 awkit lessons add 自己加。
          </Empty>
        </Panel>
      ) : (
        <div className="grid gap-5">
          {held.length > 0 && (
            <Panel className="border-signal/70" data-section="held">
              <PanelHead
                title={`等你決定：${held.length} 條被擋下的教訓`}
                sub="讀完全文再決定。放行之後它會當成新的教訓試用；不採用就停用，不會再給任何 AI。"
              />
              <ul>
                {held.map((lesson) => (
                  <HeldLesson key={lesson.id} lesson={lesson} />
                ))}
              </ul>
            </Panel>
          )}
          <Panel data-section="in-use">
            <PanelHead title="使用中的教訓" sub="幫上忙越多次的排越前面" />
            {inUse.length === 0 ? (
              <Empty icon={GraduationCap} title="沒有使用中的教訓">
                {held.length > 0 ? "上面被擋下的教訓放行之後，會出現在這裡。" : "停用的教訓在下面。"}
              </Empty>
            ) : (
              <ul>
                {inUse.map((lesson) => (
                  <LessonRow key={lesson.id} lesson={lesson} />
                ))}
              </ul>
            )}
          </Panel>
          {retired.length > 0 && (
            <Panel data-section="retired">
              <button
                type="button"
                aria-expanded={showRetired}
                onClick={() => setShowRetired((s) => !s)}
                className="flex w-full items-center gap-2 rounded-xl px-5 py-3.5 text-left text-[14px] font-bold text-ink2 transition-colors hover:bg-muted/50"
              >
                <ChevronDown className={cn("size-4 transition-transform", showRetired && "rotate-180")} aria-hidden />
                {showRetired ? "收起" : "顯示"}已停用的 {retired.length} 條
              </button>
              {showRetired && (
                <ul>
                  {retired.map((lesson) => (
                    <LessonRow key={lesson.id} lesson={lesson} />
                  ))}
                </ul>
              )}
            </Panel>
          )}
        </div>
      )}
    </>
  );
}

/** LessonText is the text the AI would read, with what a person cannot see marked. */
function LessonText({ lesson }: { lesson: LessonView }) {
  return (
    <div className="mt-2 rounded-lg bg-muted/60 px-3.5 py-2.5 ring-1 ring-inset ring-border">
      {lesson.description && <p className="mb-1.5 text-[13.5px] text-ink2">{lesson.description}</p>}
      <pre className="whitespace-pre-wrap break-words font-mono text-[13px] leading-relaxed">
        {splitHidden(lesson.content).map((part, i) =>
          part.hidden ? (
            <mark key={i} className="rounded bg-caution/25 px-0.5 text-caution-ink" title="看不見的字元">
              {part.text}
            </mark>
          ) : (
            <span key={i}>{part.text}</span>
          ),
        )}
      </pre>
    </div>
  );
}

function Provenance({ lesson }: { lesson: LessonView }) {
  const cases = lesson.evidence.map((e) => (e.pr ? `工單 #${e.issue}（PR #${e.pr}）` : `工單 #${e.issue}`));
  return (
    <p className="mt-2 text-[13px] text-muted-foreground">
      {sourceOf(lesson.source)}
      {cases.length > 0 && `，來自 ${cases.join("、")}`}
      {lesson.scope.length > 0 && `。動到 ${lesson.scope.join("、")} 時會看到`}
    </p>
  );
}

function HeldLesson({ lesson }: { lesson: LessonView }) {
  const { project, lessons } = useProject();
  const [confirming, setConfirming] = useState(false);
  const [busy, setBusy] = useState<LessonDecision | null>(null);
  const [error, setError] = useState("");

  const decide = async (decision: LessonDecision) => {
    setBusy(decision);
    setError("");
    try {
      await api.decideLesson(project.id, lesson.id, decision);
      setConfirming(false);
      toast(decision === "approve" ? `已放行 ${lesson.id}，它會當成新的教訓試用` : `${lesson.id} 不採用，已停用`);
      lessons.reload();
    } catch (e) {
      if (e instanceof ApiError && e.status === 409) {
        // Decided already, in another window or a terminal.
        setConfirming(false);
        toast(`${lesson.id} 已經有人決定了`);
        lessons.reload();
      } else {
        setError(e instanceof Error ? e.message : String(e));
      }
    } finally {
      setBusy(null);
    }
  };

  return (
    <li className="border-t border-border px-5 py-4 first:border-t-0" data-lesson={lesson.id}>
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-mono text-[13px] text-muted-foreground">{lesson.id}</span>
        <span className="text-[15px] font-bold">{lesson.title}</span>
        <Tag tone="signal">{LESSON_STATUS.quarantined.label}</Tag>
      </div>
      <p className="mt-1.5 flex items-start gap-1.5 text-[14px]">
        <ShieldAlert className="mt-0.5 size-4 shrink-0 text-caution-ink" aria-hidden />
        <span>
          被擋下的原因：<strong>{heldBackWords(lesson.held_back ?? "")}</strong>
        </span>
      </p>
      {lesson.hidden && <p className="mt-1 text-[13px] text-caution-ink">這段文字裡有看不見的字元，下面用 &lt;U+…&gt; 標出來了。</p>}
      <LessonText lesson={lesson} />
      <Provenance lesson={lesson} />
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <Button size="sm" disabled={busy !== null} onClick={() => decide("reject")}>
          {busy === "reject" ? "停用中…" : "不採用"}
        </Button>
        <Button size="sm" variant="outline" disabled={busy !== null} onClick={() => setConfirming(true)}>
          放行，讓它試用
        </Button>
        {error && (
          <span role="alert" className="text-[13px] text-destructive">
            {error}
          </span>
        )}
      </div>
      <Dialog open={confirming} onOpenChange={(open) => !busy && setConfirming(open)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>放行 {lesson.id}？</DialogTitle>
            <DialogDescription>
              放行之後，它會當成新的教訓試用：之後動到相關檔案的執行者會讀到它，並標明「未驗證」。確定你讀過全文，裡面沒有要 AI 做不該做的事？
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" disabled={busy !== null} onClick={() => setConfirming(false)}>
              再想想
            </Button>
            <Button disabled={busy !== null} onClick={() => decide("approve")}>
              {busy === "approve" ? "放行中…" : "放行"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </li>
  );
}

function LessonRow({ lesson }: { lesson: LessonView }) {
  const [open, setOpen] = useState(false);
  const status = LESSON_STATUS[lesson.status];
  return (
    <li className="border-t border-border first:border-t-0" data-lesson={lesson.id}>
      <button
        type="button"
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-start gap-3 px-5 py-3 text-left transition-colors hover:bg-muted/50"
      >
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-mono text-[13px] text-muted-foreground">{lesson.id}</span>
            <span className="text-[15px] font-bold">{lesson.title}</span>
            <Tag tone={status.tone}>{status.label}</Tag>
          </div>
          <p className="mt-0.5 text-[13px] text-muted-foreground">
            幫上忙 {lesson.hits} 次{lesson.misses > 0 && `，沒擋住 ${lesson.misses} 次`}
            {lesson.categories.length > 0 && ` · ${lesson.categories.join("、")}`}
          </p>
        </div>
        <ChevronDown className={cn("mt-1 size-4 shrink-0 text-muted-foreground transition-transform", open && "rotate-180")} aria-hidden />
      </button>
      {open && (
        <div className="px-5 pb-4">
          <p className="text-[13px] text-ink2">{status.means}</p>
          {lesson.status === "retired" && lesson.held_back && (
            <p className="mt-1 text-[13px] text-ink2">它曾被擋下，原因是{heldBackWords(lesson.held_back)}；你決定不採用。</p>
          )}
          <LessonText lesson={lesson} />
          <Provenance lesson={lesson} />
        </div>
      )}
    </li>
  );
}

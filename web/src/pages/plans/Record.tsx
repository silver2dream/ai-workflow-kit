import { useState } from "react";
import { BookOpenCheck, CircleHelp, FileText, Gavel, MessageSquareReply, ScrollText, TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Empty, Panel, PanelHead, Tag } from "@/components/bits";
import { useProject } from "@/app/project";
import type { DecisionState, Entry, PlanDetail, WrittenDocument } from "@/lib/council";
import { AssumptionLine, DecisionLine, DocumentList, EntryView, ReviewView, WordView } from "@/pages/plans/Entry";
import { byTopic } from "@/pages/plans/Meeting";

/** topicTitle names the topic of an id, as the agenda has it. */
function useTopicTitle(detail: PlanDetail) {
  const topics = detail.agenda?.topics ?? detail.record?.agenda?.topics ?? [];
  return (id?: string) => {
    if (!id) return "照你的話改的";
    const at = topics.findIndex((t) => t.id === id);
    return at >= 0 ? `第 ${at + 1} 題：${topics[at].title}` : id;
  };
}

/** DecisionsTab is every decision of the plan, standing first, with what became of the others. */
export function DecisionsTab({ detail }: { detail: PlanDetail }) {
  const { roster } = useProject();
  const title = useTopicTitle(detail);
  const record = detail.record;
  const [showGone, setShowGone] = useState(false);
  if (!record) return null;
  const standing = record.decisions.filter((d) => !d.replaced_by && !d.withdrawn);
  const gone = record.decisions.filter((d) => d.replaced_by || d.withdrawn);
  const groups = new Map<string, DecisionState[]>();
  for (const d of standing) {
    const key = title(d.topic);
    groups.set(key, [...(groups.get(key) ?? []), d]);
  }
  const decider = roster.data?.roles.find((r) => r.decides)?.id;
  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-5">
      {record.decisions.length === 0 ? (
        <Panel>
          <Empty icon={Gavel} title="還沒有決策">議題談完，負責裁定的角色會把談定的事寫成決策。</Empty>
        </Panel>
      ) : (
        <Panel>
          <PanelHead title={`現行的決策：${standing.length} 條`} sub="寫在 docs/decisions.md；以後的規劃和執行者都照這些做。" />
          <div className="grid grid-cols-[minmax(0,1fr)] gap-5 px-5 pb-5">
            {[...groups.entries()].map(([topic, decisions]) => (
              <section key={topic}>
                <h3 className="text-[13px] font-bold text-muted-foreground">{topic}</h3>
                <ul className="mt-2 grid gap-3">
                  {decisions.map((d) => (
                    <DecisionLine key={d.id ?? d.title} decision={d} />
                  ))}
                </ul>
              </section>
            ))}
          </div>
          {gone.length > 0 && (
            <div className="border-t border-border px-5 py-3.5">
              <button type="button" onClick={() => setShowGone((s) => !s)} aria-expanded={showGone} className="cursor-pointer text-[14px] font-bold text-ring underline-offset-4 hover:underline">
                {showGone ? "收起" : "看"}被取代或撤回的 {gone.length} 條
              </button>
              {showGone && (
                <ul className="mt-3 grid gap-3">
                  {gone.map((d) => (
                    <DecisionLine key={d.id ?? d.title} decision={d} state={{ replacedBy: d.replaced_by, withdrawn: d.withdrawn }} />
                  ))}
                </ul>
              )}
            </div>
          )}
        </Panel>
      )}
      {record.revisions.length > 0 && (
        <Panel>
          <PanelHead title="你說話之後的重審" sub="每次你說話，負責裁定的角色都把所有決策重審一遍。" />
          <div className="grid gap-4 px-5 pb-5">
            {record.revisions.map((r, i) => (
              <div key={i} className="grid gap-2">
                {r.words.map((w) => (
                  <WordView key={w.id} word={w} />
                ))}
                <ReviewView review={r} role={decider} />
              </div>
            ))}
          </div>
        </Panel>
      )}
    </div>
  );
}

/**
 * AssumptionsTab is what the table assumed where only the owner knew: each
 * one waits for an answer, which the deciding role reads every decision
 * against.
 */
export function AssumptionsTab({ detail, onAnswer, canAnswer }: { detail: PlanDetail; onAnswer: (question: string) => void; canAnswer: boolean }) {
  const record = detail.record;
  const title = useTopicTitle(detail);
  if (!record) return null;
  const open = record.assumptions.filter((a) => !a.answer);
  const answered = record.assumptions.filter((a) => a.answer);
  if (record.assumptions.length === 0 && record.answers.length === 0) {
    return (
      <Panel>
        <Empty icon={CircleHelp} title="沒有假設">議會沒有需要替你假設的事。</Empty>
      </Panel>
    );
  }
  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-5">
      {open.length > 0 && (
        <Panel>
          <PanelHead title={`等你確認：${open.length} 個假設`} sub="只有你知道答案的事，議會先替你假設了。答了以後，負責裁定的角色會照你的答案重審所有決策。" />
          <ul className="grid gap-4 px-5 pb-5">
            {open.map((a) => (
              <li key={a.id ?? a.question} className="grid gap-2 rounded-lg border border-border p-4 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-start">
                <ul>
                  <AssumptionLine assumption={a} />
                </ul>
                {canAnswer && a.question && (
                  <Button variant="outline" onClick={() => onAnswer(a.question)} className="justify-self-start">
                    <MessageSquareReply aria-hidden /> 回答
                  </Button>
                )}
                <p className="text-[12.5px] text-muted-foreground sm:col-span-2">{title(a.topic)}</p>
              </li>
            ))}
          </ul>
        </Panel>
      )}
      {(answered.length > 0 || record.answers.length > 0) && (
        <Panel>
          <PanelHead title="你答過的" />
          <ul className="grid gap-3 px-5 pb-5">
            {record.answers
              .filter((a) => a.answer.trim())
              .map((a) => (
                <li key={a.question} className="text-[14.5px]">
                  <div className="font-bold">{a.question}</div>
                  <div className="text-go">你答了：{a.answer}</div>
                </li>
              ))}
            {answered.map((a) => (
              <AssumptionLine key={a.id ?? a.question} assumption={a} answer={a.answer} />
            ))}
          </ul>
        </Panel>
      )}
    </div>
  );
}

/** latest keeps the last word on each document: a document written twice is shown once. */
function latest(documents: WrittenDocument[]): WrittenDocument[] {
  const by = new Map<string, WrittenDocument>();
  for (const d of documents) by.set(d.path, d);
  return [...by.values()];
}

/** DocumentsTab is what the meeting wrote, and where a document is still at odds with a rule of the project. */
export function DocumentsTab({ detail }: { detail: PlanDetail }) {
  const record = detail.record;
  const { roster } = useProject();
  if (!record) return null;
  const documents = latest(record.documents);
  const owners = roster.data?.roles ?? [];
  const titleOf = (id: string) => owners.find((r) => r.id === id)?.title ?? id;
  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-5">
      <Panel>
        <PanelHead title={`寫了 ${documents.length} 份文件`} sub="都在專案資料夾裡；每份文件有一個負責的角色，以後的規劃會接著改它。" />
        {documents.length === 0 ? (
          <Empty icon={FileText} title="還沒有寫文件" />
        ) : (
          <div className="grid gap-4 px-5 pb-5">
            {[...new Set(documents.map((d) => d.owner))].map((owner) => (
              <section key={owner}>
                <h3 className="text-[13px] font-bold text-muted-foreground">{titleOf(owner)}負責</h3>
                <DocumentList documents={documents.filter((d) => d.owner === owner)} />
              </section>
            ))}
          </div>
        )}
      </Panel>
      {record.findings.length > 0 && (
        <Panel>
          <PanelHead title={`和專案規則不符的地方：${record.findings.length} 處`} sub="寫文件的角色照規則讀過自己的文件，這些還沒改好。" />
          <ul className="grid gap-3 px-5 pb-5">
            {record.findings.map((f, i) => (
              <li key={i} className="flex gap-2.5 text-[14px]">
                <TriangleAlert className="mt-0.5 size-4 shrink-0 text-caution-ink" aria-hidden />
                <div className="min-w-0">
                  <div className="break-all font-mono text-[12.5px] text-muted-foreground">{f.document}</div>
                  <div>
                    <b>{f.rule}</b>：{f.point}
                  </div>
                  {f.fix && <div className="text-ink2">該改成：{f.fix}</div>}
                </div>
              </li>
            ))}
          </ul>
        </Panel>
      )}
      {record.data_open.length > 0 && (
        <Panel>
          <PanelHead title="遊戲資料還沒對上的地方" sub="資料檢查找到的，還沒解決。" />
          <ul className="grid gap-1.5 px-5 pb-5 text-[14px]">
            {record.data_open.map((d, i) => (
              <li key={i}>・{d}</li>
            ))}
          </ul>
        </Panel>
      )}
      {record.pages.length > 0 && (
        <Panel>
          <PanelHead title="給你審的資料頁" sub="把遊戲資料排成表格的頁面，在專案資料夾裡用瀏覽器打開。" />
          <ul className="grid gap-1.5 px-5 pb-5">
            {record.pages.map((p) => (
              <li key={p} className="flex items-center gap-2 text-[13.5px]">
                <BookOpenCheck className="size-4 text-muted-foreground" aria-hidden />
                <span className="break-all font-mono text-[12.5px]">{p}</span>
              </li>
            ))}
          </ul>
        </Panel>
      )}
    </div>
  );
}

/** TranscriptTab is everything said at the meeting, topic by topic. */
export function TranscriptTab({ detail, entries }: { detail: PlanDetail; entries: Entry[] }) {
  const topics = detail.agenda?.topics ?? [];
  if (entries.length === 0) {
    return (
      <Panel>
        <Empty icon={ScrollText} title="沒有會議經過">這個規劃沒有留下逐字紀錄。</Empty>
      </Panel>
    );
  }
  const groups = byTopic(entries, topics);
  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-5">
      {topics.length === 0 ? (
        <Panel>
          <div className="grid grid-cols-[minmax(0,1fr)] gap-5 p-5">
            {entries.map((e) => (
              <EntryView key={`${e.seq}-${e.step}`} entry={e} />
            ))}
          </div>
        </Panel>
      ) : (
        topics.map((topic, i) => {
          const said = groups.get(topic.id) ?? [];
          return (
            <Panel key={topic.id}>
              <PanelHead title={`第 ${i + 1} 題：${topic.title}`} sub={topic.question} right={<Tag>{said.length} 則</Tag>} />
              <div className="grid grid-cols-[minmax(0,1fr)] gap-5 px-5 pb-5">
                {said.length === 0 ? <p className="text-[14px] text-muted-foreground">這一題沒有談。</p> : said.map((e) => <EntryView key={`${e.seq}-${e.step}`} entry={e} />)}
              </div>
            </Panel>
          );
        })
      )}
    </div>
  );
}

import { useState } from "react";
import { CircleHelp, Play } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Panel, PanelHead, Problem } from "@/components/bits";
import { useProject } from "@/app/project";
import { council, type PlanDetail } from "@/lib/council";
import { OneAtATime } from "@/pages/plans/OneAtATime";
import { useOtherMeeting } from "@/pages/plans/usePlan";

/**
 * Questions are what only the owner can answer, put before the first topic.
 * An answer left empty is the council's to assume; the meeting starts once
 * the owner sends them.
 */
export function Questions({ detail, onDone }: { detail: PlanDetail; onDone: () => void }) {
  const { project } = useProject();
  const [answers, setAnswers] = useState<string[]>(() => detail.questions.map(() => ""));
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const other = useOtherMeeting(detail.id);

  async function send() {
    setBusy(true);
    setProblem("");
    try {
      await council.answer(
        project.id,
        detail.id,
        detail.questions.map((question, i) => ({ question, answer: (answers[i] ?? "").trim() })),
      );
      toast.success("回答送出了，議會開始討論");
      onDone();
    } catch (e) {
      setProblem((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="grid gap-5" data-tour="questions">
      <Panel>
        <PanelHead title={`${detail.questions.length} 個問題`} sub="只有你答得出來的事。留白的，議會會先假設，會後給你確認。" />
        <ol className="grid gap-4 px-5 pb-5">
          {detail.questions.map((question, i) => (
            <li key={question} className="grid gap-1.5">
              <label htmlFor={`answer-${i}`} className="flex gap-2 text-[15px] font-bold">
                <CircleHelp className="mt-1 size-4 shrink-0 text-ring" aria-hidden />
                {question}
              </label>
              <Textarea id={`answer-${i}`} rows={2} value={answers[i] ?? ""} onChange={(e) => setAnswers((all) => all.map((a, j) => (j === i ? e.target.value : a)))} placeholder="留白：交給議會先假設" />
            </li>
          ))}
        </ol>
      </Panel>
      {problem && <Problem title="沒送出去">{problem}</Problem>}
      {other && <OneAtATime other={other} then="開完，再送出回答開始討論；你寫的會留在這裡" />}
      <div>
        <Button size="lg" onClick={send} disabled={busy || !!other}>
          <Play aria-hidden /> 送出回答，開始討論
        </Button>
      </div>
    </div>
  );
}

import { useState } from "react";
import { MessageSquarePlus, Send } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { useProject } from "@/app/project";
import { council } from "@/lib/council";
import { useOnboarding } from "@/onboarding/state";

/**
 * SayBox is how the owner speaks to the meeting. What they say reaches the
 * next role to speak; when something was decided already, the deciding role
 * first reads every decision again against it. A question picked is what the
 * word answers.
 */
export function SayBox({
  planId,
  going,
  after,
  questions,
  question: picked,
  onQuestion,
  onSaid,
}: {
  planId: string;
  /** Whether the meeting goes on now: else the word waits for it to go on. */
  going: boolean;
  /** Whether the meeting is over: a word then has the plan read again. */
  after?: boolean;
  /** Questions the word may answer: of the agenda, or what an assumption asked. */
  questions: string[];
  /** The question picked, when the page picks it (an assumption's 回答). */
  question?: string;
  onQuestion?: (question: string) => void;
  onSaid: () => void;
}) {
  const { project } = useProject();
  const { mark } = useOnboarding();
  const [text, setText] = useState("");
  const [own, setOwn] = useState("");
  const question = picked ?? own;
  const setQuestion = onQuestion ?? setOwn;
  const [busy, setBusy] = useState(false);

  async function send(event: React.FormEvent) {
    event.preventDefault();
    if (!text.trim()) return;
    setBusy(true);
    try {
      const word = await council.say(project.id, planId, text.trim(), question || undefined);
      toast.success(`送給議會了（${word.id}）`, {
        description: going
          ? "下一個發言的角色會聽到；已裁定的決策會先照你的話重審。"
          : after
            ? "按「照我的話重審」，議會會重審決策、重排工單。"
            : "會議沒在開：按「接著開會」，議會會先照你的話重審。",
      });
      setText("");
      setQuestion("");
      mark("plan");
      onSaid();
    } catch (e) {
      toast.error("沒送出去", { description: (e as Error).message });
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={send} className="grid grid-cols-[minmax(0,1fr)] gap-2.5" data-tour="saybox">
      <label htmlFor="say" className="flex items-center gap-2 text-[15px] font-extrabold">
        <MessageSquarePlus className="size-4 text-ring" aria-hidden />
        你想說什麼
      </label>
      <Textarea
        id="say"
        value={text}
        onChange={(e) => setText(e.target.value)}
        rows={3}
        placeholder="例如：要能在手機上用；或：這一版不做排行榜"
        className="text-[15px]"
        onKeyDown={(e) => {
          if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) send(e);
        }}
      />
      {questions.length > 0 && (
        <select
          aria-label="這是在回答哪個問題"
          value={question}
          onChange={(e) => setQuestion(e.target.value)}
          className="h-9 w-full min-w-0 rounded-md border border-input bg-card px-2 text-[14px] text-ellipsis"
        >
          <option value="">不是在回答問題</option>
          {questions.map((q) => (
            <option key={q} value={q}>
              回答：{q}
            </option>
          ))}
        </select>
      )}
      <div className="flex flex-wrap items-center gap-3">
        <Button type="submit" disabled={busy || !text.trim()}>
          <Send aria-hidden /> 說給議會聽
        </Button>
        <span className="text-[12.5px] text-muted-foreground">Ctrl + Enter 也能送出</span>
      </div>
      <p className="text-[13px] leading-relaxed text-muted-foreground">
        {going
          ? "你的話跟需求一樣算數。正在發言的角色說完後，下一個角色就會聽到；已經裁定的事，決策角色會先照你的話全部重審一遍，受影響的文件整份重看。"
          : after
            ? "會議開完了。你的話先記下來；按「照我的話重審」，決策角色會把所有決策重審一遍、受影響的文件整份重看，再重排工單。議題不會重談。"
            : "會議現在沒在開。你的話會先記下來，接著開會時議會第一件事就是照它重審。"}
      </p>
    </form>
  );
}

import { useState } from "react";
import { ArrowLeft, Play } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { PageHead, Panel, Problem, Tag } from "@/components/bits";
import { useProject } from "@/app/project";
import { useOnboarding } from "@/onboarding/state";
import { council } from "@/lib/council";
import { href, navigate } from "@/lib/router";
import { RoleBadge, useSeats } from "@/pages/plans/roles";
import { headline } from "@/pages/plans/status";

const MAX = 20000;

function Option({ checked, onChange, title, children }: { checked: boolean; onChange: (on: boolean) => void; title: string; children: React.ReactNode }) {
  return (
    <label className="flex cursor-pointer gap-3 rounded-xl border border-border bg-card p-3.5 transition-colors hover:border-muted-foreground has-[:checked]:border-foreground/70 has-[:checked]:ring-1 has-[:checked]:ring-foreground/70 has-focus-visible:ring-3 has-focus-visible:ring-ring/50">
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} className="mt-1 size-4 shrink-0 cursor-pointer accent-[var(--foreground)]" />
      <span>
        <span className="block text-[15px] font-bold">{title}</span>
        <span className="block text-[13.5px] leading-relaxed text-muted-foreground">{children}</span>
      </span>
    </label>
  );
}

/** NewPlan is where the owner says what they want built, and starts the council's meeting on it. */
export function NewPlan() {
  const { project, plans, roster } = useProject();
  const { mark } = useOnboarding();
  const seat = useSeats();
  const [brief, setBrief] = useState("");
  const [ask, setAsk] = useState(true);
  const [review, setReview] = useState(false);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const meeting = (plans.data?.plans ?? []).find((p) => p.meeting?.state === "meeting");
  const roles = roster.data?.roles ?? [];
  const tooLong = brief.trim().length > MAX;

  async function start(event: React.FormEvent) {
    event.preventDefault();
    if (!brief.trim() || tooLong) return;
    setBusy(true);
    setProblem("");
    try {
      const { id } = await council.start(project.id, brief.trim(), review, ask);
      mark("plan");
      toast.success("議會開始開會了", { description: "關掉工作台也不會停；回來就看得到進度。" });
      plans.reload();
      navigate(href({ project: project.id, page: "plans", rest: [id] }));
    } catch (e) {
      setProblem((e as Error).message);
      setBusy(false);
    }
  }

  return (
    <>
      <a href={href({ project: project.id, page: "plans", rest: [] })} className="mb-3 inline-flex items-center gap-1.5 text-[14px] font-bold text-muted-foreground hover:text-foreground">
        <ArrowLeft className="size-4" aria-hidden />
        規劃與議會
      </a>
      <PageHead title="開新的規劃" sub="說說你想做什麼。寫得像跟同事交代事情就好：要給誰用、最重要的是什麼、哪些這次不做。" />
      <form onSubmit={start} className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_340px]">
        <div className="grid content-start gap-5">
          <Panel>
            <div className="grid gap-2 p-5">
              <label htmlFor="brief" className="text-[15px] font-extrabold">
                你想做什麼
              </label>
              <Textarea
                id="brief"
                value={brief}
                onChange={(e) => setBrief(e.target.value)}
                rows={11}
                placeholder={"例如：做一個網球對打的手機遊戲。\n玩家用手指滑動揮拍，對手是電腦，三種難度。\n先做單機就好，排行榜和連線這次不做。"}
                className="min-h-[240px] text-[15px] leading-relaxed"
                aria-describedby="brief-count"
              />
              <div id="brief-count" className={tooLong ? "text-[13px] font-bold text-destructive" : "text-[13px] text-muted-foreground"}>
                {brief.trim().length.toLocaleString()} / {MAX.toLocaleString()} 字
              </div>
            </div>
          </Panel>
          <div className="grid gap-3">
            <Option checked={ask} onChange={setAsk} title="先問我只有我答得出來的問題">
              開始討論前，議會會列出只有你知道答案的事（例如給誰用、預算多少），等你答完才開始。不勾的話，議會先自己假設，會後給你確認。
            </Option>
            <Option checked={review} onChange={setReview} title="議程排好先給我看">
              主持人排好議題後先停下來，你可以改每一題要談什麼、誰發言、順序，改好再開始。
            </Option>
          </div>
          {meeting && (
            <Problem title="現在不能開新的規劃">
              「{headline(meeting.brief, meeting.id)}」正在開會。一個專案一次開一場會，因為每場會都會改同一份決策紀錄和文件。
            </Problem>
          )}
          {problem && <Problem title="開不起來">{problem}</Problem>}
          <div className="flex flex-wrap items-center gap-3">
            <Button type="submit" size="lg" disabled={busy || !brief.trim() || tooLong || !!meeting}>
              <Play aria-hidden /> 開始規劃
            </Button>
            <span className="text-[13px] text-muted-foreground">開會會呼叫 Claude；議題越多，花的越多。</span>
          </div>
        </div>
        <Panel className="self-start">
          <div className="p-5">
            <div className="text-[15px] font-extrabold">誰會參加</div>
            {roster.data?.missing ? (
              <p className="mt-1.5 text-[14px] leading-relaxed text-ink2">這個專案還沒有議會名單。開始規劃時，awkit 會照專案的程式庫排一份，存在 .ai/roster.yaml。</p>
            ) : (
              <>
                <ul className="mt-3 grid gap-3">
                  <li className="flex gap-2.5">
                    <RoleBadge seat={seat("moderator")} size={30} />
                    <div className="min-w-0 text-[14px]">
                      <div className="font-bold">主持人</div>
                      <p className="text-[13px] text-muted-foreground">排議程、整理每一題談定了什麼。</p>
                    </div>
                  </li>
                  {roles.map((r) => (
                    <li key={r.id} className="flex gap-2.5">
                      <RoleBadge seat={seat(r.id)} size={30} />
                      <div className="min-w-0 text-[14px]">
                        <div className="flex flex-wrap items-center gap-1.5 font-bold">
                          {r.title}
                          {r.decides && <Tag tone="focus">裁定</Tag>}
                          {r.plans && <Tag>排工單</Tag>}
                        </div>
                        <p className="line-clamp-3 text-[13px] text-muted-foreground">{r.stance}</p>
                      </div>
                    </li>
                  ))}
                </ul>
                {roster.data && (
                  <p className="mt-4 border-t border-border pt-3 text-[13px] text-muted-foreground">
                    一場會最多 {roster.data.topics} 個議題、{roster.data.calls} 次模型呼叫。
                  </p>
                )}
              </>
            )}
          </div>
        </Panel>
      </form>
    </>
  );
}

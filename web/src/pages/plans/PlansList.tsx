import { ArrowRight, Loader2, Plus, Users } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { DOT, Empty, PageHead, Panel, Problem, Tag } from "@/components/bits";
import { useProject } from "@/app/project";
import type { PlanView } from "@/lib/api";
import { href, navigate } from "@/lib/router";
import { cn } from "@/lib/utils";
import { headline, planStatus } from "@/pages/plans/status";

function PlanRow({ plan }: { plan: PlanView }) {
  const { project } = useProject();
  const status = planStatus(plan);
  const meeting = plan.meeting;
  const approved = plan.tickets.filter((t) => t.issue).length;
  return (
    <li className="border-t border-border first:border-t-0">
      <a href={href({ project: project.id, page: "plans", rest: [plan.id] })} className="group flex items-start gap-3.5 px-5 py-4 transition-colors hover:bg-muted/50">
        <span className={cn("mt-2 size-2.5 shrink-0 rounded-full", DOT[status.tone], meeting?.state === "meeting" && "animate-pulse")} aria-hidden />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="min-w-0 text-[15.5px] font-extrabold leading-snug">{headline(plan.brief, `規劃 ${plan.id}`)}</span>
            <Tag tone={status.tone}>
              {meeting?.state === "meeting" && <Loader2 className="size-3.5 animate-spin" aria-hidden />}
              {status.label}
            </Tag>
          </div>
          <div className="mt-1 flex flex-wrap gap-x-3 gap-y-0.5 text-[13.5px] text-muted-foreground">
            {meeting && meeting.topics > 0 && ["meeting", "paused", "stopped", "failed"].includes(meeting.state) && (
              <span className="tnum">
                第 {Math.min(Math.max(meeting.topic, 1), meeting.topics)} / {meeting.topics} 題
              </span>
            )}
            {plan.milestones.length > 0 && <span className="tnum">{plan.milestones.length} 個里程碑</span>}
            {plan.tickets.length > 0 && (
              <span className="tnum">
                {plan.tickets.length} 張工單{approved > 0 && approved < plan.tickets.length ? `，${approved} 張已建到 GitHub` : ""}
              </span>
            )}
            <span className="font-mono text-[12.5px]">{plan.id}</span>
          </div>
          {status.todo && <p className="mt-1 text-[14px] font-bold text-foreground">{status.todo}</p>}
        </div>
        <ArrowRight className="mt-1.5 size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5" aria-hidden />
      </a>
    </li>
  );
}

/** PlansList is every plan of the project, newest first, and the way to start one. */
export function PlansList() {
  const { project, plans, roster } = useProject();
  const list = plans.data?.plans ?? [];
  const meeting = list.find((p) => p.meeting?.state === "meeting");
  const start = () => navigate(href({ project: project.id, page: "plans", rest: ["new"] }));
  return (
    <>
      <PageHead
        title="規劃與議會"
        sub="把想做的事交給議會：各角色輪流發言、互相質疑，由負責裁定的角色拍板，寫好文件，最後擬出工單給你核可。"
        right={
          <Button size="lg" onClick={start} disabled={!!meeting} title={meeting ? "一個專案一次開一場會" : undefined} data-tour="newplan">
            <Plus aria-hidden /> 開新的規劃
          </Button>
        }
      />
      {meeting && (
        <p className="-mt-3 mb-5 text-[14px] text-muted-foreground">
          正在開會：「{headline(meeting.brief, meeting.id)}」。一個專案一次開一場會，開完才能開新的。
        </p>
      )}
      <div className="grid gap-5 xl:grid-cols-[minmax(0,1fr)_320px]">
        <Panel data-tour="plans">
          {plans.loading && !plans.data ? (
            <div className="grid gap-3 p-5">
              <Skeleton className="h-12 w-full" />
              <Skeleton className="h-12 w-full" />
            </div>
          ) : plans.error && !plans.data ? (
            <div className="p-5">
              <Problem title="讀不到規劃">{plans.error.message}</Problem>
            </div>
          ) : list.length === 0 ? (
            <Empty icon={Users} title="還沒有規劃">
              說說你想做什麼，議會會討論出做法、寫好文件，再拆成一張張工單。
              <div className="mt-4">
                <Button onClick={start}>
                  <Plus aria-hidden /> 開第一個規劃
                </Button>
              </div>
            </Empty>
          ) : (
            <ul aria-label="規劃">
              {list.map((plan) => (
                <PlanRow key={plan.id} plan={plan} />
              ))}
            </ul>
          )}
        </Panel>
        <Panel className="self-start">
          <div className="p-5">
            <div className="text-[15px] font-extrabold">議會的成員</div>
            {roster.data?.missing ? (
              <p className="mt-1.5 text-[14px] text-ink2">這個專案還沒有議會名單。第一次規劃時，awkit 會照專案的程式庫排一份。</p>
            ) : (
              <ul className="mt-3 grid gap-2.5">
                {(roster.data?.roles ?? []).map((r) => (
                  <li key={r.id} className="text-[14px]">
                    <span className="font-bold">{r.title}</span>
                    {r.decides && <Tag tone="focus" className="ml-1.5">裁定</Tag>}
                    {r.plans && <Tag className="ml-1.5">排工單</Tag>}
                    <p className="line-clamp-2 text-[13px] text-muted-foreground">{r.stance}</p>
                  </li>
                ))}
              </ul>
            )}
            {roster.error && !roster.data && <p className="mt-2 text-[13px] text-muted-foreground">讀不到議會名單：{roster.error.message}</p>}
          </div>
        </Panel>
      </div>
    </>
  );
}

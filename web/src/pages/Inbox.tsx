import { ArrowRight, TriangleAlert } from "lucide-react";
import { DOT, Panel, PanelHead, PageHead, Tag } from "@/components/bits";
import { useProject } from "@/app/project";
import { href } from "@/lib/router";
import { cn } from "@/lib/utils";
import { NeedsYouList } from "@/pages/Overview";
import { headline, planStatus } from "@/pages/plans/status";

/** Inbox is what only the person can decide: what blocks the work first. */
export function InboxPage() {
  const { project, plans, status } = useProject();
  const waiting = (plans.data?.plans ?? []).map((plan) => ({ plan, status: planStatus(plan) })).filter((p) => p.status.needsYou);
  const warnings = status.data?.warnings ?? [];
  const suggestions = status.data?.suggestions ?? [];
  return (
    <>
      <PageHead title="需要你處理" sub="只有你能決定的事。擋住工作的排在最前面，處理完會自己從這裡消失。" />
      <div className="grid gap-5">
        <Panel>
          <PanelHead title="卡住的工單" sub="審查幾次都沒過，或執行者做不下去" />
          <NeedsYouList />
        </Panel>
        {waiting.length > 0 && (
          <Panel>
            <PanelHead title="規劃在等你" sub="議程要你看、問題要你答，或工單等你核可" />
            <ul>
              {waiting.map(({ plan, status: s }) => (
                <li key={plan.id} className="border-t border-border first:border-t-0">
                  <a href={href({ project: project.id, page: "plans", rest: [plan.id] })} className="group flex items-start gap-3.5 px-5 py-3.5 transition-colors hover:bg-muted/50">
                    <span className={cn("mt-2 size-2.5 shrink-0 rounded-full", DOT[s.tone])} aria-hidden />
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-[15px] font-bold">{headline(plan.brief, `規劃 ${plan.id}`)}</span>
                        <Tag tone={s.tone}>{s.label}</Tag>
                      </div>
                      <p className="mt-0.5 text-[14px] text-ink2">{s.todo}</p>
                    </div>
                    <ArrowRight className="mt-1.5 size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5" aria-hidden />
                  </a>
                </li>
              ))}
            </ul>
          </Panel>
        )}
        {(warnings.length > 0 || suggestions.length > 0) && (
          <Panel>
            <PanelHead title="awkit 的提醒" sub="和 awkit status 說的一樣" />
            <ul className="grid gap-2 px-5 pb-5">
              {warnings.map((w) => (
                <li key={w} className="flex items-start gap-2 text-[14px]">
                  <TriangleAlert className="mt-0.5 size-4 shrink-0 text-caution-ink" aria-hidden />
                  {w}
                </li>
              ))}
              {suggestions.map((s) => (
                <li key={s} className="flex items-start gap-2 text-[14px] text-ink2">
                  <Tag>建議</Tag>
                  {s}
                </li>
              ))}
            </ul>
          </Panel>
        )}
      </div>
    </>
  );
}

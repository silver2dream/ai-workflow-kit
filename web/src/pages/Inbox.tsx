import { CircleHelp, TriangleAlert } from "lucide-react";
import { Panel, PanelHead, PageHead, Tag } from "@/components/bits";
import { useProject } from "@/app/project";
import { NeedsYouList } from "@/pages/Overview";

/** Inbox is what only the person can decide: what blocks the work first. */
export function InboxPage() {
  const { plans, status } = useProject();
  const toApprove = (plans.data?.plans ?? []).filter((p) => p.tickets.some((t) => !t.issue));
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
        {toApprove.length > 0 && (
          <Panel>
            <PanelHead title="等你核可的規劃" sub="議會擬好了工單，還沒建到 GitHub" />
            <ul>
              {toApprove.map((plan) => {
                const waiting = plan.tickets.filter((t) => !t.issue).length;
                return (
                  <li key={plan.id} className="flex flex-wrap items-start gap-3.5 border-t border-border px-5 py-3.5 first:border-t-0">
                    <span className="mt-0.5 grid size-9 shrink-0 place-items-center rounded-lg bg-ring/10 text-ring">
                      <CircleHelp className="size-[18px]" aria-hidden />
                    </span>
                    <div className="min-w-0 flex-1">
                      <div className="text-[15px] font-bold">{plan.brief || plan.id}</div>
                      <p className="mt-0.5 text-[14px] text-ink2">
                        {waiting} 張工單等你核可。核可前可以改，在終端機執行 <code className="rounded bg-muted px-1 font-mono text-[13px]">awkit plan approve {plan.id}</code>；工作台裡的核可按鈕會在下一個版本加上。
                      </p>
                    </div>
                  </li>
                );
              })}
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

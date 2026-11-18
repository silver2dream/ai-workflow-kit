import { Check, CircleX, ExternalLink, Loader2, RotateCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Panel } from "@/components/bits";
import { useProject } from "@/app/project";
import type { ApprovalRecord, ApprovalStep } from "@/lib/council";
import { href } from "@/lib/router";
import { cn } from "@/lib/utils";

/** The steps of an approval, in order, as the owner reads them. */
const STEPS: { key: ApprovalStep; label: string; sub: string }[] = [
  { key: "check", label: "檢查", sub: "沒有工作在跑，GitHub 上有整合分支" },
  { key: "issues", label: "在 GitHub 建工單", sub: "先不掛工作標籤：執行者還不會動它們" },
  { key: "documents", label: "把議會的文件開成 PR", sub: "決策、文件和工單清單，放進一個新分支" },
  { key: "checks", label: "等 CI，過了就合併", sub: "合併進整合分支，執行者才讀得到" },
  { key: "label", label: "掛上工作標籤", sub: "文件到了，工單才開始算數" },
  { key: "sync", label: "更新你的專案資料夾", sub: "和整合分支一樣，才能開始工作" },
];

/** What a stop means, and what to do, by how the approval stopped. */
function stopped(approval: ApprovalRecord): string {
  const pr = approval.pr ? `PR #${approval.pr}` : "文件的 PR";
  switch (approval.why) {
    case "run":
      return "專案裡有工作正在跑，核可會讓它半路接到新工單。等它停了再按「接著核可」。";
    case "branch":
      return "GitHub 上找不到整合分支，或連不上 GitHub。到健康檢查看看，修好再按「接著核可」。";
    case "checks":
      return `${pr} 的 CI 沒過，文件還沒進整合分支，所以工單先不開工。到 GitHub 看哪裡沒過；修好後按「接著核可」，會接著做，不會重建工單。`;
    case "noci":
      return `這個專案有替 PR 跑的 CI，但 GitHub 沒有替 ${pr} 跑，所以不能當成「CI 過了」合併。到 GitHub 的 Actions 看看有沒有開、額度夠不夠；或在 GitHub 上自己合併 ${pr}，再按「接著核可」。`;
    case "slow":
      return `${pr} 的 CI 跑太久還沒結束。等它跑完，再按「接著核可」。`;
    case "refused":
      return `GitHub 不讓 ${pr} 自動合併，可能規定要有人審核。請在 GitHub 上合併它，再按「接著核可」。`;
    case "stopped":
      return "核可的程序不在了（被關掉，或電腦重開）。按「接著核可」從斷掉的地方接著做，不會重建工單。";
    default:
      return "核可時出了錯。看看下面 awkit 說的原文；按「接著核可」會從斷掉的地方接著做，不會重建工單。";
  }
}

/**
 * ApprovalProgress follows an approval step by step: the issues, the pull
 * request of the documents and its checks, the labels, the working tree. A
 * stop says what it means and how to go on.
 */
export function ApprovalProgress({ approval, onRetry, retryBlocked }: { approval: ApprovalRecord; onRetry: () => void; retryBlocked?: string }) {
  const { project } = useProject();
  const at = approval.state === "done" ? STEPS.length : Math.max(0, STEPS.findIndex((s) => s.key === approval.step));
  const failed = approval.state === "failed" || approval.state === "stopped";
  return (
    <Panel className={cn(failed ? "border-l-[6px] border-l-signal" : "")} data-tour="approval">
      <div className="p-5">
        <div className="flex flex-wrap items-center gap-2">
          <div className="text-[15.5px] font-extrabold">
            {approval.state === "working" ? "核可中" : failed ? "核可停在半路" : "核可完成"}
          </div>
          {approval.pr && approval.url && (
            <a href={approval.url} target="_blank" rel="noreferrer" className="ml-auto inline-flex items-center gap-1 text-[13.5px] font-bold text-ring underline-offset-4 hover:underline">
              文件的 PR #{approval.pr}
              <ExternalLink className="size-3.5" aria-hidden />
            </a>
          )}
        </div>
        {approval.state === "working" && <p className="mt-0.5 text-[13.5px] text-muted-foreground">關掉工作台也會繼續。CI 要跑多久看專案，可能要幾分鐘。</p>}
        <ol className="mt-4" aria-label="核可的步驟">
          {STEPS.map((s, i) => {
            const done = i < at;
            const here = i === at && approval.state !== "done";
            const broke = here && failed;
            return (
              <li key={s.key} className="relative flex gap-3 pb-3.5 last:pb-0" aria-current={here ? "step" : undefined}>
                {i < STEPS.length - 1 && <span aria-hidden className={cn("absolute left-[11px] top-6 bottom-0 w-[3px] rounded", done ? "bg-go" : "bg-border")} />}
                <span
                  className={cn(
                    "relative grid size-[25px] shrink-0 place-items-center rounded-full",
                    done && "bg-go text-white",
                    here && !broke && "bg-live text-white",
                    broke && "bg-signal text-[#231a00]",
                    !done && !here && "border-[3px] border-border bg-card",
                  )}
                >
                  {done ? <Check className="size-3.5" aria-hidden /> : broke ? <CircleX className="size-3.5" aria-hidden /> : here ? <Loader2 className="size-3.5 animate-spin" aria-hidden /> : null}
                </span>
                <div className="min-w-0 pt-0.5">
                  <div className={cn("text-[14.5px] font-bold", !done && !here && "text-muted-foreground")}>
                    {s.label}
                    {s.key === "issues" && approval.created > 0 && <span className="ml-1.5 font-normal text-muted-foreground tnum">建了 {approval.created} 張</span>}
                  </div>
                  <p className="text-[13px] text-muted-foreground">{s.sub}</p>
                </div>
              </li>
            );
          })}
        </ol>
        {failed && (
          <div className="mt-4 grid gap-3 border-t border-border pt-4">
            <p className="text-[14.5px] leading-relaxed">{stopped(approval)}</p>
            {approval.why === "branch" && (
              <a href={href({ project: project.id, page: "health", rest: [] })} className="justify-self-start text-[14px] font-bold text-ring underline-offset-4 hover:underline">
                到健康檢查看看
              </a>
            )}
            <div className="flex flex-wrap items-center gap-3">
              <Button onClick={onRetry} disabled={!!retryBlocked}>
                <RotateCw aria-hidden /> 接著核可
              </Button>
              {retryBlocked && <span className="text-[13.5px] text-muted-foreground">{retryBlocked}</span>}
            </div>
          </div>
        )}
        {(failed || approval.lines.length > 0) && (
          <details className="mt-4 text-[13px]">
            <summary className="cursor-pointer font-bold text-muted-foreground">awkit 說的原文</summary>
            <pre className="mt-1.5 max-h-56 overflow-auto whitespace-pre-wrap break-words rounded-md bg-muted px-3 py-2 font-mono text-[12px] leading-relaxed text-ink2">
              {[...approval.lines, approval.error].filter(Boolean).join("\n")}
            </pre>
          </details>
        )}
      </div>
    </Panel>
  );
}

import { Check, CircleX, ExternalLink, Loader2, RefreshCw, RotateCw, TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Panel } from "@/components/bits";
import { useProject } from "@/app/project";
import type { ApplyRecord, ApplyStep } from "@/lib/settings";
import { href } from "@/lib/router";
import { cn } from "@/lib/utils";

/** The steps of a change of the settings, in order, as a person reads them. */
const STEPS: { key: ApplyStep; label: string; sub: (r: ApplyRecord) => string }[] = [
  { key: "check", label: "檢查", sub: () => "設定沒有問題、沒有工作在跑、GitHub 上有整合分支" },
  {
    key: "change",
    label: "把修改開成 PR",
    sub: (r) => (r.what.includes("config") ? "改的是 GitHub 上最新的設定檔，連同 awkit generate 依設定產生的檔案" : "改的是 GitHub 上最新的名單"),
  },
  { key: "checks", label: "等 CI，過了就合併", sub: () => "合併進整合分支，設定才算數" },
  { key: "sync", label: "更新你的專案資料夾", sub: () => "和整合分支一樣，工作才會照新設定做" },
];

const WHAT: Record<string, string> = { roster: "議會名單", config: "專案設定" };

/** What a stop means, and what to do, by how the change stopped. */
function stopped(record: ApplyRecord): { text: string; retry: boolean } {
  const pr = record.pr ? `PR #${record.pr}` : "設定的 PR";
  switch (record.why) {
    case "invalid":
      return { text: "這樣的設定議會或工作用不了，所以沒有送出。看下面 awkit 說的原文，改好再存。", retry: false };
    case "conflict":
      return { text: "你改的地方，在你打開這頁之後，GitHub 上也有人改了。重新載入，看過最新的設定再改。", retry: false };
    case "run":
      return { text: "專案裡有工作正在跑，換設定會讓它做到一半換規則。等它停了再按「再試一次」。", retry: true };
    case "branch":
      return { text: "GitHub 上找不到整合分支，或連不上 GitHub。到健康檢查看看，修好再按「再試一次」。", retry: true };
    case "checks":
      return { text: `${pr} 的 CI 沒過，設定還沒生效。到 GitHub 看哪裡沒過：是設定造成的，就在這裡改好再存（會更新同一個 PR）；不是的話，修好後按「再試一次」。`, retry: true };
    case "noci":
      return { text: `這個專案有替 PR 跑的 CI，但 GitHub 沒有替 ${pr} 跑，所以不能當成「CI 過了」合併。到 GitHub 的 Actions 看看有沒有開、額度夠不夠；或在 GitHub 上自己合併 ${pr}，再按「再試一次」。`, retry: true };
    case "slow":
      return { text: `${pr} 的 CI 跑太久還沒結束。等它跑完，再按「再試一次」。`, retry: true };
    case "refused":
      return { text: `GitHub 不讓 ${pr} 自動合併，可能規定要有人審核。請在 GitHub 上合併它，再按「再試一次」更新你的專案資料夾。`, retry: true };
    case "stopped":
      return { text: "套用設定的程序不在了（被關掉，或電腦重開）。按「再試一次」接著做。", retry: true };
    default:
      return { text: "套用設定時出了錯。看看下面 awkit 說的原文；按「再試一次」會照同樣的修改再做一次。", retry: true };
  }
}

/** ApplyProgress follows a change of the settings to the integration branch, and says what a stop means. */
export function ApplyProgress({
  record,
  onRetry,
  onReload,
  retryBlocked,
  busy,
}: {
  record: ApplyRecord;
  onRetry: () => void;
  onReload: () => void;
  retryBlocked?: string;
  busy: boolean;
}) {
  const { project } = useProject();
  const at = record.state === "done" ? STEPS.length : Math.max(0, STEPS.findIndex((s) => s.key === record.step));
  const failed = record.state === "failed" || record.state === "stopped";
  const what = record.what.map((w) => WHAT[w] ?? w).join("和");
  const stop = failed ? stopped(record) : null;
  return (
    <Panel className={cn(failed && "border-l-[6px] border-l-signal")} aria-live="polite">
      <div className="p-5">
        <div className="flex flex-wrap items-center gap-2">
          <div className="text-[15.5px] font-extrabold">
            {record.state === "working" ? `正在套用${what}` : failed ? `${what}沒有套用成功` : record.unchanged ? `GitHub 上的${what}本來就是這樣` : `${what}已經生效`}
          </div>
          {record.pr && record.url && (
            <a href={record.url} target="_blank" rel="noreferrer" className="ml-auto inline-flex items-center gap-1 text-[13.5px] font-bold text-ring underline-offset-4 hover:underline">
              設定的 PR #{record.pr}
              <ExternalLink className="size-3.5" aria-hidden />
            </a>
          )}
        </div>
        {record.state === "working" && <p className="mt-0.5 text-[13.5px] text-muted-foreground">關掉工作台也會繼續。CI 要跑多久看專案，可能要幾分鐘；這段時間設定不能再改。</p>}
        {record.state !== "done" || !record.unchanged ? (
          <ol className="mt-4" aria-label="套用設定的步驟">
            {STEPS.map((s, i) => {
              const done = i < at;
              const here = i === at && record.state !== "done";
              const broke = here && failed;
              return (
                <li key={s.key} className="relative flex gap-3 pb-3.5 last:pb-0" aria-current={here ? "step" : undefined}>
                  {i < STEPS.length - 1 && <span aria-hidden className={cn("absolute bottom-0 left-[11px] top-6 w-[3px] rounded", done ? "bg-go" : "bg-border")} />}
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
                    <div className={cn("text-[14.5px] font-bold", !done && !here && "text-muted-foreground")}>{s.label}</div>
                    <p className="text-[13px] text-muted-foreground">{s.sub(record)}</p>
                  </div>
                </li>
              );
            })}
          </ol>
        ) : null}
        {record.state === "done" && record.left && (
          <div className="mt-4 flex gap-2 rounded-lg border-l-4 border-caution bg-caution/10 px-4 py-3 text-[14px]">
            <TriangleAlert className="mt-0.5 size-4 shrink-0 text-caution-ink" aria-hidden />
            <p>
              設定已經在 GitHub 上了，但你的專案資料夾沒有跟著更新，所以在這台電腦開工時還是舊設定。在終端機用{" "}
              <code className="rounded bg-muted px-1 font-mono text-[13px]">git pull</code> 更新；原因在下面的原文裡。
            </p>
          </div>
        )}
        {stop && (
          <div className="mt-4 grid gap-3 border-t border-border pt-4">
            <p className="text-[14.5px] leading-relaxed">{stop.text}</p>
            {record.why === "branch" && (
              <a href={href({ project: project.id, page: "health", rest: [] })} className="justify-self-start text-[14px] font-bold text-ring underline-offset-4 hover:underline">
                到健康檢查看看
              </a>
            )}
            <div className="flex flex-wrap items-center gap-3">
              {stop.retry && (
                <Button onClick={onRetry} disabled={!!retryBlocked || busy}>
                  {busy ? <Loader2 className="animate-spin" aria-hidden /> : <RotateCw aria-hidden />} 再試一次
                </Button>
              )}
              {record.why === "conflict" && (
                <Button onClick={onReload} variant="outline">
                  <RefreshCw aria-hidden /> 重新載入設定
                </Button>
              )}
              {retryBlocked && stop.retry && <span className="text-[13.5px] text-muted-foreground">{retryBlocked}</span>}
            </div>
          </div>
        )}
        {(failed || record.left || record.lines.length > 0) && (
          <details className="mt-4 text-[13px]">
            <summary className="cursor-pointer font-bold text-muted-foreground">awkit 說的原文</summary>
            <pre className="mt-1.5 max-h-56 overflow-auto whitespace-pre-wrap break-words rounded-md bg-muted px-3 py-2 font-mono text-[12px] leading-relaxed text-ink2">
              {[...record.lines, record.error, record.left].filter(Boolean).join("\n")}
            </pre>
          </details>
        )}
      </div>
    </Panel>
  );
}

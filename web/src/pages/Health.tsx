import { useEffect } from "react";
import { CircleCheck, CircleX, Eraser, HeartPulse, RefreshCw, TriangleAlert } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Empty, PageHead, Panel, PanelHead, Problem, Tag } from "@/components/bits";
import { useProject } from "@/app/project";
import { useOnboarding } from "@/onboarding/state";
import { api, type Check } from "@/lib/api";
import { useResource } from "@/lib/resource";
import { cn } from "@/lib/utils";

const ORDER: Record<string, number> = { error: 0, warning: 1, ok: 2 };
const worstFirst = (checks: Check[]) => [...checks].sort((a, b) => (ORDER[a.status] ?? 3) - (ORDER[b.status] ?? 3));

export function Checks({ checks, loading }: { checks?: Check[]; loading: boolean }) {
  if (loading && !checks) {
    return (
      <div className="grid gap-3 px-5 pb-5">
        {[0, 1, 2].map((i) => (
          <Skeleton key={i} className="h-10" />
        ))}
      </div>
    );
  }
  if (!checks || checks.length === 0) return <Empty icon={HeartPulse} title="沒有檢查結果" />;
  return (
    <ul>
      {worstFirst(checks).map((c, i) => (
        <li key={`${c.name}-${i}`} className="flex items-start gap-4 border-t border-border px-5 py-3.5">
          <span
            className={cn(
              "mt-0.5 grid size-8 shrink-0 place-items-center rounded-lg",
              c.status === "ok" ? "bg-go/10 text-go" : c.status === "warning" ? "bg-caution/15 text-caution-ink" : "bg-destructive/10 text-destructive",
            )}
          >
            {c.status === "ok" ? <CircleCheck className="size-4" aria-label="正常" /> : c.status === "warning" ? <TriangleAlert className="size-4" aria-label="提醒" /> : <CircleX className="size-4" aria-label="要處理" />}
          </span>
          <div className="min-w-0 flex-1">
            <div className="text-[15px] font-bold">{c.name}</div>
            <p className="break-words text-[14px] text-ink2">{c.message}</p>
          </div>
        </li>
      ))}
    </ul>
  );
}

/** Health is `awkit doctor`, both halves: what a run needs, and what earlier runs left behind. */
export function HealthPage() {
  const { project } = useProject();
  const { mark } = useOnboarding();
  const health = useResource(() => api.health(project.id), [project.id]);
  useEffect(() => {
    if (health.data) mark("health");
  }, [health.data, mark]);

  const all = [...(health.data?.setup ?? []), ...(health.data?.state ?? [])];
  const bad = all.filter((c) => c.status === "error").length;
  const warn = all.filter((c) => c.status === "warning").length;
  const cleanable = (health.data?.state ?? []).some((c) => c.can_clean);

  return (
    <>
      <PageHead
        title="健康檢查"
        sub="開工前，工作需要的東西都在嗎：GitHub、Claude Code、設定、分支；以及之前的工作留下了什麼。和終端機的 awkit doctor 是同一套檢查。"
        right={
          <Button variant="outline" onClick={health.reload} disabled={health.loading}>
            <RefreshCw className={cn(health.loading && "animate-spin")} aria-hidden /> 重新檢查
          </Button>
        }
      />
      {health.error && <Problem title="檢查跑不完">{health.error.message}</Problem>}
      {health.loading && !health.data && <p className="mb-4 text-[14px] text-muted-foreground">檢查中，要問 GitHub，可能要十幾秒…</p>}
      {health.data && (
        <div className="mb-4 flex flex-wrap items-center gap-2">
          {bad === 0 && warn === 0 && (
            <Tag tone="go" icon={CircleCheck}>
              全部正常，可以開始工作
            </Tag>
          )}
          {bad > 0 && (
            <Tag tone="stop" icon={CircleX}>
              {bad} 個要先處理，工作才能開始
            </Tag>
          )}
          {warn > 0 && (
            <Tag tone="caution" icon={TriangleAlert}>
              {warn} 個提醒
            </Tag>
          )}
        </div>
      )}
      <div className="grid gap-5" data-tour="health">
        <Panel>
          <PanelHead title="工作需要的" sub="工具、設定、GitHub 上的 repository 與分支" />
          <Checks checks={health.data?.setup} loading={health.loading} />
        </Panel>
        <Panel>
          <PanelHead title="之前的工作留下的" sub="上一輪沒收拾的東西" />
          <Checks checks={health.data?.state} loading={health.loading} />
          {cleanable && (
            <p className="flex items-start gap-2 border-t border-border px-5 py-3.5 text-[14px] text-ink2">
              <Eraser className="mt-0.5 size-4 shrink-0" aria-hidden />
              <span>
                這些可以在終端機用 <code className="rounded bg-muted px-1 font-mono text-[13px]">awkit reset</code> 清掉；先用{" "}
                <code className="rounded bg-muted px-1 font-mono text-[13px]">awkit reset --dry-run</code> 看會清什麼。
              </span>
            </p>
          )}
        </Panel>
      </div>
      <p className="mt-3 text-[13px] text-muted-foreground">各項檢查的說明目前是英文，和 awkit doctor 一致。</p>
    </>
  );
}

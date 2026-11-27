import { Panel, PanelHead } from "@/components/bits";
import { useProject } from "@/app/project";
import { decisionWords } from "@/lib/engine";
import { cn } from "@/lib/utils";
import { when } from "@/lib/words";

/** How many of the differences are listed, the latest first. */
const SHOWN = 5;

/**
 * Shadow says how the last run's decisions compared with the ones the
 * Principal's workflow would have made beside it (AWKIT_ENGINE=shadow), as
 * `awkit status` says it. A run without a shadow shows nothing.
 */
export function Shadow() {
  const { engine } = useProject();
  const shadow = engine.data?.shadow;
  if (!shadow) return null;
  const differences = [...shadow.differences].reverse();
  const cells = [
    { label: "比了幾步", value: shadow.steps, tone: "" },
    { label: "一樣", value: shadow.same, tone: "" },
    { label: "已知的差異", value: shadow.known, tone: "" },
    { label: "不同", value: shadow.differences.length, tone: shadow.differences.length > 0 ? "text-caution-ink" : "" },
    { label: "沒比成", value: shadow.failed, tone: shadow.failed > 0 ? "text-caution-ink" : "" },
  ];
  return (
    <Panel data-tour="runshadow">
      <PanelHead title="和主控的比對" sub={`${when(shadow.started)} 開始的那一輪用了影子模式：engine 每做一個決定，舊的主控也在旁邊算一次，只算不做。`} />
      <dl className="grid grid-cols-3 gap-y-3 px-5 pb-4 text-center sm:grid-cols-5 xl:grid-cols-3">
        {cells.map((c) => (
          <div key={c.label}>
            <dd className={cn("text-[20px] font-black tnum", c.tone)}>{c.value}</dd>
            <dt className="text-[12.5px] text-muted-foreground">{c.label}</dt>
          </div>
        ))}
      </dl>
      {differences.length === 0 ? (
        <p className="border-t border-border px-5 py-3 text-[13.5px] text-ink2">沒有說不出原因的不同。</p>
      ) : (
        <ol className="border-t border-border px-5 py-3" aria-label="不同的步驟">
          {differences.slice(0, SHOWN).map((d) => (
            <li key={d.at} className="grid gap-0.5 py-2 text-[13.5px] leading-snug">
              <time dateTime={d.at} className="text-[12.5px] font-bold text-muted-foreground tnum">
                {when(d.at)}
              </time>
              <span>
                <b>engine：</b>
                {decisionWords(d.comparison.engine)}
              </span>
              <span>
                <b>主控：</b>
                {decisionWords(d.comparison.analyzer)}
              </span>
            </li>
          ))}
          {differences.length > SHOWN && (
            <li className="pt-1 text-[12.5px] text-muted-foreground">更早的 {differences.length - SHOWN} 個不同在終端機用 awkit events 看。</li>
          )}
        </ol>
      )}
    </Panel>
  );
}

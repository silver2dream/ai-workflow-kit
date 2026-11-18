import { Info } from "lucide-react";
import { useProject } from "@/app/project";
import type { PlanView } from "@/lib/api";
import { href } from "@/lib/router";
import { headline } from "@/pages/plans/status";

/** OneAtATime says why a meeting cannot start now: another one is held in the project. */
export function OneAtATime({ other, then = "開完再回來接著開" }: { other: PlanView; then?: string }) {
  const { project } = useProject();
  return (
    <p className="flex gap-2 text-[13.5px] leading-relaxed text-ink2" role="note">
      <Info className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden />
      <span>
        <a href={href({ project: project.id, page: "plans", rest: [other.id] })} className="font-bold text-ring underline-offset-4 hover:underline">
          「{headline(other.brief, other.id)}」
        </a>
        正在開會。一個專案一次開一場會，等它{then}。
      </span>
    </p>
  );
}

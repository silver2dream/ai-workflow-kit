import { BadgeCheck, HardDrive, Map, MessagesSquare, Route } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { useOnboarding } from "@/onboarding/state";

const STEPS = [
  { icon: MessagesSquare, title: "說出想做什麼", body: "六個 AI 角色開會，寫出文件、決策和工單。" },
  { icon: BadgeCheck, title: "核可工單", body: "你點頭，工單才會建到 GitHub。" },
  { icon: Route, title: "看團隊做事", body: "一張一張寫程式、審查、合併；卡住時叫你。" },
];

/**
 * Welcome is the first thing a person sees, once: it says what the workbench
 * is for and asks how they want to learn it. It has no close button and Esc
 * does not close it, since each of its three ways out is a choice that is
 * kept; one of them is to skip all teaching.
 */
export function Welcome() {
  const { state, startTour, skipAll, listOnly } = useOnboarding();
  return (
    <Dialog open={state.status === "new"}>
      <DialogContent
        showCloseButton={false}
        onEscapeKeyDown={(e) => e.preventDefault()}
        onInteractOutside={(e) => e.preventDefault()}
        className="max-h-[calc(100dvh-32px)] gap-0 overflow-y-auto p-7 sm:max-w-[560px]"
      >
        <div className="flex items-center gap-3">
          <span className="grid size-11 place-items-center rounded-lg bg-signal text-[#231a00]">
            <Route className="size-[22px]" aria-hidden />
          </span>
          <span className="text-[13.5px] font-bold text-muted-foreground">第一次使用</span>
        </div>
        <DialogTitle className="mt-5 text-[26px] font-black leading-tight tracking-tight">你和 AI 團隊一起工作的地方</DialogTitle>
        <DialogDescription className="mt-2 text-[15.5px] leading-relaxed text-ink2">
          你說想做什麼，團隊會規劃、寫程式、審查、合併。你只要在需要你的時候出手。
        </DialogDescription>
        <ol className="mt-5 grid gap-3">
          {STEPS.map((s, i) => (
            <li key={s.title} className="flex gap-3">
              <span className="grid size-8 shrink-0 place-items-center rounded-md bg-foreground text-[14px] font-black text-background">{i + 1}</span>
              <div>
                <div className="flex items-center gap-1.5 text-[15px] font-extrabold">
                  <s.icon className="size-4 text-muted-foreground" aria-hidden />
                  {s.title}
                </div>
                <p className="text-[14px] text-ink2">{s.body}</p>
              </div>
            </li>
          ))}
        </ol>
        <p className="mt-4 rounded-lg bg-muted px-3.5 py-2.5 text-[13.5px] leading-relaxed text-ink2 ring-1 ring-inset ring-border">
          這一版的工作台讓你看見一切：工作走到哪、誰做了什麼、哪裡卡住。開會、核可和開始工作目前還在終端機做，之後會搬進工作台。
        </p>
        <div className="mt-6 flex flex-wrap items-center gap-2">
          <Button size="lg" onClick={() => startTour(0)} autoFocus>
            <Map aria-hidden /> 帶我走一遍，約 2 分鐘
          </Button>
          <Button size="lg" variant="outline" onClick={skipAll}>
            跳過全部教學
          </Button>
        </div>
        <button type="button" onClick={listOnly} className="mt-3 self-start text-[13.5px] font-bold text-ink2 underline-offset-4 hover:underline">
          不要導覽，只留上手清單
        </button>
        <p className="mt-5 flex items-start gap-2 border-t border-border pt-4 text-[13px] text-muted-foreground">
          <HardDrive className="mt-0.5 size-3.5 shrink-0" aria-hidden />
          <span>教學看到哪只記在這台電腦，不會寫進專案。之後隨時可以從右上角的「說明」重新開始。</span>
        </p>
      </DialogContent>
    </Dialog>
  );
}

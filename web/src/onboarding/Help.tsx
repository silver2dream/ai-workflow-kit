import { useState } from "react";
import { BookA, CircleHelp, HardDrive, Keyboard, ListChecks, Map, MapPin, type LucideIcon } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { useProject } from "@/app/project";
import { TaskList } from "@/onboarding/Checklist";
import { useOnboarding } from "@/onboarding/state";
import { TASKS, TOUR } from "@/onboarding/steps";
import { useAfterClose } from "@/lib/after-close";
import { href, navigate } from "@/lib/router";
import { TERMS } from "@/lib/words";

function Item({ icon: Icon, title, sub, onClick }: { icon: LucideIcon; title: string; sub: string; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} className="flex w-full items-start gap-3 rounded-lg px-3 py-2.5 text-left hover:bg-muted">
      <span className="mt-0.5 grid size-8 shrink-0 place-items-center rounded-md bg-muted text-ink2 ring-1 ring-inset ring-border">
        <Icon className="size-4" aria-hidden />
      </span>
      <span>
        <span className="block text-[14.5px] font-bold">{title}</span>
        <span className="block text-[13px] text-muted-foreground">{sub}</span>
      </span>
    </button>
  );
}

/**
 * Help is the way back to the teaching after it was left: the tour from where
 * it stopped or from the start, the checklist, the glossary, the keys, and a
 * way to forget it all and start over.
 */
export function Help({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { state, teaching, startTour, showList, reset } = useOnboarding();
  const { project } = useProject();
  const [list, setList] = useState(false);
  const [glossary, setGlossary] = useState(false);
  const n = TASKS.filter((t) => state.done.includes(t.key)).length;
  const { later, onCloseAutoFocus } = useAfterClose();
  const afterList = useAfterClose();
  const then =(action: () => void) => () => {
    later(action);
    onOpenChange(false);
  };

  return (
    <>
      <Popover open={open} onOpenChange={onOpenChange}>
        <PopoverTrigger asChild>
          <Button variant="outline" data-tour="help" aria-label="說明" className="h-10">
            <CircleHelp aria-hidden />
            <span className="hidden sm:inline">說明</span>
          </Button>
        </PopoverTrigger>
        <PopoverContent align="end" className="w-[min(380px,calc(100vw-32px))] gap-0 p-2" onCloseAutoFocus={onCloseAutoFocus}>
          <h2 className="px-3 pb-1 pt-2 text-[15px] font-extrabold">說明</h2>
          {state.status === "paused" ? (
            <Item icon={MapPin} title={`繼續導覽，第 ${Math.min(state.step, TOUR.length - 1) + 1} 步`} sub="從上次停下的地方接著看" onClick={then(() => startTour(state.step))} />
          ) : (
            <Item icon={Map} title="從頭看導覽" sub={`${TOUR.length} 步，約 2 分鐘，可以隨時跳過`} onClick={then(() => startTour(0))} />
          )}
          <Item
            icon={ListChecks}
            title={`上手清單，${n} / ${TASKS.length}`}
            sub={state.list_hidden || !teaching ? "目前收起來了，按這裡打開" : "在側邊欄最下面，做了會自己打勾"}
            onClick={then(() => {
              showList();
              setList(true);
            })}
          />
          <Item icon={BookA} title="名詞解釋" sub="工單、PR、里程碑這些詞是什麼意思" onClick={then(() => setGlossary(true))} />
          <Item
            icon={Keyboard}
            title="鍵盤快捷鍵"
            sub="Ctrl K 搜尋，? 打開說明，Esc 關閉"
            onClick={then(() => toast("Ctrl K 搜尋或跳到別頁，? 打開說明，Esc 關閉視窗或先停下導覽"))}
          />
          <div className="mt-1 border-t border-border px-3 pb-1 pt-3">
            <p className="flex items-start gap-2 text-[13px] text-muted-foreground">
              <HardDrive className="mt-0.5 size-3.5 shrink-0" aria-hidden />
              <span>教學看到哪、清單做了哪些，只記在這台電腦，不會寫進專案，也不會上傳。</span>
            </p>
            <button
              type="button"
              onClick={then(() => {
                navigate(href({ project: project.id, page: "overview", rest: [] }));
                reset();
              })}
              className="mt-2 text-[13px] font-bold text-destructive hover:underline"
            >
              清除這台電腦上的教學紀錄
            </button>
          </div>
        </PopoverContent>
      </Popover>

      <Dialog open={list} onOpenChange={setList}>
        <DialogContent className="sm:max-w-[440px]" onCloseAutoFocus={afterList.onCloseAutoFocus}>
          <DialogHeader>
            <DialogTitle>上手清單</DialogTitle>
            <DialogDescription>做了會自己打勾，進度只記在這台電腦。</DialogDescription>
          </DialogHeader>
          <TaskList
            onDo={(go) => {
              afterList.later(go);
              setList(false);
            }}
          />
        </DialogContent>
      </Dialog>

      <Sheet open={glossary} onOpenChange={setGlossary}>
        <SheetContent className="w-full gap-0 p-0 sm:max-w-[480px]">
          <SheetHeader className="border-b border-border px-6 py-4">
            <SheetTitle className="text-[19px] font-extrabold">名詞解釋</SheetTitle>
            <SheetDescription>畫面上有虛線底線的詞，滑過或用 Tab 停在上面，也會出現同樣的說明。</SheetDescription>
          </SheetHeader>
          <dl className="flex-1 overflow-y-auto px-6 py-2">
            {Object.entries(TERMS).map(([term, meaning]) => (
              <div key={term} className="border-b border-border py-3.5 last:border-b-0">
                <dt className="text-[16px] font-extrabold">{term}</dt>
                <dd className="mt-1 text-[14.5px] leading-relaxed text-ink2">{meaning}</dd>
              </div>
            ))}
          </dl>
        </SheetContent>
      </Sheet>
    </>
  );
}

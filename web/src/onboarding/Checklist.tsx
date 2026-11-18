import { useState } from "react";
import { Check, HardDrive, ListChecks, PartyPopper, X } from "lucide-react";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { useProject } from "@/app/project";
import { useOnboarding } from "@/onboarding/state";
import { TASKS, type Task } from "@/onboarding/steps";
import { useAfterClose } from "@/lib/after-close";
import { href, navigate } from "@/lib/router";
import { cn } from "@/lib/utils";

/** useDoTask goes to where a thing of the checklist is done. */
export function useDoTask() {
  const { project } = useProject();
  const { startTour } = useOnboarding();
  return (task: Task) => {
    if (task.key === "tour") startTour(0);
    else if (task.page) navigate(href({ project: project.id, page: task.page, rest: [] }));
  };
}

/**
 * TaskList is the checklist itself, wherever it is shown. onDo closes what
 * shows it and is given the task to do once that is closed.
 */
export function TaskList({ onDo }: { onDo: (go: () => void) => void }) {
  const { state } = useOnboarding();
  const doTask = useDoTask();
  return (
    <ol className="grid gap-0.5 py-1">
      {TASKS.map((task, i) => {
        const done = state.done.includes(task.key);
        return (
          <li key={task.key} className={cn("flex items-center gap-3 rounded-lg px-3 py-2", !done && "hover:bg-muted")}>
            <span
              className={cn(
                "grid size-6 shrink-0 place-items-center rounded-full text-[12px] font-extrabold",
                done ? "bg-go text-white" : "text-muted-foreground ring-2 ring-inset ring-border",
              )}
            >
              {done ? <Check className="size-3.5" aria-label="完成" /> : i + 1}
            </span>
            <span className="min-w-0 flex-1">
              <span className={cn("block text-[14px] font-bold", done && "text-muted-foreground line-through decoration-muted-foreground/50")}>{task.title}</span>
              <span className="block truncate text-[12.5px] text-muted-foreground">{task.sub}</span>
            </span>
            {!done && (
              <button
                type="button"
                onClick={() => onDo(() => doTask(task))}
                className="shrink-0 rounded-md px-2 py-1 text-[13px] font-bold text-ring hover:bg-ring/10"
              >
                {task.cta}
              </button>
            )}
          </li>
        );
      })}
    </ol>
  );
}

/**
 * Checklist sits at the foot of the side bar: how many things are done, and
 * the list a click away. onGo leaves the side bar (the menu, on a narrow
 * window) before a task is done.
 */
export function Checklist({ onGo }: { onGo: (then?: () => void) => void }) {
  const { state, teaching, hideList } = useOnboarding();
  const [open, setOpen] = useState(false);
  const { later, onCloseAutoFocus } = useAfterClose();
  const closeThen = (action: () => void) => {
    later(action);
    setOpen(false);
  };
  if (!teaching || state.list_hidden) return null;
  const n = TASKS.filter((t) => state.done.includes(t.key)).length;
  const all = n === TASKS.length;
  const Icon = all ? PartyPopper : ListChecks;
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          data-testid="checklist"
          className="w-full rounded-lg bg-white/[.06] px-3 py-2.5 text-left ring-1 ring-white/10 transition-colors hover:bg-white/10"
        >
          <span className="flex items-center gap-2 text-[13.5px] font-bold text-white">
            <Icon className="size-4 text-signal" aria-hidden />
            {all ? "上手清單都完成了" : "上手清單"}
            <span className="ml-auto text-white/70 tnum">
              {n} / {TASKS.length}
            </span>
          </span>
          <span className="mt-2 flex gap-1" aria-hidden>
            {TASKS.map((t) => (
              <span key={t.key} className={cn("h-1.5 flex-1 rounded-full", state.done.includes(t.key) ? "bg-signal" : "bg-white/15")} />
            ))}
          </span>
        </button>
      </PopoverTrigger>
      <PopoverContent side="top" align="start" className="w-[min(360px,calc(100vw-32px))] gap-0 p-2" onCloseAutoFocus={onCloseAutoFocus}>
        <div className="flex items-center gap-2 px-3 pb-1 pt-2">
          <div className="font-extrabold">上手清單</div>
          <span className="text-[13px] text-muted-foreground">做了會自己打勾</span>
          <button type="button" onClick={() => setOpen(false)} aria-label="收起" className="ml-auto grid size-8 place-items-center rounded-md hover:bg-muted">
            <X className="size-4" aria-hidden />
          </button>
        </div>
        <TaskList onDo={(go) => closeThen(() => onGo(go))} />
        <div className="flex items-center gap-2 border-t border-border px-3 pb-1 pt-2.5 text-[12.5px] text-muted-foreground">
          <HardDrive className="size-3.5" aria-hidden />
          進度只記在這台電腦
          <button
            type="button"
            onClick={() => closeThen(hideList)}
            className="ml-auto font-bold text-ink2 hover:text-foreground"
          >
            不再顯示
          </button>
        </div>
      </PopoverContent>
    </Popover>
  );
}

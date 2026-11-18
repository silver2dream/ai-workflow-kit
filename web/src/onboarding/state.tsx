import { createContext, useCallback, useContext, useMemo, useRef, useState, type ReactNode } from "react";
import { toast } from "sonner";
import { api, type Onboarding } from "@/lib/api";
import { TASKS, TOUR } from "@/onboarding/steps";

export interface OnboardingApi {
  state: Onboarding;
  /** The step of the tour shown, within the tour this version has. */
  step: number;
  /** Whether any teaching is shown: not after the person skipped it all. */
  teaching: boolean;
  /** mark ticks a thing of the checklist as done. */
  mark: (task: string) => void;
  startTour: (step?: number) => void;
  next: () => void;
  back: () => void;
  /** pause leaves the tour at its step, to go on from there later. */
  pause: () => void;
  /** end leaves the tour and keeps the checklist. */
  end: () => void;
  /** listOnly is the welcome's third way: no tour, the checklist only. */
  listOnly: () => void;
  skipAll: () => void;
  /** showList brings the checklist back, after it was put away or all teaching skipped. */
  showList: () => void;
  hideList: () => void;
  reset: () => void;
}

const Context = createContext<OnboardingApi | null>(null);

const FRESH: Onboarding = { status: "new", step: 0, done: [], list_hidden: false };

const clampStep = (step: number) => Math.max(0, Math.min(step, TOUR.length - 1));

/**
 * OnboardingProvider holds how far the person is in learning the workbench.
 * Each change is kept at once in their own settings file, by the server: the
 * next time the workbench opens, on any port and in any browser, it goes on
 * from there.
 */
export function OnboardingProvider({ initial, children }: { initial: Onboarding; children: ReactNode }) {
  const [state, setState] = useState<Onboarding>(initial);
  const latest = useRef(initial);

  const keep = useCallback((next: Onboarding) => {
    latest.current = next;
    setState(next);
    api.saveOnboarding(next).catch((error: Error) => {
      toast.error("教學進度沒有存起來", { description: error.message });
    });
  }, []);

  const change = useCallback((edit: (current: Onboarding) => Onboarding) => keep(edit(latest.current)), [keep]);

  const mark = useCallback(
    (task: string) => {
      const current = latest.current;
      // The tour passes the pages of the list; passing them is not doing them.
      if (current.done.includes(task) || current.status === "touring") return;
      keep({ ...current, done: [...current.done, task] });
      const known = TASKS.find((t) => t.key === task);
      // Said only where the list is shown.
      if (known && current.status !== "skipped" && !current.list_hidden) {
        toast.success(`上手清單：${known.title}，完成`);
      }
    },
    [keep],
  );

  const actions = useMemo(() => {
    const finish = () => {
      change((s) => ({ ...s, status: "done", done: s.done.includes("tour") ? s.done : [...s.done, "tour"] }));
      toast.success("導覽完成。上手清單在側邊欄最下面，做了會自己打勾");
    };
    return {
      startTour: (step = 0) => change((s) => ({ ...s, status: "touring", step: clampStep(step) })),
      next: () => {
        if (clampStep(latest.current.step) >= TOUR.length - 1) finish();
        else change((s) => ({ ...s, step: clampStep(s.step + 1) }));
      },
      back: () => change((s) => ({ ...s, step: clampStep(s.step - 1) })),
      pause: () => change((s) => ({ ...s, status: "paused" })),
      end: () => {
        change((s) => ({ ...s, status: "ended" }));
        toast("導覽收起來了。想再看，右上角「說明」裡有");
      },
      listOnly: () => {
        change((s) => ({ ...s, status: "ended", list_hidden: false }));
        toast("不看導覽。上手清單在側邊欄最下面，做了會自己打勾");
      },
      skipAll: () => {
        const before = latest.current;
        keep({ ...before, status: "skipped", list_hidden: true });
        toast("跳過全部教學了。需要時，右上角「說明」可以重新開始", {
          action: { label: "復原", onClick: () => keep(before) },
          duration: 8000,
        });
      },
      showList: () => change((s) => ({ ...s, status: s.status === "skipped" ? "ended" : s.status, list_hidden: false })),
      hideList: () => {
        change((s) => ({ ...s, list_hidden: true }));
        toast("上手清單收起來了，可以從右上角「說明」打開");
      },
      reset: () => keep(FRESH),
    };
  }, [change, keep]);

  const value = useMemo<OnboardingApi>(
    () => ({ state, step: clampStep(state.step), teaching: state.status !== "skipped", mark, ...actions }),
    [state, mark, actions],
  );
  return <Context.Provider value={value}>{children}</Context.Provider>;
}

export function useOnboarding(): OnboardingApi {
  const value = useContext(Context);
  if (!value) throw new Error("useOnboarding outside an OnboardingProvider");
  return value;
}

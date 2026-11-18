import { act, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Onboarding } from "@/lib/api";
import { OnboardingProvider, useOnboarding } from "./state";
import { TOUR } from "./steps";

const saved: Onboarding[] = [];
vi.mock("@/lib/api", () => ({
  api: { saveOnboarding: (o: Onboarding) => (saved.push(o), Promise.resolve(o)) },
}));
const toasts: { message: string; action?: { onClick: () => void } }[] = [];
vi.mock("sonner", () => {
  const toast = (message: string, options?: { action?: { onClick: () => void } }) => toasts.push({ message, action: options?.action });
  toast.success = (message: string) => toasts.push({ message });
  toast.error = (message: string) => toasts.push({ message });
  return { toast };
});

const start = (initial: Partial<Onboarding> = {}) => {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <OnboardingProvider initial={{ status: "new", step: 0, done: [], list_hidden: false, ...initial }}>{children}</OnboardingProvider>
  );
  return renderHook(() => useOnboarding(), { wrapper }).result;
};

beforeEach(() => {
  saved.length = 0;
  toasts.length = 0;
});

describe("the tour", () => {
  it("walks to the last step and ends done, with the tour ticked", () => {
    const o = start();
    act(() => o.current.startTour());
    for (let i = 1; i < TOUR.length; i++) act(() => o.current.next());
    expect(o.current.state).toMatchObject({ status: "touring", step: TOUR.length - 1 });
    act(() => o.current.next());
    expect(o.current.state.status).toBe("done");
    expect(o.current.state.done).toEqual(["tour"]);
    expect(saved.at(-1)).toEqual(o.current.state);
  });

  it("does not step before the first step or past the last", () => {
    const o = start();
    act(() => o.current.startTour(99));
    expect(o.current.state.step).toBe(TOUR.length - 1);
    act(() => o.current.startTour(0));
    act(() => o.current.back());
    expect(o.current.state.step).toBe(0);
  });

  it("pauses at its step and goes on from there", () => {
    const o = start();
    act(() => o.current.startTour(3));
    act(() => o.current.pause());
    expect(o.current.state).toMatchObject({ status: "paused", step: 3 });
    act(() => o.current.startTour(o.current.state.step));
    expect(o.current.state).toMatchObject({ status: "touring", step: 3 });
  });

  it("shows a step this version has, whatever step was kept", () => {
    expect(start({ status: "paused", step: 40 }).current.step).toBe(TOUR.length - 1);
  });
});

describe("the checklist", () => {
  it("ticks a thing once and says so", () => {
    const o = start({ status: "ended" });
    act(() => o.current.mark("health"));
    act(() => o.current.mark("health"));
    expect(o.current.state.done).toEqual(["health"]);
    expect(saved).toHaveLength(1);
    expect(toasts.map((t) => t.message)).toEqual(["上手清單：檢查這台電腦，完成"]);
  });

  it("does not count the pages the tour passes", () => {
    const o = start({ status: "touring", step: 4 });
    act(() => o.current.mark("health"));
    expect(o.current.state.done).toEqual([]);
    expect(saved).toHaveLength(0);
  });

  it("ticks quietly where the list is not shown", () => {
    const o = start({ status: "ended", list_hidden: true });
    act(() => o.current.mark("activity"));
    expect(o.current.state.done).toEqual(["activity"]);
    expect(toasts).toHaveLength(0);
  });

  it("comes back after all teaching was skipped", () => {
    const o = start({ status: "skipped", list_hidden: true });
    expect(o.current.teaching).toBe(false);
    act(() => o.current.showList());
    expect(o.current.state).toMatchObject({ status: "ended", list_hidden: false });
    expect(o.current.teaching).toBe(true);
  });
});

describe("skipping and starting over", () => {
  it("skips all teaching, and the toast's undo gives back what was", () => {
    const o = start({ status: "touring", step: 2, done: ["health"] });
    act(() => o.current.skipAll());
    expect(o.current.state).toMatchObject({ status: "skipped", list_hidden: true, done: ["health"] });
    expect(o.current.teaching).toBe(false);
    const undo = toasts.at(-1)?.action;
    expect(undo).toBeDefined();
    act(() => undo!.onClick());
    expect(o.current.state).toEqual({ status: "touring", step: 2, done: ["health"], list_hidden: false });
    expect(saved.at(-1)).toEqual(o.current.state);
  });

  it("forgets everything on reset", () => {
    const o = start({ status: "done", step: 5, done: ["tour", "health"], list_hidden: true });
    act(() => o.current.reset());
    expect(o.current.state).toEqual({ status: "new", step: 0, done: [], list_hidden: false });
  });

  it("keeps the list and drops the tour when the person wants only the list", () => {
    const o = start();
    act(() => o.current.listOnly());
    expect(o.current.state).toMatchObject({ status: "ended", list_hidden: false });
  });
});

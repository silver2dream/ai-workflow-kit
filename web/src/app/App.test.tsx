import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@/components/ui/tooltip";
import type { Onboarding, ProjectView } from "@/lib/api";
import { App } from "./App";

interface Sent {
  method: string;
  url: string;
  headers: Record<string, string>;
  body?: unknown;
}

/** answer stands in for the workbench's server: the session it is given, and empty projects. */
function answer(session: { status?: number; projects?: ProjectView[]; onboarding?: Onboarding; forgets?: boolean }) {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init: RequestInit = {}) => {
      const method = init.method ?? "GET";
      sent.push({ method, url, headers: (init.headers ?? {}) as Record<string, string>, body: init.body ? JSON.parse(String(init.body)) : undefined });
      const reply = (code: number, body: unknown) => new Response(JSON.stringify(body), { status: code, headers: { "Content-Type": "application/json" } });
      if (session.status) return reply(session.status, { error: "open the workbench with the address `awkit ui` printed" });
      if (url === "/api/session") {
        return reply(200, {
          version: "test",
          settings_path: "settings.json",
          projects: session.projects ?? [],
          onboarding: session.onboarding ?? { status: "skipped", step: 0, done: [], list_hidden: true },
        });
      }
      if (session.forgets) return reply(401, { error: "open the workbench with the address `awkit ui` printed" });
      if (url === "/api/onboarding") return reply(200, sent.at(-1)?.body);
      if (url.endsWith("/status")) return reply(200, { timestamp_utc: "", root: "", run: { state: "no_lock", session: {} }, control: { stop_present: false }, target: {} });
      if (url.includes("/tickets")) return reply(200, { tickets: { open: [], done: 0 }, stages: [], asked_at: "" });
      if (url.includes("/events")) return reply(200, { events: [] });
      if (url.endsWith("/plans")) return reply(200, { plans: [] });
      if (url.endsWith("/engine")) return reply(200, { going: false, lanes: [], earlier: 0, limits: { failures: 3, reviews: 3, conflicts: 3 }, pending: [], replies: [] });
      return reply(404, { error: "no" });
    }),
  );
  return sent;
}

const show = () =>
  render(
    <TooltipProvider>
      <App />
    </TooltipProvider>,
  );

beforeEach(() => {
  window.location.hash = "";
});

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("App", () => {
  it("asks for the address awkit ui printed when the workbench does not know the browser", async () => {
    answer({ status: 401 });
    show();
    expect(await screen.findByText("要用 awkit ui 給的網址打開")).toBeInTheDocument();
  });

  it("says so when the workbench, started again, no longer knows the open page", async () => {
    answer({ projects: [{ id: "rally", path: "D:\\rally", name: "rally" }], forgets: true });
    show();
    expect(await screen.findByText("要用 awkit ui 給的網址打開")).toBeInTheDocument();
  });

  it("goes to the list of projects when there is none to open", async () => {
    answer({ projects: [] });
    show();
    expect(await screen.findByText("還沒有專案")).toBeInTheDocument();
    expect(window.location.hash).toBe("#/projects");
  });

  it("opens the first project that can be opened", async () => {
    answer({
      projects: [
        { id: "gone", path: "D:\\gone", name: "gone", problem: "D:\\gone no longer has .ai/config.yaml" },
        { id: "rally", path: "D:\\rally", name: "rally" },
      ],
    });
    show();
    await waitFor(() => expect(window.location.hash).toBe("#/p/rally/overview"));
    expect(await screen.findByRole("heading", { name: "rally", level: 1 })).toBeInTheDocument();
  });

  it("welcomes a person the first time, and keeps it when they skip all teaching", async () => {
    const sent = answer({ projects: [{ id: "rally", path: "D:\\rally", name: "rally" }], onboarding: { status: "new", step: 0, done: [], list_hidden: false } });
    show();
    const skip = await screen.findByRole("button", { name: "跳過全部教學" });
    await userEvent.click(skip);
    await waitFor(() => expect(screen.queryByText("你和 AI 團隊一起工作的地方")).not.toBeInTheDocument());
    const put = sent.find((s) => s.method === "PUT");
    expect(put?.url).toBe("/api/onboarding");
    expect(put?.headers["X-AWK-Workbench"]).toBe("1");
    expect(put?.body).toMatchObject({ status: "skipped", list_hidden: true });
  });
});

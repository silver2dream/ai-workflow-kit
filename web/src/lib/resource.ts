import { useCallback, useEffect, useRef, useState } from "react";

export interface Resource<T> {
  data?: T;
  error?: Error;
  /** True until the first answer, not while asking again: the page keeps showing what it has. */
  loading: boolean;
  reload: () => void;
}

/**
 * useResource asks for something and asks again when told to. What was
 * shown stays on the page while it is asked again, so a page that follows
 * a run does not flicker every second.
 */
export function useResource<T>(load: () => Promise<T>, deps: readonly unknown[]): Resource<T> {
  const [state, setState] = useState<{ data?: T; error?: Error; loading: boolean }>({ loading: true });
  const [tick, setTick] = useState(0);
  const loadRef = useRef(load);
  loadRef.current = load;

  useEffect(() => {
    let alive = true;
    loadRef
      .current()
      .then((data) => alive && setState({ data, loading: false }))
      .catch((error: Error) => alive && setState((s) => ({ data: s.data, error, loading: false })));
    return () => {
      alive = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tick, ...deps]);

  const reload = useCallback(() => setTick((t) => t + 1), []);
  return { ...state, reload };
}

export type Area = "status" | "events" | "plans" | "tickets" | "meeting" | "settings";

/**
 * useStream listens to what changes in a project. The browser reconnects by
 * itself when the workbench restarts or the line drops; online says whether
 * it is connected now.
 */
export function useStream(url: string | undefined, onChange: (areas: Area[]) => void): boolean {
  const [online, setOnline] = useState(false);
  const handler = useRef(onChange);
  handler.current = onChange;

  useEffect(() => {
    if (!url || typeof EventSource === "undefined") return;
    const source = new EventSource(url, { withCredentials: true });
    source.onopen = () => setOnline(true);
    source.onerror = () => setOnline(false);
    source.addEventListener("change", (event) => {
      try {
        const { areas } = JSON.parse((event as MessageEvent).data) as { areas: Area[] };
        handler.current(areas);
      } catch {
        // A change the page cannot read is no change it acts on.
      }
    });
    return () => source.close();
  }, [url]);

  return online;
}

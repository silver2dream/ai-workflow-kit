import { useEffect, useState } from "react";

/**
 * The address says where the person is: #/p/<project>/<page>/<more>, or
 * #/projects. It lives in the hash, so the workbench's server serves one page
 * and any address can be bookmarked or shared on this computer.
 */
export interface Route {
  project?: string;
  page: string;
  rest: string[];
}

export function parseRoute(hash: string): Route {
  const parts = hash.replace(/^#\/?/, "").split("/").filter(Boolean).map(decodeURIComponent);
  if (parts[0] === "p" && parts[1]) {
    return { project: parts[1], page: parts[2] ?? "overview", rest: parts.slice(3) };
  }
  return { page: parts[0] ?? "", rest: parts.slice(1) };
}

export function href(route: Route): string {
  const tail = [route.page, ...route.rest].filter(Boolean).map(encodeURIComponent).join("/");
  return route.project ? `#/p/${encodeURIComponent(route.project)}/${tail}` : `#/${tail}`;
}

export function navigate(to: string) {
  if (window.location.hash !== to) window.location.hash = to;
}

/** redirect goes elsewhere without leaving the address it came from in the history. */
export function redirect(to: string) {
  if (window.location.hash === to) return;
  window.history.replaceState(null, "", to);
  window.dispatchEvent(new HashChangeEvent("hashchange"));
}

export function useRoute(): Route {
  const [route, setRoute] = useState(() => parseRoute(window.location.hash));
  useEffect(() => {
    const onChange = () => setRoute(parseRoute(window.location.hash));
    window.addEventListener("hashchange", onChange);
    return () => window.removeEventListener("hashchange", onChange);
  }, []);
  return route;
}

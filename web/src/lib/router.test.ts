import { describe as group, expect, it } from "vitest";
import { href, parseRoute } from "./router";

group("parseRoute", () => {
  it("reads a project's page and what follows it", () => {
    expect(parseRoute("#/p/rally/board/306")).toEqual({ project: "rally", page: "board", rest: ["306"] });
  });

  it("takes the overview when a project names no page", () => {
    expect(parseRoute("#/p/rally")).toEqual({ project: "rally", page: "overview", rest: [] });
  });

  it("reads a page of no project, and an empty address", () => {
    expect(parseRoute("#/projects")).toEqual({ page: "projects", rest: [] });
    expect(parseRoute("")).toEqual({ page: "", rest: [] });
  });

  it("gives back what href wrote, a name with spaces and slashes too", () => {
    const route = { project: "my game/v2", page: "board", rest: ["12"] };
    expect(href(route)).toBe("#/p/my%20game%2Fv2/board/12");
    expect(parseRoute(href(route))).toEqual(route);
  });
});

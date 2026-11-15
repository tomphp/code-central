import { describe, expect, it } from "vitest";
import { projectScopeOf, selectedProject, soleProject } from "./projectScope";

const p = (key: string, archived = false) => ({ key, archived });

describe("soleProject", () => {
  it.each([
    ["no Projects", [], undefined],
    ["one active Project", [p("a")], "a"],
    ["one active plus Archived ones", [p("a"), p("b", true), p("c", true)], "a"],
    ["two active", [p("a"), p("b")], undefined],
    ["only Archived ones", [p("a", true)], undefined],
  ])("%s", (_, projects, want) => {
    expect(soleProject(projects)).toBe(want);
  });
});

describe("selectedProject", () => {
  const one = [p("a"), p("old", true)];
  it.each([
    ["an active named Project", "b", [p("a"), p("b")], "b"],
    ["an Archived named Project beats the Sole Project", "old", one, "old"],
    ["an unknown named Project", "zzz", one, "zzz"],
    ["none named, a Sole Project", undefined, one, "a"],
    ["none named, two active", undefined, [p("a"), p("b")], undefined],
    ["none named, no Projects", undefined, [], undefined],
  ])("%s", (_, url, projects, want) => {
    expect(selectedProject(url, projects)).toBe(want);
  });
});

describe("projectScopeOf", () => {
  it("is unresolved until the list loads, unless the URL names a Project", () => {
    expect(projectScopeOf(undefined, undefined, false).resolved).toBe(false);
    expect(projectScopeOf("a", undefined, false)).toEqual({ project: "a", sole: false, resolved: true });
  });
  it("resolves with no Sole Project when the read failed", () => {
    expect(projectScopeOf(undefined, undefined, true)).toEqual({ project: undefined, sole: false, resolved: true });
  });
  it("keeps a named Project over the Sole Project, which is still there", () => {
    expect(projectScopeOf("old", [p("a"), p("old", true)] as never, false)).toEqual({ project: "old", sole: true, resolved: true });
  });
  it("carries the Sole Project", () => {
    expect(projectScopeOf(undefined, [p("a")] as never, false)).toEqual({ project: "a", sole: true, resolved: true });
  });
});

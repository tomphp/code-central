// Repository missing and each Repository's clone (Spec code-central-848):
// the Factory's clone report gives a Project's own Needs Me item per
// missing Repository, with no Ticket, naming the path, the Clone URL, the
// problem and the fix, and each Repository's clone status and path in
// Project Options. On the WASM Core.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld } from "./api/core/core";
import { rowDetail } from "./format";
import { allClear, focus, renderApp, scenario, settle } from "./worldTesting";

const webURL = "git@github.com:acme/web.git";
const problem = "/src/web isn't a git repository";

async function world({ missing = true, repositories = ["web"] } = {}) {
  const w = await newWorld();
  await allClear(w);
  const s = scenario(w);
  await s.project("acme", "Acme");
  for (const name of repositories) {
    await w.api.addRepository("acme", { name, clone_url: `git@github.com:acme/${name}.git` });
  }
  if (repositories.length > 0) {
    await s.clones("acme", [
      { name: "acme", path: "/src/acme", state: "ready" },
      missing
        ? { name: "web", path: "/src/web", state: "missing", problem }
        : { name: "web", path: "/src/web", state: "ready" },
    ]);
  }
  return w;
}

const optionsRow = (name: string) =>
  within(within(screen.getByRole("main", { name: "Project Options" })).getByRole("list", { name: "Repositories" })).getByRole(
    "listitem",
    { name },
  );

describe("Repository missing", () => {
  it("is a one-line detail naming the Repository and its problem", async () => {
    const r = (await (await world()).api.needsMe()).groups[0].tickets[0];
    expect(r.reason).toBe("repository_missing");
    expect(rowDetail(new Date(), r)).toBe(`web: ${problem}`);
  });

  it("is a Focus Item for the Project with the path, Clone URL, problem and fix", async () => {
    await renderApp(await world());
    const it = within(focus()).getByRole("article", { name: "Focus Item" });
    expect(within(it).getByText("Focus: Repository missing")).toBeInTheDocument();
    const panel = within(it).getByRole("region", { name: "Repository missing" });
    expect(panel).toHaveTextContent("/src/web");
    expect(panel).toHaveTextContent(webURL);
    expect(panel).toHaveTextContent(problem);
    expect(panel).toHaveTextContent(/local path/);
    const link = within(panel).getByRole("link", { name: "Project Options" });
    expect(link.getAttribute("href")).toContain("view=project-options");
    expect(link.getAttribute("href")).toContain("project=acme");
    expect(within(it).queryByRole("link", { name: /Open in Inbox/ })).toBeNull();
  });

  it("leaves out the Clone URL row for a local-only Repository", async () => {
    const w = await world({ repositories: [] });
    await w.api.addRepository("acme", { name: "web", local_path: "/src/web" });
    await scenario(w).clones("acme", [
      { name: "acme", path: "/src/acme", state: "ready" },
      { name: "web", path: "/src/web", state: "missing", problem: "nothing at /src/web" },
    ]);
    await renderApp(w);
    const panel = within(focus()).getByRole("region", { name: "Repository missing" });
    expect(panel).toHaveTextContent("/src/web");
    expect(panel).toHaveTextContent("nothing at /src/web");
    expect(within(panel).queryByText("Clone URL")).toBeNull();
    expect(panel).toHaveTextContent(/git repository at \/src\/web, or set its local path/);
  });

  it("is one Focus Item per missing Repository", async () => {
    const w = await world({ repositories: ["web", "api"] });
    await scenario(w).clones("acme", [
      { name: "acme", path: "/src/acme", state: "ready" },
      { name: "web", path: "/src/web", state: "missing", problem },
      { name: "api", path: "/src/api", state: "missing", problem: "/src/api isn't a git repository" },
    ]);
    await renderApp(w);
    const panel = () => within(focus()).getByRole("region", { name: "Repository missing" });
    expect(panel()).toHaveTextContent("/src/web");
    await userEvent.click(within(focus()).getByRole("button", { name: "Skip" }));
    await settle(w);
    expect(panel()).toHaveTextContent("/src/api");
  });

  it("clears once the Factory reports the clone ready", async () => {
    const v = await (await world({ missing: false })).api.needsMe();
    expect(v.groups.flatMap((g) => g.tickets).filter((r) => r.reason === "repository_missing")).toEqual([]);
  });
});

describe("a Repository's clone in Project Options", () => {
  it("shows each Repository cloned at its path, or missing there with the problem", async () => {
    await renderApp(await world(), "/?view=project-options&project=acme");
    expect(optionsRow("acme")).toHaveTextContent("Cloned at /src/acme");
    expect(optionsRow("web")).toHaveTextContent(`Missing at /src/web: ${problem}`);
  });

  it("shows a Repository the Factory hasn't reported as not reported", async () => {
    const w = await world();
    await w.api.addRepository("acme", { name: "api", clone_url: "git@github.com:acme/api.git" });
    await renderApp(w, "/?view=project-options&project=acme");
    expect(optionsRow("api")).toHaveTextContent("Not reported");
  });

  it("shows no clone while the Home Repository is the only one", async () => {
    await renderApp(await world({ repositories: [] }), "/?view=project-options&project=acme");
    expect(within(optionsRow("acme")).queryByText("Clone")).toBeNull();
    expect(optionsRow("acme")).not.toHaveTextContent("Not reported:");
  });
});

// CI Tracking per Repository in the UI (Spec code-central-848), on the
// WASM Core: CI items labelled Name@sha, the Ticket page's CI per
// Repository with failure options acting on that Repository only, each
// Repository's row in Project Options with its CI Provider and target, or
// Not watched (Spec code-central-890), and the CI Watcher's
// per-Repository errors. A
// single-repository Project shows none of it.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld, type World } from "./api/core/core";
import { allClear, renderApp, scenario, settle } from "./worldTesting";

const HOME = "9a8b7c6d5e4f";
const API = "1234567890ab";

// world is acme with the Repository api, on GitHub, added after the Home
// Repository, under Merge + Push with CI Tracking, and an Accepted Ticket
// that changed both: its Home commit passed CI and its api commit failed.
async function world() {
  const w = await newWorld();
  await allClear(w);
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.trackCI("acme");
  await s.repository("acme", "api");
  const spec = await s.spec("acme", { title: "Endpoints", labels: ["long-lived"] });
  const ticket = await s.ticket("acme", { title: "Add an endpoint", state: "Ready", spec });
  await s.accepted("acme", ticket, { branch: "acme-3", commit: HOME, repositories: [{ name: "api", branch: "acme-3", commit: API }] });
  await s.pushed("acme", ticket, HOME);
  await s.pushed("acme", ticket, API, "api");
  await s.ci("acme", ticket, "passed", HOME);
  await s.ci("acme", ticket, "failed", API, 1, "api");
  return { w, s, ticket };
}

const ticketPane = () => screen.getByRole("complementary", { name: "Ticket" });
const OPTIONS = "/?view=project-options&project=acme";

// acme is the Project as listed.
const acme = async (w: World) => (await w.api.listProjects()).find((p) => p.key === "acme")!;

describe("a red build in one of several Repositories", () => {
  it("is a CI item labelled Name@sha", async () => {
    const { w } = await world();
    const rows = (await w.api.needsMe()).groups.flatMap((g) => g.tickets);
    expect(rows.map((r) => r.title)).toEqual(["CI failed at api@1234567"]);
    expect(rows[0].ci_item).toMatchObject({ repository: "api", commit: API });

    await renderApp(w, "/?view=inbox");
    const group = within(screen.getByRole("main", { name: "Tickets" })).getByRole("region", { name: "CI failed" });
    expect(within(group).getByRole("link", { name: /CI failed at api@1234567/ })).toBeInTheDocument();
  });

  it("shows the Ticket's CI per Repository, and Land anyway acts on that Repository only", async () => {
    const { w, ticket } = await world();
    await renderApp(w, `/?view=inbox&ticket=${ticket}`);
    const panel = within(ticketPane()).getByRole("region", { name: "CI failed" });
    expect(panel).toHaveTextContent("CI failed at api@1234567 is one Needs Me item");
    const home = within(panel).getByRole("region", { name: "CI in acme" });
    expect(within(home).getByRole("heading", { level: 4 })).toHaveTextContent("acme passed 9a8b7c6");
    expect(within(home).queryByRole("group", { name: /Failure options/ })).toBeNull();
    const api = within(panel).getByRole("region", { name: "CI in api" });
    expect(within(api).getByRole("heading", { level: 4 })).toHaveTextContent("api failed 1234567");

    const options = within(api).getByRole("group", { name: "Failure options in api" });
    await userEvent.click(within(options).getByRole("button", { name: "Land anyway" }));
    await settle(w);
    const t = await w.api.getTicket("acme", ticket);
    expect(t.repositories?.map((r) => [r.name, !!r.land_anyway])).toEqual([
      ["acme", false],
      ["api", true],
    ]);
    expect(t.land_anyway).toBeFalsy();
  });

  it("names its Repository in a new fix Ticket", async () => {
    const { w, ticket } = await world();
    await renderApp(w, `/?view=inbox&ticket=${ticket}`);
    const options = within(ticketPane()).getByRole("group", { name: "Failure options in api" });
    expect((within(options).getByRole("textbox") as HTMLTextAreaElement).value).toMatch(/^CI failed at api@1234567:/);
    await userEvent.click(within(options).getByRole("button", { name: "New fix Ticket" }));
    await settle(w);
    const fixes = (await w.api.findTickets("acme")).filter((t) => t.title.startsWith("Fix the CI failure"));
    expect(fixes.map((t) => t.title)).toEqual([`Fix the CI failure of ${ticket} in api: Add an endpoint`]);
  });
});

describe("Project Options with several Repositories", () => {
  it("shows each Repository's CI in its row, editable, with Not watched", async () => {
    const { w } = await world();
    await renderApp(w, OPTIONS);
    const rows = within(screen.getByRole("group", { name: "Repositories" })).getByRole("list", { name: "Repositories" });
    const ciOf = (name: string) =>
      within(within(rows).getByRole("listitem", { name })).getByText("CI", { selector: "dt" }).nextElementSibling as HTMLElement;
    expect(ciOf("acme")).toHaveTextContent("GitHub Actions on acme/web");
    expect(ciOf("api")).toHaveTextContent("GitHub Actions on acme/api");
    expect(screen.queryByRole("region", { name: "CI per Repository" })).toBeNull();

    await userEvent.click(within(ciOf("api")).getByRole("button", { name: "Edit api's CI" }));
    const form = within(ciOf("api")).getByRole("form", { name: "api's CI" });
    await userEvent.selectOptions(within(form).getByRole("combobox", { name: "CI" }), "Not watched");
    await userEvent.click(within(form).getByRole("button", { name: "Save" }));
    await settle(w);
    expect((await acme(w)).ci.repositories).toEqual([{ name: "api", not_watched: true }]);
    expect(ciOf("api")).toHaveTextContent("Not watched");
  });

  it("shows the CI Watcher's error per Repository with each fix", async () => {
    const { w, s } = await world();
    await s.reportCIWatcherStatus("acme", {
      state: "error",
      repositories: [{ repository: "api", problem: "repository_unreadable", message: "gh can't read acme/api", fix: "Grant gh access to acme/api." }],
    });
    await renderApp(w, OPTIONS);
    const watcher = screen.getByRole("region", { name: "CI Watcher" });
    const errors = within(watcher).getByRole("list", { name: "Failing Repositories" });
    expect(within(errors).getAllByRole("listitem").map((li) => li.textContent)).toEqual(["api: gh can't read acme/apiGrant gh access to acme/api."]);
    expect(watcher).toHaveTextContent("It carries on watching the others");
  });

  it("offers a CI choice when adding a Repository while CI Tracking is on", async () => {
    const { w } = await world();
    await renderApp(w, OPTIONS);
    const add = screen.getByRole("form", { name: "Add a Repository" });
    await userEvent.type(within(add).getByRole("textbox", { name: "Name" }), "docs");
    await userEvent.type(within(add).getByRole("textbox", { name: "Clone URL" }), "git@github.com:acme/docs.git");
    await userEvent.click(within(add).getByRole("checkbox", { name: /Watch its CI/ }));
    await userEvent.click(within(add).getByRole("button", { name: "Add Repository" }));
    await settle(w);
    expect((await acme(w)).ci.repositories).toContainEqual({ name: "docs", not_watched: true });
  });
});

describe("a single-repository Project", () => {
  it("shows no Repository in its CI section or CI items", async () => {
    const w = await newWorld();
    await allClear(w);
    const s = scenario(w);
    await s.project("acme", "Acme");
    const ticket = await s.ticket("acme", { title: "Login page", state: "Accepted" });
    await s.pushed("acme", ticket, HOME);
    await s.ci("acme", ticket, "failed", HOME);
    const rows = (await w.api.needsMe()).groups.flatMap((g) => g.tickets);
    expect(rows.map((r) => r.title)).toEqual(["CI failed at 9a8b7c6"]);
    expect(rows[0].ci_item?.repository).toBeUndefined();
    await renderApp(w, `/?view=inbox&ticket=${ticket}`);
    expect(within(ticketPane()).queryByRole("region", { name: /CI in / })).toBeNull();
    expect(within(ticketPane()).getByRole("group", { name: "Failure options" })).toBeInTheDocument();
  });
});

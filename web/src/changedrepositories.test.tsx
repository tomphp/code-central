// A Ticket's Changed Repositories in the UI (Spec code-central-848): the
// Board card's name chips, the Ticket page's Repositories section, live
// while In Progress and settled by the finish, the Spec page's chips and
// the Accepted panel's branch@commit per Repository, on the WASM Core. A
// single-repository Project shows none of it.
import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { newWorld } from "./api/core/core";
import { WORK } from "./api/scenario";
import { allClear, renderApp, scenario } from "./worldTesting";

// world is acme with the Repository api added after the Home Repository,
// and an In Progress Ticket on Spec "Endpoints" whose live report names
// api.
async function world() {
  const w = await newWorld();
  await allClear(w);
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.repository("acme", "api");
  const spec = await s.spec("acme", { title: "Endpoints", labels: ["long-lived"] });
  const ticket = await s.ticket("acme", { title: "Add an endpoint", state: "In Progress", spec });
  await s.changed("acme", ticket, { api: "4f1c2d9e8a7b" });
  return { w, s, spec, ticket };
}

const SETTLED = { branch: "acme-3", commit: "9a8b7c6d5e4f", repositories: [{ name: "api", branch: "acme-3", commit: "1234567890ab" }] };

const chips = (scope: HTMLElement) =>
  within(scope)
    .getAllByRole("listitem")
    .map((c) => c.textContent);
const card = (title: string) => within(screen.getByRole("main", { name: "Board" })).getByRole("link", { name: new RegExp(title) });
const ticketPane = () => screen.getByRole("complementary", { name: "Ticket" });
const repositoryRows = () => within(within(ticketPane()).getByRole("region", { name: "Repositories" })).getAllByRole("listitem");
// rowTexts is each row's non-empty grid cells (repository, landing, PR, CI, commit), joined by a space.
const rowTexts = () =>
  repositoryRows().map((r) =>
    Array.from(r.children, (c) => c.textContent ?? "")
      .filter((t) => t !== "")
      .join(" "),
  );

describe("a Ticket's Changed Repositories", () => {
  it("show on its card, live while In Progress, then settled by its finish", async () => {
    const { w, s, ticket } = await world();
    await renderApp(w, "/?view=board&project=acme");
    expect(chips(within(card("Add an endpoint")).getByRole("list", { name: "Changed Repositories" }))).toEqual(["api"]);

    await s.inReview("acme", ticket, SETTLED);
    expect(chips(within(card("Add an endpoint")).getByRole("list", { name: "Changed Repositories" }))).toEqual(["acme", "api"]);
  });

  it("show on its page, one row per Repository with branch@commit, live then settled", async () => {
    const { w, s, ticket } = await world();
    await renderApp(w, `/?view=inbox&ticket=${ticket}`);
    const section = within(ticketPane()).getByRole("region", { name: "Repositories" });
    expect(section).toHaveTextContent("changed so far");
    expect(repositoryRows().map((r) => r.getAttribute("aria-label"))).toEqual(["api"]);
    expect(rowTexts()[0]).toBe(`api ${ticket}@4f1c2d9`);

    await s.inReview("acme", ticket, SETTLED);
    expect(within(ticketPane()).getByRole("region", { name: "Repositories" })).not.toHaveTextContent("changed so far");
    expect(rowTexts()).toEqual(["acme acme-3@9a8b7c6", "api acme-3@1234567"]);
  });

  it("list branch@commit per Repository in the Accepted panel, each not landed yet", async () => {
    const { w, s, ticket } = await world();
    await s.accepted("acme", ticket, SETTLED);
    await renderApp(w, `/?view=inbox&ticket=${ticket}`);
    const panel = within(ticketPane()).getByRole("region", { name: "Accepted" });
    expect(within(panel).getAllByRole("listitem").map((r) => r.textContent)).toEqual(["acme: acme-3@9a8b7c6d5e4f", "api: acme-3@1234567890ab"]);
    expect(rowTexts()).toEqual(["acme not landed acme-3@9a8b7c6", "api not landed acme-3@1234567"]);
  });

  it("show on the Spec page as the union of its Tickets'", async () => {
    const { w, s, spec, ticket } = await world();
    await s.inReview("acme", ticket, SETTLED);
    const other = await s.ticket("acme", { title: "Document it", state: "In Progress", spec });
    await s.changed("acme", other, { api: "aaaa111" });
    await renderApp(w, `/?requirement=${spec}`);
    const pane = screen.getByRole("region", { name: "Spec" });
    expect(chips(within(pane).getByRole("list", { name: "Changed Repositories" }))).toEqual(["acme", "api"]);
  });
});

describe("a single-repository Project", () => {
  it("shows no Repository detail on the card, the Ticket page, the Spec page or the Accepted panel", async () => {
    const w = await newWorld();
    await allClear(w);
    const s = scenario(w);
    await s.project("acme", "Acme");
    const spec = await s.spec("acme", { title: "Endpoints", labels: ["long-lived"] });
    const ticket = await s.ticket("acme", { title: "Add an endpoint", state: "Accepted", spec });

    await renderApp(w, "/?view=board&project=acme");
    expect(within(card("Add an endpoint")).queryByRole("list", { name: "Changed Repositories" })).toBeNull();

    await renderApp(w, `/?view=inbox&ticket=${ticket}`);
    expect(within(ticketPane()).queryByRole("region", { name: "Repositories" })).toBeNull();
    const panel = within(ticketPane()).getByRole("region", { name: "Accepted" });
    expect(panel).toHaveTextContent(`Branch ${WORK.branch} at ${WORK.commit}. It is Done once it is merged into the base and landed.`);
    expect(within(panel).queryByRole("list")).toBeNull();

    await renderApp(w, `/?requirement=${spec}`);
    expect(screen.queryByRole("list", { name: "Changed Repositories" })).toBeNull();
  });
});

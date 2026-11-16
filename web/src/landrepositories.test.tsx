// Landing across Repositories in the UI (Spec code-central-848, ADR 0049):
// a Partly landed Ticket's badge on its card and page, why each
// Repository hasn't landed, land naming a Repository from the Accepted
// panel, Cancel held while Partly landed, Accepted but not landed naming
// the Repositories, and Base diverged and Base missing as one item per
// Repository. On the WASM Core.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld, type World } from "./api/core/core";
import type { NeedsMeRow } from "./api/types";
import { rowDetail } from "./format";
import { allClear, focus, renderApp, scenario, settle } from "./worldTesting";

const BOTH = { branch: "acme-3", commit: "9a8b7c6d5e4f", repositories: [{ name: "api", branch: "acme-3", commit: "1234567890ab" }] };

// world is acme with the Repository api after the Home Repository, and an
// Accepted Ticket whose work is in both.
async function world() {
  const w = await newWorld();
  await allClear(w);
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.repository("acme", "api");
  const spec = await s.spec("acme", { title: "Endpoints", labels: ["long-lived"] });
  const ticket = await s.ticket("acme", { title: "Add an endpoint", state: "Ready", spec });
  await s.accepted("acme", ticket, BOTH);
  return { w, s, ticket };
}

const card = (title: string) => within(screen.getByRole("main", { name: "Board" })).getByRole("link", { name: new RegExp(title) });
const ticketPane = () => screen.getByRole("complementary", { name: "Ticket" });
const repositoryRows = () =>
  within(within(ticketPane()).getByRole("region", { name: "Repositories" }))
    .getAllByRole("listitem")
    .map((r) => r.textContent);

// rows is the Needs Me rows of reason.
async function rows(w: World, reason: string): Promise<NeedsMeRow[]> {
  return (await w.api.needsMe()).groups.filter((g) => g.reason === reason).flatMap((g) => g.tickets);
}

describe("a Ticket landing across Repositories", () => {
  it("shows Partly landed on its card and its page once it has landed in some of them", async () => {
    const { w, s, ticket } = await world();
    await renderApp(w, "/?view=board&project=acme");
    expect(within(card("Add an endpoint")).queryByText("Partly landed")).toBeNull();

    await s.landedIn("acme", ticket, "api", "def4567890ab");
    expect(within(card("Add an endpoint")).getByText("Partly landed")).toBeInTheDocument();

    await renderApp(w, `/?view=inbox&ticket=${ticket}`);
    expect(within(ticketPane()).getByText("Partly landed")).toBeInTheDocument();
    expect(repositoryRows()).toEqual(["acme acme-3@9a8b7c6 · not landed", "api acme-3@1234567 · landed at def4567"]);
  });

  it("says why a Repository hasn't landed", async () => {
    const { w, s, ticket } = await world();
    await s.base("acme", "missing", undefined, "api");
    await renderApp(w, `/?view=inbox&ticket=${ticket}`);
    expect(repositoryRows()[1]).toBe("api acme-3@1234567 · not landed: its base, main, is missing");
  });

  it("lands in the Repository chosen in the Accepted panel, and can't be cancelled once Partly landed", async () => {
    const { w, ticket } = await world();
    await renderApp(w, `/?view=inbox&ticket=${ticket}`);
    const panel = () => within(ticketPane()).getByRole("region", { name: "Accepted" });
    const repository = within(panel()).getByRole("combobox", { name: "Repository it landed in" });
    expect(within(repository).getAllByRole("option").map((o) => o.textContent)).toEqual(["acme", "api"]);
    expect(within(panel()).getByRole("button", { name: "Cancel" })).toBeEnabled();

    await userEvent.selectOptions(repository, "api");
    await userEvent.type(within(panel()).getByRole("textbox", { name: /Commit in the base/ }), "def4567890ab");
    await userEvent.click(within(panel()).getByRole("button", { name: "Land" }));
    await settle(w);

    expect(repositoryRows()[1]).toBe("api acme-3@1234567 · landed at def4567");
    // Only acme is left to land in, so there is nothing to choose.
    expect(within(panel()).queryByRole("combobox", { name: "Repository it landed in" })).toBeNull();
    expect(within(panel()).getByRole("button", { name: "Cancel" })).toBeDisabled();
  });

  it("is Accepted but not landed naming the Repositories it hasn't landed in, with why, and those it has", async () => {
    const { w, s, ticket } = await world();
    await s.landedIn("acme", ticket, "api", "def4567890ab");
    w.advance(11 * 60_000);
    // A Factory reporting now, after the wait, so its report stands.
    await s.base("acme", "missing");

    const [row] = await rows(w, "accepted_not_landed");
    expect(rowDetail(new Date(w.now()), row)).toMatch(/, not landed in acme \(its base, main, is missing\); landed in api$/);
  });
});

describe("a broken base with several Repositories", () => {
  async function bases() {
    const w = await newWorld();
    await allClear(w);
    const s = scenario(w);
    await s.project("acme", "Acme");
    await s.repository("acme", "api");
    return { w, s };
  }

  it("is one Base diverged item per Repository, each naming it", async () => {
    const { w, s } = await bases();
    await s.base("acme", "diverged");
    await s.base("acme", "diverged", { local: "ccc3333444", remote: "ddd4444555" }, "api");

    const diverged = await rows(w, "base_diverged");
    expect(diverged.map((r) => rowDetail(new Date(), r))).toEqual([
      "main in acme is at aaa1111 and the remote's at bbb2222: reconcile them by hand",
      "main in api is at ccc3333 and the remote's at ddd4444: reconcile them by hand",
    ]);

    await renderApp(w);
    const item = () => within(focus()).getByRole("article", { name: "Focus Item" });
    expect(within(item()).getByRole("region", { name: "Base diverged" })).toHaveTextContent("in Repository acme");
    await userEvent.click(within(item()).getByRole("button", { name: "Skip" }));
    await settle(w);
    expect(within(item()).getByRole("region", { name: "Base diverged" })).toHaveTextContent("in Repository api");
  });

  it("is a Base missing item naming its Repository", async () => {
    const { w, s } = await bases();
    await s.base("acme", "missing", undefined, "api");

    const [row] = await rows(w, "base_missing");
    expect(row.base_missing?.repository).toBe("api");
    await renderApp(w);
    const panel = within(within(focus()).getByRole("article", { name: "Focus Item" })).getByRole("region", { name: "Base missing" });
    expect(panel).toHaveTextContent("The base of Repository api, main, exists neither locally nor on the remote. Nothing touching api lands");
  });
});

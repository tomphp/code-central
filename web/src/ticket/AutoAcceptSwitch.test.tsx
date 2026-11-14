// The Auto-accept switch (Spec code-central-589) on a Ticket's detail and on
// the Spec and Requirement panes: the real App against FakeApi.
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";
import type { SpecView, TicketView } from "../api/types";

function renderApp(fake: FakeApi, url: string) {
  window.history.replaceState(null, "", url);
  return render(<App api={fake} now={() => fake.now} />);
}

const detail = () => screen.getByRole("complementary", { name: "Ticket" });
const switchIn = (el: HTMLElement) => within(el).findByRole("switch", { name: "Auto-accept" });

function seeded(opts: { archived?: boolean; spec?: Partial<SpecView> } = {}) {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme", { archived: opts.archived });
  const spec = fake.addSpec("acme", { title: "Accounts", ...opts.spec });
  const ticket = (title: string, state: TicketView["state"], extra: Partial<TicketView> = {}) =>
    fake.addTicket("acme", { title, state, spec: { id: spec.id, title: spec.title }, ...extra });
  return { fake, spec, ticket };
}

const ticketUrl = (t: TicketView) => `/?view=inbox&ticket=${t.id}`;

describe("the Auto-accept switch on a Ticket", () => {
  it("reads on exactly when the Ticket carries the Label, and turns off at once without asking", async () => {
    const { fake, ticket } = seeded();
    const t = ticket("Login page", "Ready", { labels: ["auto-accept"] });
    renderApp(fake, ticketUrl(t));

    const sw = await switchIn(detail());
    expect(sw).toBeChecked();
    await userEvent.click(sw);
    await waitFor(() => expect(sw).not.toBeChecked());
    expect((await fake.getTicket("acme", t.id)).labels).toEqual([]);
  });

  it("turns on at once when it would accept nothing now In Review", async () => {
    const { fake, ticket } = seeded();
    const t = ticket("Login page", "Ready");
    renderApp(fake, ticketUrl(t));

    const sw = await switchIn(detail());
    expect(sw).not.toBeChecked();
    await userEvent.click(sw);
    await waitFor(() => expect(sw).toBeChecked());
    expect(screen.queryByRole("form", { name: "Turn on auto-accept" })).toBeNull();
    expect((await fake.getTicket("acme", t.id)).labels).toEqual(["auto-accept"]);
  });

  it("asks first for a Ticket In Review, accepts it on Turn on, and writes nothing on Cancel", async () => {
    const { fake, ticket } = seeded();
    const t = ticket("Login page", "In Review");
    renderApp(fake, ticketUrl(t));

    const sw = await switchIn(detail());
    await userEvent.click(sw);
    const box = screen.getByRole("form", { name: "Turn on auto-accept" });
    expect(box).toHaveTextContent("This will accept 1 Ticket now In Review. Turn on?");
    await userEvent.click(within(box).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("form", { name: "Turn on auto-accept" })).toBeNull();
    expect(sw).not.toBeChecked();
    expect(fake.calls).not.toContain("addLabels");

    await userEvent.click(sw);
    await userEvent.click(within(screen.getByRole("form", { name: "Turn on auto-accept" })).getByRole("button", { name: "Turn on" }));
    await waitFor(async () => expect((await fake.getTicket("acme", t.id)).state).toBe("Done"));
    expect((await fake.getTicket("acme", t.id)).moves.at(-1)).toMatchObject({ actor: "human", cause: "ticket-label" });
  });

  it("notes that the Spec's Label covers the Ticket, linking the Spec, while the switch reads off", async () => {
    const { fake, spec, ticket } = seeded({ spec: { labels: ["auto-accept"] } });
    const t = ticket("Login page", "Ready");
    renderApp(fake, ticketUrl(t));

    const sw = await switchIn(detail());
    expect(sw).not.toBeChecked();
    expect(detail()).toHaveTextContent("Auto-accepted via its Spec's Label");
    // The note's link is the second Spec link after the one in the meta line.
    const links = within(detail()).getAllByRole("button", { name: `${spec.id} Accounts` });
    expect(links).toHaveLength(2);
  });

  it.each<[string, TicketView["state"], string[], string[]]>([
    ["a Prototype", "Ready", ["prototype"], []],
    ["a Map's Ticket", "Ready", [], ["wayfinder:map"]],
    ["an Accepted Ticket", "Accepted", [], []],
    ["a Done Ticket", "Done", [], []],
    ["a Cancelled Ticket", "Cancelled", [], []],
  ])("is absent on %s", async (_, state, labels, specLabels) => {
    const { fake, ticket } = seeded({ spec: { labels: specLabels } });
    const t = ticket("The one", state, { labels });
    renderApp(fake, ticketUrl(t));

    await within(detail()).findByRole("heading", { name: "The one" });
    await waitFor(() => expect(fake.calls).toContain("getSpec"));
    await act(async () => {});
    expect(within(detail()).queryByRole("switch")).toBeNull();
  });

  it("is read-only on an Archived Project", async () => {
    const { fake, ticket } = seeded({ archived: true });
    const t = ticket("Login page", "Done");
    const ready = ticket("Next page", "Ready", { labels: ["auto-accept"] });
    expect(t.state).toBe("Done");
    renderApp(fake, ticketUrl(ready));

    expect(await switchIn(detail())).toBeDisabled();
  });

  it("shows a refused write's error by the switch and leaves it as it was", async () => {
    const { fake, ticket } = seeded();
    const t = ticket("Login page", "Ready");
    fake.addLabels = async () => {
      throw new Error("the human only");
    };
    renderApp(fake, ticketUrl(t));

    const sw = await switchIn(detail());
    await userEvent.click(sw);
    expect(await within(detail()).findByRole("alert")).toHaveTextContent("the human only");
    expect(sw).not.toBeChecked();
  });
});

describe("the Auto-accept switch on a Spec", () => {
  it("asks with the count of the Spec's Tickets In Review, leaving out a Prototype, then accepts them", async () => {
    const { fake, spec, ticket } = seeded();
    const a = ticket("Login page", "In Review");
    const b = ticket("Sign-up page", "In Review", { labels: ["auto-accept"] });
    ticket("Prototype: picker", "In Review", { labels: ["prototype"] });
    ticket("Logout", "Ready");
    renderApp(fake, `/?view=inbox&requirement=${spec.id}`);

    const pane = await screen.findByRole("region", { name: "Spec" });
    await userEvent.click(await switchIn(pane));
    const box = within(pane).getByRole("form", { name: "Turn on auto-accept" });
    expect(box).toHaveTextContent("This will accept 2 Tickets now In Review. Turn on?");
    await userEvent.click(within(box).getByRole("button", { name: "Turn on" }));

    await waitFor(async () => expect((await fake.getTicket("acme", a.id)).state).toBe("Done"));
    expect((await fake.getTicket("acme", b.id)).state).toBe("Done");
    expect((await fake.getTicket("acme", a.id)).moves.at(-1)).toMatchObject({ cause: "spec-label" });
    expect(await within(pane).findByRole("switch", { name: "Auto-accept" })).toBeChecked();
  });

  it("turns on at once with nothing In Review, and off without asking, on a Long-lived Spec too", async () => {
    const { fake, spec, ticket } = seeded({ spec: { title: "Small Changes", labels: ["long-lived"] } });
    ticket("Tweak", "Ready");
    renderApp(fake, `/?view=inbox&requirement=${spec.id}`);

    const pane = await screen.findByRole("region", { name: "Spec" });
    const sw = await switchIn(pane);
    await userEvent.click(sw);
    await waitFor(() => expect(sw).toBeChecked());
    expect((await fake.getSpec("acme", spec.id)).labels).toEqual(["long-lived", "auto-accept"]);
    await userEvent.click(sw);
    await waitFor(() => expect(sw).not.toBeChecked());
    expect((await fake.getSpec("acme", spec.id)).labels).toEqual(["long-lived"]);
  });

  it("is on a Requirement's pane", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    await fake.submitRequirement("acme", "Export a Spec as Markdown");
    renderApp(fake, "/?view=inbox&requirement=acme-1");

    const sw = await switchIn(await screen.findByRole("region", { name: "Requirement" }));
    expect(sw).not.toBeDisabled();
    await userEvent.click(sw);
    await waitFor(() => expect(sw).toBeChecked());
    expect((await fake.getSpec("acme", "acme-1")).labels).toContain("auto-accept");
  });

  it("is read-only on an Archived Project", async () => {
    const { fake, spec, ticket } = seeded({ archived: true });
    ticket("Login page", "Ready");
    renderApp(fake, `/?view=inbox&requirement=${spec.id}`);

    expect(await switchIn(await screen.findByRole("region", { name: "Spec" }))).toBeDisabled();
  });

  it("is absent on a finished Spec", async () => {
    const { fake, spec, ticket } = seeded();
    ticket("Login page", "Done");
    renderApp(fake, `/?view=inbox&requirement=${spec.id}`);

    const pane = await screen.findByRole("region", { name: "Spec" });
    await within(pane).findByText("Login page");
    expect(within(pane).queryByRole("switch")).toBeNull();
  });

  it("is absent on the Map pane", async () => {
    const { fake, spec, ticket } = seeded({ spec: { labels: ["wayfinder:map"] } });
    ticket("Chart it", "In Review");
    renderApp(fake, `/?view=inbox&requirement=${spec.id}`);

    const pane = await screen.findByRole("region", { name: "Map" });
    expect(within(pane).queryByRole("switch")).toBeNull();
  });
});

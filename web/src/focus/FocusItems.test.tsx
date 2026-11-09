// Ticket Focus Items: the real App against FakeApi, at ?view=focus. One
// Focus Item at a time, in Needs Me's order, with its reason's panel; live
// changes are simulated by reseeding Needs Me and emitting a notice.
import { screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";
import { change, focus, renderApp } from "./focusTesting";

const hourAgo = (fake: FakeApi, h: number) => new Date(fake.now.getTime() - h * 3_600_000).toISOString();

// world has acme-1 In Review (waiting 3h), acme-2 Failed (waiting 1h) and
// beta-1 Accepted but not landed, in that Needs Me order.
function world() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addProject("beta", "Beta");
  fake.addTicket("acme", { title: "Login page", state: "In Review" }); // acme-1
  fake.addTicket("acme", { title: "Flaky build", state: "Failed" }); // acme-2
  fake.addTicket("beta", { title: "Beta land", state: "Accepted", branch: "beta-1" }); // beta-1
  fake.setNeedsMe([
    { reason: "failed", tickets: [{ id: "acme-2", since: hourAgo(fake, 1) }] },
    { reason: "in_review", tickets: [{ id: "acme-1", since: hourAgo(fake, 3), summary: "Added the login page" }] },
    { reason: "accepted_not_landed", tickets: [{ id: "beta-1", since: hourAgo(fake, 26) }] },
  ]);
  return fake;
}

const item = () => within(focus()).findByRole("article", { name: "Focus Item" });

describe("a Ticket Focus Item", () => {
  it("shows where it's from and why it waits, in Needs Me's order", async () => {
    renderApp(world());
    const it = await item();
    expect(within(it).getByText("Acme")).toBeInTheDocument();
    expect(within(it).getByText("acme")).toBeInTheDocument();
    expect(within(it).getByText("acme-2")).toBeInTheDocument();
    expect(within(it).getByRole("heading", { name: "Flaky build" })).toBeInTheDocument();
    expect(within(it).getByText("Failed")).toBeInTheDocument();
    expect(within(it).getByText("waiting 1h")).toBeInTheDocument();
    expect(within(focus()).getAllByRole("article", { name: "Focus Item" })).toHaveLength(1);
  });

  it("comes up To merge like any other Needs Me item, with the Accepted panel's Branch wording", async () => {
    const fake = world();
    fake.setNeedsMe([{ reason: "to_merge", tickets: [{ id: "beta-1", branch: "beta-1", since: hourAgo(fake, 1) }] }]);
    renderApp(fake);
    const it = await item();
    expect(within(it).getByRole("heading", { name: "Beta land" })).toBeInTheDocument();
    expect(within(it).getByText("To merge")).toBeInTheDocument();
    const panel = await within(it).findByRole("region", { name: "Accepted" });
    expect(panel).toHaveTextContent("merge beta-1 into the base and the Factory lands it");
  });

  it("shows the remaining count, the item on screen included", async () => {
    renderApp(world());
    await item();
    expect(within(focus()).getByText("3 left")).toBeInTheDocument();
  });

  it("limits the queue to the Project in scope", async () => {
    renderApp(world(), "/?view=focus&project=beta");
    const it = await item();
    expect(within(it).getByRole("heading", { name: "Beta land" })).toBeInTheDocument();
    expect(within(focus()).getByText("1 left")).toBeInTheDocument();
  });

  it("shows a Ticket on a Requirement with its Requirement, and a Stage Ticket with its Stage too, as itself", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    const spec = fake.addSpec("acme", { title: "Dark mode" }); // acme-1
    const stage = fake.addTicket("acme", {
      title: "Spec: Dark mode",
      state: "In Review",
      spec: { id: spec.id, title: spec.title },
      labels: ["stage:spec"],
    }); // acme-2
    const build = fake.addTicket("acme", { title: "Toggle", state: "Failed", spec: { id: spec.id, title: spec.title } }); // acme-3
    fake.setNeedsMe([
      { reason: "failed", tickets: [{ id: build.id }] },
      { reason: "in_review", tickets: [{ id: stage.id }] },
    ]);
    renderApp(fake);
    expect(within(await item()).getByText("Requirement: Dark mode")).toBeInTheDocument();

    change(fake, [{ reason: "in_review", tickets: [{ id: stage.id }] }], build.id);
    await within(focus()).findByRole("heading", { name: "Spec: Dark mode" });
    expect(within(await item()).getByText("Requirement: Dark mode · spec")).toBeInTheDocument();
  });

  it("shows its reason's panel, with the panel's own buttons", async () => {
    const fake = world();
    fake.setNeedsMe([{ reason: "in_review", tickets: [{ id: "acme-1" }] }]);
    renderApp(fake);
    const panel = await within(await item()).findByRole("region", { name: "In Review" });
    expect(within(panel).getByRole("button", { name: "Accept" })).toBeInTheDocument();
    expect(within(panel).getByRole("button", { name: "Reject" })).toBeInTheDocument();
  });
});

describe("Open in Inbox", () => {
  it("opens the Ticket in the Inbox, keeping the Project filter", async () => {
    renderApp(world(), "/?view=focus&project=acme");
    const link = within(await item()).getByRole("link", { name: "Open in Inbox" });
    expect(link).toHaveAttribute("href", "?view=inbox&project=acme&ticket=acme-2");
    await userEvent.click(link);
    expect(window.location.search).toBe("?view=inbox&project=acme&ticket=acme-2");
    expect(await screen.findByRole("complementary", { name: "Ticket" })).toBeInTheDocument();
  });

  it("opens a Stage Ticket in its Requirement, with the Ticket selected", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    const spec = fake.addSpec("acme", { title: "Dark mode" }); // acme-1
    const stage = fake.addTicket("acme", { title: "Spec: Dark mode", state: "In Review", spec: { id: spec.id, title: spec.title }, labels: ["stage:spec"] });
    fake.setNeedsMe([{ reason: "in_review", tickets: [{ id: stage.id }] }]);
    renderApp(fake);
    const link = within(await item()).getByRole("link", { name: "Open in Inbox" });
    expect(link).toHaveAttribute("href", `?view=inbox&requirement=${spec.id}&ticket=${stage.id}`);
  });
});

describe("moving on", () => {
  it("moves on to the next Focus Item once the human's action takes the Ticket out of that reason", async () => {
    const fake = world();
    fake.setNeedsMe([
      { reason: "in_review", tickets: [{ id: "acme-1" }] },
      { reason: "failed", tickets: [{ id: "acme-2" }] },
    ]);
    renderApp(fake);
    const panel = await within(await item()).findByRole("region", { name: "In Review" });
    await userEvent.click(within(panel).getByRole("button", { name: "Accept" }));

    expect(await within(focus()).findByRole("heading", { name: "Flaky build" })).toBeInTheDocument();
    expect(within(focus()).getByText("1 left")).toBeInTheDocument();
    expect((await fake.getTicket("acme", "acme-1")).state).toBe("Done");
  });

  it("moves on when the item on screen is gone from Needs Me", async () => {
    const fake = world();
    renderApp(fake);
    await within(focus()).findByRole("heading", { name: "Flaky build" });
    change(fake, [
      { reason: "in_review", tickets: [{ id: "acme-1" }] },
      { reason: "accepted_not_landed", tickets: [{ id: "beta-1" }] },
    ], "acme-2");
    expect(await within(focus()).findByRole("heading", { name: "Login page" })).toBeInTheDocument();
    expect(within(focus()).getByText("2 left")).toBeInTheDocument();
  });

  it("treats a Ticket whose reason changes as a new Focus Item at its Needs Me place", async () => {
    const fake = world();
    renderApp(fake);
    await within(focus()).findByRole("heading", { name: "Flaky build" });
    // acme-2 leaves Failed and comes back In Review, behind acme-1.
    fake.updateTicket("acme-2", { state: "In Review" });
    change(fake, [{ reason: "in_review", tickets: [{ id: "acme-1" }, { id: "acme-2" }] }], "acme-2");
    expect(await within(focus()).findByRole("heading", { name: "Login page" })).toBeInTheDocument();
    expect(within(focus()).getByText("2 left")).toBeInTheDocument();
  });

  it("goes to All clear once the last Focus Item is dealt with", async () => {
    const fake = world();
    fake.setNeedsMe([{ reason: "in_review", tickets: [{ id: "acme-1" }] }]);
    renderApp(fake);
    const panel = await within(await item()).findByRole("region", { name: "In Review" });
    await userEvent.click(within(panel).getByRole("button", { name: "Accept" }));
    expect(await within(focus()).findByText("All clear")).toBeInTheDocument();
    expect(within(focus()).queryByText(/left$/)).toBeNull();
  });
});

describe("live arrivals", () => {
  it("queues a new, higher-priority Focus Item without replacing the item on screen", async () => {
    const fake = world();
    renderApp(fake);
    await within(focus()).findByRole("heading", { name: "Flaky build" });
    const t = fake.addTicket("acme", { title: "Quiet session", state: "In Progress" }); // acme-3
    change(fake, [
      { reason: "silent_claim", tickets: [{ id: t.id }] },
      ...fake.needsMeGroups,
    ], t.id);
    await waitFor(() => expect(within(focus()).getByText("4 left")).toBeInTheDocument());
    expect(within(focus()).getByRole("heading", { name: "Flaky build" })).toBeInTheDocument();

    // Once the item on screen is dealt with, the new one is next.
    change(fake, fake.needsMeGroups.filter((g) => g.reason !== "failed"), "acme-2");
    expect(await within(focus()).findByRole("heading", { name: "Quiet session" })).toBeInTheDocument();
  });

  it("keeps the item on screen when Needs Me reorders", async () => {
    const fake = world();
    renderApp(fake);
    await within(focus()).findByRole("heading", { name: "Flaky build" });
    change(fake, [...fake.needsMeGroups].reverse(), "acme-2");
    await waitFor(() => expect(document.title).toBe("(3) Code Central"));
    expect(within(focus()).getByRole("heading", { name: "Flaky build" })).toBeInTheDocument();
  });

  it("shows the first Focus Item that arrives at All clear, with no reload", async () => {
    const fake = world();
    fake.setNeedsMe([]);
    renderApp(fake);
    await within(focus()).findByText("All clear");
    change(fake, [{ reason: "in_review", tickets: [{ id: "acme-1" }] }]);
    expect(await within(focus()).findByRole("heading", { name: "Login page" })).toBeInTheDocument();
    expect(within(focus()).queryByText("All clear")).toBeNull();
  });
});

describe("a Map Ticket or Prototype Focus Item", () => {
  // acme-2 is a Prototype on the Map acme-1, acme-3 a Map grilling Ticket
  // on it and acme-5 a Prototype on the plain Spec acme-4, all In Review.
  function wayfinding() {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    const map = fake.addSpec("acme", { title: "Find the way", labels: ["wayfinder:map"] }); // acme-1
    const onMap = { id: map.id, title: map.title };
    fake.addTicket("acme", { title: "Prototype: the graph", state: "In Review", spec: onMap, labels: ["prototype"] }); // acme-2
    fake.addTicket("acme", { title: "Which store", state: "In Review", spec: onMap, labels: ["wayfinder:grilling"] }); // acme-3
    const plain = fake.addSpec("acme", { title: "Login" }); // acme-4
    fake.addTicket("acme", { title: "Prototype: the form", state: "In Review", spec: { id: plain.id, title: plain.title }, labels: ["prototype"] }); // acme-5
    return fake;
  }

  it("shows its Map's name and a Prototype badge, and opens in the Inbox as itself", async () => {
    const fake = wayfinding();
    fake.setNeedsMe([{ reason: "in_review", tickets: [{ id: "acme-2" }] }]);
    renderApp(fake);
    const it = await item();
    expect(within(it).getByText("Map: Find the way")).toHaveClass("focus-requirement");
    expect(within(it).getByText("Prototype")).toHaveClass("label", "prototype-badge");
    expect(within(it).getByRole("link", { name: "Open in Inbox" })).toHaveAttribute("href", "?view=inbox&ticket=acme-2");
  });

  it("shows no badge on a Map Ticket that isn't a Prototype", async () => {
    const fake = wayfinding();
    fake.setNeedsMe([{ reason: "in_review", tickets: [{ id: "acme-3" }] }]);
    renderApp(fake);
    const it = await item();
    expect(within(it).getByText("Map: Find the way")).toBeInTheDocument();
    expect(within(it).queryByText("Prototype")).toBeNull();
  });

  it("names the plain Spec a Prototype is on", async () => {
    const fake = wayfinding();
    fake.setNeedsMe([{ reason: "in_review", tickets: [{ id: "acme-5" }] }]);
    renderApp(fake);
    const it = await item();
    expect(within(it).getByText("Spec: Login")).toBeInTheDocument();
    expect(within(it).getByText("Prototype")).toBeInTheDocument();
  });
});

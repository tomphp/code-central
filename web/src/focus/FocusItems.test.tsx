// Ticket Focus Items: the real App on a WASM Core world, at ?view=focus. One
// Focus Item at a time, in Needs Me's order, with its reason's panel; live
// changes are made by the operations a human or an agent would make.
import { screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { Scenario } from "../api/scenario";
import { acme, allClearOn, focus, renderApp } from "./worldTesting";

const HOUR = 3_600_000;

// base is acme and beta with All clear on.
async function base() {
  const { w, s } = await acme();
  await allClearOn(w);
  await s.project("beta", "Beta");
  return { w, s };
}

// spec makes a Spec with labels and returns its ID.
async function spec(s: Scenario, key: string, title: string, labels: string[] = []) {
  const id = await s.spec(key, { title });
  if (labels.length) await s.api.addLabels(key, id, labels);
  return id;
}

// stage makes a spec Stage Ticket on spec, finishes it with its Spec
// recommendation and returns its ID.
async function stage(s: Scenario, spec: string, title: string) {
  const id = await s.ticket("acme", { title, spec, labels: ["stage:spec"] });
  await s.finish("acme", id, "Implement: it is small", { recommendation: "implement" });
  return id;
}

// accepted makes beta's Ticket Beta land Accepted on a Long-lived Spec,
// where it is not held for Land per Spec, so once it has waited it is
// Accepted but not landed.
async function accepted(s: Scenario) {
  return s.ticket("beta", { title: "Beta land", state: "Accepted", spec: await spec(s, "beta", "A Spec", ["long-lived"]) });
}

// world has acme's Login page In Review (waiting 3h), acme's Flaky build
// Failed (waiting 1h) and beta's Beta land Accepted but not landed (waiting
// 26h), in that Needs Me order.
async function world() {
  const { w, s } = await base();
  const land = await accepted(s);
  w.advance(23 * HOUR);
  const review = await s.ticket("acme", { title: "Login page", state: "In Review" });
  w.advance(2 * HOUR);
  const failed = await s.ticket("acme", { title: "Flaky build", state: "Failed" });
  w.advance(HOUR);
  return { w, s, land, review, failed };
}

// cancel has the human cancel a Ticket, taking it out of Needs Me.
const cancel = (s: Scenario, key: string, id: string) => s.human(`${s.ticketPath(key, id)}/cancel`, {});

const item = () => within(focus()).findByRole("article", { name: "Focus Item" });

describe("a Ticket Focus Item", () => {
  it("shows where it's from and why it waits, in Needs Me's order", async () => {
    const { w, failed } = await world();
    renderApp(w);
    const it = await item();
    expect(within(it).getByText("Acme")).toBeInTheDocument();
    expect(within(it).getByText("acme")).toBeInTheDocument();
    expect(within(it).getByText(failed)).toBeInTheDocument();
    expect(within(it).getByRole("heading", { name: "Flaky build" })).toBeInTheDocument();
    expect(within(it).getByText("Focus: Failed")).toBeInTheDocument();
    // The waiting line no longer says the reason.
    expect(within(it).getByText("waiting 1h")).toHaveTextContent(/^waiting 1h$/);
    expect(within(focus()).getAllByRole("article", { name: "Focus Item" })).toHaveLength(1);
  });

  it("comes up To merge like any other Needs Me item, with the Accepted panel's Branch wording", async () => {
    const { w, s } = await base();
    await s.human(`${s.projectPath("beta")}/set_landing_method`, { method: "branch" });
    const id = await s.ticket("beta", { title: "Beta land" });
    await s.accepted("beta", id, { branch: "beta-1", commit: "abc123" });
    renderApp(w);
    const it = await item();
    expect(within(it).getByRole("heading", { name: "Beta land" })).toBeInTheDocument();
    expect(within(it).getByText("Focus: To merge")).toBeInTheDocument();
    const panel = await within(it).findByRole("region", { name: "Accepted" });
    expect(panel).toHaveTextContent("merge beta-1 into the base and the Factory lands it");
  });

  it("shows the remaining count, the item on screen included", async () => {
    const { w } = await world();
    renderApp(w);
    await item();
    expect(within(focus()).getByText("3 left")).toBeInTheDocument();
  });

  it("limits the queue to the Project in scope", async () => {
    const { w } = await world();
    renderApp(w, "/?view=focus&project=beta");
    const it = await item();
    expect(within(it).getByRole("heading", { name: "Beta land" })).toBeInTheDocument();
    expect(within(focus()).getByText("1 left")).toBeInTheDocument();
  });

  it("shows a Ticket on a Requirement with its Requirement, and a Stage Ticket with its Stage too, as itself", async () => {
    const { w, s } = await base();
    const dark = await spec(s, "acme", "Dark mode");
    await stage(s, dark, "Spec: Dark mode");
    const build = await s.ticket("acme", { title: "Toggle", state: "Failed", spec: dark });
    renderApp(w);
    expect(within(await item()).getByText("Requirement: Dark mode")).toBeInTheDocument();

    await cancel(s, "acme", build);
    await within(focus()).findByRole("heading", { name: "Spec: Dark mode" });
    expect(within(await item()).getByText("Requirement: Dark mode · spec")).toBeInTheDocument();
  });

  it("shows its reason's panel, with the panel's own buttons", async () => {
    const { w, s } = await base();
    await s.ticket("acme", { title: "Login page", state: "In Review" });
    renderApp(w);
    const panel = await within(await item()).findByRole("region", { name: "In Review" });
    expect(within(panel).getByRole("button", { name: "Accept" })).toBeInTheDocument();
    expect(within(panel).getByRole("button", { name: "Reject" })).toBeInTheDocument();
  });
});

describe("Open in Inbox", () => {
  it("opens the Ticket in the Inbox, keeping the Project filter", async () => {
    const { w, failed } = await world();
    renderApp(w, "/?view=focus&project=acme");
    const link = within(await item()).getByRole("link", { name: "Open in Inbox" });
    expect(link).toHaveAttribute("href", `?view=inbox&project=acme&ticket=${failed}`);
    await userEvent.click(link);
    expect(window.location.search).toBe(`?view=inbox&project=acme&ticket=${failed}`);
    expect(await screen.findByRole("complementary", { name: "Ticket" })).toBeInTheDocument();
  });

  it("opens a Stage Ticket in its Requirement, with the Ticket selected", async () => {
    const { w, s } = await base();
    const dark = await spec(s, "acme", "Dark mode");
    const id = await stage(s, dark, "Spec: Dark mode");
    renderApp(w);
    const link = within(await item()).getByRole("link", { name: "Open in Inbox" });
    expect(link).toHaveAttribute("href", `?view=inbox&requirement=${dark}&ticket=${id}`);
  });
});

describe("moving on", () => {
  it("moves on to the next Focus Item once the human's action takes the Ticket out of that reason", async () => {
    const { w, s } = await base();
    const review = await s.ticket("acme", { title: "Login page", state: "In Review" });
    await accepted(s);
    w.advance(11 * 60_000); // Accepted for long enough to count as not landed
    renderApp(w);
    const panel = await within(await item()).findByRole("region", { name: "In Review" });
    await userEvent.click(within(panel).getByRole("button", { name: "Accept" }));

    expect(await within(focus()).findByRole("heading", { name: "Beta land" })).toBeInTheDocument();
    expect(within(focus()).getByText("1 left")).toBeInTheDocument();
    expect((await w.api.getTicket("acme", review)).state).toBe("Accepted");
  });

  it("moves on when the item on screen is gone from Needs Me", async () => {
    const { w, s, failed } = await world();
    renderApp(w);
    await within(focus()).findByRole("heading", { name: "Flaky build" });
    await cancel(s, "acme", failed);
    expect(await within(focus()).findByRole("heading", { name: "Login page" })).toBeInTheDocument();
    expect(within(focus()).getByText("2 left")).toBeInTheDocument();
  });

  it("treats a Ticket whose reason changes as a new Focus Item at its Needs Me place", async () => {
    const { w, s } = await base();
    await s.ticket("acme", { title: "Login page", state: "In Review" });
    w.advance(HOUR);
    const failed = await s.ticket("acme", { title: "Flaky build", state: "Failed" });
    renderApp(w);
    await within(focus()).findByRole("heading", { name: "Flaky build" });
    // The Failed Ticket leaves Failed and comes back In Review, behind the first.
    await s.human(`${s.ticketPath("acme", failed)}/send_back`, { note: "Try again" });
    await s.inReview("acme", failed);
    expect(await within(focus()).findByRole("heading", { name: "Login page" })).toBeInTheDocument();
    expect(within(focus()).getByText("2 left")).toBeInTheDocument();
  });

  it("goes to All clear once the last Focus Item is dealt with", async () => {
    const { w, s } = await base();
    await s.ticket("acme", { title: "Login page", state: "In Review" });
    renderApp(w);
    const panel = await within(await item()).findByRole("region", { name: "In Review" });
    await userEvent.click(within(panel).getByRole("button", { name: "Accept" }));
    expect(await within(focus()).findByText("All clear")).toBeInTheDocument();
    expect(within(focus()).queryByText(/left$/)).toBeNull();
  });
});

describe("live arrivals", () => {
  // quiet has an agent claim a Ticket, then go silent for long enough
  // to be a Silent Claim, which a write after it makes known.
  async function quiet(w: Awaited<ReturnType<typeof world>>["w"], s: Scenario) {
    const id = await s.ticket("acme", { title: "Quiet session", state: "In Progress" });
    w.advance(31 * 60_000);
    await s.ticket("beta", { title: "Filler" }); // sends the change notice
    return id;
  }

  it("queues a new, higher-priority Focus Item without replacing the item on screen", async () => {
    const { w, s, failed } = await world();
    renderApp(w);
    await within(focus()).findByRole("heading", { name: "Flaky build" });
    await quiet(w, s);
    await waitFor(() => expect(within(focus()).getByText("4 left")).toBeInTheDocument());
    expect(within(focus()).getByRole("heading", { name: "Flaky build" })).toBeInTheDocument();

    // Once the item on screen is dealt with, the new one is next.
    await cancel(s, "acme", failed);
    expect(await within(focus()).findByRole("heading", { name: "Quiet session" })).toBeInTheDocument();
  });

  it("keeps the item on screen when Needs Me reorders", async () => {
    const { w, s } = await world();
    renderApp(w);
    await within(focus()).findByRole("heading", { name: "Flaky build" });
    await quiet(w, s); // sorts ahead of every item
    await waitFor(() => expect(document.title).toBe("(4) Code Central"));
    expect(within(focus()).getByRole("heading", { name: "Flaky build" })).toBeInTheDocument();
  });

  it("shows the first Focus Item that arrives at All clear, with no reload", async () => {
    const { w, s } = await base();
    renderApp(w);
    await within(focus()).findByText("All clear");
    await s.ticket("acme", { title: "Login page", state: "In Review" });
    expect(await within(focus()).findByRole("heading", { name: "Login page" })).toBeInTheDocument();
    expect(within(focus()).queryByText("All clear")).toBeNull();
  });
});

describe("a Retro Focus Item", () => {
  it("shows a Retro badge", async () => {
    const { w, s } = await base();
    const plain = await spec(s, "acme", "Login");
    await s.ticket("acme", { title: "Retro: Login page", state: "In Review", spec: plain, labels: ["retro"] });
    renderApp(w);
    const it = await item();
    expect(within(it).getByText("Retro")).toHaveClass("label", "retro-badge");
    expect(within(it).queryByText("Prototype")).toBeNull();
  });
});

describe("a Map Ticket or Prototype Focus Item", () => {
  // wayfinding is the Map "Find the way" with one In Review Ticket on it,
  // a Prototype or a Map grilling Ticket, or a Prototype on the plain Spec
  // "Login".
  async function wayfinding(on: { map: "prototype" | "grilling" } | { plain: true }) {
    const { w, s } = await base();
    if ("plain" in on) {
      const plain = await spec(s, "acme", "Login");
      const id = await s.ticket("acme", { title: "Prototype: the form", state: "In Review", spec: plain, labels: ["prototype"] });
      return { w, id };
    }
    const map = await spec(s, "acme", "Find the way", ["wayfinder:map"]);
    const labels = on.map === "prototype" ? ["prototype"] : ["wayfinder:grilling"];
    const title = on.map === "prototype" ? "Prototype: the graph" : "Which store";
    const id = await s.ticket("acme", { title, state: "In Review", spec: map, labels });
    return { w, id };
  }

  it("shows its Map's name and a Prototype badge, and opens in the Inbox as itself", async () => {
    const { w, id } = await wayfinding({ map: "prototype" });
    renderApp(w);
    const it = await item();
    expect(within(it).getByText("Map: Find the way")).toHaveClass("focus-requirement");
    expect(within(it).getByText("Prototype")).toHaveClass("label", "prototype-badge");
    expect(within(it).getByText("Focus: In Review")).toBeInTheDocument();
    expect(within(it).getByRole("link", { name: "Open in Inbox" })).toHaveAttribute("href", `?view=inbox&ticket=${id}`);
  });

  it("shows no badge on a Map Ticket that isn't a Prototype", async () => {
    const { w } = await wayfinding({ map: "grilling" });
    renderApp(w);
    const it = await item();
    expect(within(it).getByText("Map: Find the way")).toBeInTheDocument();
    expect(within(it).queryByText("Prototype")).toBeNull();
  });

  it("names the plain Spec a Prototype is on", async () => {
    const { w } = await wayfinding({ plain: true });
    renderApp(w);
    const it = await item();
    expect(within(it).getByText("Spec: Login")).toBeInTheDocument();
    expect(within(it).getByText("Prototype")).toBeInTheDocument();
  });
});

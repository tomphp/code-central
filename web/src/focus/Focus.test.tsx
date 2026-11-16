// Focus Mode tests: the real App on a WASM Core world, at ?view=focus. They
// check what the human sees and does: the URL, the header, All clear,
// loading and errors, and the Projects' Focus Kinds.
import { screen, within, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { World } from "../api/core/core";
import type { Scenario } from "../api/scenario";
import { ApiError } from "../api/types";
import { acme, allClearOn, focus, renderApp, settle } from "./worldTesting";

// world is acme with a Ready Ticket "Pick a colour" and beta with
// "Beta review" In Review, so only beta's needs the human; All clear is on.
// review: false leaves Beta review out, so nothing needs the human.
async function world({ review = true } = {}) {
  const { w, s } = await acme();
  await allClearOn(w);
  await s.project("beta", "Beta");
  const pick = await s.ticket("acme", { title: "Pick a colour" });
  const beta = review ? await s.ticket("beta", { title: "Beta review", state: "In Review" }) : "";
  return { w, s, pick, beta };
}

// accept has the human accept an In Review Ticket, taking it out of Needs Me.
const accept = (s: Scenario, key: string, id: string) => s.human(`${s.ticketPath(key, id)}/accept`, {});

const params = () => new URLSearchParams(window.location.search);

describe("entering and leaving Focus Mode", () => {
  it("enters from the Inbox's Focus button, carrying the Project filter, and Exit returns with it", async () => {
    const { w, pick } = await world();
    await renderApp(w, `/?view=inbox&project=acme&ticket=${pick}`);
    const nav = screen.getByRole("navigation", { name: "Filters" });
    await userEvent.click(within(nav).getByRole("button", { name: "Focus" }));

    expect(window.location.search).toBe("?view=focus&project=acme");
    expect(within(focus()).getByText(/Acme/)).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Filters" })).toBeNull();
    expect(screen.queryByRole("complementary", { name: "Ticket" })).toBeNull();

    await userEvent.click(within(focus()).getByRole("button", { name: "Exit" }));
    await settle(w);
    expect(window.location.search).toBe("?view=inbox&project=acme");
    expect(screen.getByRole("navigation", { name: "Filters" })).toBeInTheDocument();
  });

  it("enters with no project when the Inbox is unfiltered, and shows no Project in its header", async () => {
    const { w } = await world({ review: false }); // no Focus Item, whose own Project would show
    await renderApp(w, "/");
    await userEvent.click(within(screen.getByRole("navigation", { name: "Filters" })).getByRole("button", { name: "Focus" }));
    expect(window.location.search).toBe("?view=focus");
    expect(within(focus()).queryByText(/Acme|Beta/)).toBeNull();
    expect(within(focus()).getByRole("button", { name: "Exit" })).toBeInTheDocument();
  });

  it("returns to where the human was in the Inbox on Back", async () => {
    const { w, pick } = await world();
    await renderApp(w, `/?view=inbox&project=acme&ticket=${pick}`);
    await userEvent.click(within(screen.getByRole("navigation", { name: "Filters" })).getByRole("button", { name: "Focus" }));
    expect(focus()).toBeInTheDocument();

    // jsdom traverses history on a later task and fires popstate then.
    const popped = new Promise((r) => window.addEventListener("popstate", r, { once: true }));
    await act(async () => {
      window.history.back();
      await popped;
    });
    await settle(w);
    expect(screen.getByRole("heading", { name: "Pick a colour" })).toBeInTheDocument();
    expect(params().get("view")).toBe("inbox");
    expect(params().get("project")).toBe("acme");
    expect(params().get("ticket")).toBe(pick);
  });

  it("opens directly from the URL, dropping every parameter but project", async () => {
    const { w, beta } = await world();
    await renderApp(w, `/?view=focus&project=beta&state=Ready&list=requirements&requirement=beta-9&ticket=${beta}`);
    expect(focus()).toBeInTheDocument();
    expect(window.location.search).toBe("?view=focus&project=beta");
  });

  it("treats any other view as the Inbox", async () => {
    const { w } = await world();
    await renderApp(w, "/?view=calendar");
    expect(screen.getByRole("navigation", { name: "Filters" })).toBeInTheDocument();
    expect(params().get("view")).toBe("inbox");
  });
});

describe("All clear", () => {
  it("shows a loading state until the first Needs Me read arrives, never All clear", async () => {
    const { w } = await world({ review: false });
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const needsMe = w.api.needsMe.bind(w.api);
    vi.spyOn(w.api, "needsMe").mockImplementation(async () => {
      await gate;
      return needsMe();
    });
    await renderApp(w, "/?view=focus");
    expect(within(focus()).getByText("Loading…")).toBeInTheDocument();
    expect(within(focus()).queryByText("All clear")).toBeNull();

    release();
    await settle(w);
    expect(within(focus()).getByText("All clear")).toBeInTheDocument();
  });

  it("shows All clear with Nothing needs you, + New ▾ and Exit, and nothing else, when nothing needs the human", async () => {
    const { w } = await world({ review: false });
    await renderApp(w, "/?view=focus");
    const clear = within(focus()).getByRole("region", { name: "All clear" });
    expect(clear).toHaveTextContent(/^All clearNothing needs you\.$/);
    expect(within(focus()).getAllByRole("button").map((b) => b.textContent)).toEqual(["+ New", "▾", "Exit"]);
    expect(within(focus()).queryAllByRole("link")).toEqual([]);
  });

  it("shows All clear when the Options can't be read, with no error banner over it", async () => {
    const { w } = await world({ review: false });
    vi.spyOn(w.api, "options").mockRejectedValue(new ApiError(0, "service_unavailable", "Code Central isn't reachable"));
    await renderApp(w, "/?view=focus");
    expect(within(focus()).getByRole("region", { name: "All clear" })).toBeInTheDocument();
    expect(screen.queryByText("Code Central isn't reachable")).toBeNull();
  });

  it("is not All clear while something in scope needs the human, and becomes All clear when it goes", async () => {
    const { w, s, beta } = await world();
    await renderApp(w, "/?view=focus");
    expect(document.title).toBe("(1) Code Central");
    expect(within(focus()).queryByText("All clear")).toBeNull();

    await accept(s, "beta", beta);
    expect(within(focus()).getByText("All clear")).toBeInTheDocument();
  });

  it("means nothing in the filtered Project needs the human", async () => {
    const { w } = await world(); // only Beta review needs the human
    const { unmount } = await renderApp(w, "/?view=focus&project=acme");
    expect(within(focus()).getByText("All clear")).toBeInTheDocument();
    unmount();

    await renderApp(w, "/?view=focus&project=beta");
    expect(document.title).toBe("(1) Code Central");
    expect(within(focus()).queryByText("All clear")).toBeNull();
  });

  it("counts a Stage Ticket in scope as needing the human", async () => {
    // The tab title counts every Project: no Beta review, so the Stage Ticket is the one.
    const { w, s } = await world({ review: false });
    const spec = await s.spec("acme", { title: "Dark mode" });
    await s.ticket("acme", { title: "Grilling: Dark mode", state: "Needs Answer", spec, labels: ["stage:grilling"] });
    await renderApp(w, "/?view=focus&project=acme");
    expect(document.title).toBe("(1) Code Central");
    expect(within(focus()).queryByText("All clear")).toBeNull();
  });
});

describe("live reads in Focus Mode", () => {
  it("shows the App's error banner on a failed read and keeps the last data", async () => {
    const { w, s } = await world({ review: false });
    await renderApp(w, "/?view=focus");
    vi.spyOn(w.api, "needsMe").mockRejectedValue(new ApiError(0, "unreachable", "Code Central is unreachable"));
    await s.ticket("beta", { title: "Another" }); // sends a change notice
    expect(screen.getByText("Code Central is unreachable")).toBeInTheDocument();
    expect(within(focus()).getByText("All clear")).toBeInTheDocument();
  });

  it("keeps the Needs Me count in the tab title and favicon", async () => {
    const { w, s, beta } = await world();
    await renderApp(w, "/?view=focus&project=beta");
    expect(document.title).toBe("(1) Code Central");
    const icon = () => document.querySelector<HTMLLinkElement>("link[rel=icon]")!;
    expect(decodeURIComponent(icon().href)).toContain(">1<");

    await accept(s, "beta", beta);
    expect(document.title).toBe("Code Central");
  });
});

describe("Focus Kinds in Focus Mode", () => {
  // kinds is acme's Failed "Flaky build" and, unless without names them,
  // In Review "Login page", and beta's In Review "Beta search", in Needs
  // Me's order, and In Review off in acme alone.
  async function kinds({ without = [] as ("flaky" | "beta")[] } = {}) {
    const { w, s } = await acme();
    await allClearOn(w);
    await s.project("beta", "Beta");
    if (!without.includes("flaky")) await s.ticket("acme", { title: "Flaky build", state: "Failed" });
    await s.ticket("acme", { title: "Login page", state: "In Review" });
    if (!without.includes("beta")) await s.ticket("beta", { title: "Beta search", state: "In Review" });
    await w.api.setFocusKinds("acme", ["in_review"]);
    return w;
  }
  const heading = (name: string) => within(focus()).getByRole("heading", { name });
  const skip = async (w: World) => {
    await userEvent.click(within(focus()).getByRole("button", { name: "Skip" }));
    await settle(w);
  };

  it("leaves out a Project's items of a reason off there, and doesn't count them, while another Project's still show", async () => {
    const w = await kinds();
    await renderApp(w, "/?view=focus");
    expect(heading("Flaky build")).toBeInTheDocument();
    expect(within(focus()).getByText("2 left")).toBeInTheDocument();
    await skip(w);
    expect(heading("Beta search")).toBeInTheDocument();
    expect(within(focus()).getByText("2 left")).toBeInTheDocument();
    expect(within(focus()).queryByText("Login page")).toBeNull();
  });

  it("applies only the chosen Project's Focus Kinds with a Project chosen", async () => {
    const w = await kinds();
    const { unmount } = await renderApp(w, "/?view=focus&project=acme");
    expect(heading("Flaky build")).toBeInTheDocument();
    expect(within(focus()).getByText("1 left")).toBeInTheDocument();
    unmount();

    await renderApp(w, "/?view=focus&project=beta");
    expect(heading("Beta search")).toBeInTheDocument();
    expect(within(focus()).getByText("1 left")).toBeInTheDocument();
  });

  it("shows All clear when only items of reasons turned off are left, though Needs Me still counts them", async () => {
    const w = await kinds({ without: ["flaky", "beta"] });
    await renderApp(w, "/?view=focus");
    expect(within(focus()).getByRole("region", { name: "All clear" })).toBeInTheDocument();
    expect(within(focus()).queryByText(/left/)).toBeNull();
    expect(document.title).toBe("(1) Code Central");
  });

  it("shows the Activity Dashboard when only items of reasons turned off are left and it's chosen", async () => {
    const w = await kinds({ without: ["flaky", "beta"] });
    await w.api.setOptions({ focus_clear: "activity_dashboard" });
    await renderApp(w, "/?view=focus");
    expect(within(focus()).getByRole("region", { name: "Activity Dashboard" })).toBeInTheDocument();
  });

  it("follows a saved change without a reload: a reason turned off goes, and comes back once turned on", async () => {
    const w = await kinds();
    await renderApp(w, "/?view=focus&project=acme");
    expect(within(focus()).getByText("1 left")).toBeInTheDocument();

    await act(() => w.api.setFocusKinds("acme", []));
    await settle(w);
    expect(within(focus()).getByText("2 left")).toBeInTheDocument();
    await skip(w);
    expect(heading("Login page")).toBeInTheDocument();

    await act(() => w.api.setFocusKinds("acme", ["in_review", "failed"]));
    await settle(w);
    expect(within(focus()).getByRole("region", { name: "All clear" })).toBeInTheDocument();
  });
});

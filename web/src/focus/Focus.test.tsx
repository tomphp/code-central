// Focus Mode tests: the real App against FakeApi, at ?view=focus. They
// check what the human sees and does: the URL, the header, All clear,
// loading and errors.
import { screen, within, waitFor, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";
import { allClearOn, focus, renderApp } from "./focusTesting";
import { ApiError } from "../api/types";

function world() {
  const fake = new FakeApi();
  allClearOn(fake);
  fake.addProject("acme", "Acme");
  fake.addProject("beta", "Beta");
  fake.addTicket("acme", { title: "Pick a colour", state: "Needs Answer" }); // acme-1
  fake.addTicket("beta", { title: "Beta review", state: "In Review" }); // beta-1
  fake.setNeedsMe([{ reason: "in_review", tickets: [{ id: "beta-1", since: fake.now.toISOString(), summary: "Done" }] }]);
  return fake;
}

const params = () => new URLSearchParams(window.location.search);

describe("entering and leaving Focus Mode", () => {
  it("enters from the Inbox's Focus button, carrying the Project filter, and Exit returns with it", async () => {
    renderApp(world(), "/?view=inbox&project=acme&ticket=acme-1");
    const nav = screen.getByRole("navigation", { name: "Filters" });
    await userEvent.click(within(nav).getByRole("button", { name: "Focus" }));

    expect(window.location.search).toBe("?view=focus&project=acme");
    expect(within(focus()).getByText(/Acme/)).toBeInTheDocument();
    expect(screen.queryByRole("navigation", { name: "Filters" })).toBeNull();
    expect(screen.queryByRole("complementary", { name: "Ticket" })).toBeNull();

    await userEvent.click(within(focus()).getByRole("button", { name: "Exit" }));
    expect(window.location.search).toBe("?view=inbox&project=acme");
    expect(await screen.findByRole("navigation", { name: "Filters" })).toBeInTheDocument();
  });

  it("enters with no project when the Inbox is unfiltered, and shows no Project in its header", async () => {
    const fake = world();
    fake.setNeedsMe([]); // no Focus Item, whose own Project would show
    renderApp(fake, "/");
    await userEvent.click(within(screen.getByRole("navigation", { name: "Filters" })).getByRole("button", { name: "Focus" }));
    expect(window.location.search).toBe("?view=focus");
    expect(within(focus()).queryByText(/Acme|Beta/)).toBeNull();
    expect(within(focus()).getByRole("button", { name: "Exit" })).toBeInTheDocument();
  });

  it("returns to where the human was in the Inbox on Back", async () => {
    renderApp(world(), "/?view=inbox&project=acme&ticket=acme-1");
    await screen.findByRole("heading", { name: "Pick a colour" });
    await userEvent.click(within(screen.getByRole("navigation", { name: "Filters" })).getByRole("button", { name: "Focus" }));
    expect(focus()).toBeInTheDocument();

    act(() => window.history.back());
    expect(await screen.findByRole("heading", { name: "Pick a colour" })).toBeInTheDocument();
    expect(params().get("view")).toBe("inbox");
    expect(params().get("project")).toBe("acme");
    expect(params().get("ticket")).toBe("acme-1");
  });

  it("opens directly from the URL, dropping every parameter but project", async () => {
    renderApp(world(), "/?view=focus&project=beta&state=Ready&list=requirements&requirement=beta-9&ticket=beta-1");
    expect(focus()).toBeInTheDocument();
    expect(window.location.search).toBe("?view=focus&project=beta");
  });

  it("treats any other view as the Inbox", async () => {
    renderApp(world(), "/?view=calendar");
    expect(screen.getByRole("navigation", { name: "Filters" })).toBeInTheDocument();
    expect(params().get("view")).toBe("inbox");
  });
});

describe("All clear", () => {
  it("shows a loading state until the first Needs Me read arrives, never All clear", async () => {
    const fake = world();
    fake.setNeedsMe([]);
    let release!: () => void;
    const gate = new Promise<void>((r) => (release = r));
    const needsMe = fake.needsMe.bind(fake);
    vi.spyOn(fake, "needsMe").mockImplementation(async () => {
      await gate;
      return needsMe();
    });
    renderApp(fake, "/?view=focus");
    expect(within(focus()).getByText("Loading…")).toBeInTheDocument();
    expect(within(focus()).queryByText("All clear")).toBeNull();

    release();
    expect(await within(focus()).findByText("All clear")).toBeInTheDocument();
  });

  it("shows All clear with Nothing needs you, + New ▾ and Exit, and nothing else, when nothing needs the human", async () => {
    const fake = world();
    fake.setNeedsMe([]);
    renderApp(fake, "/?view=focus");
    const clear = await within(focus()).findByRole("region", { name: "All clear" });
    expect(clear).toHaveTextContent(/^All clearNothing needs you\.$/);
    expect(within(focus()).getAllByRole("button").map((b) => b.textContent)).toEqual(["+ New", "▾", "Exit"]);
    expect(within(focus()).queryAllByRole("link")).toEqual([]);
  });

  it("shows All clear when the Options can't be read, with no error banner over it", async () => {
    const fake = world();
    fake.setNeedsMe([]);
    vi.spyOn(fake, "options").mockRejectedValue(new ApiError(0, "service_unavailable", "Code Central isn't reachable"));
    renderApp(fake, "/?view=focus");
    expect(await within(focus()).findByRole("region", { name: "All clear" })).toBeInTheDocument();
    expect(screen.queryByText("Code Central isn't reachable")).toBeNull();
  });

  it("is not All clear while something in scope needs the human, and becomes All clear when it goes", async () => {
    const fake = world();
    renderApp(fake, "/?view=focus");
    await waitFor(() => expect(document.title).toBe("(1) Code Central"));
    expect(within(focus()).queryByText("All clear")).toBeNull();

    fake.setNeedsMe([]);
    act(() => fake.emit({ type: "ticket.changed", project: "beta", id: "beta-1" }));
    expect(await within(focus()).findByText("All clear")).toBeInTheDocument();
  });

  it("means nothing in the filtered Project needs the human", async () => {
    const fake = world(); // only beta-1 needs the human
    const { unmount } = renderApp(fake, "/?view=focus&project=acme");
    expect(await within(focus()).findByText("All clear")).toBeInTheDocument();
    unmount();

    renderApp(fake, "/?view=focus&project=beta");
    await waitFor(() => expect(document.title).toBe("(1) Code Central"));
    expect(within(focus()).queryByText("All clear")).toBeNull();
  });

  it("counts a Stage Ticket in scope as needing the human", async () => {
    const fake = world();
    const spec = fake.addSpec("acme", { title: "Dark mode" });
    const t = fake.addTicket("acme", { title: "Grilling: Dark mode", state: "Needs Answer", spec: { id: spec.id, title: spec.title }, labels: ["stage:grilling"] });
    fake.setNeedsMe([{ reason: "needs_answer", tickets: [{ id: t.id, since: fake.now.toISOString(), round: { number: 1, open: 1, questions: 1 } }] }]);
    renderApp(fake, "/?view=focus&project=acme");
    await waitFor(() => expect(document.title).toBe("(1) Code Central"));
    expect(within(focus()).queryByText("All clear")).toBeNull();
  });
});

describe("live reads in Focus Mode", () => {
  it("shows the App's error banner on a failed read and keeps the last data", async () => {
    const fake = world();
    fake.setNeedsMe([]);
    renderApp(fake, "/?view=focus");
    await within(focus()).findByText("All clear");

    vi.spyOn(fake, "needsMe").mockRejectedValue(new ApiError(0, "unreachable", "Code Central is unreachable"));
    act(() => fake.emit({ type: "ticket.changed", project: "beta", id: "beta-1" }));
    expect(await screen.findByText("Code Central is unreachable")).toBeInTheDocument();
    expect(within(focus()).getByText("All clear")).toBeInTheDocument();
  });

  it("keeps the Needs Me count in the tab title and favicon", async () => {
    const fake = world();
    renderApp(fake, "/?view=focus&project=acme");
    await waitFor(() => expect(document.title).toBe("(1) Code Central"));
    const icon = () => document.querySelector<HTMLLinkElement>("link[rel=icon]")!;
    expect(decodeURIComponent(icon().href)).toContain(">1<");

    fake.setNeedsMe([]);
    act(() => fake.emit({ type: "ticket.changed", project: "beta", id: "beta-1" }));
    await waitFor(() => expect(document.title).toBe("Code Central"));
  });
});

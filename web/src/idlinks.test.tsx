// ID Links in the UI: the real App on a WASM Core world, clicking a link
// whose target is a Spec's or Ticket's full ID in a Ticket's description, and
// checking what opens and the address bar.
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { App } from "./App";
import { newWorld } from "./api/core/core";
import { scenario } from "./api/scenario";

// world makes acme with a Spec (acme-1), a Ready Ticket (acme-2) whose
// description links acme-1 (Spec), acme-3 (Done Ticket), acme-4 (a Ticket
// that needs the human), beta-1 and acme-99 (nothing), and beta with a Spec.
async function world() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("beta", "Beta");
  const spec = await s.spec("acme", { title: "Login" }); // acme-1
  const links = await s.ticket("acme", {
    title: "Linker",
    state: "Ready",
    spec,
    description: "See [spec](acme-1), [old](acme-3), [asking](acme-4), [other](beta-5), [nowhere](acme-99).",
  }); // acme-2
  await s.ticket("acme", { title: "Old work", state: "Done", spec }); // acme-3
  const asking = await s.ticket("acme", { title: "Asking", state: "In Progress", spec }); // acme-4
  await s.askRound("acme", asking);
  await s.spec("beta", { title: "Beta spec" }); // beta-5? ids share a counter per Project
  return { w, links, asking };
}

function renderApp(w: Awaited<ReturnType<typeof world>>, url: string) {
  window.history.replaceState(null, "", url);
  return render(<App api={w.w.api} now={w.w.now} />);
}

const dialog = (name: string) => screen.findByRole("dialog", { name });

beforeEach(() => localStorage.clear());

describe("an ID Link on the Board", () => {
  const open = async (url: string) => {
    renderApp(await world(), url);
    await userEvent.click(await screen.findByText("Linker"));
    return within(await dialog("Ticket popup"));
  };

  it("opens a Spec's popup in place of the open one", async () => {
    const popup = await open("/?view=board&project=acme");
    await userEvent.click(await popup.findByRole("link", { name: "spec" }));
    expect(await dialog("Spec popup")).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "Ticket popup" })).toBeNull();
    expect(window.location.search).toBe("?view=board&project=acme&popup=acme-1");
  });

  it("opens a Done Ticket's, which the Board hasn't loaded, as a Ticket popup", async () => {
    const popup = await open("/?view=board&project=acme");
    await userEvent.click(await popup.findByRole("link", { name: "old" }));
    await waitFor(() => expect(window.location.search).toBe("?view=board&project=acme&popup=acme-3"));
    expect(await within(await dialog("Ticket popup")).findByRole("heading", { name: /Old work/ })).toBeInTheDocument();
  });

  it("opens the Focus popup of a Ticket that needs the human", async () => {
    const popup = await open("/?view=board&project=acme");
    await userEvent.click(await popup.findByRole("link", { name: "asking" }));
    expect(await dialog("Focus popup")).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "Ticket popup" })).toBeNull();
  });

  it("leaves a modified click to the browser", async () => {
    const popup = await open("/?view=board&project=acme");
    const link = await popup.findByRole("link", { name: "spec" });
    expect(link).toHaveAttribute("href", "?view=board&project=acme&popup=acme-1");
    expect(link).not.toHaveAttribute("target");
    fireEvent.click(link, { ctrlKey: true });
    expect(window.location.search).toBe("?view=board&project=acme&popup=acme-2");
  });
});

describe("an ID Link in the Inbox", () => {
  const ticketPane = async () => within(await screen.findByRole("article", { name: "Ticket details" }));
  const start = async (url = "/?project=acme") => {
    const w = await world();
    renderApp(w, url);
    return w;
  };

  it("selects a Ticket, replacing the address so Back returns to the Ticket the link was in", async () => {
    await start("/?project=acme&ticket=acme-2");
    await userEvent.click(await (await ticketPane()).findByRole("link", { name: "old" }));
    await waitFor(() => expect(window.location.search).toBe("?view=inbox&project=acme&ticket=acme-3"));
    await waitFor(() => expect(screen.getByRole("article", { name: "Ticket details" })).toHaveTextContent("Accepted → Done"));
    window.history.back();
    await waitFor(() => expect(window.location.search).toBe("?view=inbox&project=acme&ticket=acme-2"));
  });

  it("selects a Spec through the server as a Spec, never as a Ticket", async () => {
    await start("/?project=acme&ticket=acme-2");
    await userEvent.click(await (await ticketPane()).findByRole("link", { name: "spec" }));
    await waitFor(() => expect(window.location.search).toBe("?view=inbox&project=acme&requirement=acme-1"));
    expect(await screen.findByText(/Login/)).toBeInTheDocument();
  });

  it("switches the Project filter to the ID's Project", async () => {
    await start("/?project=acme&ticket=acme-2");
    await userEvent.click(await (await ticketPane()).findByRole("link", { name: "other" }));
    await waitFor(() => expect(window.location.search).toContain("project=beta"));
  });

  it("shows the Ticket pane's error for an ID that names nothing", async () => {
    await start("/?project=acme&ticket=acme-2");
    await userEvent.click(await (await ticketPane()).findByRole("link", { name: "nowhere" }));
    await waitFor(() => expect(window.location.search).toBe("?view=inbox&project=acme&ticket=acme-99"));
    expect(await screen.findByText(/acme-99/, { selector: "p, div" })).toBeInTheDocument();
  });
});

describe("an ID Link in the Focus popup", () => {
  it("closes the popup and opens the item in the Inbox", async () => {
    const w = await newWorld();
    const s = scenario(w);
    await s.project("acme", "Acme");
    await s.human("/api/set_options", { focus_clear: "all_clear", focus_popup: "on" });
    const spec = await s.spec("acme", { title: "Login" }); // acme-1
    const t = await s.ticket("acme", { title: "Login page", state: "Ready", spec });
    await s.inReview("acme", t, undefined, `Built per [the spec](${spec}).`);
    window.history.replaceState(null, "", "/?view=usage");
    render(<App api={w.api} now={w.now} />);
    const popup = within(await dialog("Focus popup"));
    await userEvent.click(await popup.findByRole("link", { name: "the spec" }));
    await waitFor(() => expect(screen.queryByRole("dialog", { name: "Focus popup" })).toBeNull());
    await waitFor(() => expect(window.location.search).toBe(`?view=inbox&requirement=${spec}`));
  });
});

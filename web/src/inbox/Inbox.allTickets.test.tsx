// Inbox layout tests for the All Tickets view and the CI badge on Ticket rows.
import { within, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { scenario, settle } from "../worldTesting";
import { minutes, startWorld, renderApp, list, nav, detail, rowIds, params, bySpecWorld, bySpec } from "./Inbox.testutil";

describe("the All Tickets view", () => {
  beforeEach(() => localStorage.clear());

  async function ticketsWorld() {
    const w = await bySpecWorld();
    const s = scenario(w);
    // Core stamps a Ticket's creation with its clock, so each is made later than the last.
    w.advance(minutes(10));
    await s.ticket("acme", { title: "Old and done", spec: "acme-5", state: "Done" }); // acme-12
    w.advance(minutes(10));
    await s.human(`${s.projectPath("acme")}/submit_prototype`, { spec: "acme-5", text: "Sketch the menu" }); // acme-13
    w.advance(minutes(10));
    await s.ticket("beta", { title: "Beta newest", spec: "beta-1" }); // beta-3
    return w;
  }
  const allTickets = () => userEvent.click(within(nav()).getByRole("button", { name: /^All Tickets/ }));

  it("sits after Needs Me, counting unfinished Tickets and following the Project filter, with no Requirements link", async () => {
    await renderApp(await ticketsWorld());
    within(nav()).getByRole("button", { name: /^All Tickets/ });
    const names = within(nav()).getAllByRole("button").map((b) => b.textContent);
    expect(names.some((n) => /^Requirements/.test(n ?? ""))).toBe(false);
    const at = names.findIndex((n) => /^Needs Me/.test(n ?? ""));
    expect(names[at + 1]).toMatch(/^All Tickets/);
    // acme: 4 Stages? seeded Tickets 1 Stage Done, the rest unfinished; the Done one and Done Stage excluded.
    const total = Number(within(nav()).getByRole("button", { name: /^All Tickets/ }).textContent!.replace(/\D/g, ""));
    await userEvent.click(within(nav()).getByRole("button", { name: /^Beta/ }));
    const beta = Number(within(nav()).getByRole("button", { name: /^All Tickets/ }).textContent!.replace(/\D/g, ""));
    expect(beta).toBe(2);
    expect(total).toBeGreaterThan(beta);
  });

  it("lists every kind of Ticket inline, newest first by creation across Projects, with chip, label and Spec line", async () => {
    await renderApp(await ticketsWorld(), "/?view=inbox&finished=1");
    within(nav()).getByRole("button", { name: /^All Tickets/ });
    await allTickets();
    expect(params().get("list")).toBe("tickets");
    expect(rowIds()[0]).toBe("beta-3");
    expect(rowIds()[1]).toBe("acme-13");
    expect(rowIds()[2]).toBe("acme-12"); // the oldest made after the rest
    expect(rowIds()).toContain("acme-3"); // a Stage Ticket, inline
    expect(within(list()).queryByText(/run by a Factory/)).toBeNull();

    const proto = within(list()).getByRole("link", { name: /acme-13/ });
    expect(proto).toHaveTextContent("Prototype");
    expect(proto).toHaveTextContent("Ready");
    expect(proto).toHaveTextContent("Spec");
    expect(proto).toHaveTextContent("acme-5");
    expect(proto).toHaveTextContent("Archive finished Projects");
    expect(within(list()).getByRole("link", { name: /acme-3/ })).toHaveTextContent("spec");
    expect(within(list()).getByRole("link", { name: /acme-6/ })).not.toHaveTextContent(/Prototype|grilling/);
    expect(within(within(list()).getByRole("link", { name: /acme-7/ })).getByLabelText("Needs you")).toBeInTheDocument();
  });

  it("follows the Project filter and opens a Ticket on the right", async () => {
    await renderApp(await ticketsWorld(), "/?view=inbox&list=tickets&project=beta");
    expect(rowIds()).toEqual(["beta-3", "beta-2"]);
    await userEvent.click(within(list()).getByRole("link", { name: /Beta thing/ }));
    expect(params().get("ticket")).toBe("beta-2");
    expect(params().get("list")).toBe("tickets");
    expect(detail()).toHaveTextContent("Beta thing");
  });

  it("keeps the grouping toggle, which persists", async () => {
    await renderApp(await ticketsWorld(), "/?view=inbox&list=tickets");
    within(list()).getByRole("button", { name: "Spec" });
    await bySpec();
    expect(params().get("group")).toBe("spec");
    expect(localStorage.length).toBe(1);
  });

  it("groups By Spec in the order of each Spec's newest Ticket, Stage Tickets first, with State chips", async () => {
    await renderApp(await ticketsWorld(), "/?view=inbox&list=tickets&group=spec&project=acme&finished=1");
    const rows = within(list()).getAllByRole("region");
    // acme-5's newest Ticket (acme-13) is newer than any of the Requirement's.
    expect(rows[0]).toHaveAccessibleName("Archive finished Projects");
    expect(within(rows[0]).queryByText("Prototype: Sketch the menu")).toBeNull(); // collapsed
    await userEvent.click(within(rows[0]).getByRole("button", { name: "Archive finished Projects" }));
    const plainIds = within(rows[0]).getAllByRole("link").map((a) => a.getAttribute("data-id")).filter(Boolean);
    expect(plainIds.indexOf("acme-12")).toBeLessThan(plainIds.indexOf("acme-13")); // oldest first
    expect(within(rows[0]).getByText("Prototype: Sketch the menu").closest("a")).toHaveTextContent("Ready");

    const req = within(list()).getByRole("region", { name: "Export a Spec as Markdown" });
    await userEvent.click(within(req).getByRole("button", { name: "Export a Spec as Markdown" }));
    const ids = within(req).getAllByRole("link").map((a) => a.getAttribute("data-id")).filter(Boolean);
    expect(ids.slice(1)).toEqual(["acme-2", "acme-3", "acme-4", "acme-11"]); // Stages first, in Stage order
  });

  it("switching the grouping in a State list switches it here", async () => {
    await renderApp(await ticketsWorld(), "/?state=Ready");
    within(list()).getByRole("button", { name: "Spec" });
    await bySpec();
    await allTickets();
    expect(params().get("group")).toBe("spec");
    expect(within(list()).getAllByRole("region")).not.toHaveLength(0);
  });

  it("hides Done and Cancelled until Show finished, kept in the URL and per browser", async () => {
    const w = await ticketsWorld();
    await renderApp(w, "/?view=inbox&list=tickets");
    within(list()).getByRole("button", { name: "Show finished" });
    expect(rowIds()).not.toContain("acme-12");
    await userEvent.click(within(list()).getByRole("button", { name: "Show finished" }));
    await settle(w);
    expect(params().get("finished")).toBe("1");
    expect(rowIds()).toContain("acme-12");
    expect(localStorage.getItem("code-central.inbox.finished")).toBe("1");

    window.history.replaceState(null, "", "/?view=inbox&list=tickets");
    act(() => window.dispatchEvent(new PopStateEvent("popstate")));
    await settle(w);
    expect(within(list()).getByRole("button", { name: "Show finished" })).toHaveAttribute("aria-pressed", "true");

    await userEvent.click(within(list()).getByRole("button", { name: "Show finished" }));
    await settle(w);
    expect(params().has("finished")).toBe(false);
    expect(rowIds()).not.toContain("acme-12");
  });

  describe("search by ID", () => {
    const box = () => within(list()).getByRole("searchbox", { name: "Ticket ID" });
    const search = (text: string) => userEvent.type(box(), text);

    it("matches substrings and prefixes of the full ID, case-insensitively, across Projects, finished included", async () => {
      const w = await ticketsWorld();
      await renderApp(w, "/?view=inbox&list=tickets");
      within(list()).getByRole("searchbox", { name: "Ticket ID" });
      await search("ACME-1");
      await settle(w);
      expect(rowIds()).toContain("acme-12"); // Done, Show finished off
      expect(rowIds().every((id) => id?.includes("acme-1"))).toBe(true);
      await userEvent.clear(box());
      await search("3");
      await settle(w);
      expect(rowIds()).toContain("beta-3");
      expect(rowIds()).toContain("acme-3");
      expect(window.location.search).not.toContain("3");
    });

    it("never matches titles and says No Tickets match.", async () => {
      await renderApp(await ticketsWorld(), "/?view=inbox&list=tickets");
      within(list()).getByRole("searchbox", { name: "Ticket ID" });
      await search("Sketch");
      expect(within(list()).getByText("No Tickets match.")).toBeInTheDocument();
    });

    it("By Spec shows a Spec with its matching Tickets, or all its Tickets when its own ID matches, opened", async () => {
      await renderApp(await ticketsWorld(), "/?view=inbox&list=tickets&group=spec");
      within(list()).getByRole("searchbox", { name: "Ticket ID" });
      await search("acme-12");
      const row = within(list()).getByRole("region", { name: "Archive finished Projects" });
      expect(within(row).getAllByRole("link").map((a) => a.getAttribute("data-id")).filter(Boolean).slice(1)).toEqual(["acme-12"]); // after the Spec's own link
      await userEvent.clear(box());
      await search("acme-5");
      const whole = within(list()).getByRole("region", { name: "Archive finished Projects" });
      const ids = within(whole).getAllByRole("link").map((a) => a.getAttribute("data-id")).filter(Boolean);
      expect(ids).toEqual(expect.arrayContaining(["acme-12", "acme-13"]));
    });
  });

  it("leaves out fully finished Specs By Spec", async () => {
    const w = await ticketsWorld();
    const s = scenario(w);
    await s.ticket("beta", { title: "Beta shipped", spec: await s.spec("beta", { title: "Shipped plan" }), state: "Done" });
    await renderApp(w, "/?view=inbox&list=tickets&group=spec");
    within(list()).getAllByRole("region");
    expect(within(list()).queryByRole("region", { name: "Shipped plan" })).toBeNull();
    await userEvent.click(within(list()).getByRole("button", { name: "Show finished" }));
    await settle(w);
    expect(within(list()).getByRole("region", { name: "Shipped plan" })).toBeInTheDocument();
  });

  it("says No Tickets yet. with Show finished on and nothing there", async () => {
    const { w } = await startWorld("acme");
    await renderApp(w, "/?view=inbox&list=tickets&finished=1");
    expect(within(list()).getByText("No Tickets yet.")).toBeInTheDocument();
  });

  it("says so when nothing is there", async () => {
    const { w } = await startWorld("acme");
    await renderApp(w, "/?view=inbox&list=tickets");
    expect(within(list()).getByText("No unfinished Tickets.")).toBeInTheDocument();
  });
});

describe("the CI badge on Ticket rows", () => {
  // ciWorld is a Spec (acme-1) with an Accepted Ticket whose CI is running
  // (acme-2), one with none (acme-3) and a Done one that passed (acme-4).
  async function ciWorld() {
    const { w, s } = await startWorld("acme");
    const spec = await s.spec("acme", { title: "Theming" });
    const accept = async (title: string, commit: string) => {
      const id = await s.ticket("acme", { title, spec });
      await s.accepted("acme", id, { branch: title.toLowerCase(), commit });
      return id;
    };
    const palette = await accept("Palette", "aaa1111");
    await s.pushed("acme", palette, "aaa1111");
    await s.ci("acme", palette, "running", "aaa1111", 1);
    await accept("Fonts", "bbb2222");
    const icons = await accept("Icons", "ccc3333");
    await s.pushed("acme", icons, "ccc3333");
    await s.ci("acme", icons, "passed", "ccc3333", 2);
    await s.human(`${s.ticketPath("acme", icons)}/land`, { commit: "ccc3333" });
    return w;
  }
  const row = async (title: string) => (within(list()).getByText(title)).closest("a")!;

  it("shows on a State list's rows, and not on a row with no CI result", async () => {
    await renderApp(await ciWorld(), "/?view=inbox&state=Accepted");
    expect(await row("Palette")).toHaveTextContent("CI running");
    expect(await row("Fonts")).not.toHaveTextContent("CI");
  });

  it("shows on the rows under a Spec row", async () => {
    await renderApp(await ciWorld(), "/?view=inbox&state=Accepted&group=spec");
    await userEvent.click(within(list()).getByRole("button", { name: "Theming" }));
    expect(await row("Palette")).toHaveTextContent("CI running");
  });

  it("shows in All Tickets, but not on a Done Ticket with Show finished on", async () => {
    await renderApp(await ciWorld(), "/?view=inbox&list=tickets&finished=1");
    expect(await row("Palette")).toHaveTextContent("CI running");
    expect(await row("Icons")).not.toHaveTextContent("CI");
  });
});

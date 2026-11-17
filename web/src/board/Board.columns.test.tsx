// Board view tests: the Board link, the columns and the cards.
import { within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it } from "vitest";
import { STATES } from "../api/types";
import { MINUTE, world, renderApp, settled, click, nav, boardLink, board, heads, column, cards, titles, card, columnsButton, loaded } from "./Board.testutil";

beforeEach(() => localStorage.clear());

describe("the Board link", () => {
  it("sits in the View section right after Activity, before Needs Me, on the Inbox's Project", async () => {
    await renderApp(await world(), "/?view=inbox&project=acme&list=tickets");
    const link = boardLink();
    const activity = within(nav()).getByRole("link", { name: "Activity" });
    const needsMe = within(nav()).getByRole("button", { name: /^Needs Me/ });
    expect(activity.compareDocumentPosition(link) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(link.compareDocumentPosition(needsMe) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(link).toHaveAttribute("href", "?view=board&project=acme&back=view%3Dinbox%26project%3Dacme%26list%3Dtickets");
  });

  it("counts the Tickets in the columns the Board opens with, which by default is the All Tickets count", async () => {
    await renderApp(await world(), "/?view=inbox&project=acme");
    expect(boardLink()).toHaveTextContent("Board 3");
    expect(within(nav()).getByRole("button", { name: /^All Tickets/ })).toHaveTextContent("All Tickets 3");
  });

  it("counts every Project's Tickets on All Projects, and carries the Inbox's grouping", async () => {
    await renderApp(await world(), "/?view=inbox&group=spec");
    expect(boardLink()).toHaveTextContent("Board 4");
    expect(boardLink()).toHaveAttribute("href", "?view=board&group=spec&back=view%3Dinbox%26group%3Dspec");
  });

  it("follows the columns the browser last chose", async () => {
    const fake = await world();
    const { unmount } = await renderApp(fake, "/?view=board&project=acme");
    loaded();
    await click(columnsButton());
    await click(within(board()).getByRole("checkbox", { name: "Done" }));
    await click(within(board()).getByRole("checkbox", { name: "Ready" }));
    unmount();

    await renderApp(fake, "/?view=inbox&project=acme");
    expect(boardLink()).toHaveTextContent("Board 2");
  });

  it("opens the Board on the Inbox's Project, and Back to the Inbox returns to the Inbox as it was", async () => {
    await renderApp(await world(), "/?view=inbox&project=acme&state=Ready");
    await click(boardLink());
    expect(within(board()).getByRole("combobox", { name: "Projects" })).toHaveValue("acme");
    expect(loaded()).toBeInTheDocument();

    await click(within(board()).getByRole("button", { name: "Back to the Inbox" }));
    expect(window.location.search).toBe("?view=inbox&project=acme&state=Ready");
    expect(nav()).toBeInTheDocument();
  });
});

describe("the Board's columns", () => {
  it("shows one column per State in workflow order, Done and Cancelled hidden, each with its count", async () => {
    await renderApp(await world(), "/?view=board&project=acme");
    loaded();
    expect(heads()).toEqual(["Draft 0", "Ready 2", "In Progress 1", "Needs Answer 0", "Answered 0", "Failed 0", "In Review 0", "Accepted 0"]);
    expect(columnsButton()).toHaveTextContent("Columns · 8/10");
  });

  it("reads No Tickets in an empty shown column", async () => {
    await renderApp(await world(), "/?view=board&project=acme");
    loaded();
    expect(column("Draft")).toHaveTextContent("No Tickets");
    expect(cards("Draft")).toEqual([]);
    expect(column("Ready")).not.toHaveTextContent("No Tickets");
  });

  it("shows and hides columns with Columns ▾, in the address, and shows · n/10 only while any is hidden", async () => {
    await renderApp(await world(), "/?view=board&project=acme");
    loaded();
    await click(columnsButton());
    expect(within(board()).getAllByRole("checkbox")).toEqual(
      STATES.flatMap((s) => [within(board()).getByRole("checkbox", { name: s }), ...(s === "In Progress" ? [within(board()).getByRole("checkbox", { name: "Needs Me" })] : [])]),
    );
    expect(within(board()).getByRole("checkbox", { name: "Needs Me" })).not.toBeChecked();
    expect(within(board()).getByRole("checkbox", { name: "Done" })).not.toBeChecked();
    expect(within(board()).getByRole("checkbox", { name: "Ready" })).toBeChecked();

    await click(within(board()).getByRole("checkbox", { name: "Done" }));
    expect(heads()).toContain("Done 1");
    expect(new URLSearchParams(window.location.search).get("hide")).toBe("Cancelled");
    expect(columnsButton()).toHaveTextContent("Columns · 9/10");

    await click(within(board()).getByRole("checkbox", { name: "Cancelled" }));
    expect(heads()).toHaveLength(10);
    expect(new URLSearchParams(window.location.search).get("hide")).toBe("");
    expect(columnsButton()).toHaveTextContent(/^Columns$/);

    await click(within(board()).getByRole("checkbox", { name: "In Progress" }));
    expect(heads()).not.toContain("In Progress 1");
    expect(new URLSearchParams(window.location.search).get("hide")).toBe("In Progress");
  });

  it("keeps the browser's pick for a fresh Board with no hide in the address", async () => {
    const fake = await world();
    const { unmount } = await renderApp(fake, "/?view=board&project=acme");
    loaded();
    await click(columnsButton());
    await click(within(board()).getByRole("checkbox", { name: "Done" }));
    await click(within(board()).getByRole("checkbox", { name: "Draft" }));
    unmount();

    await renderApp(fake, "/?view=board&project=acme");
    loaded();
    expect(heads()).toEqual(["Ready 2", "In Progress 1", "Needs Answer 0", "Answered 0", "Failed 0", "In Review 0", "Accepted 0", "Done 1"]);
  });

  it("lets the address's hidden columns win over the browser's pick", async () => {
    const fake = await world();
    const { unmount } = await renderApp(fake, "/?view=board&project=acme");
    loaded();
    await click(columnsButton());
    await click(within(board()).getByRole("checkbox", { name: "Done" }));
    unmount();

    await renderApp(fake, "/?view=board&project=acme&hide=Draft,Ready,Accepted");
    expect(heads()).toEqual(["In Progress 1", "Needs Answer 0", "Answered 0", "Failed 0", "In Review 0", "Done 1", "Cancelled 1"]);
  });

  it("shows every column for an empty hide=", async () => {
    await renderApp(await world(), "/?view=board&project=acme&hide=");
    loaded();
    expect(heads()).toHaveLength(10);
    expect(heads()).toContain("Done 1");
    expect(heads()).toContain("Cancelled 1");
  });

  it("follows the Projects picker, keeping the address", async () => {
    await renderApp(await world(), "/?view=board");
    loaded();
    expect(titles("In Progress")).toEqual(["Beta export", "Sign-up page"]);
    const picker = within(board()).getByRole("combobox", { name: "Projects" });
    await userEvent.selectOptions(picker, "beta");
    await settled();
    expect(window.location.search).toBe("?view=board&project=beta");
    expect(titles("In Progress")).toEqual(["Beta export"]);
    expect(cards("Ready")).toEqual([]);
  });
});

describe("the Board's cards", () => {
  it("show the ID, kind, Blocked, the needs-you dot and the Spec's kind and name", async () => {
    const fake = await world();
    await fake.s.askRound("acme", fake.ids.signup);
    await renderApp(fake, "/?view=board&project=acme");
    loaded();
    const reset = card("Password reset");
    expect(within(reset).getByRole("button", { name: fake.ids.reset })).toHaveTextContent("#6");
    expect(reset).toHaveTextContent("Prototype");
    expect(within(reset).getByTitle("Has open Blockers")).toHaveTextContent(/^Blocked$/);
    expect(within(reset).queryByRole("img", { name: "Needs you" })).toBeNull();
    expect(reset).toHaveTextContent(/Spec\s*Login$/);
    expect(within(card("Sign-up page")).getByRole("img", { name: "Needs you" })).toBeInTheDocument();

    const form = card("Login form");
    expect(within(form).getByRole("button", { name: fake.ids.form })).toBeInTheDocument();
    expect(form).not.toHaveTextContent("Prototype");
    expect(form).not.toHaveTextContent("Blocked");
    expect(within(form).queryByRole("img", { name: "Needs you" })).toBeNull();
  });

  it("show the CI badge of a not-Done Ticket, none for no CI result or a Done Ticket, and follow the live feed", async () => {
    const fake = await world();
    const { s, ids } = fake;
    // pushed makes an Accepted Ticket whose commit's CI is outcome in run.
    const pushed = async (title: string, n: number, outcome: "running" | "failed" | "passed") => {
      const id = await s.ticket("acme", { title, spec: ids.spec });
      await s.accepted("acme", id, { branch: `b${n}`, commit: `c${n}` });
      await s.pushed("acme", id, `c${n}`);
      await s.ci("acme", id, outcome, `c${n}`, n);
      return id;
    };
    const running = await pushed("Checks running", 1, "running");
    await pushed("Checks failed", 2, "failed");
    const shipped = await pushed("Shipped", 3, "passed");
    await s.human(`${s.ticketPath("acme", shipped)}/land`, { commit: "c3" });
    await renderApp(fake, "/?view=board&project=acme&hide=Cancelled");
    loaded();
    expect(card("Checks running")).toHaveTextContent("CI running");
    expect(card("Checks failed")).toHaveTextContent("CI failed");
    expect(card("Password reset")).not.toHaveTextContent("CI");
    expect(card("Shipped")).not.toHaveTextContent("CI");
    await s.ci("acme", running, "passed", "c1", 1);
    expect(card("Checks running")).toHaveTextContent("CI passed");
  });

  it("show full IDs on All Projects, and the Spec every Ticket is on", async () => {
    const fake = await world();
    await renderApp(fake, "/?view=board");
    loaded();
    expect(within(card("Login form")).getByRole("button", { name: fake.ids.form })).toHaveTextContent(fake.ids.form);
    expect(card("Beta export")).toHaveTextContent(new RegExp(`^${fake.ids.beta}\\s*Beta export\\s*Spec\\s*A Spec$`));
  });

  it("are newest first by creation", async () => {
    await renderApp(await world(), "/?view=board&project=acme");
    loaded();
    expect(titles("Ready")).toEqual(["Password reset", "Login form"]);
  });

  it("move column when a Ticket changes State, and pulse then but not on first show", async () => {
    const fake = await world();
    await renderApp(fake, "/?view=board&project=acme");
    loaded();
    expect(within(board()).getAllByRole("link").filter((c) => c.hasAttribute("data-pulsing"))).toEqual([]);

    fake.w.advance(30 * MINUTE);
    await fake.s.claimFor("acme", fake.ids.form);
    expect(titles("In Progress")).toEqual(["Sign-up page", "Login form"]);
    expect(titles("Ready")).toEqual(["Password reset"]);
    expect(card("Login form")).toHaveAttribute("data-pulsing", "true");
    expect(card("Sign-up page")).not.toHaveAttribute("data-pulsing");
  });

  it("pulse when a Ticket changes in place", async () => {
    const fake = await world();
    await renderApp(fake, "/?view=board&project=acme");
    loaded();

    fake.w.advance(30 * MINUTE);
    await fake.w.api.act("acme", fake.ids.signup, "edit_ticket", { title: "Sign-up page, redone" });
    await settled();
    expect(card("Sign-up page, redone")).toHaveAttribute("data-pulsing", "true");
    expect(card("Login form")).not.toHaveAttribute("data-pulsing");
  });

  it("don't pulse when a hidden column is shown", async () => {
    await renderApp(await world(), "/?view=board&project=acme");
    loaded();
    await click(columnsButton());
    await click(within(board()).getByRole("checkbox", { name: "Done" }));
    expect(card("Old work")).not.toHaveAttribute("data-pulsing");
  });

  it("link to the Board's own address with their popup, not into the Inbox", async () => {
    const fake = await world();
    await renderApp(fake, "/?view=board&project=acme&group=spec&hide=");
    loaded();
    expect(card("Login form")).toHaveAttribute("href", `?view=board&project=acme&group=spec&hide=&popup=${fake.ids.form}`);
    expect(card("Old work")).toHaveAttribute("href", `?view=board&project=acme&group=spec&hide=&popup=${fake.ids.old}`);
  });
});

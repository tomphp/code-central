// Usage view tests: Context Size.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { settle } from "../worldTesting";
import { projects, calls, world, renderApp, usage, ops, openTab, claimed, response } from "./Usage.testutil";

describe("Context Size", () => {
  async function withUsage() {
    const { w, s } = await projects();
    const impl = await claimed(s, "acme", "Build it", "sess-impl", [
      {
        ...response(w, 1, "implementation", 40_000),
        model: "claude-sonnet-4-6",
      },
      {
        ...response(w, 2, "implementation", 123_456),
        model: "claude-opus-4-7",
      },
      response(w, 3, "review", 21_000, "toolu_review"),
    ]);
    const spec = await claimed(s, "beta", "Write it", "sess-spec", [{ ...response(w, 4, "spec", 80_000), model: "claude-opus-4-7" }]);
    return { w, s, impl, spec };
  }

  const kindsTable = () => within(usage()).getByRole("table", { name: "Context Size by kind" });
  const sessionsTable = () => within(usage()).getByRole("table", { name: "Recent sessions" });
  const sessionRows = () => within(sessionsTable()).getAllByRole("row").slice(1);
  // cells are a recent session's cells' text, its Session cell its link's,
  // leaving out its Subagents' disclosure.
  const cells = (r: HTMLElement) =>
    within(r)
      .getAllByRole("cell")
      .map((c) => (c.classList.contains("usage-session") ? within(c).getByRole("link").textContent : c.textContent));
  const rows = (table: HTMLElement) =>
    within(table)
      .getAllByRole("row")
      .slice(1)
      .map((r) =>
        within(r)
          .getAllByRole("cell")
          .map((c) => c.textContent),
      );

  it("shows each kind's sessions, average and largest peak, with thousands separators and the exact figure on hover", async () => {
    await renderApp((await withUsage()).w, "/?view=usage");
    await openTab("Context Size");
    expect(within(usage()).getByRole("table", { name: "Context Size by kind" })).toBeInTheDocument();
    expect(
      within(kindsTable())
        .getAllByRole("columnheader")
        .map((h) => h.textContent),
    ).toEqual(["Kind", "Sessions", "Average peak", "Largest peak"]);
    expect(
      within(kindsTable())
        .getAllByRole("rowheader")
        .map((h) => h.textContent),
    ).toEqual(["spec", "implementation", "review"]);
    expect(rows(kindsTable())).toEqual([
      ["1", "80,000", "80,000"],
      ["1", "123,456", "123,456"],
      ["1", "21,000", "21,000"],
    ]);
    expect(within(kindsTable()).getAllByText("123,456")[0]).toHaveAttribute("title", "123,456 tokens");
  });

  it("lists the main sessions newest first, each with its tokens and its Subagents collapsed under it", async () => {
    const { w, impl, spec } = await withUsage();
    await renderApp(w, "/?view=usage");
    await openTab("Context Size");
    await settle(world);
    expect(within(usage()).getByRole("table", { name: "Recent sessions" })).toBeInTheDocument();
    expect(within(sessionsTable()).getAllByRole("columnheader").map((h) => h.textContent)).toEqual([
      "Ticket",
      "Project",
      "Claim",
      "Session",
      "Kind",
      "Model",
      "Started",
      "Peak Context Size",
      "Tokens",
      "Responses",
    ]);
    const sessions = sessionRows();
    expect(sessions.map((r) => within(within(r).getByRole("rowheader")).getByRole("link").textContent)).toEqual([spec, impl]);
    // Each response's tokens are its context and 50 output.
    expect(sessions.map(cells)).toEqual([
      ["Beta", "1", "sess-spec", "spec", "claude-opus-4-7", expect.any(String), "80,000", "80,050", "1"],
      ["Acme", "1", "sess-impl", "implementation", "claude-sonnet-4-6, claude-opus-4-7", expect.any(String), "123,456", "163,556 (184,606 with subagents)", "2"],
    ]);
    expect(within(sessions[1]).getByText("123,456")).toHaveAttribute("title", "123,456 tokens");
    expect(within(sessions[1]).getByText("163,556")).toHaveAttribute("title", "163,556 tokens");
    expect(within(sessions[1]).getByText("184,606")).toHaveAttribute("title", "184,606 tokens");
    expect(within(sessions[0]).getByText("80,050")).toHaveAttribute("title", "80,050 tokens");

    // A session with no Subagents has no disclosure; one with them, collapsed.
    expect(within(sessions[0]).queryByRole("button")).toBeNull();
    expect(within(sessions[1]).getByRole("button", { name: "1 subagent" })).toHaveAttribute("aria-expanded", "false");
    expect(within(sessionsTable()).queryByRole("link", { name: "toolu_review" })).toBeNull();
  });

  it("expands a main session to show its Subagents under it, oldest first, each named by its Agent Type and description", async () => {
    const { w, s } = await projects();
    const impl = await claimed(s, "acme", "Build it", "sess-impl", [
      response(w, 1, "implementation", 40_000),
      { ...response(w, 2, "subagent", 10_000, "toolu_a"), agent_type: "Explore", description: "Map Usage sessions", model: "claude-haiku-4-5" },
      // Recorded before Subagents were named.
      response(w, 3, "subagent", 11_000, "toolu_old"),
      { ...response(w, 4, "review", 12_000, "toolu_review"), description: "Review the diff" },
      { ...response(w, 5, "subagent", 13_000, "toolu_plan"), agent_type: "Plan" },
    ]);
    const spec = await claimed(s, "beta", "Write it", "sess-spec", [response(w, 6, "spec", 80_000)]);
    await renderApp(w, "/?view=usage");
    await openTab("Context Size");
    await settle(world);
    expect(sessionsTable()).toBeInTheDocument();
    const disclosure = within(sessionsTable()).getByRole("button", { name: "4 subagents" });
    expect(disclosure).toHaveAttribute("aria-expanded", "false");

    await userEvent.click(disclosure);
    expect(disclosure).toHaveAttribute("aria-expanded", "true");
    const rows = sessionRows();
    expect(rows.map((r) => within(within(r).getByRole("rowheader")).getByRole("link").textContent)).toEqual([spec, impl, impl, impl, impl, impl]);
    expect(rows.map(cells)).toEqual([
      ["Beta", "1", "sess-spec", "spec", "—", expect.any(String), "80,000", "80,050", "1"],
      ["Acme", "1", "sess-impl", "implementation", "—", expect.any(String), "40,000", "40,050 (86,250 with subagents)", "1"],
      ["Acme", "1", "Explore · Map Usage sessions", "subagent", "claude-haiku-4-5", expect.any(String), "10,000", "10,050", "1"],
      ["Acme", "1", "toolu_old", "subagent", "—", expect.any(String), "11,000", "11,050", "1"],
      ["Acme", "1", "Review the diff", "review", "—", expect.any(String), "12,000", "12,050", "1"],
      ["Acme", "1", "Plan", "subagent", "—", expect.any(String), "13,000", "13,050", "1"],
    ]);
    // Indented under their parent, with no disclosure of their own.
    expect(rows.slice(2).every((r) => r.classList.contains("usage-subagent"))).toBe(true);
    expect(rows.slice(2).every((r) => !within(r).queryByRole("button"))).toBe(true);

    await userEvent.click(disclosure);
    expect(disclosure).toHaveAttribute("aria-expanded", "false");
    expect(sessionRows()).toHaveLength(2);
  });

  it("opens and closes the Subagents from the keyboard", async () => {
    await renderApp((await withUsage()).w, "/?view=usage");
    await openTab("Context Size");
    await settle(world);
    expect(sessionsTable()).toBeInTheDocument();
    const disclosure = within(sessionsTable()).getByRole("button", { name: "1 subagent" });
    expect(disclosure.tagName).toBe("BUTTON");
    disclosure.focus();

    await userEvent.keyboard("{Enter}");
    expect(disclosure).toHaveAttribute("aria-expanded", "true");
    expect(within(sessionsTable()).getByRole("link", { name: "toolu_review" })).toBeInTheDocument();
    await userEvent.keyboard(" ");
    expect(disclosure).toHaveAttribute("aria-expanded", "false");
    expect(within(sessionsTable()).queryByRole("link", { name: "toolu_review" })).toBeNull();
  });

  it("lists a main session whose only responses in the range are its Subagents', with none of its own", async () => {
    const { w, s } = await projects();
    await claimed(s, "acme", "Build it", "sess-impl", [
      // The turn started twenty days before: out of the 7 day range.
      response(w, -20 * 24 * 60, "implementation", 40_000),
      response(w, 1, "subagent", 10_000, "toolu_a"),
    ]);
    await renderApp(w, "/?view=usage");
    await openTab("Context Size");
    await settle(world);
    expect(sessionsTable()).toBeInTheDocument();
    expect(sessionRows().map(cells)).toEqual([["Acme", "1", "sess-impl", "—", "—", expect.any(String), "0", "0 (10,050 with subagents)", "0"]]);
  });

  it("opens a session's Ticket from its link", async () => {
    const { w, impl } = await withUsage();
    await renderApp(w, "/?view=usage&days=30");
    await openTab("Context Size");
    await settle(world);
    expect(within(usage()).getByRole("table", { name: "Recent sessions" })).toBeInTheDocument();
    const link = within(sessionsTable()).getAllByRole("link", {
      name: impl,
    })[0];
    expect(link).toHaveAttribute("href", `?view=inbox&project=acme&ticket=${impl}`);

    await userEvent.click(link);
    expect(window.location.search).toBe(`?view=inbox&project=acme&ticket=${impl}`);
    await settle(world);
    expect(screen.getByRole("heading", { name: "Build it" })).toBeInTheDocument();
  });

  it("follows the scope", async () => {
    const { w, spec } = await withUsage();
    await renderApp(w, "/?view=usage&project=beta");
    await openTab("Context Size");
    await settle(world);
    expect(within(usage()).getByRole("table", { name: "Recent sessions" })).toBeInTheDocument();
    expect(
      within(sessionsTable())
        .getAllByRole("rowheader")
        .map((h) => h.textContent),
    ).toEqual([spec]);
    expect(
      within(kindsTable())
        .getAllByRole("rowheader")
        .map((h) => h.textContent),
    ).toEqual(["spec"]);
  });

  it("says there are no sessions when there are calls but no token usage", async () => {
    const { w, s } = await projects();
    await calls(s, "acme", "find_tickets", 1, {});
    await renderApp(w, "/?view=usage&tab=calls");
    expect(ops()).toEqual(["find_tickets"]);
    await openTab("Context Size");
    expect(within(usage()).getByText("No sessions in All Projects over the last 7 days. Only sessions the Factory runs are counted.")).toBeInTheDocument();
    expect(within(usage()).queryByRole("table", { name: "Recent sessions" })).toBeNull();
  });

  it("says there are no calls when there is token usage but no calls", async () => {
    await renderApp((await withUsage()).w, "/?view=usage&tab=calls");
    await settle(world);
    expect(within(usage()).getByText("No Code Central calls in All Projects over the last 7 days.")).toBeInTheDocument();
    expect(within(usage()).queryByRole("table", { name: "Code Central calls" })).toBeNull();
    await openTab("Context Size");
    expect(within(usage()).getByRole("table", { name: "Context Size by kind" })).toBeInTheDocument();
  });
});

// Usage view tests: the real App on a WASM Core world, at ?view=usage. They
// check what the human sees and does: opening it from the Inbox's Admin
// section and going Back, the address keeping the scope, range and
// tab, the tabs, the scope and range pickers, the Code Central calls table and
// its sorting, the Context Size table and the recent sessions with their
// Ticket links, the Session popup, the empty states, and loading only when
// opened or changed.
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { newWorld, type World } from "../api/core/core";
import { scenario, type Scenario } from "../api/scenario";
import type { Api } from "../api/api";
import type { UsageResponse } from "../api/types";
import { bytes, count } from "../format";

async function projects() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("beta", "Beta");
  await s.project("old", "Old");
  await w.api.archiveProject("old");
  const ticket = await s.ticket("acme", {
    title: "Pick a colour",
    state: "Ready",
  });
  return { w, s, ticket };
}

// calls makes n agent MCP calls of op to project, which Core records as
// Usage with their sizes: a call for a Ticket that isn't there is an error.
async function calls(s: Scenario, project: string, op: string, n: number, args: Record<string, unknown>) {
  for (let i = 0; i < n; i++) await s.mcpCall(project, op as "get_ticket", args);
}

function renderApp(w: World, url = "/", api: Api = w.api) {
  window.history.replaceState(null, "", url);
  return render(<App api={api} now={w.now} />);
}

const usage = () => screen.getByRole("main", { name: "Usage" });
const table = () => within(usage()).getByRole("table", { name: "Code Central calls" });
const scope = () => within(usage()).getByRole("combobox", { name: "Projects" });
const range = () => within(usage()).getByRole("combobox", { name: "Range" });
// column is the text of each body row's cell under the header named.
function column(name: string): string[] {
  const headers = within(table()).getAllByRole("columnheader");
  const i = headers.findIndex((h) => h.textContent?.startsWith(name));
  return within(table())
    .getAllByRole("row")
    .slice(1)
    .map((r) => within(r).getAllByRole(i === 0 ? "rowheader" : "cell")[i === 0 ? 0 : i - 1].textContent ?? "");
}
const ops = () => column("Operation");
const tab = (name: string) => within(usage()).getByRole("tab", { name });
const openTab = (name: string) => userEvent.click(tab(name));

describe("opening and leaving Usage", () => {
  it("opens from the Inbox's Admin section on All Projects and 7 days, and returns to the Inbox with its filter and Ticket", async () => {
    const { w, ticket } = await projects();
    renderApp(w, `/?view=inbox&project=acme&state=Ready&ticket=${ticket}`);
    const nav = screen.getByRole("navigation", { name: "Filters" });
    await userEvent.click(within(nav).getByRole("link", { name: "Usage" }));

    expect(usage()).toBeInTheDocument();
    expect(scope()).toHaveValue("");
    expect(within(scope()).getByRole("option", { selected: true })).toHaveTextContent("All Projects");
    expect(range()).toHaveValue("7");
    expect(new URLSearchParams(window.location.search).get("project")).toBeNull();

    await userEvent.click(within(usage()).getByRole("button", { name: "Back to the Inbox" }));
    expect(window.location.search).toBe(`?view=inbox&project=acme&state=Ready&ticket=${ticket}`);
    expect(await screen.findByRole("navigation", { name: "Filters" })).toBeInTheDocument();
  });

  it("returns to Focus Mode with its Project when the address names it", async () => {
    renderApp((await projects()).w, "/?view=usage&back=view%3Dfocus%26project%3Dacme");
    expect(scope()).toHaveValue("");
    await userEvent.click(within(usage()).getByRole("button", { name: "Back to Focus Mode" }));
    expect(window.location.search).toBe("?view=focus&project=acme");
    expect(screen.getByRole("main", { name: "Focus Mode" })).toBeInTheDocument();
  });

  it("returns to the Inbox when opened straight from the address", async () => {
    renderApp((await projects()).w, "/?view=usage");
    await userEvent.click(within(usage()).getByRole("button", { name: "Back to the Inbox" }));
    expect(window.location.search).toBe("?view=inbox");
  });

  it("returns to where the human was on the browser's Back", async () => {
    renderApp((await projects()).w, "/?view=inbox&project=acme");
    await userEvent.click(within(screen.getByRole("navigation", { name: "Filters" })).getByRole("link", { name: "Usage" }));
    expect(usage()).toBeInTheDocument();

    act(() => window.history.back());
    expect(await screen.findByRole("navigation", { name: "Filters" })).toBeInTheDocument();
    expect(window.location.search).toBe("?view=inbox&project=acme");
  });
});

describe("scope and range", () => {
  it("lists All Projects and every Project, Archived ones marked", async () => {
    renderApp((await projects()).w, "/?view=usage");
    await waitFor(() => expect(within(scope()).getAllByRole("option")).toHaveLength(4));
    expect(
      within(scope())
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual(["All Projects", "Acme", "Beta", "Old (Archived)"]);
    expect(
      within(range())
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual(["Last 7 days", "Last 30 days", "Last 90 days"]);
  });

  it("puts a chosen Project and range in the address, and a reload keeps them", async () => {
    const { w, s, ticket } = await projects();
    // Twenty days ago: only in the 30 and 90 day ranges.
    await calls(s, "acme", "claim", 1, { ticket: "acme-99" });
    w.advance(20 * 24 * 60 * 60_000);
    await calls(s, "acme", "get_ticket", 1, { ticket });
    await calls(s, "beta", "find_tickets", 1, {});
    const { unmount } = renderApp(w, "/?state=Ready");
    await userEvent.click(within(screen.getByRole("navigation", { name: "Filters" })).getByRole("link", { name: "Usage" }));
    await openTab("Code Central calls");
    await waitFor(() => expect(ops()).toEqual(["find_tickets", "get_ticket"]));

    await waitFor(() => expect(within(scope()).getAllByRole("option")).toHaveLength(4));
    await userEvent.selectOptions(scope(), "Acme");
    expect(window.location.search).toBe("?view=usage&project=acme&tab=calls&back=view%3Dinbox%26state%3DReady");
    await waitFor(() => expect(ops()).toEqual(["get_ticket"]));

    await userEvent.selectOptions(range(), "Last 30 days");
    expect(window.location.search).toBe("?view=usage&project=acme&days=30&tab=calls&back=view%3Dinbox%26state%3DReady");
    await waitFor(() => expect(ops()).toEqual(["claim", "get_ticket"]));

    unmount();
    renderApp(w, window.location.search);
    expect(scope()).toHaveValue("acme");
    expect(range()).toHaveValue("30");
    expect(tab("Code Central calls")).toHaveAttribute("aria-selected", "true");
    await waitFor(() => expect(ops()).toEqual(["claim", "get_ticket"]));
    await userEvent.click(within(usage()).getByRole("button", { name: "Back to the Inbox" }));
    expect(window.location.search).toBe("?view=inbox&state=Ready");
  });

  it("goes back to All Projects and 7 days, leaving them out of the address", async () => {
    renderApp((await projects()).w, "/?view=usage&project=acme&days=90");
    await waitFor(() => expect(scope()).toHaveValue("acme"));
    await userEvent.selectOptions(scope(), "All Projects");
    await userEvent.selectOptions(range(), "Last 7 days");
    expect(window.location.search).toBe("?view=usage");
  });

  it("loads when opened and when the scope or range changes, never on a live notice", async () => {
    const { w, s } = await projects();
    renderApp(w, "/?view=usage");
    const reads = () => w.calls.filter((c) => c === "usage").length;
    await waitFor(() => expect(reads()).toBe(1));

    await calls(s, "acme", "find_tickets", 1, {});
    await act(() => s.ticket("acme", { title: "Another" }));
    act(() => w.dropStreams());
    await act(() => new Promise((r) => setTimeout(r, 20)));
    expect(reads()).toBe(1);
    expect(within(usage()).getByText(/No token usage/)).toBeInTheDocument();

    await userEvent.selectOptions(range(), "Last 30 days");
    await waitFor(() => expect(reads()).toBe(2));
    await openTab("Code Central calls");
    await waitFor(() => expect(ops()).toEqual(["find_tickets"]));
  });
});

describe("tabs", () => {
  async function withUsage() {
    const { w, s, ticket } = await projects();
    await calls(s, "acme", "get_ticket", 2, { ticket });
    await calls(s, "beta", "find_tickets", 1, {});
    return w;
  }
  const tabs = () => within(usage()).getAllByRole("tab");
  const selected = () => within(usage()).getByRole("tab", { selected: true });

  it("opens on Token usage, the tabs in order, with no tab in the address", async () => {
    renderApp(await withUsage(), "/?view=usage");
    expect(tabs().map((t) => t.textContent)).toEqual(["Token usage", "Context Size", "Code Central calls"]);
    expect(selected()).toHaveTextContent("Token usage");
    expect(within(usage()).getByRole("tabpanel")).toHaveAccessibleName("Token usage");
    expect(window.location.search).toBe("?view=usage");
  });

  it("opens on Token usage for a tab it doesn't know, and writes it with no tab", async () => {
    renderApp(await withUsage(), "/?view=usage&tab=nonsense");
    expect(selected()).toHaveTextContent("Token usage");
    expect(window.location.search).toBe("?view=usage");
  });

  it("puts the open tab in the address, leaving Token usage out, and a reload keeps it", async () => {
    const w = await withUsage();
    const { unmount } = renderApp(w, "/?view=usage");
    await openTab("Context Size");
    expect(window.location.search).toBe("?view=usage&tab=context");
    await openTab("Code Central calls");
    expect(window.location.search).toBe("?view=usage&tab=calls");
    await waitFor(() => expect(ops()).toEqual(["get_ticket", "find_tickets"]));

    unmount();
    renderApp(w, window.location.search);
    expect(selected()).toHaveTextContent("Code Central calls");
    await waitFor(() => expect(ops()).toEqual(["get_ticket", "find_tickets"]));

    await openTab("Token usage");
    expect(window.location.search).toBe("?view=usage");
  });

  it("moves between the tabs with the arrow keys", async () => {
    renderApp(await withUsage(), "/?view=usage");
    tab("Token usage").focus();
    await userEvent.keyboard("{ArrowRight}");
    expect(selected()).toHaveTextContent("Context Size");
    expect(window.location.search).toBe("?view=usage&tab=context");
    await userEvent.keyboard("{ArrowLeft}{ArrowLeft}");
    expect(selected()).toHaveTextContent("Code Central calls");
  });

  it("goes back through the tabs on the browser's Back", async () => {
    renderApp(await withUsage(), "/?view=inbox");
    await userEvent.click(within(screen.getByRole("navigation", { name: "Filters" })).getByRole("link", { name: "Usage" }));
    await openTab("Code Central calls");
    await openTab("Context Size");

    act(() => window.history.back());
    await waitFor(() => expect(selected()).toHaveTextContent("Code Central calls"));
    act(() => window.history.back());
    await waitFor(() => expect(selected()).toHaveTextContent("Token usage"));
    act(() => window.history.back());
    expect(await screen.findByRole("navigation", { name: "Filters" })).toBeInTheDocument();
  });

  it("keeps the scope and range across a switch, without reading Usage again", async () => {
    const w = await withUsage();
    renderApp(w, "/?view=usage&project=acme&days=30");
    const reads = () => w.calls.filter((c) => c === "usage").length;
    await waitFor(() => expect(reads()).toBe(1));
    await openTab("Code Central calls");
    await waitFor(() => expect(ops()).toEqual(["get_ticket"]));
    await openTab("Context Size");
    await openTab("Token usage");

    expect(window.location.search).toBe("?view=usage&project=acme&days=30");
    expect(scope()).toHaveValue("acme");
    expect(range()).toHaveValue("30");
    expect(reads()).toBe(1);
  });

  it("shows the loading and error banners above the tab strip", async () => {
    const w = await withUsage();
    const down: Api = {
      ...w.api,
      usage: async () => {
        throw new Error("Usage is down");
      },
    };
    renderApp(w, "/?view=usage&tab=calls", down);
    const alert = await within(usage()).findByRole("alert");
    expect(alert).toHaveTextContent("Usage is down");
    expect(alert.compareDocumentPosition(within(usage()).getByRole("tablist")) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(selected()).toHaveTextContent("Code Central calls");
  });
});

describe("the Code Central calls table", () => {
  // The calls are real ones, so their sizes are the ones Core counts; the
  // figures below are read back from Usage, and the table must show them.
  async function withUsage() {
    const { w, s, ticket } = await projects();
    await calls(s, "acme", "get_ticket", 12, { ticket });
    // Errors: a Ticket that isn't there, and a Claim that isn't held.
    await calls(s, "acme", "claim", 3, { ticket: "acme-99" });
    await calls(s, "beta", "find_tickets", 2, { state: ["Ready", "Draft"] });
    await calls(s, "beta", "finish", 1, {
      claim_id: "no-such-claim",
      summary: "x".repeat(1_500),
      branch: "b",
      commit: "c",
    });
    const byOp = new Map((await w.api.usage()).operations.map((r) => [r.op, r]));
    return { w, s, byOp };
  }

  it("shows each operation's calls, sizes in readable units and errors, with the exact figure on hover", async () => {
    const { w, byOp } = await withUsage();
    renderApp(w, "/?view=usage");
    await openTab("Code Central calls");
    await waitFor(() => expect(ops()).toEqual(["get_ticket", "claim", "find_tickets", "finish"]));
    const rows = ops().map((op) => byOp.get(op)!);
    expect(
      within(table())
        .getAllByRole("columnheader")
        .map((h) => h.textContent),
    ).toEqual(["Operation", "Calls", "Request total", "Request average", "Response total", "Response average", "Errors"]);
    expect(column("Calls")).toEqual(["12", "3", "2", "1"]);
    expect(column("Request total")).toEqual(rows.map((r) => bytes(r.request_bytes)));
    expect(column("Request average")).toEqual(rows.map((r) => bytes(r.average_request_bytes)));
    expect(column("Response total")).toEqual(rows.map((r) => bytes(r.response_bytes)));
    expect(column("Response average")).toEqual(rows.map((r) => bytes(r.average_response_bytes)));
    expect(column("Errors")).toEqual(["0", "3", "0", "1"]);

    const row = within(table()).getByRole("row", { name: /get_ticket/ });
    expect(within(row).getByText(bytes(byOp.get("get_ticket")!.response_bytes))).toHaveAttribute(
      "title",
      `${count(byOp.get("get_ticket")!.response_bytes)} bytes`,
    );
    expect(within(within(table()).getByRole("row", { name: /get_ticket/ })).getByText("12")).toHaveAttribute("title", "12 calls");
  });

  it("sorts by every column, each way", async () => {
    const { w, byOp } = await withUsage();
    renderApp(w, "/?view=usage");
    await openTab("Code Central calls");
    await waitFor(() => expect(ops()).toHaveLength(4));
    const sortBy = (name: string) => userEvent.click(within(table()).getByRole("button", { name }));
    const sorted = (name: string) => within(table()).getByRole("columnheader", { name });
    // by is the operations ordered by a column's figure, largest first
    // when descending; the figures differ so no order is a tie.
    const by = (key: "request_bytes" | "average_request_bytes" | "response_bytes" | "average_response_bytes", descending: boolean) => {
      const figures = [...byOp.values()].map((r) => r[key]);
      expect(new Set(figures).size).toBe(figures.length);
      return [...byOp.values()].sort((a, b) => (descending ? b[key] - a[key] : a[key] - b[key])).map((r) => r.op);
    };

    expect(sorted("Calls")).toHaveAttribute("aria-sort", "descending");
    await sortBy("Calls");
    expect(ops()).toEqual(["finish", "find_tickets", "claim", "get_ticket"]);
    expect(sorted("Calls")).toHaveAttribute("aria-sort", "ascending");

    await sortBy("Operation");
    expect(ops()).toEqual(["claim", "find_tickets", "finish", "get_ticket"]);
    expect(sorted("Operation")).toHaveAttribute("aria-sort", "ascending");
    expect(sorted("Calls")).not.toHaveAttribute("aria-sort");
    await sortBy("Operation");
    expect(ops()).toEqual(["get_ticket", "finish", "find_tickets", "claim"]);

    await sortBy("Request total");
    expect(ops()).toEqual(by("request_bytes", true));
    await sortBy("Request average");
    expect(ops()).toEqual(by("average_request_bytes", true));
    await sortBy("Response total");
    expect(ops()).toEqual(by("response_bytes", true));
    await sortBy("Response average");
    expect(ops()).toEqual(by("average_response_bytes", true));
    await sortBy("Response average");
    expect(ops()).toEqual(by("average_response_bytes", false));
    await sortBy("Errors");
    expect(ops()[0]).toBe("claim");
  });

  it("says each tab has nothing to show for an empty range and scope", async () => {
    const { w, s } = await withUsage();
    await s.project("quiet", "Quiet");
    renderApp(w, "/?view=usage&project=quiet&days=30");
    expect(
      await within(usage()).findByText("No token usage in Quiet over the last 30 days. Only sessions the Factory runs are counted."),
    ).toBeInTheDocument();
    await openTab("Context Size");
    expect(within(usage()).getByText("No sessions in Quiet over the last 30 days. Only sessions the Factory runs are counted.")).toBeInTheDocument();
    await openTab("Code Central calls");
    expect(within(usage()).getByText("No Code Central calls in Quiet over the last 30 days.")).toBeInTheDocument();
    expect(within(usage()).queryByRole("table")).toBeNull();
    expect(within(usage()).queryByText(/No usage yet/)).toBeNull();
  });

  it("says so for All Projects too", async () => {
    renderApp((await projects()).w, "/?view=usage&tab=calls");
    expect(await within(usage()).findByText("No Code Central calls in All Projects over the last 7 days.")).toBeInTheDocument();
  });
});

// claimed has the Factory claim a new Ticket in project for session and
// post the session's model responses, returning the Ticket's ID.
async function claimed(s: Scenario, project: string, title: string, session: string, usage: UsageResponse[]): Promise<string> {
  const id = await s.ticket(project, { title });
  await s.factorySession(project, id, { session, usage });
  return id;
}

// response is one model response, minutes after the world's now, reading
// context tokens in all.
function response(w: World, minutes: number, kind: string, context: number, agent?: string): UsageResponse {
  const time = new Date(w.now().getTime() + minutes * 60_000).toISOString();
  return {
    time,
    ...(agent ? { agent } : {}),
    kind,
    input_tokens: 4,
    cache_write_tokens: 1_000,
    cache_read_tokens: context - 1_004,
    output_tokens: 50,
  };
}

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
    renderApp((await withUsage()).w, "/?view=usage");
    await openTab("Context Size");
    await waitFor(() => expect(within(usage()).getByRole("table", { name: "Context Size by kind" })).toBeInTheDocument());
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
    renderApp(w, "/?view=usage");
    await openTab("Context Size");
    await waitFor(() => expect(within(usage()).getByRole("table", { name: "Recent sessions" })).toBeInTheDocument());
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
    renderApp(w, "/?view=usage");
    await openTab("Context Size");
    await waitFor(() => expect(sessionsTable()).toBeInTheDocument());
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
    renderApp((await withUsage()).w, "/?view=usage");
    await openTab("Context Size");
    await waitFor(() => expect(sessionsTable()).toBeInTheDocument());
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
    renderApp(w, "/?view=usage");
    await openTab("Context Size");
    await waitFor(() => expect(sessionsTable()).toBeInTheDocument());
    expect(sessionRows().map(cells)).toEqual([["Acme", "1", "sess-impl", "—", "—", expect.any(String), "0", "0 (10,050 with subagents)", "0"]]);
  });

  it("opens a session's Ticket from its link", async () => {
    const { w, impl } = await withUsage();
    renderApp(w, "/?view=usage&days=30");
    await openTab("Context Size");
    await waitFor(() => expect(within(usage()).getByRole("table", { name: "Recent sessions" })).toBeInTheDocument());
    const link = within(sessionsTable()).getAllByRole("link", {
      name: impl,
    })[0];
    expect(link).toHaveAttribute("href", `?view=inbox&project=acme&ticket=${impl}`);

    await userEvent.click(link);
    expect(window.location.search).toBe(`?view=inbox&project=acme&ticket=${impl}`);
    expect(await screen.findByRole("heading", { name: "Build it" })).toBeInTheDocument();
  });

  it("follows the scope", async () => {
    const { w, spec } = await withUsage();
    renderApp(w, "/?view=usage&project=beta");
    await openTab("Context Size");
    await waitFor(() => expect(within(usage()).getByRole("table", { name: "Recent sessions" })).toBeInTheDocument());
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
    renderApp(w, "/?view=usage&tab=calls");
    await waitFor(() => expect(ops()).toEqual(["find_tickets"]));
    await openTab("Context Size");
    expect(within(usage()).getByText("No sessions in All Projects over the last 7 days. Only sessions the Factory runs are counted.")).toBeInTheDocument();
    expect(within(usage()).queryByRole("table", { name: "Recent sessions" })).toBeNull();
  });

  it("says there are no calls when there is token usage but no calls", async () => {
    renderApp((await withUsage()).w, "/?view=usage&tab=calls");
    expect(await within(usage()).findByText("No Code Central calls in All Projects over the last 7 days.")).toBeInTheDocument();
    expect(within(usage()).queryByRole("table", { name: "Code Central calls" })).toBeNull();
    await openTab("Context Size");
    expect(within(usage()).getByRole("table", { name: "Context Size by kind" })).toBeInTheDocument();
  });
});

describe("the Session popup", () => {
  async function withUsage() {
    const { w, s } = await projects();
    await claimed(s, "acme", "Build it", "sess-impl", [
      response(w, 1, "implementation", 40_000),
      response(w, 2, "implementation", 123_456),
      response(w, 3, "review", 21_000, "toolu_review"),
      response(w, 4, "implementation", 98_765),
    ]);
    return w;
  }

  const sessionsTable = () => within(usage()).getByRole("table", { name: "Recent sessions" });
  const popup = () => screen.getByRole("dialog", { name: /^Session / });
  const noPopup = () => expect(screen.queryByRole("dialog")).toBeNull();
  const chart = () => within(popup()).getByRole("img", { name: "Context Size per response" });
  const points = () => within(chart()).getAllByRole("button");
  const search = () => new URLSearchParams(window.location.search);

  it("opens a session from the recent sessions as a popup of its Context Size per response, over the table", async () => {
    renderApp(await withUsage(), "/?view=usage");
    await openTab("Context Size");
    await waitFor(() => expect(sessionsTable()).toBeInTheDocument());
    const link = within(sessionsTable()).getByRole("link", {
      name: "sess-impl",
    });
    expect(link).toHaveAttribute("href", "?view=usage&tab=context&session_project=acme&session=sess-impl");

    await userEvent.click(link);
    expect(search().get("session")).toBe("sess-impl");
    expect(search().get("session_project")).toBe("acme");
    expect(search().get("agent")).toBeNull();
    await waitFor(() => expect(points()).toHaveLength(3));
    expect(popup()).toHaveAccessibleName("Session sess-impl");
    expect(within(popup()).getByText("3 responses · peak 123,456 tokens")).toBeInTheDocument();
    expect(points().map((p) => p.getAttribute("aria-label"))).toEqual(["Response 1: 40,000 tokens", "Response 2: 123,456 tokens", "Response 3: 98,765 tokens"]);
    // The highest point is drawn highest, the lowest lowest.
    const ys = points().map((p) => Number(p.getAttribute("cy")));
    expect(ys[1]).toBeLessThan(ys[2]);
    expect(ys[2]).toBeLessThan(ys[0]);
    // Recent sessions stays where it was, under the popup.
    expect(sessionsTable()).toBeInTheDocument();
  });

  it("opens a Subagent's popup with its agent in the address, and its parent link switches the popup in place", async () => {
    renderApp(await withUsage(), "/?view=usage&days=30");
    await openTab("Context Size");
    await waitFor(() => expect(sessionsTable()).toBeInTheDocument());
    await userEvent.click(within(sessionsTable()).getByRole("button", { name: "1 subagent" }));
    const link = within(sessionsTable()).getByRole("link", { name: "toolu_review" });
    // The link keeps the range and tab too, for a new tab or a copied link.
    expect(link).toHaveAttribute("href", "?view=usage&days=30&tab=context&session_project=acme&session=sess-impl&agent=toolu_review");
    await userEvent.click(link);
    expect(window.location.search).toBe("?view=usage&days=30&tab=context&session_project=acme&session=sess-impl&agent=toolu_review");
    await waitFor(() => expect(points()).toHaveLength(1));
    expect(popup()).toHaveAccessibleName("Session sess-impl · toolu_review");
    expect(points()[0]).toHaveAttribute("aria-label", "Response 1: 21,000 tokens");
    // Its parent stays expanded, its row marked open.
    expect(within(sessionsTable()).getByRole("link", { name: "toolu_review" })).toHaveAttribute("aria-current", "true");

    // The popup links to the parent, whose popup is the same address without agent.
    const parent = within(popup()).getByRole("link", { name: "Parent session sess-impl" });
    expect(parent).toHaveAttribute("href", "?view=usage&days=30&tab=context&session_project=acme&session=sess-impl");
    await userEvent.click(parent);
    expect(window.location.search).toBe("?view=usage&days=30&tab=context&session_project=acme&session=sess-impl");
    await waitFor(() => expect(points()).toHaveLength(3));
    // The popup switched in place: still one, now the parent's.
    expect(screen.getAllByRole("dialog")).toHaveLength(1);
    expect(popup()).toHaveAccessibleName("Session sess-impl");
    expect(within(popup()).queryByRole("link", { name: /Parent session/ })).toBeNull();
  });

  it("names a Subagent's popup from the read and starts its parent expanded when loaded from the address", async () => {
    const w = await withUsage();
    await scenario(w).postUsage("sess-impl", [{ ...response(w, 5, "subagent", 10_000, "toolu_a"), agent_type: "Explore", description: "Map Usage sessions" }]);
    renderApp(w, "/?view=usage&session_project=acme&session=sess-impl&agent=toolu_a");
    await waitFor(() => expect(popup()).toHaveAccessibleName("Session sess-impl · Explore · Map Usage sessions"));
    await within(usage()).findByRole("table", { name: "Recent sessions" });
    const disclosure = within(sessionsTable()).getByRole("button", { name: "2 subagents" });
    expect(disclosure).toHaveAttribute("aria-expanded", "true");
    expect(within(sessionsTable()).getByRole("link", { name: "Explore · Map Usage sessions" })).toHaveAttribute("aria-current", "true");
  });

  it("opens a session from the address over the Context Size tab, whatever tab= says", async () => {
    renderApp(await withUsage(), "/?view=usage&tab=calls&session_project=acme&session=sess-impl");
    await waitFor(() => expect(points()).toHaveLength(3));
    expect(within(usage()).getByRole("tab", { selected: true })).toHaveTextContent("Context Size");
    expect(await within(usage()).findByRole("table", { name: "Recent sessions" })).toBeInTheDocument();
  });

  it("reopens the session from the address, whatever the scope", async () => {
    const w = await withUsage();
    renderApp(w, "/?view=usage&project=beta&session_project=acme&session=sess-impl");
    await waitFor(() => expect(points()).toHaveLength(3));
    expect(scope()).toHaveValue("beta");
    expect(w.calls.filter((c) => c === "sessionUsage")).toHaveLength(1);
  });

  it("opens even when the rest of Usage can't be read", async () => {
    const w = await withUsage();
    const down: Api = {
      ...w.api,
      usage: async () => {
        throw new Error("Usage is down");
      },
    };
    renderApp(w, "/?view=usage&session_project=acme&session=sess-impl", down);
    expect(await within(usage()).findByRole("alert")).toHaveTextContent("Usage is down");
    await waitFor(() => expect(points()).toHaveLength(3));
  });

  it("shows a point's exact figures on hover or focus, with thousands separators", async () => {
    renderApp(await withUsage(), "/?view=usage&session_project=acme&session=sess-impl");
    await waitFor(() => expect(points()).toHaveLength(3));
    expect(within(popup()).queryByRole("tooltip")).toBeNull();

    await userEvent.hover(points()[1]);
    const tip = within(popup()).getByRole("tooltip");
    expect(tip).toHaveTextContent("Response 2");
    expect(tip).toHaveTextContent("Context Size123,456 tokens");
    expect(tip).toHaveTextContent("Input4");
    expect(tip).toHaveTextContent("Cache write1,000");
    expect(tip).toHaveTextContent("Cache read122,452");
    expect(tip).toHaveTextContent("Output50");

    await userEvent.unhover(points()[1]);
    expect(within(popup()).queryByRole("tooltip")).toBeNull();

    // And on focus, from the keyboard.
    act(() => points()[2].focus());
    expect(within(popup()).getByRole("tooltip")).toHaveTextContent("Response 3");
  });

  it.each([
    ["×", () => userEvent.click(within(popup()).getByRole("button", { name: "Close" }))],
    ["Escape", () => userEvent.keyboard("{Escape}")],
    ["a click outside", () => userEvent.click(popup().parentElement!)],
  ])("closes on %s to Usage as it was: the Context Size tab, scope, range and expanded Subagents", async (_, close) => {
    renderApp(await withUsage(), "/?view=usage&project=acme&days=30&tab=context");
    await waitFor(() => expect(sessionsTable()).toBeInTheDocument());
    const disclosure = within(sessionsTable()).getByRole("button", { name: "1 subagent" });
    await userEvent.click(disclosure);
    await userEvent.click(within(sessionsTable()).getByRole("link", { name: "toolu_review" }));
    await waitFor(() => expect(points()).toHaveLength(1));

    await close();
    noPopup();
    expect(window.location.search).toBe("?view=usage&project=acme&days=30&tab=context");
    expect(within(usage()).getByRole("tab", { selected: true })).toHaveTextContent("Context Size");
    expect(disclosure).toHaveAttribute("aria-expanded", "true");
    expect(within(sessionsTable()).getByRole("link", { name: "toolu_review" })).not.toHaveAttribute("aria-current");
    // Focus goes back to the row it was opened from.
    expect(within(sessionsTable()).getByRole("link", { name: "toolu_review" })).toHaveFocus();
    // Collapsing is the table's own state: the address gains nothing.
    await userEvent.click(disclosure);
    expect(disclosure).toHaveAttribute("aria-expanded", "false");
    expect(window.location.search).toBe("?view=usage&project=acme&days=30&tab=context");
  });

  it("closes to the Context Size tab it showed over when the address named no tab", async () => {
    renderApp(await withUsage(), "/?view=usage&session_project=acme&session=sess-impl");
    await waitFor(() => expect(points()).toHaveLength(3));
    await userEvent.keyboard("{Escape}");
    noPopup();
    expect(window.location.search).toBe("?view=usage&tab=context");
    expect(within(usage()).getByRole("tab", { selected: true })).toHaveTextContent("Context Size");
  });

  it("closes on the browser's Back, and leaves Usage on the next", async () => {
    renderApp(await withUsage(), "/?view=inbox&project=acme");
    await userEvent.click(within(screen.getByRole("navigation", { name: "Filters" })).getByRole("link", { name: "Usage" }));
    await openTab("Context Size");
    await waitFor(() => expect(sessionsTable()).toBeInTheDocument());
    await userEvent.click(within(sessionsTable()).getByRole("link", { name: "sess-impl" }));
    await waitFor(() => expect(points()).toHaveLength(3));

    act(() => window.history.back());
    await waitFor(noPopup);
    expect(search().get("session")).toBeNull();
    expect(within(usage()).getByRole("tab", { selected: true })).toHaveTextContent("Context Size");

    act(() => window.history.back());
    await waitFor(() => expect(within(usage()).getByRole("tab", { selected: true })).toHaveTextContent("Token usage"));
    act(() => window.history.back());
    expect(await screen.findByRole("navigation", { name: "Filters" })).toBeInTheDocument();
  });

  it("returns to where Usage was opened from once the popup is closed", async () => {
    renderApp(await withUsage(), "/?view=usage&session_project=acme&session=sess-impl&back=view%3Dfocus%26project%3Dacme");
    await waitFor(() => expect(points()).toHaveLength(3));
    await userEvent.keyboard("{Escape}");
    await userEvent.click(within(usage()).getByRole("button", { name: "Back to Focus Mode" }));
    expect(window.location.search).toBe("?view=focus&project=acme");
  });

  it("says when a session has no responses in the days Usage keeps", async () => {
    renderApp(await withUsage(), "/?view=usage&session_project=acme&session=sess-gone");
    await waitFor(() => expect(within(popup()).getByText("No responses for this session in the 90 days Usage keeps.")).toBeInTheDocument());
    expect(within(popup()).queryByRole("img")).toBeNull();
  });
});

describe("Token usage", () => {
  // tokens is one model response, minutes after the world's now, with the
  // four token counts given.
  function tokens(w: World, minutes: number, kind: string, t: [number, number, number, number], agent?: string): UsageResponse {
    const time = new Date(w.now().getTime() + minutes * 60_000).toISOString();
    return {
      time,
      ...(agent ? { agent } : {}),
      kind,
      input_tokens: t[0],
      cache_write_tokens: t[1],
      cache_read_tokens: t[2],
      output_tokens: t[3],
    };
  }
  async function withUsage() {
    const { w, s } = await projects();
    await claimed(s, "acme", "Build it", "sess-impl", [
      tokens(w, 1, "implementation", [10, 2_000, 150_000, 1_200]),
      tokens(w, 2, "review", [3, 1_000, 20_000, 400], "toolu_review"),
      // Two days ago.
      tokens(w, -2 * 24 * 60, "implementation", [5, 500, 9_000, 300]),
    ]);
    await claimed(s, "beta", "Write it", "sess-spec", [tokens(w, 3, "spec", [7, 3_000, 70_000, 900])]);
    return w;
  }

  const chart = () => within(usage()).getByRole("group", { name: "Token usage per day" });
  const bars = () => within(chart()).getAllByRole("img");
  const kind = () => within(usage()).getByRole("combobox", { name: "Kind" });
  const figures = () => within(usage()).getByRole("status", { name: "Day's figures" });

  it("draws one bar per day in the range, oldest first, stacked by token type", async () => {
    renderApp(await withUsage(), "/?view=usage");
    await waitFor(() => expect(bars()).toHaveLength(7));
    expect(bars()[6]).toHaveAccessibleName(/: 248,520 tokens$/);
    expect(bars()[4]).toHaveAccessibleName(/: 9,805 tokens$/);
    expect(bars()[5]).toHaveAccessibleName(/: 0 tokens$/);
    expect(
      within(usage())
        .getAllByText(/^(Input|Cache write|Cache read|Output)$/)
        .map((l) => l.textContent),
    ).toEqual(["Input", "Cache write", "Cache read", "Output"]);

    await userEvent.selectOptions(range(), "Last 30 days");
    await waitFor(() => expect(bars()).toHaveLength(30));
  });

  it("shows a bar's exact figures with thousands separators on hover, and never a dollar figure", async () => {
    renderApp(await withUsage(), "/?view=usage");
    await waitFor(() => expect(bars()).toHaveLength(7));
    await userEvent.hover(bars()[6]);
    const shown = within(figures());
    expect(shown.getByText("Input").nextSibling).toHaveTextContent("20");
    expect(shown.getByText("Cache write").nextSibling).toHaveTextContent("6,000");
    expect(shown.getByText("Cache read").nextSibling).toHaveTextContent("240,000");
    expect(shown.getByText("Output").nextSibling).toHaveTextContent("2,500");
    expect(shown.getByText("Total").nextSibling).toHaveTextContent("248,520");
    expect(usage().textContent).not.toMatch(/\$/);
  });

  it("filters the bars by kind without reading Usage again", async () => {
    const w = await withUsage();
    renderApp(w, "/?view=usage");
    await waitFor(() => expect(bars()).toHaveLength(7));
    const reads = w.calls.filter((c) => c === "usage").length;
    expect(
      within(kind())
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual(["All kinds", "spec", "implementation", "review"]);

    await userEvent.selectOptions(kind(), "review");
    expect(bars()[6]).toHaveAccessibleName(/: 21,403 tokens$/);
    expect(bars()[4]).toHaveAccessibleName(/: 0 tokens$/);
    await userEvent.selectOptions(kind(), "implementation");
    expect(bars()[6]).toHaveAccessibleName(/: 153,210 tokens$/);
    expect(bars()[4]).toHaveAccessibleName(/: 9,805 tokens$/);
    await userEvent.selectOptions(kind(), "All kinds");
    expect(bars()[6]).toHaveAccessibleName(/: 248,520 tokens$/);
    expect(w.calls.filter((c) => c === "usage").length).toBe(reads);
  });

  it("goes back to all kinds when the chosen kind isn't in the new scope", async () => {
    renderApp(await withUsage(), "/?view=usage");
    await waitFor(() => expect(bars()).toHaveLength(7));
    await userEvent.selectOptions(kind(), "review");
    await waitFor(() => expect(within(scope()).getAllByRole("option")).toHaveLength(4));
    await userEvent.selectOptions(scope(), "Beta");

    await waitFor(() =>
      expect(
        within(kind())
          .getAllByRole("option")
          .map((o) => o.textContent),
      ).toEqual(["All kinds", "spec"]),
    );
    expect(kind()).toHaveValue("");
    expect(bars()[6]).toHaveAccessibleName(/: 73,907 tokens$/);
  });

  it("says there is no token usage when there are calls but no sessions", async () => {
    const { w, s } = await projects();
    await calls(s, "acme", "find_tickets", 1, {});
    renderApp(w, "/?view=usage");
    expect(
      await within(usage()).findByText("No token usage in All Projects over the last 7 days. Only sessions the Factory runs are counted."),
    ).toBeInTheDocument();
    expect(within(usage()).queryByRole("group", { name: "Token usage per day" })).toBeNull();
  });
});

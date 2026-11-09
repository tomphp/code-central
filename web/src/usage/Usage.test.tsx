// Usage view tests: the real App against FakeApi, at ?view=usage. They
// check what the human sees and does: opening it from the Inbox's and
// Focus Mode's headers and going Back, the address keeping the scope and
// range, the scope and range pickers, the Code Central calls table and
// its sorting, the Context Size table and the recent sessions with their
// Ticket links, the empty states, and loading only when opened or changed.
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";
import type { UsageResponse } from "../api/types";

function world() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addProject("beta", "Beta");
  fake.addProject("old", "Old", { archived: true });
  fake.addTicket("acme", { title: "Pick a colour", state: "Ready" }); // acme-1
  return fake;
}

// calls records n MCP calls of op to project, each with the sizes given.
function calls(fake: FakeApi, project: string, op: string, n: number, sizes: { request: number; response: number; error?: boolean }) {
  for (let i = 0; i < n; i++) {
    fake.recordCall(project, {
      op,
      request_bytes: sizes.request,
      response_bytes: sizes.response,
      error: sizes.error ? "not_found" : undefined,
    });
  }
}

function renderApp(fake: FakeApi, url = "/") {
  window.history.replaceState(null, "", url);
  return render(<App api={fake} now={() => fake.now} />);
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

describe("opening and leaving Usage", () => {
  it("opens from the Inbox's header on All Projects and 7 days, and returns to the Inbox with its filter and Ticket", async () => {
    renderApp(world(), "/?view=inbox&project=acme&state=Ready&ticket=acme-1");
    const nav = screen.getByRole("navigation", { name: "Filters" });
    await userEvent.click(within(nav).getByRole("link", { name: "Usage" }));

    expect(usage()).toBeInTheDocument();
    expect(scope()).toHaveValue("");
    expect(within(scope()).getByRole("option", { selected: true })).toHaveTextContent("All Projects");
    expect(range()).toHaveValue("7");
    expect(new URLSearchParams(window.location.search).get("project")).toBeNull();

    await userEvent.click(within(usage()).getByRole("button", { name: "Back to the Inbox" }));
    expect(window.location.search).toBe("?view=inbox&project=acme&state=Ready&ticket=acme-1");
    expect(await screen.findByRole("navigation", { name: "Filters" })).toBeInTheDocument();
  });

  it("opens from Focus Mode's header and returns to Focus Mode with its Project", async () => {
    renderApp(world(), "/?view=focus&project=acme");
    await userEvent.click(within(screen.getByRole("main", { name: "Focus Mode" })).getByRole("link", { name: "Usage" }));

    expect(usage()).toBeInTheDocument();
    expect(scope()).toHaveValue("");
    await userEvent.click(within(usage()).getByRole("button", { name: "Back to Focus Mode" }));
    expect(window.location.search).toBe("?view=focus&project=acme");
    expect(screen.getByRole("main", { name: "Focus Mode" })).toBeInTheDocument();
  });

  it("returns to the Inbox when opened straight from the address", async () => {
    renderApp(world(), "/?view=usage");
    await userEvent.click(within(usage()).getByRole("button", { name: "Back to the Inbox" }));
    expect(window.location.search).toBe("?view=inbox");
  });

  it("returns to where the human was on the browser's Back", async () => {
    renderApp(world(), "/?view=focus&project=acme");
    await userEvent.click(within(screen.getByRole("main", { name: "Focus Mode" })).getByRole("link", { name: "Usage" }));
    expect(usage()).toBeInTheDocument();

    act(() => window.history.back());
    expect(await screen.findByRole("main", { name: "Focus Mode" })).toBeInTheDocument();
    expect(window.location.search).toBe("?view=focus&project=acme");
  });
});

describe("scope and range", () => {
  it("lists All Projects and every Project, Archived ones marked", async () => {
    renderApp(world(), "/?view=usage");
    await waitFor(() => expect(within(scope()).getAllByRole("option")).toHaveLength(4));
    expect(within(scope()).getAllByRole("option").map((o) => o.textContent)).toEqual(["All Projects", "Acme", "Beta", "Old (Archived)"]);
    expect(within(range()).getAllByRole("option").map((o) => o.textContent)).toEqual(["Last 7 days", "Last 30 days", "Last 90 days"]);
  });

  it("puts a chosen Project and range in the address, and a reload keeps them", async () => {
    const fake = world();
    calls(fake, "acme", "claim", 1, { request: 10, response: 100 });
    calls(fake, "beta", "find_tickets", 1, { request: 10, response: 100 });
    // Twenty days ago: only in the 30 and 90 day ranges.
    fake.recordCall("acme", { op: "finish", request_bytes: 10, response_bytes: 100, time: "2026-10-18T09:00:00Z" });
    const { unmount } = renderApp(fake, "/?view=focus");
    await userEvent.click(within(screen.getByRole("main", { name: "Focus Mode" })).getByRole("link", { name: "Usage" }));
    await waitFor(() => expect(ops()).toEqual(["claim", "find_tickets"]));

    await waitFor(() => expect(within(scope()).getAllByRole("option")).toHaveLength(4));
    await userEvent.selectOptions(scope(), "Acme");
    expect(window.location.search).toBe("?view=usage&project=acme&back=view%3Dfocus");
    await waitFor(() => expect(ops()).toEqual(["claim"]));

    await userEvent.selectOptions(range(), "Last 30 days");
    expect(window.location.search).toBe("?view=usage&project=acme&days=30&back=view%3Dfocus");
    await waitFor(() => expect(ops()).toEqual(["claim", "finish"]));

    unmount();
    renderApp(fake, window.location.search);
    expect(scope()).toHaveValue("acme");
    expect(range()).toHaveValue("30");
    await waitFor(() => expect(ops()).toEqual(["claim", "finish"]));
    await userEvent.click(within(usage()).getByRole("button", { name: "Back to Focus Mode" }));
    expect(window.location.search).toBe("?view=focus");
  });

  it("goes back to All Projects and 7 days, leaving them out of the address", async () => {
    renderApp(world(), "/?view=usage&project=acme&days=90");
    await waitFor(() => expect(scope()).toHaveValue("acme"));
    await userEvent.selectOptions(scope(), "All Projects");
    await userEvent.selectOptions(range(), "Last 7 days");
    expect(window.location.search).toBe("?view=usage");
  });

  it("loads when opened and when the scope or range changes, never on a live notice", async () => {
    const fake = world();
    renderApp(fake, "/?view=usage");
    const reads = () => fake.calls.filter((c) => c === "usage").length;
    await waitFor(() => expect(reads()).toBe(1));

    calls(fake, "acme", "claim", 1, { request: 10, response: 100 });
    act(() => fake.addTicket("acme", { title: "Another" }));
    act(() => fake.reconnect());
    await act(() => new Promise((r) => setTimeout(r, 20)));
    expect(reads()).toBe(1);
    expect(within(usage()).getByText(/No usage yet/)).toBeInTheDocument();

    await userEvent.selectOptions(range(), "Last 30 days");
    await waitFor(() => expect(reads()).toBe(2));
    await waitFor(() => expect(ops()).toEqual(["claim"]));
  });
});

describe("the Code Central calls table", () => {
  function seeded() {
    const fake = world();
    calls(fake, "acme", "get_ticket", 1200, { request: 20, response: 3_000 });
    calls(fake, "acme", "claim", 3, { request: 100, response: 2_000_000 });
    calls(fake, "beta", "find_tickets", 2, { request: 512, response: 700 });
    calls(fake, "beta", "finish", 1, { request: 1_500, response: 80, error: true });
    return fake;
  }

  it("shows each operation's calls, sizes in readable units and errors, with the exact figure on hover", async () => {
    renderApp(seeded(), "/?view=usage");
    await waitFor(() => expect(ops()).toEqual(["get_ticket", "claim", "find_tickets", "finish"]));
    expect(within(table()).getAllByRole("columnheader").map((h) => h.textContent)).toEqual([
      "Operation",
      "Calls",
      "Request total",
      "Request average",
      "Response total",
      "Response average",
      "Errors",
    ]);
    expect(column("Calls")).toEqual(["1,200", "3", "2", "1"]);
    expect(column("Request total")).toEqual(["23.4 KB", "300 B", "1 KB", "1.5 KB"]);
    expect(column("Request average")).toEqual(["20 B", "100 B", "512 B", "1.5 KB"]);
    expect(column("Response total")).toEqual(["3.4 MB", "5.7 MB", "1.4 KB", "80 B"]);
    expect(column("Errors")).toEqual(["0", "0", "0", "1"]);

    const row = within(table()).getByRole("row", { name: /claim/ });
    expect(within(row).getByText("5.7 MB")).toHaveAttribute("title", "6,000,000 bytes");
    expect(within(within(table()).getByRole("row", { name: /get_ticket/ })).getByText("1,200")).toHaveAttribute("title", "1,200 calls");
  });

  it("sorts by every column, each way", async () => {
    renderApp(seeded(), "/?view=usage");
    await waitFor(() => expect(ops()).toHaveLength(4));
    const sortBy = (name: string) => userEvent.click(within(table()).getByRole("button", { name }));
    const sorted = (name: string) => within(table()).getByRole("columnheader", { name });

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
    expect(ops()).toEqual(["get_ticket", "finish", "find_tickets", "claim"]);
    await sortBy("Request average");
    expect(ops()).toEqual(["finish", "find_tickets", "claim", "get_ticket"]);
    await sortBy("Response total");
    expect(ops()).toEqual(["claim", "get_ticket", "find_tickets", "finish"]);
    await sortBy("Response average");
    expect(ops()).toEqual(["claim", "get_ticket", "find_tickets", "finish"]);
    await sortBy("Errors");
    expect(ops()[0]).toBe("finish");
  });

  it("says there is no usage yet for an empty range and scope", async () => {
    const fake = seeded();
    fake.addProject("quiet", "Quiet");
    renderApp(fake, "/?view=usage&project=quiet&days=30");
    expect(await within(usage()).findByText("No usage yet in Quiet over the last 30 days.")).toBeInTheDocument();
    expect(within(usage()).queryByRole("table")).toBeNull();
  });

  it("says so for All Projects too", async () => {
    renderApp(world(), "/?view=usage");
    expect(await within(usage()).findByText("No usage yet in All Projects over the last 7 days.")).toBeInTheDocument();
  });
});

// claimed claims a new Ticket in project for session, returning its ID.
function claimed(fake: FakeApi, project: string, title: string, session: string): string {
  const at = fake.now.toISOString();
  return fake.addTicket(project, { title, state: "In Progress", claims: [{ number: 1, session_id: session, started: at, last_activity: at }] }).id;
}

// response is one model response, minutes after the fake's now, reading
// context tokens in all.
function response(fake: FakeApi, minutes: number, kind: string, context: number, agent?: string): UsageResponse {
  const time = new Date(fake.now.getTime() + minutes * 60_000).toISOString();
  return { time, ...(agent ? { agent } : {}), kind, input_tokens: 4, cache_write_tokens: 1_000, cache_read_tokens: context - 1_004, output_tokens: 50 };
}

describe("Context Size", () => {
  function seeded() {
    const fake = world();
    const impl = claimed(fake, "acme", "Build it", "sess-impl"); // acme-2
    const spec = claimed(fake, "beta", "Write it", "sess-spec"); // beta-1
    fake.postUsage("sess-impl", [
      response(fake, 1, "implementation", 40_000),
      response(fake, 2, "implementation", 123_456),
      response(fake, 3, "review", 21_000, "toolu_review"),
    ]);
    fake.postUsage("sess-spec", [response(fake, 4, "spec", 80_000)]);
    return { fake, impl, spec };
  }

  const kindsTable = () => within(usage()).getByRole("table", { name: "Context Size by kind" });
  const sessionsTable = () => within(usage()).getByRole("table", { name: "Recent sessions" });
  const rows = (table: HTMLElement) =>
    within(table)
      .getAllByRole("row")
      .slice(1)
      .map((r) => within(r).getAllByRole("cell").map((c) => c.textContent));

  it("shows each kind's sessions, average and largest peak, with thousands separators and the exact figure on hover", async () => {
    renderApp(seeded().fake, "/?view=usage");
    await waitFor(() => expect(within(usage()).getByRole("table", { name: "Context Size by kind" })).toBeInTheDocument());
    expect(within(kindsTable()).getAllByRole("columnheader").map((h) => h.textContent)).toEqual(["Kind", "Sessions", "Average peak", "Largest peak"]);
    expect(within(kindsTable()).getAllByRole("rowheader").map((h) => h.textContent)).toEqual(["spec", "implementation", "review"]);
    expect(rows(kindsTable())).toEqual([
      ["1", "80,000", "80,000"],
      ["1", "123,456", "123,456"],
      ["1", "21,000", "21,000"],
    ]);
    expect(within(kindsTable()).getAllByText("123,456")[0]).toHaveAttribute("title", "123,456 tokens");
  });

  it("lists the recent sessions newest first, each subagent a session of its own", async () => {
    const { fake, impl, spec } = seeded();
    renderApp(fake, "/?view=usage");
    await waitFor(() => expect(within(usage()).getByRole("table", { name: "Recent sessions" })).toBeInTheDocument());
    expect(within(sessionsTable()).getAllByRole("columnheader").map((h) => h.textContent)).toEqual([
      "Ticket",
      "Project",
      "Claim",
      "Session",
      "Kind",
      "Started",
      "Peak Context Size",
      "Responses",
    ]);
    const sessions = within(sessionsTable()).getAllByRole("row").slice(1);
    expect(sessions.map((r) => within(within(r).getByRole("rowheader")).getByRole("link").textContent)).toEqual([spec, impl, impl]);
    expect(sessions.map((r) => within(r).getAllByRole("cell").map((c) => c.textContent))).toEqual([
      ["Beta", "1", "sess-spec", "spec", expect.any(String), "80,000", "1"],
      ["Acme", "1", "sess-impl · toolu_review", "review", expect.any(String), "21,000", "1"],
      ["Acme", "1", "sess-impl", "implementation", expect.any(String), "123,456", "2"],
    ]);
    expect(within(sessions[2]).getByText("123,456")).toHaveAttribute("title", "123,456 tokens");
  });

  it("opens a session's Ticket from its link", async () => {
    const { fake, impl } = seeded();
    renderApp(fake, "/?view=usage&days=30");
    await waitFor(() => expect(within(usage()).getByRole("table", { name: "Recent sessions" })).toBeInTheDocument());
    const link = within(sessionsTable()).getAllByRole("link", { name: impl })[0];
    expect(link).toHaveAttribute("href", `?view=inbox&project=acme&ticket=${impl}`);

    await userEvent.click(link);
    expect(window.location.search).toBe(`?view=inbox&project=acme&ticket=${impl}`);
    expect(await screen.findByRole("heading", { name: "Build it" })).toBeInTheDocument();
  });

  it("follows the scope", async () => {
    const { fake, spec } = seeded();
    renderApp(fake, "/?view=usage&project=beta");
    await waitFor(() => expect(within(usage()).getByRole("table", { name: "Recent sessions" })).toBeInTheDocument());
    expect(within(sessionsTable()).getAllByRole("rowheader").map((h) => h.textContent)).toEqual([spec]);
    expect(within(kindsTable()).getAllByRole("rowheader").map((h) => h.textContent)).toEqual(["spec"]);
  });

  it("says there are no sessions when there are calls but no token usage", async () => {
    const fake = world();
    calls(fake, "acme", "claim", 1, { request: 10, response: 100 });
    renderApp(fake, "/?view=usage");
    await waitFor(() => expect(ops()).toEqual(["claim"]));
    expect(within(usage()).getByText("No sessions in All Projects over the last 7 days. Only sessions the Factory runs are counted.")).toBeInTheDocument();
    expect(within(usage()).queryByRole("table", { name: "Recent sessions" })).toBeNull();
    expect(within(usage()).queryByText(/No usage yet/)).toBeNull();
  });

  it("says there are no calls when there is token usage but no calls", async () => {
    renderApp(seeded().fake, "/?view=usage");
    expect(await within(usage()).findByText("No Code Central calls in All Projects over the last 7 days.")).toBeInTheDocument();
    expect(within(usage()).queryByRole("table", { name: "Code Central calls" })).toBeNull();
    expect(within(usage()).getByRole("table", { name: "Context Size by kind" })).toBeInTheDocument();
    expect(within(usage()).queryByText(/No usage yet/)).toBeNull();
  });
});

describe("a session's Context Size", () => {
  function seeded() {
    const fake = world();
    claimed(fake, "acme", "Build it", "sess-impl");
    fake.postUsage("sess-impl", [
      response(fake, 1, "implementation", 40_000),
      response(fake, 2, "implementation", 123_456),
      response(fake, 3, "review", 21_000, "toolu_review"),
      response(fake, 4, "implementation", 98_765),
    ]);
    return fake;
  }

  const sessionsTable = () => within(usage()).getByRole("table", { name: "Recent sessions" });
  const panel = () => within(usage()).getByRole("region", { name: /^Session / });
  const chart = () => within(panel()).getByRole("img", { name: "Context Size per response" });
  const points = () => within(chart()).getAllByRole("button");
  const search = () => new URLSearchParams(window.location.search);

  it("opens a session from the recent sessions as a line chart of its Context Size per response", async () => {
    renderApp(seeded(), "/?view=usage");
    await waitFor(() => expect(sessionsTable()).toBeInTheDocument());
    const link = within(sessionsTable()).getByRole("link", { name: "sess-impl" });
    expect(link).toHaveAttribute("href", "?view=usage&session_project=acme&session=sess-impl");

    await userEvent.click(link);
    expect(search().get("session")).toBe("sess-impl");
    expect(search().get("session_project")).toBe("acme");
    expect(search().get("agent")).toBeNull();
    await waitFor(() => expect(points()).toHaveLength(3));
    expect(panel()).toHaveAccessibleName("Session sess-impl");
    expect(within(panel()).getByText("3 responses · peak 123,456 tokens")).toBeInTheDocument();
    expect(points().map((p) => p.getAttribute("aria-label"))).toEqual([
      "Response 1: 40,000 tokens",
      "Response 2: 123,456 tokens",
      "Response 3: 98,765 tokens",
    ]);
    // The highest point is drawn highest, the lowest lowest.
    const ys = points().map((p) => Number(p.getAttribute("cy")));
    expect(ys[1]).toBeLessThan(ys[2]);
    expect(ys[2]).toBeLessThan(ys[0]);
  });

  it("opens a subagent's session with its agent in the address", async () => {
    renderApp(seeded(), "/?view=usage&days=30");
    await waitFor(() => expect(sessionsTable()).toBeInTheDocument());
    const link = within(sessionsTable()).getByRole("link", { name: "sess-impl · toolu_review" });
    // The link keeps the range too, for a new tab or a copied link.
    expect(link).toHaveAttribute("href", "?view=usage&days=30&session_project=acme&session=sess-impl&agent=toolu_review");
    await userEvent.click(link);
    expect(window.location.search).toBe("?view=usage&days=30&session_project=acme&session=sess-impl&agent=toolu_review");
    await waitFor(() => expect(points()).toHaveLength(1));
    expect(panel()).toHaveAccessibleName("Session sess-impl · toolu_review");
    expect(points()[0]).toHaveAttribute("aria-label", "Response 1: 21,000 tokens");
  });

  it("reopens the session from the address, whatever the scope", async () => {
    const fake = seeded();
    renderApp(fake, "/?view=usage&project=beta&session_project=acme&session=sess-impl");
    await waitFor(() => expect(points()).toHaveLength(3));
    expect(scope()).toHaveValue("beta");
    expect(fake.calls.filter((c) => c === "sessionUsage")).toHaveLength(1);
  });

  it("shows the session even when the rest of Usage can't be read", async () => {
    const fake = seeded();
    fake.usage = async () => {
      throw new Error("Usage is down");
    };
    renderApp(fake, "/?view=usage&session_project=acme&session=sess-impl");
    expect(await within(usage()).findByRole("alert")).toHaveTextContent("Usage is down");
    await waitFor(() => expect(points()).toHaveLength(3));
  });

  it("shows a point's exact figures on hover, with thousands separators", async () => {
    renderApp(seeded(), "/?view=usage&session_project=acme&session=sess-impl");
    await waitFor(() => expect(points()).toHaveLength(3));
    expect(within(panel()).queryByRole("tooltip")).toBeNull();

    await userEvent.hover(points()[1]);
    const tip = within(panel()).getByRole("tooltip");
    expect(tip).toHaveTextContent("Response 2");
    expect(tip).toHaveTextContent("Context Size123,456 tokens");
    expect(tip).toHaveTextContent("Input4");
    expect(tip).toHaveTextContent("Cache write1,000");
    expect(tip).toHaveTextContent("Cache read122,452");
    expect(tip).toHaveTextContent("Output50");

    await userEvent.unhover(points()[1]);
    expect(within(panel()).queryByRole("tooltip")).toBeNull();
  });

  it("closes the session, leaving the scope and range in the address", async () => {
    renderApp(seeded(), "/?view=usage&project=acme&days=30&session_project=acme&session=sess-impl");
    await waitFor(() => expect(points()).toHaveLength(3));
    await userEvent.click(within(panel()).getByRole("button", { name: "Close session" }));
    expect(window.location.search).toBe("?view=usage&project=acme&days=30");
    expect(within(usage()).queryByRole("region", { name: /^Session / })).toBeNull();
  });

  it("closes the session on the browser's Back, and leaves Usage on the next", async () => {
    renderApp(seeded(), "/?view=focus&project=acme");
    await userEvent.click(within(screen.getByRole("main", { name: "Focus Mode" })).getByRole("link", { name: "Usage" }));
    await waitFor(() => expect(sessionsTable()).toBeInTheDocument());
    await userEvent.click(within(sessionsTable()).getByRole("link", { name: "sess-impl" }));
    await waitFor(() => expect(points()).toHaveLength(3));

    act(() => window.history.back());
    await waitFor(() => expect(within(usage()).queryByRole("region", { name: /^Session / })).toBeNull());
    expect(search().get("session")).toBeNull();

    act(() => window.history.back());
    expect(await screen.findByRole("main", { name: "Focus Mode" })).toBeInTheDocument();
  });

  it("returns to where Usage was opened from with a session open", async () => {
    renderApp(seeded(), "/?view=usage&session_project=acme&session=sess-impl&back=view%3Dfocus%26project%3Dacme");
    await waitFor(() => expect(points()).toHaveLength(3));
    await userEvent.click(within(usage()).getByRole("button", { name: "Back to Focus Mode" }));
    expect(window.location.search).toBe("?view=focus&project=acme");
  });

  it("says when a session has no responses", async () => {
    renderApp(seeded(), "/?view=usage&session_project=acme&session=sess-gone");
    expect(await within(usage()).findByText("No responses for this session in the last 90 days.")).toBeInTheDocument();
    expect(within(panel()).queryByRole("img")).toBeNull();
  });
});

describe("Token usage", () => {
  // tokens is one model response, minutes after the fake's now, with the
  // four token counts given.
  function tokens(fake: FakeApi, minutes: number, kind: string, t: [number, number, number, number], agent?: string): UsageResponse {
    const time = new Date(fake.now.getTime() + minutes * 60_000).toISOString();
    return { time, ...(agent ? { agent } : {}), kind, input_tokens: t[0], cache_write_tokens: t[1], cache_read_tokens: t[2], output_tokens: t[3] };
  }
  function seeded() {
    const fake = world();
    const at = fake.now.toISOString();
    fake.addTicket("acme", { title: "Build it", state: "In Progress", claims: [{ number: 1, session_id: "sess-impl", started: at, last_activity: at }] });
    fake.addTicket("beta", { title: "Write it", state: "In Progress", claims: [{ number: 1, session_id: "sess-spec", started: at, last_activity: at }] });
    fake.postUsage("sess-impl", [
      tokens(fake, 1, "implementation", [10, 2_000, 150_000, 1_200]),
      tokens(fake, 2, "review", [3, 1_000, 20_000, 400], "toolu_review"),
      // Two days ago.
      tokens(fake, -2 * 24 * 60, "implementation", [5, 500, 9_000, 300]),
    ]);
    fake.postUsage("sess-spec", [tokens(fake, 3, "spec", [7, 3_000, 70_000, 900])]);
    return fake;
  }

  const chart = () => within(usage()).getByRole("group", { name: "Token usage per day" });
  const bars = () => within(chart()).getAllByRole("img");
  const kind = () => within(usage()).getByRole("combobox", { name: "Kind" });
  const figures = () => within(usage()).getByRole("status", { name: "Day's figures" });

  it("draws one bar per day in the range, oldest first, stacked by token type", async () => {
    renderApp(seeded(), "/?view=usage");
    await waitFor(() => expect(bars()).toHaveLength(7));
    expect(bars()[6]).toHaveAccessibleName(/: 248,520 tokens$/);
    expect(bars()[4]).toHaveAccessibleName(/: 9,805 tokens$/);
    expect(bars()[5]).toHaveAccessibleName(/: 0 tokens$/);
    expect(within(usage()).getAllByText(/^(Input|Cache write|Cache read|Output)$/).map((l) => l.textContent)).toEqual(["Input", "Cache write", "Cache read", "Output"]);

    await userEvent.selectOptions(range(), "Last 30 days");
    await waitFor(() => expect(bars()).toHaveLength(30));
  });

  it("shows a bar's exact figures with thousands separators on hover, and never a dollar figure", async () => {
    renderApp(seeded(), "/?view=usage");
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
    const fake = seeded();
    renderApp(fake, "/?view=usage");
    await waitFor(() => expect(bars()).toHaveLength(7));
    const reads = fake.calls.filter((c) => c === "usage").length;
    expect(within(kind()).getAllByRole("option").map((o) => o.textContent)).toEqual(["All kinds", "spec", "implementation", "review"]);

    await userEvent.selectOptions(kind(), "review");
    expect(bars()[6]).toHaveAccessibleName(/: 21,403 tokens$/);
    expect(bars()[4]).toHaveAccessibleName(/: 0 tokens$/);
    await userEvent.selectOptions(kind(), "implementation");
    expect(bars()[6]).toHaveAccessibleName(/: 153,210 tokens$/);
    expect(bars()[4]).toHaveAccessibleName(/: 9,805 tokens$/);
    await userEvent.selectOptions(kind(), "All kinds");
    expect(bars()[6]).toHaveAccessibleName(/: 248,520 tokens$/);
    expect(fake.calls.filter((c) => c === "usage").length).toBe(reads);
  });

  it("goes back to all kinds when the chosen kind isn't in the new scope", async () => {
    renderApp(seeded(), "/?view=usage");
    await waitFor(() => expect(bars()).toHaveLength(7));
    await userEvent.selectOptions(kind(), "review");
    await waitFor(() => expect(within(scope()).getAllByRole("option")).toHaveLength(4));
    await userEvent.selectOptions(scope(), "Beta");

    await waitFor(() => expect(within(kind()).getAllByRole("option").map((o) => o.textContent)).toEqual(["All kinds", "spec"]));
    expect(kind()).toHaveValue("");
    expect(bars()[6]).toHaveAccessibleName(/: 73,907 tokens$/);
  });

  it("says there is no token usage when there are calls but no sessions", async () => {
    const fake = world();
    calls(fake, "acme", "claim", 1, { request: 10, response: 100 });
    renderApp(fake, "/?view=usage");
    await waitFor(() => expect(ops()).toEqual(["claim"]));
    expect(within(usage()).getByText("No token usage in All Projects over the last 7 days. Only sessions the Factory runs are counted.")).toBeInTheDocument();
    expect(within(usage()).queryByRole("group", { name: "Token usage per day" })).toBeNull();
  });
});

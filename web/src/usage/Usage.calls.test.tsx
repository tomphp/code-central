// Usage view tests: the Code Central calls table and Token usage.
import { within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { type World } from "../api/core/core";
import type { UsageResponse } from "../api/types";
import { bytes, count } from "../format";
import { settle } from "../worldTesting";
import { projects, calls, world, renderApp, usage, table, chooseScope, range, column, ops, openTab, claimed } from "./Usage.testutil";

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
    await renderApp(w, "/?view=usage");
    await openTab("Code Central calls");
    await settle(world);
    expect(ops()).toEqual(["get_ticket", "claim", "find_tickets", "finish"]);
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
    await renderApp(w, "/?view=usage");
    await openTab("Code Central calls");
    await settle(world);
    expect(ops()).toHaveLength(4);
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
    await renderApp(w, "/?view=usage&project=quiet&days=30");
    expect(within(usage()).getByText("No token usage in Quiet over the last 30 days. Only sessions the Factory runs are counted.")).toBeInTheDocument();
    await openTab("Context Size");
    expect(within(usage()).getByText("No sessions in Quiet over the last 30 days. Only sessions the Factory runs are counted.")).toBeInTheDocument();
    await openTab("Code Central calls");
    expect(within(usage()).getByText("No Code Central calls in Quiet over the last 30 days.")).toBeInTheDocument();
    expect(within(usage()).queryByRole("table")).toBeNull();
    expect(within(usage()).queryByText(/No usage yet/)).toBeNull();
  });

  it("says so for All Projects too", async () => {
    await renderApp((await projects()).w, "/?view=usage&tab=calls");
    await settle(world);
    expect(within(usage()).getByText("No Code Central calls in All Projects over the last 7 days.")).toBeInTheDocument();
  });
});

// claimed has the Factory claim a new Ticket in project for session and
// post the session's model responses, returning the Ticket's ID.

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
    await renderApp(await withUsage(), "/?view=usage");
    await settle(world);
    expect(bars()).toHaveLength(7);
    expect(bars()[6]).toHaveAccessibleName(/: 248,520 tokens$/);
    expect(bars()[4]).toHaveAccessibleName(/: 9,805 tokens$/);
    expect(bars()[5]).toHaveAccessibleName(/: 0 tokens$/);
    expect(
      within(usage())
        .getAllByText(/^(Input|Cache write|Cache read|Output)$/)
        .map((l) => l.textContent),
    ).toEqual(["Input", "Cache write", "Cache read", "Output"]);

    await userEvent.selectOptions(range(), "Last 30 days");
    await settle(world);
    expect(bars()).toHaveLength(30);
  });

  it("shows a bar's exact figures with thousands separators on hover, and never a dollar figure", async () => {
    await renderApp(await withUsage(), "/?view=usage");
    await settle(world);
    expect(bars()).toHaveLength(7);
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
    await renderApp(w, "/?view=usage");
    await settle(world);
    expect(bars()).toHaveLength(7);
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
    await renderApp(await withUsage(), "/?view=usage");
    await settle(world);
    expect(bars()).toHaveLength(7);
    await userEvent.selectOptions(kind(), "review");
    await settle(world);
    await chooseScope("Beta");

    await settle(world);
    expect(
      within(kind())
        .getAllByRole("option")
        .map((o) => o.textContent),
    ).toEqual(["All kinds", "spec"]);
    expect(kind()).toHaveValue("");
    expect(bars()[6]).toHaveAccessibleName(/: 73,907 tokens$/);
  });

  it("says there is no token usage when there are calls but no sessions", async () => {
    const { w, s } = await projects();
    await calls(s, "acme", "find_tickets", 1, {});
    await renderApp(w, "/?view=usage");
    expect(within(usage()).getByText("No token usage in All Projects over the last 7 days. Only sessions the Factory runs are counted.")).toBeInTheDocument();
    expect(within(usage()).queryByRole("group", { name: "Token usage per day" })).toBeNull();
  });
});

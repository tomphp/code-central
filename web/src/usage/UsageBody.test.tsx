// UsageBody takes its figures, sort and tab as props: these tests render
// it directly, without a Core.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { DEFAULT_SORT, nextSort, UsageBody, type UsageBodyProps } from "./UsageBody";
import { empty, populated } from "./fixtures";

function body(props: Partial<UsageBodyProps> = {}) {
  const onSort = vi.fn();
  const navigate = vi.fn();
  const onTab = vi.fn();
  const view = render(
    <UsageBody
      days={7}
      scopeName="Acme"
      here={{ view: "usage" }}
      projectName={(k) => k}
      navigate={navigate}
      sort={DEFAULT_SORT}
      onSort={onSort}
      tab="tokens"
      onTab={onTab}
      {...props}
    />,
  );
  return { onSort, navigate, onTab, unmount: view.unmount };
}

const strip = () => screen.getByRole("tablist", { name: "Usage" });
const panel = () => screen.getByRole("tabpanel");
// before reports whether a comes before b in the page.
const before = (a: Node, b: Node) => Boolean(a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING);

describe("UsageBody states", () => {
  it("says Loading… above the tab strip until the figures arrive", () => {
    body();
    expect(before(screen.getByText("Loading…"), strip())).toBe(true);
  });

  it("shows an error as an alert above the tab strip and no Loading…", () => {
    body({ error: "boom" });
    expect(screen.getByRole("alert")).toHaveTextContent("boom");
    expect(before(screen.getByRole("alert"), strip())).toBe(true);
    expect(screen.queryByText("Loading…")).toBeNull();
  });

  it("shows the tabs in order, the strip even when there is no usage", () => {
    body({ data: empty });
    expect(
      within(strip())
        .getAllByRole("tab")
        .map((t) => t.textContent),
    ).toEqual(["Token usage", "Context Size", "Code Central calls"]);
    expect(screen.queryByText(/No usage yet/)).toBeNull();
  });

  it("gives each tab its own note when empty", () => {
    const notes = {
      tokens: "No token usage in Acme over the last 7 days. Only sessions the Factory runs are counted.",
      context: "No sessions in Acme over the last 7 days. Only sessions the Factory runs are counted.",
      calls: "No Code Central calls in Acme over the last 7 days.",
    } as const;
    for (const tab of ["tokens", "context", "calls"] as const) {
      const { unmount } = body({ data: empty, tab });
      expect(panel()).toHaveTextContent(notes[tab]);
      expect(screen.queryByRole("table")).toBeNull();
      unmount();
    }
  });
});

describe("tabs", () => {
  it("shows only the token chart on Token usage", () => {
    body({ data: populated, tab: "tokens" });
    expect(screen.getByRole("tab", { name: "Token usage" })).toHaveAttribute("aria-selected", "true");
    expect(panel()).toHaveAccessibleName("Token usage");
    expect(within(panel()).getByRole("group", { name: "Token usage per day" })).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("shows Context Size by kind, then Recent sessions, on Context Size", () => {
    body({ data: populated, tab: "context" });
    expect(panel()).toHaveAccessibleName("Context Size");
    expect(
      within(panel())
        .getAllByRole("table")
        .map((t) => t.getAttribute("aria-label")),
    ).toEqual(["Context Size by kind", "Recent sessions"]);
    expect(screen.queryByRole("group", { name: "Token usage per day" })).toBeNull();
  });

  it("shows only the calls table on Code Central calls", () => {
    body({ data: populated, tab: "calls" });
    expect(panel()).toHaveAccessibleName("Code Central calls");
    expect(
      within(panel())
        .getAllByRole("table")
        .map((t) => t.getAttribute("aria-label")),
    ).toEqual(["Code Central calls"]);
  });

  it("calls onTab with the tab clicked", async () => {
    const { onTab } = body({ data: populated });
    await userEvent.click(screen.getByRole("tab", { name: "Code Central calls" }));
    expect(onTab).toHaveBeenCalledWith("calls");
  });
});

describe("sorting", () => {
  it("orders the rows by the sort it is given and marks the column", () => {
    body({ data: populated, tab: "calls", sort: { key: "op", descending: false } });
    const rows = within(screen.getByRole("table", { name: "Code Central calls" })).getAllByRole("rowheader");
    expect(rows.map((r) => r.textContent)).toEqual(["claim", "finish", "get_ticket"]);
    expect(screen.getByRole("columnheader", { name: "Operation" })).toHaveAttribute("aria-sort", "ascending");
  });

  it("calls onSort with the column clicked", async () => {
    const { onSort } = body({ data: populated, tab: "calls" });
    await userEvent.click(screen.getByRole("button", { name: "Errors" }));
    expect(onSort).toHaveBeenCalledWith("errors");
  });

  it("nextSort reverses a column and starts operations A to Z, numbers largest first", () => {
    expect(nextSort(DEFAULT_SORT, "calls")).toEqual({ key: "calls", descending: false });
    expect(nextSort(DEFAULT_SORT, "op")).toEqual({ key: "op", descending: false });
    expect(nextSort(DEFAULT_SORT, "errors")).toEqual({ key: "errors", descending: true });
  });
});

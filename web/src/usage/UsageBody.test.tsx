// UsageBody takes its figures and sort as props: these tests render it
// directly, without a Core.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { DEFAULT_SORT, nextSort, UsageBody, type UsageBodyProps } from "./UsageBody";
import { empty, populated } from "./fixtures";

function body(props: Partial<UsageBodyProps> = {}) {
  const onSort = vi.fn();
  const navigate = vi.fn();
  render(
    <UsageBody days={7} scopeName="Acme" here={{ view: "usage" }} projectName={(k) => k} navigate={navigate} sort={DEFAULT_SORT} onSort={onSort} {...props} />,
  );
  return { onSort, navigate };
}

describe("UsageBody states", () => {
  it("says Loading… until the figures arrive", () => {
    body();
    expect(screen.getByText("Loading…")).toBeInTheDocument();
  });

  it("shows an error as an alert and no Loading…", () => {
    body({ error: "boom" });
    expect(screen.getByRole("alert")).toHaveTextContent("boom");
    expect(screen.queryByText("Loading…")).toBeNull();
  });

  it("says there is no usage in the scope and range when empty", () => {
    body({ data: empty });
    expect(screen.getByText("No usage yet in Acme over the last 7 days.")).toBeInTheDocument();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("shows the three sections when populated", () => {
    body({ data: populated });
    expect(screen.getByRole("table", { name: "Code Central calls" })).toBeInTheDocument();
    expect(screen.getByRole("table", { name: "Recent sessions" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Token usage" })).toBeInTheDocument();
  });

  it("shows the session chart it is given", () => {
    body({ data: populated, session: <section aria-label="Open chart" /> });
    expect(screen.getByRole("region", { name: "Open chart" })).toBeInTheDocument();
  });
});

describe("sorting", () => {
  it("orders the rows by the sort it is given and marks the column", () => {
    body({ data: populated, sort: { key: "op", descending: false } });
    const rows = within(screen.getByRole("table", { name: "Code Central calls" })).getAllByRole("rowheader");
    expect(rows.map((r) => r.textContent)).toEqual(["claim", "finish", "get_ticket"]);
    expect(screen.getByRole("columnheader", { name: "Operation" })).toHaveAttribute("aria-sort", "ascending");
  });

  it("calls onSort with the column clicked", async () => {
    const { onSort } = body({ data: populated });
    await userEvent.click(screen.getByRole("button", { name: "Errors" }));
    expect(onSort).toHaveBeenCalledWith("errors");
  });

  it("nextSort reverses a column and starts operations A to Z, numbers largest first", () => {
    expect(nextSort(DEFAULT_SORT, "calls")).toEqual({ key: "calls", descending: false });
    expect(nextSort(DEFAULT_SORT, "op")).toEqual({ key: "op", descending: false });
    expect(nextSort(DEFAULT_SORT, "errors")).toEqual({ key: "errors", descending: true });
  });
});

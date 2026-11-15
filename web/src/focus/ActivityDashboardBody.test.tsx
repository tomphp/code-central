// The Activity Dashboard body's tests, through its props: its counts, its
// In Progress list and its CI Running entries, in one Project and in All
// Projects, with the data read once from a WASM Core world.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, describe, expect, it, vi } from "vitest";
import { ApiError } from "../api/types";
import type { UiLocation } from "../url";
import { ActivityDashboardBody, type ActivityDashboardBodyProps } from "./ActivityDashboardBody";
import { dashboardData, type DashboardData } from "./storyData";

let d: DashboardData;
beforeAll(async () => {
  d = await dashboardData();
});

function renderBody(more: Partial<ActivityDashboardBodyProps> = {}) {
  const navigate = vi.fn<(patch: Partial<UiLocation>) => void>();
  const props: ActivityDashboardBodyProps = { now: d.now, data: d.data, changes: {}, loc: { view: "activity" }, navigate, ...more };
  const view = render(<ActivityDashboardBody {...props} />);
  return { ...view, props, navigate };
}

const dashboard = () => screen.getByRole("region", { name: "Activity Dashboard" });
const count = (name: string) => within(within(dashboard()).getByRole("group", { name })).getByTestId("count").textContent;
const inProgress = () => within(screen.getByRole("list", { name: "In Progress" })).getAllByRole("listitem");

describe("the Activity Dashboard body in All Projects", () => {
  it("counts Ready and In Progress in every Project", () => {
    renderBody();
    expect(count("Ready")).toBe("2");
    expect(count("In Progress")).toBe("2");
  });

  it("lists the In Progress Tickets by Project, each with its Project's name", () => {
    renderBody();
    const items = inProgress();
    expect(items).toHaveLength(2);
    expect(within(items[0]).getByText(d.ids.signup)).toBeInTheDocument();
    expect(items[0]).toHaveTextContent("Sign-up page");
    expect(items[0]).toHaveTextContent("Acme");
    expect(within(items[1]).getByText(d.ids.betaExport)).toBeInTheDocument();
    expect(items[1]).toHaveTextContent("Beta export");
    expect(items[1]).toHaveTextContent("Beta");
  });

  it("opens an In Progress Ticket in the Inbox", async () => {
    const { navigate } = renderBody();
    await userEvent.click(within(inProgress()[1]).getByRole("link"));
    expect(navigate).toHaveBeenCalledWith({ view: "inbox", project: undefined, requirement: undefined, ticket: d.ids.betaExport });
  });

  it("pulses an In Progress entry when its Ticket changes", () => {
    const { rerender, props } = renderBody();
    expect(inProgress()[0]).not.toHaveAttribute("data-pulsing");
    rerender(<ActivityDashboardBody {...props} changes={{ [d.ids.signup]: 1 }} />);
    expect(inProgress()[0]).toHaveAttribute("data-pulsing", "true");
    expect(inProgress()[1]).not.toHaveAttribute("data-pulsing");
  });
});

describe("the Activity Dashboard body in one Project", () => {
  it("counts and lists only that Project's Tickets, without its name", () => {
    renderBody({ loc: { view: "activity", project: "acme" } });
    expect(count("Ready")).toBe("1");
    expect(count("In Progress")).toBe("1");
    const [signup] = inProgress();
    expect(inProgress()).toHaveLength(1);
    expect(signup).toHaveTextContent("Sign-up page");
    expect(signup).not.toHaveTextContent("Acme");
  });

  it("says nothing is in progress when nothing is", () => {
    renderBody({ data: { ...d.data, tickets: d.data.tickets.filter((t) => t.state !== "In Progress") } });
    expect(count("In Progress")).toBe("0");
    expect(within(dashboard()).getByText("Nothing in progress.")).toBeInTheDocument();
    expect(screen.queryByRole("list", { name: "In Progress" })).toBeNull();
  });
});

describe("the Activity Dashboard body's CI Running", () => {
  it("isn't there without CI Tracking in scope", () => {
    renderBody();
    expect(within(dashboard()).queryByRole("group", { name: "CI Running" })).toBeNull();
    expect(within(dashboard()).queryByRole("list", { name: "CI Running" })).toBeNull();
  });

  it("shows a dash and no list until the first read", () => {
    renderBody({ ciRuns: {} });
    expect(count("CI Running")).toBe("–");
    expect(within(dashboard()).queryByRole("list", { name: "CI Running" })).toBeNull();
  });

  it("counts and lists the running CI Runs, each with its Ticket, branch, short commit and status", () => {
    renderBody({ ciRuns: { data: d.ciRuns } });
    expect(count("CI Running")).toBe("2");
    const runs = within(screen.getByRole("list", { name: "CI Running" })).getAllByRole("listitem");
    expect(runs.map((r) => r.getAttribute("aria-label"))).toEqual(["CI", "lint.yml"]);
    expect(runs[0]).toHaveTextContent(d.ids.theme);
    expect(runs[0]).toHaveTextContent("Acme");
    expect(runs[0]).toHaveTextContent("abc1234");
    expect(runs[0]).not.toHaveTextContent("abc1234def");
    expect(within(runs[0]).getByRole("link", { name: /GitHub/ })).toHaveAttribute("href", "https://github.com/acme/web/actions/runs/1");
  });

  it("leaves the Project's name off its entries in one Project", () => {
    renderBody({ ciRuns: { data: d.ciRuns }, loc: { view: "activity", project: "acme" } });
    const [first] = within(screen.getByRole("list", { name: "CI Running" })).getAllByRole("listitem");
    expect(first).not.toHaveTextContent("Acme");
  });

  it("opens the CI view from its block, with Back to where it was", async () => {
    const loc: UiLocation = { view: "activity", project: "acme" };
    const { navigate } = renderBody({ ciRuns: { data: d.ciRuns }, loc });
    await userEvent.click(within(within(dashboard()).getByRole("group", { name: "CI Running" })).getByRole("link"));
    expect(navigate).toHaveBeenCalledWith({ view: "ci", project: "acme", back: loc });
  });

  it("opens a run's Ticket in the Inbox", async () => {
    const { navigate } = renderBody({ ciRuns: { data: d.ciRuns } });
    const [first] = within(screen.getByRole("list", { name: "CI Running" })).getAllByRole("listitem");
    await userEvent.click(within(first).getAllByRole("link")[0]);
    expect(navigate).toHaveBeenCalledWith({ view: "inbox", project: undefined, requirement: undefined, ticket: d.ids.theme });
  });

  it("says none are running when none are", () => {
    renderBody({ ciRuns: { data: { ...d.ciRuns, runs: [] } } });
    expect(count("CI Running")).toBe("0");
    expect(within(dashboard()).getByText("No CI Runs running.")).toBeInTheDocument();
  });

  it("shows a failed read's error in place of the runs", () => {
    renderBody({ ciRuns: { error: new ApiError(500, "internal", "CI is down") } });
    expect(within(dashboard()).getByRole("alert")).toHaveTextContent("CI is down");
  });
});

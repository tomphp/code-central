// CIBody prop tests: the presentational CI body from plain props.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { CIBody, type CIBodyProps } from "./CIView";
import { NOW, finished, projectName, running, status } from "./ciFixtures";

function body(props: Partial<CIBodyProps> = {}) {
  const navigate = vi.fn();
  render(<CIBody now={NOW} projectName={projectName} navigate={navigate} {...props} />);
  return navigate;
}

describe("CIBody", () => {
  it("shows Loading… before the read", () => {
    body();
    expect(screen.getByText("Loading…")).toBeInTheDocument();
  });

  it("shows the error as an alert", () => {
    body({ error: "boom" });
    expect(screen.getByRole("alert")).toHaveTextContent("boom");
    expect(screen.queryByText("Loading…")).not.toBeInTheDocument();
  });

  it("says when there are no runs", () => {
    body({ project: "acme", overview: { projects: [status("acme")], runs: [] } });
    expect(screen.getByText("Acme: Watching acme/web.")).toBeInTheDocument();
    expect(screen.getByText("No CI Runs in progress or finished in the last 7 days.")).toBeInTheDocument();
  });

  it("lists the runs, with a Project column only in All Projects", () => {
    const overview = { projects: [status("acme")], runs: [running("acme", 2), finished("acme", 1)] };
    const { unmount } = render(<CIBody now={NOW} projectName={projectName} navigate={() => {}} overview={overview} />);
    const table = screen.getByRole("table", { name: "CI Runs" });
    expect(within(table).getByRole("columnheader", { name: "Project" })).toBeInTheDocument();
    expect(within(table).getAllByRole("row")).toHaveLength(3);
    unmount();
    body({ project: "acme", overview });
    expect(screen.queryByRole("columnheader", { name: "Project" })).not.toBeInTheDocument();
  });

  it("calls navigate with the Ticket a run is tracked against", async () => {
    const navigate = body({ project: "acme", overview: { projects: [status("acme")], runs: [finished("acme", 1, { ticket: "acme-1" })] } });
    await userEvent.click(screen.getByRole("link", { name: "acme-1" }));
    expect(navigate).toHaveBeenCalledWith({ view: "inbox", project: "acme", ticket: "acme-1" });
  });
});

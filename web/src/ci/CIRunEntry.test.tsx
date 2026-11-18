// CIRunEntry prop tests: each layout keeps the fields it shows today, the
// row in the CI view's table, the item in a Ticket's CI section and the
// card on the Activity Dashboard.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it, vi } from "vitest";
import { OneProject } from "../inbox/ShortId";
import { CIRunEntry, runKey, type CIRunCardProps } from "./CIRunEntry";
import { NOW, finished, running } from "./ciFixtures";

// inTable renders a row where a <tr> may go.
const inTable = (row: ReactNode) => render(
  <table>
    <tbody>{row}</tbody>
  </table>,
);

// cells is the text of a row's header and cells.
const cells = (row: HTMLElement) => [within(row).getByRole("rowheader"), ...within(row).getAllByRole("cell")].map((c) => c.textContent);

describe("CIRunEntry's row", () => {
  it("shows the workflow, branch, short commit, status, duration, what it is tracked against, the Project and the link", () => {
    inTable(<CIRunEntry layout="row" run={finished("acme", 1, { attempt: 2, ticket: "acme-1" })} now={NOW} projectName="Acme" navigate={() => {}} />);
    const row = screen.getByRole("row", { name: "CI" });
    expect(cells(row)).toEqual(["CI ci.yml", "main", "1abcdef", "passed · attempt 2", "3m 12s", "acme-1", "Acme", "View on GitHub"]);
    expect(within(row).getByRole("link", { name: "View on GitHub" })).toHaveAttribute("href", "https://example.com/run");
  });

  it("leaves the Project out in one Project, and names a CI Rule it is tracked against", () => {
    inTable(<CIRunEntry layout="row" run={finished("acme", 1, { rule: "acme-r1" })} now={NOW} navigate={() => {}} />);
    expect(cells(screen.getByRole("row", { name: "CI" }))).toEqual(["CI ci.yml", "main", "1abcdef", "passed", "3m 12s", "CI Rule acme-r1", "View on GitHub"]);
  });

  it("opens the Ticket it is tracked against", async () => {
    const navigate = vi.fn();
    inTable(<CIRunEntry layout="row" run={finished("acme", 1, { ticket: "acme-1" })} now={NOW} navigate={navigate} />);
    await userEvent.click(screen.getByRole("link", { name: "acme-1" }));
    expect(navigate).toHaveBeenCalledWith({ view: "inbox", project: "acme", ticket: "acme-1" });
  });

  it("names the link from the CI Provider", () => {
    inTable(<CIRunEntry layout="row" run={running("acme", 1)} provider="azure_pipelines" now={NOW} navigate={() => {}} />);
    expect(screen.getByRole("link", { name: "View on Azure DevOps" })).toBeInTheDocument();
  });

  it("greys an ignored run and marks it ignored", () => {
    inTable(<CIRunEntry layout="row" run={finished("acme", 1, { ignored: true })} now={NOW} navigate={() => {}} />);
    const row = screen.getByRole("row", { name: "CI" });
    expect(row).toHaveClass("ignored");
    expect(cells(row)[3]).toBe("passedignored");
  });
});

describe("CIRunEntry's item", () => {
  const item = (more: Parameters<typeof finished>[2] = {}) => {
    render(
      <ul>
        <CIRunEntry layout="item" run={finished("acme", 1, more)} />
      </ul>,
    );
    return screen.getByRole("listitem", { name: "CI" });
  };

  it("links the run by name and shows its workflow, status, attempt and short commit", () => {
    const run = item({ attempt: 2, conclusion: "failure" });
    expect(within(run).getByRole("link", { name: "CI" })).toHaveAttribute("href", "https://example.com/run");
    expect(run).toHaveTextContent(/^CI ci\.yml failed · attempt 2 · 1abcdef$/);
  });

  it("lists the run's jobs, each linked with its status", () => {
    const run = item({
      jobs: [
        { name: "test", conclusion: "failure", url: "https://example.com/job/1" },
        { name: "lint", url: "https://example.com/job/2" },
      ],
    });
    const jobs = within(run).getAllByRole("listitem");
    expect(jobs.map((j) => j.textContent)).toEqual(["test failed", "lint running"]);
    expect(within(jobs[0]).getByRole("link", { name: "test" })).toHaveAttribute("href", "https://example.com/job/1");
  });

  it("greys an ignored run and marks it ignored", () => {
    const run = item({ ignored: true });
    expect(run).toHaveClass("ignored");
    expect(within(run).getByText("ignored")).toBeInTheDocument();
  });
});

describe("CIRunEntry's card", () => {
  const card = (more: Partial<CIRunCardProps> = {}) => {
    const navigate = vi.fn();
    render(
      <ul>
        <CIRunEntry layout="card" run={{ ...running("acme", 1), ticket: "acme-1" }} now={NOW} to={{ view: "inbox", ticket: "acme-1" }} navigate={navigate} {...more} />
      </ul>,
    );
    return { entry: screen.getByRole("listitem", { name: "CI" }), navigate };
  };

  it("shows the Ticket, the workflow, the Project, the branch, the short commit, the status as a badge and how long it has run", () => {
    const { entry } = card({ projectName: "Acme" });
    expect(entry).toHaveTextContent(/^acme-1CIAcmemain · 1abcdef · running · 3m 0sView on GitHub$/);
    expect(within(entry).getByText("running")).toHaveClass("ci-status");
  });

  it("leaves the duration of a run not started out", () => {
    const { entry } = card({ run: { ...running("acme", 1), status: "queued", started: undefined, ticket: "acme-1" } });
    expect(entry).toHaveTextContent(/^acme-1CImain · 1abcdef · queuedView on GitHub$/);
  });

  it("opens its Ticket, and links the run beside it", async () => {
    const { entry, navigate } = card({ provider: "azure_pipelines" });
    await userEvent.click(within(entry).getAllByRole("link")[0]);
    expect(navigate).toHaveBeenCalledWith({ view: "inbox", ticket: "acme-1" });
    expect(within(entry).getByRole("link", { name: "View on Azure DevOps" })).toHaveAttribute("href", "https://example.com/run");
  });
});

describe("CIRunEntry's Short IDs", () => {
  it("shows the card's Ticket as a Short ID under OneProject, and a click copies without following the link", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    const navigate = vi.fn();
    render(
      <OneProject.Provider value>
        <ul>
          <CIRunEntry layout="card" run={{ ...running("acme", 1), ticket: "acme-1" }} now={NOW} to={{ view: "inbox", ticket: "acme-1" }} navigate={navigate} />
        </ul>
      </OneProject.Provider>,
    );
    await userEvent.click(screen.getByText("#1"));
    expect(writeText).toHaveBeenCalledWith("acme-1");
    expect(navigate).not.toHaveBeenCalled();
  });

  it("keeps the row's Ticket in full even under OneProject", () => {
    render(
      <OneProject.Provider value>
        <table>
          <tbody>
            <CIRunEntry layout="row" run={finished("acme", 1, { ticket: "acme-1" })} now={NOW} navigate={() => {}} />
          </tbody>
        </table>
      </OneProject.Provider>,
    );
    expect(screen.getByText("acme-1")).toBeInTheDocument();
  });
});

describe("runKey", () => {
  it("names a run across Projects, the same for each attempt", () => {
    expect(runKey(finished("acme", 1))).toBe("acme/1");
    expect(runKey(finished("acme", 1, { attempt: 2 }))).toBe("acme/1");
    expect(runKey(finished("beta", 1))).toBe("beta/1");
  });
});

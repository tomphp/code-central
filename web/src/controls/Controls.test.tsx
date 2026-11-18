import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { backLabel } from "./backLabel";
import { Count } from "./Count";
import { Empty, Loading } from "./Empty";
import { ErrorBanner } from "./ErrorBanner";
import { ProjectName, projectName } from "./ProjectName";
import { SpecPaneHeader } from "./SpecPaneHeader";
import { ViewHeader } from "./ViewHeader";

describe("Empty", () => {
  it("renders its note in the empty style", () => {
    render(<Empty>Nothing here.</Empty>);
    expect(screen.getByText("Nothing here.")).toHaveClass("empty");
  });

  it("Loading reads Loading…", () => {
    render(<Loading />);
    expect(screen.getByText("Loading…")).toHaveClass("empty");
  });
});

describe("ErrorBanner", () => {
  it("is an alert in the warning style, with placement classes added", () => {
    render(<ErrorBanner className="project-error">Broke</ErrorBanner>);
    const alert = screen.getByRole("alert", { name: "" });
    expect(alert).toHaveTextContent("Broke");
    expect(alert.className).toBe("warning project-error");
  });
});

describe("Count", () => {
  it("shows the number, loud when asked", () => {
    const { rerender } = render(<Count n={3} />);
    expect(screen.getByText("3").className).toBe("count");
    rerender(<Count n={3} loud />);
    expect(screen.getByText("3").className).toBe("count loud");
  });

  it("names what it counts after the number", () => {
    render(<Count n={2} of="left" />);
    expect(screen.getByText("2 left")).toHaveClass("count");
  });

  it("as a block's number, shows a dash until the count is known", () => {
    const { rerender } = render(<Count big />);
    expect(screen.getByTestId("count")).toHaveTextContent("–");
    rerender(<Count n={0} big />);
    expect(screen.getByTestId("count")).toHaveTextContent("0");
  });
});

describe("ProjectName", () => {
  it("shows the display name with the key muted", () => {
    render(<ProjectName name="Acme Web" projectKey="acme" />);
    expect(screen.getByText("acme")).toHaveClass("muted");
    expect(screen.getByText("Acme Web", { exact: false })).toBeInTheDocument();
  });

  it("looks a display name up by key, falling back to the key", () => {
    const projects = [{ key: "acme", display_name: "Acme Web" }];
    expect(projectName(projects, "acme")).toBe("Acme Web");
    expect(projectName(projects, "other")).toBe("other");
  });
});

describe("backLabel", () => {
  it.each([
    [undefined, "Back to the Inbox"],
    [{ view: "focus" as const }, "Back to Focus Mode"],
    [{ view: "activity" as const }, "Back to Activity"],
  ])("labels %j as %s", (back, label) => {
    expect(backLabel({ back } as never)).toBe(label);
  });
});

describe("SpecPaneHeader", () => {
  it("shows the ID, Kind with submitted time, and title as a heading", () => {
    const now = new Date("2026-01-02T00:00:00Z");
    render(<SpecPaneHeader id="acme-1" kind="Map" title="The plan" now={now} created="2026-01-01T00:00:00Z" />);
    expect(screen.getByRole("heading", { level: 2, name: "The plan" })).toBeInTheDocument();
    expect(screen.getByText(/^Map · submitted .+ ago$/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "acme-1" })).toBeInTheDocument();
  });

  it("shows an Archived badge only when archived", () => {
    const now = new Date("2026-01-02T00:00:00Z");
    const { rerender } = render(<SpecPaneHeader id="acme-1" kind="Spec" title="T" now={now} created="2026-01-01T00:00:00Z" />);
    expect(screen.queryByText("Archived")).not.toBeInTheDocument();
    rerender(<SpecPaneHeader id="acme-1" kind="Spec" title="T" now={now} created="2026-01-01T00:00:00Z" archived />);
    expect(screen.getByText("Archived")).toBeInTheDocument();
  });
});

describe("ViewHeader", () => {
  it("shows its title as the view's heading, then its slots", () => {
    render(<ViewHeader title="Board" left={<span>left</span>} right={<button>Back</button>} />);
    expect(screen.getByRole("banner")).toHaveTextContent("Boardleft");
    expect(screen.getByRole("heading", { level: 1, name: "Board" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Back" })).toBeInTheDocument();
  });

  it("has no heading without a title, as Focus Mode's", () => {
    render(<ViewHeader left={<span>2 left</span>} right={<button>Exit</button>} />);
    expect(screen.queryByRole("heading")).toBeNull();
    expect(screen.getByRole("banner")).toHaveTextContent("2 leftExit");
  });
});

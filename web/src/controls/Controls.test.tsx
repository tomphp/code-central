import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { backLabel } from "./backLabel";
import { Count } from "./Count";
import { Empty, Loading } from "./Empty";
import { ErrorBanner } from "./ErrorBanner";
import { ProjectName, projectName } from "./ProjectName";
import { SpecPaneHeader } from "./SpecPaneHeader";

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
});

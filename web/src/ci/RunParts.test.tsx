// The shared CI Run parts' prop tests: the status badge, the ignored mark
// and the link to the run.
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { IgnoredMark, RunLink, RunStatusBadge, runStatus } from "./RunParts";
import { finished, running } from "./ciFixtures";

describe("runStatus", () => {
  it("says a run is queued, running, or how it ended", () => {
    expect(runStatus(finished("acme", 1, { status: "queued", conclusion: undefined }))).toBe("queued");
    expect(runStatus(running("acme", 1))).toBe("running");
    expect(runStatus(finished("acme", 1))).toBe("passed");
    expect(runStatus(finished("acme", 1, { conclusion: "timed_out" }))).toBe("timed out");
  });
});

describe("RunStatusBadge", () => {
  it("shows the status", () => {
    render(<RunStatusBadge status="failed" />);
    expect(screen.getByText("failed")).toBeInTheDocument();
  });

  it("shows a partially succeeded run in lowercase with no glyph", () => {
    const { container } = render(<RunStatusBadge status="partially succeeded" />);
    expect(container).toHaveTextContent(/^partially succeeded$/);
  });
});

describe("IgnoredMark", () => {
  it("says the run is ignored", () => {
    render(<IgnoredMark />);
    expect(screen.getByText("ignored")).toBeInTheDocument();
  });
});

describe("RunLink", () => {
  it("opens the run in a new tab, worded for GitHub by default", () => {
    render(<RunLink run={finished("acme", 1)} />);
    const link = screen.getByRole("link", { name: "View on GitHub" });
    expect(link).toHaveAttribute("href", "https://example.com/run");
    expect(link).toHaveAttribute("target", "_blank");
  });

  it("is worded for Azure DevOps under Azure Pipelines", () => {
    render(<RunLink run={finished("acme", 1)} provider="azure_pipelines" />);
    expect(screen.getByRole("link", { name: "View on Azure DevOps" })).toBeInTheDocument();
  });
});

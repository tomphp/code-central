import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { withCode } from "../controls/ErrorBanner";
import { SpecPaneNote } from "./SpecPaneNote";

describe("SpecPaneNote", () => {
  it("says Loading… with no error", () => {
    render(<SpecPaneNote />);
    expect(screen.getByText("Loading…")).toBeInTheDocument();
  });

  it("shows the error in its place, as an alert", () => {
    render(<SpecPaneNote error="acme-9 isn't a Spec in acme." />);
    expect(screen.getByRole("alert")).toHaveTextContent("acme-9 isn't a Spec in acme.");
    expect(screen.queryByText("Loading…")).toBeNull();
  });
});

describe("withCode", () => {
  it("sets a backticked command as code", () => {
    render(<p>{withCode("Run `gh auth login` first.")}</p>);
    expect(screen.getByText("gh auth login").tagName).toBe("CODE");
  });
});

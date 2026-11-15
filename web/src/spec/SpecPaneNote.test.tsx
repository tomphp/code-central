import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { SpecPaneNote } from "./SpecPaneNote";

describe("SpecPaneNote", () => {
  it("says Loading… with no error", () => {
    render(<SpecPaneNote />);
    expect(screen.getByText("Loading…")).toBeInTheDocument();
  });

  it("shows the error in its place", () => {
    render(<SpecPaneNote error="acme-9 isn't a Spec in acme." />);
    expect(screen.getByText("acme-9 isn't a Spec in acme.")).toBeInTheDocument();
    expect(screen.queryByText("Loading…")).toBeNull();
  });
});

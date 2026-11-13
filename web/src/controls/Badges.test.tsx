import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { KindBadge } from "./KindBadge";
import { StageBadge } from "./StageBadge";
import { StateChip, stateSlug } from "./StateChip";

describe("badges", () => {
  it("slugs a State", () => expect(stateSlug("In Progress")).toBe("in-progress"));

  it("renders a State chip", () => {
    render(<StateChip state="In Review" />);
    const el = screen.getByText("In Review");
    expect(el.className).toBe("state-chip st-in-review");
    expect(el.hasAttribute("aria-current")).toBe(false);
  });

  it("marks the chip's own State and shows children", () => {
    render(<StateChip state="Ready" here>Ready 3</StateChip>);
    const el = screen.getByText("Ready 3");
    expect(el.className).toBe("state-chip st-ready here");
    expect(el.getAttribute("aria-current")).toBe("true");
  });

  it("renders a kind badge", () => {
    render(<KindBadge kind="Long-lived" />);
    expect(screen.getByText("Long-lived").className).toBe("kind-badge kind-long-lived");
  });

  it("renders a stage badge", () => {
    render(<StageBadge label="Map" />);
    expect(screen.getByText("Map").className).toBe("stage-badge");
  });
});

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { NeedsMeReason } from "../api/types";
import type { ProjectSpec, ProjectTicket } from "../live";
import { Needing } from "../needsme/fixture";
import { LaneHeader } from "./LaneHeader";

const spec: ProjectSpec = { id: "acme-1", project: "acme", title: "Sign-in", labels: [], tags: {}, state_summary: { Ready: 1, Done: 1 }, finished: false, updated: "" };
const all = [
  { id: "acme-2", state: "Ready", spec: "acme-1", labels: [] as string[] },
  { id: "acme-3", state: "Done", spec: "acme-1", labels: [] as string[] },
] as ProjectTicket[];
const link = { href: "/acme-1", onClick: () => {}, "data-id": "acme-1" };
const header = (props: Partial<Parameters<typeof LaneHeader>[0]> = {}, reasons: Record<string, NeedsMeReason> = {}) => (
  <Needing tickets={all} reasons={reasons}>
    <LaneHeader id="acme-1" spec={spec} all={all} shown={1} link={link} collapsed={false} onToggle={() => {}} {...props} />
  </Needing>
);

describe("LaneHeader", () => {
  it("is one line: its ID, kind, title, headline and how many Tickets the lane shows", () => {
    render(header());
    const head = screen.getByRole("region", { name: "Sign-in" });
    expect(head.className).toBe("lane-header");
    expect(screen.getByRole("link").getAttribute("href")).toBe("/acme-1");
    expect(screen.getByText("Spec").className).toBe("kind-badge kind-spec");
    expect(screen.getByText("1 of 2 Done").className).toBe("lane-header-headline");
    expect(screen.getByText("1 shown")).toBeTruthy();
    expect(head.querySelector(".state-chips, .strip")).toBeNull();
    expect(screen.queryByRole("img", { name: "Needs you" })).toBeNull();
  });

  it("shows its ID as its title until the Spec loads", () => {
    render(header({ spec: undefined }));
    expect(screen.getByRole("region", { name: "acme-1" })).toBeTruthy();
  });

  it("marks a lane whose Tickets need the human", () => {
    render(header({}, { "acme-2": "needs_answer" }));
    expect(screen.getByRole("region", { name: "Sign-in" }).className).toBe("lane-header needs");
    expect(screen.getByRole("img", { name: "Needs you" }).className).toBe("dot reason-needs_answer");
  });

  it("folds and unfolds its lane with the chevron", async () => {
    const onToggle = vi.fn();
    const { rerender } = render(header({ onToggle }));
    const chev = screen.getByRole("button", { name: "Sign-in" });
    expect(chev.getAttribute("aria-expanded")).toBe("true");
    await userEvent.click(chev);
    expect(onToggle).toHaveBeenCalledOnce();
    rerender(header({ collapsed: true }));
    expect(screen.getByRole("button", { name: "Sign-in" }).getAttribute("aria-expanded")).toBe("false");
  });
});

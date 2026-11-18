// Base missing (code-central-436): a Project's own Needs Me item, with no
// Ticket, naming the branch and the fix, with a link to Project Options.
import { within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { newWorld } from "./api/core/core";
import { rowDetail } from "./format";
import { allClear, focus, renderApp, scenario } from "./worldTesting";

const fix = "Create the branch main, or change Base branch in Project Options";

async function world() {
  const w = await newWorld();
  await allClear(w);
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.base("acme", "missing");
  return w;
}

describe("Base missing", () => {
  it("is a one-line detail naming the branch and the fix", async () => {
    const r = (await (await world()).api.needsMe()).groups[0].tickets[0];
    expect(rowDetail(new Date(), r)).toBe(fix);
  });

  it("is a Focus Item for the Project with a link to Project Options", async () => {
    await renderApp(await world());
    const it = within(focus()).getByRole("article", { name: "Focus Item" });
    expect(within(it).getAllByText("Base missing")).toHaveLength(1);
    expect(within(it).getByText("Focus: Base missing")).toBeInTheDocument();
    expect(within(it).getByText(/main/)).toBeInTheDocument();
    const link = within(it).getByRole("link", { name: "Project Options" });
    expect(link.getAttribute("href")).toContain("view=project-options");
    expect(link.getAttribute("href")).toContain("project=acme");
    expect(within(within(it).getByRole("heading", { level: 2 })).queryByRole("link")).toBeNull();
  });
});

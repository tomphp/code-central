// Base missing (code-central-436): a Project's own Needs Me item, with no
// Ticket, naming the branch and the fix, with a link to Project Options.
import { within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { FakeApi } from "./api/fake";
import { allClearOn, focus, renderApp } from "./focus/focusTesting";
import { rowDetail } from "./format";

const row = {
  id: "",
  project: "acme",
  title: "Base missing",
  base_missing: {
    base: "release",
    fix: "Create the branch release, or change Base branch in Project Options",
  },
};

function world() {
  const fake = new FakeApi();
  allClearOn(fake);
  fake.addProject("acme", "Acme");
  fake.setNeedsMe([{ reason: "base_missing", tickets: [row] }]);
  return fake;
}

describe("Base missing", () => {
  it("is a one-line detail naming the branch and the fix", async () => {
    const r = (await world().needsMe()).groups[0].tickets[0];
    expect(rowDetail(new Date(), r)).toBe(row.base_missing.fix);
  });

  it("is a Focus Item for the Project with a link to Project Options", async () => {
    renderApp(world());
    const it = await within(focus()).findByRole("article", { name: "Focus Item" });
    expect(within(it).getAllByText("Base missing")).toHaveLength(2);
    expect(within(it).getByText(/release/)).toBeInTheDocument();
    const link = within(it).getByRole("link", { name: "Project Options" });
    expect(link.getAttribute("href")).toContain("view=project-options");
    expect(link.getAttribute("href")).toContain("project=acme");
    expect(within(it).queryByRole("link", { name: /Open in Inbox/ })).toBeNull();
  });
});

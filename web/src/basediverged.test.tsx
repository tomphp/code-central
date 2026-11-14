// Base diverged (code-central-394): a Project's own Needs Me item, with no
// Ticket, in the Inbox and in Focus Mode, showing the local and remote
// commits and the fix, against FakeApi.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { FakeApi } from "./api/fake";
import { allClearOn, focus, renderApp } from "./focus/focusTesting";
import { rowDetail } from "./format";

function world() {
  const fake = new FakeApi();
  allClearOn(fake);
  fake.addProject("acme", "Acme");
  fake.addProject("beta", "Beta");
  fake.addTicket("acme", { title: "Login form", state: "In Review" }); // acme-1
  fake.setNeedsMe([
    { reason: "in_review", tickets: [{ id: "acme-1" }] },
    {
      reason: "base_diverged",
      tickets: [
        {
          id: "",
          project: "acme",
          title: "Base diverged",
          base_diverged: {
            base: "main",
            local: "aaa1111222",
            remote: "bbb2222333",
          },
        },
      ],
    },
  ]);
  return fake;
}

const item = () =>
  within(focus()).findByRole("article", { name: "Focus Item" });

describe("Base diverged", () => {
  it("is a one-line detail with both commits", async () => {
    const row = (await world().needsMe()).groups[1].tickets[0];
    expect(rowDetail(new Date(), row)).toBe(
      "main is at aaa1111 and the remote's at bbb2222: reconcile them by hand",
    );
  });

  it("is a Focus Item for the Project with the commits and the fix, and no Ticket panel", async () => {
    const fake = world();
    fake.setNeedsMe([
      {
        reason: "base_diverged",
        tickets: [
          {
            id: "",
            project: "acme",
            title: "Base diverged",
            base_diverged: {
              base: "main",
              local: "aaa1111222",
              remote: "bbb2222333",
            },
          },
        ],
      },
    ]);
    renderApp(fake);
    const it = await item();
    // Its reason and its title.
    expect(within(it).getAllByText("Base diverged")).toHaveLength(1);
    expect(within(it).getByText("Focus: Base diverged")).toBeInTheDocument();
    expect(within(it).getByText("aaa1111")).toBeInTheDocument();
    expect(within(it).getByText("bbb2222")).toBeInTheDocument();
    expect(
      within(it).getByText(/carries on by itself on its next poll/),
    ).toBeInTheDocument();
    expect(
      within(it).queryByRole("link", { name: /Open in Inbox/ }),
    ).toBeNull();

    // It can be skipped like any item.
    await userEvent.click(within(it).getByRole("button", { name: "Skip" }));
    expect(
      await screen.findByRole("region", {
        name: "Only skipped Focus Items left",
      }),
    ).toBeInTheDocument();
  });
});

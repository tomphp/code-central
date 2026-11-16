// Base diverged (code-central-394): a Project's own Needs Me item, with no
// Ticket, in the Inbox and in Focus Mode, showing the local and remote
// commits and the fix, on the WASM Core.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld } from "./api/core/core";
import { scenario } from "./api/scenario";
import { rowDetail } from "./format";
import { allClear, focus, renderApp } from "./worldTesting";

// world has acme, whose base has diverged, and beta; acme also has a Ticket
// In Review unless alone.
async function world(alone = false) {
  const w = await newWorld();
  await allClear(w);
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("beta", "Beta");
  if (!alone) await s.ticket("acme", { title: "Login form", state: "In Review" });
  await s.base("acme", "diverged");
  return w;
}

const item = () =>
  within(focus()).findByRole("article", { name: "Focus Item" });

describe("Base diverged", () => {
  it("is a one-line detail with both commits", async () => {
    const groups = (await (await world()).api.needsMe()).groups;
    const row = groups.find((g) => g.reason === "base_diverged")!.tickets[0];
    expect(rowDetail(new Date(), row)).toBe(
      "main is at aaa1111 and the remote's at bbb2222: reconcile them by hand",
    );
  });

  it("is a Focus Item for the Project with the commits and the fix, and no Ticket panel", async () => {
    await renderApp(await world(true));
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

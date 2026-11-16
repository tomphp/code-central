// CI status badge tests (code-central-259): the real App on a WASM Core
// world. A Ticket with a CI result shows it as a badge on its rows in the
// Spec view, Needs Me and the Activity Dashboard, and one with none shows
// none; the Spec view sums its Tickets' badges per result.
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { newWorld } from "../api/core/core";
import type { CIOutcome, Scenario } from "../api/scenario";
import { renderApp, scenario } from "../worldTesting";
import { ListCIBadge } from "./CIBadge";

const detail = () => screen.getByRole("complementary", { name: "Ticket" });
const focus = () => screen.getByRole("main", { name: "Focus Mode" });

// badgeOf is the row's CI badge text, or null when it has none.
const badgeOf = (row: HTMLElement) => row.querySelector(".ci-badge")?.textContent ?? null;

// accepted makes an Accepted Ticket under spec whose pushed commit has the
// CI outcome given, or was never pushed for undefined.
async function accepted(s: Scenario, title: string, outcome: CIOutcome | undefined, spec?: string) {
  const id = await s.ticket("acme", { title, state: outcome ? "Ready" : "Accepted", spec });
  if (outcome) {
    await s.accepted("acme", id, { branch: id, commit: `c-${id}` });
    await s.pushed("acme", id, `c-${id}`);
    if (outcome !== "running") await s.ci("acme", id, outcome, `c-${id}`, Number(id.split("-")[1]));
  }
  return id;
}

// theming is a plain Spec whose Tickets have the CI results given, in
// order; undefined is a Ticket with none.
async function theming(results: (CIOutcome | undefined)[]) {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("spare", "Spare"); // a second active Project, so All Projects stays offered
  const spec = await s.spec("acme", { title: "Theming" });
  for (const [i, outcome] of results.entries()) await accepted(s, `Palette ${i + 1}`, outcome, spec);
  return { w, s, spec };
}

async function acme() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("spare", "Spare"); // a second active Project, so All Projects stays offered
  return { w, s };
}

describe("the CI badge", () => {
  it("is on a Ticket's row in the Spec view when it has a CI result, and not when it has none", async () => {
    const { w, spec } = await theming(["failed", "running", "passed", undefined]);
    await renderApp(w, `/?view=inbox&requirement=${spec}`);

    const pane = within(detail()).getByRole("region", { name: "Spec" });
    const rows = within(within(pane).getByRole("list", { name: "Tickets" })).getAllByRole("listitem");
    expect(rows.map(badgeOf)).toEqual(["CI failed", "CI running", "CI passed", null]);
  });

  it("shows a cancelled result in grey and a failed one in red", async () => {
    const { w, spec } = await theming(["failed", "cancelled"]);
    await renderApp(w, `/?view=inbox&requirement=${spec}`);

    const pane = within(detail()).getByRole("region", { name: "Spec" });
    const rows = within(within(pane).getByRole("list", { name: "Tickets" })).getAllByRole("listitem");
    expect(rows.map(badgeOf)).toEqual(["CI failed", "CI cancelled"]);
    expect(rows[0].querySelector(".ci-badge")).toHaveClass("ci-failed");
    expect(rows[1].querySelector(".ci-badge")).toHaveClass("ci-cancelled");
    expect(within(pane).getByText("1 cancelled")).toHaveClass("ci-cancelled");
  });

  it("is on a Ticket's row in Needs Me when it has a CI result, and not when it has none", async () => {
    const { w, s } = await acme();
    const failed = await accepted(s, "Login page", "failed");
    const none = await s.ticket("acme", { title: "Sign-up page", state: "In Review" });
    await renderApp(w, "/?view=inbox");

    const list = screen.getByRole("main", { name: "Tickets" });
    expect(badgeOf(within(list).getByRole("link", { name: new RegExp(failed) }))).toBe("CI failed");
    expect(badgeOf(within(list).getByRole("link", { name: new RegExp(none) }))).toBeNull();
  });

  it("is on a Ticket's entry in the Activity Dashboard when it has a CI result, and not when it has none", async () => {
    const { w, s } = await acme();
    // Sent back to fix its CI: In Progress again with its last push's result.
    // A Long-lived Spec's Tickets land one by one, so this one has a branch to send back.
    const spec = await s.spec("acme", { title: "Small Changes", labels: ["long-lived"] });
    const failed = await accepted(s, "Login page", "failed", spec);
    await w.api.act("acme", failed, "send_back", { note: "Fix the CI" });
    await s.claimFor("acme", failed);
    await s.ticket("acme", { title: "Sign-up page", state: "In Progress", spec });
    await renderApp(w, "/?view=focus");

    const d = within(focus()).getByRole("region", { name: "Activity Dashboard" });
    const entries = within(within(d).getByRole("list", { name: "In Progress" })).getAllByRole("listitem");
    expect(entries.map(badgeOf)).toEqual(["CI failed", null]);
  });
});

describe("the Spec view's CI sums", () => {
  it("counts its Tickets' badges per result", async () => {
    const { w, spec } = await theming(["failed", "passed", "passed", "running", undefined, "passed"]);
    await renderApp(w, `/?view=inbox&requirement=${spec}`);

    const pane = within(detail()).getByRole("region", { name: "Spec" });
    const sums = within(pane).getByRole("group", { name: "CI results" });
    expect([...sums.querySelectorAll(".ci-result")].map((s) => s.textContent)).toEqual(["1 failed", "1 running", "3 passed"]);
  });

  it("leaves out a result no Ticket has", async () => {
    const { w, spec } = await theming(["passed", undefined]);
    await renderApp(w, `/?view=inbox&requirement=${spec}`);

    const pane = within(detail()).getByRole("region", { name: "Spec" });
    expect(within(pane).getByRole("group", { name: "CI results" })).toHaveTextContent(/^CI1 passed$/);
  });

  it("isn't shown when no Ticket has a CI result", async () => {
    const { w, spec } = await theming([undefined, undefined]);
    await renderApp(w, `/?view=inbox&requirement=${spec}`);

    const pane = within(detail()).getByRole("region", { name: "Spec" });
    expect(within(pane).queryByRole("group", { name: "CI results" })).toBeNull();
  });

  it("is on a Requirement's view too, over its Build Tickets", async () => {
    const { w, s } = await acme();
    const { spec } = await w.api.submitRequirement("acme", "Dark mode");
    await accepted(s, "Dark palette", "failed", spec.id);
    await renderApp(w, `/?view=inbox&requirement=${spec.id}`);

    const pane = within(detail()).getByRole("region", { name: "Requirement" });
    expect(within(pane).getByRole("group", { name: "CI results" })).toHaveTextContent("1 failed");
    const build = within(within(pane).getByRole("list", { name: "Build" })).getAllByRole("listitem");
    expect(build.map(badgeOf)).toEqual(["CI failed"]);
  });
});

describe("ListCIBadge", () => {
  it("shows nothing for a Done Ticket or one with no CI result, and the badge otherwise", () => {
    const { container, rerender } = render(<ListCIBadge t={{ state: "Done", ci_result: "passed" }} />);
    expect(container).toBeEmptyDOMElement();
    rerender(<ListCIBadge t={{ state: "Accepted" }} />);
    expect(container).toBeEmptyDOMElement();
    rerender(<ListCIBadge t={{ state: "Accepted", ci_result: "running" }} />);
    expect(container).toHaveTextContent("CI running");
  });
});

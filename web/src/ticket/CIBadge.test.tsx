// CI status badge tests (code-central-259): the real App against FakeApi.
// A Ticket with a CI result shows it as a badge on its rows in the Spec
// view, Needs Me and the Activity Dashboard, and one with none shows none;
// the Spec view sums its Tickets' badges per result.
import { screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { FakeApi } from "../api/fake";
import type { CIResult } from "../api/types";
import { focus, renderApp } from "../focus/focusTesting";

const detail = () => screen.getByRole("complementary", { name: "Ticket" });

// badgeOf is the row's CI badge text, or null when it has none.
const badgeOf = (row: HTMLElement) => row.querySelector(".ci-badge")?.textContent ?? null;

// theming is a plain Spec, acme-1, whose Tickets have the CI results given,
// in order from acme-2; undefined is a Ticket with none.
function theming(results: (CIResult | undefined)[]) {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addSpec("acme", { title: "Theming" });
  results.forEach((ci_result, i) =>
    fake.addTicket("acme", { title: `Palette ${i + 1}`, state: "Accepted", spec: { id: "acme-1", title: "Theming" }, ...(ci_result && { ci_result }) }),
  );
  return fake;
}

describe("the CI badge", () => {
  it("is on a Ticket's row in the Spec view when it has a CI result, and not when it has none", async () => {
    renderApp(theming(["failed", "running", "passed", undefined]), "/?view=inbox&requirement=acme-1");

    const pane = await within(detail()).findByRole("region", { name: "Spec" });
    const rows = within(within(pane).getByRole("list", { name: "Tickets" })).getAllByRole("listitem");
    expect(rows.map(badgeOf)).toEqual(["CI failed", "CI running", "CI passed", null]);
  });

  it("shows a cancelled result in grey and a failed one in red", async () => {
    renderApp(theming(["failed", "cancelled"]), "/?view=inbox&requirement=acme-1");

    const pane = await within(detail()).findByRole("region", { name: "Spec" });
    const rows = within(within(pane).getByRole("list", { name: "Tickets" })).getAllByRole("listitem");
    expect(rows.map(badgeOf)).toEqual(["CI failed", "CI cancelled"]);
    expect(rows[0].querySelector(".ci-badge")).toHaveClass("ci-failed");
    expect(rows[1].querySelector(".ci-badge")).toHaveClass("ci-cancelled");
    expect(within(pane).getByText("1 cancelled")).toHaveClass("ci-cancelled");
  });

  it("is on a Ticket's row in Needs Me when it has a CI result, and not when it has none", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    fake.addTicket("acme", { title: "Login page", state: "Accepted", ci_result: "failed" }); // acme-1
    fake.addTicket("acme", { title: "Sign-up page", state: "Accepted" }); // acme-2
    fake.setNeedsMe([{ reason: "to_merge", tickets: [{ id: "acme-1" }, { id: "acme-2" }] }]);
    renderApp(fake, "/?view=inbox");

    const list = screen.getByRole("main", { name: "Tickets" });
    expect(badgeOf(await within(list).findByRole("link", { name: /acme-1/ }))).toBe("CI failed");
    expect(badgeOf(within(list).getByRole("link", { name: /acme-2/ }))).toBeNull();
  });

  it("is on a Ticket's entry in the Activity Dashboard when it has a CI result, and not when it has none", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    // Sent back to fix its CI: In Progress again with its last push's result.
    fake.addTicket("acme", { title: "Login page", state: "In Progress", ci_result: "failed" }); // acme-1
    fake.addTicket("acme", { title: "Sign-up page", state: "In Progress" }); // acme-2
    renderApp(fake);

    const d = await within(focus()).findByRole("region", { name: "Activity Dashboard" });
    const entries = within(within(d).getByRole("list", { name: "In Progress" })).getAllByRole("listitem");
    expect(entries.map(badgeOf)).toEqual(["CI failed", null]);
  });
});

describe("the Spec view's CI sums", () => {
  it("counts its Tickets' badges per result", async () => {
    renderApp(theming(["failed", "passed", "passed", "running", undefined, "passed"]), "/?view=inbox&requirement=acme-1");

    const pane = await within(detail()).findByRole("region", { name: "Spec" });
    const sums = within(pane).getByRole("group", { name: "CI results" });
    expect([...sums.querySelectorAll(".ci-result")].map((s) => s.textContent)).toEqual(["1 failed", "1 running", "3 passed"]);
  });

  it("leaves out a result no Ticket has", async () => {
    renderApp(theming(["passed", undefined]), "/?view=inbox&requirement=acme-1");

    const pane = await within(detail()).findByRole("region", { name: "Spec" });
    expect(within(pane).getByRole("group", { name: "CI results" })).toHaveTextContent(/^CI1 passed$/);
  });

  it("isn't shown when no Ticket has a CI result", async () => {
    renderApp(theming([undefined, undefined]), "/?view=inbox&requirement=acme-1");

    const pane = await within(detail()).findByRole("region", { name: "Spec" });
    expect(within(pane).queryByRole("group", { name: "CI results" })).toBeNull();
  });

  it("is on a Requirement's view too, over its Build Tickets", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    await fake.submitRequirement("acme", "Dark mode"); // acme-1, Stages acme-2 to acme-4
    const spec = { id: "acme-1", title: "Dark mode" };
    fake.addTicket("acme", { title: "Dark palette", state: "Accepted", spec, ci_result: "failed" }); // acme-5
    renderApp(fake, "/?view=inbox&requirement=acme-1");

    const pane = await within(detail()).findByRole("region", { name: "Requirement" });
    expect(within(pane).getByRole("group", { name: "CI results" })).toHaveTextContent("1 failed");
    const build = within(within(pane).getByRole("list", { name: "Build" })).getAllByRole("listitem");
    expect(build.map(badgeOf)).toEqual(["CI failed"]);
  });
});

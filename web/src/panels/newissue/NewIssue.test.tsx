// New issue and Keep as Draft (code-central-287) against FakeApi: a New
// issue's row in Needs Me, its Focus Item, its action panel, and the
// triage actions in the Ticket panel of every Incoming Issue.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { FakeApi } from "../../api/fake";
import { change, focus, renderApp, without } from "../../focus/focusTesting";
import { TicketDetail } from "../../ticket/TicketDetail";
import { panel } from "./panel";

const ISSUE = { integration: "github", id: "acme/web#7", url: "https://github.com/acme/web/issues/7" };

// world has GitHub Issues on for acme, its Issues Spec acme-1, with New
// issues need me as needMe, and issue 7 come in as acme-2.
async function world(needMe = true) {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  const on = await fake.setGitHubIssues("acme", { on: true, repository: "acme/web", label: "", new_issues_need_me: needMe });
  fake.addIncomingIssue("acme", on.github_issues.spec!, "Login is broken", ISSUE); // acme-2
  return fake;
}

async function renderPanel(fake: FakeApi) {
  const t = await fake.getTicket("acme", "acme-2");
  render(<panel.Component api={fake} project="acme" ticket={t} now={fake.now} />);
  return screen.getByRole("region", { name: "New issue" });
}

async function renderDetail(fake: FakeApi, id = "acme-2") {
  const t = await fake.getTicket("acme", id);
  render(<TicketDetail api={fake} project="acme" ticket={t} now={fake.now} />);
  return screen.getByRole("article", { name: "Ticket details" });
}

describe("a New issue in Needs Me", () => {
  it("is listed under New issue, naming its issue", async () => {
    renderApp(await world(), "/?view=inbox");
    const group = await screen.findByRole("region", { name: "New issue" });
    expect(within(group).getByRole("link", { name: /acme-2/ })).toHaveTextContent("github acme/web#7 came in just now");
  });

  it("has the New issue panel as its action panel", async () => {
    renderApp(await world(), "/?view=inbox&ticket=acme-2");
    const slot = await screen.findByRole("region", { name: "Action panel" });
    expect(await within(slot).findByRole("region", { name: "New issue" })).toBeInTheDocument();
    // The triage actions are in the panel, so the Ticket details don't
    // repeat them.
    expect(within(screen.getByRole("article", { name: "Ticket details" })).queryByRole("region", { name: "Triage" })).toBeNull();
  });

  it("links to the issue and offers Keep as Draft", async () => {
    const p = await renderPanel(await world());
    expect(within(p).getByRole("link", { name: "github acme/web#7" })).toHaveAttribute("href", ISSUE.url);
    expect(within(p).getByRole("button", { name: "Keep as Draft" })).toBeInTheDocument();
  });

  it("keeps it as a Draft, out of Needs Me", async () => {
    const fake = await world();
    const p = await renderPanel(fake);
    await userEvent.click(within(p).getByRole("button", { name: "Keep as Draft" }));
    expect(fake.calls).toContain("keep_as_draft");
    const t = await fake.getTicket("acme", "acme-2");
    expect(t).toMatchObject({ state: "Draft", triage_actions: ["keep_as_draft"] });
    expect(t.awaiting_triage).toBeUndefined();
    expect(t.needs_me_reason).toBeUndefined();
    expect((await fake.needsMe()).count).toBe(0);
  });
});

describe("a New issue in Focus Mode", () => {
  it("is a Focus Item with its panel, and Keep as Draft moves on", async () => {
    const fake = await world();
    const closed = { ...ISSUE, id: "acme/web#8", external_state: "closed", detached: false } as const;
    const t = fake.addTicket("acme", { title: "Gone upstream", state: "Draft", link: closed }); // acme-3
    fake.setNeedsMe([...fake.needsMeGroups, { reason: "closed_externally", tickets: [{ id: t.id, link: t.link }] }]);
    renderApp(fake);
    expect(await within(focus()).findByRole("heading", { name: "Login is broken" })).toBeInTheDocument();
    const item = await within(focus()).findByRole("region", { name: "New issue" });
    await userEvent.click(within(item).getByRole("button", { name: "Keep as Draft" }));
    // The service sends its notice.
    change(fake, without(fake, "acme-2"), "acme-2");
    expect(await within(focus()).findByRole("heading", { name: "Gone upstream" })).toBeInTheDocument();
  });
});

describe("the triage actions in the Ticket panel", () => {
  it("are offered on an Incoming Issue that isn't awaiting triage", async () => {
    const fake = await world(false);
    const d = await renderDetail(fake);
    const triage = within(d).getByRole("region", { name: "Triage" });
    await userEvent.click(within(triage).getByRole("button", { name: "Keep as Draft" }));
    expect(fake.calls).toContain("keep_as_draft");
  });

  it("are not offered on any other Ticket", async () => {
    const fake = await world();
    fake.addTicket("acme", { title: "Local", state: "Draft" }); // acme-3
    const d = await renderDetail(fake, "acme-3");
    expect(within(d).queryByRole("region", { name: "Triage" })).toBeNull();
  });

  it("shows a refusal", async () => {
    const fake = await world(false);
    const d = await renderDetail(fake);
    await fake.unlink("acme", "acme-2"); // the Ticket shown is stale
    await userEvent.click(within(d).getByRole("button", { name: "Keep as Draft" }));
    expect(await within(d).findByRole("alert")).toHaveTextContent("not an Incoming Issue");
  });
});

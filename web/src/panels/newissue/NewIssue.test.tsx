// New issue, Keep as Draft (code-central-287), Won't fix
// (code-central-289) and To Requirement (code-central-290) against
// FakeApi: a New issue's row in Needs Me, its
// Focus Item, its action panel, and the triage actions in the Ticket
// panel of every Incoming Issue.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { FakeApi } from "../../api/fake";
import { TRIAGE_ACTIONS } from "../../api/types";
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
    expect(t).toMatchObject({ state: "Draft", triage_actions: TRIAGE_ACTIONS });
    expect(t.awaiting_triage).toBeUndefined();
    expect(t.needs_me_reason).toBeUndefined();
    expect((await fake.needsMe()).count).toBe(0);
  });
});

// wontFix opens Won't fix's reason box in within, types reason into it,
// if any, and confirms it.
async function wontFix(within_: HTMLElement, reason = "") {
  await userEvent.click(within(within_).getByRole("button", { name: "Won't fix…" }));
  const form = within(within_).getByRole("form", { name: "Won't fix" });
  if (reason) await userEvent.type(within(form).getByRole("textbox", { name: /Reason/ }), reason);
  await userEvent.click(within(form).getByRole("button", { name: "Won't fix" }));
}

describe("Won't fix on a New issue", () => {
  it("Cancels it with the reason as its comment, out of Needs Me", async () => {
    const fake = await world();
    const p = await renderPanel(fake);
    await wontFix(p, "Works as intended.");
    expect(fake.calls).toContain("wont_fix");
    const t = await fake.getTicket("acme", "acme-2");
    expect(t).toMatchObject({ state: "Cancelled" });
    expect(t.comments.at(-1)).toMatchObject({ actor: "human", body: "Works as intended." });
    expect(t.moves.at(-1)).toMatchObject({ to: "Cancelled", via: "wont_fix" });
    expect(t.awaiting_triage).toBeUndefined();
    expect(t.triage_actions).toBeUndefined();
    expect((await fake.needsMe()).count).toBe(0);
  });

  it("needs no reason", async () => {
    const fake = await world();
    const p = await renderPanel(fake);
    await wontFix(p);
    const t = await fake.getTicket("acme", "acme-2");
    expect(t.state).toBe("Cancelled");
    expect(t.comments).toEqual([]);
  });

  it("can be backed out of before confirming", async () => {
    const fake = await world();
    const p = await renderPanel(fake);
    await userEvent.click(within(p).getByRole("button", { name: "Won't fix…" }));
    await userEvent.click(within(p).getByRole("button", { name: "Back" }));
    expect(within(p).queryByRole("form", { name: "Won't fix" })).toBeNull();
    expect(fake.calls).not.toContain("wont_fix");
  });
});

// toRequirement opens To Requirement's context box in within, types
// context into it, if any, and confirms it.
async function toRequirement(within_: HTMLElement, context = "") {
  await userEvent.click(within(within_).getByRole("button", { name: "To Requirement…" }));
  const form = within(within_).getByRole("form", { name: "To Requirement" });
  if (context) await userEvent.type(within(form).getByRole("textbox", { name: /Context/ }), context);
  await userEvent.click(within(form).getByRole("button", { name: "To Requirement" }));
}

// requirementOf is the Requirement To Requirement made of acme-2: its
// Spec, through the grilling Stage that replaced it.
async function requirementOf(fake: FakeApi) {
  const t = await fake.getTicket("acme", "acme-2");
  const grilling = await fake.getTicket("acme", t.replacements[0]);
  return fake.getSpec("acme", grilling.spec.id);
}

describe("To Requirement on a New issue", () => {
  it("makes a Requirement of it with the context, Cancelling it, out of Needs Me", async () => {
    const fake = await world();
    const p = await renderPanel(fake);
    await toRequirement(p, "Seen on every login.");
    expect(fake.calls).toContain("to_requirement");
    const t = await fake.getTicket("acme", "acme-2");
    expect(t).toMatchObject({ state: "Cancelled", link: { detached: true } });
    expect(t.moves.at(-1)).toMatchObject({ to: "Cancelled", via: "to_requirement" });
    expect(t.triage_actions).toBeUndefined();
    expect((await fake.needsMe()).count).toBe(0);
    const sp = await requirementOf(fake);
    expect(sp).toMatchObject({ title: "Login is broken", body: "# Login is broken\n\n## Context\n\nSeen on every login.\n" });
    expect(sp.link).toMatchObject({ id: "acme/web#7", outbound: true, detached: false });
  });

  it("needs no context", async () => {
    const fake = await world();
    await toRequirement(await renderPanel(fake));
    expect((await requirementOf(fake)).body).toBe("# Login is broken\n");
    expect((await fake.getTicket("acme", "acme-2")).comments).toEqual([]);
  });

  it("can be backed out of before confirming", async () => {
    const fake = await world();
    const p = await renderPanel(fake);
    await userEvent.click(within(p).getByRole("button", { name: "To Requirement…" }));
    expect(within(p).queryByRole("button", { name: "Won't fix…" })).toBeNull();
    await userEvent.click(within(p).getByRole("button", { name: "Back" }));
    expect(within(p).queryByRole("form", { name: "To Requirement" })).toBeNull();
    expect(within(p).getByRole("button", { name: "Won't fix…" })).toBeInTheDocument();
    expect(fake.calls).not.toContain("to_requirement");
  });
});

describe("a New issue in Focus Mode", () => {
  it("offers To Requirement with its context box, and moves on", async () => {
    const fake = await world();
    fake.addTicket("acme", { title: "Next up", state: "Draft" }); // acme-3
    fake.setNeedsMe([...fake.needsMeGroups, { reason: "closed_externally", tickets: [{ id: "acme-3" }] }]);
    renderApp(fake);
    const item = await within(focus()).findByRole("region", { name: "New issue" });
    await toRequirement(item, "Urgent.");
    expect((await fake.getTicket("acme", "acme-2")).comments.at(-1)?.body).toBe("Urgent.");
    change(fake, without(fake, "acme-2"), "acme-2");
    expect(await within(focus()).findByRole("heading", { name: "Next up" })).toBeInTheDocument();
  });

  it("offers Won't fix with its reason box, and moves on", async () => {
    const fake = await world();
    fake.addTicket("acme", { title: "Next up", state: "Draft" }); // acme-3
    const g = fake.needsMeGroups;
    fake.setNeedsMe([...g, { reason: "closed_externally", tickets: [{ id: "acme-3" }] }]);
    renderApp(fake);
    const item = await within(focus()).findByRole("region", { name: "New issue" });
    await wontFix(item, "Not planned.");
    expect((await fake.getTicket("acme", "acme-2")).comments.at(-1)?.body).toBe("Not planned.");
    change(fake, without(fake, "acme-2"), "acme-2");
    expect(await within(focus()).findByRole("heading", { name: "Next up" })).toBeInTheDocument();
  });

  it("is a Focus Item with its panel, and Keep as Draft moves on", async () => {
    const fake = await world();
    const closed = { ...ISSUE, id: "acme/web#8", external_state: "closed", detached: false, outbound: false } as const;
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

  it("include Won't fix with its reason box", async () => {
    const fake = await world(false);
    const d = await renderDetail(fake);
    await wontFix(within(d).getByRole("region", { name: "Triage" }), "Duplicate of #3.");
    const t = await fake.getTicket("acme", "acme-2");
    expect(t.state).toBe("Cancelled");
    expect(t.comments.at(-1)?.body).toBe("Duplicate of #3.");
  });

  it("include To Requirement with its context box", async () => {
    const fake = await world(false);
    const d = await renderDetail(fake);
    await toRequirement(within(d).getByRole("region", { name: "Triage" }), "From the support queue.");
    expect((await fake.getTicket("acme", "acme-2")).state).toBe("Cancelled");
    expect((await requirementOf(fake)).body).toContain("## Context\n\nFrom the support queue.\n");
  });

  it("show Won't fix's refusal", async () => {
    const fake = await world(false);
    const d = await renderDetail(fake);
    await fake.unlink("acme", "acme-2"); // the Ticket shown is stale
    await wontFix(d);
    expect(await within(d).findByRole("alert")).toHaveTextContent("was unlinked from github acme/web#7");
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

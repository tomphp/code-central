// New issue, Keep as Draft (code-central-287), Won't fix
// (code-central-289) and To Requirement (code-central-290) on the WASM
// Core: a New issue's row in Needs Me, its
// Focus Item, its action panel, and the triage actions in the Ticket
// panel of every Incoming Issue.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { newWorld, type World } from "../../api/core/core";
import type { Scenario } from "../../api/scenario";
import { TRIAGE_ACTIONS } from "../../api/types";
import { TicketDetail } from "../../ticket/TicketDetail";
import { renderApp, renderLive, scenario, settle } from "../../worldTesting";
import { panel } from "./panel";

const ISSUE = { integration: "github", id: "acme/web#7", url: "https://github.com/acme/web/issues/7" };

// world has GitHub Issues on for acme, its Issues Spec, with New issues
// need me as needMe, and issue 7 come in as issue.
async function world(needMe = true) {
  const w = await newWorld();
  const s = scenario(w);
  await s.createProject("acme", "Acme");
  await s.createProject("spare", "Spare"); // a second active Project, so All Projects stays offered
  const on = await w.api.setGitHubIssues("acme", { on: true, repositories: ["acme/web"], label: "", new_issues_need_me: needMe });
  const issue = await s.incomingIssue("acme", on.github_issues.spec!, "Login is broken", ISSUE);
  return { w, s, issue };
}

type Issues = Awaited<ReturnType<typeof world>>;

// nextUp has the human's next Focus Item behind the issue: a Draft whose
// issue closed upstream.
function nextUp(s: Scenario, title: string, number = 8) {
  return s.linkTicket("acme", title, { ...ISSUE, id: `acme/web#${number}`, url: `https://github.com/acme/web/issues/${number}` }, "closed");
}

const focus = () => screen.getByRole("main", { name: "Focus Mode" });

async function renderPanel({ w, issue }: Issues) {
  const t = await w.api.getTicket("acme", issue);
  await renderLive(w, <panel.Component project="acme" ticket={t} now={w.now()} />);
  return screen.getByRole("region", { name: "New issue" });
}

async function renderDetail(w: World, id: string) {
  const t = await w.api.getTicket("acme", id);
  await renderLive(w, <TicketDetail project="acme" ticket={t} now={w.now()} />);
  return screen.getByRole("article", { name: "Ticket details" });
}

describe("a New issue in Needs Me", () => {
  it("is listed under New issue, naming its issue", async () => {
    const { w, issue } = await world();
    await renderApp(w, "/?view=inbox");
    const group = screen.getByRole("region", { name: "New issue" });
    expect(within(group).getByRole("link", { name: new RegExp(issue) })).toHaveTextContent("github acme/web#7 came in just now");
  });

  it("has the New issue panel as its action panel", async () => {
    const { w, issue } = await world();
    await renderApp(w, `/?view=inbox&ticket=${issue}`);
    const slot = screen.getByRole("region", { name: "Action panel" });
    expect(within(slot).getByRole("region", { name: "New issue" })).toBeInTheDocument();
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
    const x = await world();
    const p = await renderPanel(x);
    await userEvent.click(within(p).getByRole("button", { name: "Keep as Draft" }));
    expect(x.w.calls).toContain("keep_as_draft");
    const t = await x.w.api.getTicket("acme", x.issue);
    expect(t).toMatchObject({ state: "Draft", triage_actions: TRIAGE_ACTIONS });
    expect(t.awaiting_triage).toBeUndefined();
    expect(t.needs_me_reason).toBeUndefined();
    expect((await x.w.api.needsMe()).count).toBe(0);
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
    const x = await world();
    const p = await renderPanel(x);
    await wontFix(p, "Works as intended.");
    expect(x.w.calls).toContain("wont_fix");
    const t = await x.w.api.getTicket("acme", x.issue);
    expect(t).toMatchObject({ state: "Cancelled" });
    expect(t.comments!.at(-1)).toMatchObject({ actor: "human", body: "Works as intended." });
    expect(t.moves!.at(-1)).toMatchObject({ to: "Cancelled", via: "wont_fix" });
    expect(t.awaiting_triage).toBeUndefined();
    expect(t.triage_actions).toBeUndefined();
    expect((await x.w.api.needsMe()).count).toBe(0);
  });

  it("needs no reason", async () => {
    const x = await world();
    const before = (await x.w.api.getTicket("acme", x.issue)).comments;
    const p = await renderPanel(x);
    await wontFix(p);
    const t = await x.w.api.getTicket("acme", x.issue);
    expect(t.state).toBe("Cancelled");
    expect(t.comments).toEqual(before);
  });

  it("can be backed out of before confirming", async () => {
    const x = await world();
    const p = await renderPanel(x);
    await userEvent.click(within(p).getByRole("button", { name: "Won't fix…" }));
    await userEvent.click(within(p).getByRole("button", { name: "Back" }));
    expect(within(p).queryByRole("form", { name: "Won't fix" })).toBeNull();
    expect(x.w.calls).not.toContain("wont_fix");
  });

  it("says it closes the GitHub issue as not planned", async () => {
    const p = await renderPanel(await world());
    await userEvent.click(within(p).getByRole("button", { name: "Won't fix…" }));
    expect(within(p).getByRole("form", { name: "Won't fix" })).toHaveTextContent("closes the issue on GitHub as not planned");
  });
});

// boardsWorld has Azure Boards on for acme, its Azure Boards Spec, with
// New issues need me on, and work item 4512 come in.
async function boardsWorld() {
  const w = await newWorld();
  const s = scenario(w);
  await s.createProject("acme", "Acme");
  const on = await w.api.setAzureBoards("acme", {
    on: true,
    mode: "saved_query",
    query_url: "https://dev.azure.com/contoso/web/_queries/query/6b4a0c1e-9f1d-4a51-8f43-2a7d3c1b9e10",
    project_url: "",
    where: "",
    pat_env: "",
    new_issues_need_me: true,
    hide_code_central: false,
    branch_format: "",
  });
  const issue = await s.incomingIssue("acme", on.azure_boards.spec!, "Login is broken", {
    integration: "azure-boards",
    id: "contoso#4512",
    url: "https://dev.azure.com/contoso/web/_workitems/edit/4512",
  });
  return { w, s, issue };
}

describe("a New issue from Azure Boards", () => {
  it("names Azure Boards and adds nothing to the work item on Keep as Draft", async () => {
    const p = await renderPanel(await boardsWorld());
    expect(p).toHaveTextContent("came in from Azure Boards");
    expect(p).toHaveTextContent("adds nothing to the work item in Azure Boards");
    expect(p).not.toHaveTextContent("GitHub");
  });

  it("says Won't fix leaves the work item's State for the human", async () => {
    const p = await renderPanel(await boardsWorld());
    await userEvent.click(within(p).getByRole("button", { name: "Won't fix…" }));
    const form = within(p).getByRole("form", { name: "Won't fix" });
    expect(form).toHaveTextContent("code-central:wont-fix");
    expect(form).toHaveTextContent("leaves the work item's State as it is");
    expect(form).not.toHaveTextContent("closes");
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

// requirementOf is the Requirement To Requirement made of the issue: its
// Spec, through the grilling Stage that replaced it.
async function requirementOf({ w, issue }: Issues) {
  const t = await w.api.getTicket("acme", issue);
  const grilling = await w.api.getTicket("acme", t.replacements[0]);
  return w.api.getSpec("acme", grilling.spec.id);
}

describe("To Requirement on a New issue", () => {
  it("makes a Requirement of it with the context, Cancelling it, out of Needs Me", async () => {
    const x = await world();
    const p = await renderPanel(x);
    await toRequirement(p, "Seen on every login.");
    expect(x.w.calls).toContain("to_requirement");
    const t = await x.w.api.getTicket("acme", x.issue);
    expect(t).toMatchObject({ state: "Cancelled", link: { detached: true } });
    expect(t.moves!.at(-1)).toMatchObject({ to: "Cancelled", via: "to_requirement" });
    expect(t.triage_actions).toBeUndefined();
    expect((await x.w.api.needsMe()).count).toBe(0);
    const sp = await requirementOf(x);
    expect(sp).toMatchObject({ title: "Login is broken", body: "# Login is broken\n\n## Context\n\nSeen on every login.\n" });
    expect(sp.link).toMatchObject({ id: "acme/web#7", outbound: true, detached: false });
  });

  it("needs no context", async () => {
    const x = await world();
    const before = (await x.w.api.getTicket("acme", x.issue)).comments;
    await toRequirement(await renderPanel(x));
    expect((await requirementOf(x)).body).toBe("# Login is broken\n");
    expect((await x.w.api.getTicket("acme", x.issue)).comments).toEqual(before);
  });

  it("can be backed out of before confirming", async () => {
    const x = await world();
    const p = await renderPanel(x);
    await userEvent.click(within(p).getByRole("button", { name: "To Requirement…" }));
    expect(within(p).queryByRole("button", { name: "Won't fix…" })).toBeNull();
    await userEvent.click(within(p).getByRole("button", { name: "Back" }));
    expect(within(p).queryByRole("form", { name: "To Requirement" })).toBeNull();
    expect(within(p).getByRole("button", { name: "Won't fix…" })).toBeInTheDocument();
    expect(x.w.calls).not.toContain("to_requirement");
  });
});

describe("a New issue in Focus Mode", () => {
  it("offers To Requirement with its context box, and moves on", async () => {
    const x = await world();
    await nextUp(x.s, "Next up");
    await renderApp(x.w, "/?view=focus");
    const item = within(focus()).getByRole("region", { name: "New issue" });
    await toRequirement(item, "Urgent.");
    await settle(x.w);
    expect((await x.w.api.getTicket("acme", x.issue)).comments!.at(-1)?.body).toBe("Urgent.");
    expect(within(focus()).getByRole("heading", { name: "Next up" })).toBeInTheDocument();
  });

  it("offers Won't fix with its reason box, and moves on", async () => {
    const x = await world();
    await nextUp(x.s, "Next up");
    await renderApp(x.w, "/?view=focus");
    const item = within(focus()).getByRole("region", { name: "New issue" });
    await wontFix(item, "Not planned.");
    await settle(x.w);
    expect((await x.w.api.getTicket("acme", x.issue)).comments!.at(-1)?.body).toBe("Not planned.");
    expect(within(focus()).getByRole("heading", { name: "Next up" })).toBeInTheDocument();
  });

  it("is a Focus Item with its panel, and Keep as Draft moves on", async () => {
    const x = await world();
    await nextUp(x.s, "Gone upstream");
    await renderApp(x.w, "/?view=focus");
    expect(within(focus()).getByRole("heading", { name: "Login is broken" })).toBeInTheDocument();
    const item = within(focus()).getByRole("region", { name: "New issue" });
    await userEvent.click(within(item).getByRole("button", { name: "Keep as Draft" }));
    await settle(x.w);
    expect(within(focus()).getByRole("heading", { name: "Gone upstream" })).toBeInTheDocument();
  });
});

describe("the triage actions in the Ticket panel", () => {
  it("are offered on an Incoming Issue that isn't awaiting triage", async () => {
    const { w, issue } = await world(false);
    const d = await renderDetail(w, issue);
    const triage = within(d).getByRole("region", { name: "Triage" });
    await userEvent.click(within(triage).getByRole("button", { name: "Keep as Draft" }));
    expect(w.calls).toContain("keep_as_draft");
  });

  it("include Won't fix with its reason box", async () => {
    const { w, issue } = await world(false);
    const d = await renderDetail(w, issue);
    await wontFix(within(d).getByRole("region", { name: "Triage" }), "Duplicate of #3.");
    const t = await w.api.getTicket("acme", issue);
    expect(t.state).toBe("Cancelled");
    expect(t.comments!.at(-1)?.body).toBe("Duplicate of #3.");
  });

  it("include To Requirement with its context box", async () => {
    const x = await world(false);
    const d = await renderDetail(x.w, x.issue);
    await toRequirement(within(d).getByRole("region", { name: "Triage" }), "From the support queue.");
    expect((await x.w.api.getTicket("acme", x.issue)).state).toBe("Cancelled");
    expect((await requirementOf(x)).body).toContain("## Context\n\nFrom the support queue.\n");
  });

  it("show Won't fix's refusal", async () => {
    const { w, issue } = await world(false);
    const d = await renderDetail(w, issue);
    await w.api.unlink("acme", issue); // the Ticket shown is stale
    await wontFix(d);
    await settle(w);
    expect(within(d).getByRole("alert")).toHaveTextContent("was unlinked from github acme/web#7");
  });

  it("are not offered on any other Ticket", async () => {
    const { w, s } = await world();
    const local = await s.ticket("acme", { title: "Local", state: "Draft" });
    const d = await renderDetail(w, local);
    expect(within(d).queryByRole("region", { name: "Triage" })).toBeNull();
  });

  it("shows a refusal", async () => {
    const { w, issue } = await world(false);
    const d = await renderDetail(w, issue);
    await w.api.unlink("acme", issue); // the Ticket shown is stale
    await userEvent.click(within(d).getByRole("button", { name: "Keep as Draft" }));
    await settle(w);
    expect(within(d).getByRole("alert")).toHaveTextContent("no longer an Incoming Issue to triage");
  });
});

// Open in GitHub (Spec code-central-279): every linked Spec's and
// Ticket's panel links to its external item, as its Link's URL, beside
// its Link badge, which marks an outbound Link (code-central-290), on a
// WASM Core world.
import { cleanup, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { newWorld, type World } from "../api/core/core";
import { scenario } from "../api/scenario";
import type { LinkView, SpecDetail } from "../api/types";
import { MapPane } from "../map/MapPane";
import { RequirementPane } from "../requirement/RequirementPane";
import { SpecPane } from "../spec/SpecPane";
import { renderLive } from "../worldTesting";
import { TicketDetail } from "./TicketDetail";

const URL = "https://github.com/acme/web/issues/7";
const link: LinkView = { integration: "github", id: "acme/web#7", url: URL, external_state: "open", detached: false, outbound: false };
const seed = { integration: link.integration, id: link.id, url: URL };

async function world() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  return { w, s };
}

async function ticketPanel(w: World, id: string) {
  await renderLive(w, <TicketDetail project="acme" ticket={await w.api.getTicket("acme", id)} now={w.now()} />);
  return screen.getByRole("article", { name: "Ticket details" });
}

const noLinks = { reasons: new Map(), select: () => {}, specLink: (id: string) => ({ href: `?requirement=${id}`, onClick: () => {} }) };

// specPane renders spec in the pane its kind uses.
async function specPane(w: World, spec: SpecDetail) {
  const now = w.now();
  if (spec.requirement) {
    await renderLive(w, <RequirementPane spec={{ ...spec, requirement: spec.requirement }} now={now} {...noLinks} />);
    return screen.getByRole("region", { name: "Requirement" });
  }
  if (spec.map) {
    await renderLive(w, <MapPane spec={{ ...spec, map: spec.map }} now={now} written={() => {}} {...noLinks} />);
    return screen.getByRole("region", { name: "Map" });
  }
  await renderLive(w, <SpecPane spec={spec} now={now} {...noLinks} />);
  return screen.getByRole("region", { name: "Spec" });
}

function expectOpenInGitHub(panel: HTMLElement, url = URL) {
  const a = within(panel).getByRole("link", { name: "Open in GitHub" });
  expect(a).toHaveAttribute("href", url);
  expect(a).toHaveAttribute("target", "_blank");
}

describe("Open in GitHub", () => {
  it("shows on a linked Ticket's panel, as its Link's URL", async () => {
    const { w, s } = await world();
    const id = await s.linkedTicket("acme", { title: "Issue", link: seed });
    expectOpenInGitHub(await ticketPanel(w, id));
  });

  it("stays on a Ticket whose Link is detached", async () => {
    const { w, s } = await world();
    const id = await s.linkedTicket("acme", { title: "Issue", link: seed });
    await w.api.unlink("acme", id);
    await w.api.act("acme", id, "cancel", {});
    expectOpenInGitHub(await ticketPanel(w, id));
  });

  it("is absent on an unlinked Ticket, and on one whose Link has no URL", async () => {
    const { w, s } = await world();
    const local = await s.ticket("acme", { title: "Local" });
    const noURL = await s.linkedTicket("acme", { title: "No URL", state: "Ready", link: { ...seed, url: undefined } });
    for (const id of [local, noURL]) {
      const panel = await ticketPanel(w, id);
      expect(within(panel).queryByRole("link", { name: /^Open in/ })).toBeNull();
      cleanup();
    }
  });

  it("names the Integration when it isn't GitHub", async () => {
    const { w, s } = await world();
    const id = await s.linkedTicket("acme", {
      title: "Bug",
      state: "Ready",
      link: { ...seed, integration: "jira", url: "https://acme.atlassian.net/browse/WEB-7" },
    });
    const a = within(await ticketPanel(w, id)).getByRole("link", { name: "Open in jira" });
    expect(a).toHaveAttribute("href", "https://acme.atlassian.net/browse/WEB-7");
  });

  it("shows on a linked Spec's pane", async () => {
    const { w, s } = await world();
    const id = await s.linkedSpec("acme", "Linked", seed);
    expectOpenInGitHub(await specPane(w,await w.api.getSpec("acme", id)));
  });

  it("is absent on an unlinked Spec's pane", async () => {
    const { w, s } = await world();
    const id = await s.spec("acme", { title: "GitHub Issues: acme/web" });
    expect(within(await specPane(w,await w.api.getSpec("acme", id))).queryByRole("link", { name: /^Open in/ })).toBeNull();
  });

  it("shows on a linked Requirement's pane and a linked Map's", async () => {
    const { w } = await world();
    const r = await w.api.submitRequirement("acme", "Export\n\nAs Markdown.");
    const m = await w.api.submitMap("acme", "Exports");
    for (const id of [r.spec.id, m.spec.id]) {
      const pane = await specPane(w,{ ...(await w.api.getSpec("acme", id)), link });
      expectOpenInGitHub(pane);
      cleanup();
    }
  });
});

describe("the Link badge on a Spec's pane", () => {
  it("marks the outbound Link of a Requirement made by To Requirement", async () => {
    const { w, s } = await world();
    await s.letGHTriage("acme/web");
    const on = await w.api.setGitHubIssues("acme", { on: true, repository: "acme/web", label: "", new_issues_need_me: false });
    const issue = await s.incomingIssue("acme", on.github_issues.spec!, "Login is broken", { integration: "github", id: "acme/web#7", url: URL });
    const grilling = (await w.api.getTicket("acme", (await w.api.act("acme", issue, "to_requirement", {})).id)).replacements[0];
    const pane = await specPane(w,await w.api.getSpec("acme", (await w.api.getTicket("acme", grilling)).spec.id));
    expect(within(pane).getByRole("link", { name: "github acme/web#7 (outbound)" })).toHaveAttribute("href", URL);
    expectOpenInGitHub(pane);
  });

  it("shows an owned Link plainly, and none on an unlinked Spec", async () => {
    const { w, s } = await world();
    const linked = await s.linkedSpec("acme", "Linked", seed);
    expect(within(await specPane(w,await w.api.getSpec("acme", linked))).getByRole("link", { name: "github acme/web#7" })).toBeInTheDocument();
    cleanup();
    const plain = await s.spec("acme", { title: "Local" });
    expect(within(await specPane(w,await w.api.getSpec("acme", plain))).queryByText(/github/)).toBeNull();
  });
});

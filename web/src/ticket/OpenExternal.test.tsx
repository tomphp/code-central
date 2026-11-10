// Open in GitHub (Spec code-central-279): every linked Spec's and
// Ticket's panel links to its external item, as its Link's URL, against
// FakeApi.
import { cleanup, render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { FakeApi } from "../api/fake";
import type { LinkView, SpecDetail } from "../api/types";
import { MapPane } from "../map/MapPane";
import { RequirementPane } from "../requirement/RequirementPane";
import { SpecPane } from "../spec/SpecPane";
import { TicketDetail } from "./TicketDetail";

const URL = "https://github.com/acme/web/issues/7";
const link: LinkView = { integration: "github", id: "acme/web#7", url: URL, external_state: "open", detached: false };

function world() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  return fake;
}

async function ticketPanel(fake: FakeApi, id: string) {
  render(<TicketDetail api={fake} project="acme" ticket={await fake.getTicket("acme", id)} now={fake.now} />);
  return screen.getByRole("article", { name: "Ticket details" });
}

const noLinks = { reasons: new Map(), select: () => {}, specLink: (id: string) => ({ href: `?requirement=${id}`, onClick: () => {} }) };

// specPane renders spec in the pane its kind uses.
function specPane(fake: FakeApi, spec: SpecDetail) {
  if (spec.requirement) {
    render(<RequirementPane api={fake} spec={{ ...spec, requirement: spec.requirement }} now={fake.now} {...noLinks} />);
    return screen.getByRole("region", { name: "Requirement" });
  }
  if (spec.map) {
    render(<MapPane api={fake} spec={{ ...spec, map: spec.map }} now={fake.now} written={() => {}} {...noLinks} />);
    return screen.getByRole("region", { name: "Map" });
  }
  render(<SpecPane api={fake} spec={spec} now={fake.now} {...noLinks} />);
  return screen.getByRole("region", { name: "Spec" });
}

function expectOpenInGitHub(panel: HTMLElement, url = URL) {
  const a = within(panel).getByRole("link", { name: "Open in GitHub" });
  expect(a).toHaveAttribute("href", url);
  expect(a).toHaveAttribute("target", "_blank");
}

describe("Open in GitHub", () => {
  it("shows on a linked Ticket's panel, as its Link's URL", async () => {
    const fake = world();
    fake.addTicket("acme", { title: "Issue", state: "Draft", link }); // acme-1
    expectOpenInGitHub(await ticketPanel(fake, "acme-1"));
  });

  it("stays on a Ticket whose Link is detached", async () => {
    const fake = world();
    fake.addTicket("acme", { title: "Issue", state: "Cancelled", link: { ...link, detached: true } });
    expectOpenInGitHub(await ticketPanel(fake, "acme-1"));
  });

  it("is absent on an unlinked Ticket, and on one whose Link has no URL", async () => {
    const fake = world();
    fake.addTicket("acme", { title: "Local", state: "Ready" }); // acme-1
    fake.addTicket("acme", { title: "No URL", state: "Ready", link: { ...link, url: undefined } }); // acme-2
    for (const id of ["acme-1", "acme-2"]) {
      const panel = await ticketPanel(fake, id);
      expect(within(panel).queryByRole("link", { name: /^Open in/ })).toBeNull();
      cleanup();
    }
  });

  it("names the Integration when it isn't GitHub", async () => {
    const fake = world();
    fake.addTicket("acme", { title: "Bug", state: "Ready", link: { ...link, integration: "jira", url: "https://acme.atlassian.net/browse/WEB-7" } });
    const a = within(await ticketPanel(fake, "acme-1")).getByRole("link", { name: "Open in jira" });
    expect(a).toHaveAttribute("href", "https://acme.atlassian.net/browse/WEB-7");
  });

  it("shows on a linked Spec's pane", async () => {
    const fake = world();
    const s = fake.addSpec("acme", { title: "Linked", link });
    expectOpenInGitHub(specPane(fake, await fake.getSpec("acme", s.id)));
  });

  it("is absent on an unlinked Spec's pane", async () => {
    const fake = world();
    const s = fake.addSpec("acme", { title: "GitHub Issues: acme/web" });
    expect(within(specPane(fake, await fake.getSpec("acme", s.id))).queryByRole("link", { name: /^Open in/ })).toBeNull();
  });

  it("shows on a linked Requirement's pane and a linked Map's", async () => {
    const fake = world();
    const r = await fake.submitRequirement("acme", "Export\n\nAs Markdown.");
    const m = await fake.submitMap("acme", "Exports");
    for (const id of [r.spec.id, m.spec.id]) {
      const pane = specPane(fake, { ...(await fake.getSpec("acme", id)), link });
      expectOpenInGitHub(pane);
      cleanup();
    }
  });
});

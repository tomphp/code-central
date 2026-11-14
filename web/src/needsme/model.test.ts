// The Needs Me read model's questions, over the Inbox data as loadInbox
// reads it from FakeApi: service-shaped, with no React.
import { describe, expect, it } from "vitest";
import { FakeApi } from "../api/fake";
import { loadInbox } from "../live";
import { failCI } from "../ciTesting";
import { isCIItemReason, needsMeOf } from "./model";

// twoProjects is acme with a plain Spec (acme-1) of two Tickets and a
// Requirement (acme-4, Stages acme-5 to acme-7), and beta with one Ticket.
async function twoProjects() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addProject("beta", "Beta");
  fake.addSpec("acme", { id: "acme-1", title: "Sign-in" });
  fake.addTicket("acme", { id: "acme-2", title: "Form", state: "In Review", spec: { id: "acme-1", title: "Sign-in" } });
  fake.addTicket("acme", { id: "acme-3", title: "Page", state: "Ready", spec: { id: "acme-1", title: "Sign-in" } });
  await fake.submitRequirement("acme", "Dark mode");
  fake.addTicket("beta", { id: "beta-1", title: "Docs", state: "Failed" });
  return fake;
}

describe("needsMeOf", () => {
  it("answers a Ticket's reason from its find_tickets row, and none for one that doesn't need the human", async () => {
    const fake = await twoProjects();
    fake.setNeedsMe([{ reason: "in_review", tickets: [{ id: "acme-2" }] }]);
    const m = needsMeOf(await loadInbox(fake));
    expect(m.reasonOf("acme-2")).toBe("in_review");
    expect(m.reasonOf("acme-3")).toBeUndefined();
    expect(m.reasonOf("acme-99")).toBeUndefined();
  });

  it("answers a Stage Ticket's reason and its Requirement's through it", async () => {
    const fake = await twoProjects();
    fake.setNeedsMe([{ reason: "needs_answer", tickets: [{ id: "acme-6" }] }]);
    const m = needsMeOf(await loadInbox(fake));
    expect(m.reasonOf("acme-6")).toBe("needs_answer");
    expect(m.reasonOf("acme-4")).toBe("needs_answer");
  });

  it("answers a Spec's reason as its first Ticket's that needs the human, Stages first", async () => {
    const fake = await twoProjects();
    fake.addTicket("acme", { id: "acme-8", title: "Build", state: "In Review", spec: { id: "acme-4", title: "Dark mode" } });
    fake.setNeedsMe([
      { reason: "in_review", tickets: [{ id: "acme-8" }, { id: "acme-3" }] },
      { reason: "failed", tickets: [{ id: "acme-7" }] },
    ]);
    const m = needsMeOf(await loadInbox(fake));
    expect(m.reasonOf("acme-1")).toBe("in_review");
    expect(m.reasonOf("acme-4")).toBe("failed");
  });

  it("answers the reason of each Ticket a CI item lists", async () => {
    const fake = await twoProjects();
    fake.updateTicket("acme-2", { state: "Accepted", branch: "acme-2", commit: "abc" });
    fake.updateTicket("acme-3", { state: "Accepted", branch: "acme-3", commit: "def" });
    await failCI(fake, "acme", ["acme-2", "acme-3"]);
    const m = needsMeOf(await loadInbox(fake));
    expect(m.reasonOf("acme-2")).toBe("ci_failed");
    expect(m.reasonOf("acme-3")).toBe("ci_failed");
  });

  it("counts core's count, a Project's counts entry, and 0 for a Project with nothing in Needs Me", async () => {
    const fake = await twoProjects();
    fake.setNeedsMe([
      { reason: "needs_answer", tickets: [{ id: "acme-5" }, { id: "acme-6" }] },
      { reason: "in_review", tickets: [{ id: "acme-2" }] },
    ]);
    const m = needsMeOf(await loadInbox(fake));
    expect(m.count()).toBe(2);
    expect(m.count("acme")).toBe(2);
    expect(m.count("beta")).toBe(0);
  });

  it("counts nothing before the data loads", () => {
    const m = needsMeOf(undefined);
    expect(m.count()).toBe(0);
    expect(m.reasonOf("acme-2")).toBeUndefined();
    expect(m.rows()).toEqual({ requirements: [], groups: [] });
  });

  it("lists the rows of the Project, a Requirement's Spec row in place of its Stage Tickets", async () => {
    const fake = await twoProjects();
    fake.setNeedsMe([
      { reason: "needs_answer", tickets: [{ id: "acme-5" }] },
      { reason: "in_review", tickets: [{ id: "acme-2" }] },
      { reason: "failed", tickets: [{ id: "beta-1" }] },
    ]);
    const m = needsMeOf(await loadInbox(fake));
    const all = m.rows();
    expect(all.requirements.map((s) => s.id)).toEqual(["acme-4"]);
    expect(all.groups.map((g) => [g.reason, g.tickets.map((r) => r.id)])).toEqual([
      ["in_review", ["acme-2"]],
      ["failed", ["beta-1"]],
    ]);
    const beta = m.rows("beta");
    expect(beta.requirements).toEqual([]);
    expect(beta.groups.map((g) => g.reason)).toEqual(["failed"]);
  });

  it("scopes the Tickets, Requirements and Maps to the Project, and every Spec's Tickets to their Spec", async () => {
    const fake = await twoProjects();
    const m = needsMeOf(await loadInbox(fake));
    expect(m.tickets("beta").map((t) => t.id)).toEqual(["beta-1"]);
    expect(m.tickets().length).toBe(6);
    expect(m.requirements("acme").map((s) => s.id)).toEqual(["acme-4"]);
    expect(m.requirements("beta")).toEqual([]);
    expect(m.maps()).toEqual([]);
    expect(m.ticketsOf("acme-1").map((t) => t.id)).toEqual(["acme-2", "acme-3"]);
    expect(m.ticketsOf("acme-99")).toEqual([]);
  });

  it("lists the Board's Needs Me column: Stage Tickets as themselves, in Needs Me's order, in the Project", async () => {
    const fake = await twoProjects();
    fake.setNeedsMe([
      { reason: "needs_answer", tickets: [{ id: "acme-6" }] },
      { reason: "in_review", tickets: [{ id: "acme-2" }] },
      { reason: "failed", tickets: [{ id: "beta-1" }] },
    ]);
    const m = needsMeOf(await loadInbox(fake));
    expect(m.needing().map((t) => t.id)).toEqual(["acme-6", "acme-2", "beta-1"]);
    expect(m.needing("acme").map((t) => t.id)).toEqual(["acme-6", "acme-2"]);
  });

  it("chooses the popup kind of an ID: a Spec's, a Focus popup while a Ticket needs the human, else a Ticket's", async () => {
    const fake = await twoProjects();
    fake.setNeedsMe([{ reason: "in_review", tickets: [{ id: "acme-2" }] }]);
    const m = needsMeOf(await loadInbox(fake));
    expect(m.popupKind("acme-1")).toBe("spec");
    expect(m.popupKind("acme-2")).toBe("focus");
    expect(m.popupKind("acme-3")).toBe("ticket");
    expect(m.popupKind("acme-99")).toBeUndefined();
    expect(needsMeOf(undefined).popupKind("acme-2")).toBeUndefined();
  });
});

describe("isCIItemReason", () => {
  it("is true of the CI item reasons only", () => {
    expect(["ci_failed", "ci_cancelled", "ci_unavailable"].every((r) => isCIItemReason(r as never))).toBe(true);
    expect(isCIItemReason("failed")).toBe(false);
    expect(isCIItemReason(undefined)).toBe(false);
  });
});

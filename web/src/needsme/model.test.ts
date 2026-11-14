// The Needs Me read model's questions, over the Inbox data as loadInbox
// reads it from a WASM Core world: service-shaped, with no React.
import { describe, expect, it } from "vitest";
import { newWorld } from "../api/core/core";
import { scenario } from "../api/scenario";
import { loadInbox } from "../live";
import { failCI } from "../ciTesting";
import { isCIItemReason, needsMeOf } from "./model";

// twoProjects is acme with a plain Spec, Sign-in, of two Ready Tickets
// (Form and Page) and a Requirement, Dark mode, in Grilling, and beta with
// one Ready Ticket, Docs: nothing needs the human until a test says so.
async function twoProjects() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("beta", "Beta");
  const signIn = await s.spec("acme", { title: "Sign-in" });
  const form = await s.ticket("acme", { title: "Form", spec: signIn });
  const page = await s.ticket("acme", { title: "Page", spec: signIn });
  const r = await w.api.submitRequirement("acme", "Dark mode");
  const [grilling, specStage] = r.stages.map((t) => t.id);
  const docs = await s.ticket("beta", { title: "Docs" });
  const model = async () => needsMeOf(await loadInbox(w.api));
  return { w, s, signIn, form, page, req: r.spec.id, grilling, specStage, docs, model };
}

describe("needsMeOf", () => {
  it("answers a Ticket's reason from its find_tickets row, and none for one that doesn't need the human", async () => {
    const { s, form, page, model } = await twoProjects();
    await s.inReview("acme", form);
    const m = await model();
    expect(m.reasonOf(form)).toBe("in_review");
    expect(m.reasonOf(page)).toBeUndefined();
    expect(m.reasonOf("acme-99")).toBeUndefined();
  });

  it("answers a Stage Ticket's reason and its Requirement's through it", async () => {
    const { s, req, grilling, model } = await twoProjects();
    await s.askRound("acme", grilling);
    const m = await model();
    expect(m.reasonOf(grilling)).toBe("needs_answer");
    expect(m.reasonOf(req)).toBe("needs_answer");
  });

  it("answers a Spec's reason as its first Ticket's that needs the human, Stages first", async () => {
    const { s, signIn, page, req, grilling, model } = await twoProjects();
    const build = await s.ticket("acme", { title: "Build", spec: req });
    await s.inReview("acme", build);
    await s.inReview("acme", page);
    await s.fail("acme", grilling);
    const m = await model();
    expect(m.reasonOf(signIn)).toBe("in_review");
    expect(m.reasonOf(req)).toBe("failed");
  });

  it("answers the reason of each Ticket a CI item lists", async () => {
    const { s, form, page, model } = await twoProjects();
    await s.accepted("acme", form, { branch: form, commit: "abc" });
    await s.accepted("acme", page, { branch: page, commit: "def" });
    await failCI(s, "acme", [form, page]);
    const m = await model();
    expect(m.reasonOf(form)).toBe("ci_failed");
    expect(m.reasonOf(page)).toBe("ci_failed");
  });

  it("counts core's count, a Project's counts entry, and 0 for a Project with nothing in Needs Me", async () => {
    const { s, form, grilling, model } = await twoProjects();
    await s.askRound("acme", grilling);
    await s.inReview("acme", form);
    const m = await model();
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
    const { s, form, req, grilling, docs, model } = await twoProjects();
    await s.askRound("acme", grilling);
    await s.inReview("acme", form);
    await s.fail("beta", docs);
    const m = await model();
    const all = m.rows();
    expect(all.requirements.map((r) => r.id)).toEqual([req]);
    expect(all.groups.map((g) => [g.reason, g.tickets.map((r) => r.id)])).toEqual([
      ["failed", [docs]],
      ["in_review", [form]],
    ]);
    const beta = m.rows("beta");
    expect(beta.requirements).toEqual([]);
    expect(beta.groups.map((g) => g.reason)).toEqual(["failed"]);
  });

  it("scopes the Tickets, Requirements and Maps to the Project, and every Spec's Tickets to their Spec", async () => {
    const { signIn, form, page, req, docs, model } = await twoProjects();
    const m = await model();
    expect(m.tickets("beta").map((t) => t.id)).toEqual([docs]);
    expect(m.tickets().length).toBe(6);
    expect(m.requirements("acme").map((r) => r.id)).toEqual([req]);
    expect(m.requirements("beta")).toEqual([]);
    expect(m.maps()).toEqual([]);
    expect(m.ticketsOf(signIn).map((t) => t.id)).toEqual([form, page]);
    expect(m.ticketsOf("acme-99")).toEqual([]);
  });

  it("lists the Board's Needs Me column: Stage Tickets as themselves, in Needs Me's order, in the Project", async () => {
    const { s, form, grilling, docs, model } = await twoProjects();
    await s.askRound("acme", grilling);
    await s.inReview("acme", form);
    await s.fail("beta", docs);
    const m = await model();
    expect(m.needing().map((t) => t.id)).toEqual([grilling, docs, form]);
    expect(m.needing("acme").map((t) => t.id)).toEqual([grilling, form]);
  });

  it("chooses the popup kind of an ID: a Spec's, a Focus popup while a Ticket needs the human, else a Ticket's", async () => {
    const { s, signIn, form, page, model } = await twoProjects();
    await s.inReview("acme", form);
    const m = await model();
    expect(m.popupKind(signIn)).toBe("spec");
    expect(m.popupKind(form)).toBe("focus");
    expect(m.popupKind(page)).toBe("ticket");
    expect(m.popupKind("acme-99")).toBeUndefined();
    expect(needsMeOf(undefined).popupKind(form)).toBeUndefined();
  });

  it("chooses a Focus popup for a Stage Ticket that needs the human", async () => {
    const { s, grilling, specStage, model } = await twoProjects();
    await s.askRound("acme", grilling);
    const m = await model();
    expect(m.popupKind(grilling)).toBe("focus");
    expect(m.popupKind(specStage)).toBe("ticket");
  });

  it("lists the Focus Items in scope: Stage Tickets as themselves, in Needs Me's order", async () => {
    const { s, form, grilling, docs, model } = await twoProjects();
    await s.askRound("acme", grilling);
    await s.inReview("acme", form);
    await s.fail("beta", docs);
    const m = await model();
    expect(m.focusItems({}).map((r) => r.id)).toEqual([grilling, docs, form]);
    expect(m.focusItems({ project: "acme" }).map((r) => r.id)).toEqual([grilling, form]);
    expect(m.focusItems({ project: "beta" }).map((r) => r.id)).toEqual([docs]);
    expect(m.focusItems({ ticket: form }).map((r) => r.id)).toEqual([form]);
    expect(needsMeOf(undefined).focusItems({})).toEqual([]);
  });

  it("leaves out a row that isn't a Focus Item, unless a Ticket scopes the Focus Items to it", async () => {
    const { w, s, form, docs, model } = await twoProjects();
    await w.api.setFocusKinds("acme", ["in_review"]);
    await s.inReview("acme", form);
    await s.fail("beta", docs);
    const m = await model();
    expect(m.focusItems({}).map((r) => r.id)).toEqual([docs]);
    expect(m.focusItems({ project: "acme" })).toEqual([]);
    expect(m.focusItems({ ticket: form }).map((r) => r.id)).toEqual([form]);
  });

  it("scopes the Focus Items to a Ticket a CI item lists through the item's row", async () => {
    const { w, s, form, page, model } = await twoProjects();
    await s.accepted("acme", form, { branch: form, commit: "abc" });
    await s.accepted("acme", page, { branch: page, commit: "def" });
    await failCI(s, "acme", [form, page]);
    await w.api.setFocusKinds("acme", ["ci_failed"]);
    const m = await model();
    expect(m.focusItems({})).toEqual([]);
    expect(m.focusItems({ ticket: page }).map((r) => [r.reason, r.ci_item?.tickets.map((t) => t.id)])).toEqual([["ci_failed", [form, page]]]);
  });
});

describe("isCIItemReason", () => {
  it("is true of the CI item reasons only", () => {
    expect(["ci_failed", "ci_cancelled", "ci_unavailable"].every((r) => isCIItemReason(r as never))).toBe(true);
    expect(isCIItemReason("failed")).toBe(false);
    expect(isCIItemReason(undefined)).toBe(false);
  });
});

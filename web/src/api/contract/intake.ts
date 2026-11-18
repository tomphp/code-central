import { beforeEach, expect, it } from "vitest";
import { ApiError } from "../types";
import type { Driver } from "../driver";
import { wantAcknowledgement } from "./support";

export function intakeContract(driver: () => Driver) {
  let d: Driver;
  beforeEach(() => {
    d = driver();
  });

  // ---- Requirements (ticket 74) ----
  it("submits a Requirement as its Spec and three chained Stage Tickets, waiting for a Factory", async () => {
    await d.createProject("acme", "Acme");
    const r = await d.api.submitRequirement("acme", "Export a Spec as Markdown\nOne file, Tickets included.");
    expect(Object.keys(r.spec).sort()).toEqual(["changed", "id", "updated"]);
    expect(Number.isNaN(Date.parse(r.spec.updated))).toBe(false);
    expect(r.spec.changed).toEqual(["title", "body"]);
    expect(r.stages).toHaveLength(3);
    const chained = ["title", "state", "spec", "labels", "blocked_by", "blockers", "description", "moves", "blocked"];
    for (const [i, t] of r.stages.entries()) {
      wantAcknowledgement(t, t.id, "Ready");
      expect(t.changed).toEqual(i === 0 ? ["title", "state", "spec", "labels", "description", "moves", "claimable"] : chained);
    }

    const stages = await Promise.all(r.stages.map((t) => d.api.getTicket("acme", t.id)));
    expect(stages.map((t) => [t.labels, t.state])).toEqual([
      [["stage:grilling"], "Ready"],
      [["stage:spec"], "Ready"],
      [["stage:tickets"], "Ready"],
    ]);
    expect(stages[1].blocked_by).toEqual([r.stages[0].id]);
    expect(stages[2].blocked_by).toEqual([r.stages[1].id]);

    const s = await d.api.getSpec("acme", r.spec.id);
    expect(s).toMatchObject({ id: r.spec.id, title: "Export a Spec as Markdown", body: "Export a Spec as Markdown\nOne file, Tickets included.", finished: false });
    expect(Number.isNaN(Date.parse(s.created))).toBe(false);
    expect(s.tickets!.map((t) => t.id)).toEqual(r.stages.map((t) => t.id));
    expect(s.requirement).toEqual({
      stages: [
        { name: "grilling", ticket: r.stages[0].id, state: "Ready" },
        { name: "spec", ticket: r.stages[1].id, state: "Ready" },
        { name: "tickets", ticket: r.stages[2].id, state: "Ready" },
      ],
      build: { done: 0, total: 0 },
      waiting_for_factory: true,
      segments: [
        { stage: "grilling", progress: "queued", ticket: r.stages[0].id },
        { stage: "spec", progress: "waiting", ticket: r.stages[1].id },
        { stage: "tickets", progress: "waiting", ticket: r.stages[2].id },
        { stage: "build", progress: "waiting" },
      ],
      current: "grilling",
      needs: [],
      default_ticket: r.stages[0].id,
    });
  });

  it("shows a Requirement's Land per Spec status under Pull Request with Land per Spec on", async () => {
    await d.createProject("acme", "Acme");
    await d.reportPushRemote("acme", "git@github.com:acme/web.git");
    await d.letGHWrite("acme/web");
    await d.api.setLandingMethod("acme", { method: "pull_request", push: false, remote: "", land_per_spec: true });
    const r = await d.api.submitRequirement("acme", "Export a Spec as Markdown");
    expect((await d.api.getSpec("acme", r.spec.id)).land_per_spec).toEqual({ waiting: [], ready: false });
  });

  it("refuses a Requirement with no text", async () => {
    await d.createProject("acme", "Acme");
    const e = await d.api.submitRequirement("acme", "  \n ").catch((e) => e);
    expect(e).toBeInstanceOf(ApiError);
    expect(e).toMatchObject({ status: 422, code: "invalid", field: "text" });
  });

  it("finds only Requirements with requirements: true, finished ones included when asked", async () => {
    await d.createProject("acme", "Acme");
    await d.createTicket("acme", "Not a Requirement", "Ready");
    const r = await d.api.submitRequirement("acme", "Export a Spec");

    const all = await d.api.findSpecs("acme");
    expect(all.length).toBeGreaterThan(1);
    expect(all.find((s) => s.id === r.spec.id)?.requirement?.waiting_for_factory).toBe(true);
    expect(all.filter((s) => s.id !== r.spec.id).every((s) => s.requirement === undefined)).toBe(true);

    const reqs = await d.api.findSpecs("acme", { requirements: true });
    expect(reqs.map((s) => [s.id, s.title, s.finished])).toEqual([[r.spec.id, "Export a Spec", false]]);

    for (const t of r.stages) await d.api.act("acme", t.id, "move", { to_state: "Done" });
    expect(await d.api.findSpecs("acme", { requirements: true })).toEqual([]);
    const delivered = await d.api.findSpecs("acme", { requirements: true, finished: true });
    expect(delivered.map((s) => [s.id, s.finished])).toEqual([[r.spec.id, true]]);
    expect(delivered[0].requirement).toMatchObject({ build: { done: 0, total: 0 }, waiting_for_factory: false });
    expect(delivered[0].requirement!.stages.map((s) => s.state)).toEqual(["Done", "Done", "Done"]);
  });

  // ---- Maps (code-central-218) ----
  it("submits a Map as its Spec, with the Map's sections, and one Ready Charting Ticket", async () => {
    await d.createProject("acme", "Acme");
    const r = await d.api.submitMap("acme", "Offline mode\nWorks on a plane.");
    expect(r.spec.changed).toEqual(["title", "labels", "body"]);
    wantAcknowledgement(r.charting, r.charting.id, "Ready");
    expect(r.charting.changed).toEqual(["title", "state", "spec", "labels", "description", "moves", "claimable"]);

    const charting = await d.api.getTicket("acme", r.charting.id);
    expect(charting).toMatchObject({ title: "Charting: Offline mode", labels: ["wayfinder:chart"], state: "Ready", claimable: true });
    expect(charting.description).toContain(r.spec.id);

    const s = await d.api.getSpec("acme", r.spec.id);
    expect(s).toMatchObject({ title: "Offline mode", labels: ["wayfinder:map"] });
    expect(s.body).toBe(
      "Offline mode\nWorks on a plane.\n\n## Destination\n\n## Notes\n\n## Decisions so far\n\n## Not yet specified\n\n## Out of scope\n",
    );
    expect(s.requirement).toBeUndefined();
    expect(s.map).toEqual({ decided: 0, total: 1, needs_you: 0, guided: false, can_write_spec: false, requirements: [] });
    expect(s.tickets!.map((t) => [t.id, t.node_type])).toEqual([[r.charting.id, "chart"]]);

    await d.api.act("acme", r.charting.id, "move", { to_state: "Done" });
    expect((await d.api.getSpec("acme", r.spec.id)).map).toEqual({ decided: 1, total: 1, needs_you: 0, guided: false, can_write_spec: true, requirements: [] });
  });

  it("submits Straight to Spec: only the spec and tickets Stages, written from the text, and no map Tag", async () => {
    await d.createProject("acme", "Acme");
    const m = await d.api.submitMap("acme", "Offline mode");
    const conflict = await d.api.submitRequirement("acme", "Offline mode", m.spec.id, true).catch((e) => e);
    expect(conflict).toMatchObject({ status: 422, code: "invalid", field: "skip_grilling" });
    const r = await d.api.submitRequirement("acme", "Dark mode", undefined, true);
    expect(r.stages).toHaveLength(2);
    const [spec, tickets] = await Promise.all(r.stages.map((t) => d.api.getTicket("acme", t.id)));
    expect([spec.labels, tickets.labels, tickets.blocked_by]).toEqual([["stage:spec"], ["stage:tickets"], [spec.id]]);
    expect(spec.state).toBe("Ready");
    expect(spec.description).toBe(`Write the Spec for the Requirement in Spec [${r.spec.id}](${r.spec.id}), from its text.\n`);
    const s = await d.api.getSpec("acme", r.spec.id);
    expect(s.tags).toEqual({});
    expect(s.requirement?.map).toBeUndefined();
    expect(s.requirement?.stages.map((st) => st.name)).toEqual(["spec", "tickets"]);
  });

  it("writes the Spec from a finished Map: a Requirement with only the spec and tickets Stages, tagged with the Map", async () => {
    await d.createProject("acme", "Acme");
    const plain = await d.api.submitRequirement("acme", "Not a Map");
    const m = await d.api.submitMap("acme", "Offline mode");
    const notMap = await d.api.submitRequirement("acme", "Offline mode", plain.spec.id).catch((e) => e);
    expect(notMap).toMatchObject({ status: 422, code: "invalid", field: "map" });
    const open = await d.api.submitRequirement("acme", "Offline mode", m.spec.id).catch((e) => e);
    expect(open).toBeInstanceOf(ApiError);
    expect(open).toMatchObject({ status: 409, code: "action_not_allowed" });
    expect(open.message).toContain(`${m.charting.id} (Ready)`);

    await d.api.act("acme", m.charting.id, "move", { to_state: "Done" });
    const r = await d.api.submitRequirement("acme", "Offline mode\nFrom the Map.", m.spec.id);
    expect(r.stages).toHaveLength(2);
    wantAcknowledgement(r.stages[0], r.stages[0].id, "Ready");
    expect(r.stages[0].changed).toEqual(["title", "state", "spec", "labels", "description", "moves", "claimable"]);
    const [spec, tickets] = await Promise.all(r.stages.map((t) => d.api.getTicket("acme", t.id)));
    expect([spec.labels, tickets.labels, tickets.blocked_by]).toEqual([["stage:spec"], ["stage:tickets"], [spec.id]]);
    expect(spec.description).toContain(m.spec.id);
    expect(spec.description).toContain("Decisions so far");

    const s = await d.api.getSpec("acme", r.spec.id);
    expect(s.tags).toEqual({ map: m.spec.id });
    expect(s.requirement).toEqual({
      map: m.spec.id,
      stages: [
        { name: "spec", ticket: spec.id, state: "Ready" },
        { name: "tickets", ticket: tickets.id, state: "Ready" },
      ],
      build: { done: 0, total: 0 },
      waiting_for_factory: true,
      segments: [
        { stage: "grilling", progress: "done", map: m.spec.id },
        { stage: "spec", progress: "queued", ticket: spec.id },
        { stage: "tickets", progress: "waiting", ticket: tickets.id },
        { stage: "build", progress: "waiting" },
      ],
      current: "spec",
      needs: [],
      default_ticket: spec.id,
    });
    expect((await d.api.getSpec("acme", m.spec.id)).map?.requirements).toEqual([r.spec.id]);
    expect((await d.api.getSpec("acme", plain.spec.id)).requirement?.map).toBeUndefined();
  });

  it("guides a Map only while its guided Tag is on", async () => {
    await d.createProject("acme", "Acme");
    const r = await d.api.submitMap("acme", "Offline mode");
    const guided = async () => (await d.api.getSpec("acme", r.spec.id)).map?.guided;

    expect(await d.api.setTags("acme", r.spec.id, { guided: "on" })).toEqual({ id: r.spec.id, kind: "spec", labels: ["wayfinder:map"], tags: { guided: "on" } });
    expect(await guided()).toBe(true);
    expect((await d.api.findSpecs("acme", { labels: ["wayfinder:map"] }))[0].map?.guided).toBe(true);
    await d.api.setTags("acme", r.spec.id, { guided: "yes" });
    expect(await guided()).toBe(false);
    await d.api.setTags("acme", r.spec.id, { guided: "on", area: "web" });
    expect(await d.api.removeTags("acme", r.spec.id, ["guided"])).toMatchObject({ tags: { area: "web" } });
    expect(await guided()).toBe(false);
  });

  it("refuses a Map with no text", async () => {
    await d.createProject("acme", "Acme");
    const e = await d.api.submitMap("acme", "  \n ").catch((e) => e);
    expect(e).toBeInstanceOf(ApiError);
    expect(e).toMatchObject({ status: 422, code: "invalid", field: "text" });
  });

  it("finds Maps by their Label, each row with its Map block, and no node type off a Map", async () => {
    await d.createProject("acme", "Acme");
    const plain = await d.createTicket("acme", "Not on a Map", "Ready");
    const r = await d.api.submitMap("acme", "Offline mode");

    const maps = await d.api.findSpecs("acme", { labels: ["wayfinder:map"] });
    expect(maps.map((s) => [s.id, s.map])).toEqual([[r.spec.id, { decided: 0, total: 1, needs_you: 0, guided: false, can_write_spec: false, requirements: [] }]]);
    expect((await d.api.findSpecs("acme")).filter((s) => s.id !== r.spec.id).every((s) => s.map === undefined)).toBe(true);
    const rows = await d.api.findTickets("acme");
    expect(rows.find((t) => t.id === plain)?.node_type).toBeUndefined();
    expect(rows.find((t) => t.id === r.charting.id)?.node_type).toBe("chart");
  });

  it("gives a Map's Ticket rows their Blockers, the Map view's edges, and no other row", async () => {
    await d.createProject("acme", "Acme");
    const a = await d.createTicket("acme", "Not on a Map", "Ready");
    const b = await d.createTicket("acme", "Nor this", "Ready");
    await d.addBlocker("acme", b, a);
    const r = await d.api.submitMap("acme", "Offline mode");
    await d.addBlocker("acme", r.charting.id, a);

    const s = await d.api.getSpec("acme", r.spec.id);
    expect(s.tickets!.map((t) => [t.id, t.blocked_by])).toEqual([[r.charting.id, [a]]]);
    const rows = await d.api.findTickets("acme");
    expect(rows.find((t) => t.id === b)?.blocked_by).toBeUndefined();
    expect(rows.find((t) => t.id === r.charting.id)?.blocked_by).toEqual([a]);
  });

  it("approves a Draft Ticket to Ready, and refuses one that isn't Draft", async () => {
    await d.createProject("acme", "Acme");
    const draft = await d.createTicket("acme", "A", "Draft");
    const ready = await d.createTicket("acme", "B", "Ready");

    wantAcknowledgement(await d.api.act("acme", draft, "approve", {}), draft, "Ready", ["state", "moves"]);
    expect((await d.api.getTicket("acme", draft)).moves!.at(-1)).toMatchObject({ from: "Draft", to: "Ready", actor: "human" });
    const e = await d.api.act("acme", ready, "approve", {}).catch((e) => e);
    expect(e).toMatchObject({ status: 409, code: "action_not_allowed" });
  });

  // ---- Prototypes (code-central-221, code-central-223) ----
  it("submits a Prototype on a Spec as one Ready prototype Ticket, titled by the text's first line", async () => {
    await d.createProject("acme", "Acme");
    const r = await d.api.submitRequirement("acme", "Dark mode");
    const ack = await d.api.submitPrototype("acme", r.spec.id, "A theme picker\nWould a picker beat a toggle?");
    wantAcknowledgement(ack, ack.id, "Ready", ["title", "state", "spec", "labels", "description", "moves", "claimable"]);

    const t = await d.api.getTicket("acme", ack.id);
    expect(t).toMatchObject({
      title: "Prototype: A theme picker",
      description: "A theme picker\nWould a picker beat a toggle?",
      state: "Ready",
      spec: { id: r.spec.id },
      labels: ["prototype"],
      claimable: true,
    });
    expect(t.moves).toMatchObject([{ to: "Ready", actor: "human", via: "submit_prototype" }]);
    expect((await d.api.getSpec("acme", r.spec.id)).tickets!.map((t) => t.id)).toContain(ack.id);
  });

  it("refuses a Prototype with no text, or on no Spec", async () => {
    await d.createProject("acme", "Acme");
    const r = await d.api.submitMap("acme", "Offline mode");
    const blank = await d.api.submitPrototype("acme", r.spec.id, "  \n ").catch((e) => e);
    expect(blank).toBeInstanceOf(ApiError);
    expect(blank).toMatchObject({ status: 422, code: "invalid", field: "text" });
    const nowhere = await d.api.submitPrototype("acme", "acme-99", "A theme picker").catch((e) => e);
    expect(nowhere).toMatchObject({ status: 422, code: "invalid", field: "spec" });
  });

  // ---- Retros (code-central-646) ----
  it("submits a Retro on a Cancelled Ticket as one Ready retro Ticket on the Retros Spec, with the note", async () => {
    await d.createProject("acme", "Acme");
    const target = (await d.api.submitSmallChange("acme", "Rename the button")).id;
    await d.api.act("acme", target, "cancel", {});
    const ack = await d.api.submitRetro("acme", target, "Why was it cancelled?");
    wantAcknowledgement(ack, ack.id, "Ready", ["title", "state", "spec", "labels", "tags", "description", "moves", "claimable"]);

    const t = await d.api.getTicket("acme", ack.id);
    expect(t).toMatchObject({
      title: "Retro: Rename the button",
      state: "Ready",
      spec: { title: "Retros" },
      labels: ["retro"],
      tags: { "retro-of": target },
      claimable: true,
    });
    expect(t.description).toContain("Why was it cancelled?");
    expect(t.moves).toMatchObject([{ to: "Ready", actor: "human", via: "submit_retro" }]);
    const again = await d.api.submitRetro("acme", target);
    expect((await d.api.getTicket("acme", again.id)).spec.id).toBe(t.spec.id);
  });

  it("refuses a Retro on unfinished work, or on nothing", async () => {
    await d.createProject("acme", "Acme");
    const open = (await d.api.submitSmallChange("acme", "Rename the button")).id;
    const unfinished = await d.api.submitRetro("acme", open).catch((e) => e);
    expect(unfinished).toBeInstanceOf(ApiError);
    expect(unfinished).toMatchObject({ status: 422, code: "invalid", field: "target" });
    const nowhere = await d.api.submitRetro("acme", "acme-99").catch((e) => e);
    expect(nowhere).toMatchObject({ status: 422, code: "invalid", field: "target" });
  });

  // ---- Small Changes (code-central-475) ----
  it("submits a Small Change as one Ready Ticket on the Small Changes Spec, titled by the text's first line", async () => {
    await d.createProject("acme", "Acme");
    const ack = await d.api.submitSmallChange("acme", "Rename the button\nIt says Go; make it say Start.");
    wantAcknowledgement(ack, ack.id, "Ready", ["title", "state", "spec", "description", "moves", "claimable"]);

    const t = await d.api.getTicket("acme", ack.id);
    expect(t).toMatchObject({
      title: "Rename the button",
      description: "Rename the button\nIt says Go; make it say Start.",
      state: "Ready",
      spec: { title: "Small Changes" },
      labels: [],
      claimable: true,
    });
    expect(t.moves).toMatchObject([{ to: "Ready", actor: "human", via: "submit_small_change" }]);
    const spec = await d.api.getSpec("acme", t.spec.id);
    expect(spec.labels).toContain("long-lived");
    expect(spec.tickets!.map((t) => t.id)).toContain(ack.id);
  });

  it("reuses the Small Changes Spec for a second Small Change", async () => {
    await d.createProject("acme", "Acme");
    const one = await d.api.submitSmallChange("acme", "Rename the button");
    const two = await d.api.submitSmallChange("acme", "Fix the typo");
    const a = await d.api.getTicket("acme", one.id);
    const b = await d.api.getTicket("acme", two.id);
    expect(b.spec.id).toBe(a.spec.id);
  });

  it("refuses a Small Change with no text", async () => {
    await d.createProject("acme", "Acme");
    const blank = await d.api.submitSmallChange("acme", "  \n ").catch((e) => e);
    expect(blank).toBeInstanceOf(ApiError);
    expect(blank).toMatchObject({ status: 422, code: "invalid", field: "text" });
  });

  it("saves a Requirement, a Map and a Small Change as Draft: every Ticket Draft and none claimable", async () => {
    await d.createProject("acme", "Acme");
    const r = await d.api.submitRequirement("acme", "Offline mode", undefined, false, true);
    expect(r.stages).toHaveLength(3);
    for (const s of r.stages) {
      const t = await d.api.getTicket("acme", s.id);
      expect(t).toMatchObject({ state: "Draft", claimable: false });
      expect(t.moves).toEqual([]);
    }
    const m = await d.api.submitMap("acme", "Dark mode", true);
    expect(await d.api.getTicket("acme", m.charting.id)).toMatchObject({ state: "Draft", claimable: false });
    const c = await d.api.submitSmallChange("acme", "Rename the button", true);
    expect(await d.api.getTicket("acme", c.id)).toMatchObject({ state: "Draft", claimable: false });
  });

  it("refuses to save Write the Spec as Draft", async () => {
    await d.createProject("acme", "Acme");
    const e = await d.api.submitRequirement("acme", "Offline mode", "acme-1", false, true).catch((e) => e);
    expect(e).toMatchObject({ status: 422, code: "invalid", field: "draft" });
  });
}

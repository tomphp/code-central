import { beforeEach, expect, it } from "vitest";
import type { Driver } from "../driver";

export function labelsContract(driver: () => Driver) {
  let d: Driver;
  beforeEach(() => {
    d = driver();
  });

  // ---- The auto-accept Label (Spec code-central-589) ----

  it("adds a Label once and removes it by name, leaving the others, on a Ticket and on a Spec", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Login page", "Ready");
    const spec = (await d.api.getTicket("acme", id)).spec.id;

    expect(await d.api.addLabels("acme", id, ["auto-accept"])).toEqual({ id, kind: "ticket", labels: ["auto-accept"], tags: {} });
    expect((await d.api.addLabels("acme", id, ["auto-accept"])).labels).toEqual(["auto-accept"]);
    expect((await d.api.getTicket("acme", id)).labels).toEqual(["auto-accept"]);
    expect(await d.api.removeLabels("acme", id, ["auto-accept"])).toEqual({ id, kind: "ticket", labels: [], tags: {} });

    expect(await d.api.addLabels("acme", spec, ["auto-accept"])).toMatchObject({ id: spec, kind: "spec", labels: ["auto-accept"] });
    expect((await d.api.addLabels("acme", spec, ["auto-accept"])).labels).toEqual(["auto-accept"]);
    expect((await d.api.getSpec("acme", spec)).labels).toEqual(["auto-accept"]);
    expect((await d.api.removeLabels("acme", spec, ["auto-accept"])).labels).toEqual([]);
    expect((await d.api.getSpec("acme", spec)).labels).toEqual([]);

    const bad = await d.api.addLabels("acme", id, ["long-lived"]).catch((e) => e);
    expect(bad).toMatchObject({ status: 422, code: "invalid" });
  });

  it("accepts a Spec's In Review Tickets when auto-accept is added to the Spec, as the human with the Spec's Label as the cause", async () => {
    await d.createProject("acme", "Acme");
    const review = await d.createTicket("acme", "Login page", "Ready");
    const other = await d.createTicket("acme", "Sign-up page", "Ready");
    await d.finish("acme", review, "Added the login page");
    const spec = (await d.api.getTicket("acme", review)).spec.id;

    await d.api.addLabels("acme", spec, ["auto-accept"]);
    const t = await d.api.getTicket("acme", review);
    expect(t.state).toBe("Done");
    expect(t.moves!.at(-1)).toMatchObject({ from: "In Review", to: "Done", actor: "human", cause: "spec-label" });
    expect((await d.api.getTicket("acme", other)).state).toBe("Ready");

    // Taking the Label off undoes nothing.
    await d.api.removeLabels("acme", spec, ["auto-accept"]);
    expect((await d.api.getTicket("acme", review)).state).toBe("Done");
  });

  it("accepts an In Review Ticket when auto-accept is added to it, with the Ticket's Label as the cause", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Login page", "Ready");
    const sibling = await d.createTicket("acme", "Sign-up page", "Ready");
    await d.finish("acme", id, "Added the login page", { branch: "impl-7", commit: "abc123" });
    await d.finish("acme", sibling, "Added the sign-up page");

    await d.api.addLabels("acme", id, ["auto-accept"]);
    const t = await d.api.getTicket("acme", id);
    expect(t.state).toBe("Accepted");
    expect(t.moves!.at(-1)).toMatchObject({ from: "In Review", to: "Accepted", actor: "human", cause: "ticket-label" });
    expect((await d.api.getTicket("acme", sibling)).state).toBe("In Review");
  });

  it("does not auto-accept a Prototype In Review on a Spec with the Label", async () => {
    await d.createProject("acme", "Acme");
    const id = await d.createTicket("acme", "Login page", "Ready");
    const spec = (await d.api.getTicket("acme", id)).spec.id;
    const proto = (await d.api.submitPrototype("acme", spec, "A theme picker")).id;
    await d.finish("acme", proto, "Built the picker");

    await d.api.addLabels("acme", spec, ["auto-accept"]);
    expect((await d.api.getTicket("acme", proto)).state).toBe("In Review");
  });
}

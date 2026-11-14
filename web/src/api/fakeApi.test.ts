// FakeApi's own checks, while UI tests not yet on the WASM Core use it
// (Spec code-central-580): they go with it.
import { describe, expect, it } from "vitest";
import { FakeApi } from "./fake";

describe("FakeApi long-lived Label", () => {
  it("refuses long-lived on a Ticket, as the service does, and allows it on a Spec", () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    expect(() => fake.addTicket("acme", { title: "Form", labels: ["long-lived"] })).toThrow(/goes on a Spec/);
    expect(fake.addSpec("acme", { title: "CI", labels: ["long-lived"] }).labels).toEqual(["long-lived"]);
  });
});

describe("FakeApi Land per Spec status on the Spec read", () => {
  it("is on a plain Spec and not on a Long-lived one", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    const plain = fake.addSpec("acme", { title: "Export" });
    const long = fake.addSpec("acme", { title: "CI", labels: ["long-lived"] });
    fake.addTicket("acme", { title: "Exporter", state: "Accepted", spec: { id: plain.id, title: plain.title }, waiting_for_spec: true });
    fake.addTicket("acme", { title: "Rule", state: "Accepted", spec: { id: long.id, title: long.title } });
    expect((await fake.getSpec("acme", plain.id)).land_per_spec).toEqual({ waiting: ["acme-3"], ready: true });
    expect((await fake.getSpec("acme", long.id)).land_per_spec).toBeUndefined();
  });
});

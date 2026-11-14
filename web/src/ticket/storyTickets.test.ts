// The stories' Tickets have each part a row or card shows only sometimes.
import { expect, it } from "vitest";
import { storyTickets } from "./storyTickets";

it("makes one Ticket for each optional part", async () => {
  const { plain, working, linked, stage, now } = await storyTickets();
  expect(plain).toMatchObject({ state: "Ready", blocked: false });
  expect(plain.link).toBeUndefined();
  expect(working).toMatchObject({ state: "In Progress", latest_event: { text: "Read: core/core.go" } });
  expect(now.getTime() - new Date(working.latest_event!.time).getTime()).toBe(2 * 60_000);
  expect(linked).toMatchObject({ state: "Accepted", blocked: true, waiting_for_spec: true, link: { integration: "GitHub", id: "#12" } });
  expect(stage).toMatchObject({ state: "Needs Answer", labels: ["stage:grilling"], project: "acme" });
});

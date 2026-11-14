// The Ticket pane on the WASM Core: each story's world makes one Ticket
// with the scenario builder and reads it back as the view does, so the
// pane's moves and pickers act on Core. The Spec's Labels are given, so
// the Auto-accept switch shows where the Ticket can take it.
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Api } from "../api/api";
import type { World } from "../api/core/core";
import { storyWorld } from "../api/core/story";
import { SESSION } from "../api/driver";
import type { Scenario } from "../api/scenario";
import type { TicketView } from "../api/types";
import { TicketDetail } from "./TicketDetail";

const meta = { title: "Ticket/TicketDetail" } satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

const MIN = 60_000;

interface Loaded {
  api: Api;
  now: Date;
  ticket?: TicketView;
}

// pane is a story of the Ticket that build makes, returning its ID, in the
// Project "acme"; without build there is no Ticket.
const pane = (build?: (s: Scenario, w: World) => Promise<string>, error?: string): Story => ({
  ...storyWorld(async (s, w): Promise<Loaded> => {
    await s.project("acme", "Acme");
    const id = build && (await build(s, w));
    return { api: w.api, now: w.now(), ticket: id ? await w.api.getTicket("acme", id) : undefined };
  }),
  render: (_, { loaded }) => <TicketDetail project="acme" specLabels={[]} error={error} {...(loaded.world as Loaded)} />,
});

// Before the Ticket is read.
export const Loading = pane();
export const ReadError = pane(undefined, "acme-9 isn't a Ticket in acme.");

// A Ticket with nothing optional: no description, Claims or history.
export const Empty = pane((s) => s.ticket("acme", { title: "Add a sign-in page" }));

// In Progress on its second Claim, the first released: its Labels,
// description, Blocker, model choice, the live Claim's latest Session
// Event and Live section, and a history of moves, comments and an edit.
export const Claims = pane(async (s, w) => {
  const spec = await s.spec("acme", { title: "Sign-in" });
  const blocker = await s.ticket("acme", { title: "Store sessions server-side", spec });
  const id = await s.ticket("acme", {
    title: "Rate-limit the sign-in endpoint",
    spec,
    labels: ["auth", "security"],
    description: "Five failed sign-ins in a minute **lock the account** for ten minutes.",
  });
  await s.claim("acme", id, "s-old");
  w.advance(60 * MIN);
  await w.api.act("acme", id, "release", {});
  w.advance(30 * MIN);
  await s.claim("acme", id, SESSION);
  await s.modelChoice("acme", id, { model: "opus", effort: "high" });
  w.advance(10 * MIN);
  await s.postSessionEvents(SESSION, [{ time: w.now().toISOString(), kind: "tool_call", tool: "Read", text: "Read: auth/limit.go" }]);
  await s.human(`${s.ticketPath("acme", id)}/comment`, { body: "Count failures per account, not per IP." });
  await s.agent(`${s.ticketPath("acme", id)}/edit_ticket`, { description: "Five failed sign-ins in a minute lock the account." });
  // A Blocker added once it is claimed, since one stops a claim.
  await s.addBlocker("acme", id, blocker);
  w.advance(5 * MIN);
  return id;
});

// Failed: the Failure Report in its action panel.
export const FailureReport = pane((s) => s.ticket("acme", { title: "Migrate the session table", state: "Failed" }));

// Accepted and pushed with CI passed: the CI section below the details.
export const CIPassed = pane(async (s) => {
  const id = await s.ticket("acme", { title: "Add a sign-in page", state: "Accepted" });
  await s.pushed("acme", id);
  await s.ci("acme", id, "passed");
  return id;
});

// CI failed: the CI section moves into the action panel, with its options.
export const CIFailed = pane(async (s) => {
  const id = await s.ticket("acme", { title: "Add a sign-in page", state: "Accepted" });
  await s.pushed("acme", id);
  await s.ci("acme", id, "failed");
  return id;
});

// In Review: the review panel, the Auto-accept switch and the move controls.
export const InReview = pane((s) => s.ticket("acme", { title: "Add a sign-in page", state: "In Review" }));

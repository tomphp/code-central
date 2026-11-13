// The Focus Pop-up trigger's decision, for the cases too costly to set up
// through the App: what counts as a new Focus Item against the dismissed
// set. The App-level tests in FocusPopUp.test.tsx cover the rest.
import { describe, expect, it } from "vitest";
import type { NeedsMeRow } from "../api/types";
import { popupOpen } from "./popupTrigger";

const row = (r: Partial<NeedsMeRow>): NeedsMeRow => ({ project: "acme", id: "acme-1", title: "", state: "Needs Answer", reason: "needs_answer", since: "", ...r });
const round = (number: number) => row({ round: { number, open: 2, questions: 2 } });

describe("popupOpen", () => {
  it.each<[string, NeedsMeRow, string[], boolean]>([
    ["a Round dismissed unread", round(1), ["acme-1 round 1"], false],
    ["a Round dismissed read", round(1), ["acme-1 round 1", "acme-1 round 1 question 1", "acme-1 round 1 question 2"], false],
    ["a new Round on a dismissed Ticket", round(2), ["acme-1 round 1"], true],
    ["a Ticket waiting for a new reason", row({ id: "acme-2", state: "In Review", reason: "in_review" }), ["acme-2 failed"], true],
    ["a new CI item for a dismissed Project", row({ id: "", reason: "ci_failed", ci_item: { reason: "ci_failed", commit: "bbb", tickets: [], options: [] } }), ["acme ci_failed aaa"], true],
  ])("with %s, opens: %s", (_, r, dismissed, open) => {
    const needsMe = { count: 1, groups: [{ reason: r.reason, tickets: [r] }] };
    expect(popupOpen({ option: "on", view: "inbox", needsMe, dismissed: new Set(dismissed), open: false })).toBe(open);
  });
});

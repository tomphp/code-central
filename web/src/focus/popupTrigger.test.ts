// The Focus Pop-up trigger's decision, for the cases too costly to set up
// through the App: what counts as a new Focus Item against the dismissed
// set. The App-level tests in FocusPopUp.test.tsx cover the rest.
import { describe, expect, it } from "vitest";
import type { NeedsMeReason, NeedsMeRow } from "../api/types";
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
    expect(popupOpen({ option: "on", view: "inbox", needsMe, projects: [], dismissed: new Set(dismissed), open: false, otherPopup: false, typing: false })).toBe(open);
  });

  // Each row follows its own Project's Focus Kinds: acme has Answered, CI
  // failed and Base diverged off, beta has nothing off.
  const projects = [
    { key: "acme", focus_kinds: { off: ["answered", "ci_failed", "base_diverged"] as NeedsMeReason[] } },
    { key: "beta", focus_kinds: { off: [] } },
  ];
  it.each<[string, NeedsMeRow, string | undefined, boolean]>([
    ["an Answered Ticket in a Project with Answered off", row({ state: "Answered", reason: "answered" }), undefined, false],
    ["the same, with that Project chosen", row({ state: "Answered", reason: "answered" }), "acme", false],
    ["an Answered Ticket in another Project", row({ project: "beta", id: "beta-1", state: "Answered", reason: "answered" }), undefined, true],
    ["a reason left on in that Project", row({ state: "Failed", reason: "failed" }), undefined, true],
    ["a CI item of a reason off in its Project", row({ id: "", reason: "ci_failed", ci_item: { reason: "ci_failed", commit: "aaa", tickets: [], options: [] } }), undefined, false],
    ["a Base diverged item of a Project with it off", row({ id: "", reason: "base_diverged" }), undefined, false],
    ["a Base diverged item of another Project", row({ project: "beta", id: "", reason: "base_diverged" }), undefined, true],
    ["a CI item of that reason in another Project", row({ project: "beta", id: "", reason: "ci_failed", ci_item: { reason: "ci_failed", commit: "aaa", tickets: [], options: [] } }), undefined, true],
  ])("with %s, opens: %s", (_, r, project, open) => {
    const needsMe = { count: 1, groups: [{ reason: r.reason, tickets: [r] }] };
    expect(popupOpen({ option: "on", view: "inbox", project, needsMe, projects, dismissed: new Set(), open: false, otherPopup: false, typing: false })).toBe(open);
  });
});

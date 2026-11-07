// The contract cases for the human's Round actions (ticket 34): answer,
// dismiss_question and accept_recommended. apiContract (contract.ts) runs
// them with the same Driver as every other case.
import { describe, expect, it } from "vitest";
import type { Driver } from "./contract";
import "./actions/rounds";

export function roundContract(driver: () => Driver) {
  describe("Round actions", () => {
    const questions = [
      { title: "Which colour?", body: "Red or blue", recommended: "Blue" },
      { title: "Which size?", body: "S or M" },
    ];
    const posted = async () => {
      const d = driver();
      await d.createProject("acme", "Acme");
      const id = await d.createTicket("acme", "Pick a colour", "Ready");
      await d.postRound("acme", id, questions);
      return { d, id };
    };

    it("saves answers one by one, editable until the Round is complete, then the Ticket is Answered", async () => {
      const { d, id } = await posted();

      let t = await d.api.act("acme", id, "answer", { question: 1, text: "Red" });
      expect(t.state).toBe("Needs Answer");
      expect(t.rounds[0].questions.map((q) => [q.status, q.answer])).toEqual([
        ["answered", "Red"],
        ["open", undefined],
      ]);
      expect((await d.api.needsMe()).groups[0].tickets[0].round).toEqual({ number: 1, open: 1, questions: 2 });

      t = await d.api.act("acme", id, "answer", { question: 1, text: "Green" });
      expect(t.rounds[0].questions[0]).toMatchObject({ status: "answered", answer: "Green" });

      t = await d.api.act("acme", id, "answer", { question: 2, text: "M" });
      expect(t.state).toBe("Answered");
      expect(t.needs_me_reason).toBe("answered");
      expect((await d.api.needsMe()).groups.map((g) => g.reason)).toEqual(["answered"]);

      const late = await d.api.act("acme", id, "answer", { question: 1, text: "Blue" }).catch((e) => e);
      expect(late).toMatchObject({ status: 409, code: "action_not_allowed" });
    });

    it("refuses an empty answer and an unknown Question", async () => {
      const { d, id } = await posted();
      const empty = await d.api.act("acme", id, "answer", { question: 1, text: "  " }).catch((e) => e);
      expect(empty).toMatchObject({ status: 422, code: "invalid", field: "text" });
      const unknown = await d.api.act("acme", id, "answer", { question: 9, text: "x" }).catch((e) => e);
      expect(unknown).toMatchObject({ status: 404, code: "not_found" });
    });

    it("dismisses one Question without moving the Ticket, unless it was the last open one", async () => {
      const { d, id } = await posted();
      let t = await d.api.act("acme", id, "dismiss_question", { question: 2 });
      expect(t.state).toBe("Needs Answer");
      expect(t.rounds[0].questions[1].status).toBe("dismissed");

      t = await d.api.act("acme", id, "dismiss_question", { question: 1 });
      expect(t.state).toBe("Answered");
    });

    it("accepts every recommended answer at once, leaving Questions without one open", async () => {
      const { d, id } = await posted();
      let t = await d.api.act("acme", id, "accept_recommended", { round: 1 });
      expect(t.state).toBe("Needs Answer");
      expect(t.rounds[0].questions.map((q) => [q.status, q.answer])).toEqual([
        ["answered", "Blue"],
        ["open", undefined],
      ]);

      t = await d.api.act("acme", id, "answer", { question: 2, text: "S" });
      expect(t.state).toBe("Answered");
    });

    it("rolls up a Spec's Questions: each of its Tickets that has Rounds", async () => {
      const { d, id } = await posted();
      await d.createTicket("acme", "No questions", "Ready");
      const spec = (await d.api.getTicket("acme", id)).spec.id;

      const v = await d.api.specQuestions("acme", spec);
      expect(v.spec).toBe(spec);
      expect(v.tickets.map((t) => [t.id, t.title, t.state])).toEqual([[id, "Pick a colour", "Needs Answer"]]);
      expect(v.tickets[0].rounds[0].questions.map((q) => [q.number, q.title, q.status])).toEqual([
        [1, "Which colour?", "open"],
        [2, "Which size?", "open"],
      ]);

      const notSpec = await d.api.specQuestions("acme", id).catch((e) => e);
      expect(notSpec).toMatchObject({ status: 404, code: "not_found" });
    });
  });
}

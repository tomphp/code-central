// The human's Round actions: answer(question, text), dismiss_question
// (question) and accept_recommended(round). Question numbers run across
// the whole Ticket, so answer and dismiss_question name a Question, not a
// Round. Answers stay editable until the Round is complete; the action
// that closes its last open Question moves the Ticket to Answered.
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    answer: { body: { question: number; text: string }; result: TicketAcknowledgement };
    dismiss_question: { body: { question: number }; result: TicketAcknowledgement };
    accept_recommended: { body: { round: number }; result: TicketAcknowledgement };
  }
}

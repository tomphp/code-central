// new_fix_ticket: the human opens a Ready Ticket on a Ticket's Spec
// carrying the note. On a CI failed Ticket, the failed Ticket lands as
// with Land anyway (code-central-264); on a Spec's pull-request lead that
// is To merge under Pull Request + Land per Spec, the fix Ticket's accept
// is pushed to the Spec's pull request. Its result is the fix Ticket's
// acknowledgement. With several Repositories, repository names the CI
// item's, which the fix Ticket names.
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    new_fix_ticket: { body: { note: string; repository?: string }; result: TicketAcknowledgement };
  }
}

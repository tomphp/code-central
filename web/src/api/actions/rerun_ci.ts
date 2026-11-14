// rerun_ci: the human asks the CI Watcher to re-run the failed jobs of a
// CI failed Ticket's pushed commit, which puts it back to waiting on CI
// (code-central-265). Offered under both Merge + Push and Branch + Push.
import type { TicketAcknowledgement } from "../types";

declare module "../api" {
  interface TicketActions {
    rerun_ci: { body: Record<string, never>; result: TicketAcknowledgement };
  }
}
